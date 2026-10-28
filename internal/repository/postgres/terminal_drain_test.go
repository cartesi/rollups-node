// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/stretchr/testify/require"
)

const (
	terminalDrainDisabledCase  = "disabled"
	terminalDrainDivergedCase  = "diverged"
	terminalDrainCorruptedCase = "corrupted"
	terminalDrainFailedCase    = "failed"
)

// The advancer reaches the terminal input in P=2 before E=0/1 are validated.
// A later NONE input lies in the foreclosure block itself.
func terminalDrainFixture(t *testing.T, consensus model.Consensus) (*PostgresRepository, *model.Application) {
	t.Helper()
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithConsensus(consensus).Create(t.Context(), t, r)
	rows := make(map[*model.Epoch][]*model.Input)
	for i := range uint64(4) {
		epoch := repotest.NewEpochBuilder(app.ID).WithIndex(i).WithStatus(model.EpochStatus_Closed).
			WithBlocks(i*10, i*10+9).WithInputBounds(i, i+1).Build()
		rows[epoch] = []*model.Input{repotest.NewInputBuilder().WithIndex(i).WithBlockNumber(i*10 + 5).Build()}
	}
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, rows, 40))
	for i := range uint64(2) {
		proof := repotest.DummyStateProof()
		require.NoError(t, r.StoreAdvanceResult(t.Context(), app.ID, &model.AdvanceResult{
			EpochIndex: i, InputIndex: i, Status: model.InputCompletionStatus_Accepted, StateProof: *proof,
		}))
		require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, i, proof))
	}
	require.NoError(t, r.StoreAdvanceResult(t.Context(), app.ID, &model.AdvanceResult{
		EpochIndex: 2, InputIndex: 2, Status: model.InputCompletionStatus_MachineHalted, StateProof: *repotest.DummyStateProof(),
	}))
	require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, 35, repotest.UniqueHash(), 40))
	return r, app
}

func TestTerminalInputDrainUsesExecutionCursorAfterEarlierIOR(t *testing.T) {
	for _, consensus := range model.ConsensusAllValues {
		t.Run(consensus.String(), func(t *testing.T) {
			r, app := terminalDrainFixture(t, consensus)
			pending, err := r.HasUndrainedEpochsBeforeBlock(t.Context(), app.ID, 35)
			require.NoError(t, err)
			require.False(t, pending, "the terminal machine cannot execute its later same-block input")
			require.NoError(t, r.StoreInvalidOutputsRoot(t.Context(), app.ID, 1, "earlier root mismatch"))
			state, err := r.GetEpochCompletionState(t.Context(), app.Name, 0)
			require.NoError(t, err)
			require.Equal(t, uint64(1), state.Problem.EpochIndex)
			require.Equal(t, uint64(3), state.Application.ProcessedInputs)
			pending, err = r.HasUndrainedEpochsBeforeBlock(t.Context(), app.ID, 35)
			require.NoError(t, err)
			require.False(t, pending)
			// A cut is not a substitute for execution progress. Detect an
			// inconsistent NONE below the actual cursor, even outside the
			// now-smaller publication prefix.
			_, err = r.db.Exec(t.Context(), `UPDATE application SET processed_inputs=4 WHERE id=$1`, app.ID)
			require.NoError(t, err)
			pending, err = r.HasUndrainedEpochsBeforeBlock(t.Context(), app.ID, 35)
			require.NoError(t, err)
			require.True(t, pending)
			_, err = r.db.Exec(t.Context(), `UPDATE application SET processed_inputs=3 WHERE id=$1`, app.ID)
			require.NoError(t, err)
			require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, model.ApplicationStatus_Corrupted, new("integrity failure")))
			pending, err = r.HasUndrainedEpochsBeforeBlock(t.Context(), app.ID, 35)
			require.NoError(t, err)
			require.True(t, pending, "DC does not grant terminal-suffix drain permission")
		})
	}
}

func TestTerminalForeclosureDrainsSuffixPreservingValidationAndAcceptance(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		t.Run(consensus.String(), func(t *testing.T) {
			r, app := terminalDrainFixture(t, consensus)
			count, err := r.ForecloseUnacceptedEpochsAtOrAfterBlock(t.Context(), app.ID, 35)
			require.NoError(t, err)
			require.Equal(t, int64(2), count, "P and its unreachable suffix are terminalized")
			for i := range uint64(2) {
				epoch, err := r.GetEpoch(t.Context(), app.Name, i)
				require.NoError(t, err)
				require.Equal(t, model.EpochStatus_InputsProcessed, epoch.Status, "earlier validation remains required")
			}
			require.NoError(t, r.StoreInvalidOutputsRoot(t.Context(), app.ID, 1, "earlier root mismatch"))
			count, err = r.ForecloseUnacceptedEpochsAtOrAfterBlock(t.Context(), app.ID, 35)
			require.NoError(t, err)
			require.Equal(t, int64(1), count, "the newly invalid earlier epoch can drain")
			epoch, err := r.GetEpoch(t.Context(), app.Name, 0)
			require.NoError(t, err)
			require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
			accepted := repotest.UniqueHash()
			require.NoError(t, r.UpdateEpochWithAcceptedClaim(t.Context(), app.ID, 0, &accepted))
			count, err = r.ForecloseUnacceptedEpochsAtOrAfterBlock(t.Context(), app.ID, 35)
			require.NoError(t, err)
			require.Zero(t, count)
			epoch, err = r.GetEpoch(t.Context(), app.Name, 0)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_ClaimAccepted, epoch.Status)
			require.Equal(t, &accepted, epoch.ClaimTransactionHash)
			pending, err := r.HasUnreconciledClaimsBeforeBlock(t.Context(), app.ID, 35)
			require.NoError(t, err)
			require.False(t, pending)
		})
	}
}

func TestTerminalForeclosureDrainRetainsGlobalStopGuards(t *testing.T) {
	for _, stop := range []string{terminalDrainDisabledCase, terminalDrainDivergedCase, terminalDrainCorruptedCase} {
		t.Run(stop, func(t *testing.T) {
			r, app := terminalDrainFixture(t, model.Consensus_Authority)
			if stop == terminalDrainDisabledCase {
				require.NoError(t, r.UpdateApplicationEnabled(t.Context(), app.ID, false))
			} else {
				status := model.ApplicationStatus_Diverged
				if stop == terminalDrainCorruptedCase {
					status = model.ApplicationStatus_Corrupted
				}
				require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, status, new("integrity failure")))
			}
			count, err := r.ForecloseUnacceptedEpochsAtOrAfterBlock(t.Context(), app.ID, 35)
			require.NoError(t, err)
			require.Zero(t, count)
			require.ErrorIs(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, 0), repository.ErrNoUpdate)
			require.ErrorIs(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, 2), repository.ErrNoUpdate)
			epoch, err := r.GetEpoch(t.Context(), app.Name, 2)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_Closed, epoch.Status)
		})
	}
}

func TestTerminalIndividualDrainPreservesPrefixAwaitingValidator(t *testing.T) {
	r, app := terminalDrainFixture(t, model.Consensus_PRT)
	require.ErrorIs(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, 0), repository.ErrNoUpdate)
	require.NoError(t, r.StoreInvalidOutputsRoot(t.Context(), app.ID, 1, "earlier root mismatch"))
	require.NoError(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, 1))
	require.NoError(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, 2))
	epoch, err := r.GetEpoch(t.Context(), app.Name, 0)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_InputsProcessed, epoch.Status)
}

// CLAIM_* reconciliation existed before terminal-prefix draining. Unhealthy or
// disabled applications must not leave these already-computed rows retrying forever.
func TestForeclosedClaimReconciliationPreservesHealthAndEnableState(t *testing.T) {
	for _, stop := range []string{
		terminalDrainFailedCase, terminalDrainDivergedCase, terminalDrainCorruptedCase, terminalDrainDisabledCase,
	} {
		for _, claimStatus := range []model.EpochStatus{
			model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted, model.EpochStatus_ClaimStaged,
		} {
			t.Run(stop+"/"+claimStatus.String(), func(t *testing.T) {
				r := newEpochPublicationRepository(t)
				app, epochs := seedProblemEpochs(t, r)
				require.NoError(t, r.StoreClaimAndProofs(t.Context(), epochs[0], nil))
				if claimStatus != model.EpochStatus_ClaimComputed {
					require.NoError(t, r.UpdateEpochWithSubmittedClaim(t.Context(), app.ID, 0, repotest.UniqueHash()))
				}
				if claimStatus == model.EpochStatus_ClaimStaged {
					require.NoError(t, r.UpdateEpochToStaged(t.Context(), app.ID, 0, 20))
				}
				require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, 35, repotest.UniqueHash(), 40))
				if stop == terminalDrainDisabledCase {
					require.NoError(t, r.UpdateApplicationEnabled(t.Context(), app.ID, false))
				} else {
					status := model.ApplicationStatus_Failed
					switch stop {
					case terminalDrainDivergedCase:
						status = model.ApplicationStatus_Diverged
					case terminalDrainCorruptedCase:
						status = model.ApplicationStatus_Corrupted
					}
					require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, status, new("independent stop")))
				}
				before, err := r.GetApplication(t.Context(), app.Name)
				require.NoError(t, err)
				require.NoError(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, 0))
				epoch, err := r.GetEpoch(t.Context(), app.Name, 0)
				require.NoError(t, err)
				require.Equal(t, model.EpochStatus_ClaimForeclosed, epoch.Status)
				require.ErrorIs(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, 0), repository.ErrNoUpdate)
				after, err := r.GetApplication(t.Context(), app.Name)
				require.NoError(t, err)
				require.Equal(t, before.Status, after.Status)
				require.Equal(t, before.Reason, after.Reason)
				require.Equal(t, before.Enabled, after.Enabled)
				// Reopening reconciliation does not grant permission to drain
				// unvalidated prefix or suffix rows under the same stop.
				require.ErrorIs(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, 1), repository.ErrNoUpdate)
				require.ErrorIs(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, 2), repository.ErrNoUpdate)
			})
		}
	}
}
