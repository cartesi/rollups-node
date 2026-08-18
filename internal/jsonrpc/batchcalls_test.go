// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/stretchr/testify/require"
)

const (
	testBatchSize           = 100
	testBatchSuccessCount   = 10
	testLargeResultSize     = 1<<20 - 38 // 1 MB - `,{"jsonrpc":"2.0","result":"...","id":??}`
	testResponseBudgetSlack = 1 << 20
)

func serveRPC(t *testing.T, s *Service, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleRPC(rr, req)
	return rr
}

func newBatchTestService() *Service {
	return &Service{
		Service: service.Service{
			Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		},
	}
}

func decodeRPCResponse(t *testing.T, body []byte) RPCResponse {
	t.Helper()
	var response RPCResponse
	require.NoError(t, json.Unmarshal(body, &response))
	return response
}

func decodeRPCBatch(t *testing.T, body []byte) []RPCResponse {
	t.Helper()
	var responses []RPCResponse
	require.NoError(t, json.Unmarshal(body, &responses))
	return responses
}

func requireRPCError(t *testing.T, response RPCResponse, id any, code int) {
	t.Helper()
	require.Equal(t, "2.0", response.JSONRPC)
	require.Equal(t, id, response.ID)
	require.NotNil(t, response.Error)
	require.Equal(t, code, response.Error.Code)
}

func TestListOutputsRejectsEmptyOutputTypeList(t *testing.T) {
	s := newBatchTestService()
	rr := serveRPC(t, s, []byte(`{
		"jsonrpc":"2.0",
		"method":"cartesi_listOutputs",
		"params":{"application":"app","output_type":[]},
		"id":1
	}`))

	require.Equal(t, http.StatusOK, rr.Code)
	response := decodeRPCResponse(t, rr.Body.Bytes())
	requireRPCError(t, response, float64(1), JSONRPC_INVALID_PARAMS)
	require.Equal(t, "Invalid output type: expected at least one selector", response.Error.Message)
}

func TestJSONRPCBatchRejectsEmptyBatchWithSingleObject(t *testing.T) {
	s := newBatchTestService()
	rr := serveRPC(t, s, []byte(`[]`))

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	requireRPCError(t, decodeRPCResponse(t, rr.Body.Bytes()), nil, JSONRPC_INVALID_BATCH)

	var array []RPCResponse
	require.Error(t, json.Unmarshal(rr.Body.Bytes(), &array),
		"an empty batch error must be one JSON-RPC object, not an array")
}

func TestJSONRPCBatchRejectsMoreThanMaximumBeforeDispatch(t *testing.T) {
	s := newBatchTestService()
	var calls atomic.Int32
	const method = "test_batch_cap"
	withTestRPCHandler(t, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		calls.Add(1)
		return true, nil
	})

	requests := make([]json.RawMessage, testBatchSize+1)
	for i := range requests {
		requests[i] = json.RawMessage(fmt.Sprintf(
			`{"jsonrpc":"2.0","method":%q,"id":%d}`, method, i))
	}
	body, err := json.Marshal(requests)
	require.NoError(t, err)
	rr := serveRPC(t, s, body)

	require.Equal(t, http.StatusOK, rr.Code)
	requireRPCError(t, decodeRPCResponse(t, rr.Body.Bytes()), nil, JSONRPC_INVALID_BATCH)
	require.Zero(t, calls.Load(), "an oversized batch must be rejected before dispatch")
}

func TestJSONRPCMalformedBatchReturnsParseErrorObject(t *testing.T) {
	s := newBatchTestService()
	rr := serveRPC(t, s, []byte(`[{"jsonrpc":"2.0","method":"rpc.discover","id":1},`))

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	requireRPCError(t, decodeRPCResponse(t, rr.Body.Bytes()), nil, JSONRPC_PARSE_ERROR)
}

func TestJSONRPCBatchMalformedElementDoesNotPoisonValidSiblings(t *testing.T) {
	s := newBatchTestService()
	body := []byte(`[
		{"jsonrpc":"2.0","method":"cartesi_getNodeVersion","id":1},
		17,
		{"jsonrpc":"2.0","method":"cartesi_getNodeVersion","id":3}
	]`)
	rr := serveRPC(t, s, body)

	require.Equal(t, http.StatusOK, rr.Code)
	responses := decodeRPCBatch(t, rr.Body.Bytes())
	require.Len(t, responses, 3)
	require.Nil(t, responses[0].Error)
	require.EqualValues(t, 1, responses[0].ID)
	requireRPCError(t, responses[1], nil, JSONRPC_INVALID_REQUEST)
	require.Nil(t, responses[2].Error)
	require.EqualValues(t, 3, responses[2].ID)
}

func TestJSONRPCBatchStructurallyInvalidElementsDoNotPoisonValidSiblings(t *testing.T) {
	tests := map[string]struct {
		request string
		id      any
	}{
		"null":            {request: `null`},
		"empty object":    {request: `{}`},
		"missing method":  {request: `{"jsonrpc":"2.0","id":2}`, id: float64(2)},
		"invalid version": {request: `{"jsonrpc":"1.0","method":"cartesi_getNodeVersion","id":2}`, id: float64(2)},
		"invalid id":      {request: `{"jsonrpc":"2.0","method":"cartesi_getNodeVersion","id":true}`},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			s := newBatchTestService()
			body := []byte(fmt.Sprintf(`[
				{"jsonrpc":"2.0","method":"cartesi_getNodeVersion","id":1},
				%s,
				{"jsonrpc":"2.0","method":"cartesi_getNodeVersion","id":3}
			]`, test.request))
			rr := serveRPC(t, s, body)

			require.Equal(t, http.StatusOK, rr.Code)
			responses := decodeRPCBatch(t, rr.Body.Bytes())
			require.Len(t, responses, 3)

			require.Nil(t, responses[0].Error)
			require.EqualValues(t, 1, responses[0].ID)

			requireRPCError(t, responses[1], test.id, JSONRPC_INVALID_REQUEST)

			require.Nil(t, responses[2].Error)
			require.EqualValues(t, 3, responses[2].ID)
		})
	}
}

func TestJSONRPCValidationErrorsEchoValidID(t *testing.T) {
	s := newBatchTestService()
	tests := map[string]string{
		"missing method":  `{"jsonrpc":"2.0","id":"request-id"}`,
		"invalid version": `{"jsonrpc":"1.0","method":"cartesi_getNodeVersion","id":42}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			response := decodeRPCResponse(t, serveRPC(t, s, []byte(body)).Body.Bytes())
			expectedID := any("request-id")
			if name == "invalid version" {
				expectedID = float64(42)
			}
			requireRPCError(t, response, expectedID, JSONRPC_INVALID_REQUEST)
		})
	}
}

func TestJSONRPCRejectsInvalidIDTypesWithNullID(t *testing.T) {
	s := newBatchTestService()
	for name, id := range map[string]string{
		"boolean": `true`,
		"array":   `[]`,
		"object":  `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(
				`{"jsonrpc":"2.0","method":"cartesi_getNodeVersion","id":%s}`, id))
			response := decodeRPCResponse(t, serveRPC(t, s, body).Body.Bytes())
			requireRPCError(t, response, nil, JSONRPC_INVALID_REQUEST)
		})
	}
}

func TestJSONRPCBatchNotificationsReceiveNullIDResponses(t *testing.T) {
	s := newBatchTestService()
	rr := serveRPC(t, s, []byte(`[
		{"jsonrpc":"2.0","method":"cartesi_getNodeVersion"},
		{"jsonrpc":"2.0","method":"does_not_exist"}
	]`))

	require.Equal(t, http.StatusOK, rr.Code)
	responses := decodeRPCBatch(t, rr.Body.Bytes())
	require.Len(t, responses, 2, "notifications are deliberately answered by this server")
	require.Nil(t, responses[0].ID)
	require.Nil(t, responses[0].Error)
	requireRPCError(t, responses[1], nil, JSONRPC_METHOD_NOT_FOUND)
}

func TestJSONRPCBatchAlwaysReturnsHTTP200ForJSONErrors(t *testing.T) {
	s := newBatchTestService()
	tests := map[string][]byte{
		"parse error":     []byte(`[nope`),
		"invalid request": []byte(`[]`),
		"error entries":   []byte(`[false,{"jsonrpc":"2.0","method":"does_not_exist","id":2}]`),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			rr := serveRPC(t, s, body)
			require.Equal(t, http.StatusOK, rr.Code)
			require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
			require.True(t, json.Valid(rr.Body.Bytes()))
		})
	}
}

func TestJSONRPCBatchReplacesResponsesAtCumulativeResponseBudget(t *testing.T) {
	s := newBatchTestService()
	var calls atomic.Int32
	const method = "test_large_batch_result"
	largeResult := strings.Repeat("x", testLargeResultSize)
	withTestRPCHandler(t, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		calls.Add(1)
		return largeResult, nil
	})

	requests := make([]json.RawMessage, testBatchSize)
	for i := range requests {
		requests[i] = json.RawMessage(fmt.Sprintf(
			`{"jsonrpc":"2.0","method":%q,"params":{"limit":10000},"id":%d}`, method, i))
	}
	body, err := json.Marshal(requests)
	require.NoError(t, err)
	require.Less(t, len(body), 10<<10, "the request cap must not be mistaken for a response cap")
	rr := serveRPC(t, s, body)

	require.Equal(t, http.StatusOK, rr.Code)
	responses := decodeRPCBatch(t, rr.Body.Bytes())
	require.Len(t, responses, testBatchSize)
	require.Equal(t, int32(testBatchSuccessCount+1), calls.Load())
	require.LessOrEqual(t, rr.Body.Len(), (10<<20)+testResponseBudgetSlack)
	for i := range responses[:testBatchSuccessCount] {
		require.Equal(t, "2.0", responses[i].JSONRPC)
		require.Equal(t, float64(i), responses[i].ID)
		require.Nil(t, responses[i].Error)
		require.Equal(t, responses[i].Result, largeResult)
	}
	for i := testBatchSuccessCount; i < len(responses); i++ {
		requireRPCError(t, responses[i], float64(i), JSONRPC_RESPONSE_SIZE_LIMIT_EXCEEDED)
		require.Equal(t, "Response size limit exceeded", responses[i].Error.Message)
	}
}

func TestJSONRPCBatchStopsBetweenEntriesWhenContextIsCanceled(t *testing.T) {
	s := newBatchTestService()
	var calls atomic.Int32
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithCancel(context.Background())
	const method = "test_cancel_batch"
	withTestRPCHandler(t, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		calls.Add(1)
		cancel()
		return true, nil
	})

	body := []byte(fmt.Sprintf(`[
		{"jsonrpc":"2.0","method":%q,"id":1},
		{"jsonrpc":"2.0","method":%q,"id":2},
		{"jsonrpc":"2.0","method":%q,"id":3}
	]`, method, method, method))
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body)).WithContext(ctx)
	rr := httptest.NewRecorder()
	s.handleRPC(rr, req)

	require.Equal(t, int32(1), calls.Load(),
		"a canceled request must not run the remaining batch handlers")
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		require.False(t,
			record["level"] == "ERROR" && strings.Contains(strings.ToLower(line), "context canceled"),
			"context.Canceled is a graceful stop and must not be ERROR logged")
	}
}

func TestJSONRPCBatchStopsSilentlyWhenRepositoryCallIsCanceled(t *testing.T) {
	s := newBatchTestService()
	var calls atomic.Int32
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithCancel(context.Background())
	const method = "test_repository_cancel_batch"
	withTestRPCHandler(t, method, func(s *Service, _ *http.Request, _ RPCRequest) (any, error) {
		calls.Add(1)
		cancel()
		return nil, s.repositoryError(ctx, "Unable to retrieve test data from repository",
			fmt.Errorf("repository query failed: %w", ctx.Err()))
	})

	body := []byte(fmt.Sprintf(`[
		{"jsonrpc":"2.0","method":%q,"id":1},
		{"jsonrpc":"2.0","method":%q,"id":2}
	]`, method, method))
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body)).WithContext(ctx)
	rr := httptest.NewRecorder()
	s.handleRPC(rr, req)

	require.Equal(t, int32(1), calls.Load())
	require.NotContains(t, rr.Body.String(), "Internal server error")
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		require.NotEqual(t, "ERROR", record["level"],
			"context.Canceled must not be logged as an operator error")
	}
}

func TestJSONRPCBatchReturnsErrorsForIDDRequestsAfterDeadline(t *testing.T) {
	s := newBatchTestService()
	var calls atomic.Int32
	const method = "test_deadline_batch"
	withTestRPCHandler(t, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		calls.Add(1)
		return true, nil
	})

	body := []byte(fmt.Sprintf(`[
		{"jsonrpc":"2.0","method":%q,"id":1},
		{"jsonrpc":"2.0","method":%q},
		{"jsonrpc":"2.0","method":%q,"id":"three"},
		false,
		{"jsonrpc":"2.0","method":%q,"id":null}
	]`, method, method, method, method))
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body)).WithContext(ctx)
	rr := httptest.NewRecorder()
	s.handleRPC(rr, req)

	require.Zero(t, calls.Load(), "expired batch entries must not be dispatched")
	responses := decodeRPCBatch(t, rr.Body.Bytes())
	require.Len(t, responses, 5, "not all entries receive deadline errors")
	requireRPCError(t, responses[0], float64(1), JSONRPC_TIMEOUT_ERROR)
	requireRPCError(t, responses[1], nil, JSONRPC_TIMEOUT_ERROR)
	requireRPCError(t, responses[2], "three", JSONRPC_TIMEOUT_ERROR)
	requireRPCError(t, responses[3], nil, JSONRPC_INVALID_REQUEST)
	requireRPCError(t, responses[4], nil, JSONRPC_TIMEOUT_ERROR)
	for i, response := range responses {
		if i == 3 {
			require.Equal(t, "invalid request", response.Error.Message)
		} else {
			require.Equal(t, "Request timed out", response.Error.Message)
		}
	}
}

func TestJSONRPCSingleRequestReturnsTimeoutWhenItsContextExpires(t *testing.T) {
	s := newBatchTestService()
	s.dispatchTimeout = 5 * time.Millisecond
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	const method = "test_single_request_timeout"
	withTestRPCHandler(t, method, func(s *Service, r *http.Request, _ RPCRequest) (any, error) {
		<-r.Context().Done()
		return nil, s.repositoryError(r.Context(), "Unable to retrieve test data from repository",
			fmt.Errorf("repository query failed: %w", r.Context().Err()))
	})

	rr := serveRPC(t, s, []byte(fmt.Sprintf(
		`{"jsonrpc":"2.0","method":%q,"id":1}`, method)))
	response := decodeRPCResponse(t, rr.Body.Bytes())
	requireRPCError(t, response, float64(1), JSONRPC_TIMEOUT_ERROR)
	require.Equal(t, "Request timed out", response.Error.Message)
	require.Contains(t, logs.String(), "RPC method dispatch timeout")
	require.NotContains(t, logs.String(), `"level":"ERROR"`)
}

func TestJSONRPCUpstreamDeadlineRemainsInternalError(t *testing.T) {
	s := newBatchTestService()
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	const method = "test_upstream_deadline"
	withTestRPCHandler(t, method, func(s *Service, r *http.Request, _ RPCRequest) (any, error) {
		return nil, s.repositoryError(r.Context(), "Unable to retrieve test data from repository",
			fmt.Errorf("upstream deadline: %w", context.DeadlineExceeded))
	})

	rr := serveRPC(t, s, []byte(fmt.Sprintf(
		`{"jsonrpc":"2.0","method":%q,"id":1}`, method)))
	response := decodeRPCResponse(t, rr.Body.Bytes())
	requireRPCError(t, response, float64(1), JSONRPC_INTERNAL_ERROR)
	require.Contains(t, logs.String(), `"level":"ERROR"`)
}

func TestJSONRPCBatchUsesOneAdmissionPermit(t *testing.T) {
	s := newBatchTestService()
	s.admission = service.NewSemaphoreAdmission(1)
	s.server = &http.Server{
		Handler:           rebuildHandlerWithAdmission(s),
		ReadHeaderTimeout: 2 * time.Second,
	}
	var nestedAcquisitions atomic.Int32
	const method = "test_batch_admission"
	withTestRPCHandler(t, method, func(s *Service, _ *http.Request, _ RPCRequest) (any, error) {
		if s.admission.TryAcquire() {
			nestedAcquisitions.Add(1)
			s.admission.Release()
		}
		return true, nil
	})

	body := []byte(fmt.Sprintf(`[
		{"jsonrpc":"2.0","method":%q,"id":1},
		{"jsonrpc":"2.0","method":%q,"id":2}
	]`, method, method))
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Zero(t, nestedAcquisitions.Load(),
		"the HTTP request's one permit must remain held for the whole batch")
	require.Len(t, decodeRPCBatch(t, rr.Body.Bytes()), 2)
}

func TestJSONRPCBatchLoggingHasOneInfoAndDebugMethods(t *testing.T) {
	s := newBatchTestService()
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	const entries = 3
	body := []byte(`[
		{"jsonrpc":"2.0","method":"attacker_method_0","id":0},
		{"jsonrpc":"2.0","method":"attacker_method_1","id":1},
		{"jsonrpc":"2.0","method":"attacker_method_2","id":2}
	]`)
	serveRPC(t, s, body)

	var batchInfo int
	debugMethods := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		level, _ := record["level"].(string)
		encoded := string(line)
		if level == "INFO" && strings.Contains(strings.ToLower(encoded), "batch") {
			batchInfo++
			require.Contains(t, encoded, fmt.Sprint(entries))
		}
		for i := range entries {
			method := fmt.Sprintf("attacker_method_%d", i)
			if strings.Contains(encoded, method) {
				require.Equal(t, "DEBUG", level, "per-entry method names must never be Info logged")
				debugMethods[method] = true
			}
		}
	}
	require.Equal(t, 1, batchInfo)
	require.Len(t, debugMethods, entries)
}

func withTestRPCHandler(t *testing.T, method string, handler rpcHandler) {
	t.Helper()
	previous, existed := jsonrpcHandlers[method]
	jsonrpcHandlers[method] = handler
	t.Cleanup(func() {
		if existed {
			jsonrpcHandlers[method] = previous
		} else {
			delete(jsonrpcHandlers, method)
		}
	})
}
