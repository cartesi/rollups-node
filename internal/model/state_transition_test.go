// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"fmt"
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestStateTransitionPins(t *testing.T) {
	t.Parallel()
	require.Equal(t, uint64(22), CartesiMachineMarchID)
	expected := common.HexToHash("0x558f38f2350970a0a566b2450afa64250059c1cff66c9a55560e5b93fa783926")
	require.Equal(t, expected, UarchPristineStateHash())
	changed := UarchPristineStateHash()
	changed[0] ^= 1
	require.Equal(t, expected, UarchPristineStateHash())
}

func TestCheckMarchID(t *testing.T) {
	t.Parallel()
	require.NoError(t, CheckMarchID("linked library", 22))
	for _, found := range []uint64{0, 21, 23, math.MaxUint64} {
		err := CheckMarchID("remote server", found)
		require.ErrorIs(t, err, ErrIncompatibleStateTransition)
		require.EqualError(t, err, fmt.Sprintf(
			"remote server marchid %d, expected 22: incompatible state transition", found))
	}
}

func TestCheckUarchPristineStateHash(t *testing.T) {
	t.Parallel()
	expected := common.HexToHash("0x558f38f2350970a0a566b2450afa64250059c1cff66c9a55560e5b93fa783926")
	require.NoError(t, CheckUarchPristineStateHash("linked library", expected))
	firstChanged, middleChanged, lastChanged := expected, expected, expected
	firstChanged[0] ^= 1
	middleChanged[len(middleChanged)/2] ^= 1
	lastChanged[len(lastChanged)-1] ^= 1
	for _, found := range []common.Hash{{}, firstChanged, middleChanged, lastChanged} {
		err := CheckUarchPristineStateHash("remote server", found)
		require.ErrorIs(t, err, ErrIncompatibleStateTransition)
		require.EqualError(t, err, fmt.Sprintf(
			"remote server pristine uarch hash %s, expected %s: incompatible state transition", found, expected))
	}
}
