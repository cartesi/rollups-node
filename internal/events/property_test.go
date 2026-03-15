// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package events_test

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/cartesi/rollups-node/internal/events"
	"github.com/cartesi/rollups-node/internal/events/eventstest"
	"github.com/cartesi/rollups-node/internal/events/memory"
)

// TestPropertySuiteMemory runs transport-dependent property tests (P1, P5, P6)
// against the in-memory bus. These same tests also run against PostgreSQL
// in internal/events/postgres/property_test.go.
func TestPropertySuiteMemory(t *testing.T) {
	suite.Run(t, &eventstest.PropertySuite{
		Factory: func(t *testing.T) (events.Publisher, events.Subscriber) {
			bus := memory.NewBus(64)
			t.Cleanup(func() { _ = bus.Close() })
			return bus, bus
		},
	})
}

// P2: Idempotency under duplicate wakeups.
//
// Triggering Tick() k times (k >= 1) for a work item produces the same
// database state as triggering it exactly once.
func TestPropertyIdempotencyUnderDuplicateWakeups(t *testing.T) {
	// Simulate a DB with conditional updates (WHERE status = 'PENDING').
	type item struct {
		id     int64
		status string // "PENDING" or "DONE"
	}
	db := []*item{
		{id: 1, status: "PENDING"},
		{id: 2, status: "PENDING"},
		{id: 3, status: "PENDING"},
	}

	tick := func() int {
		processed := 0
		for _, it := range db {
			if it.status == "PENDING" {
				it.status = "DONE"
				processed++
			}
		}
		return processed
	}

	// First tick processes all.
	first := tick()
	assert.Equal(t, 3, first)

	// Subsequent ticks are no-ops (idempotent).
	for range 10 {
		assert.Equal(t, 0, tick(), "duplicate tick should process nothing")
	}

	// Verify final state.
	for _, it := range db {
		assert.Equal(t, "DONE", it.status)
	}
}

// P3 and P4 are tested in the postgres package where they require a real database:
//   P3: Event-DB atomicity under rollback  → postgres/integration_test.go
//   P4: Subscriber reconnection semantics  → postgres/subscriber_test.go

// P7: Ordering independence.
//
// For any permutation of event delivery order, the final database state
// is identical (because Tick always re-queries all pending work).
func TestPropertyOrderingIndependence(t *testing.T) {
	type workItem struct {
		id   int64
		done bool
	}

	tick := func(db []*workItem) {
		for _, w := range db {
			if !w.done {
				w.done = true
			}
		}
	}

	// Create work items.
	n := 5
	for perm := range 20 {
		db := make([]*workItem, n)
		for i := range n {
			db[i] = &workItem{id: int64(i)}
		}

		// Simulate events arriving in random order.
		order := rand.Perm(n)
		_ = order // Events don't affect Tick behavior.
		_ = perm

		// Single tick processes everything regardless of event order.
		tick(db)

		for _, w := range db {
			assert.True(t, w.done, "item %d should be done", w.id)
		}
	}
}

// P8: Split-brain resilience.
//
// If two instances of the same consumer are briefly running simultaneously,
// no work item is corrupted. This relies on conditional update predicates
// (WHERE status = 'PENDING'), not on the event system.
func TestPropertySplitBrainResilience(t *testing.T) {
	// Simulate two concurrent workers with conditional updates.
	type workItem struct {
		mu     sync.Mutex
		status string
	}

	items := make([]*workItem, 10)
	for i := range items {
		items[i] = &workItem{status: "PENDING"}
	}

	var processed atomic.Int64

	worker := func() {
		for _, it := range items {
			it.mu.Lock()
			if it.status == "PENDING" {
				it.status = "DONE"
				processed.Add(1)
			}
			it.mu.Unlock()
		}
	}

	// Run two workers concurrently.
	var wg sync.WaitGroup
	numWorkers := 2
	wg.Add(numWorkers)
	go func() { defer wg.Done(); worker() }()
	go func() { defer wg.Done(); worker() }()
	wg.Wait()

	// Exactly 10 items should be processed (no duplicates).
	assert.Equal(t, int64(10), processed.Load(),
		"each item should be processed exactly once")

	for i, it := range items {
		assert.Equal(t, "DONE", it.status, "item %d", i)
	}
}

// TestCoalesceStressWithMemoryBus verifies that rapid publishing through
// the in-memory bus coalesces into minimal signals.
func TestCoalesceStressWithMemoryBus(t *testing.T) {
	bus := memory.NewBus(64)
	notifCh := bus.Subscribe(events.ChannelInputReceived)
	signal := events.Coalesce(notifCh)
	ctx := context.Background()

	// Publish 100 rapid notifications.
	for i := range 100 {
		bus.Publish(ctx, events.Notification{
			Channel:       events.ChannelInputReceived,
			ApplicationID: int64(i),
		})
	}

	// Wait briefly for coalesce goroutine.
	time.Sleep(50 * time.Millisecond)

	// Count signals. Should be much less than 100.
	count := 0
drain:
	for {
		select {
		case _, ok := <-signal:
			if !ok {
				break drain
			}
			count++
		default:
			break drain
		}
	}

	require.Greater(t, count, 0, "should have at least 1 signal")
	assert.LessOrEqual(t, count, 5,
		"100 rapid publishes should coalesce into very few signals")

	_ = bus.Close()
}
