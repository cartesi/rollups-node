// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type observationCallRPC struct {
	requests  chan rpc.BlockNumberOrHash
	consensus common.Address
}

func (*observationCallRPC) GetBlockByNumber(_ context.Context, number rpc.BlockNumber, _ bool) *types.Header {
	block := big.NewInt(int64(number))
	if !block.IsUint64() {
		return nil
	}
	return deliveryHeader(block.Uint64())
}

func (s *observationCallRPC) Call(_ context.Context, call struct {
	Input hexutil.Bytes
	Gas   *hexutil.Uint64
}, block rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	s.requests <- block
	if call.Gas != nil {
		return nil, errors.New("a binding view must leave simulation gas unspecified")
	}
	for _, meta := range []*bind.MetaData{iapplication.IApplicationMetaData, iconsensus.IConsensusMetaData} {
		contractABI, err := meta.GetAbi()
		if err != nil {
			return nil, err
		}
		method, err := contractABI.MethodById(call.Input[:4])
		if err != nil {
			continue
		}
		switch method.Name {
		case "getOutputsMerkleRootValidator":
			return method.Outputs.Pack(s.consensus)
		case "getClaim":
			return method.Outputs.Pack(iconsensus.IConsensusClaim{Status: claimStatusUnstaged, StagingBlockNumber: new(big.Int)})
		case "getNumberOfSubmittedClaims", "getNumberOfStagedClaims", "getNumberOfAcceptedClaims":
			return method.Outputs.Pack(big.NewInt(2))
		}
	}
	return nil, errors.New("unexpected observation binding call")
}

func TestFinalizedObservationBindingsRequireCanonicalHashesOnTheWire(t *testing.T) {
	app, epoch := deliveryApplication(1, "Authority")
	api := &observationCallRPC{requests: make(chan rpc.BlockNumberOrHash, 6), consensus: app.IConsensusAddress}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", api))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	facts := txsender.BlockRef{Number: 45, Hash: deliveryHeader(45).Hash()}
	cb := &claimerBlockchain{client: client, observed: facts}
	address, err := cb.getConsensusAddress(t.Context(), app, big.NewInt(45))
	require.NoError(t, err)
	require.Equal(t, app.IConsensusAddress, address)
	_, err = cb.getClaimStatus(t.Context(), app, epoch, big.NewInt(45))
	require.NoError(t, err)
	caller, err := iconsensus.NewIConsensusCaller(app.IConsensusAddress, txsender.ProtectedClient{Client: client})
	require.NoError(t, err)
	for _, counter := range []func(*bind.CallOpts, common.Address) (*big.Int, error){
		caller.GetNumberOfSubmittedClaims, caller.GetNumberOfStagedClaims, caller.GetNumberOfAcceptedClaims,
	} {
		_, err = cb.newOracle(app.IApplicationAddress, counter)(t.Context(), 45)
		require.NoError(t, err)
	}
	_, err = cb.newOracle(app.IApplicationAddress, caller.GetNumberOfSubmittedClaims)(t.Context(), 44)
	require.NoError(t, err)
	for index := range 6 {
		request := <-api.requests
		expected := facts.Hash
		if index == 5 {
			expected = deliveryHeader(44).Hash()
		}
		require.NotNil(t, request.BlockHash)
		require.Equal(t, expected, *request.BlockHash)
		require.True(t, request.RequireCanonical, "the actual RPC selector must reject orphaned branches")
		require.Nil(t, request.BlockNumber, "binding views must not fall back to numeric/latest reads")
	}
}
