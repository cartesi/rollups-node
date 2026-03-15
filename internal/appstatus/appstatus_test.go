// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package appstatus

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
)

func TestAppStatus(t *testing.T) {
	suite.Run(t, new(AppStatusSuite))
}

type AppStatusSuite struct{ suite.Suite }

func newTestApp() *Application {
	return &Application{
		ID:                  42,
		Name:                "test-app",
		IApplicationAddress: common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678"),
		IConsensusAddress:   common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		IInputBoxAddress:    common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Enabled:             true,
		Health:              ApplicationHealth_Running,
	}
}

type mockRepo struct {
	lastAppID  int64
	lastState  ApplicationState
	lastReason *string
	err        error
	callCount  int
}

func (m *mockRepo) UpdateApplicationState(
	_ context.Context,
	appID int64,
	state ApplicationState,
	reason *string,
) error {
	m.callCount++
	m.lastAppID = appID
	m.lastState = state
	m.lastReason = reason
	return m.err
}

func (s *AppStatusSuite) TestSetFailed() {
	require := s.Require()
	repo := &mockRepo{}
	logger := slog.Default()
	app := newTestApp()

	err := SetFailed(context.Background(), logger, repo, app, "machine crashed: OOM")

	require.NoError(err)
	require.Equal(1, repo.callCount)
	require.Equal(int64(42), repo.lastAppID)
	require.Equal(ApplicationState_Failed, repo.lastState)
	require.NotNil(repo.lastReason)
	require.Equal("machine crashed: OOM", *repo.lastReason)

	// Verify in-memory state was updated
	require.Equal(ApplicationState_Failed, app.Health)
	require.NotNil(app.Reason)
	require.Equal("machine crashed: OOM", *app.Reason)
}

func (s *AppStatusSuite) TestSetFailedf() {
	require := s.Require()
	repo := &mockRepo{}
	logger := slog.Default()
	app := newTestApp()

	err := SetFailedf(context.Background(), logger, repo, app,
		"epoch %d input %d: %s", 5, 42, "timeout")

	require.NoError(err)
	require.Equal(1, repo.callCount)
	require.Equal(ApplicationState_Failed, repo.lastState)
	require.NotNil(repo.lastReason)
	require.Equal("epoch 5 input 42: timeout", *repo.lastReason)

	// Verify in-memory state was updated
	require.Equal(ApplicationState_Failed, app.Health)
}

func (s *AppStatusSuite) TestSetInoperable() {
	require := s.Require()
	repo := &mockRepo{}
	logger := slog.Default()
	app := newTestApp()

	err := SetInoperable(context.Background(), logger, repo, app, "hash mismatch: 0xaa != 0xbb")

	// SetInoperable always returns a non-nil error (INOPERABLE is terminal)
	require.Error(err)
	require.Contains(err.Error(), "hash mismatch: 0xaa != 0xbb")
	require.Equal(1, repo.callCount)
	require.Equal(int64(42), repo.lastAppID)
	require.Equal(ApplicationState_Inoperable, repo.lastState)
	require.NotNil(repo.lastReason)
	require.Equal("hash mismatch: 0xaa != 0xbb", *repo.lastReason)

	// Verify in-memory state was updated
	require.Equal(ApplicationState_Inoperable, app.Health)
	require.NotNil(app.Reason)
	require.Equal("hash mismatch: 0xaa != 0xbb", *app.Reason)
}

func (s *AppStatusSuite) TestSetFailedDBError() {
	require := s.Require()
	dbErr := errors.New("db connection failed")
	repo := &mockRepo{err: dbErr}
	logger := slog.Default()
	app := newTestApp()

	err := SetFailed(context.Background(), logger, repo, app, "process crashed")

	require.ErrorIs(err, dbErr)
	require.Equal(1, repo.callCount)
	require.Equal(ApplicationState_Failed, repo.lastState)
	require.NotNil(repo.lastReason)
	require.Equal("process crashed", *repo.lastReason)

	// In-memory state must NOT be updated on DB error to stay consistent
	require.Equal(ApplicationState_Enabled, app.Health)
	require.Nil(app.Reason)
}

func (s *AppStatusSuite) TestSetInoperableDBError() {
	require := s.Require()
	dbErr := errors.New("db connection failed")
	repo := &mockRepo{err: dbErr}
	logger := slog.Default()
	app := newTestApp()

	err := SetInoperable(context.Background(), logger, repo, app, "state corruption")

	require.ErrorIs(err, dbErr)
	require.Contains(err.Error(), "state corruption")
	require.Equal(1, repo.callCount)
	require.Equal(ApplicationState_Inoperable, repo.lastState)

	// In-memory state must NOT be updated on DB error to stay consistent
	require.Equal(ApplicationState_Enabled, app.Health)
	require.Nil(app.Reason)
}

func (s *AppStatusSuite) TestReasonStoredExactly() {
	require := s.Require()
	repo := &mockRepo{}
	logger := slog.Default()

	err := SetFailed(context.Background(), logger, repo, newTestApp(), "epoch 5 input 42: timeout")

	require.NoError(err)
	require.NotNil(repo.lastReason)
	require.Equal("epoch 5 input 42: timeout", *repo.lastReason)
}

func (s *AppStatusSuite) TestSetInoperablef() {
	require := s.Require()
	repo := &mockRepo{}
	logger := slog.Default()
	app := newTestApp()

	err := SetInoperablef(context.Background(), logger, repo, app,
		"epoch %d: hash mismatch %s != %s", 5, "0xaa", "0xbb")

	// SetInoperablef always returns a non-nil error (INOPERABLE is terminal)
	require.Error(err)
	require.Contains(err.Error(), "epoch 5: hash mismatch 0xaa != 0xbb")
	require.Equal(1, repo.callCount)
	require.Equal(ApplicationState_Inoperable, repo.lastState)
	require.NotNil(repo.lastReason)
	require.Equal("epoch 5: hash mismatch 0xaa != 0xbb", *repo.lastReason)

	// Verify in-memory state was updated (DB succeeded)
	require.Equal(ApplicationState_Inoperable, app.Health)
	require.NotNil(app.Reason)
	require.Equal("epoch 5: hash mismatch 0xaa != 0xbb", *app.Reason)
}

func (s *AppStatusSuite) TestSetInoperablefDBError() {
	require := s.Require()
	dbErr := errors.New("db connection failed")
	repo := &mockRepo{err: dbErr}
	logger := slog.Default()
	app := newTestApp()

	err := SetInoperablef(context.Background(), logger, repo, app, "reason: %s", "test")

	require.Error(err)
	require.ErrorIs(err, dbErr)
	require.Contains(err.Error(), "reason: test")

	// In-memory state must NOT be updated on DB error
	require.Equal(ApplicationState_Enabled, app.Health)
	require.Nil(app.Reason)
}

func (s *AppStatusSuite) TestSetFailedfDBError() {
	require := s.Require()
	dbErr := errors.New("db connection failed")
	repo := &mockRepo{err: dbErr}
	logger := slog.Default()

	err := SetFailedf(context.Background(), logger, repo, newTestApp(),
		"input %d: %s", 7, "crash")

	require.ErrorIs(err, dbErr)
	require.Equal(1, repo.callCount)
	require.Equal(ApplicationState_Failed, repo.lastState)
	require.NotNil(repo.lastReason)
	require.Equal("input 7: crash", *repo.lastReason)
}

func (s *AppStatusSuite) TestReasonTruncation() {
	require := s.Require()
	repo := &mockRepo{}
	logger := slog.Default()
	app := newTestApp()

	// Create a reason string longer than maxReasonLength
	longReason := strings.Repeat("x", maxReasonLength+500)

	err := SetFailed(context.Background(), logger, repo, app, longReason)

	require.NoError(err)
	require.NotNil(repo.lastReason)
	require.LessOrEqual(len(*repo.lastReason), maxReasonLength+len("... (truncated)"))
	require.True(strings.HasSuffix(*repo.lastReason, "... (truncated)"))

	// Short reasons should pass through unchanged
	repo2 := &mockRepo{}
	app2 := newTestApp()
	err = SetFailed(context.Background(), logger, repo2, app2, "short reason")
	require.NoError(err)
	require.NotNil(repo2.lastReason)
	require.Equal("short reason", *repo2.lastReason)
}
