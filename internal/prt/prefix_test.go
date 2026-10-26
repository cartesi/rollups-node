// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	snapshotReadFailedCase   = "snapshot read failed"
	divergedApplicationCase  = "diverged"
	failedApplicationCase    = "failed"
	corruptedApplicationCase = "corrupted"
	missingBoundaryCase      = "missing boundary"
)

func prtTestProblem(kind model.ApplicationStatus, cutoff uint64) *model.ApplicationProblem {
	p := &model.ApplicationProblem{EpochIndex: cutoff, Kind: kind}
	if kind.IsExecutionTerminal() {
		p.InputIndex = new(uint64(99))
	}
	return p
}

func setPRTTestStop(f *deliveryFixture, kind model.ApplicationStatus, cutoff uint64) {
	f.app.Status = kind
	f.repo.problems = map[int64]*model.ApplicationProblem{f.app.ID: prtTestProblem(kind, cutoff)}
}

func TestPRTCompletesTerminalEpochPrefix(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_GuestException,
		model.ApplicationStatus_MachineHalted, model.ApplicationStatus_McycleOverflow,
		model.ApplicationStatus_UnexpectedYield, model.ApplicationStatus_InvalidOutputsRoot} {
		for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
			t.Run(status.String()+"/"+actionName(kind), func(t *testing.T) {
				f, _, _ := preparePRTAction(t, kind)
				setPRTTestStop(f, status, f.epoch.Index+1)
				call, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
				require.NoError(t, err)
				require.NotNil(t, call, "the complete earlier epoch remains eligible")
				submitPRTAction(t, f, *call)
				report, err := f.d.sender.PublishNext(t.Context(), f.d)
				require.NoError(t, err)
				require.True(t, report.Attempted)
				require.Len(t, f.backend.raw, 2)
				require.Equal(t, f.backend.raw[0], f.backend.raw[1])
				// Defensive case: a single ordered validator cannot tighten
				// to a previously computed epoch. The consumer still refuses
				// it if that premise is violated.
				f.repo.problems[f.app.ID].EpochIndex = f.epoch.Index
				callAtCut, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
				require.ErrorIs(t, err, txsender.ErrIneligible)
				require.Nil(t, callAtCut)
				report, err = f.d.sender.PublishNext(t.Context(), f.d)
				require.NoError(t, err)
				require.True(t, report.Current.Neutral, "positive cutoff evidence makes the held action obsolete")
				require.Len(t, f.backend.published, 3)
				require.Empty(t, f.backend.published[2].Data())
				require.Equal(t, status, f.app.Status)
			})
		}
	}
}

func TestPRTUnknownStopEvidenceAloneDoesNotJustifyReplacement(t *testing.T) {
	for _, fault := range []string{missingBoundaryCase, snapshotReadFailedCase} {
		t.Run(fault, func(t *testing.T) {
			f, call, _ := preparePRTAction(t, stageTournament)
			submitPRTAction(t, f, call)
			setPRTTestStop(f, model.ApplicationStatus_MachineHalted, f.epoch.Index+1)
			cause := errors.New("application epoch snapshot unavailable")
			if fault == missingBoundaryCase {
				f.repo.problems = nil
			} else {
				f.repo.stateRead = func(context.Context, string, uint64) (*repository.EpochCompletionState, error) {
					return nil, cause
				}
			}
			addDeliveryJoinCandidate(f, 8, 25)
			// No separately verified priority evidence is supplied by this failed read.
			report, err := f.d.sender.PublishNext(t.Context(), f.d)
			if fault == missingBoundaryCase {
				require.ErrorIs(t, err, txsender.ErrInconclusive)
			} else {
				require.ErrorIs(t, err, cause)
			}
			require.False(t, report.Attempted)
			require.Equal(t, call.Key, report.Current.Action)
			require.False(t, report.Current.Neutral)
			require.Len(t, f.backend.published, 1)
		})
	}
}

func TestPRTPostSignRechecksTerminalPrefix(t *testing.T) {
	for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
		for _, change := range []string{"later stop", "cutoff", failedApplicationCase, divergedApplicationCase,
			corruptedApplicationCase, missingBoundaryCase, snapshotReadFailedCase} {
			t.Run(actionName(kind)+"/"+change, func(t *testing.T) {
				f, call, _ := preparePRTAction(t, kind)
				cause := errors.New("post-sign application epoch snapshot unavailable")
				factory := deliverySigningFactory{TransactOptsFactory: f.d.txOptsFactory, afterSign: func() {
					setPRTTestStop(f, model.ApplicationStatus_MachineHalted, f.epoch.Index+1)
					switch change {
					case "cutoff":
						f.repo.problems[f.app.ID].EpochIndex = f.epoch.Index
					case failedApplicationCase:
						f.app.Status = model.ApplicationStatus_Failed
					case divergedApplicationCase:
						f.app.Status = model.ApplicationStatus_Diverged
					case corruptedApplicationCase:
						f.app.Status = model.ApplicationStatus_Corrupted
					case missingBoundaryCase:
						f.repo.problems = nil
					case snapshotReadFailedCase:
						f.repo.stateRead = func(context.Context, string, uint64) (*repository.EpochCompletionState, error) {
							return nil, cause
						}
					}
				}}
				sender, err := txsender.New[actionID](f.backend, factory, 31337, f.d.deliveryPolicy, f.d.guardPublication, f.d.Logger)
				require.NoError(t, err)
				report, err := sender.PublishNext(t.Context(), singleAction{f.d, call})
				if change == "later stop" {
					require.NoError(t, err)
					require.True(t, report.Attempted)
				} else {
					require.Error(t, err)
					require.False(t, report.Attempted)
					require.Empty(t, f.backend.published)
					if change == missingBoundaryCase {
						require.ErrorIs(t, err, txsender.ErrInconclusive)
					}
					if change == snapshotReadFailedCase {
						require.ErrorIs(t, err, cause)
					}
				}
			})
		}
	}
}

func TestPRTObserverStagesValidPrefixAfterExecutionStop(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
		t.Run(status.String(), func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			f.app.Status = status
			epoch := checkpointEpoch(0, "0x100")
			problem := prtTestProblem(status, 1)
			f.repo.On("GetEpochCompletionState", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.Index).
				Return(&repository.EpochCompletionState{Application: f.app, Epoch: epoch, Problem: problem}, nil)
			tournament := resultBoundaryTournament(f, epoch, *epoch.Commitment)
			f.expectResultBoundaryWindow(epoch, tournament, 100)
			view := resultTestSnapshot(epoch, true)
			view.sealed.StagingBlockNumber = 100
			f.expectResultBoundarySnapshot(epoch, view, 100)
			f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, epoch.Index, uint64(100)).Return(nil).Once()

			expectProtectionReads(t, f.factory, f.consensus, observerFacts(100), epoch)
			require.NoError(t, f.s.observeApplication(t.Context(), f.app, observerFacts(100)))
			require.Equal(t, uint64(100), tournament.Snapshot.AsOfBlock)
			require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
			require.Equal(t, status, f.app.Status)
		})
	}
}

func TestPRTStopReadFailurePreservesPassiveRootObservation(t *testing.T) {
	for _, fault := range []string{missingBoundaryCase, snapshotReadFailedCase, "consensus read failed"} {
		t.Run(fault, func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			f.app.Status = model.ApplicationStatus_MachineHalted
			epoch := checkpointEpoch(0, "0x100")
			cause := errors.New("protected source unavailable")
			state := &repository.EpochCompletionState{Application: f.app, Epoch: epoch, Problem: prtTestProblem(f.app.Status, 1)}
			var readErr error
			if fault == missingBoundaryCase {
				state.Problem = nil
			}
			if fault == snapshotReadFailedCase {
				readErr = cause
			}
			f.repo.On("GetEpochCompletionState", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.Index).Return(state, readErr)
			if fault == "consensus read failed" {
				f.consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(100))).
					Return(CurrentSealedEpoch{}, cause).Once()
			}
			f.epochs(epoch)
			f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
			f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
			f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(nil).Once()

			expectProtectionReads(t, f.factory, f.consensus, observerFacts(100), epoch)
			err := f.s.observeApplication(t.Context(), f.app, observerFacts(100))

			if fault == missingBoundaryCase {
				require.ErrorIs(t, err, txsender.ErrInconclusive)
			} else {
				require.ErrorIs(t, err, cause)
			}
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
			require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
			f.repo.AssertNumberOfCalls(t, "StoreTournamentInterval", 1)
			f.repo.AssertNotCalled(t, "UpdateEpochReconciledStaged", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}
