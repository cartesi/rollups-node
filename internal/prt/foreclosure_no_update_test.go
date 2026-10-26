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

func TestForeclosePendingClaimEpochs_NoUpdateContinuesAndFreshPrefixIsPreserved(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	app := prtForeclosedApp(1, 100)
	stale := &model.Epoch{
		ApplicationID: app.ID, Index: 2, Status: model.EpochStatus_Closed, FirstBlock: 80, LastBlock: 90,
	}
	suffix := &model.Epoch{
		ApplicationID: app.ID, Index: 3, Status: model.EpochStatus_Closed, FirstBlock: 90, LastBlock: 110,
	}
	fresh := *stale
	fresh.Status = model.EpochStatus_InputsProcessed
	filter := repository.EpochFilter{Status: model.NonTerminalEpochStatuses()}
	firstRead := r.On("ListEpochs", mock.Anything, app.Name, filter, repository.Pagination{}, false).
		Return([]*model.Epoch{stale, suffix}, uint64(2), nil).Once()
	noUpdate := r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, stale.Index).
		Return(repository.ErrNoUpdate).Once()
	success := r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, suffix.Index).Return(nil).Once()
	secondRead := r.On("ListEpochs", mock.Anything, app.Name, filter, repository.Pagination{}, false).
		Return([]*model.Epoch{&fresh}, uint64(1), nil).Once()
	mock.InOrder(firstRead, noUpdate, success, secondRead)

	// The conditional update saw a newer row than the CLOSED snapshot. Another
	// eligible epoch must still progress, without claiming the no-op succeeded.
	require.NoError(t, s.foreclosePendingClaimEpochs(t.Context(), app))
	assert.Equal(t, []uint64{suffix.Index}, loggedForeclosedEpochs(t, &logs))

	// On the next observation the healthy, fully pre-foreclosure IP epoch is
	// visible and must retain its validator path without another update or log.
	require.NoError(t, s.foreclosePendingClaimEpochs(t.Context(), app))
	assert.Equal(t, []uint64{suffix.Index}, loggedForeclosedEpochs(t, &logs))
	r.AssertNumberOfCalls(t, "UpdateEpochWithForeclosedClaim", 2)
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
	noUpdate := r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, stale.Index).
		Return(repository.ErrNoUpdate).Once()
	writeFailure := r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, next.Index).Return(dbErr).Once()
	mock.InOrder(noUpdate, writeFailure)

	err := s.foreclosePendingClaimEpochs(t.Context(), app)
	assert.ErrorIs(t, err, dbErr)
	assert.Empty(t, loggedForeclosedEpochs(t, &logs))
}

func TestForeclosePendingClaimEpochs_JoinedNoUpdateAndWriteErrorRemainVisible(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	app := prtForeclosedApp(1, 100)
	dbErr := errors.New("terminalization write failed")
	epoch := &model.Epoch{
		ApplicationID: app.ID, Index: 2, Status: model.EpochStatus_Closed, FirstBlock: 80, LastBlock: 90,
	}
	r.On("ListEpochs", mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, epoch.Index).
		Return(errors.Join(repository.ErrNoUpdate, dbErr)).Once()

	err := s.foreclosePendingClaimEpochs(t.Context(), app)
	assert.ErrorIs(t, err, repository.ErrNoUpdate)
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
