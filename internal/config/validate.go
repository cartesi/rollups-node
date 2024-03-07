// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package config

import "fmt"

func (nodeConfig *NodeConfig) Validate() {
	if nodeConfig.cartesiExperimentalSunodoValidatorEnabled != nil &&
		*nodeConfig.cartesiExperimentalSunodoValidatorEnabled {
		if nodeConfig.cartesiExperimentalSunodoValidatorRedisEndpoint == nil {
			fail("Missing required %s env var",
				"CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_REDIS_ENDPOINT")
		}
	}

	if nodeConfig.cartesiFeatureDisableClaimer != nil && !*nodeConfig.cartesiFeatureDisableClaimer {
		if nodeConfig.cartesiAuthError != nil {
			panic(fmt.Errorf("Auth config error: %w ", nodeConfig.cartesiAuthError))
		}
	}

	if nodeConfig.cartesiBlockchainHttpEndpoint == nil {
		fail("Missing required %s env var",
			"CARTESI_BLOCKCHAIN_HTTP_ENDPOINT")
	}

	if nodeConfig.cartesiBlockchainId == nil {
		fail("Missing required %s env var",
			"CARTESI_BLOCKCHAIN_ID config")
	}

	if nodeConfig.cartesiBlockchainWsEndpoint == nil {
		fail("Missing required %s env var",
			"CARTESI_BLOCKCHAIN_WS_ENDPOINT config")
	}

	if nodeConfig.cartesiContractsInputBoxDeploymentBlockNumber == nil {
		fail("Missing required %s env var",
			"CARTESI_CONTRACTS_INPUT_BOX_DEPLOYMENT_BLOCK_NUMBER config")
	}

	if nodeConfig.cartesiContractsApplicationAddress == nil {
		fail("Missing required %s env var",
			"CARTESI_CONTRACTS_APPLICATION_ADDRESS config")
	}

	if nodeConfig.cartesiContractsApplicationDeploymentBlockNumber == nil {
		fail("Missing required %s env var",
			"CARTESI_CONTRACTS_APPLICATION_DEPLOYMENT_BLOCK_NUMBER config")
	}

	if nodeConfig.cartesiContractsAuthorityAddress == nil {
		fail("Missing required %s env var",
			"CARTESI_CONTRACTS_AUTHORITY_ADDRESS config")
	}

	if nodeConfig.cartesiContractsHistoryAddress == nil {
		fail("Missing required %s env var",
			"CARTESI_CONTRACTS_HISTORY_ADDRESS config")
	}

	if nodeConfig.cartesiContractsInputBoxAddress == nil {
		fail("Missing required %s env var",
			"CARTESI_CONTRACTS_INPUT_BOX_ADDRESS config")
	}

	if nodeConfig.cartesiSnapshotDir == nil {
		fail("Missing required %s env var",
			"CARTESI_SNAPSHOT_DIR config")
	}

}
