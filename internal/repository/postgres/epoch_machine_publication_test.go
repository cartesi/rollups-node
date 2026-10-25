// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/stretchr/testify/require"
)

func machinePublicationEpoch(t *testing.T, r *PostgresRepository,
	foreclosed bool,
) (*model.Application, *model.Epoch, *model.StateProof) {
	t.Helper()
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_Authority).Create(t.Context(), t, r)
	epoch := repotest.NewEpochBuilder(app.ID).WithIndex(4).WithBlocks(40, 49).
		WithStatus(model.EpochStatus_Closed).WithInputBounds(0, 1).Build()
	input := repotest.NewInputBuilder().WithIndex(0).WithBlockNumber(45).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*model.Epoch][]*model.Input{epoch: {input}}, 80))
	proof := repotest.DummyStateProof()
	require.NoError(t, r.StoreAdvanceResult(t.Context(), app.ID, &model.AdvanceResult{
		EpochIndex: epoch.Index, InputIndex: 0, Status: model.InputCompletionStatus_Accepted, StateProof: *proof,
	}))
	if foreclosed {
		require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, 80, repotest.UniqueHash(), 80))
	}
	return app, epoch, proof
}

func TestMachinePublicationGateRetainsAfterLastAcceptedInputUntilIP(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch, proof := machinePublicationEpoch(t, r, true)
	undrained, err := r.HasUndrainedEpochsBeforeBlock(t.Context(), app.ID, 80)
	require.NoError(t, err)
	require.False(t, undrained, "all inputs are accepted, but the final epoch proof is not published yet")
	unpublished, err := r.HasUnpublishedEpochBeforeBlock(t.Context(), app.ID, 80)
	require.NoError(t, err)
	require.True(t, unpublished, "CLOSED -> IP still needs the resident machine")
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
	unpublished, err = r.HasUnpublishedEpochBeforeBlock(t.Context(), app.ID, 80)
	require.NoError(t, err)
	require.False(t, unpublished, "IP validates from durable proof without retaining the machine")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = r.HasUnpublishedEpochBeforeBlock(ctx, app.ID, 80)
	require.ErrorIs(t, err, context.Canceled, "lookup failure is not evidence that publication is complete")
}

func TestMachinePublicationGateRequiresAuthorizedCompleteAQEpoch(t *testing.T) {
	const computedCase = "computed"
	for _, test := range []struct {
		name string
		want bool
	}{
		{"authority", true}, {"quorum", true}, {string(model.Consensus_PRT), false}, {"disabled", false},
		{string(model.ApplicationStatus_Failed), false}, {string(model.ApplicationStatus_Diverged), false},
		{string(model.ApplicationStatus_Corrupted), false},
		{string(model.ApplicationStatus_MachineHalted), false}, {string(model.ApplicationStatus_InvalidOutputsRoot), false},
		{"straddling foreclosure", false}, {"caller bound before epoch end", false},
		{"IP", false}, {computedCase, false}, {"not foreclosed", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app, epoch, proof := machinePublicationEpoch(t, r, test.name != "not foreclosed")
			bound := uint64(80)
			switch test.name {
			case "quorum", string(model.Consensus_PRT):
				app.ConsensusType = model.Consensus_Quorum
				if test.name == string(model.Consensus_PRT) {
					app.ConsensusType = model.Consensus_PRT
				}
				require.NoError(t, r.UpdateApplication(t.Context(), app))
			case "disabled":
				require.NoError(t, r.UpdateApplicationEnabled(t.Context(), app.ID, false))
			case string(model.ApplicationStatus_Failed), string(model.ApplicationStatus_Diverged),
				string(model.ApplicationStatus_Corrupted), string(model.ApplicationStatus_MachineHalted),
				string(model.ApplicationStatus_InvalidOutputsRoot):
				require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID,
					model.ApplicationStatus(test.name), new("producer stopped")))
			case "straddling foreclosure":
				_, err := r.db.Exec(t.Context(), `UPDATE application SET foreclose_block=49 WHERE id=$1`, app.ID)
				require.NoError(t, err)
			case "caller bound before epoch end":
				bound = 49
			case "IP", computedCase:
				require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
				if test.name == computedCase {
					require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
				}
			}
			got, err := r.HasUnpublishedEpochBeforeBlock(t.Context(), app.ID, bound)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}
