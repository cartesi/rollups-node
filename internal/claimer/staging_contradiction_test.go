// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	claimFreshEntry    = "fresh"
	claimRetainedEntry = "retained"
	claimRecheckEntry  = "recheck"
)

func TestPublisherMissingPersistedStageBeforeObserver(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Finalized, model.DefaultBlock_Latest, model.DefaultBlock_Safe} {
		for _, status := range []model.ApplicationStatus{model.ApplicationStatus_OK,
			model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
			t.Run(policy.String()+"/"+status.String(), func(t *testing.T) {
				s, backend, lookup := newDeliveryFixture(t, policy)
				app, epoch := queueDelivery(t, backend, lookup, 1)
				if status != model.ApplicationStatus_OK {
					setClaimTestStop(lookup, app, epoch.Index+1, status)
				}
				epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
				// Only latest/safe may repair the vanished stage before the observer runs.
				err := s.publisher.tick(t.Context())
				if policy.Provisional() {
					require.NoError(t, err)
					require.Len(t, backend.published, 1)
					require.Equal(t, claimSubmitMethod, publishedClaimMethod(t, backend.published[0].Data()))
				} else {
					require.ErrorIs(t, err, txsender.ErrInconclusive)
					require.True(t, txsender.WaitOnly(err))
					require.Empty(t, backend.published)
				}
			})
		}
	}
}

// This contradictory read alone supplies no authorization to submit, cancel
// or replace retained work. A priority decision with independent evidence keeps
// its own rules. The observer outcome is covered separately below.
func TestFinalizedMissingPersistedStageIsLeftForObserver(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_OK,
		model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
		for _, entry := range []string{claimFreshEntry, claimRetainedEntry, claimRecheckEntry} {
			t.Run(status.String()+"/"+entry, func(t *testing.T) {
				s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Finalized)
				app, epoch := queueDelivery(t, backend, lookup, 1)
				if status != model.ApplicationStatus_OK {
					setClaimTestStop(lookup, app, epoch.Index+1, status)
				}
				epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
				backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 30)
				unstage := func() { delete(backend.claims, app.ID) }
				signed := false
				switch entry {
				case claimFreshEntry:
					unstage()
				case claimRetainedEntry:
					require.NoError(t, s.publisher.tick(t.Context()))
					require.Len(t, backend.published, 1)
					require.Equal(t, "acceptClaim", publishedClaimMethod(t, backend.published[0].Data()))
					action := s.publisher.lastChoice
					unstage()
					block := txsender.BlockRef{Number: backend.last, Hash: deliveryHeader(backend.last).Hash()}
					_, err := s.publisher.Recheck(t.Context(), block, action)
					require.ErrorIs(t, err, txsender.ErrInconclusive)
				case claimRecheckEntry:
					useClaimSigningHook(t, s, backend, func() { signed = true; unstage() })
				}
				posts := len(backend.published)
				require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrInconclusive)
				if entry == claimRecheckEntry {
					require.True(t, signed, "the initial decision must reach the signer before the stage disappears")
				}
				require.Len(t, backend.published, posts, "a read of the contradiction must not authorize POST or cancellation")
			})
		}
	}
}

// useClaimSigningHook invokes after a successful signature, before the pre-send recheck.
func useClaimSigningHook(t *testing.T, s *Service, backend *deliveryBackend, after func()) {
	t.Helper()
	key, err := crypto.HexToECDSA("59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
	require.NoError(t, err)
	opts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(31337))
	require.NoError(t, err)
	factory := claimSigningHookFactory{TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(opts), after: after}
	s.publisher.sender, err = txsender.New[claimAction](backend, factory, 31337,
		txsender.Policy{MaxFee: big.NewInt(1000)}, func(context.Context) error { return nil }, s.Logger)
	require.NoError(t, err)
}

func TestSubmittedClaimWithoutPersistedStageCanBeRepublished(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	_, epoch := queueDelivery(t, backend, lookup, 1)
	epoch.Status = model.EpochStatus_ClaimSubmitted
	require.Nil(t, epoch.StagedAtBlock)
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 1, "submission alone is not evidence of staging")
	require.Equal(t, claimSubmitMethod, publishedClaimMethod(t, backend.published[0].Data()))
}

func TestUnknownClaimStatusPreservesStopAndRefusesPublication(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
		t.Run(status.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := queueDelivery(t, backend, lookup, 1)
			setClaimTestStop(lookup, app, epoch.Index+1, status)
			epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
			claim := makeClaimStatus(3, epoch, 30)
			backend.claims[app.ID] = claim
			require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrUnsupported)
			require.Empty(t, backend.published)
			result, done := s.handlePreAcceptClaimStatus(t.Context(), app, epoch, claim, backend.last)
			require.True(t, done)
			require.NoError(t, result.err)
			require.Equal(t, status, app.Status)
			require.Empty(t, s.repository.(*claimerRepositoryMock).Calls)
		})
	}
}

func TestStagingContradictionRecoveryReason(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_OK,
		model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
		t.Run(status.String(), func(t *testing.T) {
			s, _, _ := newDeliveryFixture(t)
			app := makeApplication()
			app.Status, app.Reason = status, new("original execution or root diagnosis")
			epoch := makeStagedEpoch(app, 3, 30)
			expected := model.ApplicationStatus_Corrupted
			if status == model.ApplicationStatus_OK {
				expected = model.ApplicationStatus_Failed
			}
			repo := s.repository.(*claimerRepositoryMock)
			repo.On("UpdateApplicationStatus", mock.Anything, app.ID, expected, mock.Anything).Return(nil).Once()
			_, done := s.handlePreAcceptClaimStatus(t.Context(), app, epoch, makeClaimStatus(claimStatusUnstaged, epoch, 0), 50)
			require.True(t, done)
			require.Equal(t, expected, app.Status)
			require.Contains(t, *app.Reason, "UNSTAGED")
			if status == model.ApplicationStatus_OK {
				require.Contains(t, *app.Reason, "endpoint")
				require.Contains(t, *app.Reason, "re-enabling")
				require.Contains(t, *app.Reason, "fresh database")
			} else {
				require.Contains(t, *app.Reason, "fresh database")
				require.Contains(t, *app.Reason, "foreclosure drain")
			}
			repo.AssertExpectations(t)
		})
	}
}

// A head behind the persisted staging fact is inconclusive even under a
// provisional policy. This tests the observed result, not the ordering of guards.
func TestMissingStageBelowPersistedStagingHeightWaits(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Latest)
	app, epoch := queueDelivery(t, backend, lookup, 1)
	epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(backend.last+1)
	block := txsender.BlockRef{Number: backend.last, Hash: deliveryHeader(backend.last).Hash()}
	_, _, err := s.publisher.decide(t.Context(), block, repository.ClaimWork{Application: app, Epoch: epoch}, false)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
}
