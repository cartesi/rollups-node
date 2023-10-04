// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"os"
	"os/exec"
	"syscall"

	"github.com/cartesi/rollups-node/internal/pkg/logger"
)

const (
	serviceName = "graphql-server"
	binaryName  = "cartesi-rollups-graphql-server"
)

type GraphQLService struct{}

func (g GraphQLService) Start(ctx context.Context) error {
	cmd := exec.Command(binaryName)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		logger.Debug.Printf("%v: %v\n", g.String(), ctx.Err())
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			msg := "%v: failed to send SIGTERM to %v\n"
			logger.Error.Printf(msg, g.String(), binaryName)
		}
	}()

	err := cmd.Wait()
	if err != nil && cmd.ProcessState.ExitCode() != int(syscall.SIGTERM) {
		return err
	}
	return nil
}

func (g GraphQLService) String() string {
	return serviceName
}
