// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

// The mock manager supplies the runtime; manager tests cover its retention.
func (s *AdvancerSuite) TestForeclosedClosedEpochPublishesWithoutNewInput() {
	env := s.setupOneApp()
	app := env.app.Application
	app.Status, app.ForecloseBlock, app.LastInputCheckBlock = model.ApplicationStatus_OK, 80, 80
	app.ProcessedInputs, env.app.processedInputs = 2, 2
	epoch := &model.Epoch{ApplicationID: app.ID, Index: 5, FirstBlock: 50, LastBlock: 59,
		InputIndexLowerBound: 1, InputIndexUpperBound: 2, Status: model.EpochStatus_Closed}
	env.repo.GetEpochsReturn = map[common.Address][]*model.Epoch{app.IApplicationAddress: {epoch}}
	// There are no new inputs. If manager preserves the runtime, a subsequent
	// Step can finish IP even after the first publication write was unavailable.
	failure := errors.New("publication write unavailable")
	env.repo.UpdateEpochsError = failure
	_, err := env.service.Step(context.Background())
	s.Require().ErrorIs(err, failure)
	s.Equal(1, env.repo.EpochInputsProcessedCount)
	env.repo.UpdateEpochsError = nil
	_, err = env.service.Step(context.Background())
	s.Require().NoError(err)
	s.Equal(2, env.repo.EpochInputsProcessedCount)
	s.Equal(env.app.StateProofReturn, env.repo.PublishedStateProof)
	s.Empty(env.repo.StoredResults, "do not replay or advance another input to publish CLOSED")
	s.Zero(env.repo.ApplicationStatusUpdates)
}
