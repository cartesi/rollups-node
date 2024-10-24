// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/cartesi/rollups-node/internal/advancer"
	"github.com/cartesi/rollups-node/internal/advancer/config"
	"github.com/cartesi/rollups-node/internal/advancer/machines"
	"github.com/cartesi/rollups-node/internal/inspect"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/services"
	"github.com/cartesi/rollups-node/internal/services/startup"

	"github.com/spf13/cobra"
)

const CMD_NAME = "advancer"

var (
	buildVersion = "devel"
	Cmd          = &cobra.Command{
		Use:   CMD_NAME,
		Short: "Runs the Advancer",
		Long:  "Runs the Advancer in standalone mode",
		RunE:  run,
	}
)

func getDatabase(ctx context.Context, endpoint string) (*repository.Database, error) {
	database, err := repository.Connect(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the database: %w", err)
	}

	return database, nil
}

func healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Advancer received a healthcheck request")
	w.WriteHeader(http.StatusOK)
}

func run(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := config.GetAdvancerConfig()
	startup.ConfigLogs(c.LogLevel, c.LogPrettyEnabled)

	slog.Info("Starting the Cartesi Rollups Node Advancer", "version", buildVersion, "config", c)

	database, err := getDatabase(ctx, c.PostgresEndpoint.Value)
	if err != nil {
		return err
	}
	defer database.Close()

	repo := &repository.MachineRepository{Database: database}

	machines, err := machines.Load(ctx, repo, c.MachineServerVerbosity)
	if err != nil {
		return fmt.Errorf("failed to load the machines: %w", err)
	}
	defer machines.Close()

	inspector, err := inspect.New(machines)
	if err != nil {
		return fmt.Errorf("failed to create the inspector: %w", err)
	}

	advancer, err := advancer.New(machines, repo)
	if err != nil {
		return fmt.Errorf("failed to create the advancer: %w", err)
	}

	poller, err := advancer.Poller(c.AdvancerPollingInterval)
	if err != nil {
		return fmt.Errorf("failed to create the advancer service: %w", err)
	}

	serveMux := http.NewServeMux()
	serveMux.Handle("/healthz", http.HandlerFunc(healthcheckHandler))
	serveMux.Handle("/inspect/{dapp}", http.Handler(inspector))
	serveMux.Handle("/inspect/{dapp}/{payload}", http.Handler(inspector))

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%v:%v", c.HttpAddress, c.HttpPort),
		Handler: services.CorsMiddleware(serveMux),
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Could not listen on %s: %v\n", httpServer.Addr, err)
			stop()
		}
	}()

	return poller.Start(ctx)
}
