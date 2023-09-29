package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestGraphQLService(t *testing.T) {
	t.Run("it stops when the context is cancelled", func(t *testing.T) {
		service := GraphQLService{}
		setupEnvVars()
		ctx, cancel := context.WithCancel(context.Background())
		exit := make(chan error)

		go func() {
			<-time.After(100 * time.Millisecond)
			if err := service.Start(ctx); err != nil {
				exit <- err
			}
		}()

		<-time.After(200 * time.Millisecond)
		cancel()

		err := <-exit
		exitError, ok := err.(*exec.ExitError)
		if !ok || !assertExitErrorWasCausedBy(exitError, syscall.SIGTERM) {
			fmt.Printf("service exited for the wrong reason: %v", err)
			t.FailNow()
		}
	})
}

func setupEnvVars() {
	abs, _ := filepath.Abs("../../../offchain/target/debug")
	os.Setenv("PATH", abs)
	os.Setenv("POSTGRES_PASSWORD", "pw")
}

func assertExitErrorWasCausedBy(err *exec.ExitError, signal syscall.Signal) bool {
	status := err.Sys().(syscall.WaitStatus)
	return status.Signal() == signal
}
