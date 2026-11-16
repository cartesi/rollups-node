// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func TestForeclosePendingClaimEpochs_NoUpdateContinuesAndRereadsNextPass(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	app := prtForeclosedApp(1, 100)
	pending := checkpointEpoch(2, "0x200")
	pending.ApplicationID = app.ID
	open := &model.Epoch{ApplicationID: app.ID, Index: 3, Status: model.EpochStatus_Open, FirstBlock: 90}
	filter := repository.EpochFilter{Status: model.NonTerminalEpochStatuses()}
	firstRead := r.On("ListEpochs", mock.Anything, app.Name, filter, repository.Pagination{}, false).
		Return([]*model.Epoch{pending, open}, uint64(2), nil).Once()
	noUpdate := r.On("ForeclosePRTEpoch", mock.Anything, app.ID, pending.Index, uint64(100)).
		Return(repository.ErrNoUpdate).Once()
	success := r.On("ForeclosePRTEpoch", mock.Anything, app.ID, open.Index, uint64(100)).Return(nil).Once()
	secondRead := r.On("ListEpochs", mock.Anything, app.Name, filter, repository.Pagination{}, false).
		Return([]*model.Epoch{pending}, uint64(1), nil).Once()
	ready := r.On("ForeclosePRTEpoch", mock.Anything, app.ID, pending.Index, uint64(101)).Return(nil).Once()
	mock.InOrder(firstRead, noUpdate, success, secondRead, ready)

	// A refused target must not stop an independent unsealed exit or claim
	// success. A later pass obtains a new page and supplies its actual head.
	require.NoError(t, s.foreclosePendingClaimEpochs(t.Context(), app, 100))
	assert.Equal(t, []uint64{open.Index}, loggedForeclosedEpochs(t, &logs))
	require.NoError(t, s.foreclosePendingClaimEpochs(t.Context(), app, 101))
	assert.Equal(t, []uint64{open.Index, pending.Index}, loggedForeclosedEpochs(t, &logs))
	r.AssertNumberOfCalls(t, "ForeclosePRTEpoch", 3)
}

func TestForeclosePendingClaimEpochs_NoUpdateDoesNotHideLaterWriteError(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	app := prtForeclosedApp(1, 100)
	dbErr := errors.New("terminalization write failed")
	stale := &model.Epoch{
		ApplicationID: app.ID, Index: 2, Status: model.EpochStatus_Closed, FirstBlock: 80, LastBlock: 90,
	}
	next := &model.Epoch{
		ApplicationID: app.ID, Index: 3, Status: model.EpochStatus_Closed, FirstBlock: 90, LastBlock: 110,
	}
	r.On("ListEpochs", mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{stale, next}, uint64(2), nil).Once()
	noUpdate := r.On("ForeclosePRTEpoch", mock.Anything, app.ID, stale.Index, mock.Anything).
		Return(repository.ErrNoUpdate).Once()
	writeFailure := r.On("ForeclosePRTEpoch", mock.Anything, app.ID, next.Index, mock.Anything).Return(dbErr).Once()
	mock.InOrder(noUpdate, writeFailure)

	err := s.foreclosePendingClaimEpochs(t.Context(), app, app.ForecloseBlock)
	assert.ErrorIs(t, err, dbErr)
	assert.Empty(t, loggedForeclosedEpochs(t, &logs))
}

func loggedForeclosedEpochs(t *testing.T, logs *bytes.Buffer) []uint64 {
	t.Helper()
	var epochs []uint64
	for _, line := range bytes.Split(logs.Bytes(), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record struct {
			Message    string `json:"msg"`
			Level      string `json:"level"`
			EpochIndex uint64 `json:"epoch_index"`
		}
		require.NoError(t, json.Unmarshal(line, &record))
		if record.Message == "Terminalized unaccepted epoch of foreclosed application" {
			assert.Equal(t, "INFO", record.Level)
			epochs = append(epochs, record.EpochIndex)
		}
	}
	return epochs
}
