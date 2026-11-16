// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIORScanMultiBlockAdvanceDetectsInteriorEvent(t *testing.T) {
	for _, phase := range []int{claimSubmitPage, claimStagePage, claimAcceptPage} {
		t.Run(iorScanPhaseName(phase), func(t *testing.T) {
			s, repo, chain, app, epoch, _ := newIORScanFixture(t, phase)
			app.ConsensusType = model.Consensus_Quorum
			require.NoError(t, runIORScan(t.Context(), s, phase, app, epoch, chain.head).err)
			through := chain.head
			chain.head += 32
			if phase == claimStagePage {
				foreign := makeStagedEvent(app, epoch)
				foreign.MachineMerkleRoot[0] ^= 1
				foreign.Raw.BlockNumber = through + 2
				chain.staged = foreign
			} else {
				foreign := makeAcceptedEvent(app, epoch)
				foreign.MachineMerkleRoot[0] ^= 1
				foreign.Raw.BlockNumber = through + 2
				chain.accepted = foreign
			}
			repo.On("RejectEpochAndSetApplicationDiverged", mock.Anything, app.ID, epoch.Index, mock.Anything).
				Return(repository.RejectEpochAndDivergeResult{EpochRejected: true, ApplicationDiverged: true}, nil).Maybe()
			repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
				Return(nil).Maybe()
			runIORScan(t.Context(), s, phase, app, epoch, chain.head)
			require.Equal(t, model.ApplicationStatus_Diverged, app.Status,
				"an event inside a multi-block FINALIZED advance must not be skipped")
		})
	}
}

func TestIORScanFailureDoesNotForecloseFromLocalAbsence(t *testing.T) {
	for _, phase := range []int{claimSubmitPage, claimStagePage} {
		t.Run(iorScanPhaseName(phase), func(t *testing.T) {
			s, repo, chain, app, epoch, _ := newIORScanFixture(t, phase)
			app.ForecloseBlock = iorScanTestStage
			stream := iorAcceptedStream
			if phase == claimStagePage {
				stream = iorStagedStream
			}
			scanErr := errors.New("history incomplete")
			chain.scanErrors[stream] = scanErr
			repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, epoch.Index).Return(nil).Maybe()
			result := runIORScan(t.Context(), s, phase, app, epoch, chain.head)
			require.ErrorIs(t, result.err, scanErr)
			require.Zero(t, result.progress)
			require.Empty(t, repo.Calls, "an incomplete stream cannot terminalize the epoch")
		})
	}
}

func TestIORScanIdentityChangeRescansBeforeReuse(t *testing.T) {
	s, repo, chain, app, epoch, _ := newIORScanFixture(t, claimSubmitPage)
	app.ConsensusType = model.Consensus_Quorum
	require.NoError(t, runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head).err)
	// This synthetic identity change checks the cache consumer contract.
	// Production writers do not change the bounds of a computed epoch.
	epoch.LastBlock--
	foreign := makeAcceptedEvent(app, epoch)
	foreign.MachineMerkleRoot[0] ^= 1
	foreign.Raw.BlockNumber = epoch.LastBlock + 1
	chain.accepted = foreign
	repo.On("RejectEpochAndSetApplicationDiverged", mock.Anything, app.ID, epoch.Index, mock.Anything).
		Return(repository.RejectEpochAndDivergeResult{EpochRejected: true, ApplicationDiverged: true}, nil).Maybe()
	runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head)
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status, "changed claim identity must not reuse the previous key's prefix")
}
