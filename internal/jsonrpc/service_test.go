// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/stretchr/testify/require"
)

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
}

// TestJSONRPC_RequestIDPropagated verifies the middleware chain echoes a
// valid X-Request-ID back on the response. Runs directly against the
// Service.server handler so the whole chain (Recover -> RequestID -> CORS
// -> mux) is exercised.
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
