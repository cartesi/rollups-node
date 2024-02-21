// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package config

import "fmt"

func (nodeConfig *NodeConfig) Validate() {
	if nodeConfig.cartesiExperimentalSunodoValidatorEnabled != nil &&
		*nodeConfig.cartesiExperimentalSunodoValidatorEnabled {
		if nodeConfig.cartesiExperimentalSunodoValidatorRedisEndpoint == nil {
			panic("missing CartesiExperimentalSunodoValidatorRedisEndpoint config")
		}
	}

	if nodeConfig.cartesiFeatureDisableClaimer != nil && !*nodeConfig.cartesiFeatureDisableClaimer {
		if nodeConfig.cartesiAuthError != nil {
			panic(fmt.Errorf("Auth config error: %w ", nodeConfig.cartesiAuthError))
		}
	}

	if nodeConfig.cartesiBlockchainHttpEndpoint == nil {
		panic("Missing CartesiBlockchainHttpEndpoint")
	}

	if nodeConfig.cartesiBlockchainId == nil {
		panic("Missing CartesiBlockChainId config")
	}

	if nodeConfig.cartesiBlockchainWsEndpoint == nil {
		panic("Missing CartesiBlockchainWsEndpoint config")
	}

	if nodeConfig.cartesiContractsInputBoxDeploymentBlockNumber == nil {
		panic("Missing CartesiContractsInputBoxDeploymentBlockNumber config")
	}

	if nodeConfig.cartesiContractsApplicationAddress == nil {
		panic("Missing CartesiContractsApplicationAddress config")
	}

	if nodeConfig.cartesiContractsApplicationDeploymentBlockNumber == nil {
		panic("Missing CartesiContractsApplicationDeploymentBlockNumber config")
	}

	if nodeConfig.cartesiContractsAuthorityAddress == nil {
		panic("Missing CartesiContractsAuthorityAddress config")
	}

	if nodeConfig.cartesiContractsHistoryAddress == nil {
		panic("Missing CartesiContractsHistoryAddress config")
	}

	if nodeConfig.cartesiContractsInputBoxAddress == nil {
		panic("Missing CartesiContractsInputBoxAddress config")
	}

	if nodeConfig.cartesiSnapshotDir == nil {
		panic("Missing CartesiSnapshotDir config")
	}

	if nodeConfig.cartesiAuthError != nil {
		panic(nodeConfig.cartesiAuthError)
	}

}
