// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"time"
)

const provenanceCheckTimeout = 5 * time.Second

// The contributing observer can register after this independent service starts.
// Check once before publication; do not couple each POST to registration reads.
func (s *Service) waitForObserver(ctx context.Context) error {
	interval := s.pollInterval
	if interval <= 0 || interval > time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var warnedAt time.Time
	for ctx.Err() == nil {
		check, cancel := context.WithTimeout(ctx, provenanceCheckTimeout)
		err := s.repository.CheckSubmitterProvenance(check, PrtConfigKey)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			return nil
		}
		if warningDue(warnedAt) && ctx.Err() == nil {
			s.Logger.Warn("PRT publication waits for compatible observer registration", "error", err)
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
