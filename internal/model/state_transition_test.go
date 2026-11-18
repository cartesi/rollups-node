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
	require.Equal(t, uint64(23), CartesiMachineMarchID)
	expected := common.HexToHash("0x89ae34ba1dc65a8a3668b484bcedf897665c803c41dc9ef9f1faf147b9d443d7")
	require.Equal(t, expected, UarchPristineStateHash())
	changed := UarchPristineStateHash()
	changed[0] ^= 1
	require.Equal(t, expected, UarchPristineStateHash())
}

func TestCheckMarchID(t *testing.T) {
	t.Parallel()
	require.NoError(t, CheckMarchID("linked library", 23))
	for _, found := range []uint64{0, 22, 24, math.MaxUint64} {
		err := CheckMarchID("remote server", found)
		require.ErrorIs(t, err, ErrIncompatibleStateTransition)
		require.EqualError(t, err, fmt.Sprintf(
			"remote server marchid %d, expected 23: incompatible state transition", found))
	}
}

func TestCheckUarchPristineStateHash(t *testing.T) {
	t.Parallel()
	expected := common.HexToHash("0x89ae34ba1dc65a8a3668b484bcedf897665c803c41dc9ef9f1faf147b9d443d7")
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
