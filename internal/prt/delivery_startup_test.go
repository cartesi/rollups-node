// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/stretchr/testify/require"
)

const (
	startupCheckTimeoutCase        = "check timeout"
	startupDatabaseUnavailableCase = "database unavailable"
)

type startupTickFixture struct {
	fn func(context.Context) (bool, error)
}

func (t *startupTickFixture) Tick(ctx context.Context) (bool, error) { return t.fn(ctx) }

func TestPRTStartupRetriesRegistrationFailuresBeforeRunningLoops(t *testing.T) {
	for _, failure := range []string{"missing observer", startupDatabaseUnavailableCase, startupCheckTimeoutCase} {
		t.Run(failure, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newDeliveryFixture(t)
				f.d.delivery, f.d.submissionEnabled, f.d.pollInterval = f.d, true, time.Hour
				original := f.d.sender
				var logs bytes.Buffer
				f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				attempts, failures := 0, 1
				if failure == startupDatabaseUnavailableCase {
					failures = 3
				}
				f.repo.onProvenance = func(check context.Context) error {
					require.False(t, f.d.Ready(), "publication must remain unready throughout registration")
					require.Empty(t, f.backend.published)
					deadline, bounded := check.Deadline()
					require.True(t, bounded)
					require.LessOrEqual(t, time.Until(deadline), provenanceCheckTimeout)
					attempts++
					if attempts > failures {
						return nil
					}
					cause := repository.ErrObserverNotRegistered
					switch failure {
					case startupDatabaseUnavailableCase:
						cause = errors.New("database connection reset")
					case startupCheckTimeoutCase:
						<-check.Done()
						require.ErrorIs(t, check.Err(), context.DeadlineExceeded)
						require.NoError(t, ctx.Err(), "the check deadline must not stop the service")
						cause = check.Err()
					}
					return errors.Join(repository.ErrSubmitterProvenance, cause)
				}
				// Keep publication blocked at its first fake RPC until the observer
				// checks readiness and cancels. No candidate setup is needed here.
				f.backend.onHeader = func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				}
				var observedReady bool
				var attemptsAtStart int
				tick := &startupTickFixture{fn: func(context.Context) (bool, error) {
					observedReady, attemptsAtStart = f.d.Ready(), attempts
					cancel()
					return false, ctx.Err()
				}}
				require.NoError(t, service.InitTickServiceTemplate(&f.d.TickServiceTemplate, &service.TickServiceConfigs{
					BaseConfigs: service.BaseConfigs{Logger: f.d.Logger}, PollInterval: time.Hour,
				}, tick))
				require.False(t, f.d.Ready())
				before := time.Now()
				require.ErrorIs(t, f.d.Serve(ctx), context.Canceled)
				require.Equal(t, failures+1, attemptsAtStart, "both loops must wait for successful registration")
				require.True(t, observedReady, "successful registration must enable readiness")
				elapsed := time.Duration(failures) * time.Second
				if failure == startupCheckTimeoutCase {
					elapsed = provenanceCheckTimeout
				}
				require.Equal(t, elapsed, time.Since(before), "retry must be paced without waiting for the normal hourly poll")
				require.Equal(t, 1, strings.Count(logs.String(), "PRT publication waits for compatible observer registration"))
				require.Contains(t, logs.String(), "level=WARN")
				require.Same(t, original, f.d.sender)
				require.False(t, f.d.Ready(), "shutdown must clear publication readiness")
				require.Empty(t, f.backend.published)
			})
		})
	}
}

func TestPRTStartupParentStopTakesPrecedenceOverRegistrationResult(t *testing.T) {
	for _, result := range []string{"joined cancellation", "successful read", "parent deadline"} {
		t.Run(result, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newDeliveryFixture(t)
				f.d.delivery, f.d.submissionEnabled = f.d, true
				ctx, cancel := context.WithCancel(t.Context())
				if result == "parent deadline" {
					cancel()
					ctx, cancel = context.WithTimeout(t.Context(), time.Second)
				} else {
					go func() {
						time.Sleep(time.Second)
						cancel()
					}()
				}
				defer cancel()
				f.repo.onProvenance = func(check context.Context) error {
					<-check.Done()
					if result == "successful read" {
						return nil
					}
					return errors.Join(repository.ErrSubmitterProvenance, check.Err())
				}
				err := f.d.Serve(ctx)
				require.Equal(t, ctx.Err(), err, "shutdown must return only the parent cause, without a provenance fault")
				require.False(t, f.d.Ready())
				require.Empty(t, f.backend.published)
			})
		})
	}
}
