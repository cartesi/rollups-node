// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// The config package manages the node configuration, which comes from environment variables.
// The sub-package generate specifies these environment variables.
package config

import (
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
)

// NodeConfig contains all the Node variables.
// See the corresponding environment variable for the variable documentation.
type NodeConfig struct {
	LogLevel                                  LogLevel
	LogPrettyEnabled                          bool
	RollupsEpochLength                        uint64
	BlockchainID                              uint64
	BlockchainHttpEndpoint                    Redacted[string]
	BlockchainWsEndpoint                      Redacted[string]
	LegacyBlockchainEnabled                   bool
	BlockchainFinalityOffset                  int
	EvmReaderDefaultBlock                     DefaultBlock
	EvmReaderRetryPolicyMaxRetries            uint64
	EvmReaderRetryPolicyMaxDelay              Duration
	BlockchainBlockTimeout                    int
	ContractsInputBoxAddress                  string
	ContractsInputBoxDeploymentBlockNumber    int64
	SnapshotDir                               string
	PostgresEndpoint                          Redacted[string]
	HttpAddress                               string
	HttpPort                                  int
	FeatureClaimerEnabled                     bool
	FeatureMachineHashCheckEnabled            bool
	ExperimentalServerManagerLogBypassEnabled bool
	ExperimentalSunodoValidatorEnabled        bool
	ExperimentalSunodoValidatorRedisEndpoint  string
	Auth                                      Auth
	AdvancerPollingInterval                   Duration
	ValidatorPollingInterval                  Duration
	// Temporary
	MachineServerVerbosity cartesimachine.ServerVerbosity
}

// Auth is used to sign transactions.
type Auth any

// AuthPrivateKey allows signing through private keys.
type AuthPrivateKey struct {
	PrivateKey Redacted[string]
}

// AuthMnemonic allows signing through mnemonics.
type AuthMnemonic struct {
	Mnemonic     Redacted[string]
	AccountIndex Redacted[int]
}

// AuthAWS allows signing through AWS services.
type AuthAWS struct {
	KeyID  Redacted[string]
	Region Redacted[string]
}

// Redacted is a wrapper that redacts a given field from the logs.
type Redacted[T any] struct {
	Value T
}

func (r Redacted[T]) String() string {
	return "[REDACTED]"
}

// FromEnv loads the config from environment variables.
func FromEnv() NodeConfig {
	var config NodeConfig
	config.LogLevel = GetLogLevel()
	config.LogPrettyEnabled = GetLogPrettyEnabled()
	config.RollupsEpochLength = GetEpochLength()
	config.BlockchainID = GetBlockchainId()
	config.BlockchainHttpEndpoint = Redacted[string]{GetBlockchainHttpEndpoint()}
	config.BlockchainWsEndpoint = Redacted[string]{GetBlockchainWsEndpoint()}
	config.LegacyBlockchainEnabled = GetLegacyBlockchainEnabled()
	config.BlockchainFinalityOffset = GetBlockchainFinalityOffset()
	config.EvmReaderDefaultBlock = GetEvmReaderDefaultBlock()
	config.EvmReaderRetryPolicyMaxRetries = GetEvmReaderRetryPolicyMaxRetries()
	config.EvmReaderRetryPolicyMaxDelay = GetEvmReaderRetryPolicyMaxDelay()
	config.BlockchainBlockTimeout = GetBlockchainBlockTimeout()
	config.ContractsInputBoxAddress = GetContractsInputBoxAddress()
	config.ContractsInputBoxDeploymentBlockNumber = GetContractsInputBoxDeploymentBlockNumber()
	config.SnapshotDir = GetSnapshotDir()
	config.PostgresEndpoint = Redacted[string]{GetPostgresEndpoint()}
	config.HttpAddress = GetHttpAddress()
	config.HttpPort = GetHttpPort()
	config.FeatureClaimerEnabled = GetFeatureClaimerEnabled()
	config.FeatureMachineHashCheckEnabled = GetFeatureMachineHashCheckEnabled()
	config.ExperimentalServerManagerLogBypassEnabled =
		GetExperimentalServerManagerLogBypassEnabled()
	config.ExperimentalSunodoValidatorEnabled = GetExperimentalSunodoValidatorEnabled()
	if GetExperimentalSunodoValidatorEnabled() {
		config.ExperimentalSunodoValidatorRedisEndpoint =
			GetExperimentalSunodoValidatorRedisEndpoint()
	}
	if GetFeatureClaimerEnabled() && !GetExperimentalSunodoValidatorEnabled() {
		config.Auth = authFromEnv()
	}
	config.AdvancerPollingInterval = GetAdvancerPollingInterval()
	config.ValidatorPollingInterval = GetValidatorPollingInterval()
	// Temporary.
	config.MachineServerVerbosity = cartesimachine.ServerVerbosity(GetMachineServerVerbosity())
	return config
}

func authFromEnv() Auth {
	switch GetAuthKind() {
	case AuthKindPrivateKeyVar:
		return AuthPrivateKey{
			PrivateKey: Redacted[string]{GetAuthPrivateKey()},
		}
	case AuthKindPrivateKeyFile:
		path := GetAuthPrivateKeyFile()
		privateKey, err := os.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("failed to read private-key file: %v", err))
		}
		return AuthPrivateKey{
			PrivateKey: Redacted[string]{string(privateKey)},
		}
	case AuthKindMnemonicVar:
		return AuthMnemonic{
			Mnemonic:     Redacted[string]{GetAuthMnemonic()},
			AccountIndex: Redacted[int]{GetAuthMnemonicAccountIndex()},
		}
	case AuthKindMnemonicFile:
		path := GetAuthMnemonicFile()
		mnemonic, err := os.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("failed to read mnemonic file: %v", err))
		}
		return AuthMnemonic{
			Mnemonic:     Redacted[string]{string(mnemonic)},
			AccountIndex: Redacted[int]{GetAuthMnemonicAccountIndex()},
		}
	case AuthKindAWS:
		return AuthAWS{
			KeyID:  Redacted[string]{GetAuthAwsKmsKeyId()},
			Region: Redacted[string]{GetAuthAwsKmsRegion()},
		}
	default:
		panic("invalid auth kind")
	}
}

// ------------------------------------------------------------------------------------------------

type AdvancerConfig struct {
	LogLevel                LogLevel
	LogPrettyEnabled        bool
	PostgresEndpoint        Redacted[string]
	PostgresSslMode         bool
	AdvancerPollingInterval Duration
	MachineServerVerbosity  cartesimachine.ServerVerbosity
}

func GetAdvancerConfig() AdvancerConfig {
	var config AdvancerConfig
	config.LogLevel = GetLogLevel()
	config.LogPrettyEnabled = GetLogPrettyEnabled()
	config.PostgresEndpoint = Redacted[string]{GetPostgresEndpoint()}
	config.AdvancerPollingInterval = GetAdvancerPollingInterval()
	// Temporary.
	config.MachineServerVerbosity = cartesimachine.ServerVerbosity(GetMachineServerVerbosity())
	return config
}
