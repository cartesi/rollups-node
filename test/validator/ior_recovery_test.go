// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"bytes"
	"log/slog"
	"strings"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/ethereum/go-ethereum/common"
)

func (s *ValidatorRepositoryIntegrationSuite) TestIORRecoveryReconstructsAcceptedFactAfterRestart() {
	for _, health := range []model.ApplicationStatus{model.ApplicationStatus_OK, model.ApplicationStatus_Failed,
		model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted} {
		s.Run(health.String(), func() {
			app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(s.ctx, s.T(), s.repository)
			tournament := repotest.NewTournamentBuilder(app.ID).Build()
			epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).
				WithBlocks(0, 9).WithInputBounds(0, 0).Build()
			epoch.TournamentAddress = &tournament.Address
			s.Require().NoError(s.repository.CreateEpochsAndInputs(s.ctx, app.Name,
				map[*model.Epoch][]*model.Input{epoch: nil}, 100))
			proof := repotest.KeccakStateProof(common.Hash{})
			s.Require().NoError(s.repository.UpdateEpochInputsProcessed(s.ctx, app.Name, 0, &proof))
			calculated := merkle.CreatePostContext()[merkle.TREE_DEPTH]
			epoch.CalculatedOutputsRoot = &calculated
			epoch.Commitment = new(repotest.UniqueHash())
			epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
			s.Require().NoError(s.repository.StoreClaimAndProofs(s.ctx, epoch, nil))
			s.Require().NoError(s.repository.CreateTournament(s.ctx, app.Name, tournament))
			s.Require().NoError(s.repository.UpdateEpochReconciledStaged(s.ctx, app.ID, 0, 80))
			s.Require().NoError(s.repository.UpdateEpochWithAcceptedClaim(s.ctx, app.ID, 0, nil))
			if health != model.ApplicationStatus_OK {
				s.Require().NoError(s.repository.UpdateApplicationStatus(s.ctx, app.ID, health,
					new("stored runtime or integrity stop")))
			}
			reconnected, err := factory.NewRepositoryFromConnectionString(s.ctx, s.postgresEndpoint.Value)
			s.Require().NoError(err)
			defer reconnected.Close()
			var output bytes.Buffer
			restarted, err := validator.Create(s.ctx, &validator.CreateInfo{Repository: reconnected,
				Config: config.ValidatorConfig{LogLevel: slog.LevelDebug}, Logger: slog.New(slog.NewTextHandler(&output, nil))})
			s.Require().NoError(err)
			_, err = restarted.(*validator.Service).Tick(s.ctx)
			s.Require().NoError(err)
			var finding string
			for _, line := range strings.Split(output.String(), "\n") {
				if strings.Contains(line, "condition=ior_settlement_withheld") &&
					strings.Contains(line, "application="+app.Name+" ") {
					finding = line
				}
			}
			s.Require().NotEmpty(finding, "this application's stored condition is recovered")
			s.Contains(finding, "epoch_status=CLAIM_ACCEPTED")
			s.Contains(finding, "application_status="+health.String())
			s.Contains(finding, "foreclose_block=0")
			s.Contains(finding, "declared_outputs_root="+common.Hash{}.Hex())
			s.Contains(finding, "calculated_outputs_root="+calculated.Hex())
			s.Contains(finding, "observation_source=stored")
			s.Contains(finding, "join_state=not_checked")
			s.Contains(finding, "commitment="+epoch.Commitment.Hex())
			s.Contains(strings.ToLower(finding), "root_tournament="+strings.ToLower(tournament.Address.Hex()))
			s.NotContains(finding, "tournament_standing=")
			s.NotContains(finding, "defended")
			s.NotContains(output.String(), "condition=ior_result_staged", "accepted epoch keeps only historical staging evidence")
			output.Reset()
			_, err = restarted.(*validator.Service).Tick(s.ctx)
			s.Require().NoError(err)
			s.NotContains(output.String(), "condition=ior_settlement_withheld", "successful recovery is not repeated")
			stored, err := reconnected.GetEpoch(s.ctx, app.Name, 0)
			s.Require().NoError(err)
			s.True(stored.HasInvalidOutputsRoot())
			s.Equal(model.EpochStatus_ClaimAccepted, stored.Status)
			s.Require().NotNil(stored.StagedAtBlock)
			s.Equal(uint64(80), *stored.StagedAtBlock)
		})
	}
}
