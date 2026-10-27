// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"encoding/json"
	"math/big"
	"strconv"
	"testing"

	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectAdvancedMatchSchedule(t *testing.T) {
	for _, test := range []struct {
		name              string
		height            uint64
		responderDeadline uint64
		eliminableAt      uint64
	}{
		{name: "bisection response", height: 6, responderDeadline: 210, eliminableAt: 310},
		{name: "seal response at height one", height: 1, responderDeadline: 420, eliminableAt: 520},
	} {
		t.Run(test.name, func(t *testing.T) {
			matchID := common.HexToHash("0xaa")
			events := collectScheduleEvent(t, "MatchAdvanced", []common.Hash{matchID},
				[32]byte{0x01}, [32]byte{0x02}, [32]byte{0x03}, [32]byte{0x04},
				big.NewInt(0), test.height, test.responderDeadline, test.eliminableAt)
			require.Len(t, events.matchesAdvanced, 1)
			result := formatAdvanceEvents(events.matchesAdvanced)
			require.Len(t, result, 1)
			encoded, err := json.Marshal(result[0])
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &fields))
			require.JSONEq(t, `"`+matchID.Hex()+`"`, string(fields["match_id_hash"]))
			require.JSONEq(t, `"0"`, string(fields["segment_start_position"]))
			require.Equal(t, strconv.FormatUint(test.height, 10), string(fields["current_height"]))
			require.Equal(t, strconv.FormatUint(test.responderDeadline, 10), string(fields["responder_deadline"]))
			require.Equal(t, strconv.FormatUint(test.eliminableAt, 10), string(fields["eliminable_at"]))
		})
	}
}

func TestPopulateBisectingMatchResult(t *testing.T) {
	result := &MatchResult{}
	value := itournament.ITournamentBisectingMatchView{
		RevealingParent:      [32]byte{0x01},
		WaitingLeft:          [32]byte{0x02},
		WaitingRight:         [32]byte{0x03},
		SegmentStartPosition: big.NewInt(4),
		SegmentStartCycle:    big.NewInt(5),
		CurrentHeight:        6,
		Responder:            1,
	}

	require.NoError(t, populateBisectingMatchResult(result, value))
	require.NotNil(t, result.CurrentHeight)
	assert.Equal(t, uint64(6), *result.CurrentHeight)
	assert.Equal(t, "4", result.SegmentStartPosition)
	assert.Equal(t, "5", result.SegmentStartCycle)
	assert.Equal(t, formatHash(value.RevealingParent), result.RevealingParent)
	assert.Equal(t, formatHash(value.WaitingLeft), result.WaitingLeft)
	assert.Equal(t, formatHash(value.WaitingRight), result.WaitingRight)
	assert.Equal(t, "TWO", result.Responder)
}

func TestPopulateReadyToSealMatchResult(t *testing.T) {
	result := &MatchResult{}
	value := itournament.ITournamentReadyToSealMatchView{
		RevealingParent:      [32]byte{0x01},
		WaitingLeft:          [32]byte{0x02},
		WaitingRight:         [32]byte{0x03},
		SegmentStartPosition: big.NewInt(7),
		SegmentStartCycle:    big.NewInt(8),
		Responder:            0,
	}

	require.NoError(t, populateReadyToSealMatchResult(result, value))
	assert.Nil(t, result.CurrentHeight)
	assert.Equal(t, "7", result.SegmentStartPosition)
	assert.Equal(t, "8", result.SegmentStartCycle)
	assert.Equal(t, "ONE", result.Responder)
}

func TestPopulateSealedMatchResult(t *testing.T) {
	result := &MatchResult{}
	value := itournament.ITournamentSealedMatchView{
		AgreeState:         [32]byte{0x01},
		DivergencePosition: big.NewInt(9),
		DivergenceCycle:    big.NewInt(10),
		FinalStateOne:      [32]byte{0x02},
		FinalStateTwo:      [32]byte{0x03},
	}

	require.NoError(t, populateSealedMatchResult(result, value))
	assert.Equal(t, formatHash(value.AgreeState), result.AgreeState)
	assert.Equal(t, "9", result.DivergencePosition)
	assert.Equal(t, "10", result.DivergenceCycle)
	assert.Equal(t, formatHash(value.FinalStateOne), result.FinalStateOne)
	assert.Equal(t, formatHash(value.FinalStateTwo), result.FinalStateTwo)
}

func TestMatchPayloadRejectsNilPositions(t *testing.T) {
	assert.Error(t, populateBisectingMatchResult(&MatchResult{}, itournament.ITournamentBisectingMatchView{}))
	assert.Error(t, populateReadyToSealMatchResult(&MatchResult{}, itournament.ITournamentReadyToSealMatchView{}))
	assert.Error(t, populateSealedMatchResult(&MatchResult{}, itournament.ITournamentSealedMatchView{}))
}
