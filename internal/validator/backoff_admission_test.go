// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A successful admission reload may finish after an existing cooldown expires.
func TestClaimBackoffCooldownExpiresDuringAdmissionRead(t *testing.T) {
	s, repo, now, _ := newClaimBackoffFixture(t, time.Minute)
	key := claimRetryKey{73, 2}
	cause := errors.New("storage unavailable")
	repo.store = func(context.Context, claimRetryKey) error { return cause }
	_ = backoffTick(t, s, cause)
	require.Len(t, repo.stores, 2)
	*now = now.Add(30 * time.Second)
	repo.read = func(k claimRetryKey) error {
		if k == key {
			*now = now.Add(31 * time.Second)
		}
		return nil
	}
	_ = backoffTick(t, s, cause)
	var attempts int
	for _, k := range repo.stores {
		if k == key {
			attempts++
		}
	}
	require.Equal(t, 2, attempts, "the admission read finished after the cooldown expired")
}

// A returned cancellation can accompany an independent physical write failure.
func TestClaimBackoffMixedFailureHasCooldownWhileServiceLives(t *testing.T) {
	s, repo, _, _ := newClaimBackoffFixture(t, time.Minute)
	cause := errors.New("independent storage failure")
	repo.store = func(context.Context, claimRetryKey) error { return errors.Join(context.Canceled, cause) }
	_ = backoffTick(t, s, cause)
	require.Len(t, repo.stores, 2)
	_ = backoffTick(t, s, nil)
	require.Len(t, repo.stores, 2, "cooling physical failures must not repeat on every poll")
}
