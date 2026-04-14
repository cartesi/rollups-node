// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/manager"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/stretchr/testify/require"
)

// newInspectorForTest constructs an Inspector via NewInspector with a mock
// repository and machine set, but returns only the http.Handler — the real
// http.Server is never started. Tests exercise it with httptest.
//
// When machineErr is non-nil, the mock machine's Inspect call returns it,
// so tests can cover the internal-error path.
func newInspectorForTest(t *testing.T, machineErr error) (*Inspector, *Application) {
	t.Helper()

	app := &Application{
		ID:                  1,
		IApplicationAddress: randomAddress(),
		Name:                "test-app",
	}
	repo := newMockRepository()
	repo.apps = append(repo.apps, app)

	mm := newMockMachines()
	mm.Map[1] = MockMachine{application: app}

	insp, err := NewInspector(CreateInfo{
		Repository: repo,
		Machines:   hardeningMachines{MachinesMock: mm, err: machineErr},
		Address:    "127.0.0.1:0",
		LogLevel:   slog.LevelError,
		LogPretty:  false,
	})
	require.NoError(t, err)
	return insp, app
}

// hardeningMachines wraps MachinesMock so an injected error propagates out
// of the MockMachine.Inspect call used by tests. The underlying MockMachine
// never returns an error on its own.
type hardeningMachines struct {
	*MachinesMock
	err error
}

func (h hardeningMachines) GetMachine(appID int64) (manager.MachineInstance, bool) {
	inst, ok := h.MachinesMock.GetMachine(appID)
	if !ok {
		return nil, false
	}
	if h.err != nil {
		return &erroringMachine{inner: inst, err: h.err}, true
	}
	return inst, true
}

type erroringMachine struct {
	inner manager.MachineInstance
	err   error
}

func (m *erroringMachine) Inspect(_ context.Context, _ []byte) (*InspectResult, error) {
	if m.err == errPanicSentinel {
		panic("boom-from-machine")
	}
	return nil, m.err
}

// errPanicSentinel is a marker value understood by erroringMachine.Inspect:
// when passed as the machineErr to newInspectorForTest, the mock machine
// panics instead of returning an error, so tests can exercise the
// RecoverMiddleware branch without monkey-patching the inspect handler.
var errPanicSentinel = errors.New("sentinel: make machine panic")

// Forward the rest to the inner machine so the type satisfies the interface
// without reimplementing the stubs. Tests only reach Inspect.
func (m *erroringMachine) Advance(ctx context.Context, input []byte, a, b uint64, c bool) (*AdvanceResult, error) {
	return m.inner.Advance(ctx, input, a, b, c)
}
func (m *erroringMachine) Application() *Application { return m.inner.Application() }
func (m *erroringMachine) ProcessedInputs() uint64   { return m.inner.ProcessedInputs() }
func (m *erroringMachine) OutputsProof(ctx context.Context) (*OutputsProof, error) {
	return m.inner.OutputsProof(ctx)
}
func (m *erroringMachine) Synchronize(ctx context.Context, repo manager.MachineRepository, batchSize uint64) error {
	return m.inner.Synchronize(ctx, repo, batchSize)
}
func (m *erroringMachine) CreateSnapshot(ctx context.Context, processedInputs uint64, path string) error {
	return m.inner.CreateSnapshot(ctx, processedInputs, path)
}
func (m *erroringMachine) Hash(ctx context.Context) ([32]byte, error) { return m.inner.Hash(ctx) }
func (m *erroringMachine) Close() error                               { return m.inner.Close() }

// -----------------------------------------------------------------------------

func TestInspector_NewWithCreateInfo(t *testing.T) {
	insp, _ := newInspectorForTest(t, nil)
	// Package-internal access: the hardened http.Server is unexported and
	// tests pin its fields directly rather than via a public accessor.
	srv := insp.server
	require.NotNil(t, srv)

	opts := service.DefaultInspectOptions()
	require.Equal(t, opts.ReadHeaderTimeout, srv.ReadHeaderTimeout)
	require.Equal(t, opts.ReadTimeout, srv.ReadTimeout)
	require.Equal(t, opts.WriteTimeout, srv.WriteTimeout)
	require.Equal(t, opts.IdleTimeout, srv.IdleTimeout)
	require.Equal(t, opts.MaxHeaderBytes, srv.MaxHeaderBytes)
}

func TestInspector_NewRejectsNilMachines(t *testing.T) {
	_, err := NewInspector(CreateInfo{
		Repository: newMockRepository(),
		Machines:   nil,
		Address:    "127.0.0.1:0",
	})
	require.ErrorIs(t, err, ErrInvalidMachines)
}

func TestInspector_OversizedPayloadReturns413(t *testing.T) {
	insp, app := newInspectorForTest(t, nil)
	body := bytes.NewReader(make([]byte, maxPayloadSize+1))
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/inspect/%s", app.Name), body)
	rr := httptest.NewRecorder()
	insp.ServeMux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	require.Contains(t, rr.Body.String(), "Payload too large")
}

func TestInspector_ExactBoundaryAccepted(t *testing.T) {
	insp, app := newInspectorForTest(t, nil)
	body := bytes.NewReader(make([]byte, maxPayloadSize))
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/inspect/%s", app.Name), body)
	rr := httptest.NewRecorder()
	insp.ServeMux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body at exact limit must be accepted")
}

func TestInspector_GETReturns405WithAllowHeader(t *testing.T) {
	insp, app := newInspectorForTest(t, nil)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inspect/%s", app.Name), nil)
	rr := httptest.NewRecorder()
	insp.ServeMux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	require.Equal(t, http.MethodPost, rr.Header().Get("Allow"))
}

func TestInspector_PUTReturns405WithAllowHeader(t *testing.T) {
	insp, app := newInspectorForTest(t, nil)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/inspect/%s", app.Name), nil)
	rr := httptest.NewRecorder()
	insp.ServeMux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	require.Equal(t, http.MethodPost, rr.Header().Get("Allow"))
}

func TestInspector_InternalErrorBodyIsGeneric(t *testing.T) {
	secret := errors.New("secret-credentials-xyzzy")
	insp, app := newInspectorForTest(t, secret)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/inspect/%s", app.Name),
		strings.NewReader("hello"))
	rr := httptest.NewRecorder()
	insp.ServeMux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Contains(t, rr.Body.String(), "Internal server error (request_id=")
	require.NotContains(t, rr.Body.String(), "secret-credentials-xyzzy",
		"internal error body must never leak err.Error()")
}

func TestInspector_InternalErrorIncludesRequestID(t *testing.T) {
	insp, app := newInspectorForTest(t, errors.New("boom"))

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/inspect/%s", app.Name),
		strings.NewReader("hello"))
	req.Header.Set("X-Request-ID", "pinned-id-42")
	rr := httptest.NewRecorder()
	insp.ServeMux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Contains(t, rr.Body.String(), "request_id=pinned-id-42")
	require.Equal(t, "pinned-id-42", rr.Header().Get("X-Request-ID"))
}

// TestInspector_ChainOrder_RecoverCoversRequestID pins the customer-
// visible consequences of the chain order built by
// [service.NewServiceHandler]: a panic from deep in the handler chain
// must turn into a clean 500 Internal Server Error (not a dropped TCP
// connection), and the response must still carry X-Request-ID on the way
// out so callers and log aggregators retain correlation.
//
// The precise semantics of how RecoverMiddleware reads the request id
// from the outer request (via the shared ResponseWriter header, not
// r.Context()) are a property of [service.RecoverMiddleware] itself and
// are documented and tested there.
func TestInspector_ChainOrder_RecoverCoversRequestID(t *testing.T) {
	insp, app := newInspectorForTest(t, errPanicSentinel)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/inspect/%s", app.Name),
		strings.NewReader("hello"))
	req.Header.Set("X-Request-ID", "chain-order-test-id")
	rr := httptest.NewRecorder()

	require.NotPanics(t, func() {
		insp.ServeMux.ServeHTTP(rr, req)
	}, "panic in handler must be caught by RecoverMiddleware, not propagate to the test")

	require.Equal(t, http.StatusInternalServerError, rr.Code,
		"Recover must turn handler panics into a 500")
	require.Equal(t, "chain-order-test-id", rr.Header().Get("X-Request-ID"),
		"X-Request-ID must be echoed on the 500 response so clients can correlate")
}

func TestInspector_HappyPathStillWorks(t *testing.T) {
	insp, app := newInspectorForTest(t, nil)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/inspect/%s", app.Name),
		strings.NewReader("hello"))
	rr := httptest.NewRecorder()
	insp.ServeMux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), `"status":"Accepted"`)
}

func TestInspector_EmptyDappPathReturns404(t *testing.T) {
	insp, _ := newInspectorForTest(t, nil)
	// An empty dapp path value does not match the /inspect/{dapp} pattern,
	// so this exercises the 404 branch. Ensure the request instead just
	// does not match the inspect route.
	req := httptest.NewRequest(http.MethodPost, "/inspect/", strings.NewReader("x"))
	rr := httptest.NewRecorder()
	insp.ServeMux.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

// TestInspector_RealServer_PayloadTooLarge runs the oversized-body path
// through a real httptest.Server so the full middleware chain and
// MaxBytesReader wire up correctly end-to-end. This is the keystone test
// for the responseWriterTap.Unwrap() behaviour; if the tap ever loses
// Unwrap, MaxBytesReader silently stops enforcing connection-close and
// this test is the first thing to notice.
func TestInspector_RealServer_PayloadTooLarge(t *testing.T) {
	insp, app := newInspectorForTest(t, nil)

	srv := httptest.NewServer(insp.ServeMux)
	defer srv.Close()

	body := bytes.NewReader(make([]byte, maxPayloadSize+1))
	resp, err := http.Post(
		fmt.Sprintf("%s/inspect/%s", srv.URL, app.Name),
		"application/octet-stream",
		body)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	respBody, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(respBody), "Payload too large")
}

// TestInspector_ServeReturnsNilOnGracefulShutdown verifies the new Serve()
// method swallows http.ErrServerClosed and returns nil, matching the
// contract the advancer relies on.
func TestInspector_ServeReturnsNilOnGracefulShutdown(t *testing.T) {
	insp, _ := newInspectorForTest(t, nil)

	// Pre-bind a listener on an OS-assigned port so the test knows the
	// real address (http.Server never rewrites Addr after binding to :0).
	// Inject it via the listen hook so Serve() uses it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	insp.listen = func(string, string) (net.Listener, error) { return listener, nil }

	serveErr := make(chan error, 1)
	go func() { serveErr <- insp.Serve() }()

	// Wait until the server is actually accepting connections before
	// shutting down. Any HTTP response (including 404) confirms the
	// accept loop is live.
	deadline := time.Now().Add(2 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr)
		if err == nil {
			resp.Body.Close()
			ready = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.True(t, ready, "server did not start listening in time")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, insp.Shutdown(ctx))

	select {
	case err := <-serveErr:
		require.NoError(t, err, "Serve() must return nil on graceful shutdown")
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after Shutdown")
	}
}
