// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"
)

const claimProvenanceCheckTimeout = 5 * time.Second

// Serve runs observation and publication independently. Temporary publication
// failures retain the sender and retry in the publisher's paced loop.
func (s *Service) Serve(ctx context.Context) error {
	if !s.submissionEnabled {
		return s.TickServiceTemplate.Serve(ctx)
	}
	if err := s.waitForClaimObserver(ctx); err != nil {
		return err
	}
	s.publicationReady.Store(true)
	defer s.publicationReady.Store(false)
	group, runCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return s.TickServiceTemplate.Serve(runCtx) })
	group.Go(func() error { return s.publisher.run(runCtx) })
	return group.Wait()
}

// The contributing observer can register after this independent service starts.
// This prerequisite is checked once, without coupling each POST to the database.
func (s *Service) waitForClaimObserver(ctx context.Context) error {
	interval := s.publisher.interval
	if interval <= 0 || interval > time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var warnedAt time.Time
	for ctx.Err() == nil {
		check, cancel := context.WithTimeout(ctx, claimProvenanceCheckTimeout)
		err := s.deliveryRepository.CheckSubmitterProvenance(check, ClaimerConfigKey)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			return nil
		}
		if time.Since(warnedAt) >= claimWarningInterval && ctx.Err() == nil {
			s.Logger.Warn("Claim publication waits for compatible observer registration", "error", err)
			warnedAt = time.Now()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return ctx.Err()
}

func (s *Service) guardClaimPublication(ctx context.Context) error {
	if cause := s.failure.Load(); cause != nil {
		return *cause
	}
	return ctx.Err()
}
