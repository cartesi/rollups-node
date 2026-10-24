// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	computedVisibilityPath  = "computed"
	submittedVisibilityPath = "submitted"
	stagedVisibilityPath    = "staged"
)

func TestForeclosureWaitsForObservationAcrossClaimPaths(t *testing.T) {
	for _, path := range []string{computedVisibilityPath, submittedVisibilityPath, stagedVisibilityPath} {
		t.Run(path, func(t *testing.T) {
			s, r, b := newServiceMock(t)
			app := withForeclosed(makeApplication(), 105)
			epoch := makeComputedEpoch(app, 3)
			b.On("getConsensusAddress", mock.Anything, app, mock.Anything).Return(app.IConsensusAddress, nil).Maybe()
			var run func(uint64) claimStepResult
			switch path {
			case computedVisibilityPath:
				run = func(head uint64) claimStepResult {
					expectNoForeignClaimAccepted(b, app, epoch, epoch.LastBlock+1, head)
					b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, head).
						Return(&iconsensus.IConsensus{}, nil, nil, nil).Once()
					expectVisibilityClaimStatus(b, app, epoch, head)
					return s.processComputedClaim(t.Context(), claimWork{app: app, epoch: epoch}, new(big.Int).SetUint64(head))
				}
			case submittedVisibilityPath:
				epoch.Status = model.EpochStatus_ClaimSubmitted
				run = func(head uint64) claimStepResult {
					var staged *iconsensus.IConsensusClaimStaged
					if head >= 103 {
						staged = makeStagedEvent(app, epoch)
						staged.Raw.BlockNumber = 101
					}
					b.On("findClaimStagedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, head).
						Return(&iconsensus.IConsensus{}, staged, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
					return s.processSubmittedClaim(t.Context(), claimWork{app: app, epoch: epoch}, new(big.Int).SetUint64(head))
				}
			default:
				epoch = makeStagedEpoch(app, 3, 50)
				run = func(head uint64) claimStepResult {
					expectVisibilityClaimStatus(b, app, epoch, head)
					return s.processStagedClaim(t.Context(), claimWork{app: app, epoch: epoch}, new(big.Int).SetUint64(head))
				}
			}

			lagged := run(100)
			require.ErrorIs(t, lagged.err, txsender.ErrInconclusive)
			require.Zero(t, lagged.progress)
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, mock.Anything, mock.Anything)
			r.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

			if path == submittedVisibilityPath {
				r.On("UpdateEpochToStaged", mock.Anything, app.ID, epoch.Index, uint64(101)).Return(nil).Once()
			} else {
				r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, mock.Anything).Return(nil).Once()
			}
			caughtUp := run(106)
			require.NoError(t, caughtUp.err)
			require.Equal(t, 1, caughtUp.progress)
			if path == submittedVisibilityPath {
				// The next phase reloads the staged row and observes acceptance.
				epoch = makeStagedEpoch(app, epoch.Index, 101)
				expectVisibilityClaimStatus(b, app, epoch, 106)
				r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, mock.Anything).Return(nil).Once()
				result := s.processStagedClaim(t.Context(), claimWork{app: app, epoch: epoch}, big.NewInt(106))
				require.NoError(t, result.err)
				require.Equal(t, 1, result.progress)
			}
			r.AssertExpectations(t)
			b.AssertExpectations(t)
		})
	}
}

func expectVisibilityClaimStatus(b *claimerBlockchainMock, app *model.Application, epoch *model.Epoch, head uint64) {
	status := claimStatusUnstaged
	stagedAt := uint64(0)
	if epoch.StagedAtBlock != nil && head >= *epoch.StagedAtBlock {
		status, stagedAt = claimStatusStaged, *epoch.StagedAtBlock
	}
	if head >= 103 {
		status = claimStatusAccepted
	}
	b.On("getClaimStatus", mock.Anything, app, epoch, new(big.Int).SetUint64(head)).
		Return(makeClaimStatus(status, epoch, stagedAt), nil).Once()
}

func TestFutureForeclosureDoesNotBypassStagingVisibility(t *testing.T) {
	s, r, b := newServiceMock(t)
	app := withForeclosed(makeApplication(), 107)
	epoch := makeStagedEpoch(app, 3, 105)
	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).Return(app.IConsensusAddress, nil).Once()
	result := s.processStagedClaim(t.Context(), claimWork{app: app, epoch: epoch}, big.NewInt(100))
	require.ErrorIs(t, result.err, txsender.ErrInconclusive)
	require.Empty(t, r.Calls)
	b.AssertNotCalled(t, "getClaimStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestBulkForeclosureWaitsForClaimerView(t *testing.T) {
	s, r, _ := newServiceMock(t)
	app := foreclosedAppHelper(1, 105, model.Consensus_Authority)
	require.True(t, app.ForeclosureScanCaughtUp(), "reader catch-up is independent of Claimer view")
	require.NoError(t, s.processForeclosedApps(t.Context(), map[int64]*model.Application{app.ID: app}, 100))
	require.Empty(t, r.Calls)
}

func TestForeclosureAtObservationBoundaryStillTerminalizes(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_OK, model.ApplicationStatus_Failed,
		model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted} {
		for _, path := range []string{computedVisibilityPath, submittedVisibilityPath, stagedVisibilityPath} {
			t.Run(status.String()+"/"+path, func(t *testing.T) {
				s, r, b := newServiceMock(t)
				app := withForeclosed(makeApplication(), 105)
				app.Status = status
				epoch := makeComputedEpoch(app, 3)
				head := new(big.Int).SetUint64(app.ForecloseBlock)
				b.On("getConsensusAddress", mock.Anything, app, mock.Anything).Return(app.IConsensusAddress, nil).Maybe()
				r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, epoch.Index).Return(nil).Once()
				var result claimStepResult
				switch path {
				case computedVisibilityPath:
					expectNoForeignClaimAccepted(b, app, epoch, epoch.LastBlock+1, head.Uint64())
					b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, head.Uint64()).
						Return(&iconsensus.IConsensus{}, nil, nil, nil).Once()
					expectGetClaimStatusUnstaged(b, app, epoch, head)
					result = s.processComputedClaim(t.Context(), claimWork{app: app, epoch: epoch}, head)
				case submittedVisibilityPath:
					epoch.Status = model.EpochStatus_ClaimSubmitted
					b.On("findClaimStagedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, head.Uint64()).
						Return(&iconsensus.IConsensus{}, (*iconsensus.IConsensusClaimStaged)(nil),
							(*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
					result = s.processSubmittedClaim(t.Context(), claimWork{app: app, epoch: epoch}, head)
				default:
					epoch = makeStagedEpoch(app, 3, 50)
					b.On("getClaimStatus", mock.Anything, app, epoch, head).
						Return(makeClaimStatus(claimStatusStaged, epoch, 50), nil).Once()
					result = s.processStagedClaim(t.Context(), claimWork{app: app, epoch: epoch}, head)
				}
				require.NoError(t, result.err)
				require.Equal(t, 1, result.progress)
				require.Equal(t, model.EpochStatus_ClaimForeclosed, epoch.Status)
				require.Equal(t, status, app.Status)
				r.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				r.AssertExpectations(t)
				b.AssertExpectations(t)
			})
		}
	}
}

func TestPreAcceptUnstagedContradictionRequiresVisibleStagingBlock(t *testing.T) {
	for _, head := range []uint64{49, 50} {
		t.Run(new(big.Int).SetUint64(head).String(), func(t *testing.T) {
			s, r, b := newServiceMock(t)
			app := makeApplication()
			app.ClaimStagingPeriod = 0
			epoch := makeStagedEpoch(app, 3, 50)
			b.On("getConsensusAddress", mock.Anything, app, mock.Anything).Return(app.IConsensusAddress, nil).Once()
			if head == 50 {
				expectGetClaimStatusUnstaged(b, app, epoch, new(big.Int).SetUint64(head))
				r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
					Return(nil).Once()
			}
			result := s.processStagedClaim(t.Context(), claimWork{app: app, epoch: epoch}, new(big.Int).SetUint64(head))
			require.Zero(t, result.progress)
			require.Nil(t, s.publisher, "observation owns no publication queue")
			require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
			if head < 50 {
				require.ErrorIs(t, result.err, txsender.ErrInconclusive)
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				require.Empty(t, r.Calls)
				b.AssertNotCalled(t, "getClaimStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			} else {
				require.NoError(t, result.err)
				require.Equal(t, model.ApplicationStatus_Failed, app.Status)
			}
			r.AssertExpectations(t)
			b.AssertExpectations(t)
		})
	}
}

func TestForeclosureObservationWaitRetainsHeldTransaction(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, epoch := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	// A lost volatile receipt hint must not release the sender's signed bytes.
	app.ForecloseBlock = 105 // Another service advances the saved application.
	chain := s.blockchain.(*claimerBlockchainMock)
	chain.On("getConsensusAddress", mock.Anything, app, mock.Anything).Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(chain, app, epoch, epoch.LastBlock+1, 100)
	chain.On("findClaimSubmittedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(100)).
		Return(&iconsensus.IConsensus{}, nil, nil, nil).Once()
	expectVisibilityClaimStatus(chain, app, epoch, 100)
	_, err := s.submitClaimsAndUpdateDatabase(t.Context(), makeEpochMap(epoch),
		makeApplicationMap(app), big.NewInt(100))

	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrInconclusive)
	require.Len(t, backend.published, 1)
	chain.AssertExpectations(t)
}
