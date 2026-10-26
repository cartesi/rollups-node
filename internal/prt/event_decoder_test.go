// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func TestStaticABIWordCount(t *testing.T) {
	components := []abi.ArgumentMarshaling{{Name: "root", Type: "bytes32"}, {Name: "values", Type: "uint64[2]"}}
	for _, test := range []struct {
		kind       string
		components []abi.ArgumentMarshaling
		words      int
	}{
		{kind: "uint256", words: 1}, {kind: "bytes32[3]", words: 3},
		{kind: "tuple", components: components, words: 3}, {kind: "tuple[2]", components: components, words: 6},
		{kind: "string"}, {kind: "bytes"}, {kind: "uint64[]"},
		{kind: "tuple", components: []abi.ArgumentMarshaling{{Name: "payload", Type: "bytes"}}},
	} {
		t.Run(test.kind, func(t *testing.T) {
			value, err := abi.NewType(test.kind, "", test.components)
			require.NoError(t, err)
			words, err := staticABIWordCount(abi.Arguments{{Type: value}})
			if test.words == 0 {
				require.ErrorContains(t, err, "not static")
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.words, words)
		})
	}
}

func TestTournamentEventDecoderRejectsIncorrectABIWidths(t *testing.T) {
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	binding, err := itournament.NewITournament(common.Address{}, nil)
	require.NoError(t, err)
	adapter := &ITournamentAdapterImpl{tournament: binding}
	for _, entry := range currentTournamentLogs(t) {
		definition, err := contractABI.EventByID(entry.Topics[0])
		require.NoError(t, err)
		for _, mutation := range []string{"truncated", "trailing word", testMissingTopicMutation, "extra topic"} {
			t.Run(definition.Name+"/"+mutation, func(t *testing.T) {
				invalid := entry
				invalid.Data, invalid.Topics = slices.Clone(entry.Data), slices.Clone(entry.Topics)
				switch mutation {
				case "truncated":
					invalid.Data = invalid.Data[:len(invalid.Data)-1]
				case "trailing word":
					invalid.Data = append(invalid.Data, make([]byte, 32)...)
				case testMissingTopicMutation:
					invalid.Topics = invalid.Topics[:len(invalid.Topics)-1]
				case "extra topic":
					invalid.Topics = append(invalid.Topics, common.Hash{})
				}
				require.Error(t, adapter.appendTournamentEvent(contractABI, invalid, &TournamentEvents{}))
			})
		}
	}
}

func TestTournamentEventDecoderChecksStatelessFields(t *testing.T) {
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	binding, err := itournament.NewITournament(common.Address{}, nil)
	require.NoError(t, err)
	adapter := &ITournamentAdapterImpl{tournament: binding}
	fixtures := make(map[string]types.Log)
	for _, entry := range currentTournamentLogs(t) {
		definition, err := contractABI.EventByID(entry.Topics[0])
		require.NoError(t, err)
		fixtures[definition.Name] = entry
	}
	for _, test := range []struct {
		name, event string
		word        int
		value       uint64
		topic       bool
		valid       bool
	}{
		{name: "created pair", event: testMatchCreatedEvent, topic: true},
		{name: "advance pair", event: testMatchAdvancedEvent},
		{name: "leaf pair", event: testLeafMatchSealedEvent},
		{name: "leaf first maximum", event: testLeafMatchSealedEvent, word: 6, value: 22, valid: true},
		{name: "child pair", event: testNewInnerTournamentEvent},
		{name: "child kind", event: testNewInnerTournamentEvent, word: 9, value: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := fixtures[test.event]
			entry.Data, entry.Topics = slices.Clone(entry.Data), slices.Clone(entry.Topics)
			if test.topic {
				entry.Topics[1] = common.Hash{}
			} else {
				offset := test.word * 32
				clear(entry.Data[offset : offset+32])
				binary.BigEndian.PutUint64(entry.Data[offset+24:offset+32], test.value)
			}
			err := adapter.appendTournamentEvent(contractABI, entry, &TournamentEvents{})
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	t.Run("first deadline maximum", func(t *testing.T) {
		entry := fixtures[testLeafMatchSealedEvent]
		entry.Data = slices.Clone(entry.Data)
		binary.BigEndian.PutUint64(entry.Data[6*32+24:7*32], 23)
		binary.BigEndian.PutUint64(entry.Data[8*32+24:9*32], 23)
		require.NoError(t, adapter.appendTournamentEvent(contractABI, entry, &TournamentEvents{}))
	})
	for _, test := range []struct {
		name             string
		dangling, parent common.Hash
		result, expiry   uint64
		valid            bool
	}{
		{name: "root result", result: 10, valid: true},
		{name: "inner result", dangling: common.Hash{1}, parent: common.Hash{2}, result: 10, expiry: 12, valid: true},
		{name: "zero allowance", dangling: common.Hash{1}, parent: common.Hash{2}, result: 10, expiry: 10, valid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := fixtures[testStandingChangedEvent]
			entry.Data, err = contractABI.Events[testStandingChangedEvent].Inputs.NonIndexed().Pack(
				new(big.Int), test.dangling, test.parent, test.result, test.expiry)
			require.NoError(t, err)
			err := adapter.appendTournamentEvent(contractABI, entry, &TournamentEvents{})
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

type tournamentRangeQuery struct {
	FromBlock *hexutil.Big    `json:"fromBlock"`
	ToBlock   *hexutil.Big    `json:"toBlock"`
	Topics    [][]common.Hash `json:"topics"`
}

type segmentedTournamentLogs struct {
	mu      sync.Mutex
	logs    []types.Log
	queries [][2]uint64
	failAt  *uint64
}

func (s *segmentedTournamentLogs) GetLogs(_ context.Context, query tournamentRangeQuery) ([]types.Log, error) {
	from, to := query.FromBlock.ToInt().Uint64(), query.ToBlock.ToInt().Uint64()
	s.mu.Lock()
	s.queries = append(s.queries, [2]uint64{from, to})
	s.mu.Unlock()
	if s.failAt != nil && from <= *s.failAt && *s.failAt <= to {
		return nil, errors.New("interval unavailable")
	}
	logs := []types.Log{}
	for _, entry := range s.logs {
		if from <= entry.BlockNumber && entry.BlockNumber <= to && slices.Contains(query.Topics[0], entry.Topics[0]) {
			logs = append(logs, entry)
		}
	}
	return logs, nil
}

func TestRetrieveTournamentEventsRetainsEverySliceAcrossChunks(t *testing.T) {
	var reference *TournamentEvents
	for _, maximum := range []*big.Int{nil, big.NewInt(1)} {
		source := &segmentedTournamentLogs{}
		for _, block := range []uint64{12, 10} {
			for _, entry := range currentTournamentLogs(t) {
				entry.BlockNumber = block
				entry.BlockHash = common.BigToHash(new(big.Int).SetUint64(block))
				entry.TxHash = common.BigToHash(new(big.Int).SetUint64(block + 100))
				source.logs = append(source.logs, entry)
			}
		}
		server := rpc.NewServer()
		require.NoError(t, server.RegisterName("eth", source))
		t.Cleanup(server.Stop)
		httpServer := httptest.NewServer(server)
		t.Cleanup(httpServer.Close)
		client, err := ethutil.DialEthClient(t.Context(), httpServer.URL)
		require.NoError(t, err)
		t.Cleanup(client.Close)
		adapter, err := NewITournamentAdapter(common.HexToAddress("0x1234"), client,
			ethutil.Filter{MaxChunkSize: maximum, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		require.NoError(t, err)
		end := uint64(12)
		events, err := adapter.RetrieveAllEvents(&bind.FilterOpts{Context: t.Context(), Start: 10, End: &end})
		require.NoError(t, err)
		fields := reflect.ValueOf(events).Elem()
		for index := range fields.NumField() {
			field := fields.Field(index)
			require.Equal(t, reflect.Slice, field.Kind())
			require.Equal(t, 2, field.Len(), fields.Type().Field(index).Name)
			for position, block := range []uint64{10, 12} {
				raw := field.Index(position).Elem().FieldByName("Raw").Interface().(types.Log)
				require.Equal(t, block, raw.BlockNumber)
			}
		}
		source.mu.Lock()
		queries := slices.Clone(source.queries)
		source.mu.Unlock()
		if maximum == nil {
			reference = events
			require.Equal(t, [][2]uint64{{10, 12}}, queries)
		} else {
			require.Equal(t, reference, events)
			require.Equal(t, [][2]uint64{{10, 10}, {11, 11}, {12, 12}}, queries)
		}
	}
}

func TestRetrieveTournamentEventsRejectsTerminalIteratorError(t *testing.T) {
	source := &segmentedTournamentLogs{logs: currentTournamentLogs(t), failAt: new(uint64(11))}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", source))
	t.Cleanup(server.Stop)
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	client, err := ethutil.DialEthClient(t.Context(), httpServer.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	adapter, err := NewITournamentAdapter(common.HexToAddress("0x1234"), client,
		ethutil.Filter{MaxChunkSize: big.NewInt(1), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	require.NoError(t, err)
	end := uint64(12)
	events, err := adapter.RetrieveAllEvents(&bind.FilterOpts{Context: t.Context(), Start: 10, End: &end})
	require.ErrorContains(t, err, "interval unavailable")
	require.Nil(t, events)
}

func TestTournamentEventCountsRequiresEveryStandingChange(t *testing.T) {
	events := &TournamentEvents{CommitmentJoined: []*itournament.ITournamentCommitmentJoined{{}, {}},
		MatchDeleted: []*itournament.ITournamentMatchDeleted{{}}}
	before := zeroStructuralEventCounts()
	before.CommitmentJoined.SetUint64(9)
	before.MatchDeleted.SetUint64(17)
	after := zeroStructuralEventCounts()
	after.CommitmentJoined.SetUint64(11)
	after.MatchDeleted.SetUint64(18)
	for count := 2; count <= 4; count++ {
		events.StandingChanged = make([]*itournament.ITournamentStandingChanged, count)
		err := validateTournamentEventCounts(before, after, events)
		if count == 3 {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, txsender.ErrInconclusive)
			require.ErrorContains(t, err, "StandingChanged event count mismatch")
		}
	}
}
