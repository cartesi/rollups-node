// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

// DeploymentTier names a confirmed deployment restriction, independently of application health.
type DeploymentTier string

const (
	DeploymentTierGeneration DeploymentTier = "generation"
	DeploymentTierTable      DeploymentTier = "table"
)
