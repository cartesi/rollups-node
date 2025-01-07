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
	"net/http"
	"os"
	"time"

	"github.com/cartesi/rollups-node/internal/advancer/machines"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/services"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

var (
	ErrInvalidMachines = errors.New("machines must not be nil")
	ErrNoApp           = errors.New("no application")
)

type IInspectMachines interface {
	GetInspectMachine(appId int64) (machines.InspectMachine, bool)
}

type IInspectMachine interface {
	Inspect(_ context.Context, query []byte) (*InspectResult, error)
}

type InspectRepository interface {
	GetApplication(ctx context.Context, nameOrAddress string) (*Application, error)
}

type Inspector struct {
	IInspectMachines
	repository InspectRepository
	Logger     *slog.Logger
	ServeMux   *http.ServeMux
}

type ReportResponse struct {
	Payload string `json:"payload"`
}

type InspectResponse struct {
	Status          string           `json:"status"`
	Exception       string           `json:"exception"`
	Reports         []ReportResponse `json:"reports"`
	ProcessedInputs uint64           `json:"processed_input_count"`
}

func NewInspector(
	repo InspectRepository,
	machines IInspectMachines,
	address string,
	logLevel service.LogLevel,
	logPretty bool,
) (*Inspector, *http.Server, func() error) {
	logger := service.NewLogger(slog.Level(logLevel), logPretty)
	logger = logger.With("service", "inspect")
	inspector := &Inspector{
		IInspectMachines: machines,
		repository:       repo,
		Logger:           logger,
		ServeMux:         http.NewServeMux(),
	}

	inspector.ServeMux.Handle("/inspect/{dapp}", services.CorsMiddleware(http.Handler(inspector)))

	server := &http.Server{
		Addr:     address,
		Handler:  inspector.ServeMux,
		ErrorLog: slog.NewLogLogger(inspector.Logger.Handler(), slog.LevelError),
	}

	return inspector, server, func() error {
		maxRetries := 3                  // FIXME: should go to config
		retryInterval := 5 * time.Second // FIXME: should go to config
		inspector.Logger.Info("Create", "LogLevel", logLevel, "pid", os.Getpid())
		inspector.Logger.Info("Listening", "address", address)
		var err error = nil
		for retry := 0; retry <= maxRetries; retry++ {
			switch err = server.ListenAndServe(); err {
			case http.ErrServerClosed:
				return nil
			default:
				inspector.Logger.Error("http",
					"error", err,
					"try", retry+1,
					"maxRetries", maxRetries,
					"error", err)
			}
			time.Sleep(retryInterval)
		}
		return err
	}
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
	if r.Method == "POST" {
		payload, err = io.ReadAll(r.Body)
		if err != nil {
			inspect.Logger.Info("Bad request", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		inspect.Logger.Info("HTTP method not supported", "application", dapp)
		http.Error(w, "HTTP method not supported", http.StatusNotFound)
		return
	}

	inspect.Logger.Info("Got new inspect request", "application", dapp)
	result, err := inspect.process(r.Context(), dapp, payload)
	if err != nil {
		if errors.Is(err, ErrNoApp) {
			inspect.Logger.Error("Application not found", "application", dapp, "err", err)
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}
		inspect.Logger.Error("Internal server error", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		inspect.Logger.Error("Internal server error",
			"err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	machine, exists := inspect.GetInspectMachine(app.ID)
	if !exists {
		return nil, fmt.Errorf("%w %s", ErrNoApp, nameOrAddress)
	}

	res, err := machine.Inspect(ctx, query)
	if err != nil {
		return nil, err
	}

	return res, nil
}
