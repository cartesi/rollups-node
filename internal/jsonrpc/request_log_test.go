// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/stretchr/testify/require"
)

const logRequestID = "11111111-1111-4111-8111-111111111111"

type failedApplicationRepository struct{ repository.Repository }

func (*failedApplicationRepository) ListApplications(
	context.Context, repository.ApplicationFilter, repository.Pagination, bool,
) ([]*model.Application, uint64, error) {
	return nil, 0, errors.New("SAFE_REPOSITORY_FAILURE")
}

func requireRequestLogs(t *testing.T, logs *bytes.Buffer, levels []slog.Level) {
	t.Helper()
	decoder := json.NewDecoder(logs)
	var records []map[string]any
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		records = append(records, record)
	}
	require.Len(t, records, len(levels), "adding the request ID must not add log events")
	for i, record := range records {
		require.Equal(t, levels[i].String(), record["level"])
		require.Equal(t, logRequestID, record["request_id"])
	}
}

func serveWithLogRequestID(s *Service, w http.ResponseWriter, body string) {
	r := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(body))
	r.Header.Set("X-Request-ID", logRequestID)
	service.RequestIDMiddleware(http.HandlerFunc(s.handleRPC)).ServeHTTP(w, r)
}

func TestRepositoryFailureLogsRequestID(t *testing.T) {
	var logs bytes.Buffer
	s := newBatchTestService()
	s.repository = &failedApplicationRepository{}
	s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	recorder := httptest.NewRecorder()
	serveWithLogRequestID(s, recorder,
		`{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":1},"id":1}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, logRequestID, recorder.Header().Get("X-Request-ID"))
	response := decodeRPCResponse(t, recorder.Body.Bytes())
	requireRPCError(t, response, float64(1), JSONRPC_INTERNAL_ERROR)
	require.Equal(t, "Internal server error", response.Error.Message)
	require.NotContains(t, recorder.Body.String(), "SAFE_REPOSITORY_FAILURE")
	require.Contains(t, logs.String(), "SAFE_REPOSITORY_FAILURE", "the cause must stay in the server log")
	requireRequestLogs(t, &logs, []slog.Level{slog.LevelInfo, slog.LevelError})

	// The service logger itself must not retain a request's attributes.
	s.Logger.Info("outside the request")
	require.NotContains(t, logs.String(), "request_id")
}

type failedResponseWriter struct{ *httptest.ResponseRecorder }

func (failedResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("SAFE_RESPONSE_WRITE_FAILURE")
}

func TestDispatchAndWriteFailureLogsRequestID(t *testing.T) {
	for _, test := range []struct {
		name        string
		handler     rpcHandler
		levels      []slog.Level
		failedWrite bool
	}{
		{name: "unexpected error", levels: []slog.Level{slog.LevelInfo, slog.LevelError},
			handler: func(*Service, *http.Request, RPCRequest) (any, error) {
				return nil, errors.New("SAFE_DISPATCH_FAILURE")
			}},
		{name: "panic", levels: []slog.Level{slog.LevelInfo, slog.LevelError},
			handler: func(*Service, *http.Request, RPCRequest) (any, error) { panic("SAFE_DISPATCH_PANIC") }},
		{name: "encode error", levels: []slog.Level{slog.LevelInfo, slog.LevelError},
			handler: func(*Service, *http.Request, RPCRequest) (any, error) { return make(chan int), nil }},
		{name: "timeout", levels: []slog.Level{slog.LevelInfo, slog.LevelWarn},
			handler: func(_ *Service, r *http.Request, _ RPCRequest) (any, error) {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}},
		{name: "expected domain error", levels: []slog.Level{slog.LevelInfo},
			handler: func(*Service, *http.Request, RPCRequest) (any, error) {
				return nil, newRPCError(JSONRPC_APPLICATION_NOT_FOUND, "Application not found")
			}},
		{name: "response write", levels: []slog.Level{slog.LevelInfo, slog.LevelWarn}, failedWrite: true,
			handler: func(*Service, *http.Request, RPCRequest) (any, error) { return true, nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			s := newBatchTestService()
			s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			if test.name == "timeout" {
				s.dispatchTimeout = 100 * time.Millisecond
			}
			withTestRPCHandler(t, s, "test_request_log", test.handler)
			recorder := httptest.NewRecorder()
			var writer http.ResponseWriter = recorder
			if test.failedWrite {
				writer = failedResponseWriter{recorder}
			}
			serveWithLogRequestID(s, writer, `{"jsonrpc":"2.0","method":"test_request_log","id":1}`)
			requireRequestLogs(t, &logs, test.levels)
		})
	}
}
