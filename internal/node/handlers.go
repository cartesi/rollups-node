// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"log/slog"
	"net/http"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/inspect"
)

func newHttpServiceHandler(c config.NodeConfig, i *inspect.Inspector) http.Handler {
	handler := http.NewServeMux()
	handler.Handle("/healthz", http.HandlerFunc(healthcheckHandler))

	handler.Handle("/inspect/{dapp}", http.Handler(i))
	handler.Handle("/inspect/{dapp}/{payload}", http.Handler(i))

	return handler
}

func healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Node received a healthcheck request")
	w.WriteHeader(http.StatusOK)
}
