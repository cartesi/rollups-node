// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRecoveryDoesNotDependOnCurrentClaimReadiness(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_Open, model.EpochStatus_Closed, model.EpochStatus_InputsProcessed} {
		t.Run(status.String(), func(t *testing.T) {
			f := newRecoveryPortFixture(t)
			f.epoch.Status = status
			call, err := f.plan(t.Context())
			require.NoError(t, err)
			require.NotNil(t, call, "refunds remain useful while the current epoch is computing")
			f.repo.AssertNotCalled(t, "GetEpoch", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestFailedRootAllowsRecoveryWithoutPreparedClaim(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_Open, model.EpochStatus_Closed, model.EpochStatus_InputsProcessed} {
		f := newRecoveryPortFixture(t)
		f.epoch.Status = status
		call, err := f.plan(t.Context())
		require.NoError(t, err)
		require.NotNil(t, call)
		f.repo.AssertNotCalled(t, "GetEpoch", mock.Anything, mock.Anything, mock.Anything)
		f.factory.AssertNotCalled(t, "CreateDaveConsensusAdapter", mock.Anything)
	}
}

func TestRecoveryRunsDuringKnownSafeTournamentWait(t *testing.T) {
	for _, staged := range []bool{false, true} {
		f := newRecoveryPortFixture(t)
		if staged {
			f.epoch.Status = model.EpochStatus_ClaimStaged
		}
		view := resultTestSnapshot(f.epoch, staged)
		view.stage.IsFinished = false
		view.accept.IsClaimStagingPeriodOver = false
		view.accept.DoAllSentriesAgreeWithStagedTournamentResult = false
		f.snapshot(view)
		for _, kind := range []actionKind{stageTournament, acceptTournament} {
			call, err := f.d.planAction(t.Context(), f.app, kind, f.block, txsender.BlockRef{Number: 100})
			require.NoError(t, err)
			require.Nil(t, call)
		}
		call, err := f.plan(t.Context())
		require.NoError(t, err)
		require.NotNil(t, call)
	}
}

func TestRecoveryRunsWhenJoinIsAwaitingEventSync(t *testing.T) {
	f := newRecoveryPortFixture(t)
	f.epoch.ClaimTransactionHash = nil
	call, err := f.plan(t.Context())
	require.NoError(t, err)
	require.NotNil(t, call, "published root refund selection requires no local join record")
	f.repo.AssertNotCalled(t, "GetCommitment", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
