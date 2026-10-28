// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFinalizedStagingContradictionStopsEarlierClaimWork(t *testing.T) {
	for _, stop := range []model.ApplicationStatus{model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
		t.Run(stop.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := queueDelivery(t, backend, lookup, 1)
			epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
			setClaimTestStop(lookup, app, epoch.Index+1, stop)
			backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 30)
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 1)
			action := s.publisher.lastChoice

			// Publication reads a separate committed object. Mutating only the
			// observer's copy would leave the previously authorized work useful.
			observed := *app
			repo := s.repository.(*claimerRepositoryMock)
			repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted, mock.Anything).
				Run(func(args mock.Arguments) {
					app.Status = args.Get(2).(model.ApplicationStatus)
					reason := *args.Get(3).(*string)
					app.Reason = &reason
				}).Return(nil).Once()
			chain := s.blockchain.(*claimerBlockchainMock)
			chain.On("getConsensusAddress", mock.Anything, &observed, big.NewInt(50)).
				Return(app.IConsensusAddress, nil).Once()
			expectGetClaimStatusUnstaged(chain, &observed, epoch, big.NewInt(50))

			result := s.processStagedClaim(t.Context(), claimWork{app: &observed, epoch: epoch}, big.NewInt(50))
			require.Equal(t, model.ApplicationStatus_Corrupted, app.Status)
			require.Equal(t, model.ApplicationStatus_Corrupted, observed.Status)
			require.ErrorContains(t, result.err, "UNSTAGED")
			require.Zero(t, result.progress)
			require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)

			block := txsender.BlockRef{Number: backend.last, Hash: deliveryHeader(backend.last).Hash()}
			for _, held := range []bool{false, true} {
				_, eligibility, err := s.publisher.decide(t.Context(), block, repository.ClaimWork{Application: app, Epoch: epoch}, held)
				require.NoError(t, err)
				require.Equal(t, txsender.Obsolete, eligibility)
			}
			eligibility, err := s.publisher.Recheck(t.Context(), block, action)
			require.NoError(t, err)
			require.Equal(t, txsender.Obsolete, eligibility)
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 2)
			require.Empty(t, backend.published[1].Data(), "only neutralization is authorized after the persisted finding")
			repo.AssertExpectations(t)
			chain.AssertExpectations(t)
		})
	}
}

func TestProvisionalMissingStagePreservesExecutionStop(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe} {
		for _, stop := range []model.ApplicationStatus{model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
			t.Run(policy.String()+"/"+stop.String(), func(t *testing.T) {
				s, repo, chain := newServiceMock(t)
				s.defaultBlock = policy
				app := makeApplication()
				app.Status = stop
				epoch := makeStagedEpoch(app, 3, 30)
				chain.On("getConsensusAddress", mock.Anything, app, big.NewInt(50)).Return(app.IConsensusAddress, nil).Once()
				expectGetClaimStatusUnstaged(chain, app, epoch, big.NewInt(50))
				result := s.processStagedClaim(t.Context(), claimWork{app: app, epoch: epoch}, big.NewInt(50))
				require.ErrorIs(t, result.err, txsender.ErrInconclusive)
				require.True(t, txsender.WaitOnly(result.err))
				require.Equal(t, stop, app.Status)
				require.Empty(t, repo.Calls, "a provisional absence is not an integrity finding")
				chain.AssertExpectations(t)
			})
		}
	}
}

func TestMissingClaimEpochWaitsWithoutRetainedReplacement(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, epoch := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	action := s.publisher.lastChoice
	delete(lookup.epochs[app.IApplicationAddress.Hex()], epoch.Index)
	queueDelivery(t, backend, lookup, 2) // No independently established priority replacement.
	block := txsender.BlockRef{Number: backend.last, Hash: deliveryHeader(backend.last).Hash()}
	_, eligibility, err := s.publisher.decide(t.Context(), block, repository.ClaimWork{Application: app, Epoch: epoch}, false)
	require.NoError(t, err)
	require.Equal(t, txsender.Obsolete, eligibility)
	_, err = s.publisher.Recheck(t.Context(), block, action)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.True(t, txsender.WaitOnly(err))
	err = s.publisher.tick(t.Context())
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.True(t, txsender.WaitOnly(err))
	require.Len(t, backend.published, 1, "missing row alone cannot justify cancelling or replacing retained work")
	lookup.add(app, epoch)
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 2)
	require.Equal(t, backend.published[0].Hash(), backend.published[1].Hash())
}

// The publication page fixture is intentionally independent of the domain
// predicate. Its contract mirrors the stored status/cut filters, not the
// validation performed by AllowsEpochCompletion after the page is selected.
func fixtureClaimPublishable(app *model.Application, epoch *model.Epoch, problem *model.ApplicationProblem) bool {
	if app.ForecloseBlock != 0 {
		return false
	}
	switch app.Status {
	case model.ApplicationStatus_OK, model.ApplicationStatus_GuestException, model.ApplicationStatus_MachineHalted,
		model.ApplicationStatus_McycleOverflow, model.ApplicationStatus_UnexpectedYield, model.ApplicationStatus_InvalidOutputsRoot:
		return problem == nil && app.Status == model.ApplicationStatus_OK || problem != nil && epoch.Index < problem.EpochIndex
	case model.ApplicationStatus_Failed, model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted:
		return false
	default:
		return false
	}
}
