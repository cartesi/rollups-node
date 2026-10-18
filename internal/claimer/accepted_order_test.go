// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

// Match claim events to the current epoch:
// the real claimerBlockchain event search runs against an in-process JSON-RPC
// server. A third party accepts epoch e+1 (block 40) before epoch e (block 45);
// observing e must return e's event, not the first acceptance after e.LastBlock.
// Oracle: AbstractConsensus emits ClaimAccepted(appContract, lastProcessedBlockNumber, ...)
// per epoch and acceptClaim is permissionless with no cross-epoch order.

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type acceptedOrderRPC struct {
	byHash map[common.Hash]uint64
	logs   []types.Log
}

func (s *acceptedOrderRPC) GetBlockByNumber(_ context.Context, number rpc.BlockNumber, _ bool) *types.Header {
	if number < 0 {
		return deliveryHeader(100)
	}
	return deliveryHeader(uint64(number))
}

func (s *acceptedOrderRPC) count(block uint64) int64 {
	var n int64
	for _, l := range s.logs {
		if l.BlockNumber <= block {
			n++
		}
	}
	return n
}

func (s *acceptedOrderRPC) Call(_ context.Context, call struct {
	Input hexutil.Bytes
	Gas   *hexutil.Uint64
}, block rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	var n uint64
	if hash, ok := block.Hash(); ok {
		known, found := s.byHash[hash]
		if !found {
			return nil, errors.New("unknown block hash")
		}
		n = known
	} else if number, ok := block.Number(); ok && number >= 0 {
		n = uint64(number)
	} else {
		return nil, errors.New("unsupported block selector")
	}
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	method, err := parsed.MethodById(call.Input[:4])
	if err != nil {
		return nil, errors.New("unexpected call")
	}
	switch method.Name {
	case "getNumberOfAcceptedClaims", "getNumberOfStagedClaims", "getNumberOfSubmittedClaims":
		return method.Outputs.Pack(big.NewInt(s.count(n)))
	default:
		return nil, errors.New("unexpected counter " + method.Name)
	}
}

type acceptedOrderFilter struct {
	FromBlock *hexutil.Big     `json:"fromBlock"`
	ToBlock   *hexutil.Big     `json:"toBlock"`
	Address   []common.Address `json:"address"`
	Topics    [][]common.Hash  `json:"topics"`
}

func (s *acceptedOrderRPC) GetLogs(_ context.Context, arg acceptedOrderFilter) ([]types.Log, error) {
	from, to := arg.FromBlock.ToInt().Uint64(), arg.ToBlock.ToInt().Uint64()
	var out []types.Log
	for _, l := range s.logs {
		if l.BlockNumber >= from && l.BlockNumber <= to {
			out = append(out, l)
		}
	}
	return out, nil
}

func TestClaimEventScansMatchCurrentEpochAfterLaterEpochFirst(t *testing.T) {
	for _, name := range []string{"ClaimAccepted", "ClaimStaged", claimSubmittedEventName} {
		for _, divergent := range []bool{false, true} {
			rootCase := "matching roots"
			if divergent {
				rootCase = "different roots for current epoch"
			}
			t.Run(name+"/"+rootCase, func(t *testing.T) {
				app, epoch := deliveryApplication(1, "Authority")
				epoch.LastBlock = 20
				root := *epoch.TxBufferDataBlock
				if divergent {
					root[0] ^= 1
				}
				api := &acceptedOrderRPC{byHash: map[common.Hash]uint64{}}
				for n := uint64(0); n <= 100; n++ {
					api.byHash[deliveryHeader(n).Hash()] = n
				}
				for i, event := range []struct {
					epochBlock, block uint64
				}{{30, 40}, {epoch.LastBlock, 45}} {
					e := *epoch
					e.LastBlock = event.epochBlock
					e.TxBufferDataBlock = &root
					log := *claimEventLog(app, &e, name)
					log.BlockNumber, log.BlockHash = event.block, deliveryHeader(event.block).Hash()
					log.TxHash = common.BigToHash(big.NewInt(int64(i + 1)))
					api.logs = append(api.logs, log)
				}
				server := rpc.NewServer()
				require.NoError(t, server.RegisterName("eth", api))
				t.Cleanup(server.Stop)
				client := ethclient.NewClient(rpc.DialInProc(server))
				t.Cleanup(client.Close)
				cb := &claimerBlockchain{client: client, backend: txsender.ProtectedClient{Client: client},
					observed: txsender.BlockRef{Number: 100, Hash: deliveryHeader(100).Hash()}}
				var lastBlock *big.Int
				var currentRoot common.Hash
				var eventBlock uint64
				var matches, complete bool
				switch name {
				case "ClaimAccepted":
					_, current, _, err := cb.findClaimAcceptedEventAndSucc(t.Context(), app, epoch, epoch.LastBlock+1, 100)
					require.NoError(t, err)
					require.NotNil(t, current)
					lastBlock = current.LastProcessedBlockNumber
					currentRoot, eventBlock = current.OutputsMerkleRoot, current.Raw.BlockNumber
					matches, complete = claimAcceptedEventMatches(app, epoch, current)
				case "ClaimStaged":
					_, current, _, err := cb.findClaimStagedEventAndSucc(t.Context(), app, epoch, epoch.LastBlock+1, 100)
					require.NoError(t, err)
					require.NotNil(t, current)
					lastBlock = current.LastProcessedBlockNumber
					currentRoot, eventBlock = current.OutputsMerkleRoot, current.Raw.BlockNumber
					matches, complete = claimStagedEventMatches(app, epoch, current)
				case claimSubmittedEventName:
					_, current, err := cb.findClaimSubmittedEventAndSucc(t.Context(), app, epoch, epoch.LastBlock+1, 100)
					require.NoError(t, err)
					require.NotEmpty(t, current)
					lastBlock = current[0].LastProcessedBlockNumber
					currentRoot, eventBlock = current[0].OutputsMerkleRoot, current[0].Raw.BlockNumber
					matches, complete = claimSubmittedEventMatches(app, epoch, current[0])
				}
				require.Equal(t, epoch.LastBlock, lastBlock.Uint64(), "skip the later epoch's earlier event")
				require.Equal(t, uint64(45), eventBlock)
				require.Equal(t, root, currentRoot, "a different claim for this epoch must remain visible")
				require.True(t, complete)
				require.Equal(t, !divergent, matches)
			})
		}
	}
}
