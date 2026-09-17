// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitHTTPServiceTemplate_PanicBecomes500WithRequestID(t *testing.T) {
	var logs bytes.Buffer
	s := &HTTPServiceTemplate{}
	InitHTTPServiceTemplate(s, &HTTPServiceConfigs{
		BaseConfigs:   BaseConfigs{Logger: captureLogger(&logs)},
		SafeRequestID: true,
	}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "chain-id")
	rr := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Equal(t, "chain-id", rr.Header().Get("X-Request-ID"))
	require.Contains(t, logs.String(), "chain-id")
}

func TestInitHTTPServiceTemplate_NilAdmissionAndDisabledCORS(t *testing.T) {
	s := &HTTPServiceTemplate{}
	var seenID string
	InitHTTPServiceTemplate(s, &HTTPServiceConfigs{
		BaseConfigs:   BaseConfigs{Logger: discardLogger()},
		SafeRequestID: true,
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Nil(t, s.Admission)
	require.Equal(t, http.StatusOK, rr.Code)
	require.NotEmpty(t, seenID)
	require.Equal(t, seenID, rr.Header().Get("X-Request-ID"))
	require.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestInitHTTPServiceTemplate_CORSShortCircuitsBeforeAdmission(t *testing.T) {
	s := &HTTPServiceTemplate{}
	calls := 0
	InitHTTPServiceTemplate(s, &HTTPServiceConfigs{
		BaseConfigs:        BaseConfigs{Logger: discardLogger()},
		MaxInflight:        1,
		CorsAllowedOrigins: "https://allowed.example",
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	// Occupy the only permit: preflights must bypass admission, not return 503.
	require.True(t, s.Admission.TryAcquire())
	func() {
		defer s.Admission.Release()
		for _, origin := range []string{"https://allowed.example", "https://evil.example"} {
			req := httptest.NewRequest(http.MethodOptions, "/", nil)
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", "POST")
			rr := httptest.NewRecorder()
			s.Server.Handler.ServeHTTP(rr, req)
			require.Equal(t, http.StatusNoContent, rr.Code)
		}
		require.Zero(t, calls)
		require.Zero(t, s.Admission.Rejected())
		// A real request still passes through admission and is rejected while full.
		rr := httptest.NewRecorder()
		s.Server.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/", nil))
		require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	}()
	rr := httptest.NewRecorder()
	s.Server.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, 1, calls)
}
