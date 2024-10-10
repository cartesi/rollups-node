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

	"github.com/cartesi/rollups-node/internal/advancer/machines"
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/nodemachine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

var (
	ErrInvalidMachines = errors.New("machines must not be nil")
	ErrNoApp           = errors.New("no machine for application")
)

type Inspector struct {
	machines Machines
}

type InspectResponse struct {
	Status     string   `json:"status"`
	Exception  string   `json:"exception"`
	Reports    []string `json:"reports"`
	InputIndex uint64   `json:"processed_input_count"`
}

// New instantiates a new Inspector.
func New(machines Machines) (*Inspector, error) {
	if machines == nil {
		return nil, ErrInvalidMachines
	}

	return &Inspector{machines: machines}, nil
}

func (inspect *Inspector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		dapp         Address
		payload      []byte
		err          error
		reports      []string
		status       string
		errorMessage string
	)

	if r.PathValue("dapp") == "" {
		slog.Info("Bad request",
			"service", "inspect",
			"err", "Missing application address")
		http.Error(w, "Missing application address", http.StatusBadRequest)
		return
	}

	dapp = common.HexToAddress(r.PathValue("dapp"))
	if r.Method == "POST" {
		payload, err = io.ReadAll(r.Body)
		if err != nil {
			slog.Info("Bad request",
				"service", "inspect",
				"err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if r.PathValue("payload") == "" {
			slog.Info("Bad request",
				"service", "inspect",
				"err", "Missing payload")
			http.Error(w, "Missing payload", http.StatusBadRequest)
			return
		}
		payload, err = hexutil.Decode(r.PathValue("payload"))
		if err != nil {
			slog.Info("Internal server error",
				"service", "inspect",
				"err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	result, err := inspect.process(r.Context(), dapp, payload)
	if err != nil {
		slog.Info("Internal server error",
			"service", "inspect",
			"err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, report := range result.Reports {
		reports = append(reports, hexutil.Encode(report))
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
		Status:     status,
		Exception:  errorMessage,
		Reports:    reports,
		InputIndex: *result.InputIndex,
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		slog.Info("Internal server error",
			"service", "inspect",
			"err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// process sends an inspect request to the machine
func (inspect *Inspector) process(
	ctx context.Context,
	app Address,
	query []byte) (*nodemachine.InspectResult, error) {
	// Asserts that the app has an associated machine.
	machine := inspect.machines.GetInspectMachine(app)
	if machine == nil {
		return nil, fmt.Errorf("%w %s", ErrNoApp, app.String())
	}

	res, err := machine.Inspect(ctx, query)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// ------------------------------------------------------------------------------------------------

type Machines interface {
	GetInspectMachine(app Address) machines.InspectMachine
}

type Machine interface {
	Inspect(_ context.Context, query []byte) (*nodemachine.InspectResult, error)
}
