// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package espressononce

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrInvalidMachines = errors.New("machines must not be nil")
	ErrNoApp           = errors.New("no machine for application")
)

type NonceQuerier struct {
}

func NewNonceQuerier() (*NonceQuerier, error) {

	return &NonceQuerier{}, nil
}

func (q *NonceQuerier) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		senderAddress      Address
		applicationAddress Address
	)

	if r.PathValue("sender") == "" {
		slog.Info("Bad request",
			"service", "espresso nonce",
			"err", "Missing sender address")
		http.Error(w, "Missing sender address", http.StatusBadRequest)
		return
	}
	senderAddress = common.HexToAddress(r.PathValue("sender"))

	if r.PathValue("dapp") == "" {
		slog.Info("Bad request",
			"service", "espresso nonce",
			"err", "Missing application address")
		http.Error(w, "Missing application address", http.StatusBadRequest)
		return
	}
	applicationAddress = common.HexToAddress(r.PathValue("dapp"))

	nonce := q.process(r.Context(), senderAddress, applicationAddress)

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(nonce)
	if err != nil {
		slog.Info("Internal server error",
			"service", "espresso nonce querier",
			"err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (q *NonceQuerier) process(
	ctx context.Context,
	senderAddress Address,
	applicationAddress Address) uint64 {
	c := config.FromEnv()

	database, err := repository.Connect(ctx, c.PostgresEndpoint.Value)
	if err != nil {
		slog.Error("EVM Reader couldn't connect to the database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if database == nil {
		panic("Database was not initialized")
	}

	nonce, err := database.GetEspressoNonce(ctx, senderAddress, applicationAddress)
	if err != nil {
		slog.Error("failed to get espresso nonce", "error", err)
		os.Exit(1)
	}

	return nonce
}
