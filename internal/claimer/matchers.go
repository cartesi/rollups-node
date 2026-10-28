// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
)

// The full matchers return (matches, ok).
//
// ok=false means local epoch data is missing, so the comparison could not run.
// That is different from matches=false. Missing local data is a DB/state
// problem and should use the local-state corruption path. A real mismatch is a
// chain-vs-local claim disagreement and should use the divergence path.

func claimSubmittedEventMatches(application *model.Application, epoch *model.Epoch, event *iconsensus.IConsensusClaimSubmitted) (matches bool, ok bool) {
	if application == nil || epoch == nil || event == nil {
		return false, false
	}
	if epoch.TxBufferDataBlock == nil || epoch.MachineHash == nil {
		return false, false
	}
	return application.IApplicationAddress == event.AppContract &&
		*epoch.TxBufferDataBlock == event.OutputsMerkleRoot &&
		*epoch.MachineHash == event.MachineMerkleRoot &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64(), true
}

func claimAcceptedEventMatches(application *model.Application, epoch *model.Epoch, event *iconsensus.IConsensusClaimAccepted) (matches bool, ok bool) {
	if application == nil || epoch == nil || event == nil {
		return false, false
	}
	if epoch.TxBufferDataBlock == nil || epoch.MachineHash == nil {
		return false, false
	}
	return application.IApplicationAddress == event.AppContract &&
		*epoch.TxBufferDataBlock == event.OutputsMerkleRoot &&
		*epoch.MachineHash == event.MachineMerkleRoot &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64(), true
}

// claimAcceptedEventMatchesEpoch checks only app and lastBlock. It ignores the
// Merkle roots.
//
// This lets Quorum detect that another validator's claim was accepted for the
// same epoch, even if its roots differ from ours.
func claimAcceptedEventMatchesEpoch(application *model.Application, epoch *model.Epoch, event *iconsensus.IConsensusClaimAccepted) bool {
	if application == nil || epoch == nil || event == nil {
		return false
	}
	return application.IApplicationAddress == event.AppContract &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64()
}

// claimSubmittedEventMatchesEpoch checks only app and lastBlock.
//
// The event scan first finds events for the same epoch. A later full match
// decides whether the event is our claim or a different claim.
func claimSubmittedEventMatchesEpoch(application *model.Application, epoch *model.Epoch, event *iconsensus.IConsensusClaimSubmitted) bool {
	if application == nil || epoch == nil || event == nil {
		return false
	}
	return application.IApplicationAddress == event.AppContract &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64()
}

// claimStagedEventMatches checks app, lastBlock, outputs, and machine root.
// This is the normal path where our own claim was staged.
func claimStagedEventMatches(application *model.Application, epoch *model.Epoch, event *iconsensus.IConsensusClaimStaged) (matches bool, ok bool) {
	if application == nil || epoch == nil || event == nil {
		return false, false
	}
	if epoch.TxBufferDataBlock == nil || epoch.MachineHash == nil {
		return false, false
	}
	return application.IApplicationAddress == event.AppContract &&
		*epoch.TxBufferDataBlock == event.OutputsMerkleRoot &&
		*epoch.MachineHash == event.MachineMerkleRoot &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64(), true
}

// claimStagedEventMatchesEpoch checks only app and lastBlock.
//
// The staging scan must find any ClaimStaged event for our epoch. A later full
// match decides whether it is our claim or a different claim.
func claimStagedEventMatchesEpoch(application *model.Application, epoch *model.Epoch, event *iconsensus.IConsensusClaimStaged) bool {
	if application == nil || epoch == nil || event == nil {
		return false
	}
	return application.IApplicationAddress == event.AppContract &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64()
}
