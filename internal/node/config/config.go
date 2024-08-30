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
	ContractsApplicationAddress               string
	ContractsInputBoxAddress                  string
	ContractsInputBoxDeploymentBlockNumber    int64
	SnapshotDir                               string
	PostgresEndpoint                          Redacted[string]
	PostgresSslMode                           bool
	HttpAddress                               string
	HttpPort                                  int
	FeatureClaimerEnabled                     bool
	FeatureMachineHashCheckEnabled            bool
	ExperimentalServerManagerLogBypassEnabled bool
	ExperimentalSunodoValidatorEnabled        bool
	ExperimentalSunodoValidatorRedisEndpoint  string
	Auth                                      Auth
	MaxConcurrentInspects                     uint8
	AdvancerPollingInterval                   Duration
	ValidatorPollingInterval                  Duration
	// Temporary
	MachineServerVerbosity cartesimachine.ServerVerbosity
	MachineIncCycles       uint64
	MachineMaxCycles       uint64
	MachineAdvanceTimeout  Duration
	MachineInspectTimeout  Duration
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
	config.LogLevel = getLogLevel()
	config.LogPrettyEnabled = getLogPrettyEnabled()
	config.RollupsEpochLength = getEpochLength()
	config.BlockchainID = getBlockchainId()
	config.BlockchainHttpEndpoint = Redacted[string]{getBlockchainHttpEndpoint()}
	config.BlockchainWsEndpoint = Redacted[string]{getBlockchainWsEndpoint()}
	config.LegacyBlockchainEnabled = getLegacyBlockchainEnabled()
	config.BlockchainFinalityOffset = getBlockchainFinalityOffset()
	config.EvmReaderDefaultBlock = getEvmReaderDefaultBlock()
	config.EvmReaderRetryPolicyMaxRetries = getEvmReaderRetryPolicyMaxRetries()
	config.EvmReaderRetryPolicyMaxDelay = getEvmReaderRetryPolicyMaxDelay()
	config.BlockchainBlockTimeout = getBlockchainBlockTimeout()
	config.ContractsApplicationAddress = getContractsApplicationAddress()
	config.ContractsInputBoxAddress = getContractsInputBoxAddress()
	config.ContractsInputBoxDeploymentBlockNumber = getContractsInputBoxDeploymentBlockNumber()
	config.SnapshotDir = getSnapshotDir()
	config.PostgresEndpoint = Redacted[string]{getPostgresEndpoint()}
	config.PostgresSslMode = getPostgresSslmodeEnabled()
	config.HttpAddress = getHttpAddress()
	config.HttpPort = getHttpPort()
	config.FeatureClaimerEnabled = getFeatureClaimerEnabled()
	config.FeatureMachineHashCheckEnabled = getFeatureMachineHashCheckEnabled()
	config.ExperimentalServerManagerLogBypassEnabled =
		getExperimentalServerManagerLogBypassEnabled()
	config.ExperimentalSunodoValidatorEnabled = getExperimentalSunodoValidatorEnabled()
	if getExperimentalSunodoValidatorEnabled() {
		config.ExperimentalSunodoValidatorRedisEndpoint =
			getExperimentalSunodoValidatorRedisEndpoint()
	}
	if getFeatureClaimerEnabled() && !getExperimentalSunodoValidatorEnabled() {
		config.Auth = authFromEnv()
	}
	config.MaxConcurrentInspects = getMaxConcurrentInspects()
	config.AdvancerPollingInterval = getAdvancerPollingInterval()
	config.ValidatorPollingInterval = getValidatorPollingInterval()
	// Temporary.
	config.MachineServerVerbosity = cartesimachine.ServerVerbosity(getMachineServerVerbosity())
	config.MachineIncCycles = getMachineIncCycles()
	config.MachineMaxCycles = getMachineMaxCycles()
	config.MachineAdvanceTimeout = getMachineAdvanceTimeout()
	config.MachineInspectTimeout = getMachineInspectTimeout()
	return config
}

func authFromEnv() Auth {
	switch getAuthKind() {
	case AuthKindPrivateKeyVar:
		return AuthPrivateKey{
			PrivateKey: Redacted[string]{getAuthPrivateKey()},
		}
	case AuthKindPrivateKeyFile:
		path := getAuthPrivateKeyFile()
		privateKey, err := os.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("failed to read private-key file: %v", err))
		}
		return AuthPrivateKey{
			PrivateKey: Redacted[string]{string(privateKey)},
		}
	case AuthKindMnemonicVar:
		return AuthMnemonic{
			Mnemonic:     Redacted[string]{getAuthMnemonic()},
			AccountIndex: Redacted[int]{getAuthMnemonicAccountIndex()},
		}
	case AuthKindMnemonicFile:
		path := getAuthMnemonicFile()
		mnemonic, err := os.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("failed to read mnemonic file: %v", err))
		}
		return AuthMnemonic{
			Mnemonic:     Redacted[string]{string(mnemonic)},
			AccountIndex: Redacted[int]{getAuthMnemonicAccountIndex()},
		}
	case AuthKindAWS:
		return AuthAWS{
			KeyID:  Redacted[string]{getAuthAwsKmsKeyId()},
			Region: Redacted[string]{getAuthAwsKmsRegion()},
		}
	default:
		panic("invalid auth kind")
	}
}
