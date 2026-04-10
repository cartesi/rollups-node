// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package trace_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/events"
	"github.com/cartesi/rollups-node/internal/events/memory"
	"github.com/cartesi/rollups-node/internal/events/trace"
)

// TLA+ model constants matching MC.tla.
const (
	tlaMaxItems     = 3
	tlaSyncInterval = 2
	simMaxSteps     = 30

	statusAbsent  = "ABSENT"
	statusPending = "PENDING"
	statusDone    = "DONE"
	stateIdle     = "idle"
	stateDead     = "dead"

	filePerms = 0644
)

var tlaWorkers = []string{"w1", "w2"}

// action represents a TLA+ action that can be taken.
type action struct {
	name   string
	worker string // for worker-specific actions
}

// enabledActions returns all actions whose preconditions hold in the current state.
func enabledActions(
	ec map[int]bool,
	ws map[string]string,
	wc map[string]int,
	producerNext int,
) []action {
	var actions []action

	// ProduceItem: producerNext <= MaxItems
	if producerNext <= tlaMaxItems {
		actions = append(actions, action{name: "ProduceItem"})
	}

	// EventWakeup(w): worker idle AND eventChannel non-empty
	hasEvents := false
	for _, v := range ec {
		if v {
			hasEvents = true
			break
		}
	}
	for _, w := range tlaWorkers {
		if ws[w] == stateIdle && hasEvents {
			actions = append(actions, action{name: "EventWakeup", worker: w})
		}
	}

	// SyncWakeup(w): worker idle AND clock >= SyncInterval
	for _, w := range tlaWorkers {
		if ws[w] == stateIdle && wc[w] >= tlaSyncInterval {
			actions = append(actions, action{name: "SyncWakeup", worker: w})
		}
	}

	// ClockTick: some idle worker with clock < SyncInterval
	for _, w := range tlaWorkers {
		if ws[w] == stateIdle && wc[w] < tlaSyncInterval {
			actions = append(actions, action{name: "ClockTick", worker: w})
		}
	}

	// WorkerCrash(w): worker idle
	for _, w := range tlaWorkers {
		if ws[w] == stateIdle {
			actions = append(actions, action{name: "WorkerCrash", worker: w})
		}
	}

	// WorkerRestart(w): worker dead
	for _, w := range tlaWorkers {
		if ws[w] == stateDead {
			actions = append(actions, action{name: "WorkerRestart", worker: w})
		}
	}

	return actions
}

// TestTraceGeneration runs a deterministic simulation of the TLA+ model
// using the real memory.Bus and Coalesce(), records a trace of TLA+ states,
// and validates safety invariants. If TLA_TOOLS_JAR is set (or tla2tools.jar
// is found in a standard location), it also runs TLC to validate the trace
// against the HybridEvents specification.
func TestTraceGeneration(t *testing.T) {
	seeds := []uint64{42, 123, 7, 999, 2024}

	for _, seed := range seeds {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			rec, bus, coalesced := runSimulation(t, seed)
			defer func() {
				_ = bus.Close()
				// Verify the Coalesce goroutine exited cleanly by
				// confirming it closed the signal channel. Drain any
				// buffered value first (buffer size is 1).
				for range coalesced {
				}
			}()

			verifySafetyInvariants(t, rec)
			runTLCValidation(t, rec, seed)
		})
	}
}

// runSimulation executes a deterministic simulation of the TLA+ model,
// using the real memory.Bus and events.Coalesce for event delivery.
func runSimulation(t *testing.T, seed uint64) (*trace.Recorder, *memory.Bus, <-chan struct{}) {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, 0)) //nolint:gosec

	bus := memory.NewBus(64)
	notifCh := bus.Subscribe(events.ChannelInputReceived)
	// Start the Coalesce goroutine so Bus.Publish exercises the real
	// event pipeline (useful under -race). The signal is not consumed
	// by the simulation — see EventWakeup comment.
	coalesced := events.Coalesce(notifCh) // intentionally not consumed; goroutine exercises -race
	ctx := context.Background()

	// Initialize TLA+ state (matching HybridEvents.Init).
	db := make(map[int]string, tlaMaxItems)
	for i := 1; i <= tlaMaxItems; i++ {
		db[i] = statusAbsent
	}
	ec := make(map[int]bool)
	ws := make(map[string]string, len(tlaWorkers))
	wc := make(map[string]int, len(tlaWorkers))
	for _, w := range tlaWorkers {
		ws[w] = stateIdle
		wc[w] = 0
	}
	processed := make(map[int]bool)
	producerNext := 1

	rec := trace.NewRecorder(tlaMaxItems, tlaWorkers, tlaSyncInterval)
	rec.Record(trace.Snapshot("Init", db, ec, ws, wc, processed, producerNext))

	for range simMaxSteps {
		actions := enabledActions(ec, ws, wc, producerNext)
		if len(actions) == 0 {
			break
		}

		act := actions[rng.IntN(len(actions))]

		switch act.name {
		case "ProduceItem":
			db[producerNext] = statusPending
			delivered := rng.IntN(2) == 0
			if delivered {
				bus.Publish(ctx, events.Notification{
					Channel:       events.ChannelInputReceived,
					ApplicationID: int64(producerNext),
				})
				ec[producerNext] = true
			}
			producerNext++

		case "EventWakeup":
			// The TLA+ model clears the event channel atomically
			// (eventChannel' = {}). The simulation tracks this via the
			// ec map. We do not consume from the coalesced channel
			// because the state transition is fully determined by the
			// simulated DB, not the signal — matching the design
			// invariant "events affect WHEN, not WHAT." The Coalesce
			// goroutine still runs (exercised by Bus.Publish above)
			// and is validated by dedicated unit and property tests.
			for i := 1; i <= tlaMaxItems; i++ {
				if db[i] == statusPending {
					db[i] = statusDone
					processed[i] = true
				}
			}
			ec = make(map[int]bool)

		case "SyncWakeup":
			for i := 1; i <= tlaMaxItems; i++ {
				if db[i] == statusPending {
					db[i] = statusDone
					processed[i] = true
				}
			}
			wc[act.worker] = 0

		case "ClockTick":
			wc[act.worker]++

		case "WorkerCrash":
			ws[act.worker] = stateDead

		case "WorkerRestart":
			ws[act.worker] = stateIdle
			wc[act.worker] = tlaSyncInterval
		}

		rec.Record(trace.Snapshot(act.name, db, ec, ws, wc, processed, producerNext))
	}

	t.Logf("seed %d: generated trace with %d states", seed, len(rec.States))
	return rec, bus, coalesced
}

// verifySafetyInvariants checks TLA+ safety properties on every recorded state.
func verifySafetyInvariants(t *testing.T, rec *trace.Recorder) {
	t.Helper()
	validStatuses := []string{statusAbsent, statusPending, statusDone}
	validWorkerStates := []string{stateIdle, "processing", stateDead}

	for i, s := range rec.States {
		// Safety_NoDuplicateProcessing: db[i] = "DONE" => i in processed.
		for item, status := range s.DB {
			if status == statusDone {
				assert.True(t, s.Processed[item],
					"step %d (%s): item %d is DONE but not in processed",
					i, s.Action, item)
			}
		}

		// Safety_TypeOK: check domains.
		for item, status := range s.DB {
			assert.True(t, item >= 1 && item <= tlaMaxItems,
				"step %d: db key %d out of range", i, item)
			assert.Contains(t, validStatuses, status,
				"step %d: invalid db status %q", i, status)
		}
		for _, status := range s.WorkerState {
			assert.Contains(t, validWorkerStates, status,
				"step %d: invalid worker status %q", i, status)
		}
		for _, clock := range s.WorkerClock {
			assert.True(t, clock >= 0 && clock <= tlaSyncInterval,
				"step %d: clock %d out of range", i, clock)
		}
		assert.True(t, s.ProducerNext >= 1 && s.ProducerNext <= tlaMaxItems+1,
			"step %d: producerNext %d out of range", i, s.ProducerNext)
	}
}

// runTLCValidation generates a Trace.tla module and runs TLC if available.
func runTLCValidation(t *testing.T, rec *trace.Recorder, seed uint64) {
	t.Helper()

	tlaJar := findTLAToolsJar()
	requireTLC := tlcRequired()

	tmpDir := t.TempDir()

	content, err := rec.WriteTLA(filepath.Join(tmpDir, "Trace.tla"))
	require.NoError(t, err)
	t.Logf("Generated Trace.tla (%d bytes, %d states)", len(content), len(rec.States))

	specDir := findSpecDir(t)

	copyTLAFile(t, specDir, tmpDir, "HybridEvents.tla")
	copyTLAFile(t, specDir, tmpDir, "TraceHybridEvents.tla")
	copyTLAFile(t, specDir, tmpDir, "TraceHybridEvents.cfg")

	if tlaJar == "" {
		if requireTLC {
			t.Fatal("TLC is required but TLA_TOOLS_JAR is not set and tla2tools.jar was not found")
		}
		t.Log("TLA_TOOLS_JAR not set and tla2tools.jar not found; " +
			"skipping TLC validation (Go safety checks passed)")
		t.Logf("To run TLC manually:\n  cd %s\n  "+
			"java -jar tla2tools.jar -config TraceHybridEvents.cfg "+
			"-deadlock -workers auto TraceHybridEvents", tmpDir)
		return
	}

	t.Logf("Running TLC with seed %d...", seed)
	cmd := exec.Command(
		"java", "-jar", tlaJar,
		"-config", "TraceHybridEvents.cfg",
		"-deadlock",
		"-workers", "1",
		"TraceHybridEvents",
	)
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "Operation not permitted") &&
			!requireTLC {
			t.Log("TLC requires local socket permissions in this environment; " +
				"skipping TLC validation after Go-side safety checks")
			return
		}
		t.Fatalf("TLC failed for seed %d:\n%s", seed, string(output))
	}
	t.Logf("TLC validated trace (seed %d): %d states OK", seed, len(rec.States))
}

// findTLAToolsJar locates tla2tools.jar from env or standard locations.
func findTLAToolsJar() string {
	if jar := os.Getenv("TLA_TOOLS_JAR"); jar != "" {
		return jar
	}
	candidates := []string{
		"/opt/tla/tla2tools.jar",
		filepath.Join(os.Getenv("HOME"), "tla2tools.jar"),
	}
	for _, c := range candidates {
		clean := filepath.Clean(c)
		if _, err := os.Stat(clean); err == nil { //nolint:gosec
			return clean
		}
	}
	return ""
}

func tlcRequired() bool {
	return os.Getenv("CARTESI_REQUIRE_TLC") == "1"
}

// findSpecDir locates the spec/events/ directory by walking up from cwd.
func findSpecDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		candidate := filepath.Join(dir, "spec", "events")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find spec/events/ directory")
		}
		dir = parent
	}
}

// copyTLAFile copies a TLA+ file from srcDir to dstDir.
func copyTLAFile(t *testing.T, srcDir, dstDir, filename string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(srcDir, filename))
	require.NoError(t, err, "reading %s", filename)
	err = os.WriteFile(filepath.Join(dstDir, filename), data, filePerms)
	require.NoError(t, err, "writing %s", filename)
}
