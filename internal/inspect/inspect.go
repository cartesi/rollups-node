// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/internal/manager"
	. "github.com/cartesi/rollups-node/internal/model"
	pkgmachine "github.com/cartesi/rollups-node/pkg/machine"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// maxPayloadSize is the maximum allowed inspect request body size.
// This matches the Cartesi Machine's CMIO RX buffer (2 MiB, defined as log2 size 21
// in the machine emulator's pma-defines.h). Payloads larger than this are rejected
// by the machine anyway, so there is no reason to read them into memory.
const maxPayloadSize = 1 << 21 // 2 MiB

// inspectResponseHeadroom is the time budget reserved for HTTP response
// serialization after the Cartesi Machine's inspect deadline fires.
const inspectResponseHeadroom = 30 * time.Second

const (
	inspectStatusFailed   = "Failed"
	inspectFailureMessage = "The node could not complete the inspection"
)

var (
	ErrInvalidMachines        = errors.New("machines must not be nil")
	ErrNoApp                  = errors.New("no application")
	ErrMachineNotReady        = errors.New("machine not ready for application")
	ErrForeclosedAppNoMachine = errors.New("application was foreclosed; machine unavailable")
)

type IInspectMachines interface {
	GetMachine(appId int64) (manager.MachineInstance, bool)
}

type InspectRepository interface {
	GetApplication(ctx context.Context, nameOrAddress string) (*Application, error)
}

type Inspector struct {
	IInspectMachines
	repository       InspectRepository
	Logger           *slog.Logger
	ServeMux         *http.ServeMux
	server           *http.Server
	admission        *service.SemaphoreAdmission
	deadlineWarnedMu sync.Mutex
	deadlineWarned   map[int64]struct{}
	// listen opens the HTTP listener. It defaults to net.Listen and is
	// overridden in tests so Serve() can be exercised against a pre-bound
	// listener whose actual address is known to the test.
	listen func(network, address string) (net.Listener, error)
}

type ReportResponse struct {
	Payload string `json:"payload"`
}

type InspectResponse struct {
	Status          string           `json:"status"`
	Error           string           `json:"error,omitempty"`
	ExceptionData   string           `json:"exception_data,omitempty"`
	Reports         []ReportResponse `json:"reports"`
	ProcessedInputs uint64           `json:"processed_input_count"`
}

// CreateInfo bundles the parameters for [NewInspector].
type CreateInfo struct {
	Repository InspectRepository
	Machines   IInspectMachines
	Address    string
	LogLevel   slog.Level
	LogPretty  bool
	// Admission is an optional HTTP-level concurrency gate. A nil value
	// disables admission control; the middleware chain treats nil as a
	// pass-through so wiring is uniform regardless of configuration.
	Admission *service.SemaphoreAdmission
	// CORSAllowedOrigins is the raw comma-separated origin allowlist.
	// Empty disables CORS entirely.
	CORSAllowedOrigins string
}

// NewInspector constructs an [Inspector] and its backing HTTP server
// with the node's canonical hardening chain applied via
// [service.NewServiceHandler]. See that helper for the middleware order
// and rationale.
//
// Use [Inspector.Serve] to run the HTTP server and [Inspector.Shutdown]
// to stop it gracefully.
func NewInspector(c CreateInfo) (*Inspector, error) {
	if c.Machines == nil {
		return nil, ErrInvalidMachines
	}

	logger := service.NewLogger(c.LogLevel, c.LogPretty).With("service", "inspect")
	inspector := &Inspector{
		IInspectMachines: c.Machines,
		repository:       c.Repository,
		Logger:           logger,
		deadlineWarned:   make(map[int64]struct{}),
		ServeMux:         http.NewServeMux(),
		admission:        c.Admission,
	}

	handler := service.NewServiceHandler(inspector, service.HandlerOptions{
		Logger:    logger,
		Admission: c.Admission,
		CORS: service.ParseCORSConfig(logger, c.CORSAllowedOrigins,
			[]string{"POST", "OPTIONS"}, []string{"Content-Type"}),
	})
	inspector.ServeMux.Handle("/inspect/{dapp}", handler)

	inspector.server = service.NewHTTPServer(c.Address, inspector.ServeMux, service.DefaultInspectOptions(), logger)
	inspector.listen = net.Listen
	service.StartupBindWarning(logger, "inspect", c.Address)

	return inspector, nil
}

// Serve opens the HTTP listener and runs the server. Returns nil on
// graceful shutdown.
func (inspect *Inspector) Serve() error {
	listener, err := inspect.listen("tcp", inspect.server.Addr)
	if err != nil {
		return err
	}
	inspect.Logger.Info("Listening", "address", listener.Addr().String())
	if err := inspect.server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully stops the inspect HTTP server, waiting for in-flight
// requests to complete or ctx to expire. Callers should not access the
// underlying *http.Server directly; exposing only Shutdown keeps the API
// surface minimal and prevents misuse (e.g. reaching for ListenAndServe,
// SetKeepAlivesEnabled, or mutating Handler after construction).
func (inspect *Inspector) Shutdown(ctx context.Context) error {
	return inspect.server.Shutdown(ctx)
}

// Admission returns the concurrency gate used by the inspect HTTP surface,
// or nil when admission control is disabled. This accessor gives the
// advancer (or a future metrics hook) a path to reach the inspect
// admission counters without threading the controller separately.
func (inspect *Inspector) Admission() *service.SemaphoreAdmission {
	return inspect.admission
}

func (inspect *Inspector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := service.RequestIDFromContext(r.Context())
	dapp := r.PathValue("dapp")

	if dapp == "" {
		inspect.Logger.Info("Bad request",
			"err", "Missing application address")
		http.Error(w, "Missing application address", http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodPost {
		inspect.Logger.Info("HTTP method not allowed", "application", dapp, "method", r.Method)
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Cap the request body at the machine's CMIO RX buffer size. MaxBytesReader
	// both enforces the limit and signals the server to close the connection
	// on over-limit so clients can't pipeline further requests on it.
	r.Body = http.MaxBytesReader(w, r.Body, maxPayloadSize)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			inspect.Logger.Info("Payload too large",
				"limit", maxPayloadSize,
				"application", dapp)
			http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
			return
		}
		inspect.Logger.Info("Bad request", "err", err, "application", dapp)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	inspect.Logger.Info("Got new inspect request", "application", dapp)

	app, machine, resolveErr := inspect.resolveApp(r.Context(), dapp)
	if resolveErr != nil {
		if errors.Is(resolveErr, ErrMachineNotReady) {
			inspect.Logger.Warn("Machine not ready", "application", dapp, "err", resolveErr)
			http.Error(w, "Machine not ready", http.StatusServiceUnavailable)
			return
		}
		if errors.Is(resolveErr, ErrForeclosedAppNoMachine) {
			inspect.Logger.Info("Foreclosed application machine unavailable", "application", dapp, "err", resolveErr)
			http.Error(w, "Application was foreclosed; machine unavailable", http.StatusServiceUnavailable)
			return
		}
		if errors.Is(resolveErr, ErrNoApp) {
			inspect.Logger.Info("Application not found", "application", dapp, "err", resolveErr)
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}
		service.WriteInternalError(r.Context(), w, inspect.Logger,
			fmt.Errorf("inspect resolve failed: %w", resolveErr))
		return
	}

	deadline := app.ExecutionParameters.InspectMaxDeadline + inspectResponseHeadroom
	if inspect.server != nil && deadline > inspect.server.WriteTimeout {
		inspect.warnDeadlineExceedsWriteTimeout(app, deadline)
	}
	ctx, cancel := context.WithTimeout(r.Context(), deadline)
	defer cancel()

	result, err := machine.Inspect(ctx, payload)
	if err != nil {
		if errors.Is(err, manager.ErrInspectAtCapacity) {
			inspect.Logger.Info("Application inspect at capacity",
				"application", dapp)
			http.Error(w, "Application inspect at capacity", http.StatusServiceUnavailable)
			return
		}
		service.WriteInternalError(ctx, w, inspect.Logger,
			fmt.Errorf("inspect processing failed: %w", err))
		return
	}

	response := inspect.buildInspectResponse(dapp, requestID, result)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Headers are already flushed; we can only log. Writing a 500 via
		// WriteInternalError here would produce "superfluous WriteHeader"
		// warnings and garble the response.
		inspect.Logger.Error("failed to encode inspect response",
			"err", err,
			"application", dapp,
			"request_id", requestID,
		)
		return
	}
	inspect.Logger.Info("Request executed",
		"status", response.Status,
		"application", dapp)
}

// buildInspectResponse maps the manager result to the public inspect contract.
// Machine execution details remain in trusted logs; anonymous clients receive
// a stable sanitized failure message and any reports emitted before the stop.
func (inspect *Inspector) buildInspectResponse(
	dapp string,
	requestID string,
	result *manager.InspectResult,
) InspectResponse {
	response := InspectResponse{
		Reports:         make([]ReportResponse, 0, len(result.Reports)),
		ProcessedInputs: result.ProcessedInputs,
	}
	for _, report := range result.Reports {
		response.Reports = append(response.Reports, ReportResponse{Payload: hexutil.Encode(report)})
	}

	if result.Error != nil {
		response.Status = inspectStatusFailed
		response.Error = inspectFailureMessage
		// Inspect is an anonymous endpoint. Keep machine positions and local
		// policy values in operator logs so clients cannot discover configured
		// limits or price a resource-exhaustion input.
		inspect.Logger.Warn("Machine failed while inspecting",
			"application", dapp,
			"error", result.Error,
			"request_id", requestID,
		)
		return response
	}

	switch result.Status {
	case pkgmachine.CompletionStatusAccepted:
		response.Status = "Accepted"
	case pkgmachine.CompletionStatusRejected:
		response.Status = "Rejected"
	case pkgmachine.CompletionStatusException:
		response.Status = "Exception"
		response.Error = "The machine raised an exception while inspecting"
		response.ExceptionData = hexutil.Encode(result.ExceptionData)
		inspect.Logger.Debug("Machine returned a guest inspect exception",
			"application", dapp,
			"request_id", requestID,
		)
	case pkgmachine.CompletionStatusHalted:
		response.Status = "MachineHalted"
		inspect.Logger.Debug("Machine halted while inspecting",
			"application", dapp,
			"request_id", requestID,
		)
	case pkgmachine.CompletionStatusUnknown:
		response.Status = inspectStatusFailed
		response.Error = inspectFailureMessage
		inspect.Logger.Warn("Machine returned an incomplete inspect result",
			"application", dapp,
			"request_id", requestID,
		)
	default:
		response.Status = inspectStatusFailed
		response.Error = inspectFailureMessage
		inspect.Logger.Warn("Machine returned an unknown inspect status",
			"application", dapp,
			"status", result.Status,
			"request_id", requestID,
		)
	}
	return response
}

func (inspect *Inspector) warnDeadlineExceedsWriteTimeout(app *Application, deadline time.Duration) {
	inspect.deadlineWarnedMu.Lock()
	defer inspect.deadlineWarnedMu.Unlock()
	if _, seen := inspect.deadlineWarned[app.ID]; seen {
		return
	}
	inspect.deadlineWarned[app.ID] = struct{}{}
	inspect.Logger.Warn(
		"application inspect deadline exceeds HTTP WriteTimeout; response may be truncated",
		"application", app.Name,
		"inspect_max_deadline", app.ExecutionParameters.InspectMaxDeadline,
		"response_headroom", inspectResponseHeadroom,
		"effective_deadline", deadline,
		"http_write_timeout", inspect.server.WriteTimeout,
	)
}

func (inspect *Inspector) resolveApp(
	ctx context.Context,
	nameOrAddress string,
) (*Application, manager.MachineInstance, error) {
	app, err := inspect.repository.GetApplication(ctx, nameOrAddress)
	if app == nil {
		if err != nil {
			return nil, nil, fmt.Errorf("%w %s", err, nameOrAddress)
		}
		return nil, nil, fmt.Errorf("%w %s", ErrNoApp, nameOrAddress)
	}
	machine, exists := inspect.GetMachine(app.ID)
	if !exists {
		if app.IsForeclosed() {
			return nil, nil, fmt.Errorf("%w %s", ErrForeclosedAppNoMachine, nameOrAddress)
		}
		return nil, nil, fmt.Errorf("%w %s", ErrMachineNotReady, nameOrAddress)
	}
	return app, machine, nil
}
