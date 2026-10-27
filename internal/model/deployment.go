// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

// DeploymentTier names a confirmed deployment restriction, independently of application health.
type DeploymentTier string

const (
	DeploymentTierGeneration DeploymentTier = "generation"
	DeploymentTierTable      DeploymentTier = "table"
)

// DeploymentAdmissionState retains a restriction independently of application health.
// ResetGeneration fences explicit enable resets and process startup episodes.
// A nil restriction is neutral storage, never proof of compatibility.
type DeploymentAdmissionState struct {
	ApplicationID    int64
	ResetGeneration  int64
	ConfirmedTier    *DeploymentTier
	DeploymentReason *string
}
