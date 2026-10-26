// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"math/big"
	"slices"
	"testing"

	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

// Keep expected event names independent of the production filter list.
const (
	testCommitmentJoinedEvent   = "CommitmentJoined"
	testMatchCreatedEvent       = "MatchCreated"
	testMatchAdvancedEvent      = "MatchAdvanced"
	testNewInnerTournamentEvent = "NewInnerTournament"
	testStandingChangedEvent    = "StandingChanged"
	testMissingTopicMutation    = "missing topic"
	testLeafMatchSealedEvent    = "LeafMatchSealed"
)

func TestAllTournamentEventsFilter(t *testing.T) {
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	address := common.HexToAddress("0x1234")
	end := uint64(20)
	query, err := buildAllEventsFilterQuery(&bind.FilterOpts{Start: 10, End: &end}, address, contractABI)
	require.NoError(t, err)
	require.Equal(t, []common.Address{address}, query.Addresses)
	require.Equal(t, uint64(10), query.FromBlock.Uint64())
	require.Equal(t, end, query.ToBlock.Uint64())
	require.Len(t, query.Topics, 1)
	names := []string{testCommitmentJoinedEvent, testMatchAdvancedEvent, testMatchCreatedEvent, "MatchDeleted",
		testNewInnerTournamentEvent, testLeafMatchSealedEvent, "PartialBondRefund", "BondRecovered", testStandingChangedEvent}
	expected := make([]common.Hash, 0, len(names))
	for _, name := range names {
		expected = append(expected, contractABI.Events[name].ID)
		t.Run("missing "+name, func(t *testing.T) {
			incompleteABI := *contractABI
			incompleteABI.Events = maps.Clone(contractABI.Events)
			delete(incompleteABI.Events, name)
			_, err := buildAllEventsFilterQuery(&bind.FilterOpts{}, address, &incompleteABI)
			require.ErrorContains(t, err, "missing monitored event "+name)
		})
	}
	require.Equal(t, expected, query.Topics[0])
}

type tournamentLogsSource struct {
	logs []types.Log
}

func (s *tournamentLogsSource) GetLogs(context.Context, map[string]any) ([]types.Log, error) {
	return s.logs, nil
}

func currentTournamentLogs(t *testing.T) []types.Log {
	t.Helper()
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	address := common.HexToAddress("0x1234")
	one := common.HexToHash("0x11")
	two := common.HexToHash("0x22")
	three := common.HexToHash("0x33")
	pair := crypto.Keccak256Hash(two[:], three[:])
	source := &tournamentLogsSource{}
	large := new(big.Int).Lsh(big.NewInt(1), 200)
	for index, event := range []struct {
		name    string
		indexed []common.Hash
		data    []any
	}{
		{name: testCommitmentJoinedEvent, indexed: []common.Hash{one, two}, data: []any{three}},
		{name: testMatchCreatedEvent, indexed: []common.Hash{pair, two, three}, data: []any{one, uint64(19), uint64(20)}},
		{name: testMatchAdvancedEvent, indexed: []common.Hash{pair},
			data: []any{two, three, two, three, large, uint64(5), uint64(20), uint64(21)}},
		{name: "MatchDeleted", indexed: []common.Hash{pair, two, three}, data: []any{uint8(1), uint8(2)}},
		{name: testNewInnerTournamentEvent, indexed: []common.Hash{pair, two}, data: []any{two, three, one, two,
			itournament.ITournamentTournamentDescriptor{InitialHash: one, BaseCycle: new(big.Int), Height: 38,
				Level: 1, StartInstant: 10, Allowance: 20}, big.NewInt(9)}},
		{name: testLeafMatchSealedEvent, indexed: []common.Hash{pair},
			data: []any{two, three, one, large, one, two, uint64(21), uint64(22), uint64(22)}},
		{name: "PartialBondRefund", indexed: []common.Hash{two, common.HexToHash("0x01")}, data: []any{large}},
		{name: "BondRecovered", indexed: []common.Hash{one, two}, data: []any{large, big.NewInt(7)}},
		{name: testStandingChangedEvent, data: []any{big.NewInt(1), common.Hash{}, common.Hash{}, uint64(0), uint64(0)}},
	} {
		definition := contractABI.Events[event.name]
		data, err := definition.Inputs.NonIndexed().Pack(event.data...)
		require.NoError(t, err)
		source.logs = append(source.logs, types.Log{
			Address: address, BlockNumber: 10, Topics: append([]common.Hash{definition.ID}, event.indexed...), Data: data,
			BlockHash: three, TxHash: two, Index: uint(index),
		})
	}
	return source.logs
}

func TestRetrieveAllTournamentEventsParsesCurrentABI(t *testing.T) {
	address := common.HexToAddress("0x1234")
	one, two, three := common.HexToHash("0x11"), common.HexToHash("0x22"), common.HexToHash("0x33")
	pair := crypto.Keccak256Hash(two[:], three[:])
	large := new(big.Int).Lsh(big.NewInt(1), 200)
	source := &tournamentLogsSource{logs: currentTournamentLogs(t)}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", source))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	adapter, err := NewITournamentAdapter(address, client, ethutil.Filter{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)
	end := uint64(10)
	events, err := adapter.RetrieveAllEvents(&bind.FilterOpts{Context: t.Context(), Start: 10, End: &end})
	require.NoError(t, err)
	require.Len(t, events.CommitmentJoined, 1)
	require.Len(t, events.MatchCreated, 1)
	require.Len(t, events.MatchAdvanced, 1)
	require.Len(t, events.MatchDeleted, 1)
	require.Len(t, events.NewInnerTournament, 1)
	require.Len(t, events.LeafMatchSealed, 1)
	require.Len(t, events.PartialBondRefund, 1)
	require.Len(t, events.BondRecovered, 1)
	require.Equal(t, [32]byte(one), events.CommitmentJoined[0].Commitment)
	require.Equal(t, [32]byte(three), events.CommitmentJoined[0].FinalStateHash)
	require.Equal(t, [32]byte(two), events.MatchCreated[0].One)
	require.Equal(t, [32]byte(three), events.MatchCreated[0].Two)
	require.Equal(t, [32]byte(one), events.MatchCreated[0].LeftOfTwo)
	require.Equal(t, uint64(20), events.MatchCreated[0].EliminableAt)
	require.Equal(t, [32]byte(two), events.MatchAdvanced[0].OtherParent)
	require.Equal(t, [32]byte(three), events.MatchAdvanced[0].LeftNode)
	require.Equal(t, large, events.MatchAdvanced[0].SegmentStartPosition)
	require.Equal(t, uint64(21), events.MatchAdvanced[0].EliminableAt)
	require.Equal(t, uint8(1), events.MatchDeleted[0].Reason)
	require.Equal(t, uint8(2), events.MatchDeleted[0].WinnerCommitment)
	require.Equal(t, common.BytesToAddress(two[:]), events.NewInnerTournament[0].ChildTournament)
	require.Equal(t, uint64(22), events.LeafMatchSealed[0].EliminableAt)
	require.Equal(t, [32]byte(pair), events.LeafMatchSealed[0].MatchIdHash)
	require.Equal(t, common.BytesToAddress(two[:]), events.PartialBondRefund[0].Recipient)
	require.True(t, events.PartialBondRefund[0].Success)
	require.Equal(t, large, events.PartialBondRefund[0].Value)
	require.Equal(t, [32]byte(one), events.BondRecovered[0].Commitment)
	require.Equal(t, common.BytesToAddress(two[:]), events.BondRecovered[0].Claimer)
	require.Equal(t, large, events.BondRecovered[0].Payment)
	require.Equal(t, big.NewInt(7), events.BondRecovered[0].Burned)
	require.Equal(t, source.logs[7], events.BondRecovered[0].Raw)
	require.Equal(t, uint64(19), events.MatchCreated[0].ResponderDeadline)
	require.Equal(t, [32]byte(two), events.MatchAdvanced[0].One)
	require.Equal(t, [32]byte(three), events.MatchAdvanced[0].Two)
	require.Equal(t, uint64(5), events.MatchAdvanced[0].CurrentHeight)
	require.Equal(t, uint64(20), events.MatchAdvanced[0].ResponderDeadline)
	require.Equal(t, [32]byte(two), events.LeafMatchSealed[0].One)
	require.Equal(t, [32]byte(three), events.LeafMatchSealed[0].Two)
	require.Equal(t, [32]byte(one), events.LeafMatchSealed[0].AgreeState)
	require.Equal(t, large, events.LeafMatchSealed[0].DivergencePosition)
	require.Equal(t, [32]byte(one), events.LeafMatchSealed[0].FinalStateOne)
	require.Equal(t, [32]byte(two), events.LeafMatchSealed[0].FinalStateTwo)
	require.Equal(t, uint64(21), events.LeafMatchSealed[0].DeadlineOne)
	require.Equal(t, uint64(22), events.LeafMatchSealed[0].DeadlineTwo)
	require.Equal(t, [32]byte(two), events.NewInnerTournament[0].One)
	require.Equal(t, [32]byte(three), events.NewInnerTournament[0].Two)
	require.Equal(t, [32]byte(one), events.NewInnerTournament[0].ContestedFinalStateOne)
	require.Equal(t, [32]byte(two), events.NewInnerTournament[0].ContestedFinalStateTwo)
	descriptor := events.NewInnerTournament[0].Descriptor
	require.NotNil(t, descriptor.BaseCycle)
	require.Zero(t, descriptor.BaseCycle.Sign())
	descriptor.BaseCycle = nil
	require.Equal(t, itournament.ITournamentTournamentDescriptor{InitialHash: one,
		Height: 38, Level: 1, StartInstant: 10, Allowance: 20}, descriptor)
	require.Equal(t, big.NewInt(9), events.NewInnerTournament[0].BondValue)
	require.Len(t, events.StandingChanged, 1)
	require.Equal(t, big.NewInt(1), events.StandingChanged[0].MatchCount)
	require.Equal(t, source.logs[8], events.StandingChanged[0].Raw)
}

func TestRetrieveTournamentEventsRejectsInvalidLogs(t *testing.T) {
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	address := common.HexToAddress("0x1234")
	definition := contractABI.Events["MatchDeleted"]
	data, err := definition.Inputs.NonIndexed().Pack(uint8(1), uint8(2))
	require.NoError(t, err)
	valid := types.Log{
		Address: address, BlockNumber: 10, BlockHash: common.HexToHash("0xb0"), TxHash: common.HexToHash("0xc0"),
		Topics: []common.Hash{definition.ID, common.HexToHash("0x1"), common.HexToHash("0x2"), common.HexToHash("0x3")},
		Data:   data,
	}
	for _, test := range []struct {
		name      string
		mutate    func(*types.Log)
		duplicate bool
	}{
		{name: "wrong address", mutate: func(log *types.Log) { log.Address = common.Address{} }},
		{name: "removed", mutate: func(log *types.Log) { log.Removed = true }},
		{name: "before range", mutate: func(log *types.Log) { log.BlockNumber = 9 }},
		{name: "after range", mutate: func(log *types.Log) { log.BlockNumber = 11 }},
		{name: "missing transaction", mutate: func(log *types.Log) { log.TxHash = common.Hash{} }},
		{name: "missing block hash", mutate: func(log *types.Log) { log.BlockHash = common.Hash{} }},
		{name: testMissingTopicMutation, mutate: func(log *types.Log) { log.Topics = nil }},
		{name: "unexpected topic", mutate: func(log *types.Log) { log.Topics[0] = common.Hash{} }},
		{name: "bad payload", mutate: func(log *types.Log) { log.Data = nil }},
		{name: "unknown reason", mutate: func(log *types.Log) { log.Data[31] = 3 }},
		{name: "unknown winner", mutate: func(log *types.Log) { log.Data[63] = 3 }},
		{name: "node sentinel is not a contract reason", mutate: func(log *types.Log) { log.Data[31] = 255 }},
		{name: "maximum unknown winner", mutate: func(log *types.Log) { log.Data[63] = 255 }},
		{name: "duplicate identity", duplicate: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := valid
			entry.Data = slices.Clone(valid.Data)
			entry.Topics = slices.Clone(valid.Topics)
			if test.mutate != nil {
				test.mutate(&entry)
			}
			source := &tournamentLogsSource{logs: []types.Log{entry}}
			if test.duplicate {
				source.logs = append(source.logs, entry)
			}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", source))
			t.Cleanup(server.Stop)
			client := ethclient.NewClient(rpc.DialInProc(server))
			t.Cleanup(client.Close)
			adapter, err := NewITournamentAdapter(address, client, ethutil.Filter{
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			require.NoError(t, err)
			end := uint64(10)
			events, err := adapter.RetrieveAllEvents(&bind.FilterOpts{Context: t.Context(), Start: 10, End: &end})
			require.Error(t, err)
			require.Nil(t, events, "a rejected range must not return partial events")
		})
	}
}

func TestRetrieveTournamentEventsOrdersLogsAndChecksCrossLogIdentity(t *testing.T) {
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	address := common.HexToAddress("0x1234")
	definition := contractABI.Events["PartialBondRefund"]
	value := big.NewInt(17)
	data, err := definition.Inputs.NonIndexed().Pack(value)
	require.NoError(t, err)
	first := types.Log{Address: address, BlockNumber: 10, BlockHash: common.HexToHash("0xb0"), TxHash: common.HexToHash("0xc0"),
		Topics: []common.Hash{definition.ID, common.HexToHash("0x22"), {}}, Data: data}
	second := first
	second.Index = 1
	for _, test := range []struct {
		name      string
		mutate    func(*types.Log)
		wantError string
	}{
		{name: "same block order"},
		{name: "different block hashes", mutate: func(log *types.Log) { log.BlockHash = common.HexToHash("0xb1") },
			wantError: "disagree on the block hash"},
		{name: "same transaction identity in different blocks", mutate: func(log *types.Log) {
			log.BlockNumber = 11
			log.Index = 0
		}, wantError: "transaction log identity"},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := second
			if test.mutate != nil {
				test.mutate(&entry)
			}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", &tournamentLogsSource{logs: []types.Log{entry, first}}))
			t.Cleanup(server.Stop)
			client := ethclient.NewClient(rpc.DialInProc(server))
			t.Cleanup(client.Close)
			adapter, err := NewITournamentAdapter(address, client, ethutil.Filter{
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			require.NoError(t, err)
			end := uint64(11)
			events, err := adapter.RetrieveAllEvents(&bind.FilterOpts{Context: t.Context(), Start: 10, End: &end})
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Nil(t, events)
				return
			}
			require.NoError(t, err)
			require.Len(t, events.PartialBondRefund, 2)
			require.Equal(t, first, events.PartialBondRefund[0].Raw)
			require.Equal(t, second, events.PartialBondRefund[1].Raw)
			for _, refund := range events.PartialBondRefund {
				require.False(t, refund.Success)
				require.Equal(t, value, refund.Value, "failed refund retains its requested amount")
			}
		})
	}
}
