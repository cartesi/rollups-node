// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestVerifyClaimOutputsMismatch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	claim := makeClaimStatus(claimStatusStaged, currEpoch, stagedAt)
	claim.StagedOutputsMerkleRoot = common.HexToHash("0xdeadbeef")
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(claim, nil).Once()
	// No acceptClaimOnBlockchain call — mismatch trips before broadcast.
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	_, errs := m.acceptStagedClaimsAndIssueAcceptTx(makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs), "chain_claim_outputs_mismatch must surface as an error")
	assert.Equal(t, 0, len(m.acceptsInFlight))
}

func TestRejectEpochAndSetApplicationDiverged_MirrorsAppliedWrites(t *testing.T) {
	t.Run("preserves terminal application mirror", func(t *testing.T) {
		service, repo, _ := newServiceMock(t)
		defer repo.AssertExpectations(t)

		app := makeApplication()
		app.Status = model.ApplicationStatus_MachineHalted
		originalReason := "machine halted"
		app.Reason = &originalReason
		epoch := makeComputedEpoch(app, 3)

		repo.On("RejectEpochAndSetApplicationDiverged",
			mock.Anything, app.ID, epoch.Index, mock.Anything).
			Return(repository.RejectEpochAndDivergeResult{EpochRejected: true}, nil).Once()

		err := service.rejectEpochAndSetApplicationDiverged(app, epoch, "later claim disagreement")

		require.Error(t, err)
		assert.Equal(t, model.ApplicationStatus_MachineHalted, app.Status)
		require.NotNil(t, app.Reason)
		assert.Equal(t, originalReason, *app.Reason)
		assert.Equal(t, model.EpochStatus_ClaimRejected, epoch.Status)
	})

	t.Run("preserves epoch mirror when rejection did not apply", func(t *testing.T) {
		service, repo, _ := newServiceMock(t)
		defer repo.AssertExpectations(t)

		app := makeApplication()
		epoch := makeComputedEpoch(app, 3)
		epoch.Status = model.EpochStatus_Closed
		reason := "claim disagreement"

		repo.On("RejectEpochAndSetApplicationDiverged",
			mock.Anything, app.ID, epoch.Index, reason).
			Return(repository.RejectEpochAndDivergeResult{ApplicationDiverged: true}, nil).Once()

		err := service.rejectEpochAndSetApplicationDiverged(app, epoch, reason)

		require.Error(t, err)
		assert.Equal(t, model.ApplicationStatus_Diverged, app.Status)
		require.NotNil(t, app.Reason)
		assert.Equal(t, reason, *app.Reason)
		assert.Equal(t, model.EpochStatus_Closed, epoch.Status)
	})
}

// TestCleanupOrphanedInFlight — entries whose app is no longer in any work
// map (e.g. transitioned to FAILED/DIVERGED/CORRUPTED/DISABLED mid-flight) must be
