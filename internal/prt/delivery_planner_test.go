// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	disabledApplicationCase = "disabled"
	removedApplicationCase  = "removed"
)

func preparePRTAction(t *testing.T, kind actionKind) (*deliveryFixture, plannedAction, daveConsensusSnapshot) {
	t.Helper()
	f := newDeliveryFixture(t)
	view := resultTestSnapshot(f.epoch, kind == acceptTournament)
	view.stage.IsFinished = kind != joinTournament
	view.accept.IsClaimStagingPeriodOver = true
	f.snapshot(view)
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, mock.Anything).Return((*uint64)(nil), nil).Maybe()
	if kind == joinTournament {
		deliveryJoinAdapter(f, f.epoch, 50)
	}
	call, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
	require.NoError(t, err)
	require.NotNil(t, call)
	return f, *call, view
}

func updatePRTView(f *deliveryFixture, block uint64, view daveConsensusSnapshot) {
	f.backend.advance(block)
	f.block = txsender.BlockRef{Number: block, Hash: f.backend.headers[block].Hash()}
	f.consensus.ExpectedCalls = nil
	f.snapshot(view)
}

func submitPRTAction(t *testing.T, f *deliveryFixture, call plannedAction) txsender.Report[actionID] {
	t.Helper()
	report, err := f.d.sender.PublishNext(t.Context(), singleAction{f.d, call})
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.NotNil(t, report.Current)
	require.Equal(t, call.Key, report.Current.Action)
	return report
}

func TestPRTPublicationUsesRealSenderAndDoesNotWriteProtocolFacts(t *testing.T) {
	for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
		t.Run(actionName(kind), func(t *testing.T) {
			f, call, _ := preparePRTAction(t, kind)
			report := submitPRTAction(t, f, call)
			require.Equal(t, uint64(7), report.Nonce)
			require.Len(t, f.backend.published, 1)
			tx := f.backend.published[0]
			require.Equal(t, call.To, *tx.To())
			require.Equal(t, call.Data, tx.Data())
			require.Equal(t, call.Value, tx.Value())
			require.Equal(t, uint64(intrinsicGasFloor+150_000), tx.Gas())
			for _, block := range f.backend.calls {
				require.Equal(t, f.block, block)
			}
			require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
			require.Empty(t, f.repo.windows)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestPRTCurrentBytesSurviveLostReplyAndRebroadcastWithoutNewDomainWrites(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	lost := errors.New("connection reset after accepted POST")
	f.backend.publishError = lost
	report, err := f.d.sender.PublishNext(t.Context(), singleAction{f.d, call})
	require.ErrorIs(t, err, lost)
	require.True(t, report.Attempted)
	require.NotNil(t, report.Current)
	f.backend.publishError = nil
	report, err = f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.Len(t, f.backend.raw, 2)
	require.Equal(t, f.backend.raw[0], f.backend.raw[1])
	require.Equal(t, call.Key, report.Current.Action)
	require.Empty(t, f.repo.windows)
}

func TestPRTHeldActionEligibilityUsesCurrentContractState(t *testing.T) {
	for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
		for _, change := range []string{disabledApplicationCase, "unhealthy", "foreclosed",
			removedApplicationCase, "epoch advanced", "contract foreclosed"} {
			t.Run(actionName(kind)+"/"+change, func(t *testing.T) {
				f, call, view := preparePRTAction(t, kind)
				submitPRTAction(t, f, call)
				switch change {
				case disabledApplicationCase:
					f.app.Enabled = false
				case "unhealthy":
					f.app.Status = model.ApplicationStatus_Diverged
				case "foreclosed":
					f.app.ForecloseBlock = 100
				case removedApplicationCase:
					f.repo.apps = nil
				case "epoch advanced":
					view.sealed.EpochNumber++
					view.stage.EpochNumber++
					view.accept.EpochNumber++
					f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), view.sealed.EpochNumber).
						Return((*model.Epoch)(nil), nil)
					updatePRTView(f, 111, view)
				case "contract foreclosed":
					f.backend.foreclosure = true
				}
				report, err := f.d.sender.PublishNext(t.Context(), f.d)
				require.NoError(t, err)
				require.True(t, report.Attempted)
				require.True(t, report.Current.Neutral)
				require.Len(t, f.backend.published, 2)
				neutral := f.backend.published[1]
				require.Equal(t, f.d.txOptsFactory.From(), *neutral.To())
				require.Empty(t, neutral.Data())
				require.Zero(t, neutral.Value().Sign())
				require.Equal(t, uint64(7), neutral.Nonce())
			})
		}
	}
}

func TestPRTHeldReadFailureNeverAuthorizesCancellation(t *testing.T) {
	for _, fault := range []string{"RPC failure", "future foreclosure", "future staging", "future root",
		"missing tournament", "missing commitment", "missing machine hash", "missing outputs"} {
		t.Run(fault, func(t *testing.T) {
			f, call, _ := preparePRTAction(t, stageTournament)
			submitPRTAction(t, f, call)
			switch fault {
			case "RPC failure":
				f.backend.callError = errors.New("application view unavailable")
			case "future foreclosure":
				f.app.ForecloseBlock = 120
			case "future staging":
				f.epoch.StagedAtBlock = new(uint64(120))
			case "future root":
				f.repo.publishRoot(f.app, f.epoch, 120)
			case "missing tournament":
				f.epoch.TournamentAddress = nil
			case "missing commitment":
				f.epoch.Commitment = nil
			case "missing machine hash":
				f.epoch.MachineHash = nil
			case "missing outputs":
				f.epoch.TxBufferDataBlock = nil
			}
			report, err := f.d.sender.PublishNext(t.Context(), f.d)
			require.Error(t, err)
			require.False(t, report.Attempted)
			require.NotNil(t, report.Current)
			require.False(t, report.Current.Neutral)
			require.Equal(t, call.Key, report.Current.Action)
			require.Len(t, f.backend.published, 1)
		})
	}
}

func TestPRTHeldAcceptanceDistinguishesMissingStageFromTimedWait(t *testing.T) {
	for _, staged := range []bool{false, true} {
		t.Run(map[bool]string{false: "stage orphaned", true: "staging wait"}[staged], func(t *testing.T) {
			f, call, _ := preparePRTAction(t, acceptTournament)
			submitPRTAction(t, f, call)
			view := resultTestSnapshot(f.epoch, staged)
			if staged {
				view.sealed.StagingBlockNumber = 111
			}
			updatePRTView(f, 111, view)
			report, err := f.d.sender.PublishNext(t.Context(), f.d)
			require.NoError(t, err)
			require.Equal(t, !staged, report.Attempted)
			require.False(t, report.Current.Neutral)
			if !staged {
				require.Equal(t, stageTournament, report.Current.Action.Kind)
			}
			require.Len(t, f.backend.published, map[bool]int{true: 1, false: 2}[staged])
		})
	}
}

func TestPRTFreshDiscoveryExcludesAcceptedButFixedSelectionRechecksLiveState(t *testing.T) {
	for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
		t.Run(actionName(kind), func(t *testing.T) {
			f, call, view := preparePRTAction(t, kind)
			f.epoch.Status = model.EpochStatus_ClaimAccepted
			fresh, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
			require.ErrorIs(t, err, txsender.ErrIneligible)
			require.Nil(t, fresh)
			// Fixed selection is allowed to finish if the live contract still needs it.
			eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
			require.NoError(t, err)
			require.Equal(t, txsender.Useful, eligibility)
			// Canonical completion (the consensus advanced) suppresses the same action.
			view.sealed.EpochNumber++
			view.stage.EpochNumber++
			view.accept.EpochNumber++
			updatePRTView(f, 111, view)
			eligibility, err = f.d.Recheck(t.Context(), f.block, call.Key)
			require.NoError(t, err)
			require.Equal(t, txsender.Obsolete, eligibility)
			require.Empty(t, f.backend.published)
		})
	}
}

func TestPRTHeldTerminalProjectionDoesNotCreateFreshWorkAfterReorg(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe, model.DefaultBlock_Finalized} {
		t.Run(policy.String(), func(t *testing.T) {
			f, call, view := preparePRTAction(t, acceptTournament)
			f.d.defaultBlock = policy
			submitPRTAction(t, f, call)
			f.epoch.Status = model.EpochStatus_ClaimAccepted
			updatePRTView(f, 111, view)
			report, err := f.d.sender.PublishNext(t.Context(), f.d)
			require.NoError(t, err)
			require.True(t, report.Attempted)
			require.Equal(t, call.Key, report.Current.Action)
			f.backend.nonce = 8
			report, err = f.d.sender.PublishNext(t.Context(), f.d)
			require.NoError(t, err)
			require.False(t, report.Attempted)
			require.Nil(t, report.Current, "consumed current is not restored from an inventory")
			require.Equal(t, model.EpochStatus_ClaimAccepted, f.epoch.Status)
		})
	}
}

func TestPRTObservationFailureDoesNotStopUsefulCurrentRebroadcast(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	submitPRTAction(t, f, call)
	client := &ethClientMock{}
	client.On("HeaderByNumber", mock.Anything, mock.Anything).Return((*types.Header)(nil), errors.New("selected head delayed"))
	f.d.client = client
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.Equal(t, f.backend.raw[0], f.backend.raw[1])
}

func TestPRTUrgentJoinReplacesOnlyLaterDeadlineCurrent(t *testing.T) {
	for _, allowance := range []uint64{25, 50, 80} {
		t.Run(new(big.Int).SetUint64(allowance).String(), func(t *testing.T) {
			f, call, _ := preparePRTAction(t, joinTournament)
			submitPRTAction(t, f, call)
			other := addDeliveryJoinCandidate(f, 8, allowance)
			report, err := f.d.sender.PublishNext(t.Context(), f.d)
			require.NoError(t, err)
			require.True(t, report.Attempted)
			if allowance < 50 {
				require.Equal(t, other.ID, report.Current.Action.ApplicationID)
			} else {
				require.Equal(t, f.app.ID, report.Current.Action.ApplicationID)
				require.Equal(t, f.backend.raw[0], f.backend.raw[1])
			}
			require.Equal(t, uint64(7), f.backend.published[1].Nonce())
		})
	}
}

func TestPRTNeutralReplacementIsNotSticky(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	submitPRTAction(t, f, call)
	f.app.Enabled = false
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Current.Neutral)
	other := addDeliveryJoinCandidate(f, 8, 25)
	other.Enabled = true
	report, err = f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.False(t, report.Current.Neutral)
	require.Equal(t, other.ID, report.Current.Action.ApplicationID)
	require.Equal(t, uint64(7), f.backend.published[2].Nonce())
}

func TestPRTRecoveryExplicitGasAndUsefulReturnWithRealSender(t *testing.T) {
	for _, useful := range []bool{true, false} {
		t.Run(map[bool]string{true: "payment succeeds", false: "callback returns false"}[useful], func(t *testing.T) {
			f := newRecoveryPortFixture(t)
			f.backend.recoveryFalse = !useful
			call, err := f.plan(t.Context())
			require.NoError(t, err)
			require.NotNil(t, call)
			report, err := f.d.sender.PublishNext(t.Context(), singleAction{f.d, *call})
			if useful {
				require.NoError(t, err)
				require.True(t, report.Attempted)
				require.Equal(t, uint64(recoveryGasLimit), f.backend.published[0].Gas())
			} else {
				require.ErrorIs(t, err, txsender.ErrIneligible)
				require.False(t, report.Attempted)
				require.Empty(t, f.backend.published)
			}
			require.Empty(t, f.repo.windows)
		})
	}
}

func TestPRTUnfundedJoinFallsThroughToAffordableAction(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	other := addDeliveryJoinCandidate(f, 8, 25)
	// The urgent join's bond is unaffordable; an ordinary zero-value stage fits.
	f.backend.balance = big.NewInt(int64((intrinsicGasFloor + 150_000) * 3))
	require.Zero(t, call.Value.Sign())
	err := f.d.tick(t.Context())
	require.ErrorIs(t, err, txsender.ErrLimit)
	require.Len(t, f.backend.published, 1)
	require.Equal(t, f.app.IConsensusAddress, *f.backend.published[0].To())
	require.NotEqual(t, other.IConsensusAddress, *f.backend.published[0].To())
	require.NotEmpty(t, f.d.candidateCooldown)
}

func TestPRTIncludedLostRacesDoNotPauseLaterEpochs(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	for range 8 {
		report := submitPRTAction(t, f, call)
		require.False(t, report.Current.Neutral)
		f.backend.nonce++ // Included revert still consumes a nonce; no receipt breaker exists.
	}
	require.Len(t, f.backend.published, 8)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	require.False(t, f.d.publicationStopped())
}

func TestPRTReorgReconstructsCurrentNonceWithoutOldGenerationInventory(t *testing.T) {
	f, call, view := preparePRTAction(t, stageTournament)
	submitPRTAction(t, f, call)
	f.backend.nonce = 8
	submitPRTAction(t, f, call)
	f.backend.nonce = 7
	f.backend.headers[110].Extra = []byte("replacement branch")
	updatePRTView(f, 111, view)
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.Equal(t, uint64(7), f.backend.published[2].Nonce())
	require.Equal(t, call.Key, report.Current.Action)
	require.Equal(t, uint64(8), f.backend.published[1].Nonce(), "old higher-nonce bytes remain distributed")
}

func TestPRTPublicationWarningsHaveApplicationAndActionContext(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	f.d.warnPublicationRefusal(call.Key, txsender.Report[actionID]{Nonce: 7, Block: f.block}, errors.New("fee cap too low"))
	first := logs.String()
	require.Contains(t, first, f.app.IApplicationAddress.Hex())
	require.Contains(t, first, "stage tournament result")
	require.Contains(t, first, "epoch=")
	f.d.warnPublicationRefusal(call.Key, txsender.Report[actionID]{Nonce: 7, Block: f.block}, errors.New("same refusal"))
	require.Equal(t, first, logs.String())
	f.d.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	f.d.diagnostics.head, f.d.diagnostics.headSince = f.block, time.Now().Add(-2*frozenLiveHeadInterval)
	f.d.recordLiveProgress(txsender.Report[actionID]{Block: f.block, Nonce: 7,
		Current: &txsender.Current[actionID]{Action: call.Key}}, nil, nil)
	require.False(t, f.d.diagnostics.warnedHead.IsZero())
}

func TestPRTMissingEpochEvidenceWaitsRatherThanCancelling(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	f.repo.ExpectedCalls = nil
	f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.Index).Return((*model.Epoch)(nil), nil)
	eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.Equal(t, txsender.ProtocolWait, eligibility)
	require.False(t, f.d.publicationStopped())
}
