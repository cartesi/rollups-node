// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package eventstest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/events"
)

func TestRecorderCapturesEvents(t *testing.T) {
	r := &Recorder{}
	ctx := context.Background()

	r.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})
	r.Publish(ctx, events.Notification{
		Channel:       events.ChannelEpochClosed,
		ApplicationID: 2,
		EpochIndex:    5,
	})

	evts := r.Events()
	require.Len(t, evts, 2)
	assert.Equal(t, events.ChannelInputReceived, evts[0].Channel)
	assert.Equal(t, events.ChannelEpochClosed, evts[1].Channel)
}

func TestRecorderEventsForChannel(t *testing.T) {
	r := &Recorder{}
	ctx := context.Background()

	r.Publish(ctx, events.Notification{Channel: events.ChannelInputReceived, ApplicationID: 1})
	r.Publish(ctx, events.Notification{Channel: events.ChannelEpochClosed, ApplicationID: 2})
	r.Publish(ctx, events.Notification{Channel: events.ChannelInputReceived, ApplicationID: 3})

	filtered := r.EventsForChannel(events.ChannelInputReceived)
	require.Len(t, filtered, 2)
	assert.Equal(t, int64(1), filtered[0].ApplicationID)
	assert.Equal(t, int64(3), filtered[1].ApplicationID)
}

func TestRecorderEventsForChannelEmpty(t *testing.T) {
	r := &Recorder{}
	r.Publish(context.Background(), events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})

	filtered := r.EventsForChannel(events.ChannelClaimComputed)
	assert.Empty(t, filtered)
}

func TestRecorderReset(t *testing.T) {
	r := &Recorder{}
	r.Publish(context.Background(), events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})
	require.Len(t, r.Events(), 1)

	r.Reset()
	assert.Empty(t, r.Events())
}

func TestRecorderEventsReturnsCopy(t *testing.T) {
	r := &Recorder{}
	r.Publish(context.Background(), events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})

	evts := r.Events()
	evts[0].ApplicationID = 999

	// Original should be unchanged.
	assert.Equal(t, int64(1), r.Events()[0].ApplicationID)
}
