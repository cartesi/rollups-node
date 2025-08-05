// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"context"
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/cartesi/rollups-node/internal/advancer"
	"github.com/cartesi/rollups-node/internal/claimer"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/prt"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/validator"

	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateInfo struct {
	service.CreateInfo

	Config config.NodeConfig

	ClaimerClient  *ethclient.Client
	ReaderClient   *ethclient.Client
	ReaderWSClient *ethclient.Client
	Repository     repository.Repository
}

type Service struct {
	service.Service

	Children   []service.IService
	Client     *ethclient.Client
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

	s.Logger.Debug("Creating services", "config", c.Config)

	err = createServices(ctx, c, s)
	if err != nil {
		s.Logger.Error(fmt.Sprint(err))
		return nil, err
	}
	return s, nil
}

func createServices(ctx context.Context, c *CreateInfo, s *Service) error {
	ch := make(chan service.IService)
	numChildren := 0

	numChildren++
	go func() {
		ch <- newEVMReader(ctx, c, s)
	}()

	numChildren++
	go func() {
		ch <- newAdvancer(ctx, c, s)
	}()

	numChildren++
	go func() {
		ch <- newValidator(ctx, c, s)
	}()

	numChildren++
	go func() {
		ch <- newClaimer(ctx, c, s)
	}()

	numChildren++
	go func() {
		ch <- newPRT(ctx, c, s)
	}()

	if c.Config.FeatureJsonrpcApiEnabled {
		numChildren++
		go func() {
			ch <- newJsonrpc(ctx, c, s)
		}()
	}

	for range numChildren {
		select {
		case child := <-ch:
			s.Children = append(s.Children, child)
		case <-ctx.Done():
			err := ctx.Err()
			s.Logger.Error("Failed to create services. Time limit exceeded",
				"err", err)
			return fmt.Errorf("failed to create services. Time limit exceeded")
		}
	}
	return nil
}

func (me *Service) Alive() bool {
	allAlive := true
	for _, s := range me.Children {
		allAlive = allAlive && s.Alive()
	}
	return allAlive
}

func (me *Service) Ready() bool {
	allReady := true
	for _, s := range me.Children {
		allReady = allReady && s.Ready()
	}
	return allReady
}

func (s *Service) Reload() []error { return nil }
func (s *Service) Tick() []error   { return nil }
func (me *Service) Stop(force bool) []error {
	errs := []error{}
	for _, s := range me.Children {
		errs = append(errs, s.Stop(force)...)
	}
	return errs
}

func (me *Service) Serve() error {
	for _, s := range me.Children {
		go s.Serve()
	}
	return me.Service.Serve()
}

// services creation

func newEVMReader(ctx context.Context, c *CreateInfo, s *Service) service.IService {
	readerArgs := evmreader.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 "evm-reader",
			LogLevel:             c.Config.LogLevel,
			LogColor:             c.Config.LogColor,
			EnableSignalHandling: false,
			TelemetryCreate:      false,
			ServeMux:             s.ServeMux,
		},
		EthClient:   c.ReaderClient,
		EthWsClient: c.ReaderWSClient,
		Repository:  c.Repository,
		Config:      *c.Config.ToEvmreaderConfig(),
	}

	readerService, err := evmreader.Create(ctx, &readerArgs)
	if err != nil {
		s.Logger.Error("Fatal", "error", err)
		os.Exit(1)
	}
	return readerService
}

func newAdvancer(ctx context.Context, c *CreateInfo, s *Service) service.IService {
	advancerArgs := advancer.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 "advancer",
			LogLevel:             c.Config.LogLevel,
			LogColor:             c.Config.LogColor,
			EnableSignalHandling: false,
			TelemetryCreate:      false,
			PollInterval:         c.Config.AdvancerPollingInterval,
			ServeMux:             s.ServeMux,
		},
		Repository: c.Repository,
		Config:     *c.Config.ToAdvancerConfig(),
	}

	advancerService, err := advancer.Create(ctx, &advancerArgs)
	if err != nil {
		s.Logger.Error("Fatal", "error", err)
		os.Exit(1)
	}
	return advancerService
}

func newPRT(ctx context.Context, c *CreateInfo, s *Service) service.IService {
	prtArgs := prt.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 "PRT",
			LogLevel:             c.Config.LogLevel,
			LogColor:             c.Config.LogColor,
			EnableSignalHandling: false,
			TelemetryCreate:      false,
			PollInterval:         c.Config.PrtPollingInterval,
			ServeMux:             s.ServeMux,
		},
		Repository: c.Repository,
		Config:     *c.Config.ToPrtConfig(),
	}

	prtService, err := prt.Create(ctx, &prtArgs)
	if err != nil {
		s.Logger.Error("Fatal", "error", err)
		os.Exit(1)
	}
	return prtService
}

func newValidator(ctx context.Context, c *CreateInfo, s *Service) service.IService {
	validatorArgs := validator.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 "validator",
			LogLevel:             c.Config.LogLevel,
			LogColor:             c.Config.LogColor,
			EnableSignalHandling: false,
			TelemetryCreate:      false,
			PollInterval:         c.Config.ValidatorPollingInterval,
			ServeMux:             s.ServeMux,
		},
		Repository: c.Repository,
		Config:     *c.Config.ToValidatorConfig(),
	}

	validatorService, err := validator.Create(ctx, &validatorArgs)
	if err != nil {
		s.Logger.Error("Fatal", "error", err)
		os.Exit(1)
	}
	return validatorService
}

func newClaimer(ctx context.Context, c *CreateInfo, s *Service) service.IService {
	claimerArgs := claimer.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 "claimer",
			LogLevel:             c.Config.LogLevel,
			LogColor:             c.Config.LogColor,
			EnableSignalHandling: false,
			TelemetryCreate:      false,
			PollInterval:         c.Config.ClaimerPollingInterval,
			ServeMux:             s.ServeMux,
		},
		EthConn:    c.ClaimerClient,
		Repository: c.Repository,
		Config:     *c.Config.ToClaimerConfig(),
	}

	claimerService, err := claimer.Create(ctx, &claimerArgs)
	if err != nil {
		s.Logger.Error("Fatal", "error", err)
		os.Exit(1)
	}
	return claimerService
}

func newJsonrpc(ctx context.Context, c *CreateInfo, s *Service) service.IService {
	jsonrpcArgs := jsonrpc.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 "jsonrpc",
			LogLevel:             c.Config.LogLevel,
			LogColor:             c.Config.LogColor,
			EnableSignalHandling: false,
			TelemetryCreate:      false,
			ServeMux:             s.ServeMux,
		},
		Repository: c.Repository,
		Config:     *c.Config.ToJsonrpcConfig(),
	}

	jsonrpcService, err := jsonrpc.Create(ctx, &jsonrpcArgs)
	if err != nil {
		s.Logger.Error("Fatal", "error", err)
		os.Exit(1)
	}
	return jsonrpcService
}
