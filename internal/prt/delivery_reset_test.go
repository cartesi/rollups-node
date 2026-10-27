// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func resetDeliveryAdmission(f *deliveryFixture, app *model.Application) {
	row := f.repo.deploymentRows[app.ID]
	row.ResetGeneration++
	row.ConfirmedTier, row.DeploymentReason = nil, nil
	f.repo.deploymentRows[app.ID] = row
}

func TestRetainedResetKeepsIndependentJoinSelection(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, resetSibling := range []bool{false, true} {
			name := map[bool]string{false: "disabled", true: "enabled"}[enabled]
			if resetSibling {
				name += "/sibling reset before final authorization"
			}
			t.Run(name, func(t *testing.T) {
				f, held, _ := preparePRTAction(t, stageTournament)
				before := submitPRTAction(t, f, held)
				sibling := addDeliveryJoinCandidate(f, 8, 290)
				join, err := f.d.planAction(t.Context(), sibling, joinTournament, f.block, observerFacts(100))
				require.NoError(t, err)
				require.NotNil(t, join)
				f.app.Enabled = enabled
				resetDeliveryAdmission(f, f.app)
				if resetSibling {
					f.backend.onPreflight = func(context.Context, ethereum.CallMsg, txsender.BlockRef) error {
						resetDeliveryAdmission(f, sibling)
						return nil
					}
				}
				err = f.d.tick(t.Context())
				if resetSibling {
					require.ErrorIs(t, err, txsender.ErrInconclusive)
					if enabled {
						require.Len(t, f.backend.published, 1)
					} else {
						require.Len(t, f.backend.published, 2)
						require.Empty(t, f.backend.published[1].Data())
						require.Equal(t, f.d.sender.From(), *f.backend.published[1].To())
					}
					return
				}
				require.NoError(t, err)
				require.Len(t, f.backend.published, 2)
				published := f.backend.published[1]
				require.Equal(t, before.Nonce, published.Nonce())
				require.Equal(t, join.To, *published.To())
				require.Equal(t, join.Data, published.Data())
				require.Equal(t, join.Value, published.Value())
				require.False(t, f.d.publicationStopped())
			})
		}
	}
}

func TestUnknownDeploymentAllowsIndependentlyObsoleteCancellation(t *testing.T) {
	for _, state := range []deploymentState{deploymentNone, deploymentPending} {
		t.Run(string(state), func(t *testing.T) {
			f, held, _ := preparePRTAction(t, stageTournament)
			before := submitPRTAction(t, f, held)
			updatePRTView(f, 110, resultTestSnapshot(f.epoch, true))
			setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, state, nil)
			report, err := f.d.sender.PublishNext(t.Context(), f.d)
			require.NoError(t, err)
			require.True(t, report.Attempted)
			require.True(t, report.Current.Neutral)
			require.Equal(t, before.Nonce, report.Nonce)
			require.Len(t, f.backend.published, 2)
			require.Empty(t, f.backend.published[1].Data())
			require.Zero(t, f.backend.published[1].Value().Sign())
		})
	}
}

func TestUnknownHeldJoinYieldsNonceToLaterAuthorizedSibling(t *testing.T) {
	f, held, _ := preparePRTAction(t, joinTournament)
	before := submitPRTAction(t, f, held)
	sibling := addDeliveryJoinCandidate(f, 8, 290)
	join, err := f.d.planAction(t.Context(), sibling, joinTournament, f.block, observerFacts(100))
	require.NoError(t, err)
	require.NotNil(t, join)
	require.Greater(t, join.Deadline, held.Deadline)
	resetDeliveryAdmission(f, f.app)
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.False(t, report.Current.Neutral)
	require.Equal(t, sibling.ID, report.Current.Action.ApplicationID)
	require.Equal(t, before.Nonce, report.Nonce)
	require.Len(t, f.backend.published, 2)
	require.Equal(t, join.Data, f.backend.published[1].Data())
}

func TestUnknownHeldJoinYieldsToAuthorizedZeroDeadlineActions(t *testing.T) {
	for _, kind := range []actionKind{stageTournament, recoverBond} {
		t.Run(actionName(kind), func(t *testing.T) {
			f, held, _ := preparePRTAction(t, joinTournament)
			before := submitPRTAction(t, f, held)
			sibling := addSecondStageApplication(f)
			var call *plannedAction
			var err error
			if kind == recoverBond {
				target := common.HexToAddress("0x9870")
				f.factory.On("CreateTournamentAdapter", target).Return(&recoveryPortAdapter{
					recovery: canonicalBondRecovery(model.BondDispositionRecoverable, f.repo.signer, 1)}, nil)
				call, err = f.d.recoveryCall(t.Context(), sibling, f.epoch.Index, target, f.block, observerFacts(100))
			} else {
				call, err = f.d.planAction(t.Context(), sibling, kind, f.block, observerFacts(100))
			}
			require.NoError(t, err)
			require.NotNil(t, call)
			require.Zero(t, call.Deadline)
			resetDeliveryAdmission(f, f.app)
			pass := &deliveryPass{delivery: f.d, planned: true, candidates: []plannedAction{*call}}
			report, err := f.d.sender.PublishNext(t.Context(), pass)
			require.NoError(t, err)
			require.True(t, report.Attempted)
			require.False(t, report.Current.Neutral)
			require.Equal(t, sibling.ID, report.Current.Action.ApplicationID)
			require.Equal(t, before.Nonce, report.Nonce)
			require.Len(t, f.backend.published, 2)
			require.Equal(t, call.Data, f.backend.published[1].Data())
		})
	}
}

func TestRetainedActionBeforeStoredSealDoesNotBecomeObsolete(t *testing.T) {
	for _, equal := range []bool{false, true} {
		t.Run(map[bool]string{false: "older head", true: "seal head"}[equal], func(t *testing.T) {
			f, call, view := preparePRTAction(t, stageTournament)
			f.epoch.LastBlock = f.block.Number
			if !equal {
				f.epoch.LastBlock++
				view.sealed.EpochNumber--
				view.stage.EpochNumber--
				view.accept.EpochNumber--
			}
			updatePRTView(f, f.block.Number, view)
			eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
			if equal {
				require.NoError(t, err)
				require.Equal(t, txsender.Useful, eligibility)
			} else {
				require.ErrorIs(t, err, txsender.ErrInconclusive)
				require.Equal(t, txsender.ProtocolWait, eligibility)
			}
			require.Empty(t, f.backend.published)
		})
	}
}

func TestMissingDeploymentControlOrCacheRetainsUntilIndependentObsolescence(t *testing.T) {
	for _, missing := range []string{"control", "cache"} {
		t.Run(missing, func(t *testing.T) {
			f, call, _ := preparePRTAction(t, stageTournament)
			before := submitPRTAction(t, f, call)
			if missing == "control" {
				delete(f.repo.deploymentRows, f.app.ID)
			} else {
				f.d.deployments = deploymentCache{}
			}
			report, err := f.d.sender.PublishNext(t.Context(), f.d)
			require.ErrorIs(t, err, txsender.ErrInconclusive)
			require.False(t, report.Attempted)
			require.Equal(t, before.Current, report.Current)
			require.Len(t, f.backend.published, 1)
			updatePRTView(f, 110, resultTestSnapshot(f.epoch, true))
			report, err = f.d.sender.PublishNext(t.Context(), f.d)
			require.NoError(t, err)
			require.True(t, report.Attempted)
			require.True(t, report.Current.Neutral)
			require.Len(t, f.backend.published, 2)
			require.Empty(t, f.backend.published[1].Data())
		})
	}
}

func TestHeldAcceptDoesNotRegressToStageAtOlderCanonicalHead(t *testing.T) {
	f, call, _ := preparePRTAction(t, acceptTournament)
	before := submitPRTAction(t, f, call)
	require.Nil(t, f.epoch.StagedAtBlock)
	older := resultTestSnapshot(f.epoch, false)
	older.stage.IsFinished = true
	updatePRTView(f, 109, older)
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.False(t, report.Attempted)
	require.Equal(t, before.Current, report.Current)
	require.Len(t, f.backend.published, 1, "an older same-nonce view must not replace accept with redundant stage")
}
