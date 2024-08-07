// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deps

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/cartesi/rollups-node/internal/deps"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "run-deps",
	Short:   "Run node dependencies with Docker",
	Example: examples,
	Run:     run,
}

const examples = `# Run all deps:
cartesi-rollups-cli run-deps`

var depsConfig = deps.NewDefaultDepsConfig()
var verbose = false

func init() {
	Cmd.Flags().StringVar(&depsConfig.Postgres.DockerImage, "postgres-docker-image",
		deps.DefaultPostgresDockerImage,
		"Postgress docker image name")

	Cmd.Flags().StringVar(&depsConfig.Postgres.Port, "postgres-mapped-port",
		deps.DefaultPostgresPort,
		"Postgres local listening port number")

	Cmd.Flags().StringVar(&depsConfig.Postgres.Password, "postgres-password",
		deps.DefaultPostgresPassword,
		"Postgres password")

	Cmd.Flags().StringVar(&depsConfig.Devnet.DockerImage, "devnet-docker-image",
		deps.DefaultDevnetDockerImage,
		"Devnet docker image name")

	Cmd.Flags().StringVar(&depsConfig.Devnet.Port, "devnet-mapped-port",
		deps.DefaultDevnetPort,
		"Devnet local listening port number")

	Cmd.Flags().StringVar(&depsConfig.Devnet.BlockTime, "devnet-block-time",
		deps.DefaultDevnetBlockTime,
		"Devnet mining block time")

	Cmd.Flags().BoolVar(&depsConfig.Devnet.NoMining, "devnet-no-mining",
		deps.DefaultDevnetNoMining,
		"Devnet disable mining")

	Cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose logs")
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if verbose {
		// setup log
		opts := &tint.Options{
			Level:     slog.LevelDebug,
			AddSource: true,
			NoColor:   false || !isatty.IsTerminal(os.Stdout.Fd()),
		}
		handler := tint.NewHandler(os.Stdout, opts)
		logger := slog.New(handler)
		slog.SetDefault(logger)
	}

	depsContainers, err := deps.Run(ctx, *depsConfig)
	cobra.CheckErr(err)

	slog.Info("All dependencies are up")

	<-ctx.Done()

	err = deps.Terminate(context.Background(), depsContainers)
	cobra.CheckErr(err)
}
