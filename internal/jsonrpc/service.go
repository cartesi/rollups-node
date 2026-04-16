// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

const (
	jsonrpcShutdownTimeout = 5 * time.Second
	jsonrpcWriteHeadroom   = 5 * time.Second
)

// Service implements the IService interface.
type Service struct {
	service.HTTPServiceTemplate
	repository repository.Repository
	inputABI   *abi.ABI
	outputABI  *abi.ABI
	// OpenAPI description for JSON-RPC API loaded from 'jsonrpc-discover.json' file
	discoverSpec json.RawMessage
	handlers     dispatchTable
	// dispatchTimeout expires requests early enough to serialize a complete
	// timeout response before the HTTP server's write deadline.
	dispatchTimeout time.Duration
}

type CreateInfo struct {
	Config     config.JsonrpcConfig
	Logger     *slog.Logger
	Repository repository.Repository
}

func Create(ctx context.Context, c *CreateInfo) (service.SupervisedService, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}
	cfg := service.HTTPServiceConfigs{
		BaseConfigs: service.BaseConfigs{
			Name:     config.ServiceJsonrpc,
			Logger:   c.Logger,
			LogLevel: c.Config.LogLevel,
			LogColor: c.Config.LogColor,
		},
		HTTPServerOptions:  service.DefaultJSONRPCOptions(),
		Address:            c.Config.JsonrpcApiAddress,
		SafeRequestID:      true,
		CorsAllowedOrigins: c.Config.JsonrpcCorsAllowedOrigins,
		MaxInflight:        c.Config.JsonrpcMaxInflight,
		ShutdownTimeout:    jsonrpcShutdownTimeout,
	}

	s.repository = c.Repository
	s.handlers = cloneDispatchTable(jsonrpcHandlers)
	if s.repository == nil {
		return nil, fmt.Errorf("repository on validator service Create is nil")
	}

	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	if err != nil {
		return nil, fmt.Errorf("unable to read jsonrpc-discover content: %w", err)
	}
	if err := json.Unmarshal(data, &s.discoverSpec); err != nil {
		return nil, fmt.Errorf("unable to unmarshal discovery spec JSON: %w", err)
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

	service.InitHTTPServiceTemplate(&s.HTTPServiceTemplate, &cfg, mux)

	s.dispatchTimeout = cfg.HTTPServerOptions.WriteTimeout - jsonrpcWriteHeadroom

	s.Logger.Info("Created", "config", c.Config)

	return s, nil
}
