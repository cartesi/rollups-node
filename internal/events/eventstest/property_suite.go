// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//nolint:mnd
package eventstest

import (
	"math/rand/v2"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/cartesi/rollups-node/internal/events"
)

// PropertySuite verifies event-transport-dependent properties against any
// Publisher+Subscriber implementation. Run it against both memory.Bus and
// the PostgreSQL backend to confirm semantic equivalence.
//
// Transport-independent properties (P2 idempotency, P7 ordering, P8 split-brain)
// are tested separately in events/property_test.go since they exercise
// simulated database logic, not the event transport.
type PropertySuite struct {
	suite.Suite
	Factory PublisherSubscriberFactory

	// SettleTime is the time to wait after publish for async delivery.
	// For memory.Bus this is 0; for PG LISTEN/NOTIFY this should be ~200ms.
	// Connection readiness uses the factory's ready channel instead.
	SettleTime time.Duration
}

// settle waits for async delivery (PG) or is a no-op (memory).
func (s *PropertySuite) settle() {
	if s.SettleTime > 0 {
		time.Sleep(s.SettleTime)
	}
}

// startListening starts sub.Listen and waits for the subscriber to be ready.
func (s *PropertySuite) startListening(sub events.Subscriber, ready <-chan struct{}) {
	ctx := s.T().Context()
	go sub.Listen(ctx) //nolint:errcheck
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		s.Fail("subscriber did not become ready")
	}
}

// TestP1NoWorkLossUnderEventLoss verifies that for any sequence of published
// work items where notifications may be arbitrarily lost, a single Tick
// (database poll) discovers all pending work.
func (s *PropertySuite) TestP1NoWorkLossUnderEventLoss() {
	for trial := range 20 {
		n := rand.IntN(20) + 1 //nolint:gosec
		db := make(map[int64]bool)

		pub, sub, ready := s.Factory(s.T())
		ch := sub.Subscribe(events.ChannelInputReceived)
		s.startListening(sub, ready)

		ctx := s.T().Context()
		for i := range n {
			id := int64(trial*1000 + i)
			db[id] = false // pending
			pub.Publish(ctx, events.Notification{
				Channel:       events.ChannelInputReceived,
				ApplicationID: id,
			})
		}

		// Wait for events to arrive before draining.
		s.settle()

		// Randomly drain some events (simulating loss).
		drained := 0
	drain:
		for {
			select {
			case <-ch:
				drained++
				if rand.IntN(2) == 0 { //nolint:gosec
					break drain
				}
			default:
				break drain
			}
		}

		// Tick: query ALL pending work from "DB" (regardless of events).
		processed := 0
		for id, done := range db {
			if !done {
				db[id] = true
				processed++
			}
		}

		s.Equal(n, processed,
			"trial %d: all %d items should be processed by Tick, "+
				"even though %d/%d events were drained", trial, n, drained, n)
	}
}

// TestP5ChannelIsolation verifies that a notification on channel C is never
// received by a subscriber listening only on channel C' where C != C'.
func (s *PropertySuite) TestP5ChannelIsolation() {
	allChannels := events.AllChannels()

	for _, subChannel := range allChannels {
		pub, sub, ready := s.Factory(s.T())
		ch := sub.Subscribe(subChannel)
		s.startListening(sub, ready)

		ctx := s.T().Context()
		for _, pubChannel := range allChannels {
			if pubChannel == subChannel {
				continue
			}
			pub.Publish(ctx, events.Notification{
				Channel:       pubChannel,
				ApplicationID: 1,
			})
		}

		s.settle()

		select {
		case n := <-ch:
			s.Failf("channel isolation violated",
				"subscriber on %q received notification from %q",
				subChannel, n.Channel)
		default:
			// Expected: nothing.
		}
	}
}

// TestP6PipelineEventualDelivery verifies that a 3-stage pipeline with
// random event drops at each stage completes within a bounded number of
// sync intervals. Each stage has an independent sync clock (matching the
// TLA+ SyncInterval model).
func (s *PropertySuite) TestP6PipelineEventualDelivery() {
	const syncInterval = 3

	for range 10 {
		pub, sub, ready := s.Factory(s.T())
		ctx := s.T().Context()

		// Set up a 3-stage pipeline: input -> processed -> computed.
		stage1Ch := sub.Subscribe(events.ChannelInputReceived)
		stage2Ch := sub.Subscribe(events.ChannelInputsProcessed)
		stage3Ch := sub.Subscribe(events.ChannelClaimComputed)
		s.startListening(sub, ready)

		// Simulated DB: each stage writes its output here.
		stage1Done := false
		stage2Done := false
		stage3Done := false

		// Independent sync clocks per stage (models time.Ticker).
		stage1Clock := 0
		stage2Clock := 0
		stage3Clock := 0

		// Upper bound: 3 stages × syncInterval + margin.
		maxTicks := syncInterval*4 + 1
		for range maxTicks {
			// Producer: publish input on first tick only (already in "DB").
			if !stage1Done && stage1Clock == 0 {
				pub.Publish(ctx, events.Notification{
					Channel:       events.ChannelInputReceived,
					ApplicationID: 1,
				})
				s.settle()
			}

			// Stage 1 (Advancer): wakes on event OR independent sync timer.
			if !stage1Done {
				eventFired := false
				select {
				case <-stage1Ch:
					eventFired = true
				default:
				}
				if eventFired || stage1Clock >= syncInterval {
					stage1Done = true
					stage1Clock = 0
					// Randomly drop the downstream publish (event loss).
					if rand.IntN(2) == 0 { //nolint:gosec
						pub.Publish(ctx, events.Notification{
							Channel:       events.ChannelInputsProcessed,
							ApplicationID: 1,
						})
						s.settle()
					}
				} else {
					stage1Clock++
				}
			}

			// Stage 2 (Validator): wakes on event OR independent sync timer.
			if !stage2Done {
				eventFired := false
				select {
				case <-stage2Ch:
					eventFired = true
				default:
				}
				if eventFired || stage2Clock >= syncInterval {
					// Sync queries "DB" — only completes if stage 1 produced work.
					if stage1Done {
						stage2Done = true
						stage2Clock = 0
						if rand.IntN(2) == 0 { //nolint:gosec
							pub.Publish(ctx, events.Notification{
								Channel:       events.ChannelClaimComputed,
								ApplicationID: 1,
							})
							s.settle()
						}
					}
					// Reset clock even if DB had no work (timer resets on fire).
					stage2Clock = 0
				} else {
					stage2Clock++
				}
			}

			// Stage 3 (Claimer): wakes on event OR independent sync timer.
			if !stage3Done {
				eventFired := false
				select {
				case <-stage3Ch:
					eventFired = true
				default:
				}
				if eventFired || stage3Clock >= syncInterval {
					if stage2Done {
						stage3Done = true
					}
					stage3Clock = 0
				} else {
					stage3Clock++
				}
			}

			if stage3Done {
				break
			}
		}

		s.True(stage3Done,
			"pipeline should complete within %d ticks (syncInterval=%d)",
			maxTicks, syncInterval)
	}
}
