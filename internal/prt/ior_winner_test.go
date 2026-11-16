// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"errors"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	withheldIORWinnerCondition = "ior_winner_settlement_withheld"
	iorWinnerUnfinishedCase    = "tournament has not finished"
	iorWinnerFailedCase        = "tournament has failed"
)

func TestIORWinnerObservationReportsCurrentCompatibleResultOnce(t *testing.T) {
	f := newDeliveryFixture(t)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
	view := resultTestSnapshot(f.epoch, false)
	observed := resultTestObservation(f.epoch, view)
	require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, observed))
	records := prtIORRecords(t, &logs, withheldIORWinnerCondition)
	require.Len(t, records, 1)
	record := records[0]
	require.Equal(t, "ERROR", record["level"])
	require.Equal(t, float64(f.epoch.Index), record["epoch_index"])
	require.Equal(t, view.stage.WinnerCommitment.Hex(), record["winner_commitment"])
	require.Equal(t, f.epoch.TxBufferDataBlock.Hex(), record["declared_outputs_root"])
	require.Equal(t, f.epoch.CalculatedOutputsRoot.Hex(), record["calculated_outputs_root"])
	require.Equal(t, "consensus_snapshot", record["observation_source"])
	require.Equal(t, "FINALIZED", record["observation_policy"])
	require.Equal(t, observed.Hash.Hex(), record["observation_hash"])
	require.Equal(t, float64(observed.Number), record["observation_block"])
	require.Contains(t, record["msg"], "guardian")
	require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, observed))
	require.Len(t, prtIORRecords(t, &logs, withheldIORWinnerCondition), 1, "the same winner is not a periodic reminder")

	// Another commitment with the same final execution is compatible, not divergence.
	view.stage.WinnerCommitment = common.HexToHash("0x123abc")
	require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, observed))
	require.Len(t, prtIORRecords(t, &logs, withheldIORWinnerCondition), 2)
	require.Len(t, f.d.iorWinners, 1, "only the current identity of this application is retained")
	require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	require.Empty(t, f.backend.published)

	// A staged result retires the held-winner marker without re-emitting it.
	view = resultTestSnapshot(f.epoch, true)
	f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index,
		view.sealed.StagingBlockNumber).Return(nil).Once()
	require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, observed))
	require.Empty(t, f.d.iorWinners)
	require.Len(t, prtIORRecords(t, &logs, withheldIORWinnerCondition), 2)
	f.repo.AssertExpectations(t)
}

func TestIORWinnerObservationRequiresCurrentEligibleCompatibleWinner(t *testing.T) {
	for _, name := range []string{
		iorMatchingRootsCase, "unknown root", iorWinnerUnfinishedCase, iorWinnerFailedCase,
		noWinnerCase, disabledApplicationCase, foreclosedCase, "different current epoch", "different execution",
	} {
		t.Run(name, func(t *testing.T) {
			f := newDeliveryFixture(t)
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
			view := resultTestSnapshot(f.epoch, false)
			integrityFailure := false
			switch name {
			case iorMatchingRootsCase:
				f.epoch.CalculatedOutputsRoot = f.epoch.TxBufferDataBlock
			case "unknown root":
				f.epoch.CalculatedOutputsRoot = nil
			case iorWinnerUnfinishedCase:
				view.stage.IsFinished = false
			case iorWinnerFailedCase:
				view.stage.IsTournamentFailed = true
			case noWinnerCase:
				view.stage.WinnerCommitment = common.Hash{}
			case disabledApplicationCase:
				f.app.Enabled = false
			case foreclosedCase:
				f.app.ForecloseBlock = 123
			case "different current epoch":
				view.sealed.EpochNumber++
				integrityFailure = true
			case "different execution":
				view.stage.WinnerPostEpochMachineStateHash = common.HexToHash("0xdead")
				integrityFailure = true
			}
			if integrityFailure {
				expectStoredProtection(t, f.repo.prtRepositoryMock, f.app)
				f.repo.On("UpdateApplicationStatus", mock.Anything, f.app.ID, mock.Anything, mock.Anything).Return(nil).Once()
			}
			err := f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, resultTestObservation(f.epoch, view))
			if integrityFailure {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, prtIORRecords(t, &logs, withheldIORWinnerCondition))
			require.Empty(t, f.backend.published)
			f.repo.AssertExpectations(t)
		})
	}
}

func TestIORWinnerMarkerClearsWhenApplicationIsInactive(t *testing.T) {
	f := newDeliveryFixture(t)
	f.d.iorWinners = map[int64]iorWinnerIdentity{f.app.ID: {epochIndex: f.epoch.Index}}
	f.app.Enabled = false
	require.NoError(t, f.d.observeApplication(t.Context(), f.app, observerFacts(100)))
	require.Empty(t, f.d.iorWinners)
	f.d.iorWinners = map[int64]iorWinnerIdentity{f.app.ID: {epochIndex: f.epoch.Index}}
	f.repo.apps = nil
	require.NoError(t, f.d.observe(t.Context()))
	require.Empty(t, f.d.iorWinners)
}

func TestIORStageObservationIncludesCurrentAcceptanceConditions(t *testing.T) {
	for _, test := range []struct {
		name          string
		sentriesAgree bool
		periodOver    bool
	}{
		{"waiting", false, false},
		{"unanimous sentries", true, false},
		{"period elapsed", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newDeliveryFixture(t)
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
			view := resultTestSnapshot(f.epoch, true)
			view.accept.DoAllSentriesAgreeWithStagedTournamentResult = test.sentriesAgree
			view.accept.IsClaimStagingPeriodOver = test.periodOver
			f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index,
				view.sealed.StagingBlockNumber).Return(nil).Once()
			require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, resultTestObservation(f.epoch, view)))
			records := prtIORRecords(t, &logs, stagedIORCondition)
			require.Len(t, records, 1)
			require.Equal(t, test.sentriesAgree, records[0]["sentries_agree"])
			require.Equal(t, test.periodOver, records[0]["staging_period_over"])
			f.repo.AssertExpectations(t)
		})
	}
}

func TestIORStageDiagnosticRequiresSuccessfulProjection(t *testing.T) {
	f := newDeliveryFixture(t)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
	view := resultTestSnapshot(f.epoch, true)
	writeErr := errors.New("stage projection failed")
	f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index,
		view.sealed.StagingBlockNumber).Return(writeErr).Once()
	err := f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, resultTestObservation(f.epoch, view))
	require.ErrorIs(t, err, writeErr)
	require.Empty(t, prtIORRecords(t, &logs, stagedIORCondition))
	require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
	f.repo.AssertExpectations(t)
}

func TestIORAcceptanceDiagnosticRequiresSuccessfulProjection(t *testing.T) {
	f := newAcceptanceReceiptFixture(t)
	var logs bytes.Buffer
	f.service.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
	f.expectReceipt()
	f.consensus.On("ParseEpochSealed", *f.log).Return(&idaveconsensus.IDaveConsensusEpochSealed{
		EpochNumber: new(big.Int).SetUint64(f.epoch.Index + 1), InitialMachineStateHash: *f.epoch.MachineHash,
		OutputsMerkleRoot: *f.epoch.TxBufferDataBlock, Raw: *f.log,
	}, nil).Once()
	writeErr := errors.New("accept projection failed")
	f.repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, f.app.ID, f.epoch.Index, f.epoch.ClaimTransactionHash).
		Return(writeErr).Once()
	deferred, err := f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, f.log.BlockNumber)
	require.ErrorIs(t, err, writeErr)
	require.True(t, deferred)
	require.Empty(t, prtIORRecords(t, &logs, "ior_result_accepted"))
}

func TestIORHistoricalAcceptanceKeepsCurrentWinnerMarker(t *testing.T) {
	f := newDeliveryFixture(t)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
	view := resultTestSnapshot(f.epoch, false)
	observed := resultTestObservation(f.epoch, view)
	require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, observed))
	require.Len(t, prtIORRecords(t, &logs, withheldIORWinnerCondition), 1)
	historical := *f.epoch
	historical.Index--
	f.d.reportAcceptedIOR(f.app, &historical, &types.Log{BlockNumber: 80})
	require.NoError(t, f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view, observed))
	require.Len(t, prtIORRecords(t, &logs, withheldIORWinnerCondition), 1,
		"catching up an older acceptance must not repeat the unchanged current winner")
}
