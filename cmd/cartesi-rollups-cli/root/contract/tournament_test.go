// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
)

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
			matchIDHash: matchID,
			one:         commitOne,
			two:         commitTwo,
			leftOfTwo:   [32]byte{0x33},
			blockNumber: 100,
			txHash:      common.HexToHash("0xC1"),
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
			matchIDHash: [32]byte{0xAA},
			otherParent: [32]byte{0xBB},
			leftNode:    [32]byte{0xCC},
			blockNumber: 100,
			txHash:      common.HexToHash("0xDD"),
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
	assert.Equal(t, uint64(100), result[0].BlockNumber)
	assert.Equal(t, raw[0].txHash.Hex(), result[0].TxHash)

	assert.Equal(t, uint64(200), result[1].BlockNumber)
}
