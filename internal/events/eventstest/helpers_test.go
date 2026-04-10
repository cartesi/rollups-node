// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package eventstest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/events"
)

func TestDrainChannelReturnsBuffered(t *testing.T) {
	ch := make(chan events.Notification, 3)
	ch <- events.Notification{Channel: events.ChannelInputReceived, ApplicationID: 1}
	ch <- events.Notification{Channel: events.ChannelInputReceived, ApplicationID: 2}
	ch <- events.Notification{Channel: events.ChannelInputReceived, ApplicationID: 3}

	drained := DrainChannel(ch)
	require.Len(t, drained, 3)
	assert.Equal(t, int64(1), drained[0].ApplicationID)
	assert.Equal(t, int64(3), drained[2].ApplicationID)
}

func TestDrainChannelEmptyChannel(t *testing.T) {
	ch := make(chan events.Notification, 10)
	drained := DrainChannel(ch)
	assert.Empty(t, drained)
}

func TestDrainChannelClosedChannel(t *testing.T) {
	ch := make(chan events.Notification, 2)
	ch <- events.Notification{Channel: events.ChannelInputReceived, ApplicationID: 1}
	close(ch)

	drained := DrainChannel(ch)
	require.Len(t, drained, 1)
	assert.Equal(t, int64(1), drained[0].ApplicationID)
}
