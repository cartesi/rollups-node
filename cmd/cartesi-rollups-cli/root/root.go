// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"log/slog"
	"os"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/db"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/execute"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/increasetime"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/inspect"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/mine"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/send"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/validate"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "cartesi-rollups-cli",
	Short: "Command line interface for Cartesi Rollups",
	Long: `This command line interface provides functionality to help develop and debug the
Cartesi Rollups node.`,
}

var verbose bool

func init() {

	cobra.OnInitialize(setup)

	Cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	Cmd.AddCommand(send.Cmd)
	Cmd.AddCommand(read.Cmd)
	Cmd.AddCommand(inspect.Cmd)
	Cmd.AddCommand(increasetime.Cmd)
	Cmd.AddCommand(validate.Cmd)
	Cmd.AddCommand(execute.Cmd)
	Cmd.AddCommand(mine.Cmd)
	Cmd.AddCommand(app.Cmd)
	Cmd.AddCommand(db.Cmd)
	Cmd.DisableAutoGenTag = true
}

func setup() {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	opts := &tint.Options{
		Level:      logLevel,
		AddSource:  logLevel == slog.LevelDebug,
		TimeFormat: "2006-01-02T15:04:05.000", // RFC3339 with milliseconds and without timezone
	}
	handler := tint.NewHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Debug("Verbose log enabled")
}
