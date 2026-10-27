// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

// DeploymentIdentity binds an assessment to the application configuration it read.
type DeploymentIdentity struct {
	ApplicationID      int64
	ApplicationAddress common.Address
	ConsensusAddress   common.Address
}

type DeploymentAssessment struct {
	Identity        DeploymentIdentity
	ResetGeneration int64
	ConfirmedTier   *model.DeploymentTier // Nil clears a saved restriction after fresh compatible recovery.
	Reason          string
}

// DeploymentCompletionState contains one fresh, closed, read-only RR snapshot.
// Missing control state never establishes compatible deployment admission.
type DeploymentCompletionState struct {
	Completion *EpochCompletionState
	Deployment *model.DeploymentAdmissionState
}

type DeploymentRepository interface {
	GetDeploymentAdmissionState(context.Context, DeploymentIdentity) (*model.DeploymentAdmissionState, error)
	// BeginDeploymentStartup requires an existing control row and returns only its committed increment.
	BeginDeploymentStartup(context.Context, DeploymentIdentity, int64) (*model.DeploymentAdmissionState, error)
	// AcknowledgeDeploymentAssessment stores the nullable tier and returns the previous row after commit.
	AcknowledgeDeploymentAssessment(context.Context, DeploymentAssessment) (*model.DeploymentAdmissionState, error)
	GetDeploymentCompletionState(context.Context, DeploymentIdentity, uint64) (*DeploymentCompletionState, error)
}
