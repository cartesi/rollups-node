// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTelemetryTestService returns a *Service ready to have createDefaultTelemetry
// called on it. It wires a ServeMux, a supervisor, and a discard logger.
func newTelemetryTestService() *telemetryService {
	sup := &supervisorImpl{
		Name:   "test",
		logger: discardLogger(),
	}
	sup.serving.Store(true)

	svc := createDefaultTelemetry(sup, "localhost:0")

	return svc.(*telemetryService)
}

func TestTelemetry_Hardened(t *testing.T) {
	s := newTelemetryTestService()
	srv := s.Server

	opts := DefaultTelemetryOptions()
	require.Equal(t, opts.ReadHeaderTimeout, srv.ReadHeaderTimeout)
	require.Equal(t, opts.ReadTimeout, srv.ReadTimeout)
	require.Equal(t, opts.WriteTimeout, srv.WriteTimeout)
	require.Equal(t, opts.IdleTimeout, srv.IdleTimeout)
	require.Equal(t, opts.MaxHeaderBytes, srv.MaxHeaderBytes)
	require.NotNil(t, srv.ErrorLog)
}

func TestTelemetry_HandlersWired(t *testing.T) {
	s := newTelemetryTestService()
	srv := s.Server

	// /readyz: no services are unready, so expect 200.
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	body, _ := io.ReadAll(rr.Body)
	require.Contains(t, string(body), "ready")

	// /livez: the supervisor is alive, so expect 200.
	rr = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/livez", nil))
	require.Equal(t, http.StatusOK, rr.Code)
}

// Telemetry intentionally does NOT wire RequestIDMiddleware. /livez and
// /readyz are hammered by orchestrator probes, and burning a crypto/rand
// UUID per probe is wasted work for responses that have nothing to correlate
// against. A static "telemetry" sentinel is used instead so panic logs are
// greppable without the cost of crypto/rand per probe.
func TestTelemetry_StaticRequestID(t *testing.T) {
	s := newTelemetryTestService()
	srv := s.Server

	for _, path := range []string{"/livez", "/readyz"} {
		rr := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rr.Code, "path=%s", path)
		require.Equal(t, "telemetry", rr.Header().Get(requestIDHeader), "path=%s", path)
	}

	// A client-supplied X-Request-ID is ignored (RequestIDMiddleware is not
	// in the chain), so the static sentinel is always used.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	req.Header.Set(requestIDHeader, "client-supplied-id")
	srv.Handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "telemetry", rr.Header().Get(requestIDHeader))
}

func TestTelemetry_PanicRecovered(t *testing.T) {
	s := newTelemetryTestService()
	srv := s.Server

	s.serveMux.Handle("/boom", http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("kaboom")
	}))

	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/boom", nil))

	// RecoverMiddleware must convert the panic into a generic 500.
	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Contains(t, rr.Body.String(), "Internal server error")
	require.NotContains(t, rr.Body.String(), "kaboom")
}

func TestTelemetry_LifecycleFailure(t *testing.T) {
	s := newTelemetryTestService()

	s.supervisor.(*supervisorImpl).serving.Store(false)
	require.False(t, s.supervisor.Alive())
	require.Equal(t, []string{"test"}, s.supervisor.NotReady())

	for _, path := range []string{"/readyz", "/livez"} {
		rr := httptest.NewRecorder()
		s.Server.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		status := http.StatusInternalServerError
		if path == "/readyz" {
			status = http.StatusServiceUnavailable
			require.Equal(t, "test/telemetry: ready check failed: test\n", rr.Body.String())
		}
		require.Equal(t, status, rr.Code, "path=%s", path)
	}
}

func TestReadinessReportsAllFailingServicesWithoutLogging(t *testing.T) {
	s := newTelemetryTestService()
	sup := s.supervisor.(*supervisorImpl)
	var logs bytes.Buffer
	sup.logger = slog.New(slog.NewTextHandler(&logs, nil))
	first := &testServiceImpl{BaseTemplate: BaseTemplate{Name: "evm-reader"}}
	second := &testServiceImpl{BaseTemplate: BaseTemplate{Name: "advancer"}}
	healthy := &testServiceImpl{BaseTemplate: BaseTemplate{Name: "jsonrpc"}, ready: true}
	sup.services = []SupervisedService{first, healthy, second}

	for range 3 {
		require.Equal(t, []string{"advancer", "evm-reader"}, sup.NotReady())
		rr := httptest.NewRecorder()
		s.Server.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		require.Equal(t, http.StatusServiceUnavailable, rr.Code)
		require.Equal(t, "test/telemetry: ready check failed: advancer, evm-reader\n", rr.Body.String())
	}
	require.Empty(t, logs.String(), "readiness probes must not log failures")
	first.ready = true
	require.Equal(t, []string{"advancer"}, sup.NotReady())
	second.ready = true
	rr := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "test/telemetry: ready\n", rr.Body.String())
}
