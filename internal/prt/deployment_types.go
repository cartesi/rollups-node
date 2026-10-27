// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
)

type deploymentState string

const (
	deploymentNone       deploymentState = "none"
	deploymentPending    deploymentState = "pending"
	deploymentCompatible deploymentState = "compatible"
	deploymentGeneration deploymentState = deploymentState(model.DeploymentTierGeneration)
	deploymentTable      deploymentState = deploymentState(model.DeploymentTierTable)
)

type deploymentCause string

const (
	deploymentCauseNone        deploymentCause = "none"
	deploymentCauseStaticShape deploymentCause = "static_shape"
	deploymentCauseMarchID     deploymentCause = "marchid"
	deploymentCausePristine    deploymentCause = "pristine_hash"
	deploymentCauseCount       deploymentCause = "level_count"
	deploymentCauseGeometry    deploymentCause = "geometry"
	deploymentCauseComplete    deploymentCause = "complete"
	deploymentCauseEmpty       deploymentCause = "empty_return"
)

// DeploymentParameters retains every field in the factory's static alpha.6 tuple.
// The three budgets are EVM block counts, rather than wall-clock durations.
type DeploymentParameters struct {
	Levels           uint64
	Log2Step         uint64
	Height           uint64
	ResponseBudget   uint64
	CommitmentBudget uint64
	MaxAllowance     uint64
}

// DeploymentCallEvidence identifies a malformed response without retaining unbounded bytes.
type DeploymentCallEvidence struct {
	Target       common.Address
	Selector     [4]byte
	HasLevel     bool
	Level        uint64
	ReturnLength uint64
	ReturnHash   common.Hash
}

// DeploymentEvidence contains facts from one explicit block hash, including partial successful reads.
type DeploymentEvidence struct {
	Head      txsender.BlockRef
	Consensus common.Address

	FactoryKnown bool
	Factory      common.Address
	Alpha6Shape  bool
	Transition   common.Address
	MarchID      uint64
	Pristine     common.Hash
	Count        uint64
	Rows         []DeploymentParameters
	Malformed    *DeploymentCallEvidence
	Candidate    deploymentState
	Cause        deploymentCause
}

type deploymentKey struct {
	ID          int64
	Application common.Address
	Consensus   common.Address
}

func deploymentApplicationKey(app *model.Application) deploymentKey {
	return deploymentKey{ID: app.ID, Application: app.IApplicationAddress, Consensus: app.IConsensusAddress}
}

type deploymentAssessment struct {
	State           deploymentState
	PendingEvidence *DeploymentEvidence
}

func (a deploymentAssessment) confirmed() bool {
	return a.State == deploymentGeneration || a.State == deploymentTable
}

// Pending evidence is immutable after publication and is discarded after confirmation.
type deploymentSnapshot struct {
	Assessment   deploymentAssessment
	Head         txsender.BlockRef
	Cause        deploymentCause
	Alpha6Shape  bool
	ShapeFactory common.Address
}
