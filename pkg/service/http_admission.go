// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync/atomic"

	"golang.org/x/sync/semaphore"
)

const (
	retryAfterMin = 1
	retryAfterMax = 3
)

// SemaphoreAdmission caps the number of concurrent HTTP requests that may
// be in flight on a given route. It is a front-door gate used by callers
// (inspect, jsonrpc) to fail fast under saturation before spending memory
// or backend resources on a request.
//
// Production callers disable admission by passing nil to
// [AdmissionMiddleware], which produces a no-op middleware so callers can
// wire admission unconditionally and disable it via configuration.
type SemaphoreAdmission struct {
	sem      *semaphore.Weighted
	limit    uint64
	rejected atomic.Uint64
}

// NewSemaphoreAdmission constructs a [SemaphoreAdmission] with the given
// capacity. Values above [math.MaxInt64] are clamped to MaxInt64 because
// the underlying semaphore primitive is int64-based; realistic operator
// inputs are orders of magnitude below this bound.
//
// Production callers disable admission by passing nil to
// [AdmissionMiddleware], not by constructing a zero-limit controller.
func NewSemaphoreAdmission(limit uint64) *SemaphoreAdmission {
	sizedLimit := min(limit, uint64(math.MaxInt64))
	return &SemaphoreAdmission{
		sem:   semaphore.NewWeighted(int64(sizedLimit)), //nolint:gosec // G115: clamped above
		limit: sizedLimit,
	}
}

// TryAcquire attempts to reserve one in-flight slot without blocking.
// It returns true when a permit was obtained; false otherwise. Rejected
// attempts bump the monotonic [Rejected] counter.
func (a *SemaphoreAdmission) TryAcquire() bool {
	if !a.sem.TryAcquire(1) {
		a.rejected.Add(1)
		return false
	}
	return true
}

// Release returns a permit previously obtained via [TryAcquire]. It is
// an error to call Release without a matching successful TryAcquire.
func (a *SemaphoreAdmission) Release() {
	a.sem.Release(1)
}

// Rejected returns the total number of permit attempts that were
// rejected since construction. Monotonic.
func (a *SemaphoreAdmission) Rejected() uint64 {
	return a.rejected.Load()
}

// Limit returns the configured capacity the controller was constructed with.
func (a *SemaphoreAdmission) Limit() uint64 {
	return a.limit
}

// AdmissionMiddleware returns a middleware that gates requests through
// the supplied controller. When ac is nil the middleware is a no-op
// pass-through, so callers can wire it unconditionally and disable
// admission via configuration.
//
// On reject: 503 Service Unavailable, Retry-After: 1-3 (jittered),
// body "service at capacity (request_id=<id>)\n", no per-request log
// (logging every rejection would amplify a flood). On admit: defers
// [SemaphoreAdmission.Release] so panics in downstream handlers still
// free the permit.
func AdmissionMiddleware(ac *SemaphoreAdmission) func(http.Handler) http.Handler {
	if ac == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !ac.TryAcquire() {
				reqID := RequestIDFromContext(r.Context())
				jitter := retryAfterMin + rand.IntN(retryAfterMax-retryAfterMin+1) //nolint:gosec // G404: jitter, not security
				w.Header().Set("Retry-After", strconv.Itoa(jitter))
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.WriteHeader(http.StatusServiceUnavailable)
				// gosec G705 is a false positive: text/plain with nosniff,
				// and reqID is validated upstream by RequestIDMiddleware
				// against a safe charset, so no taint reaches a browser.
				_, _ = fmt.Fprintf(w, "service at capacity (request_id=%s)\n", reqID) //nolint:gosec // G705
				return
			}
			defer ac.Release()
			next.ServeHTTP(w, r)
		})
	}
}
