// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"
)

// divergenceStage names the claim step where the node saw different on-chain
// data than it expected.
type divergenceStage int

const (
	divergenceStageSubmit divergenceStage = iota
	divergenceStageStaging
	divergenceStageAcceptance
)

func (d divergenceStage) String() string {
	switch d {
	case divergenceStageSubmit:
		return "submission"
	case divergenceStageStaging:
		return "staging"
	case divergenceStageAcceptance:
		return "acceptance"
	default:
		return fmt.Sprintf("unknown(%d)", int(d))
	}
}

// divergenceBucket returns reason keys such as
// "authority_divergence_at_submission" or "quorum_divergence_at_staging".
// Keeping this in one place avoids repeating the string format in each
// handler.
func divergenceBucket(c model.Consensus, stage divergenceStage) string {
	consensus := "quorum"
	if c == model.Consensus_Authority {
		consensus = "authority"
	}
	return fmt.Sprintf("%s_divergence_at_%s", consensus, stage)
}

// markDivergence handles a claim that does not match what this node computed.
//
// In Quorum, before our local epoch reaches CLAIM_STAGED, a different claim
// can mean that another validator's vote won. In that case we mark the local
// epoch CLAIM_REJECTED and also mark the app DIVERGED. The two writes happen
// together so the epoch cannot disappear from claim work while the app stays
// runnable.
//
// After CLAIM_STAGED, the local DB says our claim was staged. If chain data
// later disagrees, the whole app is unsafe and becomes DIVERGED. Authority
// has only one submitter, so any divergence is app-level.
func (s *Service) markDivergence(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	stage divergenceStage,
	reasonText string,
) error {
	rejectable := app.ConsensusType == model.Consensus_Quorum &&
		stage != divergenceStageSubmit &&
		epoch.Status != model.EpochStatus_ClaimStaged
	if rejectable {
		return s.rejectEpochAndSetApplicationDiverged(ctx, app, epoch, reasonText)
	}
	return s.setApplicationDiverged(ctx, app, "%s", reasonText)
}

// markStagingDivergence handles a ClaimStaged event for our epoch whose data
// differs from our local claim. markDivergence decides whether this is only an
// epoch reject or an app-level failure.
func (s *Service) markStagingDivergence(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	event *iconsensus.IConsensusClaimStaged,
	site string,
) error {
	ourMachineMerkleRoot := common.Hash{}
	if epoch.MachineHash != nil {
		ourMachineMerkleRoot = *epoch.MachineHash
	}
	reason := fmt.Sprintf(
		"%s: divergent ClaimStaged observed at %s. "+
			"on-chain machineMerkleRoot=%s, our machineMerkleRoot=%s, "+
			"epoch %d (lastBlock %d). "+
			"Guardian SHOULD call foreclose() on the application contract "+
			"before staged_at_block + claim_staging_period elapses, "+
			"after which outputs from this divergent claim become executable.",
		divergenceBucket(app.ConsensusType, divergenceStageStaging), site,
		common.BytesToHash(event.MachineMerkleRoot[:]).Hex(),
		ourMachineMerkleRoot.Hex(),
		epoch.Index, epoch.LastBlock,
	)
	return s.markDivergence(ctx, app, epoch, divergenceStageStaging, reason)
}

// markSubmittedDivergence handles a ClaimSubmitted event whose data differs
// from our local claim. This is always an app-level problem. Even in Quorum,
// if the submitted claim later gets staged, our local claim is wrong.
func (s *Service) markSubmittedDivergence(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	event *iconsensus.IConsensusClaimSubmitted,
	site string,
) error {
	ourOutputsMerkleRoot := common.Hash{}
	if epoch.OutputsMerkleRoot != nil {
		ourOutputsMerkleRoot = *epoch.OutputsMerkleRoot
	}
	ourMachineMerkleRoot := common.Hash{}
	if epoch.MachineHash != nil {
		ourMachineMerkleRoot = *epoch.MachineHash
	}
	reason := fmt.Sprintf(
		"%s: divergent ClaimSubmitted observed at %s. "+
			"on-chain outputsMerkleRoot=%s machineMerkleRoot=%s, "+
			"our outputsMerkleRoot=%s machineMerkleRoot=%s, "+
			"epoch %d (lastBlock %d).",
		divergenceBucket(app.ConsensusType, divergenceStageSubmit), site,
		common.BytesToHash(event.OutputsMerkleRoot[:]).Hex(),
		common.BytesToHash(event.MachineMerkleRoot[:]).Hex(),
		ourOutputsMerkleRoot.Hex(), ourMachineMerkleRoot.Hex(),
		epoch.Index, epoch.LastBlock,
	)
	return s.markDivergence(ctx, app, epoch, divergenceStageSubmit, reason)
}

// markAcceptedDivergence handles a ClaimAccepted event whose data differs from
// our local claim. markDivergence decides whether this rejects only the epoch
// or marks the whole app DIVERGED.
func (s *Service) markAcceptedDivergence(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	event *iconsensus.IConsensusClaimAccepted,
	site string,
) error {
	ourOutputsMerkleRoot := common.Hash{}
	if epoch.OutputsMerkleRoot != nil {
		ourOutputsMerkleRoot = *epoch.OutputsMerkleRoot
	}
	ourMachineMerkleRoot := common.Hash{}
	if epoch.MachineHash != nil {
		ourMachineMerkleRoot = *epoch.MachineHash
	}
	reason := fmt.Sprintf(
		"%s: divergent ClaimAccepted observed at %s. "+
			"on-chain outputsMerkleRoot=%s machineMerkleRoot=%s, "+
			"our outputsMerkleRoot=%s machineMerkleRoot=%s, "+
			"epoch %d (lastBlock %d). "+
			"Outputs from this divergent claim are now executable on-chain; "+
			"manual remediation required.",
		divergenceBucket(app.ConsensusType, divergenceStageAcceptance), site,
		common.BytesToHash(event.OutputsMerkleRoot[:]).Hex(),
		common.BytesToHash(event.MachineMerkleRoot[:]).Hex(),
		ourOutputsMerkleRoot.Hex(), ourMachineMerkleRoot.Hex(),
		epoch.Index, epoch.LastBlock,
	)
	return s.markDivergence(ctx, app, epoch, divergenceStageAcceptance, reason)
}

func (s *Service) setApplicationDiverged(
	ctx context.Context,
	app *model.Application,
	reasonFmt string,
	args ...any,
) error {
	return appstatus.SetDivergedf(ctx, s.Logger, s.repository, app, reasonFmt, args...)
}

func (s *Service) setApplicationCorrupted(
	ctx context.Context,
	app *model.Application,
	reasonFmt string,
	args ...any,
) error {
	return appstatus.SetCorruptedf(ctx, s.Logger, s.repository, app, reasonFmt, args...)
}

func (s *Service) rejectEpochAndSetApplicationDiverged(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	reason string,
) error {
	s.Logger.Error("marking application as diverged (terminal)",
		"application", app.Name,
		"address", app.IApplicationAddress.String(),
		"reason", reason)

	err := s.repository.RejectEpochAndSetApplicationDiverged(
		ctx, app.ID, epoch.Index, reason)
	reasonErr := errors.New(reason)
	if err != nil {
		s.Logger.Error("failed to reject epoch and update application status",
			"application", app.Name,
			"address", app.IApplicationAddress.String(),
			"epoch_index", epoch.Index,
			"error", err)
		return errors.Join(reasonErr, err)
	}

	app.Status = model.ApplicationStatus_Diverged
	app.Reason = &reason
	epoch.Status = model.EpochStatus_ClaimRejected
	return reasonErr
}

// markMatcherPrecondFailure marks an app CORRUPTED when local epoch data is
// missing. The matcher needs outputs_merkle_root and machine_hash to compare a
// chain event with our local claim. Those fields should be present after
// CLAIM_COMPUTED, so missing values mean local state was corrupted or changed
// later. The node cannot safely continue because it cannot compare claims, and
// the missing data cannot be reconstructed by restarting — the status is
// terminal.
func (s *Service) markMatcherPrecondFailure(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	site string,
) error {
	return s.setApplicationCorrupted(ctx, app,
		"%s: cannot compare epoch %d (%d) against chain event — local row is missing "+
			"outputs_merkle_root or machine_hash. This is terminal; inspect the epoch row.",
		site, epoch.Index, epoch.VirtualIndex)
}

// verifyClaimOutputsMatch checks the outputs returned by getClaim.
//
// getClaim looks up a claim by app, last processed block, and machine root.
// If the chain says that claim is STAGED or ACCEPTED, its outputs root should
// match our local outputs root. A mismatch means this node and the submitter
// disagree about the claim. It can also indicate a spoof attempt, because the
// lookup key includes our machine root but the outputs root comes from whoever
// submitted the claim.
//
// Returns nil when the outputs match or when there is not enough local data to
// check. Returns an error after marking the app DIVERGED when they differ.
func (s *Service) verifyClaimOutputsMatch(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	claim iconsensus.IConsensusClaim,
	site string,
) error {
	if epoch.OutputsMerkleRoot == nil {
		// Other paths mark this as a local data problem. Here we only compare
		// outputs when the local value exists.
		return nil
	}
	chainStagedOutputs := common.BytesToHash(claim.StagedOutputsMerkleRoot[:])
	if chainStagedOutputs == *epoch.OutputsMerkleRoot {
		return nil
	}
	status := fmt.Sprintf("status %d", claim.Status)
	switch claim.Status {
	case claimStatusStaged:
		status = "STAGED"
	case claimStatusAccepted:
		status = "ACCEPTED"
	}
	return s.setApplicationDiverged(ctx, app,
		"chain_claim_outputs_mismatch: %s — getClaim returned %s for our "+
			"(app, lpbn, machineMerkleRoot) tuple but with stagedOutputsMerkleRoot=%s "+
			"while our local outputs_merkle_root is %s. Epoch %d (lastBlock %d). "+
			"Indicates determinism breakage between this node and the submitter of our "+
			"machineMerkleRoot; manual remediation required.",
		site, status,
		chainStagedOutputs.Hex(),
		epoch.OutputsMerkleRoot.Hex(),
		epoch.Index, epoch.LastBlock)
}

type consensusAddressCheckKey struct {
	appID       int64
	blockNumber string
}

func (s *Service) checkConsensusForAddressChange(
	ctx context.Context,
	app *model.Application,
	defaultBlockNumber *big.Int,
) error {
	if s.consensusAddressChecks != nil {
		blockNumber := "<nil>"
		if defaultBlockNumber != nil {
			blockNumber = defaultBlockNumber.String()
		}
		key := consensusAddressCheckKey{
			appID:       app.ID,
			blockNumber: blockNumber,
		}
		if err, ok := s.consensusAddressChecks[key]; ok {
			return err
		}
		err := s.checkConsensusForAddressChangeUncached(ctx, app, defaultBlockNumber)
		s.consensusAddressChecks[key] = err
		return err
	}
	return s.checkConsensusForAddressChangeUncached(ctx, app, defaultBlockNumber)
}

func (s *Service) checkConsensusForAddressChangeUncached(
	ctx context.Context,
	app *model.Application,
	defaultBlockNumber *big.Int,
) error {
	newConsensusAddress, err := s.blockchain.getConsensusAddress(ctx, app, defaultBlockNumber)
	if err != nil {
		return fmt.Errorf("getting consensus address for app %v: %w", app.IApplicationAddress, err)
	}
	if app.IConsensusAddress != newConsensusAddress {
		err = s.setApplicationCorrupted(
			ctx,
			app,
			"consensus change detected. application: %v.",
			app.IApplicationAddress,
		)
		return err
	}
	return nil
}
