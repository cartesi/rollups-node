// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package poller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Service interface {
	Step(context.Context) error
}

type Poller struct {
	name    string
	service Service
	ticker  *time.Ticker
}

var ErrInvalidPollingInterval = errors.New("polling interval must be greater than zero")

func New(name string, service Service, pollingInterval time.Duration) (*Poller, error) {
	if pollingInterval <= 0 {
		return nil, ErrInvalidPollingInterval
	}
	ticker := time.NewTicker(pollingInterval)
	return &Poller{name: name, service: service, ticker: ticker}, nil
}

func (poller *Poller) Start(ctx context.Context) error {
	slog.Debug(fmt.Sprintf("%s: poller started", poller.name))

	for {
		// Runs the service's inner routine.
		err := poller.service.Step(ctx)
		if err != nil {
			return err
		}

		// Waits for the polling interval to elapse (or for the context to be canceled).
		select {
		case <-poller.ticker.C:
			continue
		case <-ctx.Done():
			poller.ticker.Stop()
			return nil
		}
	}
}
