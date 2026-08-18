// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/stretchr/testify/require"
)

// ensureSentinelRejects fires a request against s.server.Handler and
// expects the admission middleware to reject it with a 503.
func ensureSentinelRejects(t *testing.T, s *Service) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/rpc", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	retryAfter, err := strconv.Atoi(rr.Header().Get("Retry-After"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, retryAfter, 1)
	require.LessOrEqual(t, retryAfter, 3)
	require.Contains(t, rr.Body.String(), "service at capacity")
	require.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
}

// TestJSONRPC_HardenedServerOptions checks that Create wires the jsonrpc
// HTTP server through NewHTTPServer with DefaultJSONRPCOptions. If a future
// refactor reintroduces an inline &http.Server{} literal or drops a field,
// this test catches it.
func TestJSONRPC_HardenedServerOptions(t *testing.T) {
	s := newTestService(t, "jsonrpc-server-options")
	require.NotNil(t, s.server)

	opts := service.DefaultJSONRPCOptions()
	require.Equal(t, opts.ReadHeaderTimeout, s.server.ReadHeaderTimeout)
	require.Equal(t, opts.ReadTimeout, s.server.ReadTimeout)
	require.Equal(t, opts.WriteTimeout, s.server.WriteTimeout)
	require.Equal(t, opts.IdleTimeout, s.server.IdleTimeout)
	require.Equal(t, opts.MaxHeaderBytes, s.server.MaxHeaderBytes)
	require.NotNil(t, s.server.ErrorLog)
	require.Equal(t, opts.WriteTimeout-jsonrpcWriteHeadroom, s.dispatchTimeout)
}

func TestJSONRPC_ServerHandlerAppliesBatchDispatchTimeout(t *testing.T) {
	s := newTestService(t, "jsonrpc-dispatch-timeout")
	s.dispatchTimeout = 10 * time.Millisecond

	const method = "test_server_dispatch_timeout"
	withTestRPCHandler(t, method, func(_ *Service, r *http.Request, _ RPCRequest) (any, error) {
		<-r.Context().Done()
		return true, nil
	})

	body := []byte(fmt.Sprintf(`[
		{"jsonrpc":"2.0","method":%q,"id":1},
		{"jsonrpc":"2.0","method":%q,"id":2},
		{"jsonrpc":"2.0","method":%q,"id":3}
	]`, method, method, method))
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	var responses []RPCResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &responses))
	require.Len(t, responses, 3)
	require.Nil(t, responses[0].Error)
	requireRPCError(t, responses[1], float64(2), JSONRPC_TIMEOUT_ERROR)
	requireRPCError(t, responses[2], float64(3), JSONRPC_TIMEOUT_ERROR)
}

// TestJSONRPC_RequestIDPropagated verifies the middleware chain echoes a
// valid X-Request-ID back on the response. Runs directly against the
// Service.server handler so the whole chain (Recover -> RequestID -> CORS
// -> Admission -> mux) is exercised.
func TestJSONRPC_RequestIDPropagated(t *testing.T) {
	s := newTestService(t, "jsonrpc-request-id")

	req := httptest.NewRequest(http.MethodPost, "/rpc", http.NoBody)
	req.Header.Set("X-Request-ID", "pinned-xyz")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	// handleRPC will reject the empty body as a bad request, but the
	// middleware chain still runs and must echo the header.
	require.Equal(t, "pinned-xyz", rr.Header().Get("X-Request-ID"))
}

// -----------------------------------------------------------------------------
// Oversized body handling
// -----------------------------------------------------------------------------

func TestJSONRPC_OversizedBodyReturns413(t *testing.T) {
	s := newTestService(t, "jsonrpc-oversize")

	// Build a body that exceeds MAX_BODY_SIZE (1 MiB).
	oversized := bytes.Repeat([]byte("x"), MAX_BODY_SIZE+1)

	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(oversized))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	require.Contains(t, rr.Body.String(), "Payload too large")
}

// -----------------------------------------------------------------------------
// Admission control
// -----------------------------------------------------------------------------

func TestJSONRPC_AdmissionDisabledWhenZero(t *testing.T) {
	// JsonrpcMaxInflight=0 must leave s.admission == nil and make the
	// middleware a passthrough. We can't assert "no 503" directly
	// without coordinating a slow handler; instead verify the field
	// and confirm a basic request reaches handleRPC (which rejects
	// an empty body with 400, not 503).
	s := newTestServiceWithInflight(t, "jsonrpc-adm-zero", 0)
	require.Nil(t, s.admission, "limit=0 must produce nil admission")

	req := httptest.NewRequest(http.MethodPost, "/rpc", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)
	require.NotEqual(t, http.StatusServiceUnavailable, rr.Code,
		"disabled admission must not return 503")
}

func TestJSONRPC_AdmissionWiredWhenPositive(t *testing.T) {
	// JsonrpcMaxInflight>0 must construct a non-nil SemaphoreAdmission
	// with the matching limit.
	s := newTestServiceWithInflight(t, "jsonrpc-adm-wired", 7)
	require.NotNil(t, s.admission)
	require.Equal(t, uint64(7), s.admission.Limit())
}

func TestJSONRPC_AdmissionRejectsWhenExhausted(t *testing.T) {
	// Replace the post-Create admission with a pre-filled semaphore
	// and rebuild just the middleware stack onto a fresh mux. This is
	// the same wiring Create() does — inlined here to exercise the
	// rejection path without a blocking RPC handler.
	s := newTestServiceWithInflight(t, "jsonrpc-adm-reject", 1)

	// Swap the admission underlying the server handler for a
	// pre-filled one to force rejection on every request.
	s.admission = service.NewSemaphoreAdmission(1)
	s.admission.TryAcquire() // pre-fill the single permit
	s.server.Handler = rebuildHandlerWithAdmission(s)

	ensureSentinelRejects(t, s)
	require.Equal(t, uint64(1), s.admission.Rejected())
}

func TestJSONRPC_AdmissionPermitReleasedAfterRequest(t *testing.T) {
	// With limit=1 a sequential burst must all succeed: each request
	// releases its permit on return.
	s := newTestServiceWithInflight(t, "jsonrpc-adm-release", 1)
	require.NotNil(t, s.admission)

	for range 5 {
		req := httptest.NewRequest(http.MethodPost, "/rpc", http.NoBody)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(rr, req)
		// handleRPC returns 400 on empty body; the key assertion is
		// that we never see 503 because the permit is released each
		// time the handler returns.
		require.NotEqual(t, http.StatusServiceUnavailable, rr.Code)
	}
	require.Equal(t, uint64(0), s.admission.Rejected())
}

// rebuildHandlerWithAdmission rewraps the service's mux with only
// AdmissionMiddleware bound to the service's current admission field.
// Used by tests that swap the admission after construction to exercise
// the rejection path. The outer chain (Recover, RequestID, CORS) from
// Create() is deliberately omitted — admission is what these tests
// pin, and the other layers are inert for a plain POST without Origin.
func rebuildHandlerWithAdmission(s *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", s.handleRPC)

	var handler http.Handler = mux
	handler = service.AdmissionMiddleware(s.admission)(handler)
	return handler
}

// -----------------------------------------------------------------------------
// CORS integration
// -----------------------------------------------------------------------------

func TestJSONRPC_CORSDisabledByDefault(t *testing.T) {
	s := newTestService(t, "jsonrpc-cors-off")

	req := httptest.NewRequest(http.MethodPost, "/rpc", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.com")
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	require.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, rr.Header().Get("Vary"))
}

func TestJSONRPC_CORSAllowedOriginEchoed(t *testing.T) {
	s := newTestServiceWithCORS(t, "jsonrpc-cors-on", "http://trusted.example.com")

	req := httptest.NewRequest(http.MethodPost, "/rpc", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://trusted.example.com")
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	require.Equal(t, "http://trusted.example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rr.Header().Values("Vary"), "Origin")
}

func TestJSONRPC_CORSDisallowedOriginNoGrant(t *testing.T) {
	s := newTestServiceWithCORS(t, "jsonrpc-cors-reject", "http://trusted.example.com")

	req := httptest.NewRequest(http.MethodPost, "/rpc", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.com")
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	require.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rr.Header().Values("Vary"), "Origin")
}

func TestJSONRPC_CORSNoOriginPassthrough(t *testing.T) {
	s := newTestServiceWithCORS(t, "jsonrpc-cors-no-origin", "http://trusted.example.com")

	req := httptest.NewRequest(http.MethodPost, "/rpc", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	require.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	require.NotEqual(t, http.StatusServiceUnavailable, rr.Code)
}
