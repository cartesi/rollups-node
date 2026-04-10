// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package events

// ExternalNotificationChannels are published for external consumers
// (for example direct PG LISTEN or a future WS subscription service),
// but are not subscribed to by in-node services.
var ExternalNotificationChannels = []Channel{
	ChannelClaimSubmitted,
	ChannelClaimAccepted,
	ChannelSettleSubmitted,
	ChannelJoinSubmitted,
}

// EVMReaderChannels returns the channels subscribed to by the evm-reader.
func EVMReaderChannels() []Channel {
	return copyChannels([]Channel{ChannelAppStateChanged})
}

// AdvancerChannels returns the channels subscribed to by the advancer.
func AdvancerChannels() []Channel {
	return copyChannels([]Channel{
		ChannelInputReceived,
		ChannelEpochClosed,
		ChannelAppStateChanged,
	})
}

// ValidatorChannels returns the channels subscribed to by the validator.
func ValidatorChannels() []Channel {
	return copyChannels([]Channel{
		ChannelInputsProcessed,
		ChannelAppStateChanged,
	})
}

// ClaimerChannels returns the channels subscribed to by the claimer.
func ClaimerChannels() []Channel {
	return copyChannels([]Channel{
		ChannelClaimComputed,
		ChannelAppStateChanged,
	})
}

// PRTChannels returns the channels subscribed to by the PRT service.
func PRTChannels() []Channel {
	return copyChannels([]Channel{
		ChannelClaimComputed,
		ChannelAppStateChanged,
	})
}

func copyChannels(channels []Channel) []Channel {
	result := make([]Channel, len(channels))
	copy(result, channels)
	return result
}
