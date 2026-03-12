// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contracttests

import (
	"context"
	"encoding/json"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/cartesi/rollups-node/internal/events"
)

type TimeoutGroup struct {
	Context context.Context
	cancel  context.CancelFunc
	channel chan struct{}
	count   int
}

func MakeTimeoutGroup(ctx context.Context, timeout time.Duration, count int) *TimeoutGroup {
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	return &TimeoutGroup{
		Context: tCtx,
		cancel:  cancel,
		channel: make(chan struct{}, count),
	}
}

func (g *TimeoutGroup) Go(fn func(ctx context.Context)) {
	g.count++
	go func() {
		fn(g.Context)
		g.channel <- struct{}{}
	}()
}

func (g *TimeoutGroup) Close() {
	g.cancel()
}

func (g *TimeoutGroup) Wait() {
	for g.count > 0 {
		select {
		case <-g.channel:
			g.count--
		case <-g.Context.Done():
			panic("tasks did not complete before timeout")
		}
	}
}

const (
	eventCount      = 50
	subsCount       = 5 // TODO: cannot increase above the limit of simultaneous connections to PostgreSQL.
	testcaseTimeout = 5 * time.Second
)

var expected = events.Event{
	Type:      events.EventAppRegistered,
	AppID:     "echo-dapp",
	Payload:   json.RawMessage(`"Hello, World!"`),
	Timestamp: time.Now().Truncate(time.Second),
}

type EventsServiceFactory func() events.Service

type PublisherTestSuite struct {
	suite.Suite
	CreateService EventsServiceFactory
	Service       events.Service
}

func NewPublisherTestSuite(factory EventsServiceFactory) *PublisherTestSuite {
	return &PublisherTestSuite{CreateService: factory}
}

func (s *PublisherTestSuite) SetupTest() {
	s.Service = s.CreateService()
}

func (s *PublisherTestSuite) TeardownTest() {
	s.Service.Close()
}

func (s *PublisherTestSuite) TestSingleEvent() {
	req := s.Require()
	ctx := s.T().Context()

	tmGrp := MakeTimeoutGroup(ctx, testcaseTimeout, 1)
	defer tmGrp.Wait()

	ch, err := s.Service.Subscribe(ctx, events.SubscriptionFilter{})
	req.NoError(err)

	tmGrp.Go(func(ctx context.Context) {
		select {
		case actual := <-ch:
			req.Equal(actual, expected)
		case <-ctx.Done():
			req.Fail("no event received before timeout")
		}
	})

	req.NoError(s.Service.Publish(ctx, expected))
}

func (s *PublisherTestSuite) TestRepeatedEvents() {
	req := s.Require()
	ctx := s.T().Context()

	tmGrp := MakeTimeoutGroup(ctx, testcaseTimeout, 1)
	defer tmGrp.Wait()

	ch, err := s.Service.Subscribe(ctx, events.SubscriptionFilter{})
	req.NoError(err)

	tmGrp.Go(func(ctx context.Context) {
		for range eventCount {
			select {
			case actual := <-ch:
				req.Equal(actual, expected)
			case <-ctx.Done():
				req.Fail("got no events before timeout")
			}
		}
	})

	for range eventCount {
		req.NoError(s.Service.Publish(ctx, expected))
	}
}

func (s *PublisherTestSuite) TestSinglePublisherManySubscribers() {
	req := s.Require()
	ctx := s.T().Context()

	tmGrp := MakeTimeoutGroup(ctx, testcaseTimeout, subsCount)
	defer tmGrp.Wait()

	for range subsCount {
		ch, err := s.Service.Subscribe(ctx, events.SubscriptionFilter{})
		req.NoError(err)

		tmGrp.Go(func(ctx context.Context) {
			select {
			case actual := <-ch:
				req.Equal(actual, expected)
			case <-ctx.Done():
				req.Fail("got no events before timeout")
			}
		})
	}

	req.NoError(s.Service.Publish(ctx, expected))
}

// Publishers with multiple events to a single subscriber
func (s *PublisherTestSuite) TestManyPublishersSingleSubscriber() {
	pubsCount := 50

	req := s.Require()
	ctx := s.T().Context()

	tmGrp := MakeTimeoutGroup(ctx, testcaseTimeout, 1)
	defer tmGrp.Wait()

	ch, err := s.Service.Subscribe(ctx, events.SubscriptionFilter{})
	req.NoError(err)

	tmGrp.Go(func(ctx context.Context) {
		for i := range pubsCount {
			select {
			case actual := <-ch:
				req.Equal(actual, expected)
			case <-ctx.Done():
				req.Fail("got only %d/%d events before timeout", i+1, pubsCount)
			}
		}
	})

	for range pubsCount {
		tmGrp.Go(func(ctx context.Context) {
			err := s.Service.Publish(ctx, expected)
			req.NoError(err)
		})
	}
}

// Slow subscribers
func (s *PublisherTestSuite) TestManySlowSubscribers() {
	req := s.Require()
	ctx := s.T().Context()

	tmGrp := MakeTimeoutGroup(ctx, testcaseTimeout, subsCount)
	defer tmGrp.Wait()

	startCh := make(chan struct{}, subsCount)

	for range subsCount {
		ch, err := s.Service.Subscribe(ctx, events.SubscriptionFilter{})
		req.NoError(err)

		tmGrp.Go(func(ctx context.Context) {
			<-startCh // pause the subscriber to fill any queue on the pub/sub system

			for i := range eventCount {
				select {
				case actual := <-ch:
					req.Equal(actual, expected)
				case <-ctx.Done():
					req.Fail("got only %d/%d events before timeout", i+1, eventCount)
				}
			}
		})
	}

	for range eventCount {
		req.NoError(s.Service.Publish(ctx, expected))
	}

	for range subsCount {
		startCh <- struct{}{} // resume paused subscriber
	}
}

// Filter events
func (s *PublisherTestSuite) TestFilteredEventsWithNewSubscriptions() {
	var manyEventTypes = []events.EventType{
		events.EventAppRegistered,
		events.EventAppDeactivated,
		// events.EventAppReactivated,
		// events.EventAppInoperable,
	}
	var manyApplicationIDs = []events.ApplicationID{
		"app-1",
		// "myApp",
		"echo-dapp",
	}

	phaseCount := 3
	// TODO: cannot increase above the limit of simultaneous connections to PostgreSQL.
	subsCount := len(manyEventTypes) * len(manyApplicationIDs) * phaseCount

	req := s.Require()
	ctx := s.T().Context()

	tmGrp := MakeTimeoutGroup(ctx, testcaseTimeout, subsCount)
	defer tmGrp.Wait()

	for phase := range phaseCount {

		for _, evtType := range manyEventTypes {
			for _, appID := range manyApplicationIDs {
				ch, err := s.Service.Subscribe(ctx, events.SubscriptionFilter{
					EventTypes: []events.EventType{evtType},
					AppIDs:     []events.ApplicationID{appID},
				})
				req.NoError(err)

				tmGrp.Go(func(ctx context.Context) {
					for i := range eventCount * (phaseCount - phase) {
						select {
						case actual := <-ch:
							req.Equal(actual.Type, evtType)
							req.Equal(actual.AppID, appID)
							req.Equal(actual.Payload, expected.Payload)
							req.Equal(actual.Timestamp, expected.Timestamp)
						case <-ctx.Done():
							req.Fail("got only %d/%d events before timeout", i+1, eventCount)
						}
					}
				})
			}
		}

		for range eventCount {
			for _, evtType := range manyEventTypes {
				for _, appID := range manyApplicationIDs {
					var expected = events.Event{
						Type:      evtType,
						AppID:     appID,
						Payload:   expected.Payload,
						Timestamp: expected.Timestamp,
					}
					req.NoError(s.Service.Publish(ctx, expected))
				}
			}
		}

	}
}

func (s *PublisherTestSuite) TestCancelledContext() {
	req := s.Require()
	ctx := s.T().Context()

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	err := s.Service.Publish(cancelledCtx, expected)
	req.ErrorIs(err, context.Canceled)

	ch, err := s.Service.Subscribe(cancelledCtx, events.SubscriptionFilter{})
	req.ErrorIs(err, context.Canceled)

	select {
	case evt, open := <-ch:
		req.Equal(evt, events.Event{})
		req.False(open)
	case <-time.After(testcaseTimeout):
		req.Fail("subscription blocked on canceled context")
	}
}

func (s *PublisherTestSuite) TestContextCancelledLater() {
	req := s.Require()
	ctx := s.T().Context()

	tmGrp := MakeTimeoutGroup(ctx, testcaseTimeout, 1)
	defer tmGrp.Wait()

	newCtx, cancel := context.WithCancel(tmGrp.Context)

	ch, err := s.Service.Subscribe(newCtx, events.SubscriptionFilter{})
	req.NoError(err)

	tmGrp.Go(func(ctx context.Context) {
		select {
		case evt, open := <-ch:
			req.Equal(evt, events.Event{})
			req.False(open)
		case <-ctx.Done():
			req.Fail("subscription blocked on canceled context")
		}
	})

	cancel()
	s.Service.Close()

	err = s.Service.Publish(newCtx, expected)
	req.ErrorIs(err, context.Canceled)
}
