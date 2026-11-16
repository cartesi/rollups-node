// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	stagedIORCondition      = "ior_result_staged"
	iorMatchingRootsCase    = "matching roots"
	iorMismatchingRootsCase = "mismatching roots"
)

func prtIORRecords(t *testing.T, logs *bytes.Buffer, condition string) []map[string]any {
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

func TestIORStageObservationPersistsAndReconstructsDiagnostic(t *testing.T) {
	const stagingPeriod = uint64(17)
	f := newDeliveryFixture(t)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	f.app.ClaimStagingPeriod = stagingPeriod
	f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
	declared, calculated := *f.epoch.TxBufferDataBlock, *f.epoch.CalculatedOutputsRoot
	view := resultTestSnapshot(f.epoch, true)
	observed := resultTestObservation(f.epoch, view)
	f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index,
		view.sealed.StagingBlockNumber).Return(nil).Once()
	require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, observed))
	require.Len(t, prtIORRecords(t, &logs, stagedIORCondition), 1, "the first projection must report immediately")
	require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, observed))
	records := prtIORRecords(t, &logs, stagedIORCondition)
	require.Len(t, records, 1, "an unchanged observer snapshot must not repeat the stage alarm")
	record := records[0]
	require.Equal(t, "ERROR", record["level"])
	require.Equal(t, declared.Hex(), record["declared_outputs_root"])
	require.Equal(t, calculated.Hex(), record["calculated_outputs_root"])
	require.Equal(t, float64(stagingPeriod), record["staging_period"])
	require.Equal(t, float64(*f.epoch.StagedAtBlock), record["staged_at_block"])
	require.Equal(t, f.epoch.TournamentAddress.Hex(), record["root_tournament"])
	require.Equal(t, float64(observed.Number), record["observation_block"])
	require.Equal(t, observed.Hash.Hex(), record["observation_hash"])
	require.Equal(t, "consensus_snapshot", record["observation_source"])
	require.Equal(t, "FINALIZED", record["observation_policy"])

	// A new service has no volatile marker and can reconstruct the condition
	// from the stored staged epoch, without running the validator.
	restarted := newDeliveryFixture(t)
	var restartedLogs bytes.Buffer
	restarted.d.Logger = slog.New(slog.NewJSONHandler(&restartedLogs, nil))
	persisted := *f.epoch
	require.NoError(t, restarted.d.recordTournamentResult(t.Context(), f.app, &persisted, view, observed))
	require.Len(t, prtIORRecords(t, &restartedLogs, stagedIORCondition), 1)
	restarted.repo.AssertNotCalled(t, "UpdateEpochReconciledStaged", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	// A different provisional stage identity is a new observation, not spam.
	f.d.defaultBlock = model.DefaultBlock_Latest
	oldBlock := *f.epoch.StagedAtBlock
	view.sealed.StagingBlockNumber++
	f.repo.On("UpdateEpochStagingBlock", mock.Anything, f.app.ID, f.epoch.Index,
		oldBlock, view.sealed.StagingBlockNumber).Return(nil).Once()
	observed = resultTestObservation(f.epoch, view)
	require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, observed))
	require.Len(t, prtIORRecords(t, &logs, stagedIORCondition), 2)
	require.Equal(t, declared, *f.epoch.TxBufferDataBlock)
	require.Equal(t, calculated, *f.epoch.CalculatedOutputsRoot)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	require.Empty(t, f.backend.published)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.repo.AssertExpectations(t)
}

func TestIORStageObservationRequiresKnownMismatchingRoots(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: iorMatchingRootsCase, true: "unknown calculated root"}[unknown], func(t *testing.T) {
			f := newDeliveryFixture(t)
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			if unknown {
				f.epoch.CalculatedOutputsRoot = nil
			}
			view := resultTestSnapshot(f.epoch, true)
			f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index,
				view.sealed.StagingBlockNumber).Return(nil).Once()
			require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, resultTestObservation(f.epoch, view)))
			require.Empty(t, prtIORRecords(t, &logs, stagedIORCondition))
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.repo.AssertExpectations(t)
		})
	}
}

func TestIORAcceptanceObservationFollowsVerifiedProjection(t *testing.T) {
	for _, roots := range []string{iorMismatchingRootsCase, iorMatchingRootsCase, "unknown"} {
		t.Run(roots, func(t *testing.T) {
			f := newAcceptanceReceiptFixture(t)
			var logs bytes.Buffer
			f.service.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			switch roots {
			case iorMismatchingRootsCase:
				f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
			case "unknown":
				f.epoch.CalculatedOutputsRoot = nil
			}
			f.service.iorWinners = map[int64]iorWinnerIdentity{f.app.ID: {epochIndex: f.epoch.Index}}
			f.expectReceipt()
			f.expectAcceptance()
			deferred, err := f.service.reconcileAcceptedEpochs(t.Context(), f.app,
				[]*model.Epoch{f.epoch}, f.consensus, f.log.BlockNumber)
			require.NoError(t, err)
			require.False(t, deferred)
			records := prtIORRecords(t, &logs, "ior_result_accepted")
			if roots == iorMismatchingRootsCase {
				require.Len(t, records, 1)
				require.Equal(t, "ERROR", records[0]["level"])
				require.Equal(t, f.epoch.TxBufferDataBlock.Hex(), records[0]["declared_outputs_root"])
				require.Equal(t, f.epoch.CalculatedOutputsRoot.Hex(), records[0]["calculated_outputs_root"])
				require.Equal(t, f.log.TxHash.Hex(), records[0]["event_transaction"])
			} else {
				require.Empty(t, records, "acceptance is projected but only known IOR has its diagnostic")
			}
			require.Empty(t, f.service.iorWinners, "acceptance retires the current-winner signal")
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}
