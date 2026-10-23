// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

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

type claimProvenanceRepository struct {
	repository.DeliveryRepository
	check func(context.Context, string) error
}

func (r *claimProvenanceRepository) CheckSubmitterProvenance(ctx context.Context, key string) error {
	if r.check != nil {
		return r.check(ctx, key)
	}
	return nil
}

type claimTickFixture struct {
	fn func(context.Context) (bool, error)
}

func (t *claimTickFixture) Tick(ctx context.Context) (bool, error) { return t.fn(ctx) }

func TestClaimStartupRetriesRegistrationFailuresBeforeRunningLoops(t *testing.T) {
	for _, failure := range []string{"missing observer", startupDatabaseUnavailableCase, startupCheckTimeoutCase} {
		t.Run(failure, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, backend, _ := newDeliveryFixture(t)
				s.publisher.interval = time.Hour
				original := s.publisher.sender
				var logs bytes.Buffer
				s.Logger = slog.New(slog.NewTextHandler(&logs, nil))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				attempts, failures := 0, 1
				if failure == startupDatabaseUnavailableCase {
					failures = 3
				}
				s.deliveryRepository = &claimProvenanceRepository{check: func(check context.Context, key string) error {
					require.Equal(t, ClaimerConfigKey, key)
					require.False(t, s.Ready(), "publication must remain unready throughout registration")
					require.Empty(t, backend.published)
					deadline, bounded := check.Deadline()
					require.True(t, bounded)
					require.LessOrEqual(t, time.Until(deadline), claimProvenanceCheckTimeout)
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
				}}
				var observedReady bool
				var attemptsAtStart int
				tick := &claimTickFixture{fn: func(context.Context) (bool, error) {
					observedReady, attemptsAtStart = s.Ready(), attempts
					cancel()
					return false, ctx.Err()
				}}
				require.NoError(t, service.InitTickServiceTemplate(&s.TickServiceTemplate, &service.TickServiceConfigs{
					BaseConfigs: service.BaseConfigs{Logger: s.Logger}, PollInterval: time.Hour,
				}, tick))
				require.False(t, s.Ready())
				before := time.Now()
				require.ErrorIs(t, s.Serve(ctx), context.Canceled)
				require.Equal(t, failures+1, attemptsAtStart, "both loops must wait for successful registration")
				require.True(t, observedReady, "successful registration must enable readiness")
				elapsed := time.Duration(failures) * time.Second
				if failure == startupCheckTimeoutCase {
					elapsed = claimProvenanceCheckTimeout
				}
				require.Equal(t, elapsed, time.Since(before), "retry must be paced without waiting for the normal hourly poll")
				require.Equal(t, 1, strings.Count(logs.String(), "Claim publication waits for compatible observer registration"))
				require.Contains(t, logs.String(), "level=WARN")
				require.Same(t, original, s.publisher.sender)
				require.False(t, s.Ready(), "shutdown must clear publication readiness")
				require.Empty(t, backend.published)
			})
		})
	}
}

func TestClaimStartupParentStopTakesPrecedenceOverRegistrationResult(t *testing.T) {
	for _, result := range []string{"joined cancellation", "successful read", "parent deadline"} {
		t.Run(result, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, backend, _ := newDeliveryFixture(t)
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
				s.deliveryRepository = &claimProvenanceRepository{check: func(check context.Context, _ string) error {
					<-check.Done()
					if result == "successful read" {
						return nil
					}
					return errors.Join(repository.ErrSubmitterProvenance, check.Err())
				}}
				err := s.Serve(ctx)
				require.Equal(t, ctx.Err(), err, "shutdown must return only the parent cause, without a provenance fault")
				require.False(t, s.Ready())
				require.Empty(t, backend.published)
			})
		})
	}
}

func TestClaimPublicationGuardUsesFailureAndCancellationOnly(t *testing.T) {
	s, _, _ := newDeliveryFixture(t)
	s.deliveryRepository = &claimProvenanceRepository{check: func(context.Context, string) error {
		t.Fatal("publication guard must not query database registration")
		return nil
	}}
	require.NoError(t, s.guardClaimPublication(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, s.guardClaimPublication(ctx), context.Canceled)
	cause := errors.New("publication stopped")
	s.failure.Store(&cause)
	require.ErrorIs(t, s.guardClaimPublication(t.Context()), cause)
}
