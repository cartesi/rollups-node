// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/cartesi/rollups-node/internal/advancer"
	"github.com/cartesi/rollups-node/internal/advancer/machines"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
)

type AdvancerService struct {
	database                *repository.Database
	AdvancerPollingInterval time.Duration
	MachineServerVerbosity  cartesimachine.ServerVerbosity
}

func NewAdvancerService(
	database *repository.Database,
	pollingInterval time.Duration,
	machineServerVerbosity cartesimachine.ServerVerbosity,
) *AdvancerService {
	return &AdvancerService{
		database:                database,
		AdvancerPollingInterval: pollingInterval,
		MachineServerVerbosity:  machineServerVerbosity,
	}
}

func (s *AdvancerService) Start(
	ctx context.Context,
	ready chan<- struct{},
) error {

	repo := &repository.MachineRepository{Database: s.database}

	machines, err := machines.Load(ctx, repo, s.MachineServerVerbosity)
	if err != nil {
		return fmt.Errorf("failed to load the machines: %w", err)
	}
	defer machines.Close()

	advancer, err := advancer.New(machines, repo)
	if err != nil {
		return fmt.Errorf("failed to create the advancer: %w", err)
	}

	poller, err := advancer.Poller(s.AdvancerPollingInterval)
	if err != nil {
		return fmt.Errorf("failed to create the advancer service: %w", err)
	}

	ready <- struct{}{}
	return poller.Start(ctx)
}

func (s *AdvancerService) String() string {
	return "advancer"
}
