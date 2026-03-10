package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const wrongAppID = "mismatch"

var manyEventTypes = []EventType{
	EventAppRegistered,
	EventAppDeactivated,
	EventAppReactivated,
	EventAppInoperable,
}

var manyApplicationIDs = []string{
	"app-1",
	"myApp",
	"echo-dapp",
}

var nilEvent Event

func TestSubscriptionFilter_SingleEventTypeAndApp(t *testing.T) {
	event := Event{
		Type:  EventAppRegistered,
		AppID: manyApplicationIDs[0],
	}

	filter := SubscriptionFilter{
		EventTypes: []EventType{event.Type},
		AppIDs:     []string{event.AppID},
	}

	assert.False(t, filter.Matches(nilEvent), "unexpected to match a nil event")

	assert.True(t, filter.Matches(event), "expected event to match filter")

	mismatch := event
	mismatch.Type = EventAppDeactivated
	assert.False(t, filter.Matches(mismatch), "unexpected match for wrong event type")

	mismatch = event
	mismatch.AppID = "other-app"
	assert.False(t, filter.Matches(mismatch), "unexpected match for wrong application ID")
}

func TestSubscriptionFilter_ManyEventTypeOfSignleApp(t *testing.T) {

	filter := SubscriptionFilter{
		EventTypes: manyEventTypes,
		AppIDs:     manyApplicationIDs[0:1],
	}

	assert.False(t, filter.Matches(nilEvent), "unexpected to match a nil event")

	for _, evType := range manyEventTypes {
		event := Event{Type: evType}

		assert.False(t, filter.Matches(event), "unexpected match of empty application ID")

		event.AppID = manyApplicationIDs[0]
		assert.True(t, filter.Matches(event), "expected event to match filter")

		event.AppID = wrongAppID
		assert.False(t, filter.Matches(event), "unexpected match for wrong application ID")
	}

	mismatch := Event{
		Type:  EventEpochOpened,
		AppID: manyApplicationIDs[0],
	}
	assert.False(t, filter.Matches(mismatch), "unexpected match for wrong event type")

}

func TestSubscriptionFilter_ManyEventTypeOfAnyApp(t *testing.T) {

	filter := SubscriptionFilter{EventTypes: manyEventTypes}

	assert.False(t, filter.Matches(nilEvent), "unexpected to match a nil event")

	for _, evType := range manyEventTypes {
		event := Event{Type: evType}

		assert.True(t, filter.Matches(event), "expected event to match filter")

		for _, appID := range manyApplicationIDs {
			event.AppID = appID
			assert.True(t, filter.Matches(event), "expected event to match filter")

			mismatch := event
			mismatch.Type = EventEpochOpened
			assert.False(t, filter.Matches(mismatch), "unexpected match for wrong event type")
		}
	}

}

func TestSubscriptionFilter_SingleEventTypeOfManyApps(t *testing.T) {

	filter := SubscriptionFilter{
		EventTypes: manyEventTypes[0:1],
		AppIDs:     manyApplicationIDs,
	}

	assert.False(t, filter.Matches(nilEvent), "unexpected to match a nil event")

	for _, appID := range manyApplicationIDs {
		event := Event{AppID: appID}

		assert.False(t, filter.Matches(event), "unexpected match of nil event type")

		event.Type = manyEventTypes[0]
		assert.True(t, filter.Matches(event), "expected event to match filter")

		event.Type = EventEpochOpened
		assert.False(t, filter.Matches(event), "unexpected match for wrong event type")
	}

	mismatch := Event{
		Type:  manyEventTypes[0],
		AppID: wrongAppID,
	}
	assert.False(t, filter.Matches(mismatch), "unexpected match for wrong application ID")
}

func TestSubscriptionFilter_AnyEventTypeOfManyApps(t *testing.T) {

	filter := SubscriptionFilter{AppIDs: manyApplicationIDs}

	assert.False(t, filter.Matches(nilEvent), "unexpected to match a nil event")

	for _, appID := range manyApplicationIDs {
		event := Event{AppID: appID}

		assert.True(t, filter.Matches(event), "expected event to match filter")

		for _, evType := range manyEventTypes {
			event.Type = evType
			assert.True(t, filter.Matches(event), "expected event to match filter")

			mismatch := event
			mismatch.AppID = wrongAppID
			assert.False(t, filter.Matches(mismatch), "unexpected match for wrong application ID")
		}
	}

}

func TestSubscriptionFilter_ManyEventTypesOfManyApps(t *testing.T) {

	filter := SubscriptionFilter{
		EventTypes: manyEventTypes,
		AppIDs:     manyApplicationIDs,
	}

	assert.False(t, filter.Matches(nilEvent), "unexpected to match a nil event")

	for _, evType := range manyEventTypes {
		event := Event{Type: evType}

		assert.False(t, filter.Matches(event), "unexpected match of nil application ID")

		for _, appID := range manyApplicationIDs {
			event.AppID = appID
			assert.True(t, filter.Matches(event), "expected event to match filter")

			mismatch := event
			mismatch.Type = EventEpochOpened
			assert.False(t, filter.Matches(mismatch), "unexpected match for wrong event type")

			mismatch = Event{AppID: appID}
			assert.False(t, filter.Matches(mismatch), "unexpected match of nil event type")
		}

		mismatch := event
		mismatch.AppID = wrongAppID
		assert.False(t, filter.Matches(mismatch), "unexpected match for wrong application ID")
	}

}

func TestSubscriptionFilter_EveryEvent(t *testing.T) {

	filter := SubscriptionFilter{}

	assert.True(t, filter.Matches(nilEvent), "expected to match a nil event")

	event := Event{
		Type:  EventEpochOpened,
		AppID: wrongAppID,
	}
	assert.True(t, filter.Matches(event), "expected event to match filter")

	for _, appID := range manyApplicationIDs {
		for _, evType := range manyEventTypes {
			event.Type = evType
			event.AppID = appID
			assert.True(t, filter.Matches(event), "expected event to match filter")
		}
	}

}
