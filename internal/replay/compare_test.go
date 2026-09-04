// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package replay

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func replayFixture(status model.InputCompletionStatus, consensus model.Consensus) (
	*model.Application,
	*model.ReplayRecord,
	*model.AdvanceResult,
) {
	machineHash := common.HexToHash("0x11")
	txBufferDataBlock := common.HexToHash("0x22")
	app := &model.Application{ID: 7, Name: "replay-app", ConsensusType: consensus}
	record := &model.ReplayRecord{
		Input: model.ReplayInput{
			ApplicationID:     app.ID,
			EpochIndex:        3,
			InputIndex:        9,
			RawData:           []byte("input"),
			Status:            status,
			MachineHash:       &machineHash,
			TxBufferDataBlock: &txBufferDataBlock,
		},
	}
	actual := &model.AdvanceResult{
		EpochIndex: 3,
		InputIndex: 9,
		Status:     status,
		StateProof: model.StateProof{
			MachineHash:       machineHash,
			TxBufferDataBlock: txBufferDataBlock,
		},
	}
	if status == model.InputCompletionStatus_Exception {
		record.Input.ExceptionData = []byte("guest exception")
		actual.ExceptionData = []byte("guest exception")
	}
	if status == model.InputCompletionStatus_Accepted {
		record.Outputs = [][]byte{[]byte("output-a"), []byte("output-b")}
		record.Reports = [][]byte{[]byte("report-a"), []byte("report-b")}
		actual.Outputs = [][]byte{[]byte("output-a"), []byte("output-b")}
		actual.Reports = [][]byte{[]byte("report-a"), []byte("report-b")}
	}
	return app, record, actual
}

func TestCompareRecordCanonicalIgnoresFullEvidence(t *testing.T) {
	app, record, actual := replayFixture(
		model.InputCompletionStatus_Accepted,
		model.Consensus_PRT,
	)
	record.Outputs = [][]byte{[]byte("persisted-output")}
	record.Reports = [][]byte{[]byte("persisted-report")}
	record.StateHashes = []model.ReplayStateHash{{MachineHash: *record.Input.MachineHash, Repetitions: 1}}
	actual.Outputs = [][]byte{[]byte("different-output")}
	actual.Reports = [][]byte{[]byte("different-report")}
	actual.PeriodicStateHashes = [][32]byte{{1}}
	actual.PaddingRepetitions = model.InputHashCollectionCapacity - 1

	require.NoError(t, compareRecord(
		app.Name,
		app.ID,
		app.IsDaveConsensus(),
		repository.ReplayVerificationCanonical,
		record,
		actual,
	))

	actual.MachineHash[0]++
	require.ErrorIs(t, compareRecord(
		app.Name,
		app.ID,
		app.IsDaveConsensus(),
		repository.ReplayVerificationCanonical,
		record,
		actual,
	), ErrContradiction)
}

func TestCompareReplayRecordExceptionData(t *testing.T) {
	t.Parallel()

	t.Run("payload mismatch", func(t *testing.T) {
		app, record, actual := replayFixture(
			model.InputCompletionStatus_Exception,
			model.Consensus_Authority,
		)
		actual.ExceptionData = []byte("different guest exception")
		err := compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual)
		var detail *ContradictionError
		require.ErrorAs(t, err, &detail)
		require.Equal(t, "exception_data", detail.Field)
		require.NotContains(t, err.Error(), "guest exception")
	})

	for _, test := range []struct {
		name   string
		status model.InputCompletionStatus
		mutate func(*model.ReplayRecord, *model.AdvanceResult)
	}{
		{"missing persisted exception payload", model.InputCompletionStatus_Exception, func(r *model.ReplayRecord, _ *model.AdvanceResult) {
			r.Input.ExceptionData = nil
		}},
		{"missing replayed exception payload", model.InputCompletionStatus_Exception, func(_ *model.ReplayRecord, a *model.AdvanceResult) {
			a.ExceptionData = nil
		}},
		{"unexpected persisted payload", model.InputCompletionStatus_Rejected, func(r *model.ReplayRecord, _ *model.AdvanceResult) {
			r.Input.ExceptionData = []byte("unexpected")
		}},
		{"unexpected replayed payload", model.InputCompletionStatus_Rejected, func(_ *model.ReplayRecord, a *model.AdvanceResult) {
			a.ExceptionData = []byte("unexpected")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, record, actual := replayFixture(test.status, model.Consensus_Authority)
			test.mutate(record, actual)
			require.ErrorIs(t,
				compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual),
				ErrContradiction,
			)
		})
	}
}

func TestCompareReplayRecordCompletionMatrix(t *testing.T) {
	t.Parallel()

	statuses := []model.InputCompletionStatus{
		model.InputCompletionStatus_Accepted,
		model.InputCompletionStatus_Rejected,
		model.InputCompletionStatus_Exception,
		model.InputCompletionStatus_MachineHalted,
	}
	consensuses := []model.Consensus{
		model.Consensus_Authority,
		model.Consensus_Quorum,
		model.Consensus_PRT,
	}
	for _, consensus := range consensuses {
		for _, status := range statuses {
			t.Run(consensus.String()+"/"+status.String(), func(t *testing.T) {
				app, record, actual := replayFixture(status, consensus)
				if status != model.InputCompletionStatus_Accepted {
					actual.Outputs = [][]byte{[]byte("noncanonical diagnostic output")}
					actual.Reports = [][]byte{[]byte("noncanonical diagnostic report")}
				}
				if consensus == model.Consensus_PRT {
					checkpoint := common.HexToHash("0x33")
					actual.IsDaveConsensus = true
					actual.PeriodicStateHashes = [][32]byte{checkpoint}
					actual.PaddingRepetitions = model.InputHashCollectionCapacity - 1
					record.StateHashes = []model.ReplayStateHash{
						{Index: 20, MachineHash: checkpoint, Repetitions: 1},
						{Index: 21, MachineHash: actual.MachineHash, Repetitions: model.InputHashCollectionCapacity - 1},
					}
				} else {
					require.False(t, actual.IsDaveConsensus)
					require.Empty(t, actual.PeriodicStateHashes)
					require.Zero(t, actual.PaddingRepetitions)
					require.Empty(t, record.StateHashes)
				}

				require.NoError(t,
					compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual),
				)
				mismatchStatus := model.InputCompletionStatus_Accepted
				if status == model.InputCompletionStatus_Accepted {
					mismatchStatus = model.InputCompletionStatus_Rejected
				}
				actual.Status = mismatchStatus
				require.ErrorIs(t,
					compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual),
					ErrContradiction,
				)
			})
		}
	}
}

func TestCompareReplayRecordAcceptedMutationTable(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name   string
		mutate func(*model.Application, *model.ReplayRecord, *model.AdvanceResult)
	}{
		{"application", func(_ *model.Application, r *model.ReplayRecord, _ *model.AdvanceResult) {
			r.Input.ApplicationID++
		}},
		{"status", func(_ *model.Application, _ *model.ReplayRecord, a *model.AdvanceResult) {
			a.Status = model.InputCompletionStatus_Rejected
		}},
		{"machine-root", func(_ *model.Application, _ *model.ReplayRecord, a *model.AdvanceResult) { a.MachineHash[0]++ }},
		{"tx-buffer-data-block", func(_ *model.Application, _ *model.ReplayRecord, a *model.AdvanceResult) { a.TxBufferDataBlock[0]++ }},
		{"outputs-count", func(_ *model.Application, _ *model.ReplayRecord, a *model.AdvanceResult) {
			a.Outputs = a.Outputs[:1]
		}},
		{"outputs-content", func(_ *model.Application, _ *model.ReplayRecord, a *model.AdvanceResult) {
			a.Outputs[0] = []byte("changed")
		}},
		{"outputs-order", func(_ *model.Application, _ *model.ReplayRecord, a *model.AdvanceResult) {
			a.Outputs[0], a.Outputs[1] = a.Outputs[1], a.Outputs[0]
		}},
		{"reports-count", func(_ *model.Application, _ *model.ReplayRecord, a *model.AdvanceResult) {
			a.Reports = a.Reports[:1]
		}},
		{"reports-content", func(_ *model.Application, _ *model.ReplayRecord, a *model.AdvanceResult) {
			a.Reports[0] = []byte("changed")
		}},
		{"reports-order", func(_ *model.Application, _ *model.ReplayRecord, a *model.AdvanceResult) {
			a.Reports[0], a.Reports[1] = a.Reports[1], a.Reports[0]
		}},
		{"unexpected-hash-collection", func(_ *model.Application, _ *model.ReplayRecord, a *model.AdvanceResult) {
			a.PeriodicStateHashes = append(a.PeriodicStateHashes, common.HexToHash("0x33"))
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			app, record, actual := replayFixture(model.InputCompletionStatus_Accepted, model.Consensus_Authority)
			mutation.mutate(app, record, actual)
			err := compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual)
			require.ErrorIs(t, err, ErrContradiction)
			var detail *ContradictionError
			require.ErrorAs(t, err, &detail)
			require.NotEmpty(t, detail.Field)
			require.NotNil(t, detail.EpochIndex)
			require.Equal(t, uint64(3), *detail.EpochIndex)
			require.Contains(t, err.Error(), "epoch=3")
			require.NotContains(t, err.Error(), "output-a")
		})
	}

}

func TestCompareReplayRecordPersistedRecordValidation(t *testing.T) {
	t.Parallel()

	t.Run("missing machine root", func(t *testing.T) {
		app, record, actual := replayFixture(model.InputCompletionStatus_Rejected, model.Consensus_Authority)
		record.Input.MachineHash = nil
		require.ErrorIs(t,
			compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual),
			ErrContradiction,
		)
	})
	t.Run("missing TX buffer data block", func(t *testing.T) {
		app, record, actual := replayFixture(model.InputCompletionStatus_Rejected, model.Consensus_Authority)
		record.Input.TxBufferDataBlock = nil
		require.ErrorIs(t,
			compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual),
			ErrContradiction,
		)
	})
	t.Run("unknown persisted status", func(t *testing.T) {
		app, record, actual := replayFixture(model.InputCompletionStatus_Rejected, model.Consensus_Authority)
		record.Input.Status = model.InputCompletionStatus("UNKNOWN")
		require.ErrorIs(t,
			compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual),
			ErrContradiction,
		)
	})
	for _, effect := range []string{"output", "report"} {
		t.Run("nonaccepted persisted "+effect, func(t *testing.T) {
			app, record, actual := replayFixture(model.InputCompletionStatus_Rejected, model.Consensus_Authority)
			if effect == "output" {
				record.Outputs = [][]byte{[]byte("illegal")}
			} else {
				record.Reports = [][]byte{[]byte("illegal")}
			}
			// Replay diagnostics are ignored, but persisted effects are corruption.
			actual.Outputs = [][]byte{[]byte("diagnostic")}
			actual.Reports = [][]byte{[]byte("diagnostic")}
			require.ErrorIs(t,
				compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual),
				ErrContradiction,
			)
		})
	}
	t.Run("nonaccepted replay diagnostics ignored", func(t *testing.T) {
		app, record, actual := replayFixture(model.InputCompletionStatus_Exception, model.Consensus_Authority)
		actual.Outputs = [][]byte{[]byte("diagnostic")}
		actual.Reports = [][]byte{[]byte("diagnostic")}
		require.NoError(t, compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual))
	})
}

func TestCompareReplayRecordPRTMutationTable(t *testing.T) {
	t.Parallel()

	fixture := func() (*model.Application, *model.ReplayRecord, *model.AdvanceResult) {
		app, record, actual := replayFixture(model.InputCompletionStatus_Accepted, model.Consensus_PRT)
		hash0 := common.HexToHash("0x31")
		hash1 := common.HexToHash("0x32")
		actual.IsDaveConsensus = true
		actual.PeriodicStateHashes = [][32]byte{hash0, hash1}
		actual.PaddingRepetitions = model.InputHashCollectionCapacity - 2
		record.StateHashes = []model.ReplayStateHash{
			{Index: 40, MachineHash: hash0, Repetitions: 1},
			{Index: 41, MachineHash: hash1, Repetitions: 1},
			{Index: 42, MachineHash: actual.MachineHash, Repetitions: model.InputHashCollectionCapacity - 2},
		}
		return app, record, actual
	}

	app, record, actual := fixture()
	require.NoError(t, compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual))

	mutations := []struct {
		name   string
		mutate func(*model.ReplayRecord, *model.AdvanceResult)
	}{
		{"count", func(r *model.ReplayRecord, _ *model.AdvanceResult) { r.StateHashes = r.StateHashes[:2] }},
		{"hash", func(r *model.ReplayRecord, _ *model.AdvanceResult) { r.StateHashes[0].MachineHash[0]++ }},
		{"order", func(r *model.ReplayRecord, _ *model.AdvanceResult) {
			r.StateHashes[0], r.StateHashes[1] = r.StateHashes[1], r.StateHashes[0]
		}},
		{"index-order", func(r *model.ReplayRecord, _ *model.AdvanceResult) { r.StateHashes[1].Index++ }},
		{"repetition", func(r *model.ReplayRecord, _ *model.AdvanceResult) { r.StateHashes[0].Repetitions++ }},
		{"final-row", func(r *model.ReplayRecord, _ *model.AdvanceResult) { r.StateHashes[2].MachineHash[0]++ }},
		{"padding", func(r *model.ReplayRecord, _ *model.AdvanceResult) { r.StateHashes[2].Repetitions++ }},
		{"result-padding", func(_ *model.ReplayRecord, a *model.AdvanceResult) { a.PaddingRepetitions++ }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			app, record, actual := fixture()
			mutation.mutate(record, actual)
			require.ErrorIs(t,
				compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual),
				ErrContradiction,
			)
		})
	}

	t.Run("diagnostics keep persisted expected and replay actual", func(t *testing.T) {
		app, record, actual := fixture()
		actual.PaddingRepetitions = model.InputHashCollectionCapacity - 1
		err := compareRecord(app.Name, app.ID, app.IsDaveConsensus(), repository.ReplayVerificationFull, record, actual)
		var detail *ContradictionError
		require.ErrorAs(t, err, &detail)
		require.Equal(t, "state_hashes.span", detail.Field)
		require.Equal(t, fmt.Sprint(model.InputHashCollectionCapacity), detail.Expected)
		require.Equal(t, fmt.Sprintf("hashes=2 padding=%d", model.InputHashCollectionCapacity-1), detail.Actual)
	})
}

func TestContradictionErrorIdentity(t *testing.T) {
	t.Parallel()
	t.Run("unknown epoch", func(t *testing.T) {
		err := &ContradictionError{Application: "app", Field: "status"}
		require.True(t, errors.Is(err, ErrContradiction))
		require.Contains(t, err.Error(), "epoch=<unknown>")
	})
	t.Run("known nonzero epoch", func(t *testing.T) {
		epochIndex := uint64(17)
		err := &ContradictionError{
			Application: "app",
			EpochIndex:  &epochIndex,
			Field:       "status",
		}
		require.Contains(t, err.Error(), "epoch=17")
		require.NotContains(t, err.Error(), "epoch=<unknown>")
	})
}
