// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

/*
 * Alternative service template that implements the tick-based processing.
 */

package service

import (
	"context"
	"fmt"
	"time"
)

type TickImpl interface {
	Tick(ctx context.Context) (bool, error)
}

type TickServiceTemplate struct {
	BaseTemplate
	tickImpl TickImpl
	interval time.Duration
}

type TickServiceConfigs struct {
	BaseConfigs
	PollInterval time.Duration
}

func InitTickServiceTemplate(
	s *TickServiceTemplate,
	cfg *TickServiceConfigs,
	tickImpl TickImpl,
) error {
	if s == nil || cfg == nil || tickImpl == nil {
		return ErrInvalid
	}

	InitServiceTemplate(&s.BaseTemplate, &cfg.BaseConfigs)

	s.tickImpl = tickImpl

	// ticker
	if cfg.PollInterval < 0 {
		return fmt.Errorf("PollInterval must be non-negative, got %v", cfg.PollInterval)
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = time.Minute
	}
	s.interval = cfg.PollInterval

	return nil
}

func (s *TickServiceTemplate) tick(ctx context.Context) {
	reschedule := true
	for reschedule && ctx.Err() == nil {
		var err error

		start := time.Now()
		reschedule, err = s.tickImpl.Tick(ctx)
		elapsed := time.Since(start)

		if err != nil {
			s.Logger.Error("Tick",
				"duration", elapsed,
				"reschedule", reschedule,
				"error", err,
			)
		} else {
			s.Logger.Debug("Tick",
				"duration", elapsed,
				"reschedule", reschedule,
			)
		}
	}
}

func (s *TickServiceTemplate) Serve(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.tick(ctx)
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			ticker.Stop()
		case <-ticker.C:
			s.tick(ctx)
		}
	}
	return ctx.Err()
}
