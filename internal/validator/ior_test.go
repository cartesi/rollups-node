// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStoredIORRecoveryUsesEpochFactsWithoutJoinObservation(t *testing.T) {
	repo, s, app, epoch := rootValidationFixture()
	app.ConsensusType = model.Consensus_PRT
	epoch.Status = model.EpochStatus_ClaimAccepted
	epoch.StagedAtBlock = new(uint64(80))
	epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
	epoch.TournamentAddress = new(common.HexToAddress("0x123"))
	epoch.Commitment = new(common.HexToHash("0x456"))
	repo.invalidRootEpochs = []*model.Epoch{epoch}
	var output bytes.Buffer
	s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	s.recoverWithheldSettlements(t.Context(), app)
	require.Equal(t, 1, repo.invalidRootReads)
	require.Contains(t, output.String(), "condition=ior_settlement_withheld")
	require.Contains(t, output.String(), "epoch_status=CLAIM_ACCEPTED")
	require.NotContains(t, output.String(), "condition=ior_result_staged", "a historical stage is not a pending stage")
	require.Contains(t, output.String(), "observation_source=stored")
	require.Contains(t, output.String(), "join_state=not_checked")
	require.Contains(t, output.String(), "commitment="+epoch.Commitment.Hex())
	require.Contains(t, output.String(), "root_tournament="+epoch.TournamentAddress.Hex())
	require.NotContains(t, output.String(), "tournament_standing=")
	require.NotContains(t, output.String(), "defended")
	repo.AssertNotCalled(t, "GetTournament", mock.Anything, mock.Anything, mock.Anything)
	require.True(t, epoch.HasInvalidOutputsRoot())
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
}

func TestWithheldSettlementInventoryFailureDoesNotBlockComputation(t *testing.T) {
	repo, s, app, epoch := rootValidationFixture()
	repo.invalidRootError = errors.New("inventory unavailable")
	repo.On("ListApplications", mock.Anything, enabledApplicationsFilter(), repository.Pagination{}, false).
		Return([]*model.Application{app}, uint64(1), nil).Once()
	expectRootValidationReads(repo, app, epoch,
		&model.Input{MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock}, nil)
	repo.On("StoreClaimAndProofs", mock.Anything, epoch, mock.Anything).Return(nil).Once()
	var output bytes.Buffer
	s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	_, err := s.Tick(t.Context())
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
	require.Contains(t, output.String(), repo.invalidRootError.Error())
	require.NotContains(t, output.String(), "condition=ior_settlement_withheld")
	repo.AssertExpectations(t)
}

func TestWithheldSettlementRecoveryDoesNotLabelUnknownRootsIOR(t *testing.T) {
	repo, s, app, epoch := rootValidationFixture()
	repo.invalidRootEpochs = []*model.Epoch{epoch}
	var output bytes.Buffer
	s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	s.recoverWithheldSettlements(t.Context(), app)
	require.NotContains(t, output.String(), "condition=ior_settlement_withheld")
	require.False(t, epoch.HasInvalidOutputsRoot())
}

func TestWithheldSettlementRecoveryKeepsStoppedHealthSeparateFromComputation(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_Failed, model.ApplicationStatus_Diverged,
		model.ApplicationStatus_Corrupted} {
		t.Run(status.String(), func(t *testing.T) {
			repo, s, app, epoch := rootValidationFixture()
			app.Status = status
			epoch.Status = model.EpochStatus_ClaimAccepted
			epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
			repo.invalidRootEpochs = []*model.Epoch{epoch}
			repo.On("ListApplications", mock.Anything, enabledApplicationsFilter(), repository.Pagination{}, false).
				Return([]*model.Application{app}, uint64(1), nil).Once()
			var output bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			_, err := s.Tick(t.Context())
			require.NoError(t, err)
			require.Contains(t, output.String(), "condition=ior_settlement_withheld")
			require.Contains(t, output.String(), "application_status="+status.String())
			require.Contains(t, output.String(), "foreclose_block=0")
			repo.AssertNotCalled(t, "ListEpochs", mock.Anything, mock.Anything,
				repository.EpochFilter{Status: []model.EpochStatus{model.EpochStatus_InputsProcessed}}, repository.Pagination{}, false)
			repo.AssertNotCalled(t, "StoreClaimAndProofs", mock.Anything, mock.Anything, mock.Anything)
			require.Equal(t, status, app.Status)
			repo.AssertExpectations(t)
		})
	}
}

// Successful empty or nonempty recovery must not become a historical poll loop.
func TestStoredIORRecoveryRunsOnceAfterSuccessfulRead(t *testing.T) {
	for _, finding := range []bool{false, true} {
		t.Run(fmt.Sprint(finding), func(t *testing.T) {
			repo, s, app, epoch := rootValidationFixture()
			app.Status = model.ApplicationStatus_Failed
			if finding {
				epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
				repo.invalidRootEpochs = []*model.Epoch{epoch}
			}
			const visits = 3
			repo.On("ListApplications", mock.Anything, enabledApplicationsFilter(), repository.Pagination{}, false).
				Return([]*model.Application{app}, uint64(1), nil).Times(visits)
			var output bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			for range visits {
				_, err := s.Tick(t.Context())
				require.NoError(t, err)
			}
			require.Equal(t, 1, repo.invalidRootReads)
			expected := 0
			if finding {
				expected = 1
			}
			require.Equal(t, expected, strings.Count(output.String(), "condition=ior_settlement_withheld"))
			require.Equal(t, model.ApplicationStatus_Failed, app.Status)
			repo.AssertExpectations(t)
		})
	}
}

func TestStoredIORRecoveryRetriesFailedInventory(t *testing.T) {
	repo, s, app, epoch := rootValidationFixture()
	epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
	repo.invalidRootEpochs = []*model.Epoch{epoch}
	repo.invalidRootError = errors.New("inventory unavailable")
	var output bytes.Buffer
	s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	s.recoverWithheldSettlements(t.Context(), app)
	repo.invalidRootError = nil
	s.recoverWithheldSettlements(t.Context(), app)
	s.recoverWithheldSettlements(t.Context(), app)
	const recoveryReads = 2
	require.Equal(t, recoveryReads, repo.invalidRootReads)
	require.Equal(t, 1, strings.Count(output.String(), "condition=ior_settlement_withheld"))
}

func TestStoredIORRecoveryCancellationDoesNotConsumeRecovery(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprint(empty), func(t *testing.T) {
			repo, s, app, epoch := rootValidationFixture()
			epoch.Status = model.EpochStatus_ClaimAccepted
			epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
			if !empty {
				repo.invalidRootEpochs = []*model.Epoch{epoch}
			}
			var output bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			repo.invalidRootVisit = func(context.Context) error { cancel(); return nil }
			s.recoverWithheldSettlements(ctx, app)
			require.NotContains(t, output.String(), "level=WARN")
			require.NotContains(t, output.String(), "context canceled")
			repo.invalidRootVisit = nil
			repo.invalidRootEpochs = []*model.Epoch{epoch}
			s.recoverWithheldSettlements(t.Context(), app)
			const recoveryReads = 2
			require.Equal(t, recoveryReads, repo.invalidRootReads)
			require.Contains(t, output.String(), "condition=ior_settlement_withheld")
		})
	}
}

func validatorIORRecords(t *testing.T, logs *bytes.Buffer, condition string) []map[string]any {
	t.Helper()
	var records []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(logs.Bytes()))
	for decoder.More() {
		var record map[string]any
		require.NoError(t, decoder.Decode(&record))
		if record["condition"] == condition {
			records = append(records, record)
		}
	}
	return records
}

func TestStoredIORRecoveryReportsStagedResultWithoutOpeningHealth(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_OK, model.ApplicationStatus_Failed,
		model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted} {
		for _, foreclose := range []uint64{0, 123} {
			t.Run(fmt.Sprintf("%s/foreclose_%d", status, foreclose), func(t *testing.T) {
				repo, s, app, epoch := rootValidationFixture()
				app.Status, app.ForecloseBlock, app.ClaimStagingPeriod = status, foreclose, 17
				epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(80))
				epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
				repo.invalidRootEpochs = []*model.Epoch{epoch}
				var output bytes.Buffer
				s.Logger = slog.New(slog.NewJSONHandler(&output, nil))
				s.recoverWithheldSettlements(t.Context(), app)
				s.recoverWithheldSettlements(t.Context(), app)
				require.Equal(t, 1, repo.invalidRootReads)
				records := validatorIORRecords(t, &output, "ior_result_staged")
				require.Len(t, records, 1)
				record := records[0]
				require.Equal(t, "ERROR", record["level"])
				require.Equal(t, "stored", record["observation_source"])
				require.Equal(t, float64(80), record["staged_at_block"], "staged_at_block")
				require.Equal(t, float64(17), record["staging_period"], "staging_period")
				require.Equal(t, float64(foreclose), record["foreclose_block"])
				require.Equal(t, float64(epoch.Index), record["epoch_index"])
				require.Equal(t, epoch.TxBufferDataBlock.Hex(), record["declared_outputs_root"])
				require.Equal(t, epoch.CalculatedOutputsRoot.Hex(), record["calculated_outputs_root"])
				require.NotContains(t, record, "sentries_agree", "the row does not supply a live sentry verdict")
				require.NotContains(t, record, "staging_period_over")
				require.Equal(t, status, app.Status)
				repo.AssertNotCalled(t, "StoreClaimAndProofs", mock.Anything, mock.Anything, mock.Anything)
			})
		}
	}
}

func TestStoredIORRecoveryReadErrorHonorsCancellationAndRetries(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			repo, s, app, epoch := rootValidationFixture()
			epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
			repo.invalidRootEpochs = []*model.Epoch{epoch}
			readErr := errors.New("recovery read failed")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			repo.invalidRootVisit = func(context.Context) error {
				if cancelled {
					cancel()
				}
				return readErr
			}
			var output bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			s.recoverWithheldSettlements(ctx, app)
			if cancelled {
				require.Empty(t, output.String(), "shutdown read failures do not produce a warning")
			} else {
				require.Contains(t, output.String(), "level=WARN")
				require.Contains(t, output.String(), readErr.Error())
			}
			require.False(t, s.recoveredIOR[app.ID], "a failed read cannot consume recovery")
			output.Reset()
			repo.invalidRootVisit = nil
			s.recoverWithheldSettlements(t.Context(), app)
			s.recoverWithheldSettlements(t.Context(), app)
			require.Equal(t, 2, repo.invalidRootReads)
			require.Equal(t, 1, strings.Count(output.String(), "condition=ior_settlement_withheld"))
		})
	}
}
