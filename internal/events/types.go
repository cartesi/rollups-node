package events

import (
	"context"
	"encoding/json"
	"time"
)

//
// Low level driver interface. For alternative implementations.
//

type Notification struct {
	Payload string
	Topic   string
}

type Driver interface {
	Notify(ctx context.Context, notification *Notification) error

	Close(ctx context.Context) error
	Connect(ctx context.Context) error
	Listen(ctx context.Context, topics []string) error
	Ping(ctx context.Context) error
	Unlisten(ctx context.Context, topics []string) error
	WaitForNotification(ctx context.Context) (*Notification, error)
}

//
// High level interface.
//

type ApplicationID string

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
	AppID     ApplicationID   `json:"app_id"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

// Publisher publishes events.
type Publisher interface {
	Publish(ctx context.Context, event Event) error
}

// Subscriber subscribes to events with a filter and receives matched events via a channel.
type Subscriber interface {
	Subscribe(ctx context.Context, filter SubscriptionFilter) (Subscription, error)
}

// SubscriptionFilter filters events by types and app IDs.
type SubscriptionFilter struct {
	EventTypes []EventType
	AppIDs     []ApplicationID
}

type Subscription interface {
	// The event channel will be closed when the subscription is clised or an unrecoverable error occurs.
	Close(ctx context.Context) error
	Channel() <-chan Event
}

type Service interface {
	BaseService
	Publisher
	Subscriber
}