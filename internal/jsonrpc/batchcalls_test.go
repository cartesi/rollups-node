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
		HTTPServiceTemplate: service.HTTPServiceTemplate{
			BaseTemplate: service.BaseTemplate{
				Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
			},
		},
		handlers: cloneDispatchTable(jsonrpcHandlers),
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
	require.Equal(t, id, decodeRPCID(t, response.ID))
	require.NotNil(t, response.Error)
	require.Equal(t, code, response.Error.Code)
}

func decodeRPCID(t *testing.T, id json.RawMessage) any {
	t.Helper()
	var decoded any
	require.NoError(t, json.Unmarshal(id, &decoded))
	return decoded
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

func TestListOutputsRejectsMalformedOutputType(t *testing.T) {
	for name, selector := range map[string]string{
		"wrong length": "0x1234",
		"non hex":      "0xzzzzzzzz",
	} {
		t.Run(name, func(t *testing.T) {
			s := newBatchTestService()
			body := []byte(fmt.Sprintf(`{
				"jsonrpc":"2.0",
				"method":"cartesi_listOutputs",
				"params":{"application":"app","output_type":%q},
				"id":1
			}`, selector))
			rr := serveRPC(t, s, body)

			require.Equal(t, http.StatusOK, rr.Code)
			response := decodeRPCResponse(t, rr.Body.Bytes())
			requireRPCError(t, response, float64(1), JSONRPC_INVALID_PARAMS)
			require.Contains(t, response.Error.Message, "Invalid output type")
		})
	}
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
	withTestRPCHandler(t, s, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
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
	response := decodeRPCResponse(t, rr.Body.Bytes())
	requireRPCError(t, response, nil, JSONRPC_PARSE_ERROR)
	require.Equal(t, "Parse error", response.Error.Message)
}

func TestJSONRPCMalformedObjectReturnsJSONContentType(t *testing.T) {
	s := newBatchTestService()
	rr := serveRPC(t, s, []byte(`{"jsonrpc":"2.0"`))

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	response := decodeRPCResponse(t, rr.Body.Bytes())
	requireRPCError(t, response, nil, JSONRPC_PARSE_ERROR)
	require.Equal(t, "Parse error", response.Error.Message)
}

func TestJSONRPCDiscoverPreservesLargeIntegerLiterals(t *testing.T) {
	s := newBatchTestService()
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &s.discoverSpec))

	rr := serveRPC(t, s, []byte(`{"jsonrpc":"2.0","method":"rpc.discover","id":1}`))

	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), `"maximum":9223372036854775807`)
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
	require.EqualValues(t, 1, decodeRPCID(t, responses[0].ID))
	requireRPCError(t, responses[1], nil, JSONRPC_INVALID_REQUEST)
	require.Nil(t, responses[2].Error)
	require.EqualValues(t, 3, decodeRPCID(t, responses[2].ID))
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
			require.EqualValues(t, 1, decodeRPCID(t, responses[0].ID))

			requireRPCError(t, responses[1], test.id, JSONRPC_INVALID_REQUEST)

			require.Nil(t, responses[2].Error)
			require.EqualValues(t, 3, decodeRPCID(t, responses[2].ID))
		})
	}
}

func TestJSONRPCValidationErrorsEchoValidID(t *testing.T) {
	s := newBatchTestService()
	tests := map[string]struct {
		body    string
		id      any
		message string
	}{
		"missing method": {
			body: `{"jsonrpc":"2.0","id":"request-id"}`, id: "request-id", message: "Invalid Request",
		},
		"invalid version": {
			body: `{"jsonrpc":"1.0","method":"cartesi_getNodeVersion","id":42}`, id: float64(42), message: "Unsupported JSON-RPC version",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			response := decodeRPCResponse(t, serveRPC(t, s, []byte(test.body)).Body.Bytes())
			requireRPCError(t, response, test.id, JSONRPC_INVALID_REQUEST)
			require.Equal(t, test.message, response.Error.Message)
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
			require.Equal(t, "Invalid request ID", response.Error.Message)
		})
	}
}

func TestRPCIDRoundTripsWithoutNumericPrecisionLoss(t *testing.T) {
	s := newBatchTestService()
	single := serveRPC(t, s, []byte(`{
		"jsonrpc":"2.0",
		"method":"cartesi_getNodeVersion",
		"id":9007199254740993
	}`))
	singleResponse := decodeRPCResponse(t, single.Body.Bytes())
	require.Nil(t, singleResponse.Error)
	require.Equal(t, `9007199254740993`, string(singleResponse.ID))

	rr := serveRPC(t, s, []byte(`[
		{"jsonrpc":"2.0","method":"missing","id":9007199254740993},
		{"jsonrpc":"2.0","method":"missing","id":18446744073709551616},
		{"jsonrpc":"2.0","method":"missing","id":"request-3"}
	]`))

	responses := decodeRPCBatch(t, rr.Body.Bytes())
	require.Len(t, responses, 3)
	require.Equal(t, `9007199254740993`, string(responses[0].ID))
	require.Equal(t, `18446744073709551616`, string(responses[1].ID))
	require.Equal(t, `"request-3"`, string(responses[2].ID))
	for _, response := range responses {
		require.NotNil(t, response.Error)
		require.Equal(t, JSONRPC_METHOD_NOT_FOUND, response.Error.Code)
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
	require.Nil(t, decodeRPCID(t, responses[0].ID))
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
	withTestRPCHandler(t, s, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
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
		require.Equal(t, float64(i), decodeRPCID(t, responses[i].ID))
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
	withTestRPCHandler(t, s, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
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
	withTestRPCHandler(t, s, method, func(s *Service, _ *http.Request, _ RPCRequest) (any, error) {
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
	withTestRPCHandler(t, s, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		calls.Add(1)
		return true, nil
	})

	body := []byte(fmt.Sprintf(`[
		{"jsonrpc":"2.0","method":%q,"id":1},
		{"jsonrpc":"2.0","method":%q},
		{"jsonrpc":"2.0","method":%q,"id":"three"},
		false,
		{"jsonrpc":"2.0","method":%q,"id":true},
		{"jsonrpc":"2.0","method":%q,"id":null}
	]`, method, method, method, method, method))
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body)).WithContext(ctx)
	rr := httptest.NewRecorder()
	s.handleRPC(rr, req)

	require.Zero(t, calls.Load(), "expired batch entries must not be dispatched")
	responses := decodeRPCBatch(t, rr.Body.Bytes())
	require.Len(t, responses, 6, "not all entries receive deadline errors")
	requireRPCError(t, responses[0], float64(1), JSONRPC_TIMEOUT_ERROR)
	requireRPCError(t, responses[1], nil, JSONRPC_TIMEOUT_ERROR)
	requireRPCError(t, responses[2], "three", JSONRPC_TIMEOUT_ERROR)
	requireRPCError(t, responses[3], nil, JSONRPC_INVALID_REQUEST)
	requireRPCError(t, responses[4], nil, JSONRPC_INVALID_REQUEST)
	requireRPCError(t, responses[5], nil, JSONRPC_TIMEOUT_ERROR)
	for i, response := range responses {
		if i == 3 {
			require.Equal(t, "Invalid Request", response.Error.Message)
		} else if i == 4 {
			require.Equal(t, "Invalid request ID", response.Error.Message)
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
	withTestRPCHandler(t, s, method, func(s *Service, r *http.Request, _ RPCRequest) (any, error) {
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
	withTestRPCHandler(t, s, method, func(s *Service, r *http.Request, _ RPCRequest) (any, error) {
		return nil, s.repositoryError(r.Context(), "Unable to retrieve test data from repository",
			fmt.Errorf("upstream deadline: %w", context.DeadlineExceeded))
	})

	rr := serveRPC(t, s, []byte(fmt.Sprintf(
		`{"jsonrpc":"2.0","method":%q,"id":1}`, method)))
	response := decodeRPCResponse(t, rr.Body.Bytes())
	requireRPCError(t, response, float64(1), JSONRPC_INTERNAL_ERROR)
	require.Contains(t, logs.String(), `"level":"ERROR"`)
}

func TestJSONRPCBatchRecoversPanicPerEntry(t *testing.T) {
	s := newBatchTestService()
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	panicMethod := strings.Repeat("p", MAX_LOGGED_METHOD_LEN+32)
	const okMethod = "test_after_panic_batch"
	withTestRPCHandler(t, s, panicMethod, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		panic("test panic")
	})
	withTestRPCHandler(t, s, okMethod, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		return "ok", nil
	})

	body := []byte(fmt.Sprintf(`[
		{"jsonrpc":"2.0","method":%q,"id":1},
		{"jsonrpc":"2.0","method":%q,"id":2},
		{"jsonrpc":"2.0","method":%q,"id":3}
	]`, okMethod, panicMethod, okMethod))
	rr := serveRPC(t, s, body)

	responses := decodeRPCBatch(t, rr.Body.Bytes())
	require.Len(t, responses, 3)
	require.Nil(t, responses[0].Error)
	requireRPCError(t, responses[1], float64(2), JSONRPC_INTERNAL_ERROR)
	require.Equal(t, "Internal server error", responses[1].Error.Message)
	require.Nil(t, responses[2].Error, "entries after a panic must still be dispatched")
	require.Contains(t, logs.String(), "RPC method panic")
	require.Contains(t, logs.String(), "test panic")
	require.Contains(t, logs.String(), "goroutine", "panic log must include a stack trace")
	require.Contains(t, logs.String(), truncatedMethod(panicMethod))
	require.NotContains(t, logs.String(), panicMethod)
}

func TestJSONRPCDoesNotRecoverAbortHandler(t *testing.T) {
	s := newBatchTestService()
	const method = "test_abort_handler"
	withTestRPCHandler(t, s, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		panic(http.ErrAbortHandler)
	})

	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		serveRPC(t, s, []byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","method":%q,"id":1}`, method)))
	})
}

func TestJSONRPCBatchUsesOneAdmissionPermit(t *testing.T) {
	s := newBatchTestService()
	s.Admission = service.NewSemaphoreAdmission(1)
	s.Server = &http.Server{
		Handler:           rebuildHandlerWithAdmission(s),
		ReadHeaderTimeout: 2 * time.Second,
	}
	var nestedAcquisitions atomic.Int32
	const method = "test_batch_admission"
	withTestRPCHandler(t, s, method, func(s *Service, _ *http.Request, _ RPCRequest) (any, error) {
		if s.Admission.TryAcquire() {
			nestedAcquisitions.Add(1)
			s.Admission.Release()
		}
		return true, nil
	})

	body := []byte(fmt.Sprintf(`[
		{"jsonrpc":"2.0","method":%q,"id":1},
		{"jsonrpc":"2.0","method":%q,"id":2}
	]`, method, method))
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rr, req)

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

func TestJSONRPCBatchMethodLoggingIsTruncated(t *testing.T) {
	s := newBatchTestService()
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	method := strings.Repeat("b", MAX_LOGGED_METHOD_LEN+32)
	body := []byte(fmt.Sprintf(`[{"jsonrpc":"2.0","method":%q,"id":1}]`, method))
	serveRPC(t, s, body)

	var found bool
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["method"] == truncatedMethod(method) {
			require.Equal(t, "DEBUG", record["level"])
			found = true
		}
		require.NotEqual(t, method, record["method"])
	}
	require.True(t, found)
}

func withTestRPCHandler(t *testing.T, service *Service, method string, handler rpcHandler) {
	t.Helper()
	previous, existed := service.handlers[method]
	service.handlers[method] = handler
	t.Cleanup(func() {
		if existed {
			service.handlers[method] = previous
		} else {
			delete(service.handlers, method)
		}
	})
}

func TestRPCHandlerOverridesAreServiceLocal(t *testing.T) {
	first := newBatchTestService()
	second := newBatchTestService()
	const method = "test_service_local_handler"
	withTestRPCHandler(t, first, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		return true, nil
	})

	_, firstHasHandler := first.handlers[method]
	_, secondHasHandler := second.handlers[method]
	_, globalHasHandler := jsonrpcHandlers[method]
	require.True(t, firstHasHandler)
	require.False(t, secondHasHandler)
	require.False(t, globalHasHandler)
}
