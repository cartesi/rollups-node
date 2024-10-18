// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"log/slog"
	"net/http"
)

func healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Node received a healthcheck request")
	w.WriteHeader(http.StatusOK)
}
