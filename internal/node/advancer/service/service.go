// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

type Service interface {
	Run(context.Context) error
}

type Poller struct {
	name       string
	service    Service
	shouldStop atomic.Bool
	ticker     *time.Ticker
}

var ErrInvalidPollingInterval = errors.New("polling interval must be greater than zero")

func NewPoller(name string, service Service, pollingInterval time.Duration) (*Poller, error) {
	if pollingInterval <= 0 {
		return nil, ErrInvalidPollingInterval
	}
	ticker := time.NewTicker(pollingInterval)
	return &Poller{name: name, service: service, ticker: ticker}, nil
}

func (poller *Poller) Start(ctx context.Context, ready chan<- struct{}) error {
	ready <- struct{}{}

	slog.Info(fmt.Sprintf("%s: started", poller.name))

	for {
		// Runs the service's inner routine.
		err := poller.service.Run(ctx)
		if err != nil {
			return err
		}

		// Checks if the service was ordered to stop.
		if poller.shouldStop.Load() {
			poller.shouldStop.Store(false)
			slog.Info(fmt.Sprintf("%s: stopped", poller.name))
			return nil
		}

		// Waits for the polling interval to elapse.
		slog.Info(fmt.Sprintf("%s: waiting for the polling interval to elapse", poller.name))
		<-poller.ticker.C
	}
}

// Stop orders the service to stop, which will happen before the next poll.
func (poller *Poller) Stop() {
	poller.shouldStop.Store(true)
}
