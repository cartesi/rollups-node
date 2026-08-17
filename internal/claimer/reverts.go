// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"fmt"
	"math/big"
	"reflect"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

type submitClaimRevertOutcome int

const (
	// submitClaimUnknown means the error is not a known IConsensus revert.
	// The caller should return it as an unexpected error.
	submitClaimUnknown submitClaimRevertOutcome = iota

	// submitClaimAlreadyOnChain means the claim was already recorded on chain.
	// This is common for Authority after restart, when NotFirstClaim means the
	// prior submit already emitted the needed events. The caller should wait for
	// the normal event scan to update the DB.
	submitClaimAlreadyOnChain

	// submitClaimAppHalted means this handler already changed the app status to
	// DIVERGED, CORRUPTED, or FAILED. DIVERGED/CORRUPTED are terminal for normal
	// work; FAILED is recoverable by operator action. The caller should remove
	// this epoch from its work map and return the status-change error, if any.
	submitClaimAppHalted

	// submitClaimRetryLater means a later tick may make progress. For example,
	// Quorum may need to see an event from a previous vote, or EVM reader may
	// still need to record foreclosure. Keep the epoch and retry next tick.
	submitClaimRetryLater
)

// Solidity ClaimStatus values from IConsensus.sol. getClaim() returns these
// values, and ClaimNotStaged includes one of them in the revert data.
const (
	claimStatusUnstaged uint8 = 0
	claimStatusStaged   uint8 = 1
	claimStatusAccepted uint8 = 2
)

// acceptClaimRevertOutcome describes what the caller should do after an
// acceptClaim revert. It is separate from submitClaimRevertOutcome because
// acceptClaim has different expected errors.
type acceptClaimRevertOutcome int

const (
	// acceptClaimUnknown means the caller should return the error.
	acceptClaimUnknown acceptClaimRevertOutcome = iota

	// acceptClaimReconciledAccepted means another party accepted our claim
	// first. The caller should record CLAIM_ACCEPTED locally.
	acceptClaimReconciledAccepted

	// acceptClaimAppHalted means this handler already changed the app status to
	// DIVERGED, CORRUPTED, or FAILED. The caller should return statusErr and
	// drop the work.
	acceptClaimAppHalted

	// acceptClaimRetryLater means the next tick may make progress. Examples:
	// the staging period is not over yet, or EVM reader still needs to record
	// foreclosure so later claim work can be partitioned away from broadcasts.
	acceptClaimRetryLater
)

// handleSubmitClaimRevert checks whether a submitClaim error is a known v3
// IConsensus revert. It may update app status or write a log message. The
// returned outcome is an internal node action — what the caller should do
// next — not a direct translation of the Solidity revert semantics.
//
// v3 submitClaim can return these known reverts:
//
//   - NotFirstClaim: Authority already has an epoch claim; Quorum already has
//     this validator's vote for this epoch. Wait for event sync. Event data,
//     not only the revert name, decides whether this is a mismatch.
//   - ApplicationForeclosed: retry later while EVM reader records foreclose_block.
//     The app remains enabled for L1 observation; normal work stops once
//     foreclose_block is recorded.
//   - InvalidOutputsMerkleRootProofSize: CORRUPTED; local data corruption.
//   - InvalidNodeIndex: CORRUPTED; the outputs merkle proof in the DB does not
//     form a valid machine-tree replacement proof.
//   - CallerIsNotValidator: FAILED; wrong operator key.
//   - the reverts shared with acceptClaim (see classifySharedConsensusRevert):
//     ApplicationNotDeployed, ApplicationReverted,
//     IllformedApplicationReturnData, NotEpochFinalBlock (all FAILED) and
//     NotPastBlock (retry later).
//
// ClaimNotStaged and ClaimStagingPeriodNotOverYet only come from acceptClaim,
// so handleAcceptClaimRevert handles them.
func (s *Service) handleSubmitClaimRevert(
	err error,
	app *model.Application,
	epoch *model.Epoch,
) (submitClaimRevertOutcome, error) {
	switch {
	case ethutil.IsNonceTooLowError(err):
		// A transaction with this signer nonce was already mined. This can
		// happen after a restart: the old process sent the transaction, and
		// the new process tries the same nonce before it sees the old receipt.
		// On the next tick, getClaim will show whether the old transaction
		// reached STAGED or ACCEPTED. If not, the next broadcast gets a fresh
		// nonce.
		s.Logger.Info(
			"submitClaim broadcast rejected with 'nonce too low'; "+
				"deferring to the next tick's getClaim reconciliation",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
			"last_block", epoch.LastBlock,
		)
		return submitClaimRetryLater, nil

	case isCustomConsensusError(err, "NotFirstClaim"):
		// Gas estimation runs the call before sending the transaction. With the
		// default GasLimit == 0, this revert is caught before spending gas. If
		// GasLimit were set manually, the transaction could revert on chain.
		//
		// Authority: submitClaim allows only one claim per epoch. A duplicate
		// reverts with NotFirstClaim, even if the Merkle root is the same.
		// After restart this can be harmless: the node recomputed the same
		// claim that was already on chain.
		//
		// Quorum v3 checks whether this validator already voted in the epoch.
		// A second vote reverts, even for the same machine root. Treat this as
		// "a prior vote exists". Later event checks decide whether that vote
		// matches our current computation.
		if app.ConsensusType == model.Consensus_Quorum {
			s.Logger.Warn(
				"submitClaim reverted with NotFirstClaim on Quorum; waiting for event reconciliation",
				"app", app.IApplicationAddress,
				"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
				"last_block", epoch.LastBlock,
			)
			return submitClaimRetryLater, nil
		}
		s.Logger.Info("Claim already on-chain, waiting for event sync",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
			"last_block", epoch.LastBlock,
		)
		return submitClaimAlreadyOnChain, nil

	case isCustomConsensusError(err, "ApplicationForeclosed"):
		// EVM reader should record foreclose_block soon. Keep the epoch for now.
		// Later ticks will see foreclose_block and skip new broadcasts. Read-only
		// reconciliation can still copy any already-accepted chain state into
		// the DB.
		s.Logger.Warn("submitClaim reverted with ApplicationForeclosed; "+
			"awaiting Foreclosure observer to record the foreclosure marker",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
			"last_block", epoch.LastBlock,
		)
		return submitClaimRetryLater, nil

	case isCustomConsensusError(err, "InvalidOutputsMerkleRootProofSize"):
		stateErr := s.setApplicationCorrupted(
			s.Context, app,
			"submitClaim reverted with InvalidOutputsMerkleRootProofSize for "+
				"epoch %d (%d), last_block %d — tx_buffer_proof in DB is "+
				"the wrong length for the machine memory tree.",
			epoch.Index, epoch.VirtualIndex, epoch.LastBlock,
		)
		return submitClaimAppHalted, stateErr

	case isCustomConsensusError(err, "CallerIsNotValidator"):
		// Operator configuration error: the signing key is not a Quorum
		// validator. The operator can fix the key, so use FAILED rather than a
		// terminal DIVERGED/CORRUPTED status.
		stateErr := appstatus.SetFailedf(s.Context, s.Logger, s.repository, app,
			"submitClaim reverted with CallerIsNotValidator: the configured "+
				"signing key is not a member of the Quorum for app %s. "+
				"Check the validator key configuration.",
			app.IApplicationAddress,
		)
		return submitClaimAppHalted, stateErr

	case isCustomApplicationError(err, "InvalidNodeIndex"):
		// Raised by the on-chain merkle library while replacing the
		// outputs-root leaf in the machine memory tree: the proof's shape does
		// not fit the tree. Like InvalidOutputsMerkleRootProofSize, this means
		// the proof stored locally is bad — local data corruption.
		stateErr := s.setApplicationCorrupted(
			s.Context, app,
			"submitClaim reverted with InvalidNodeIndex for "+
				"epoch %d (%d), last_block %d — tx_buffer_proof in DB does "+
				"not form a valid replacement proof for the machine memory tree.",
			epoch.Index, epoch.VirtualIndex, epoch.LastBlock,
		)
		return submitClaimAppHalted, stateErr
	}

	switch action, stateErr := s.classifySharedConsensusRevert("submitClaim", err, app, epoch); action {
	case sharedRevertAppHalted:
		return submitClaimAppHalted, stateErr
	case sharedRevertRetryLater:
		return submitClaimRetryLater, nil
	case sharedRevertNoMatch:
		// Not a shared revert either; report unknown.
	}
	return submitClaimUnknown, nil
}

// sharedRevertAction tells the caller how to map a revert that submitClaim
// and acceptClaim have in common onto its own outcome type.
type sharedRevertAction int

const (
	// sharedRevertNoMatch means the error is not one of the shared reverts.
	sharedRevertNoMatch sharedRevertAction = iota

	// sharedRevertAppHalted means the classifier already changed the app
	// status; the caller should drop the work item and propagate the
	// returned state-change error, if any.
	sharedRevertAppHalted

	// sharedRevertRetryLater means a later tick may make progress; the
	// classifier already logged the condition.
	sharedRevertRetryLater
)

// classifySharedConsensusRevert handles the reverts that submitClaim and
// acceptClaim have in common. Both methods run the application foreclosure
// probe — ApplicationNotDeployed, ApplicationReverted, and
// IllformedApplicationReturnData (all FAILED) — and validate the last
// processed block number — NotEpochFinalBlock (FAILED) and NotPastBlock
// (retry later). call names the reverting method in reasons and logs.
func (s *Service) classifySharedConsensusRevert(
	call string,
	err error,
	app *model.Application,
	epoch *model.Epoch,
) (sharedRevertAction, error) {
	switch {
	case isCustomConsensusError(err, "ApplicationNotDeployed"):
		// The consensus contract found no code at the application address —
		// the common wrong-address case, which the contract reports separately
		// from ApplicationReverted. A provider simulating against a block just
		// before a fresh deployment could fire this transiently, but FAILED is
		// recoverable, so that rare case only costs an operator re-enable.
		stateErr := appstatus.SetFailedf(s.Context, s.Logger, s.repository, app,
			"%s reverted with ApplicationNotDeployed for app %s, "+
				"epoch %d (%d), last_block %d: no contract code exists at the "+
				"application address. Verify the application address and that "+
				"the application contract is deployed on this chain before "+
				"re-enabling.",
			call, app.IApplicationAddress, epoch.Index, epoch.VirtualIndex, epoch.LastBlock,
		)
		return sharedRevertAppHalted, stateErr

	case isCustomConsensusError(err, "ApplicationReverted"):
		// Not a divergence (no on-chain claim disagrees with ours) and not
		// local DB corruption — operator action can fix it, so FAILED. The
		// application's own revert data is the only evidence of why the call
		// reverted — and an adversarial application contract can revert
		// selectively (e.g. per tx.origin) to suppress a targeted validator's
		// votes — so the reason carries it.
		stateErr := appstatus.SetFailedf(s.Context, s.Logger, s.repository, app,
			"%s reverted with ApplicationReverted for app %s, "+
				"epoch %d (%d), last_block %d: the application contract reverted "+
				"when the consensus contract queried it. Verify the deployed "+
				"contract and its compatibility with the consensus contract "+
				"before re-enabling.%s",
			call, app.IApplicationAddress, epoch.Index, epoch.VirtualIndex, epoch.LastBlock,
			appReturnDataSuffix(err, "ApplicationReverted"),
		)
		return sharedRevertAppHalted, stateErr

	case isCustomConsensusError(err, "IllformedApplicationReturnData"):
		stateErr := appstatus.SetFailedf(s.Context, s.Logger, s.repository, app,
			"%s reverted with IllformedApplicationReturnData for app %s, "+
				"epoch %d (%d), last_block %d: the application contract returned "+
				"malformed data when the consensus contract queried it. Verify "+
				"the deployed contract and its compatibility with the consensus "+
				"contract before re-enabling.%s",
			call, app.IApplicationAddress, epoch.Index, epoch.VirtualIndex, epoch.LastBlock,
			appReturnDataSuffix(err, "IllformedApplicationReturnData"),
		)
		return sharedRevertAppHalted, stateErr

	case isCustomConsensusError(err, "NotEpochFinalBlock"):
		stateErr := appstatus.SetFailedf(s.Context, s.Logger, s.repository, app,
			"%s reverted with NotEpochFinalBlock for app %s, "+
				"epoch %d (%d), last_block %d: the node submitted a "+
				"lastProcessedBlockNumber that the consensus contract does not "+
				"consider an epoch final block. Verify epoch length, consensus "+
				"contract, persisted node_config, and chain configuration before "+
				"re-enabling.",
			call, app.IApplicationAddress, epoch.Index, epoch.VirtualIndex, epoch.LastBlock,
		)
		return sharedRevertAppHalted, stateErr

	case isCustomConsensusError(err, "NotPastBlock"):
		// Can be transient: the RPC provider may simulate the call against a
		// block newer or different from the block used by this tick's pinned
		// reads. It is permanent when last_block is beyond the real chain head
		// (wrong chain endpoint or broken epoch arithmetic); the decoded bounds
		// make the two cases distinguishable in logs.
		logArgs := []any{
			"app", app.IApplicationAddress,
			"epoch_index", epoch.Index,
			"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
			"last_block", epoch.LastBlock,
		}
		if lastProcessed, upperBound, ok := decodeNotPastBlockBounds(err); ok {
			logArgs = append(logArgs,
				"submitted_last_block", lastProcessed,
				"chain_upper_bound", upperBound,
			)
		}
		s.Logger.Warn(call+" reverted with NotPastBlock; block may not be "+
			"past relative to the provider's simulation block, retrying next tick",
			logArgs...,
		)
		return sharedRevertRetryLater, nil
	}
	return sharedRevertNoMatch, nil
}

// handleAcceptClaimRevert checks whether an acceptClaim error is a known v3
// IConsensus revert and returns what the caller should do next:
//
//   - ClaimNotStaged with claimStatus=ACCEPTED: another party accepted first;
//     update the DB to CLAIM_ACCEPTED. This costs one reverted tx, but it is
//     not an app-health failure.
//   - ClaimNotStaged with claimStatus=UNSTAGED: this violates the local staged
//     invariant at the finalized block. Retry and let the operator fix
//     block/configuration issues.
//   - ClaimStagingPeriodNotOverYet: retry later.
//   - ApplicationForeclosed: retry until EVM reader records foreclosure and the
//     normal foreclosed-app gates stop future broadcasts.
//
// acceptClaim also runs the application foreclosure probe and validates the
// last processed block number, so it shares submitClaim's
// ApplicationNotDeployed / ApplicationReverted / IllformedApplicationReturnData
// / NotEpochFinalBlock (all FAILED) and NotPastBlock (retry later) reverts —
// classified by classifySharedConsensusRevert.
func (s *Service) handleAcceptClaimRevert(
	err error,
	app *model.Application,
	epoch *model.Epoch,
) (acceptClaimRevertOutcome, error) {
	switch {
	case ethutil.IsNonceTooLowError(err):
		// Same idea as submitClaim: a transaction with this signer nonce was
		// already mined. On the next tick, getClaim will show whether our old
		// acceptClaim landed. If it did, we update the DB. If not, we send
		// another transaction with a fresh nonce. acceptAttempts still limits
		// how many times this can repeat.
		s.Logger.Info(
			"acceptClaim broadcast rejected with 'nonce too low'; "+
				"deferring to the next tick's getClaim reconciliation",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
			"last_block", epoch.LastBlock,
		)
		return acceptClaimRetryLater, nil

	case isCustomConsensusError(err, "ClaimNotStaged"):
		status, ok := decodeClaimNotStagedStatus(err)
		if !ok {
			s.Logger.Warn("acceptClaim reverted with ClaimNotStaged but the status could not be decoded; "+
				"will retry next tick",
				"app", app.IApplicationAddress,
				"epoch_index", epoch.Index,
			)
			return acceptClaimRetryLater, nil
		}
		switch status {
		case claimStatusAccepted:
			s.Logger.Info("Claim was accepted by a front-runner; reconciling",
				"app", app.IApplicationAddress,
				"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
				"last_block", epoch.LastBlock,
			)
			return acceptClaimReconciledAccepted, nil
		case claimStatusUnstaged:
			s.Logger.Warn("acceptClaim reverted with ClaimNotStaged(UNSTAGED); "+
				"the on-chain status disagrees with our local view. "+
				"This can happen under reorgs when reading non-final blocks; "+
				"retry on the next tick.",
				"app", app.IApplicationAddress,
				"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
				"last_block", epoch.LastBlock,
			)
			return acceptClaimRetryLater, nil
		case claimStatusStaged:
			// ClaimNotStaged reporting STAGED is contradictory but a modeled
			// status (a race against the claim being staged). Retry rather than
			// escalate — it can resolve on its own next tick.
			s.Logger.Warn("acceptClaim reverted with ClaimNotStaged(STAGED); "+
				"on-chain status disagrees with our local view, retrying next tick",
				"app", app.IApplicationAddress,
				"epoch_index", epoch.Index,
			)
			return acceptClaimRetryLater, nil
		default:
			// ClaimNotStaged decoded cleanly but carries a ClaimStatus this node
			// does not model (the enum is 0/1/2). Unlike a decode failure —
			// which can be a transient malformed response and is retried above —
			// a value that round-tripped through the ABI came from the contract,
			// so retrying cannot fix it: the IConsensus contract is most likely a
			// newer version than this node. Surface it as FAILED (recoverable: an
			// operator can upgrade the node and re-enable) instead of spinning
			// silently every tick.
			stateErr := appstatus.SetFailedf(s.Context, s.Logger, s.repository, app,
				"acceptClaim reverted with ClaimNotStaged carrying unmodeled ClaimStatus %d for "+
					"epoch %d (%d) — this node models only 0/1/2. The IConsensus contract may be "+
					"newer than this node supports; upgrade the node or verify the contract.",
				status, epoch.Index, epoch.VirtualIndex)
			return acceptClaimAppHalted, stateErr
		}

	case isCustomConsensusError(err, "ClaimStagingPeriodNotOverYet"):
		s.Logger.Warn("acceptClaim reverted with ClaimStagingPeriodNotOverYet; "+
			"local arithmetic disagrees with chain. Will retry next tick.",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
			"last_block", epoch.LastBlock,
		)
		return acceptClaimRetryLater, nil

	case isCustomConsensusError(err, "ApplicationForeclosed"):
		s.Logger.Warn("acceptClaim reverted with ApplicationForeclosed; "+
			"awaiting Foreclosure observer to record the foreclosure marker",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
			"last_block", epoch.LastBlock,
		)
		return acceptClaimRetryLater, nil
	}

	switch action, stateErr := s.classifySharedConsensusRevert("acceptClaim", err, app, epoch); action {
	case sharedRevertAppHalted:
		return acceptClaimAppHalted, stateErr
	case sharedRevertRetryLater:
		return acceptClaimRetryLater, nil
	case sharedRevertNoMatch:
		// Not a shared revert either; report unknown.
	}
	return acceptClaimUnknown, nil
}

// unpackConsensusRevert checks that err carries revert data for the named
// IConsensus error and ABI-decodes its arguments. It returns (values, true)
// when the selector matches and the data decodes cleanly, and (nil, false)
// otherwise. For errors declared in other ABIs, call ethutil.UnpackRevert
// with the appropriate metadata directly.
func unpackConsensusRevert(err error, name string) ([]any, bool) {
	return ethutil.UnpackRevert(err, iconsensus.IConsensusMetaData, name)
}

// decodeClaimNotStagedStatus reads claimStatus from a ClaimNotStaged revert.
// It returns (status, true) when decoding succeeds. It returns (0, false) when
// decoding fails, and the caller should treat that as "unknown, retry later".
//
// The ClaimNotStaged ABI (IConsensus.sol):
//
//	error ClaimNotStaged(
//	    address appContract,
//	    uint256 lastProcessedBlockNumber,
//	    bytes32 machineMerkleRoot,
//	    enum ClaimStatus claimStatus);
func decodeClaimNotStagedStatus(err error) (uint8, bool) {
	values, ok := unpackConsensusRevert(err, "ClaimNotStaged")
	if !ok || len(values) < 4 {
		return 0, false
	}
	// Use reflection instead of a direct `.(uint8)` cast. abigen returns uint8
	// today, but a future version may return a named uint8 type. Checking the
	// kind works for both forms.
	v := reflect.ValueOf(values[3])
	if !v.IsValid() || v.Kind() != reflect.Uint8 {
		return 0, false
	}
	return uint8(v.Uint()), true
}

// appReturnDataSuffix formats the application-provided returndata carried by
// ApplicationReverted and IllformedApplicationReturnData reverts as a reason
// suffix:
//
//	error ApplicationReverted(address appContract, bytes returndata);
//	error IllformedApplicationReturnData(address appContract, bytes returndata);
//
// The bytes are controlled by the application contract, so they are
// hex-encoded rather than string-decoded to keep them inert in logs and in
// the database. Returns "" when the revert data cannot be decoded.
func appReturnDataSuffix(err error, name string) string {
	values, ok := unpackConsensusRevert(err, name)
	if !ok || len(values) < 2 {
		return ""
	}
	data, ok := values[1].([]byte)
	if !ok {
		return ""
	}
	return fmt.Sprintf(" Application return data: 0x%x.", data)
}

// decodeNotPastBlockBounds reads the arguments from a NotPastBlock revert:
//
//	error NotPastBlock(uint256 lastProcessedBlockNumber, uint256 upperBound);
//
// upperBound is the simulating node's block number: slightly behind
// lastProcessedBlockNumber means a lagging provider that catches up on its
// own; far behind means the node is pointed at the wrong chain or the epoch
// arithmetic is wrong.
func decodeNotPastBlockBounds(err error) (lastProcessed, upperBound *big.Int, ok bool) {
	values, ok := unpackConsensusRevert(err, "NotPastBlock")
	if !ok || len(values) < 2 {
		return nil, nil, false
	}
	lastProcessed, okLast := values[0].(*big.Int)
	upperBound, okUpper := values[1].(*big.Int)
	if !okLast || !okUpper {
		return nil, nil, false
	}
	return lastProcessed, upperBound, true
}
