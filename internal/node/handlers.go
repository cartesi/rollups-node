// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"log/slog"
	"net/http"

	"github.com/cartesi/rollups-node/internal/node/config"
)

func newHttpServiceHandler(c config.NodeConfig) http.Handler {
	handler := http.NewServeMux()
	handler.Handle("/healthz", http.HandlerFunc(healthcheckHandler))
	return handler
}

func healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Node received a healthcheck request")
	w.WriteHeader(http.StatusOK)
}
