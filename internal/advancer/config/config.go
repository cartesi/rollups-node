// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// The config package manages the node configuration, which comes from environment variables.
// The sub-package generate specifies these environment variables.
package config

import (
	. "github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
)

type AdvancerConfig struct {
	LogLevel                LogLevel
	LogPrettyEnabled        bool
	PostgresEndpoint        Redacted[string]
	PostgresSslMode         bool
	AdvancerPollingInterval Duration
	HttpAddress             string
	HttpPort                int
	MachineServerVerbosity  cartesimachine.ServerVerbosity
}

func GetAdvancerConfig() AdvancerConfig {
	return AdvancerConfig{
		LogLevel:                GetLogLevel(),
		LogPrettyEnabled:        GetLogPrettyEnabled(),
		HttpAddress:             GetHttpAddress(),
		HttpPort:                GetHttpPort(),
		PostgresEndpoint:        Redacted[string]{Value: GetPostgresEndpoint()},
		AdvancerPollingInterval: GetAdvancerPollingInterval(),
		MachineServerVerbosity:  cartesimachine.ServerVerbosity(GetMachineServerVerbosity()),
	}
}
