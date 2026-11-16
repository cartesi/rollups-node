// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testJoinedCounterMethod   = "getCommitmentJoinedCount"
	testCreatedCounterMethod  = "getMatchCreatedCount"
	testAdvancedCounterMethod = "getMatchAdvancedCount"
	testLeafCounterMethod     = "getLeafMatchSealedCount"
	testDeletedCounterMethod  = "getMatchDeletedCount"
	testChildCounterMethod    = "getNewInnerTournamentCount"
	testRecoveryMethod        = "bondRecovery"
	testCompleteInterval      = "complete"
)

type discoveryCall struct {
	name  string
	block uint64
}

type discoveryRPC struct {
	segmentedTournamentLogs
	abi         *abi.ABI
	head        uint64
	counts      map[string]func(uint64) *big.Int
	outputs     map[string][]any
	recoveredAt uint64
	callError   error
	calls       []discoveryCall
}

func (s *discoveryRPC) Call(
	_ context.Context, call map[string]json.RawMessage, at rpc.BlockNumberOrHash,
) (hexutil.Bytes, error) {
	var input hexutil.Bytes
	if err := json.Unmarshal(call["input"], &input); err != nil {
		return nil, err
	}
	method, err := s.abi.MethodById(input)
	if err != nil {
		return nil, err
	}
	block := s.head
	if number, ok := at.Number(); ok {
		if number < 0 {
			return nil, errors.New("historical sample used a moving block tag")
		}
		block = uint64(number)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, discoveryCall{method.Name, block})
	if s.callError != nil {
		return nil, s.callError
	}
	if strings.HasPrefix(method.Name, "get") && strings.HasSuffix(method.Name, "Count") {
		value := new(big.Int)
		if read := s.counts[method.Name]; read != nil {
			value = read(block)
		}
		return method.Outputs.Pack(value)
	}
	if method.Name == testRecoveryMethod {
		disposition := uint8(0)
		if s.recoveredAt > 0 && block >= s.recoveredAt {
			disposition = 3
		}
		return method.Outputs.Pack(disposition, common.Address{}, new(big.Int))
	}
	values, ok := s.outputs[method.Name]
	if !ok {
		return nil, errors.New("unexpected view: " + method.Name)
	}
	return method.Outputs.Pack(values...)
}

func newDiscoveryAdapter(t *testing.T, address common.Address) (TournamentAdapter, *discoveryRPC) {
	t.Helper()
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	source := &discoveryRPC{abi: contractABI, head: 1000, counts: map[string]func(uint64) *big.Int{}, outputs: map[string][]any{}}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", source))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	adapter, err := NewITournamentAdapter(address, client)
	require.NoError(t, err)
	return adapter, source
}

func stepCount(at uint64, before, after int64) func(uint64) *big.Int {
	return func(block uint64) *big.Int {
		if block < at {
			return big.NewInt(before)
		}
		return big.NewInt(after)
	}
}

func TestTournamentDiscoveryUnionsEveryCounterStream(t *testing.T) {
	adapter, source := newDiscoveryAdapter(t, common.HexToAddress("0x1234"))
	// This checks the six independent locator inputs. Decoding and projection
	// completeness have separate fixtures with actual event responses.
	for _, name := range []string{testJoinedCounterMethod, testCreatedCounterMethod, testAdvancedCounterMethod,
		testLeafCounterMethod, testDeletedCounterMethod, testChildCounterMethod} {
		source.counts[name] = stepCount(10, 0, 1)
	}
	source.counts[testAdvancedCounterMethod] = stepCount(600, 0, 1)
	current := StructuralEventCounts{big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(1)}
	_, err := adapter.RetrieveAllEvents(t.Context(), TournamentEventQuery{Start: 10, End: 1000,
		Previous: zeroStructuralEventCounts(), Current: current})
	require.NoError(t, err)
	require.Equal(t, [][2]uint64{{10, 10}, {600, 600}}, source.queries)
	for _, call := range source.calls {
		require.NotEqual(t, uint64(1000), call.block, "the already-read head count is reused")
	}
	require.Len(t, source.queries, 2, "overlapping stream hits fetch a block only once")
}

func TestTournamentDiscoveryUsesEachCounterGetter(t *testing.T) {
	for index, name := range []string{testJoinedCounterMethod, testCreatedCounterMethod, testAdvancedCounterMethod,
		testLeafCounterMethod, testDeletedCounterMethod, testChildCounterMethod} {
		t.Run(name, func(t *testing.T) {
			adapter, source := newDiscoveryAdapter(t, common.HexToAddress("0x1234"))
			current := zeroStructuralEventCounts()
			fields := []*big.Int{current.CommitmentJoined, current.MatchCreated, current.MatchAdvanced,
				current.LeafMatchSealed, current.MatchDeleted, current.NewInnerTournament}
			fields[index].SetInt64(1)
			source.counts[name] = stepCount(10, 0, 1)
			_, err := adapter.RetrieveAllEvents(t.Context(), TournamentEventQuery{Start: 10, End: 1000,
				Previous: zeroStructuralEventCounts(), Current: current})
			require.NoError(t, err)
			require.Equal(t, [][2]uint64{{10, 10}}, source.queries)
			require.Equal(t, []discoveryCall{{name, 10}}, source.calls)
		})
	}
}

func TestTournamentDiscoveryContinuesCountedMovesAndOwnRecovery(t *testing.T) {
	for _, test := range []struct {
		name     string
		block    uint64
		recovery bool
	}{
		{"first unread advance", 51, false},
		{"later advance", 600, false},
		{"isolated recovery", 600, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			recovery := test.recovery
			adapter, source := newDiscoveryAdapter(t, common.HexToAddress("0x1234"))
			before := zeroStructuralEventCounts()
			before.CommitmentJoined.SetInt64(2)
			before.MatchCreated.SetInt64(1)
			current := zeroStructuralEventCounts()
			current.CommitmentJoined.SetInt64(2)
			current.MatchCreated.SetInt64(1)
			block := test.block
			corpus := currentTournamentLogs(t)
			if recovery {
				before.CommitmentJoined.SetInt64(1)
				before.MatchCreated.SetInt64(0)
				current.CommitmentJoined.SetInt64(1)
				current.MatchCreated.SetInt64(0)
				source.recoveredAt = block
				source.logs = []types.Log{corpus[7]}
			} else {
				before.MatchAdvanced.SetInt64(4)
				current.MatchAdvanced.SetInt64(5)
				source.counts[testAdvancedCounterMethod] = stepCount(block, 4, 5)
				source.logs = []types.Log{corpus[2], corpus[6]}
			}
			for i := range source.logs {
				source.logs[i].BlockNumber = block
			}
			events, err := adapter.RetrieveAllEvents(t.Context(), TournamentEventQuery{Start: 51, End: 1000,
				Previous: before, Current: current, FindRecovery: recovery})
			require.NoError(t, err)
			require.Equal(t, [][2]uint64{{block, block}}, source.queries)
			require.NoError(t, validateTournamentEventCounts(before, current, events))
			beforeBond, afterBond := model.BondDispositionTournamentRunning, model.BondDispositionTournamentRunning
			if recovery {
				beforeBond, afterBond = model.BondDispositionRecoverable, model.BondDispositionRecovered
			}
			require.NoError(t, validateTournamentFinancialEvents(beforeBond, afterBond, events))
			for _, call := range source.calls {
				want := testAdvancedCounterMethod
				if recovery {
					want = testRecoveryMethod
				}
				require.Equal(t, want, call.name, "a sample reads only its own stream")
			}
		})
	}
}

func TestTournamentDiscoverySkipsUnchangedHistory(t *testing.T) {
	adapter, source := newDiscoveryAdapter(t, common.HexToAddress("0x1234"))
	events, err := adapter.RetrieveAllEvents(t.Context(), TournamentEventQuery{Start: 51, End: 1000,
		Previous: zeroStructuralEventCounts(), Current: zeroStructuralEventCounts()})
	require.NoError(t, err)
	require.Equal(t, &TournamentEvents{}, events)
	require.Empty(t, source.calls)
	require.Empty(t, source.queries)
}

func TestTournamentDiscoveryRejectsObservedCounterContradictions(t *testing.T) {
	for _, test := range []struct {
		name       string
		seed, head int64
		values     map[uint64][]int64
	}{
		{"seed regression", 2, 1, nil},
		{"interior decrease", 0, 7, map[uint64][]int64{10: {6}, 11: {5}, 12: {7}}},
		{"decrease within endpoints", 0, 3, map[uint64][]int64{10: {1}, 11: {2}, 12: {1}}},
		{"conflicting repeated block", 0, 2, map[uint64][]int64{10: {0, 1}, 11: {1}, 12: {1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var hits []uint64
			read := func(_ context.Context, block uint64) (*big.Int, error) {
				values := test.values[block]
				require.NotEmpty(t, values)
				value := values[0]
				if len(values) > 1 {
					test.values[block] = values[1:]
				}
				return big.NewInt(value), nil
			}
			err := findTournamentTransitions(t.Context(), TournamentEventQuery{Start: 10, End: 13},
				big.NewInt(test.seed), big.NewInt(test.head), read, func(block uint64) error { hits = append(hits, block); return nil })
			require.ErrorIs(t, err, ethutil.ErrMonotonicViolation)
		})
	}
	for _, value := range []*big.Int{nil, big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 256)} {
		err := findTournamentTransitions(t.Context(), TournamentEventQuery{Start: 10, End: 13}, new(big.Int), big.NewInt(1),
			func(context.Context, uint64) (*big.Int, error) { return value, nil }, func(uint64) error { return nil })
		require.Error(t, err)
	}
}

func TestTournamentDiscoveryFailureReturnsNoPartialEvents(t *testing.T) {
	adapter, source := newDiscoveryAdapter(t, common.HexToAddress("0x1234"))
	source.counts[testAdvancedCounterMethod] = stepCount(600, 0, 1)
	source.failAt = new(uint64(600))
	current := zeroStructuralEventCounts()
	current.MatchAdvanced.SetInt64(1)
	events, err := adapter.RetrieveAllEvents(t.Context(), TournamentEventQuery{Start: 10, End: 1000,
		Previous: zeroStructuralEventCounts(), Current: current})
	require.ErrorContains(t, err, "interval unavailable")
	require.Nil(t, events)
}

func TestTournamentDiscoveryCounterRPCErrorDoesNotFetchLogs(t *testing.T) {
	adapter, source := newDiscoveryAdapter(t, common.HexToAddress("0x1234"))
	source.callError = errors.New("historical count unavailable")
	current := zeroStructuralEventCounts()
	current.MatchAdvanced.SetInt64(1)
	events, err := adapter.RetrieveAllEvents(t.Context(), TournamentEventQuery{Start: 10, End: 1000,
		Previous: zeroStructuralEventCounts(), Current: current})
	require.ErrorContains(t, err, "historical count unavailable")
	require.Nil(t, events)
	require.Empty(t, source.queries)
}

func TestCounterDiscoveryCommitsOnlyCompleteServiceInterval(t *testing.T) {
	for _, missing := range []string{testCompleteInterval, "join omitted", "standing omitted"} {
		t.Run(missing, func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			epoch := checkpointEpoch(0, "0x1234")
			address := *epoch.TournamentAddress
			adapter, source := newDiscoveryAdapter(t, address)
			source.head = 100
			source.counts[testJoinedCounterMethod] = stepCount(51, 0, 1)
			source.outputs["tournamentDescriptor"] = []any{itournament.ITournamentTournamentDescriptor{
				BaseCycle: new(big.Int), StartInstant: 10}}
			source.outputs["tournamentStanding"] = []any{itournament.ITournamentTournamentStandingView{
				Standing: 1, HasCandidate: true, Candidate: *epoch.Commitment}}
			source.outputs["commitmentStanding"] = []any{itournament.ITournamentCommitmentStandingView{
				Joined: true, FinalState: *epoch.MachineHash}}
			corpus := currentTournamentLogs(t)
			source.logs = []types.Log{corpus[0], corpus[8]}
			joined := source.abi.Events["CommitmentJoined"]
			data, err := joined.Inputs.NonIndexed().Pack(*epoch.MachineHash)
			require.NoError(t, err)
			source.logs[0].Topics = []common.Hash{joined.ID, *epoch.Commitment, common.HexToHash("0x22")}
			source.logs[0].Data = data
			standingData, err := source.abi.Events["StandingChanged"].Inputs.NonIndexed().Pack(
				new(big.Int), *epoch.Commitment, common.Hash{}, uint64(120), uint64(0))
			require.NoError(t, err)
			source.logs[1].Data = standingData
			for i := range source.logs {
				source.logs[i].BlockNumber = 51
			}
			switch missing {
			case "join omitted":
				source.logs = source.logs[1:]
			case "standing omitted":
				source.logs = source.logs[:1]
			case testCompleteInterval:
			}
			root := &model.Tournament{ApplicationID: f.app.ID, EpochIndex: epoch.Index, Address: address,
				MaxLevel: 1, Kind: model.TournamentKindLeaf, StartInstant: 10,
				Snapshot: model.TournamentSnapshot{AsOfBlock: 50, BondRecovery: model.TournamentBondRecovery{
					Disposition: model.BondDispositionTournamentRunning}}}
			before := *root
			f.repo.storedTournaments = []*model.Tournament{root}
			f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).Return(root, nil).Once()
			f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
			f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(1), nil).Once()
			if missing == testCompleteInterval {
				f.emptyParticipants(epoch, address)
				f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID,
					mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool {
						return len(batches) == 1 && batches[0].Tournament.Snapshot.AsOfBlock == 100 &&
							len(batches[0].Commitments) == 1 && len(batches[0].StandingChanges) == 1
					}), uint64(100)).Return(nil).Once()
			}
			err = f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, observerFacts(100))
			if missing == testCompleteInterval {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, txsender.ErrInconclusive)
				require.Empty(t, f.repo.committedTournaments)
			}
			require.Equal(t, before, *root)
			require.Equal(t, [][2]uint64{{51, 51}}, source.queries)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			require.True(t, slices.Contains(source.calls, discoveryCall{testJoinedCounterMethod, 51}))
		})
	}
}
