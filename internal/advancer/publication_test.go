// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func (s *AdvancerSuite) TestFinalizeEpochPublicationOutcome() {
	for _, test := range []struct {
		name     string
		storeErr error
		obsolete bool
	}{
		{name: "published"},
		{name: "foreclosed during publication", storeErr: fmt.Errorf("state changed: %w", repository.ErrEpochForeclosed), obsolete: true},
		{name: "unexpected state", storeErr: repository.ErrNoUpdate},
		{name: "database failure", storeErr: errors.New("database write failed")},
	} {
		s.Run(test.name, func() {
			env := s.setupOneApp()
			logs := &advancerLogCapture{}
			env.service.Logger = slog.New(logs)
			env.repo.UpdateEpochsError = test.storeErr
			epoch := &Epoch{ApplicationID: env.app.Application.ID, Index: 2, Status: EpochStatus_Closed}

			err := env.service.finalizeEpoch(context.Background(), env.app.Application, epoch)

			if test.obsolete || test.storeErr == nil {
				s.Require().NoError(err)
			} else {
				s.Require().ErrorIs(err, test.storeErr)
			}
			s.Equal(1, env.repo.EpochInputsProcessedCount, "make one publication attempt; do not retry a rejected write")
			s.Equal(test.storeErr == nil, logs.contains(slog.LevelInfo, "Epoch updated to Inputs Processed"))
			s.Equal(test.obsolete, logs.contains(slog.LevelInfo,
				"Epoch was foreclosed before state-proof publication; discarding obsolete publication"))
			s.Zero(env.repo.ApplicationStatusUpdates)
			s.Equal(EpochStatus_Closed, epoch.Status, "the stale input object must not be used to reset stored state")
		})
	}
}
