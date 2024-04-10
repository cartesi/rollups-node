// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

type DeploymentInfo struct {
	AuthorityAddress   string `json:"CARTESI_CONTRACTS_AUTHORITY_ADDRESS"`
	ApplicationAddress string `json:"CARTESI_CONTRACTS_APPLICATION_ADDRESS"`
	BlockNumber        string `json:"CARTESI_CONTRACTS_APPLICATION_DEPLOYMENT_BLOCK_NUMBER"`
}
