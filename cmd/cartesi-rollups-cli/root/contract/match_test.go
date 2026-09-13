// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
