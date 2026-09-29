// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

func (s *AdvancerSuite) TestInvalidOutputsRootStopsClaimPreparation() {
	env := s.setupOneApp()
	terminal := randomAdvanceResult(0)
	terminal.Status = model.InputCompletionStatus_InvalidOutputsRoot
	terminal.Outputs = nil
	terminal.Reports = nil
	address := env.app.Application.IApplicationAddress
	env.repo.GetEpochsReturn = map[common.Address][]*model.Epoch{
		address: {{
			Index: 0, Status: model.EpochStatus_Closed,
			InputIndexLowerBound: 0, InputIndexUpperBound: 2,
		}},
	}
	env.repo.GetInputsReturn = map[common.Address][]*model.Input{
		address: {
			newInput(env.app.Application.ID, 0, 0, marshal(terminal)),
			newInput(env.app.Application.ID, 0, 1, []byte("must not execute")),
		},
	}

	hadWork, err := env.service.Step(context.Background())
	s.Require().NoError(err)
	s.False(hadWork)
	s.Require().Len(env.repo.StoredResults, 1)
	s.Equal(model.InputCompletionStatus_InvalidOutputsRoot, env.repo.StoredResults[0].Status)
	s.Zero(env.repo.EpochInputsProcessedCount, "the epoch must not become claim work")
	s.Zero(env.repo.ApplicationStatusUpdates, "the status belongs to the atomic input-result write")

	env.app.Application.Status = model.ApplicationStatus_InvalidOutputsRoot
	hadWork, err = env.service.Step(context.Background())
	s.Require().NoError(err)
	s.False(hadWork)
	s.Len(env.repo.StoredResults, 1, "later ticks must not execute the next input")
}

func (s *AdvancerSuite) TestInvalidOutputsRootStoreFailureStopsService() {
	env := s.setupOneApp()
	terminal := randomAdvanceResult(0)
	terminal.Status = model.InputCompletionStatus_InvalidOutputsRoot
	terminal.Outputs = nil
	terminal.Reports = nil
	pending := newInput(env.app.Application.ID, 0, 0, marshal(terminal))
	env.repo.StoreAdvanceError = errors.New("terminal result write failed")

	_, _, err := env.service.processInputs(context.Background(), env.app.Application, []*model.Input{
		pending,
		newInput(env.app.Application.ID, 0, 1, []byte("must not execute")),
	})
	s.Require().ErrorIs(err, env.repo.StoreAdvanceError)
	s.Require().NotNil(env.supervisor.FatalError.Load())
	s.ErrorIs(*env.supervisor.FatalError.Load(), env.repo.StoreAdvanceError)
	s.True(env.supervisor.StopCalled.Load())
	s.Empty(env.repo.StoredResults)
	s.Zero(env.repo.ApplicationStatusUpdates, "do not record a terminal status without its input evidence")
}
