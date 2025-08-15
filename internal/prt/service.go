// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
)

type Service struct {
	service.Service
	repository PrtRepository

	// cached constants
	pristineRootHash    common.Hash
	pristinePostContext []common.Hash
}

type CreateInfo struct {
	service.CreateInfo

	Config config.ValidatorConfig

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

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on validator service Create is nil")
	}

	s.pristinePostContext = merkle.CreatePostContext()
	s.pristineRootHash = s.pristinePostContext[merkle.TREE_DEPTH]

	return s, nil
}

func (s *Service) Alive() bool     { return true }
func (s *Service) Ready() bool     { return true }
func (s *Service) Reload() []error { return nil }

// Tick executes the Validator main logic of producing claims and/or proofs
// for processed epochs of all running applications.
func (s *Service) Tick() []error {
	apps, _, err := getAllRunningApplications(s.Context, s.repository)
	if err != nil {
		return []error{fmt.Errorf("failed to get running applications. %w", err)}
	}

	// validate each application
	errs := []error{}
	for idx := range apps {
		if err := s.validateApplication(s.Context, apps[idx]); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
func (s *Service) Stop(b bool) []error {
	return nil
}

func (v *Service) String() string {
	return v.Name
}
