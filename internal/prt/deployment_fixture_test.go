// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
)

// Existing protocol fixtures explicitly supply compatible deployment evidence.
// Reader rejection tests use the real raw-call adapter instead of this fixture.
func newCompatibleDaveConsensusAdapterMock(addresses ...common.Address) *daveConsensusAdapterMock {
	address := prtRevertTestApp().IConsensusAddress
	if len(addresses) != 0 {
		address = addresses[0]
	}
	return &daveConsensusAdapterMock{deploymentRead: func(_ context.Context, head txsender.BlockRef) (DeploymentEvidence, error) {
		evidence := compatibleDeploymentEvidence(head)
		evidence.Consensus = address
		return evidence, nil
	}}
}

func compatibleDeploymentEvidence(head txsender.BlockRef) DeploymentEvidence {
	return DeploymentEvidence{Head: head, Consensus: prtRevertTestApp().IConsensusAddress,
		FactoryKnown: true, Factory: common.HexToAddress("0x160"),
		Alpha6Shape: true,
		Transition:  common.HexToAddress("0x170"),
		MarchID:     22, Pristine: model.UarchPristineStateHash(), Count: 2,
		Rows: []DeploymentParameters{{Levels: 2, Log2Step: 38, Height: 54, ResponseBudget: 11, CommitmentBudget: 22, MaxAllowance: 33},
			{Levels: 2, Log2Step: 0, Height: 38, ResponseBudget: 11, CommitmentBudget: 22, MaxAllowance: 33}},
		Candidate: deploymentCompatible, Cause: deploymentCauseComplete}
}
