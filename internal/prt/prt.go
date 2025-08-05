// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
)

var (
	ErrInvalidMachines   = errors.New("machines must not be nil")
	ErrInvalidRepository = errors.New("repository must not be nil")
)

type PRTRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool) ([]*model.Application, uint64, error)
}

type Service struct {
	service.Service
	config         config.PrtConfig
	repository     PRTRepository
	machineManager manager.MachineProvider
}

type CreateInfo struct {
	service.CreateInfo
	Config     config.PrtConfig
	Repository repository.Repository
}

func Create(ctx context.Context, c *CreateInfo) (*Service, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}
	c.Impl = s

	err = service.Create(ctx, &c.CreateInfo, &s.Service)
	if err != nil {
		return nil, err
	}

	s.config = c.Config
	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on PRT service Create is nil")
	}

	// Create the machine manager
	manager := manager.NewMachineManager(
		ctx,
		c.Repository,
		c.Config.RemoteMachineLogLevel,
		s.Logger,
		c.Config.FeatureMachineHashCheckEnabled,
	)
	s.machineManager = manager

	return s, nil
}

// Service interface implementation
func (s *Service) Alive() bool     { return true }
func (s *Service) Ready() bool     { return true }
func (s *Service) Reload() []error { return nil }
func (s *Service) Stop(b bool) []error {
	return nil
}
func (s *Service) Serve() error {
	return s.Service.Serve()
}
func (s *Service) String() string {
	return s.Name
}
func (s *Service) Tick() []error {
	s.Logger.Info("PRT service tick...")
	apps, _, err := s.repository.ListApplications(context.Background(),
		repository.ApplicationFilter{
			State:         model.Pointer(model.ApplicationState_Enabled),
			ConsensusType: model.Pointer(model.ConsensusType_Prt),
		}, repository.Pagination{}, false)
	if err != nil {
		s.Logger.Error("Failed to list applications", "error", err)
		return []error{err}
	}
	for _, app := range apps {
		s.Logger.Info("Application found", "name", app.Name, "consensus type:", app.ConsensusType)
	}
	return []error{}
}
