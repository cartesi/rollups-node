// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllChannels(t *testing.T) {
	channels := AllChannels()
	assert.Len(t, channels, 9)

	expected := []Channel{
		ChannelInputReceived,
		ChannelEpochClosed,
		ChannelInputsProcessed,
		ChannelClaimComputed,
		ChannelClaimSubmitted,
		ChannelClaimAccepted,
		ChannelSettleSubmitted,
		ChannelJoinSubmitted,
		ChannelAppStateChanged,
	}
	assert.ElementsMatch(t, expected, channels)
}

func TestValidateChannel(t *testing.T) {
	for _, ch := range AllChannels() {
		require.NoError(t, ValidateChannel(ch), "valid channel %q should not error", ch)
	}
}

func TestValidateChannelRejectsUnknown(t *testing.T) {
	err := ValidateChannel("bogus_channel")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus_channel")
}

func TestValidateChannelRejectsEmpty(t *testing.T) {
	err := ValidateChannel("")
	require.Error(t, err)
}
