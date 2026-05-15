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
		Status:              ApplicationStatus_OK,
	}
}

type mockRepo struct {
	lastAppID  int64
	lastStatus ApplicationStatus
	lastReason *string
	err        error
	callCount  int
}

func (m *mockRepo) UpdateApplicationStatus(
	_ context.Context,
	appID int64,
	state ApplicationStatus,
	reason *string,
) error {
	m.callCount++
	m.lastAppID = appID
	m.lastStatus = state
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
	require.Equal(ApplicationStatus_Failed, repo.lastStatus)
	require.NotNil(repo.lastReason)
	require.Equal("machine crashed: OOM", *repo.lastReason)

	// Verify in-memory status was updated.
	require.Equal(ApplicationStatus_Failed, app.Status)
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
	require.Equal(ApplicationStatus_Failed, repo.lastStatus)
	require.NotNil(repo.lastReason)
	require.Equal("epoch 5 input 42: timeout", *repo.lastReason)

	// Verify in-memory status was updated.
	require.Equal(ApplicationStatus_Failed, app.Status)
}

func (s *AppStatusSuite) TestSetDiverged() {
	require := s.Require()
	repo := &mockRepo{}
	logger := slog.Default()
	app := newTestApp()

	err := SetDiverged(context.Background(), logger, repo, app, "hash mismatch: 0xaa != 0xbb")

	// SetDiverged always returns a non-nil error (DIVERGED is terminal)
	require.Error(err)
	require.Contains(err.Error(), "hash mismatch: 0xaa != 0xbb")
	require.Equal(1, repo.callCount)
	require.Equal(int64(42), repo.lastAppID)
	require.Equal(ApplicationStatus_Diverged, repo.lastStatus)
	require.NotNil(repo.lastReason)
	require.Equal("hash mismatch: 0xaa != 0xbb", *repo.lastReason)

	// Verify in-memory status was updated.
	require.Equal(ApplicationStatus_Diverged, app.Status)
	require.NotNil(app.Reason)
	require.Equal("hash mismatch: 0xaa != 0xbb", *app.Reason)
}

func (s *AppStatusSuite) TestSetCorrupted() {
	require := s.Require()
	repo := &mockRepo{}
	logger := slog.Default()
	app := newTestApp()

	err := SetCorrupted(context.Background(), logger, repo, app, "machine snapshot missing")

	// SetCorrupted always returns a non-nil error (CORRUPTED is terminal)
	require.Error(err)
	require.Contains(err.Error(), "machine snapshot missing")
	require.Equal(1, repo.callCount)
	require.Equal(int64(42), repo.lastAppID)
	require.Equal(ApplicationStatus_Corrupted, repo.lastStatus)
	require.NotNil(repo.lastReason)
	require.Equal("machine snapshot missing", *repo.lastReason)

	// Verify in-memory status was updated.
	require.Equal(ApplicationStatus_Corrupted, app.Status)
	require.NotNil(app.Reason)
	require.Equal("machine snapshot missing", *app.Reason)
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
	require.Equal(ApplicationStatus_Failed, repo.lastStatus)
	require.NotNil(repo.lastReason)
	require.Equal("process crashed", *repo.lastReason)

	// In-memory status must NOT be updated on DB error to stay consistent.
	require.Equal(ApplicationStatus_OK, app.Status)
	require.Nil(app.Reason)
}

func (s *AppStatusSuite) TestSetDivergedDBError() {
	require := s.Require()
	dbErr := errors.New("db connection failed")
	repo := &mockRepo{err: dbErr}
	logger := slog.Default()
	app := newTestApp()

	err := SetDiverged(context.Background(), logger, repo, app, "claim disagreement")

	require.ErrorIs(err, dbErr)
	require.Contains(err.Error(), "claim disagreement")
	require.Equal(1, repo.callCount)
	require.Equal(ApplicationStatus_Diverged, repo.lastStatus)

	// In-memory status must NOT be updated on DB error to stay consistent.
	require.Equal(ApplicationStatus_OK, app.Status)
	require.Nil(app.Reason)
}

func (s *AppStatusSuite) TestSetCorruptedDBError() {
	require := s.Require()
	dbErr := errors.New("db connection failed")
	repo := &mockRepo{err: dbErr}
	logger := slog.Default()
	app := newTestApp()

	err := SetCorrupted(context.Background(), logger, repo, app, "state corruption")

	require.ErrorIs(err, dbErr)
	require.Contains(err.Error(), "state corruption")
	require.Equal(1, repo.callCount)
	require.Equal(ApplicationStatus_Corrupted, repo.lastStatus)

	// In-memory status must NOT be updated on DB error to stay consistent.
	require.Equal(ApplicationStatus_OK, app.Status)
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

func (s *AppStatusSuite) TestSetDivergedf() {
	require := s.Require()
	repo := &mockRepo{}
	logger := slog.Default()
	app := newTestApp()

	err := SetDivergedf(context.Background(), logger, repo, app,
		"epoch %d: hash mismatch %s != %s", 5, "0xaa", "0xbb")

	// SetDivergedf always returns a non-nil error (DIVERGED is terminal)
	require.Error(err)
	require.Contains(err.Error(), "epoch 5: hash mismatch 0xaa != 0xbb")
	require.Equal(1, repo.callCount)
	require.Equal(ApplicationStatus_Diverged, repo.lastStatus)
	require.NotNil(repo.lastReason)
	require.Equal("epoch 5: hash mismatch 0xaa != 0xbb", *repo.lastReason)

	// Verify in-memory status was updated (DB succeeded).
	require.Equal(ApplicationStatus_Diverged, app.Status)
	require.NotNil(app.Reason)
	require.Equal("epoch 5: hash mismatch 0xaa != 0xbb", *app.Reason)
}

func (s *AppStatusSuite) TestSetCorruptedf() {
	require := s.Require()
	repo := &mockRepo{}
	logger := slog.Default()
	app := newTestApp()

	err := SetCorruptedf(context.Background(), logger, repo, app,
		"epoch %d: missing snapshot", 7)

	// SetCorruptedf always returns a non-nil error (CORRUPTED is terminal)
	require.Error(err)
	require.Contains(err.Error(), "epoch 7: missing snapshot")
	require.Equal(1, repo.callCount)
	require.Equal(ApplicationStatus_Corrupted, repo.lastStatus)
	require.NotNil(repo.lastReason)
	require.Equal("epoch 7: missing snapshot", *repo.lastReason)

	// Verify in-memory status was updated (DB succeeded).
	require.Equal(ApplicationStatus_Corrupted, app.Status)
	require.NotNil(app.Reason)
	require.Equal("epoch 7: missing snapshot", *app.Reason)
}

func (s *AppStatusSuite) TestSetDivergedfDBError() {
	require := s.Require()
	dbErr := errors.New("db connection failed")
	repo := &mockRepo{err: dbErr}
	logger := slog.Default()
	app := newTestApp()

	err := SetDivergedf(context.Background(), logger, repo, app, "reason: %s", "test")

	require.Error(err)
	require.ErrorIs(err, dbErr)
	require.Contains(err.Error(), "reason: test")

	// In-memory status must NOT be updated on DB error.
	require.Equal(ApplicationStatus_OK, app.Status)
	require.Nil(app.Reason)
}

func (s *AppStatusSuite) TestSetCorruptedfDBError() {
	require := s.Require()
	dbErr := errors.New("db connection failed")
	repo := &mockRepo{err: dbErr}
	logger := slog.Default()
	app := newTestApp()

	err := SetCorruptedf(context.Background(), logger, repo, app, "reason: %s", "test")

	require.Error(err)
	require.ErrorIs(err, dbErr)
	require.Contains(err.Error(), "reason: test")

	// In-memory status must NOT be updated on DB error.
	require.Equal(ApplicationStatus_OK, app.Status)
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
	require.Equal(ApplicationStatus_Failed, repo.lastStatus)
	require.NotNil(repo.lastReason)
	require.Equal("input 7: crash", *repo.lastReason)
}

// captureHandler is an slog.Handler that records every emitted Record so
// tests can assert on log output. It is concurrency-safe enough for
// single-goroutine test scenarios.
type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

// findRecord returns the first record whose message equals msg, or nil.
func findRecord(records []slog.Record, msg string) *slog.Record {
	for i := range records {
		if records[i].Message == msg {
			return &records[i]
		}
	}
	return nil
}

// attrValue extracts the value of a named attribute from a record, or nil.
func attrValue(r *slog.Record, key string) any {
	var found any
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = a.Value.Any()
			return false
		}
		return true
	})
	return found
}

// TestSetDivergedDBErrorLogsBothLines asserts the logging contract
// documented on SetDiverged: when the DB write fails, BOTH the "marking
// application as diverged" line and the "failed to update application
// status" line are emitted at ERROR level. This is the invariant that lets
// callers discard the returned error with `_ =` without losing operator
// visibility into the DB failure.
func (s *AppStatusSuite) TestSetDivergedDBErrorLogsBothLines() {
	require := s.Require()
	dbErr := errors.New("db connection failed")
	repo := &mockRepo{err: dbErr}
	handler := &captureHandler{}
	logger := slog.New(handler)
	app := newTestApp()

	err := SetDiverged(context.Background(), logger, repo, app, "claim disagreement")
	require.ErrorIs(err, dbErr)

	transition := findRecord(handler.records, "marking application as diverged (terminal)")
	require.NotNil(transition, "transition log line must fire even on DB failure")
	require.Equal(slog.LevelError, transition.Level)
	require.Equal("claim disagreement", attrValue(transition, "reason"))

	dbFailure := findRecord(handler.records, "failed to update application status")
	require.NotNil(dbFailure, "DB-failure log line must fire so operators see the persist error")
	require.Equal(slog.LevelError, dbFailure.Level)
	loggedErr, ok := attrValue(dbFailure, "error").(error)
	require.True(ok, "error attr must be an error value")
	require.ErrorIs(loggedErr, dbErr)
}

// TestSetCorruptedDBErrorLogsBothLines mirrors the diverged logging contract
// for the CORRUPTED terminal status.
func (s *AppStatusSuite) TestSetCorruptedDBErrorLogsBothLines() {
	require := s.Require()
	dbErr := errors.New("db connection failed")
	repo := &mockRepo{err: dbErr}
	handler := &captureHandler{}
	logger := slog.New(handler)
	app := newTestApp()

	err := SetCorrupted(context.Background(), logger, repo, app, "state corruption")
	require.ErrorIs(err, dbErr)

	transition := findRecord(handler.records, "marking application as corrupted (terminal)")
	require.NotNil(transition, "transition log line must fire even on DB failure")
	require.Equal(slog.LevelError, transition.Level)
	require.Equal("state corruption", attrValue(transition, "reason"))

	dbFailure := findRecord(handler.records, "failed to update application status")
	require.NotNil(dbFailure, "DB-failure log line must fire so operators see the persist error")
	require.Equal(slog.LevelError, dbFailure.Level)
	loggedErr, ok := attrValue(dbFailure, "error").(error)
	require.True(ok, "error attr must be an error value")
	require.ErrorIs(loggedErr, dbErr)
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
