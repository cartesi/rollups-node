// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------------
// SemaphoreAdmission
// -----------------------------------------------------------------------------

func TestSemaphoreAdmission_BasicAcquireRelease(t *testing.T) {
	a := NewSemaphoreAdmission(2)

	require.True(t, a.TryAcquire())
	require.True(t, a.TryAcquire())
	require.False(t, a.TryAcquire(), "third should be rejected at limit 2")

	a.Release()
	require.True(t, a.TryAcquire(), "after one release a new permit should be available")
}

func TestSemaphoreAdmission_RejectedCounter(t *testing.T) {
	a := NewSemaphoreAdmission(1)

	require.True(t, a.TryAcquire())
	for range 4 {
		require.False(t, a.TryAcquire())
	}
	require.Equal(t, uint64(4), a.Rejected())

	a.Release()
	require.True(t, a.TryAcquire())
	require.Equal(t, uint64(4), a.Rejected(), "successful acquire must not bump counter")
}

func TestSemaphoreAdmission_ZeroLimit(t *testing.T) {
	// A zero-capacity semaphore naturally rejects every TryAcquire.
	a := NewSemaphoreAdmission(0)

	for range 3 {
		require.False(t, a.TryAcquire())
	}
	require.Equal(t, uint64(3), a.Rejected())
}

// -----------------------------------------------------------------------------
// AdmissionMiddleware
// -----------------------------------------------------------------------------

func TestAdmissionMiddleware_NilIsPassthrough(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mw := AdmissionMiddleware(nil)

	rr := httptest.NewRecorder()
	mw(handler).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "ok", rr.Body.String())
}

func TestAdmissionMiddleware_AdmitsBelowLimit(t *testing.T) {
	ac := NewSemaphoreAdmission(2)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AdmissionMiddleware(ac)

	for range 5 {
		rr := httptest.NewRecorder()
		mw(handler).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusOK, rr.Code)
	}
	require.Equal(t, uint64(0), ac.Rejected())
}

func TestAdmissionMiddleware_RejectsAtLimit(t *testing.T) {
	ac := NewSemaphoreAdmission(1)

	// A handler that signals when it has been entered (i.e. the permit
	// has been acquired by AdmissionMiddleware) and then blocks until
	// we release it. This gives the test a deterministic point to fire
	// the second request without any time-based polling.
	entered := make(chan struct{})
	block := make(chan struct{})
	done := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-block
		w.WriteHeader(http.StatusOK)
	})

	wrapped := AdmissionMiddleware(ac)(handler)

	// Fire the first request in a goroutine; it will hold the permit
	// until we close(block).
	go func() {
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		close(done)
	}()

	<-entered // first request has the permit

	// Second request — must be rejected immediately.
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	retryAfter, err := strconv.Atoi(rr.Header().Get("Retry-After"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, retryAfter, 1)
	require.LessOrEqual(t, retryAfter, 3)
	require.Contains(t, rr.Body.String(), "service at capacity")
	require.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	require.Equal(t, uint64(1), ac.Rejected())

	close(block)
	<-done
}

func TestAdmissionMiddleware_ReleasesOnSuccess(t *testing.T) {
	ac := NewSemaphoreAdmission(1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AdmissionMiddleware(ac)

	for range 3 {
		rr := httptest.NewRecorder()
		mw(handler).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusOK, rr.Code)
	}
	// The permit should always be back after each sequential request.
	require.Equal(t, uint64(0), ac.Rejected())
}

// TestAdmissionReleaseOnPanic pins the "permit is released even when a
// downstream handler panics" invariant. This must work under the real
// middleware chain — RecoverMiddleware wraps AdmissionMiddleware and
// catches the panic, but only after AdmissionMiddleware's deferred
// Release has run.
func TestAdmissionReleaseOnPanic(t *testing.T) {
	ac := NewSemaphoreAdmission(2)

	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(errors.New("kaboom"))
	})

	// Chain: Recover(Admission(panicking)). Admission defers its
	// Release(), then the handler panics, then the defer runs (releasing
	// the permit), then Recover catches the panic and writes a 500.
	var buf bytes.Buffer
	chain := RecoverMiddleware(captureLogger(&buf))(AdmissionMiddleware(ac)(panicking))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)

	// Both permits should be available: verify by acquiring both in a row.
	require.True(t, ac.TryAcquire(), "permit 1 must be available after panic")
	require.True(t, ac.TryAcquire(), "permit 2 must be available after panic")
	ac.Release()
	ac.Release()
}

func TestAdmissionMiddleware_NoLogSpam(t *testing.T) {
	ac := NewSemaphoreAdmission(1)
	ac.TryAcquire() // pre-fill the single permit so all subsequent acquires fail
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})

	// No logger is wired into AdmissionMiddleware itself, so rejection
	// must produce no log output regardless of what wraps it. Verify
	// that wrapping in RecoverMiddleware with a capture logger doesn't
	// pick up any rejection logs.
	var buf bytes.Buffer
	chain := RecoverMiddleware(captureLogger(&buf))(AdmissionMiddleware(ac)(handler))

	for range 10 {
		rr := httptest.NewRecorder()
		chain.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	}
	require.Empty(t, buf.String(), "rejection must not log per-request")
}

func TestAdmissionMiddleware_ConcurrentStress(t *testing.T) {
	ac := NewSemaphoreAdmission(4)

	// Handler that yields briefly so concurrent requests have a chance
	// to collide on the semaphore.
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AdmissionMiddleware(ac)
	wrapped := mw(handler)

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rr := httptest.NewRecorder()
			wrapped.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		}()
	}
	wg.Wait()

	// After everything settles, all permits must be back.
	for range 4 {
		require.True(t, ac.TryAcquire())
	}
}
