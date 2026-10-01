// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package replay

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func TestCompareRecordNonacceptedReports(t *testing.T) {
	t.Parallel()
	for _, status := range model.InputCompletionStatusAllValues {
		if status == model.InputCompletionStatus_Accepted || !status.IsCompleted() {
			continue
		}
		t.Run(status.String(), func(t *testing.T) {
			for _, test := range []struct {
				name    string
				reports [][]byte
				field   string
			}{
				{"matching", [][]byte{[]byte("first"), []byte("second")}, ""},
				{"payload", [][]byte{[]byte("changed"), []byte("second")}, "reports[0]"},
				{"count", [][]byte{[]byte("first")}, "reports.count"},
				{"order", [][]byte{[]byte("second"), []byte("first")}, "reports[0]"},
			} {
				t.Run(test.name, func(t *testing.T) {
					app, record, actual := replayFixture(status, model.Consensus_Authority)
					record.Reports = [][]byte{[]byte("first"), []byte("second")}
					actual.Reports = test.reports
					err := compareRecord(app.Name, app.ID, false, repository.ReplayVerificationFull, record, actual)
					if test.field == "" {
						require.NoError(t, err)
						return
					}
					require.ErrorIs(t, err, ErrContradiction)
					var detail *ContradictionError
					require.ErrorAs(t, err, &detail)
					require.Equal(t, test.field, detail.Field)
				})
			}
		})
	}
}

func TestCompareRecordLegacyReports(t *testing.T) {
	t.Parallel()
	app, record, actual := replayFixture(model.InputCompletionStatus_Rejected, model.Consensus_Authority)
	actual.Reports = [][]byte{[]byte("historically discarded")}
	require.NoError(t, compareRecord(app.Name, app.ID, false, repository.ReplayVerificationCanonical, record, actual))
	var detail *ContradictionError
	err := compareRecord(app.Name, app.ID, false, repository.ReplayVerificationFull, record, actual)
	require.ErrorAs(t, err, &detail)
	require.Equal(t, "reports.count", detail.Field)
	require.Equal(t, "0", detail.Expected)
	require.Equal(t, "1", detail.Actual)
}

func TestRunLegacyReports(t *testing.T) {
	t.Parallel()
	for _, level := range []repository.ReplayVerificationLevel{
		repository.ReplayVerificationCanonical, repository.ReplayVerificationFull,
	} {
		t.Run(level.String(), func(t *testing.T) {
			record := replayRecords(1)[0]
			record.Input.Status = model.InputCompletionStatus_Rejected
			source := &fakeSource{
				summary: model.ReplaySummary{ApplicationID: 7, ProcessedInputs: 1, Consensus: model.Consensus_Authority},
				records: []*model.ReplayRecord{record},
			}
			executor := &fakeExecutor{
				statuses: map[uint64]model.InputCompletionStatus{0: model.InputCompletionStatus_Rejected},
				reports:  map[uint64][][]byte{0: {[]byte("historically discarded")}},
			}
			opts := replayOptions(model.Consensus_Authority, 0, 1)
			opts.Verification = level
			result, err := Run(context.Background(), source, executor, opts)
			if level == repository.ReplayVerificationCanonical {
				require.NoError(t, err)
				require.Equal(t, uint64(1), result.ReplayedInputs)
			} else {
				require.ErrorIs(t, err, ErrContradiction)
				var detail *ContradictionError
				require.ErrorAs(t, err, &detail)
				require.Equal(t, "reports.count", detail.Field)
			}
			require.Nil(t, record.Reports, "replay must leave the source record unchanged")
		})
	}
}
