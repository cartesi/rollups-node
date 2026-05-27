// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"context"
	"fmt"

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

// serviceResult carries either a successfully created service or an error
// back from the goroutines in createServices.
type serviceResult struct {
	service service.IService
	err     error
}

type CreateInfo struct {
	service.CreateInfo

	Config config.NodeConfig

	PrtClient     *ethclient.Client
	ClaimerClient *ethclient.Client
	ReaderClient  *ethclient.Client
	Repository    repository.Repository
}

type Service struct {
	service.Service

	Children   []service.IService
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

	err = createServices(ctx, c, s)
	if err != nil {
		s.Logger.Error(fmt.Sprint(err))
		return nil, err
	}
	return s, nil
}

type serviceCreator func(context.Context, *CreateInfo, *Service) (service.IService, error)

func createServices(ctx context.Context, c *CreateInfo, s *Service) error {
	creators := []serviceCreator{
		newEVMReader,
		newAdvancer,
		newValidator,
		newClaimer,
		newPrt,
	}
	if c.Config.FeatureJsonrpcApiEnabled {
		creators = append(creators, newJsonrpc)
	}

	ch := make(chan serviceResult, len(creators))
	for _, create := range creators {
		go func() {
			svc, err := create(ctx, c, s)
			ch <- serviceResult{service: svc, err: err}
		}()
	}

	for range len(creators) {
		select {
		case result := <-ch:
			if result.err != nil {
				stopAndDrain(s.Children, ch, len(creators)-len(s.Children)-1)
				return fmt.Errorf("failed to create service: %w", result.err)
			}
			s.Children = append(s.Children, result.service)
		case <-ctx.Done():
			stopAndDrain(s.Children, ch, len(creators)-len(s.Children))
			return fmt.Errorf("failed to create services: %w", ctx.Err())
		}
	}
	return nil
}

// stopAndDrain stops already-created children and drains remaining results
// from the channel, stopping any successful services to prevent resource leaks.
func stopAndDrain(children []service.IService, ch <-chan serviceResult, remaining int) {
	for _, child := range children {
		child.Stop(true)
	}
	go func() {
		for range remaining {
			if r := <-ch; r.err == nil && r.service != nil {
				r.service.Stop(true)
			}
		}
	}()
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
	me.SetStopping()
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

func newEVMReader(ctx context.Context, c *CreateInfo, s *Service) (service.IService, error) {
	readerArgs := evmreader.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceEvmReader,
			Context:              s.Context,
			Cancel:               s.Cancel,
			LogLevel:             config.ResolveServiceLogLevel(config.ServiceEvmReader, c.Config.LogLevel),
			LogColor:             c.Config.LogColor,
			EnableSignalHandling: false,
			TelemetryCreate:      false,
			PollInterval:         c.Config.EvmReaderPollingInterval,
			ServeMux:             s.ServeMux,
		},
		EthClient:  c.ReaderClient,
		Repository: c.Repository,
		Config:     *c.Config.ToEvmreaderConfig(),
	}

	readerService, err := evmreader.Create(ctx, &readerArgs)
	if err != nil {
		return nil, fmt.Errorf("create evm-reader: %w", err)
	}
	return readerService, nil
}

func newAdvancer(ctx context.Context, c *CreateInfo, s *Service) (service.IService, error) {
	advancerArgs := advancer.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceAdvancer,
			Context:              s.Context,
			Cancel:               s.Cancel,
			LogLevel:             config.ResolveServiceLogLevel(config.ServiceAdvancer, c.Config.LogLevel),
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
		return nil, fmt.Errorf("create advancer: %w", err)
	}
	return advancerService, nil
}

func newValidator(ctx context.Context, c *CreateInfo, s *Service) (service.IService, error) {
	validatorArgs := validator.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceValidator,
			Context:              s.Context,
			Cancel:               s.Cancel,
			LogLevel:             config.ResolveServiceLogLevel(config.ServiceValidator, c.Config.LogLevel),
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
		return nil, fmt.Errorf("create validator: %w", err)
	}
	return validatorService, nil
}

func newClaimer(ctx context.Context, c *CreateInfo, s *Service) (service.IService, error) {
	claimerArgs := claimer.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceClaimer,
			Context:              s.Context,
			Cancel:               s.Cancel,
			LogLevel:             config.ResolveServiceLogLevel(config.ServiceClaimer, c.Config.LogLevel),
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
		return nil, fmt.Errorf("create claimer: %w", err)
	}
	return claimerService, nil
}

func newJsonrpc(ctx context.Context, c *CreateInfo, s *Service) (service.IService, error) {
	jsonrpcArgs := jsonrpc.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceJsonrpc,
			Context:              s.Context,
			Cancel:               s.Cancel,
			LogLevel:             config.ResolveServiceLogLevel(config.ServiceJsonrpc, c.Config.LogLevel),
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
		return nil, fmt.Errorf("create jsonrpc: %w", err)
	}
	return jsonrpcService, nil
}

func newPrt(ctx context.Context, c *CreateInfo, s *Service) (service.IService, error) {
	prtArgs := prt.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServicePrt,
			Context:              s.Context,
			Cancel:               s.Cancel,
			LogLevel:             config.ResolveServiceLogLevel(config.ServicePrt, c.Config.LogLevel),
			LogColor:             c.Config.LogColor,
			EnableSignalHandling: false,
			TelemetryCreate:      false,
			PollInterval:         c.Config.PrtPollingInterval,
			ServeMux:             s.ServeMux,
		},
		EthClient:  c.PrtClient,
		Repository: c.Repository,
		Config:     *c.Config.ToPrtConfig(),
	}

	prtService, err := prt.Create(ctx, &prtArgs)
	if err != nil {
		return nil, fmt.Errorf("create prt: %w", err)
	}
	return prtService, nil
}
