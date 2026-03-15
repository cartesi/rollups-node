// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//nolint:mnd
package eventstest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/cartesi/rollups-node/internal/events"
)

// PublisherSubscriberFactory creates a matched Publisher and Subscriber
// pair for testing. The factory is called once per test case.
type PublisherSubscriberFactory func(t *testing.T) (events.Publisher, events.Subscriber)

// ContractSuite verifies that any Publisher+Subscriber implementation
// satisfies the events library's behavioral contract. Run it against
// every backend to guarantee semantic equivalence.
type ContractSuite struct {
	suite.Suite
	Factory PublisherSubscriberFactory
}

func (s *ContractSuite) TestSinglePublishSubscribe() {
	pub, sub := s.Factory(s.T())
	ch := sub.Subscribe(events.ChannelInputReceived)
	ctx := s.T().Context()
	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	expected := events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 42,
		EpochIndex:    7,
	}
	pub.Publish(ctx, expected)

	select {
	case actual := <-ch:
		s.Equal(expected.Channel, actual.Channel)
		s.Equal(expected.ApplicationID, actual.ApplicationID)
	case <-time.After(5 * time.Second):
		s.Fail("timed out waiting for notification")
	}
}

func (s *ContractSuite) TestChannelIsolation() {
	pub, sub := s.Factory(s.T())
	ch := sub.Subscribe(events.ChannelInputReceived)
	ctx := s.T().Context()
	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	// Publish on a different channel.
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelClaimComputed,
		ApplicationID: 1,
	})
	// Publish on the subscribed channel.
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 2,
	})

	select {
	case actual := <-ch:
		s.Equal(int64(2), actual.ApplicationID)
	case <-time.After(5 * time.Second):
		s.Fail("timed out waiting for notification")
	}
}

func (s *ContractSuite) TestBufferOverflowDropsWithoutBlocking() {
	pub, sub := s.Factory(s.T())
	ch := sub.Subscribe(events.ChannelInputReceived)
	ctx := s.T().Context()
	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	// Publish more than buffer size (64) without reading.
	for i := range 100 {
		pub.Publish(ctx, events.Notification{
			Channel:       events.ChannelInputReceived,
			ApplicationID: int64(i),
		})
	}

	// Give time for delivery.
	time.Sleep(100 * time.Millisecond)

	// Drain what we can — should be at most 64.
	drained := DrainChannel(ch)
	s.LessOrEqual(len(drained), 64)
	// Publisher must not have blocked.
}

func (s *ContractSuite) TestMultipleSubscriptions() {
	pub, sub := s.Factory(s.T())
	ch1 := sub.Subscribe(events.ChannelInputReceived)
	ch2 := sub.Subscribe(events.ChannelClaimComputed)
	ctx := s.T().Context()
	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelClaimComputed,
		ApplicationID: 2,
	})

	select {
	case n := <-ch1:
		s.Equal(events.ChannelInputReceived, n.Channel)
	case <-time.After(5 * time.Second):
		s.Fail("ch1: timed out")
	}
	select {
	case n := <-ch2:
		s.Equal(events.ChannelClaimComputed, n.Channel)
	case <-time.After(5 * time.Second):
		s.Fail("ch2: timed out")
	}
}

func (s *ContractSuite) TestSubscribeWithFilterByAppID() {
	pub, sub := s.Factory(s.T())
	filter := events.SubscriptionFilter{ApplicationIDs: []int64{42}}
	ch := sub.SubscribeWithFilter(filter, events.ChannelInputReceived)
	ctx := s.T().Context()
	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	// Publish for a non-matching app.
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 99,
	})
	// Publish for the matching app.
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 42,
	})

	select {
	case n := <-ch:
		s.Equal(int64(42), n.ApplicationID)
	case <-time.After(5 * time.Second):
		s.Fail("timed out waiting for filtered notification")
	}

	// Verify non-matching was not delivered.
	select {
	case n := <-ch:
		s.Failf("unexpected notification", "got app_id=%d", n.ApplicationID)
	default:
		// Expected: no more notifications.
	}
}
