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

	"github.com/cartesi/rollups-node/internal/manager"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/services"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// maxPayloadSize is the maximum allowed inspect request body size.
// This matches the Cartesi Machine's CMIO RX buffer (2 MiB, defined as log2 size 21
// in the machine emulator's pma-defines.h). Payloads larger than this are rejected
// by the machine anyway, so there is no reason to read them into memory.
const maxPayloadSize = 1 << 21 // 2 MiB

var (
	ErrInvalidMachines = errors.New("machines must not be nil")
	ErrNoApp           = errors.New("no application")
	ErrMachineNotReady = errors.New("machine not ready for application")
)

type IInspectMachines interface {
	GetMachine(appId int64) (manager.MachineInstance, bool)
}

type InspectRepository interface {
	GetApplication(ctx context.Context, nameOrAddress string) (*Application, error)
}

type Inspector struct {
	IInspectMachines
	repository InspectRepository
	Logger     *slog.Logger
	ServeMux   *http.ServeMux
	server     *http.Server
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
	Exception       string           `json:"exception,omitempty"`
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
}

// NewInspector constructs an [Inspector] and its backing HTTP server with
// the standard hardening chain applied:
//
//	RecoverMiddleware -> RequestIDMiddleware -> CorsMiddleware -> AdmissionMiddleware -> Inspector
//
// RecoverMiddleware is the outermost wrapper so it also catches panics that
// occur during request-id generation (e.g. an entropy failure inside
// uuid.NewString). Without this ordering such a panic would escape to
// net/http's default goroutine recover, dropping the connection with no
// structured log and no 500 response.
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
		ServeMux:         http.NewServeMux(),
	}

	var handler http.Handler = inspector
	handler = services.CorsMiddleware(handler)
	handler = service.RequestIDMiddleware(handler)
	handler = service.RecoverMiddleware(logger)(handler)
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

func (inspect *Inspector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		dapp         string
		payload      []byte
		err          error
		reports      []ReportResponse
		status       string
		errorMessage string
	)

	if r.PathValue("dapp") == "" {
		inspect.Logger.Info("Bad request",
			"err", "Missing application address")
		http.Error(w, "Missing application address", http.StatusBadRequest)
		return
	}

	dapp = r.PathValue("dapp")
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
	payload, err = io.ReadAll(r.Body)
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
	result, err := inspect.process(r.Context(), dapp, payload)
	if err != nil {
		if errors.Is(err, ErrMachineNotReady) {
			inspect.Logger.Warn("Machine not ready", "application", dapp, "err", err)
			http.Error(w, "Machine not ready", http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, ErrNoApp) {
			inspect.Logger.Info("Application not found", "application", dapp, "err", err)
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}
		service.WriteInternalError(r.Context(), w, inspect.Logger, fmt.Errorf("inspect processing failed: %w", err))
		return
	}

	for _, report := range result.Reports {
		reports = append(reports, ReportResponse{Payload: hexutil.Encode(report)})
	}

	if result.Accepted {
		status = "Accepted"
	} else {
		status = "Rejected"
	}

	if result.Error != nil {
		status = "Exception"
		errorMessage = fmt.Sprintf("Error on the machine while inspecting: %s", result.Error)
	}

	response := InspectResponse{
		Status:          status,
		Exception:       errorMessage,
		Reports:         reports,
		ProcessedInputs: result.ProcessedInputs,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Headers are already flushed; we can only log. Writing a 500 via
		// WriteInternalError here would produce "superfluous WriteHeader"
		// warnings and garble the response.
		inspect.Logger.Error("failed to encode inspect response",
			"err", err,
			"application", dapp,
			"request_id", service.RequestIDFromContext(r.Context()),
		)
		return
	}
	inspect.Logger.Info("Request executed",
		"status", status,
		"application", dapp)
}

// process sends an inspect request to the machine
func (inspect *Inspector) process(
	ctx context.Context,
	nameOrAddress string,
	query []byte) (*InspectResult, error) {

	app, err := inspect.repository.GetApplication(ctx, nameOrAddress)
	if app == nil {
		if err != nil {
			return nil, fmt.Errorf("%w %s", err, nameOrAddress)
		}
		return nil, fmt.Errorf("%w %s", ErrNoApp, nameOrAddress)
	}
	// Asserts that the app has an associated machine.
	machine, exists := inspect.GetMachine(app.ID)
	if !exists {
		return nil, fmt.Errorf("%w %s", ErrMachineNotReady, nameOrAddress)
	}

	res, err := machine.Inspect(ctx, query)
	if err != nil {
		return nil, err
	}

	return res, nil
}
