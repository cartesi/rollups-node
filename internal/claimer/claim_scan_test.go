// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"slices"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

// Event data is encoded directly from alpha.10's published signature and ABI
// word layout. These are observation fixtures, not application execution proofs.
const claimCounterHistoryCase = "counter history"

const claimWrongHashCase = "wrong hash"

var (
	watchStageTopic  = crypto.Keccak256Hash([]byte("ClaimStaged(address,uint256,bytes32,bytes32)"))
	watchAcceptTopic = crypto.Keccak256Hash([]byte("ClaimAccepted(address,uint256,bytes32,bytes32)"))
)

type watchCallArgs struct {
	To    common.Address
	Input hexutil.Bytes
}

type watchRPCHistory struct {
	app, consensus   common.Address
	applicationFrom  uint64
	bindingError     error
	head             uint64
	headers          map[uint64]*types.Header
	byHash           map[common.Hash]uint64
	logs             []types.Log
	staged           *big.Int
	accepted         *big.Int
	clock, period    *big.Int
	claimAt          func(*big.Int) (uint8, *big.Int)
	claimStatus      uint8
	machine, roots   common.Hash
	noCode           map[uint64]bool
	codeError        error
	noCodeAt         func(uint64) bool
	counterAt        func(uint64) (*big.Int, *big.Int, error)
	stagedReads      int
	headerReads      int
	claimReads       int
	codeReads        int
	getLogs          func(uint64, uint64) ([]types.Log, error)
	selectors        []rpc.BlockNumberOrHash
	filters          []acceptedOrderFilter
	foreclosedAt     func(uint64) (bool, error)
	foreclosureReads []uint64
}

func (s *watchRPCHistory) header(height uint64) *types.Header {
	if header := s.headers[height]; header != nil {
		return header
	}
	header := deliveryHeader(height)
	s.headers[height], s.byHash[header.Hash()] = header, height
	return header
}

func (s *watchRPCHistory) codeAbsent(block uint64) bool {
	return s.noCode[block] || (s.noCodeAt != nil && s.noCodeAt(block))
}

func (s *watchRPCHistory) counters(block uint64) (*big.Int, *big.Int, error) {
	if s.counterAt != nil {
		return s.counterAt(block)
	}
	// Independent oracle: this fixture's first stage is at40 even when a
	// faulty provider omits, duplicates or fabricates the returned logs.
	if block < 40 {
		return new(big.Int), new(big.Int), nil
	}
	return s.staged, s.accepted, nil
}

func (s *watchRPCHistory) GetBlockByNumber(_ context.Context, n rpc.BlockNumber, _ bool) *types.Header {
	s.headerReads++
	if n < 0 {
		return s.header(s.head)
	}
	return s.header(uint64(n))
}

func (s *watchRPCHistory) at(block rpc.BlockNumberOrHash) (uint64, error) {
	s.selectors = append(s.selectors, block)
	if block.BlockHash == nil || block.BlockNumber != nil || !block.RequireCanonical {
		return 0, errors.New("watch reads must require a canonical hash")
	}
	n, ok := s.byHash[*block.BlockHash]
	if !ok {
		return 0, errors.New("unknown fixture hash")
	}
	return n, nil
}

func (s *watchRPCHistory) GetCode(_ context.Context, address common.Address, block rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	n, err := s.at(block)
	if err != nil {
		return nil, err
	}
	s.codeReads++
	if s.codeError != nil {
		return nil, s.codeError
	}
	if address == s.app {
		if n < s.applicationFrom {
			return hexutil.Bytes{}, nil
		}
		return hexutil.Bytes{0x60, 0x00}, nil
	}
	if s.codeAbsent(n) {
		return hexutil.Bytes{}, nil
	}
	return hexutil.Bytes{0x60, 0x00}, nil
}

func watchWord(n *big.Int) []byte { return n.FillBytes(make([]byte, 32)) }

func (s *watchRPCHistory) Call(_ context.Context, call watchCallArgs, block rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	height, err := s.at(block)
	if err != nil {
		return nil, err
	}
	if len(call.Input) < 4 {
		return nil, errors.New("missing selector")
	}
	selector := call.Input[:4]
	is := func(signature string) bool { return bytes.Equal(selector, crypto.Keccak256([]byte(signature))[:4]) }
	if is("getDeploymentBlockNumber()") {
		if call.To != s.app {
			return nil, errors.New("deployment read used a different application")
		}
		return watchWord(new(big.Int).SetUint64(s.applicationFrom)), nil
	}
	if is("getOutputsMerkleRootValidator()") {
		if call.To != s.app {
			return nil, errors.New("binding read used a different application")
		}
		if s.bindingError != nil {
			return nil, s.bindingError
		}
		if height < s.applicationFrom {
			return hexutil.Bytes{}, nil
		}
		return common.LeftPadBytes(s.consensus.Bytes(), 32), nil
	}
	if is("isForeclosed()") {
		if call.To != s.app {
			return nil, errors.New("foreclosure read used a different application")
		}
		s.foreclosureReads = append(s.foreclosureReads, height)
		closed, readErr := s.foreclosedAt(height)
		if readErr != nil {
			return nil, readErr
		}
		value := new(big.Int)
		if closed {
			value.SetUint64(1)
		}
		return watchWord(value), nil
	}
	if call.To != s.consensus {
		return nil, errors.New("claim read used a different consensus")
	}
	if s.codeAbsent(height) {
		return hexutil.Bytes{}, nil
	}
	switch {
	case is("getNumberOfStagedClaims(address)"):
		s.stagedReads++
		staged, _, err := s.counters(height)
		if err != nil {
			return nil, err
		}
		return watchWord(staged), nil
	case is("getNumberOfAcceptedClaims(address)"):
		_, accepted, err := s.counters(height)
		if err != nil {
			return nil, err
		}
		return watchWord(accepted), nil
	case is("getClaimStagingPeriod()"):
		return watchWord(s.period), nil
	case is("getClaim(address,uint256,bytes32)"):
		s.claimReads++
		if len(call.Input) != 100 || common.BytesToAddress(call.Input[4:36]) != s.app {
			return nil, errors.New("invalid claim identity")
		}
		status, clock := s.claimStatus, s.clock
		if s.claimAt != nil {
			status, clock = s.claimAt(new(big.Int).SetBytes(call.Input[36:68]))
		}
		result := append(watchWord(new(big.Int).SetUint64(uint64(status))), watchWord(clock)...)
		return append(result, s.roots[:]...), nil
	default:
		return nil, fmt.Errorf("unexpected selector %x", selector)
	}
}

func (s *watchRPCHistory) GetLogs(_ context.Context, filter acceptedOrderFilter) ([]types.Log, error) {
	s.filters = append(s.filters, filter)
	from, to := filter.FromBlock.ToInt().Uint64(), filter.ToBlock.ToInt().Uint64()
	if s.getLogs != nil {
		return s.getLogs(from, to)
	}
	result := make([]types.Log, 0)
	for _, log := range s.logs {
		if log.BlockNumber < from || log.BlockNumber > to || len(filter.Address) > 0 && !slices.Contains(filter.Address, log.Address) {
			continue
		}
		matches := true
		for index, topics := range filter.Topics {
			if len(topics) > 0 && (index >= len(log.Topics) || !slices.Contains(topics, log.Topics[index])) {
				matches = false
				break
			}
		}
		if matches {
			result = append(result, log)
		}
	}
	return result, nil
}

func watchLog(s *watchRPCHistory, topic common.Hash, covered *big.Int, block uint64, index uint) types.Log {
	data := append(watchWord(covered), s.roots[:]...)
	data = append(data, s.machine[:]...)
	return types.Log{
		Address: s.consensus, Topics: []common.Hash{topic, common.BytesToHash(s.app.Bytes())}, Data: data,
		BlockNumber: block, BlockHash: s.header(block).Hash(), TxHash: common.HexToHash("0x1234"),
		TxIndex: 1, Index: index,
	}
}

func newClaimScanHistory(t *testing.T) (*claimerBlockchain, *watchRPCHistory, *model.Application) {
	t.Helper()
	app := makeApplication()
	app.ID, app.Enabled, app.IInputBoxBlock = 1, true, 90
	api := &watchRPCHistory{app: app.IApplicationAddress, consensus: app.IConsensusAddress, head: 100,
		headers: map[uint64]*types.Header{}, byHash: map[common.Hash]uint64{}, noCode: map[uint64]bool{},
		staged: big.NewInt(1), accepted: new(big.Int), clock: big.NewInt(40), period: big.NewInt(7),
		claimStatus: claimStatusStaged, machine: common.HexToHash("0x33"), roots: common.HexToHash("0x22")}
	for height := uint64(0); height <= api.head; height++ {
		api.header(height)
	}
	api.logs = []types.Log{watchLog(api, watchStageTopic, big.NewInt(29), 40, 0)}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", api))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	chain := &claimerBlockchain{client: client, backend: txsender.ProtectedClient{Client: client},
		observed: txsender.BlockRef{Number: api.head, Hash: api.header(api.head).Hash()},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil))}
	return chain, api, app
}

func TestIndependentClaimScanReadsBeforeInputBoxAndCertifiesBothStreams(t *testing.T) {
	chain, api, app := newClaimScanHistory(t)
	api.accepted.SetUint64(1)
	api.claimStatus = claimStatusAccepted
	api.logs = append(api.logs, watchLog(api, watchAcceptTopic, big.NewInt(29), 40, 1))
	batch, err := chain.scanClaims(t.Context(), app, 0, nil, 0)
	require.NoError(t, err)
	require.NotNil(t, batch)
	require.EqualValues(t, 100, batch.Read.At.Number)
	require.Equal(t, big.NewInt(1), batch.Read.Staged)
	require.Equal(t, big.NewInt(1), batch.Read.Accepted)
	require.Len(t, batch.Claims, 1)
	claim := batch.Claims[0]
	require.Equal(t, "29", claim.Key.Covered)
	require.Equal(t, api.machine, claim.Key.Machine)
	require.Equal(t, api.roots, claim.Outputs)
	require.Equal(t, big.NewInt(40), claim.StagingBlock)
	require.Equal(t, big.NewInt(7), claim.Period)
	require.NotNil(t, claim.Stage)
	require.NotNil(t, claim.Acceptance)
	require.EqualValues(t, 40, claim.Stage.BlockNumber, "stage predates the registered InputBox floor of 90")
	require.EqualValues(t, 0, claim.Stage.Index)
	require.EqualValues(t, 1, claim.Acceptance.Index)
	filters := len(api.filters)
	headers, selectors := api.headerReads, len(api.selectors)
	repeat, err := chain.scanClaims(t.Context(), app, batch.Read.At.Number, &batch.Read, 0)
	require.NoError(t, err)
	require.Nil(t, repeat)
	require.Len(t, api.filters, filters, "same head does not reread a log range")
	require.Equal(t, headers, api.headerReads, "the exact certified head needs no discovery RPC")
	require.Len(t, api.selectors, selectors, "the exact certified head needs no contract read")
}

func TestIndependentClaimScanAcceptancesOutsidePendingProjection(t *testing.T) {
	chain, api, app := newClaimScanHistory(t)
	api.accepted.SetUint64(1)
	api.counterAt = func(n uint64) (*big.Int, *big.Int, error) {
		accepted := new(big.Int)
		if n >= 41 {
			accepted.SetUint64(1)
		}
		staged := new(big.Int)
		if n >= 40 {
			staged.SetUint64(1)
		}
		return staged, accepted, nil
	}
	api.logs = []types.Log{watchLog(api, watchAcceptTopic, big.NewInt(29), 41, 0)}
	before := claimRead{At: txsender.BlockRef{Number: 40, Hash: api.header(40).Hash()}, Staged: big.NewInt(1), Accepted: new(big.Int)}
	batch, err := chain.scanClaims(t.Context(), app, 40, &before, 0)
	require.NoError(t, err)
	require.NotNil(t, batch)
	require.Len(t, batch.Claims, 1)
	require.Nil(t, batch.Claims[0].Stage)
	require.NotNil(t, batch.Claims[0].Acceptance)
	require.Equal(t, big.NewInt(1), batch.Read.Accepted)
	require.Zero(t, before.Accepted.Sign(), "scanner must not mutate the previous certified point")
}

func TestIndependentClaimScanRejectsIncompleteAndIncoherentObservations(t *testing.T) {
	for _, test := range []string{
		"omitted stage", "duplicate", "removed", claimWrongHashCase, "wrong app", "wrong root", "binding", claimCounterHistoryCase,
	} {
		t.Run(test, func(t *testing.T) {
			chain, api, app := newClaimScanHistory(t)
			want := txsender.ErrInconclusive
			switch test {
			case "omitted stage":
				api.logs = nil
			case "duplicate":
				api.logs = append(api.logs, api.logs[0])
			case "removed":
				api.logs[0].Removed = true
			case claimWrongHashCase:
				api.logs[0].BlockHash = common.HexToHash("0xdead")
				want = txsender.ErrNonCanonical
			case "wrong app":
				api.logs[0].Topics[1] = common.HexToHash("0xbeef")
			case "wrong root":
				api.roots = common.HexToHash("0xabc") // getClaim disagrees with the log
			case "binding":
				app.IConsensusAddress = common.HexToAddress("0xdead")
			case claimCounterHistoryCase:
				want = errors.New("historical state unavailable")
				api.counterAt = func(_ uint64) (*big.Int, *big.Int, error) {
					return nil, nil, want
				}
			}
			batch, err := chain.scanClaims(t.Context(), app, 0, nil, 0)
			if test == claimCounterHistoryCase {
				require.ErrorContains(t, err, want.Error())
			} else {
				require.ErrorIs(t, err, want)
			}
			require.Nil(t, batch, "failure cannot return an advanceable range")
		})
	}
}

func TestIndependentClaimScanBoundAndEmptyTail(t *testing.T) {
	t.Run("zero range has an internal cap", func(t *testing.T) {
		chain, api, app := newClaimScanHistory(t)
		api.head = 10000
		chain.observed = txsender.BlockRef{Number: api.head, Hash: api.header(api.head).Hash()}
		api.counterAt = func(n uint64) (*big.Int, *big.Int, error) {
			if n < 9000 {
				return new(big.Int), new(big.Int), nil
			}
			return big.NewInt(1), new(big.Int), nil
		}
		api.logs = nil
		batch, err := chain.scanClaims(t.Context(), app, 0, nil, 0)
		require.NoError(t, err)
		require.EqualValues(t, claimScanBlockLimit, batch.Read.At.Number)
		require.Len(t, api.filters, 1)
	})
	t.Run("configured smaller range", func(t *testing.T) {
		chain, api, app := newClaimScanHistory(t)
		batch, err := chain.scanClaims(t.Context(), app, 0, nil, 5)
		require.NoError(t, err)
		require.EqualValues(t, 5, batch.Read.At.Number)
		require.Empty(t, batch.Claims)
		require.Len(t, api.filters, 1)
	})
	t.Run("both counters prove empty tail", func(t *testing.T) {
		chain, api, app := newClaimScanHistory(t)
		api.staged.SetUint64(0)
		api.logs = nil
		batch, err := chain.scanClaims(t.Context(), app, 0, nil, 1)
		require.NoError(t, err)
		require.EqualValues(t, 100, batch.Read.At.Number)
		require.Empty(t, api.filters, "empty tail can jump to head without scanning every capped range")
	})
}

func TestIndependentClaimScanYoungDeploymentAndUnavailableHistory(t *testing.T) {
	t.Run("young app does not need predeployment state", func(t *testing.T) {
		chain, api, app := newClaimScanHistory(t)
		api.applicationFrom = 95
		api.clock.SetUint64(99)
		api.logs = []types.Log{watchLog(api, watchStageTopic, big.NewInt(89), 99, 0)}
		api.counterAt = func(n uint64) (*big.Int, *big.Int, error) {
			if n < 95 {
				return nil, nil, errors.New("predeployment state is pruned")
			}
			if n < 99 {
				return new(big.Int), new(big.Int), nil
			}
			return big.NewInt(1), new(big.Int), nil
		}
		batch, err := chain.scanClaims(t.Context(), app, 0, nil, 0)
		require.NoError(t, err)
		require.Len(t, batch.Claims, 1)
		require.EqualValues(t, 95, api.filters[0].FromBlock.ToInt().Uint64())
	})
	t.Run("deploy not visible is a wait", func(t *testing.T) {
		chain, api, app := newClaimScanHistory(t)
		api.applicationFrom = 101
		batch, err := chain.scanClaims(t.Context(), app, 0, nil, 0)
		require.ErrorIs(t, err, errClaimDeploymentPending)
		require.Nil(t, batch)
	})
	t.Run("confirmed historical no-code is not missing state", func(t *testing.T) {
		chain, api, app := newClaimScanHistory(t)
		api.noCodeAt = func(n uint64) bool { return n < 45 }
		api.clock.SetUint64(60)
		api.logs = []types.Log{watchLog(api, watchStageTopic, big.NewInt(49), 60, 0)}
		api.counterAt = func(n uint64) (*big.Int, *big.Int, error) {
			if n < 60 {
				return new(big.Int), new(big.Int), nil
			}
			return big.NewInt(1), new(big.Int), nil
		}
		batch, err := chain.scanClaims(t.Context(), app, 0, nil, 10)
		require.NoError(t, err)
		require.EqualValues(t, 10, batch.Read.At.Number)
		require.Empty(t, batch.Claims)
		require.Zero(t, batch.Read.Staged.Sign())
	})
	t.Run("old checkpoint needs its actual historical state", func(t *testing.T) {
		chain, api, app := newClaimScanHistory(t)
		api.applicationFrom = 5
		missing := errors.New("state at checkpoint is pruned")
		api.counterAt = func(n uint64) (*big.Int, *big.Int, error) {
			if n == 20 {
				return nil, nil, missing
			}
			return big.NewInt(1), new(big.Int), nil
		}
		batch, err := chain.scanClaims(t.Context(), app, 20, nil, 0)
		require.ErrorContains(t, err, missing.Error())
		require.Nil(t, batch)
	})
}

func TestIndependentClaimClosureAuthenticatesFlagAtCertifiedRead(t *testing.T) {
	const (
		latestCase = "latest"
		safeCase   = "safe"
	)
	for _, scenario := range []string{
		"true", "false before actual foreclosure", latestCase, safeCase,
		"future reference", claimWrongHashCase, "RPC error", "post-read reorg",
	} {
		t.Run(scenario, func(t *testing.T) {
			chain, api, app := newClaimScanHistory(t)
			chain.defaultBlock = model.DefaultBlock_Finalized
			app.ForecloseBlock = 1 // a stale event hint cannot prove the storage gate
			ref := txsender.BlockRef{Number: 40, Hash: api.header(40).Hash()}
			wantClosed := true
			var wantErr error
			api.foreclosedAt = func(block uint64) (bool, error) { return block >= 40, nil }
			switch scenario {
			case "false before actual foreclosure":
				ref = txsender.BlockRef{Number: 29, Hash: api.header(29).Hash()}
				wantClosed = false
			case latestCase:
				chain.defaultBlock = model.DefaultBlock_Latest
				wantErr = txsender.ErrInconclusive
			case safeCase:
				chain.defaultBlock = model.DefaultBlock_Safe
				wantErr = txsender.ErrInconclusive
			case "future reference":
				ref = txsender.BlockRef{Number: 101, Hash: api.header(101).Hash()}
				wantErr = txsender.ErrInconclusive
			case claimWrongHashCase:
				ref.Hash = common.HexToHash("0xdead")
				wantErr = txsender.ErrNonCanonical
			case "RPC error":
				wantErr = errors.New("foreclosure state unavailable")
				api.foreclosedAt = func(uint64) (bool, error) { return false, wantErr }
			case "post-read reorg":
				ref = chain.observed // The post-call check must not reuse the Tick-start head.
				wantErr = txsender.ErrNonCanonical
				api.foreclosedAt = func(block uint64) (bool, error) {
					header := *api.header(block)
					header.Extra = []byte("replacement branch")
					api.headers[block] = &header
					return true, nil
				}
			}
			closed, err := chain.claimForeclosureFinalized(t.Context(), app, ref)
			if wantErr == nil {
				require.NoError(t, err)
				require.Equal(t, wantClosed, closed)
				require.Equal(t, []uint64{ref.Number}, api.foreclosureReads, "the gate must use the certified read, not head 100")
			} else {
				require.ErrorContains(t, err, wantErr.Error())
				require.False(t, closed)
			}
		})
	}
}
