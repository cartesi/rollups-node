// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestProvisionalStagingClockRefreshKeepsMatchingPRTResult(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe} {
		for _, replacement := range []uint64{9, 11} {
			t.Run(policy.String()+new(big.Int).SetUint64(replacement).String(), func(t *testing.T) {
				f := newDeliveryFixture(t)
				f.d.defaultBlock = policy
				f.epoch.Status, f.epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(10))
				view := resultTestSnapshot(f.epoch, true)
				view.sealed.StagingBlockNumber = replacement
				f.repo.On("UpdateEpochStagingBlock", mock.Anything, f.app.ID, f.epoch.Index, uint64(10), replacement).
					Return(nil).Once()
				require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view))
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
				require.Equal(t, replacement, *f.epoch.StagedAtBlock)
				f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				f.repo.AssertExpectations(t)
			})
		}
	}
}

func TestInconsistentConsensusSnapshotDoesNotPublishObservation(t *testing.T) {
	f := newDeliveryFixture(t)
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{f.epoch}, uint64(1), nil).Once()
	view := resultTestSnapshot(f.epoch, false)
	view.stage.EpochNumber++
	f.consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(view.sealed, nil).Once()
	f.consensus.On("CanStageTournamentResult", mock.Anything).Return(view.stage, nil).Once()
	f.consensus.On("CanAcceptStagedTournamentResult", mock.Anything).Return(view.accept, nil).Once()
	expectProtectionReads(t, f.factory, f.consensus, observerFacts(100), f.epoch)
	err := f.d.observeApplication(t.Context(), f.app, observerFacts(100))
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.Contains(t, err.Error(), f.app.Name)
	require.Contains(t, err.Error(), "block 100")
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	f.repo.AssertNotCalled(t, "StoreTournamentInterval", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateEpochReconciledStaged", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
