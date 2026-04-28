// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

const jsonrpcShutdownTimeout = 5 * time.Second

// -----------------------------------------------------------------------------
// Service Implementation
// -----------------------------------------------------------------------------

// Service implements the IService interface.
type Service struct {
	service.ServiceTemplate
	repository repository.Repository
	server     *http.Server
	admission  *service.SemaphoreAdmission
	inputABI   *abi.ABI
	outputABI  *abi.ABI
	// listen opens the HTTP listener. It defaults to net.Listen and is
	// overridden in tests so Serve() can be exercised without real sockets.
	listen func(network, address string) (net.Listener, error)
}

type CreateInfo struct {
	service.ServiceConfigs

	Config config.JsonrpcConfig

	Repository repository.Repository
}

func Create(ctx context.Context, c *CreateInfo) (service.IService, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}

	err = service.InitServiceTemplate(&c.ServiceConfigs, &s.ServiceTemplate, s)
	if err != nil {
		return nil, err
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on validator service Create is nil")
	}

	s.inputABI, err = inputs.InputsMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	s.outputABI, err = outputs.OutputsMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	if c.Config.JsonrpcMaxInflight > 0 {
		s.admission = service.NewSemaphoreAdmission(c.Config.JsonrpcMaxInflight)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", s.handleRPC)

	handler := service.NewServiceHandler(mux, service.HandlerOptions{
		Logger:    s.Logger,
		Admission: s.admission,
		CORS: service.ParseCORSConfig(s.Logger, c.Config.JsonrpcCorsAllowedOrigins,
			[]string{"POST", "OPTIONS"}, []string{"Content-Type"}),
	})

	s.server = service.NewHTTPServer(c.Config.JsonrpcApiAddress, handler, service.DefaultJSONRPCOptions(), s.Logger)
	service.StartupBindWarning(s.Logger, "jsonrpc", c.Config.JsonrpcApiAddress)

	if s.listen == nil {
		s.listen = net.Listen
	}

	s.LogConfig(c.Config)

	return s, nil
}

func (s *Service) OnStop(_ bool) []error {
	var errs []error
	s.Logger.Info("Shutting down JSON-RPC HTTP server", "addr", s.server.Addr)
	ctx, cancel := context.WithTimeout(context.Background(), jsonrpcShutdownTimeout)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	return errs
}

func (s *Service) OnServe(ctx context.Context) error {
	listener, err := s.listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}

	s.Logger.Info("Listening", "addr", listener.Addr().String())

	serverDone := make(chan error, 1)
	go func() {
		// Run the HTTP accept loop in parallel with the framework's lifecycle
		// loop below. The lifecycle loop is the component that consumes
		// SIGINT/SIGTERM and calls Stop() for graceful shutdown.
		err := s.server.Serve(listener)
		s.Logger.Info("Stopped listening", "addr", listener.Addr().String(), "err", err)
		if errors.Is(err, http.ErrServerClosed) {
			serverDone <- nil
			return
		}
		serverDone <- err
	}()

	select {
	case err := <-serverDone:
		// The HTTP loop exited first. This is unexpected unless the listener
		// failed or the server was already closed, so cancel the framework
		// loop and wait for it to observe the cancellation before returning.
		s.Stop(true)
		<-ctx.Done()
		if err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		// The framework loop exited first because it handled a shutdown signal
		// or context cancellation and called Stop(), which should trigger
		// s.server.Shutdown(). Wait for the HTTP loop to finish so Serve()
		// returns only after the listener is fully closed.
		serverErr := <-serverDone
		if serverErr != nil {
			return serverErr
		}
		return nil
	}
}
