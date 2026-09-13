// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDaveCorruptionReasonIsNotLoggedAgainOnEveryScan(t *testing.T) {
	const head uint64 = 100
	repo := newMockRepository()
	var output bytes.Buffer
	r := &Service{repository: repo}
	r.Logger = slog.New(slog.NewTextHandler(&output, nil))
	app := newDaveAppContracts(newMockInputBox(), nil)
	app.application.LastEpochCheckBlock = head
	app.application.Status = model.ApplicationStatus_OK
	repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(nil, nil).Twice()
	repo.On("UpdateApplicationStatus", t.Context(), app.application.ID, model.ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).Once()

	require.True(t, r.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, head))
	require.Equal(t, model.ApplicationStatus_Corrupted, app.application.Status)
	require.Equal(t, 1, strings.Count(output.String(), "level=ERROR"), "the status helper logs the first corruption once")
	require.Contains(t, output.String(), "no non open epochs found")
	output.Reset()

	require.True(t, r.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, head))
	require.NotContains(t, output.String(), "level=ERROR", "the same terminal reason needs no repeated Error log")
	repo.AssertNotCalled(t, "GetEpoch", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

func TestDaveStatusWriteFailureRemainsVisibleAndRetryable(t *testing.T) {
	const head uint64 = 100
	repo := newMockRepository()
	var output bytes.Buffer
	r := &Service{repository: repo}
	r.Logger = slog.New(slog.NewTextHandler(&output, nil))
	app := newDaveAppContracts(newMockInputBox(), nil)
	app.application.LastEpochCheckBlock = head
	app.application.Status = model.ApplicationStatus_OK
	writeErr := errors.New("status database unavailable")
	repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(nil, nil).Twice()
	repo.On("UpdateApplicationStatus", t.Context(), app.application.ID, model.ApplicationStatus_Corrupted, mock.Anything).
		Return(writeErr).Twice()

	err := r.processApplicationOpenEpoch(t.Context(), app, head)
	require.ErrorIs(t, err, writeErr, "the marker must preserve the actual database error")
	require.ErrorIs(t, err, errApplicationStatusReported)
	require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
	output.Reset()

	require.False(t, r.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, head))
	require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
	require.Equal(t, 2, strings.Count(output.String(), "level=ERROR"), "retain the reason and the failed-write log")
	require.Contains(t, output.String(), "failed to update application status")
	require.Contains(t, output.String(), writeErr.Error())
	repo.AssertNotCalled(t, "GetEpoch", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

func TestDaveTerminalApplicationStillLogsObservationFailure(t *testing.T) {
	const head uint64 = 100
	for _, sealed := range []bool{true, false} {
		name := daveOpenScan
		if sealed {
			name = daveSealedScan
		}
		t.Run(name, func(t *testing.T) {
			repo := newMockRepository()
			dave := newMockDaveConsensus()
			var output bytes.Buffer
			r := &Service{repository: repo}
			r.Logger = slog.New(slog.NewTextHandler(&output, nil))
			app := newDaveAppContracts(newMockInputBox(), dave)
			app.application.LastEpochCheckBlock = head
			app.application.Status = model.ApplicationStatus_Corrupted
			readErr := errors.New("observation temporarily unavailable")
			if sealed {
				app.application.LastEpochCheckBlock--
				dave.On("GetCurrentSealedEpoch", blockRange(head-1, head)).Return(DaveCurrentSealedEpoch{}, readErr).Once()
			} else {
				repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(nil, readErr).Once()
			}

			require.False(t, r.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, head))
			require.Equal(t, 1, strings.Count(output.String(), "level=ERROR"))
			require.Contains(t, output.String(), readErr.Error())
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
			dave.AssertExpectations(t)
		})
	}
}
