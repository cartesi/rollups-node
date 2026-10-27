// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type tournamentScheduleRPC struct{ mock.Mock }

func (m *tournamentScheduleRPC) Call(
	_ context.Context, call map[string]json.RawMessage, block rpc.BlockNumber,
) (hexutil.Bytes, error) {
	var address common.Address
	var input hexutil.Bytes
	if err := json.Unmarshal(call["to"], &address); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(call["input"], &input); err != nil {
		return nil, err
	}
	args := m.Called(address, []byte(input), block)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *tournamentScheduleRPC) GetLogs(
	_ context.Context, query map[string]json.RawMessage,
) ([]types.Log, error) {
	args := m.Called(query)
	return args.Get(0).([]types.Log), args.Error(1)
}

func collectScheduleEvent(
	t *testing.T, eventName string, indexed []common.Hash, values ...any,
) *tournamentEvents {
	t.Helper()
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	event := contractABI.Events[eventName]
	data, err := event.Inputs.NonIndexed().Pack(values...)
	require.NoError(t, err)
	address := common.HexToAddress("0x100")
	block := rpc.BlockNumber(1234)
	log := types.Log{
		Address: address, BlockNumber: uint64(block), TxHash: common.HexToHash("0x200"),
		Topics: append([]common.Hash{event.ID}, indexed...), Data: data,
	}
	backend := &tournamentScheduleRPC{}
	for _, name := range []string{"CommitmentJoined", "MatchCreated", "MatchAdvanced", "MatchDeleted"} {
		method := "get" + name + "Count"
		input, packErr := contractABI.Pack(method)
		require.NoError(t, packErr)
		count := big.NewInt(0)
		if name == eventName {
			count.SetUint64(1)
		}
		output, packErr := contractABI.Methods[method].Outputs.Pack(count)
		require.NoError(t, packErr)
		backend.On("Call", address, input, block).Return(output, nil).Once()
	}
	queryJSON, err := json.Marshal(map[string]any{
		"address": []common.Address{address}, "topics": [][]common.Hash{{event.ID}},
		"fromBlock": hexutil.EncodeUint64(uint64(block)), "toBlock": hexutil.EncodeUint64(uint64(block)),
	})
	require.NoError(t, err)
	var query map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(queryJSON, &query))
	backend.On("GetLogs", query).Return([]types.Log{log}, nil).Once()
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	t.Cleanup(func() { backend.AssertExpectations(t) })
	c := &chainClient{
		eth: client, callOpts: &bind.CallOpts{Context: t.Context()}, blockNum: uint64(block),
		filter: ethutil.Filter{Logger: slog.Default()},
	}
	events, err := c.fetchTournamentEvents(address, uint64(block))
	require.NoError(t, err)
	return events
}

func TestCollectCreatedMatchSchedule(t *testing.T) {
	matchID := common.HexToHash("0xaa")
	one := common.HexToHash("0xbb")
	two := common.HexToHash("0xcc")
	leftOfTwo := [32]byte{0xdd}
	// Distinct values above 2^53 also check exact decimal JSON numbers.
	const responderDeadline, eliminableAt = uint64(9007199254740993), uint64(9007199254740999)
	events := collectScheduleEvent(t, "MatchCreated", []common.Hash{matchID, one, two},
		leftOfTwo, responderDeadline, eliminableAt)
	require.Len(t, events.matchesCreated, 1)
	result := formatMatchEvents(events.matchesCreated, events.matchesDeleted, nil)
	require.Len(t, result, 1)
	encoded, err := json.Marshal(result[0])
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.JSONEq(t, `"`+matchID.Hex()+`"`, string(fields["match_id_hash"]))
	require.Equal(t, "9007199254740993", string(fields["responder_deadline"]))
	require.Equal(t, "9007199254740999", string(fields["eliminable_at"]))
}

func TestCommitmentRegistryResolve(t *testing.T) {
	addr := common.HexToAddress("0xABCDEF0000000000000000000000000000000001")
	commitment := [32]byte{0x01, 0x02, 0x03}

	registry := make(commitmentRegistry)
	registry[commitment] = addr

	// Known key returns address.
	got := registry.resolve(commitment)
	assert.Equal(t, addr.Hex(), got)

	// Unknown key returns empty string.
	unknown := [32]byte{0xFF}
	assert.Equal(t, "", registry.resolve(unknown))

	// Empty registry returns empty string.
	empty := make(commitmentRegistry)
	assert.Equal(t, "", empty.resolve(commitment))
}

func TestFormatCommitmentEvents(t *testing.T) {
	// Empty input returns nil.
	assert.Nil(t, formatCommitmentEvents(nil))
	assert.Nil(t, formatCommitmentEvents([]rawCommitmentJoined{}))

	raw := []rawCommitmentJoined{
		{
			commitment:     [32]byte{0xAA},
			finalStateHash: [32]byte{0xBB},
			submitter:      common.HexToAddress("0x1111111111111111111111111111111111111111"),
			blockNumber:    100,
			txHash:         common.HexToHash("0xCC"),
		},
		{
			commitment:     [32]byte{0xDD},
			finalStateHash: [32]byte{0xEE},
			submitter:      common.HexToAddress("0x2222222222222222222222222222222222222222"),
			blockNumber:    200,
			txHash:         common.HexToHash("0xFF"),
		},
	}

	result := formatCommitmentEvents(raw)
	assert.Len(t, result, 2)
	assert.Equal(t, formatHash(raw[0].commitment), result[0].Commitment)
	assert.Equal(t, formatHash(raw[0].finalStateHash), result[0].FinalStateHash)
	assert.Equal(t, "0x1111111111111111111111111111111111111111", result[0].Submitter)
	assert.Equal(t, uint64(100), result[0].BlockNumber)
	assert.Equal(t, raw[0].txHash.Hex(), result[0].TxHash)

	assert.Equal(t, formatHash(raw[1].commitment), result[1].Commitment)
	assert.Equal(t, uint64(200), result[1].BlockNumber)
}

func TestFormatMatchEvents(t *testing.T) {
	// Empty input returns nil.
	assert.Nil(t, formatMatchEvents(nil, nil, nil))
	assert.Nil(t, formatMatchEvents([]rawMatchCreated{}, nil, nil))

	commitOne := [32]byte{0x01}
	commitTwo := [32]byte{0x02}
	matchID := [32]byte{0xAA}
	matchID2 := [32]byte{0xBB}

	registry := make(commitmentRegistry)
	registry[commitOne] = common.HexToAddress("0x1111111111111111111111111111111111111111")
	registry[commitTwo] = common.HexToAddress("0x2222222222222222222222222222222222222222")

	created := []rawMatchCreated{
		{
			matchIDHash:  matchID,
			one:          commitOne,
			two:          commitTwo,
			leftOfTwo:    [32]byte{0x33},
			eliminableAt: 120,
			blockNumber:  100,
			txHash:       common.HexToHash("0xC1"),
		},
		{
			matchIDHash: matchID2,
			one:         commitOne,
			two:         [32]byte{0x99}, // unknown in registry
			leftOfTwo:   [32]byte{0x44},
			blockNumber: 200,
			txHash:      common.HexToHash("0xC2"),
		},
	}

	deleted := []rawMatchDeleted{
		{
			matchIDHash:      matchID,
			one:              commitOne,
			two:              commitTwo,
			reason:           1, // TIMEOUT
			winnerCommitment: 1, // ONE
			blockNumber:      150,
			txHash:           common.HexToHash("0xD1"),
		},
	}

	result := formatMatchEvents(created, deleted, registry)
	assert.Len(t, result, 2)

	// First match: has deletion info.
	m0 := result[0]
	assert.Equal(t, formatHash(matchID), m0.MatchIDHash)
	assert.Equal(t, "0x1111111111111111111111111111111111111111", m0.PlayerOneAddr)
	assert.Equal(t, "0x2222222222222222222222222222222222222222", m0.PlayerTwoAddr)
	assert.Equal(t, uint64(120), m0.EliminableAt)
	assert.Equal(t, "TIMEOUT", m0.DeletionReason)
	assert.Equal(t, "ONE", m0.Winner)
	assert.NotNil(t, m0.DeletionBlock)
	assert.Equal(t, uint64(150), *m0.DeletionBlock)
	// Winner is player ONE -> resolve commitOne.
	assert.Equal(t, "0x1111111111111111111111111111111111111111", m0.WinnerAddr)

	// Second match: no deletion.
	m1 := result[1]
	assert.Equal(t, formatHash(matchID2), m1.MatchIDHash)
	assert.Equal(t, "0x1111111111111111111111111111111111111111", m1.PlayerOneAddr)
	assert.Equal(t, "", m1.PlayerTwoAddr) // unknown commitment
	assert.Equal(t, "", m1.DeletionReason)
	assert.Nil(t, m1.DeletionBlock)
}

func TestFormatMatchEventsWinnerTwo(t *testing.T) {
	commitOne := [32]byte{0x01}
	commitTwo := [32]byte{0x02}
	matchID := [32]byte{0xAA}

	registry := make(commitmentRegistry)
	registry[commitOne] = common.HexToAddress("0x1111111111111111111111111111111111111111")
	registry[commitTwo] = common.HexToAddress("0x2222222222222222222222222222222222222222")

	created := []rawMatchCreated{
		{matchIDHash: matchID, one: commitOne, two: commitTwo, blockNumber: 100},
	}
	deleted := []rawMatchDeleted{
		{matchIDHash: matchID, one: commitOne, two: commitTwo,
			reason: 2, winnerCommitment: 2, blockNumber: 200},
	}

	result := formatMatchEvents(created, deleted, registry)
	assert.Len(t, result, 1)
	assert.Equal(t, "TWO", result[0].Winner)
	assert.Equal(t, "0x2222222222222222222222222222222222222222", result[0].WinnerAddr)
	assert.Equal(t, "CHILD_TOURNAMENT", result[0].DeletionReason)
}

func TestFormatMatchEventsNoWinner(t *testing.T) {
	matchID := [32]byte{0xAA}
	commitOne := [32]byte{0x01}
	commitTwo := [32]byte{0x02}

	created := []rawMatchCreated{
		{matchIDHash: matchID, one: commitOne, two: commitTwo, blockNumber: 100},
	}
	deleted := []rawMatchDeleted{
		{matchIDHash: matchID, one: commitOne, two: commitTwo,
			reason: 0, winnerCommitment: 0, blockNumber: 200},
	}

	result := formatMatchEvents(created, deleted, nil)
	assert.Len(t, result, 1)
	assert.Equal(t, "NONE (both eliminated)", result[0].Winner)
	assert.Equal(t, "STEP (on-chain proof)", result[0].DeletionReason)
	assert.Equal(t, "", result[0].WinnerAddr) // winner=0 -> no resolution
}

func TestFormatAdvanceEvents(t *testing.T) {
	// Empty input returns nil.
	assert.Nil(t, formatAdvanceEvents(nil))
	assert.Nil(t, formatAdvanceEvents([]rawMatchAdvanced{}))

	raw := []rawMatchAdvanced{
		{
			matchIDHash:          [32]byte{0xAA},
			otherParent:          [32]byte{0xBB},
			leftNode:             [32]byte{0xCC},
			segmentStartPosition: big.NewInt(42),
			eliminableAt:         120,
			blockNumber:          100,
			txHash:               common.HexToHash("0xDD"),
		},
		{
			matchIDHash: [32]byte{0xEE},
			otherParent: [32]byte{0xFF},
			leftNode:    [32]byte{0x11},
			blockNumber: 200,
			txHash:      common.HexToHash("0x22"),
		},
	}

	result := formatAdvanceEvents(raw)
	assert.Len(t, result, 2)
	assert.Equal(t, formatHash(raw[0].matchIDHash), result[0].MatchIDHash)
	assert.Equal(t, formatHash(raw[0].otherParent), result[0].OtherParent)
	assert.Equal(t, formatHash(raw[0].leftNode), result[0].LeftNode)
	assert.Equal(t, "42", result[0].SegmentStartPosition)
	assert.Equal(t, uint64(120), result[0].EliminableAt)
	assert.Equal(t, uint64(100), result[0].BlockNumber)
	assert.Equal(t, raw[0].txHash.Hex(), result[0].TxHash)

	assert.Equal(t, uint64(200), result[1].BlockNumber)
}

func TestTournamentEnumNamesMatchSolidityOrder(t *testing.T) {
	assert.Equal(t, "MATCHES_ACTIVE", tournamentStandingName(0))
	assert.Equal(t, "AWAITING_CLOSURE", tournamentStandingName(1))
	assert.Equal(t, "ROOT_WINNER", tournamentStandingName(2))
	assert.Equal(t, "ROOT_FAILED", tournamentStandingName(3))
	assert.Equal(t, "INNER_WINNER", tournamentStandingName(4))
	assert.Equal(t, "INNER_ELIMINABLE_NO_WINNER", tournamentStandingName(5))
	assert.Equal(t, "INNER_ELIMINABLE_WINNER_EXPIRED", tournamentStandingName(6))
	assert.Equal(t, "UNKNOWN(7)", tournamentStandingName(7))

	assert.Equal(t, "LEAF", tournamentKindName(0))
	assert.Equal(t, "NON_LEAF", tournamentKindName(1))
	assert.Equal(t, "UNKNOWN(2)", tournamentKindName(2))
	assert.Equal(t, "root", tournamentLevelName(0, tournamentKindNonLeaf))
	assert.Equal(t, "inner", tournamentLevelName(1, tournamentKindNonLeaf))
	assert.Equal(t, "leaf", tournamentLevelName(2, tournamentKindLeaf))
}

func TestMaxLevelFromCount(t *testing.T) {
	maxLevel, err := maxLevelFromCount(1)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), maxLevel)

	maxLevel, err = maxLevelFromCount(3)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), maxLevel)

	_, err = maxLevelFromCount(0)
	assert.EqualError(t, err, "tournament level count is zero")
}

func TestPopulateTournamentStandingResult(t *testing.T) {
	candidate := [32]byte{0x11}
	directFinalState := [32]byte{0x22}
	recoveredFinalState := [32]byte{0x33}

	tests := []struct {
		name                    string
		level                   uint64
		standing                itournament.ITournamentTournamentStandingView
		wantClosed              bool
		wantFinished            bool
		wantFinishedAt          *uint64
		wantHasWinner           *bool
		wantWinner              string
		wantFinalState          string
		wantWinnerExpiresAt     *uint64
		wantCanBeEliminated     *bool
		wantFinalStateReadCount int
	}{
		{
			name: "active root",
			standing: itournament.ITournamentTournamentStandingView{
				Standing: tournamentStandingMatchesActive, AcceptsJoins: true,
			},
		},
		{
			name: "root winner",
			standing: itournament.ITournamentTournamentStandingView{
				Standing: tournamentStandingRootWinner, HasCandidate: true,
				Candidate: candidate, FinalState: directFinalState, FinishedAt: 42,
			},
			wantClosed:     true,
			wantFinished:   true,
			wantFinishedAt: uint64Ptr(42),
			wantHasWinner:  boolPtr(true),
			wantWinner:     formatHash(candidate),
			wantFinalState: formatHash(directFinalState),
		},
		{
			name: "failed root",
			standing: itournament.ITournamentTournamentStandingView{
				Standing: tournamentStandingRootFailed, FinishedAt: 43,
			},
			wantClosed:     true,
			wantFinished:   true,
			wantFinishedAt: uint64Ptr(43),
			wantHasWinner:  boolPtr(false),
		},
		{
			name:  "inner winner",
			level: 1,
			standing: itournament.ITournamentTournamentStandingView{
				Standing: tournamentStandingInnerWinner, HasCandidate: true,
				Candidate: candidate, FinalState: directFinalState, FinishedAt: 44, WinnerExpiresAt: 60,
			},
			wantClosed:          true,
			wantFinished:        true,
			wantFinishedAt:      uint64Ptr(44),
			wantHasWinner:       boolPtr(true),
			wantWinner:          formatHash(candidate),
			wantFinalState:      formatHash(directFinalState),
			wantWinnerExpiresAt: uint64Ptr(60),
			wantCanBeEliminated: boolPtr(false),
		},
		{
			name:  "expired inner winner",
			level: 1,
			standing: itournament.ITournamentTournamentStandingView{
				Standing: tournamentStandingInnerEliminableWinnerExpired, HasCandidate: true,
				Candidate: candidate, FinishedAt: 45,
			},
			wantClosed:              true,
			wantFinished:            true,
			wantFinishedAt:          uint64Ptr(45),
			wantHasWinner:           boolPtr(true),
			wantWinner:              formatHash(candidate),
			wantFinalState:          formatHash(recoveredFinalState),
			wantCanBeEliminated:     boolPtr(true),
			wantFinalStateReadCount: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := &TournamentResult{Level: test.level}
			readCount := 0
			err := populateTournamentStandingResult(result, test.standing, func(got [32]byte) ([32]byte, error) {
				readCount++
				assert.Equal(t, candidate, got)
				return recoveredFinalState, nil
			})

			assert.NoError(t, err)
			assert.Equal(t, test.wantClosed, result.Closed)
			assert.Equal(t, test.wantFinished, result.Finished)
			assert.Equal(t, test.wantFinishedAt, result.FinishedAtBlock)
			assert.Equal(t, test.wantHasWinner, result.HasWinner)
			assert.Equal(t, test.wantWinner, result.WinnerCommitment)
			assert.Equal(t, test.wantFinalState, result.FinalMachineHash)
			assert.Equal(t, test.wantWinnerExpiresAt, result.WinnerExpiresAt)
			assert.Equal(t, test.wantCanBeEliminated, result.CanBeEliminated)
			assert.Equal(t, test.wantFinalStateReadCount, readCount)
		})
	}
}

func TestPopulateTournamentStandingResultReturnsFinalStateError(t *testing.T) {
	wantErr := errors.New("read failed")
	result := &TournamentResult{Level: 1}
	err := populateTournamentStandingResult(result, itournament.ITournamentTournamentStandingView{
		Standing:     tournamentStandingInnerEliminableWinnerExpired,
		HasCandidate: true,
		Candidate:    [32]byte{1},
		FinishedAt:   10,
	}, func([32]byte) ([32]byte, error) {
		return [32]byte{}, wantErr
	})
	assert.ErrorIs(t, err, wantErr)
}

func uint64Ptr(value uint64) *uint64 { return &value }

func boolPtr(value bool) *bool { return &value }
