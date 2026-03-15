// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package events

import (
	"fmt"
	"slices"
)

// Channel identifies a PostgreSQL NOTIFY/LISTEN channel.
// Each Channel maps 1:1 to a stage in the epoch processing pipeline.
type Channel string

const (
	// ChannelInputReceived signals that new inputs were written for an application.
	// Producer: EvmReader. Consumer: Advancer.
	ChannelInputReceived Channel = "input_received"

	// ChannelEpochClosed signals that an epoch transitioned to CLOSED status.
	// Producer: EvmReader. Consumer: Advancer.
	ChannelEpochClosed Channel = "epoch_closed"

	// ChannelInputsProcessed signals that all inputs for an epoch were processed.
	// Producer: Advancer. Consumer: Validator.
	ChannelInputsProcessed Channel = "inputs_processed"

	// ChannelClaimComputed signals that Merkle proofs and commitment are ready.
	// Producer: Validator. Consumer: Claimer, PRT.
	ChannelClaimComputed Channel = "claim_computed"

	// ChannelClaimSubmitted signals that a claim transaction was sent to L1.
	// Producer: Claimer. Consumer: (informational).
	ChannelClaimSubmitted Channel = "claim_submitted"

	// ChannelClaimAccepted signals terminal epoch state.
	// Producer: Claimer, PRT. Consumer: (informational).
	ChannelClaimAccepted Channel = "claim_accepted"

	// ChannelAppStateChanged signals an application state change.
	// Producer: any service that changes app lifecycle fields (or DB trigger).
	// Consumer: all services (re-read active application list).
	ChannelAppStateChanged Channel = "app_state_changed"
)

// AllChannels returns the complete set of valid channels.
func AllChannels() []Channel {
	return []Channel{
		ChannelInputReceived,
		ChannelEpochClosed,
		ChannelInputsProcessed,
		ChannelClaimComputed,
		ChannelClaimSubmitted,
		ChannelClaimAccepted,
		ChannelAppStateChanged,
	}
}

// ValidateChannel returns an error if ch is not a recognized channel.
func ValidateChannel(ch Channel) error {
	if slices.Contains(AllChannels(), ch) {
		return nil
	}
	return fmt.Errorf("events: unrecognized channel %q", ch)
}

// Notification is a received event. It carries enough information
// for the consumer to scope a database query. It does NOT carry
// authoritative state — the database is the source of truth.
type Notification struct {
	// Channel identifies what type of event occurred.
	Channel Channel `json:"ch"`

	// ApplicationID identifies the application this event pertains to.
	ApplicationID int64 `json:"app_id"`

	// EpochIndex is set for epoch-related events (epoch 0 is valid).
	EpochIndex uint64 `json:"epoch_idx"`
}
