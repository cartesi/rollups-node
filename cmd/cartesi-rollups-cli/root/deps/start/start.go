// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package start

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/deps"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "start",
	Short:   "start all deps",
	Example: examples,
	Run:     run,
}

const examples = `# Start all deps:
cartesi-rollups-cli deps start`

var (
	postgresDockerImage string
	postgresPort        string
	postgresPassword    string

	devnetDockerImage string
	devnetPort        string
)

func init() {
	Cmd.Flags().StringVar(&postgresDockerImage, "postgres-docker-image",
		"",
		"Postgress docker image name")

	Cmd.Flags().StringVar(&postgresPort, "postgres-mapped-port",
		"",
		"Postgres local listening port number")

	Cmd.Flags().StringVar(&postgresPassword, "postgres-password",
		"",
		"Postgres password")

	Cmd.Flags().StringVar(&devnetDockerImage, "devnet-docker-image",
		"",
		"Devnet docker image name")

	Cmd.Flags().StringVar(&devnetPort, "devnet-mapped-port",
		"",
		"devnet local listening port number")
}

func run(cmd *cobra.Command, args []string) {

	ctx := context.Background()

	depsConfig := deps.NewDefaultDepsConfig().
		WithPostgresDockerImage(postgresDockerImage).
		WithPostgresPort(postgresPort).
		WithPostgresPassword(postgresPassword).
		WithDevenetDockerImage(devnetDockerImage).
		WithDevenetPort(devnetPort)

	depContainers, err := deps.Run(ctx, *depsConfig)

	if err != nil {
		cobra.CheckErr(err)
	}

	config.InfoLogger.Println("all deps are up")

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	config.InfoLogger.Println("Terminating deps...")
	errors := deps.Terminate(ctx, depContainers)
	for _, containerError := range errors {
		cobra.CheckErr(containerError)
	}

	os.Exit(0)

}
