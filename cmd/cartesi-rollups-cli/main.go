// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package contains the cartesi-rollups CLI binary.
package main

import (
	"log/slog"
	"os"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root"
	"github.com/lmittmann/tint"
)

func main() {
	opts := &tint.Options{
		Level: slog.LevelInfo,
	}
	handler := tint.NewHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	err := root.Cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
