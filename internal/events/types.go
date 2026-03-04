package events

import (
	"context"
	"encoding/json"
	"slices"
	"time"
)

// EventType is the domain event type.
type EventType string

const (
	// Application events
	EventAppRegistered  EventType = "app.registered"
	EventAppDeactivated EventType = "app.deactivated"
	EventAppReactivated EventType = "app.reactivated"
	EventAppInoperable  EventType = "app.inoperable"

	// Epoch events
	EventEpochOpened          EventType = "epoch.opened"
	EventEpochClosed          EventType = "epoch.closed"
	EventEpochInputsProcessed EventType = "epoch.inputs_processed"
	EventEpochClaimCalculated EventType = "epoch.claim_calculated"
	EventEpochClaimSubmitted  EventType = "epoch.claim_submitted"
	EventEpochClaimAccepted   EventType = "epoch.claim_accepted"
	EventEpochClaimRejected   EventType = "epoch.claim_rejected"

	// Input events
	EventInputReceived EventType = "input.received"
)

// Event represents a domain event.
type Event struct {
	Type      EventType       `json:"type"`
	AppID     string          `json:"app_id"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

// Publisher publishes events.
type Publisher interface {
	Publish(ctx context.Context, e Event) error
}

// Subscriber subscribes to events with a filter and receives matched events via a channel.
// The returned channel will be closed when the context is cancelled or an unrecoverable error occurs.
type Subscriber interface {
	Subscribe(ctx context.Context, filter SubscriptionFilter) (<-chan Event, error)
}

// SubscriptionFilter filters events by types and app IDs.
type SubscriptionFilter struct {
	EventTypes []EventType
	AppIDs     []string
}

func (f SubscriptionFilter) Matches(e Event) bool {
	if len(f.EventTypes) > 0 {
		if !slices.Contains(f.EventTypes, e.Type) {
			return false
		}
	}
	if len(f.AppIDs) > 0 {
		if !slices.Contains(f.AppIDs, e.AppID) {
			return false
		}
	}
	return true
}

type Service interface {
	Publisher
	Subscriber
	Close()
}
