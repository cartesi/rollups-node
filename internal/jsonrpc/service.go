// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/services"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// -----------------------------------------------------------------------------
// Service Implementation
// -----------------------------------------------------------------------------

// Service implements the IService interface.
type Service struct {
	service.Service
	repository repository.Repository
	server     *http.Server
	inputABI   *abi.ABI
	outputABI  *abi.ABI
}

type CreateInfo struct {
	service.CreateInfo

	Config config.JsonrpcConfig

	Repository repository.Repository
}

func Create(ctx context.Context, c *CreateInfo) (*Service, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}
	c.Impl = s

	err = service.Create(ctx, &c.CreateInfo, &s.Service)
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

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", s.handleRPC)
	s.server = &http.Server{
		Addr:              c.Config.JsonrpcApiAddress,
		Handler:           services.CorsMiddleware(mux), // FIXME: add proper cors config
		WriteTimeout:      30 * time.Second,             //nolint: mnd
		ReadTimeout:       30 * time.Second,             //nolint: mnd
		ReadHeaderTimeout: 10 * time.Second,             //nolint: mnd
	}

	return s, nil
}

func (s *Service) Alive() bool {
	return true
}

func (s *Service) Ready() bool {
	return true
}

func (s *Service) Reload() []error {
	return nil
}

func (s *Service) Tick() []error {
	// No periodic tasks.
	return nil
}

func (s *Service) Stop(_ bool) []error {
	s.SetStopping()
	var errs []error
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint: mnd
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	return errs
}

func (s *Service) String() string {
	return s.Name
}

func (s *Service) Serve() error {
	s.Logger.Info("Listening", "addr", s.server.Addr)
	return s.server.ListenAndServe()
}
