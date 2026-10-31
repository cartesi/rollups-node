// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"errors"
	"fmt"
	"time"

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/cartesi/rollups-node/internal/repository"
)

const maxClaimRetryDelay = 5 * time.Minute

type claimRetryKey struct {
	applicationID int64
	epochIndex    uint64
}

type claimRetry struct {
	attempts    uint64
	delay       time.Duration
	nextAttempt time.Time
}

// claimStatusRecorded marks a failure that wrote an application status.
// That status stops the epoch, so the failure starts no cooldown.
type claimStatusRecorded struct{ error }

func (e *claimStatusRecorded) Unwrap() error { return e.error }

func (s *Service) claimNow() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Service) claimWaiting(key claimRetryKey) bool {
	retry, exists := s.claimRetries[key]
	return exists && s.claimNow().Before(retry.nextAttempt)
}

// failedClaim starts or doubles the epoch's cooldown after an operational
// failure and names the next attempt in the returned error.
func (s *Service) failedClaim(key claimRetryKey, address string, cause error) error {
	if !claimFailureNeedsBackoff(cause) {
		return cause
	}
	retry := s.claimRetries[key]
	retry.attempts++
	if retry.delay == 0 {
		retry.delay = s.claimRetryBase
	} else {
		retry.delay = min(retry.delay+retry.delay, maxClaimRetryDelay) // Double it.
	}
	// Read the clock after the failure, including a slow transaction.
	retry.nextAttempt = s.claimNow().Add(retry.delay)
	if s.claimRetries == nil {
		s.claimRetries = make(map[claimRetryKey]claimRetry)
	}
	s.claimRetries[key] = retry
	return fmt.Errorf("application %s epoch %d: claim attempt %d failed, next attempt at %s: %w",
		address, key.epochIndex, retry.attempts, retry.nextAttempt.Format(time.RFC3339), cause)
}

// Recorded status and foreclosure are definitive outcomes from their producers.
// A cancellation branch must not hide an independent operational failure.
func claimFailureNeedsBackoff(err error) bool {
	var recorded *claimStatusRecorded
	return err != nil && !errors.As(err, &recorded) && !errors.Is(err, repository.ErrEpochForeclosed) &&
		!errutil.IsOnlyCancellation(err)
}
