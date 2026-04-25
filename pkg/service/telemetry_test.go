// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTelemetryTestService returns a *Service ready to have CreateDefaultTelemetry
// called on it. It wires a ServeMux, a mockImpl, and a discard logger.
func newTelemetryTestService() *Service {
	impl := &mockImpl{}
	return &Service{
		Name:     "test",
		Logger:   discardLogger(),
		ServeMux: http.NewServeMux(),
		Impl:     impl,
	}
}

func TestCreateDefaultTelemetry_Hardened(t *testing.T) {
	s := newTelemetryTestService()
	srv, _ := s.CreateDefaultTelemetry(":0")

	opts := DefaultTelemetryOptions()
	require.Equal(t, opts.ReadHeaderTimeout, srv.ReadHeaderTimeout)
	require.Equal(t, opts.ReadTimeout, srv.ReadTimeout)
	require.Equal(t, opts.WriteTimeout, srv.WriteTimeout)
	require.Equal(t, opts.IdleTimeout, srv.IdleTimeout)
	require.Equal(t, opts.MaxHeaderBytes, srv.MaxHeaderBytes)
	require.NotNil(t, srv.ErrorLog)
}

func TestCreateDefaultTelemetry_HandlersWired(t *testing.T) {
	s := newTelemetryTestService()
	srv, _ := s.CreateDefaultTelemetry(":0")

	// /readyz: mockImpl.Ready() is true, so expect 200.
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	body, _ := io.ReadAll(rr.Body)
	require.Contains(t, string(body), "ready")

	// /livez: mockImpl.Alive() is true, so expect 200.
	rr = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/livez", nil))
	require.Equal(t, http.StatusOK, rr.Code)
}

// Telemetry intentionally does NOT wire RequestIDMiddleware. /livez and
// /readyz are hammered by orchestrator probes, and burning a crypto/rand
// UUID per probe is wasted work for responses that have nothing to correlate
// against. A static "telemetry" sentinel is used instead so panic logs are
// greppable without the cost of crypto/rand per probe.
func TestCreateDefaultTelemetry_StaticRequestID(t *testing.T) {
	s := newTelemetryTestService()
	srv, _ := s.CreateDefaultTelemetry(":0")

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

func TestCreateDefaultTelemetry_PanicRecovered(t *testing.T) {
	s := newTelemetryTestService()
	s.ServeMux.Handle("/boom", http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("kaboom")
	}))

	srv, _ := s.CreateDefaultTelemetry(":0")

	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/boom", nil))

	// RecoverMiddleware must convert the panic into a generic 500.
	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Contains(t, rr.Body.String(), "Internal server error")
	require.NotContains(t, rr.Body.String(), "kaboom")
}

type falseLifecycleImpl struct{ Service }

func (*falseLifecycleImpl) Alive() bool                   { return false }
func (*falseLifecycleImpl) Ready() bool                   { return false }
func (*falseLifecycleImpl) OnServe(context.Context) error { return nil }

func TestCreateDefaultTelemetry_Returns500WhenLifecycleFails(t *testing.T) {
	service := &Service{
		Name:     "test",
		Logger:   discardLogger(),
		ServeMux: http.NewServeMux(),
		Impl:     &falseLifecycleImpl{},
	}

	srv, _ := service.CreateDefaultTelemetry(":0")

	for _, path := range []string{"/readyz", "/livez"} {
		rr := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusInternalServerError, rr.Code, "path=%s", path)
	}
}
