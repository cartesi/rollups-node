// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

type daveConsensusSnapshot struct {
	sealed CurrentSealedEpoch
	stage  CanStageTournamentResult
	accept CanAcceptStagedTournamentResult
}

func readDaveConsensusSnapshot(
	ctx context.Context,
	consensus DaveConsensusAdapter,
	mostRecentBlock uint64,
) (daveConsensusSnapshot, error) {
	callOpts := pinnedCallOpts(ctx, mostRecentBlock)
	sealed, err := consensus.GetCurrentSealedEpoch(callOpts)
	if err != nil {
		return daveConsensusSnapshot{}, fmt.Errorf("reading current sealed epoch: %w", err)
	}
	stage, err := consensus.CanStageTournamentResult(callOpts)
	if err != nil {
		return daveConsensusSnapshot{}, fmt.Errorf("reading tournament stage readiness: %w", err)
	}
	accept, err := consensus.CanAcceptStagedTournamentResult(callOpts)
	if err != nil {
		return daveConsensusSnapshot{}, fmt.Errorf("reading tournament accept readiness: %w", err)
	}
	snapshot := daveConsensusSnapshot{sealed: sealed, stage: stage, accept: accept}
	if err := validateDaveConsensusSnapshot(snapshot, mostRecentBlock); err != nil {
		return daveConsensusSnapshot{}, err
	}
	return snapshot, nil
}

func validateDaveConsensusSnapshot(snapshot daveConsensusSnapshot, mostRecentBlock uint64) error {
	sealed := snapshot.sealed
	stage := snapshot.stage
	accept := snapshot.accept
	if sealed.EpochNumber != stage.EpochNumber || sealed.EpochNumber != accept.EpochNumber {
		return fmt.Errorf("inconsistent DaveConsensus epoch numbers: sealed=%d stage=%d accept=%d",
			sealed.EpochNumber, stage.EpochNumber, accept.EpochNumber)
	}
	if sealed.IsTournamentResultStaged != stage.IsTournamentResultStaged ||
		sealed.IsTournamentResultStaged != accept.IsTournamentResultStaged {
		return fmt.Errorf("inconsistent DaveConsensus staged flags: sealed=%t stage=%t accept=%t",
			sealed.IsTournamentResultStaged,
			stage.IsTournamentResultStaged,
			accept.IsTournamentResultStaged)
	}
	if sealed.InputIndexLowerBound > sealed.InputIndexUpperBound {
		return fmt.Errorf("sealed epoch %d has input lower bound %d above upper bound %d",
			sealed.EpochNumber, sealed.InputIndexLowerBound, sealed.InputIndexUpperBound)
	}
	if sealed.Tournament == (common.Address{}) {
		return fmt.Errorf("sealed epoch %d has zero tournament address", sealed.EpochNumber)
	}

	if sealed.IsTournamentResultStaged {
		if sealed.StagingBlockNumber == 0 || sealed.StagingBlockNumber > mostRecentBlock {
			return fmt.Errorf("sealed epoch %d has invalid staging block %d at observed block %d",
				sealed.EpochNumber, sealed.StagingBlockNumber, mostRecentBlock)
		}
		if sealed.StagedPostEpochMachineStateHash != accept.StagedPostEpochMachineStateHash {
			return errors.New("staged machine state differs between sealed and accept views")
		}
		if sealed.StagedPostEpochOutputsMerkleRoot != accept.StagedPostEpochOutputsMerkleRoot {
			return errors.New("staged outputs root differs between sealed and accept views")
		}
		if stage.WinnerPostEpochMachineStateHash != sealed.StagedPostEpochMachineStateHash {
			return errors.New("staged machine state differs from the tournament winner")
		}
	}
	return nil
}

// progressTournamentResult records results at the published tournament block,
// bounded by the configured head. A separate latest snapshot decides whether
// to send a stage or accept transaction now.
// A non-nil return value is the epoch that can be joined from that latest snapshot.
// Recovery is allowed only during a known wait or to reconcile a sent refund.
func (s *Service) progressTournamentResult(
	ctx context.Context,
	app *model.Application,
	confirmedBlock uint64,
	latestBlock uint64,
) (*model.Epoch, bool, error) {
	recovery, err := s.rootBondRecoveryInFlight(app.ID)
	if err != nil {
		return nil, false, err
	}
	if recovery != nil {
		if s.hasNonRecoveryMutationInFlight(app.ID) {
			return nil, false, fmt.Errorf("application %s has bond recovery and another PRT transaction in flight",
				app.IApplicationAddress)
		}
		s.Logger.Info("Tournament actions wait for pending root bond recovery",
			"application", app.Name, "epoch_index", recovery.EpochIndex, "tx", *recovery.TxHash)
		return nil, true, nil
	}
	if blocked, err := s.waitForTournamentTransaction(ctx, app, latestBlock); blocked || err != nil {
		return nil, false, err
	}

	consensus, err := s.adapterFactory.CreateDaveConsensusAdapter(app.IConsensusAddress)
	if err != nil {
		return nil, false, fmt.Errorf("binding DaveConsensus for app %s: %w", app.IApplicationAddress, err)
	}
	observedBlock := min(confirmedBlock, app.LastTournamentCheckBlock)
	var snapshot daveConsensusSnapshot
	var epoch *model.Epoch
	observedSnapshot := false
	// Root discovery can stay behind a moving configured head. Read the result
	// at the committed window instead of waiting for those heads to coincide.
	// Zero means that no tournament window has been published yet.
	if observedBlock != 0 {
		snapshot, err = readDaveConsensusSnapshot(ctx, consensus, observedBlock)
		if err != nil && !errors.Is(err, bind.ErrNoCode) {
			return nil, false, fmt.Errorf("reading DaveConsensus snapshot for app %s: %w", app.IApplicationAddress, err)
		}
		if err == nil {
			observedSnapshot = true
			epoch, err = s.repository.GetEpoch(ctx, app.IApplicationAddress.Hex(), snapshot.sealed.EpochNumber)
			if err != nil {
				return nil, false, fmt.Errorf("loading current sealed epoch %d: %w", snapshot.sealed.EpochNumber, err)
			}
			if pendingTournamentResult(epoch) && (epoch.StagedAtBlock == nil || *epoch.StagedAtBlock <= observedBlock) {
				if err := s.recordTournamentResult(ctx, app, epoch, snapshot); err != nil {
					return nil, false, err
				}
			}
		} else {
			// A bootstrap window can precede consensus deployment. This does
			// not prevent a submitter from checking the latest chain below.
			s.Logger.Debug("DaveConsensus is not deployed at the published tournament block",
				"application", app.Name, "block", observedBlock)
		}
	}
	if !s.submissionEnabled {
		return nil, false, nil
	}

	if !observedSnapshot || latestBlock != observedBlock {
		snapshot, err = readDaveConsensusSnapshot(ctx, consensus, latestBlock)
		if err != nil {
			return nil, false, fmt.Errorf("reading latest DaveConsensus snapshot for app %s: %w", app.IApplicationAddress, err)
		}
		if epoch == nil || epoch.Index != snapshot.sealed.EpochNumber {
			epoch, err = s.repository.GetEpoch(ctx, app.IApplicationAddress.Hex(), snapshot.sealed.EpochNumber)
			if err != nil {
				return nil, false, fmt.Errorf("loading latest sealed epoch %d: %w", snapshot.sealed.EpochNumber, err)
			}
		}
	}
	if !pendingTournamentResult(epoch) {
		s.Logger.Debug("Local epoch is not ready for tournament result processing",
			"application", app.Name,
			"epoch_index", snapshot.sealed.EpochNumber)
		// A failed root has no remaining join or stage action to prioritize.
		return nil, snapshot.stage.IsTournamentFailed, nil
	}
	if err := s.validateResultEpoch(ctx, app, epoch, snapshot.sealed.EpochNumber); err != nil {
		return nil, false, err
	}
	// Latest evidence may prevent this action, but cannot change stored epoch
	// or application status. Re-read it on the next tick after a race or reorg.
	if err := matchConsensusSnapshotToEpoch(epoch, snapshot); err != nil {
		return nil, false, fmt.Errorf("latest tournament result does not match local epoch: %w", err)
	}
	if snapshot.stage.IsTournamentFailed {
		return nil, true, nil
	}
	if snapshot.sealed.IsTournamentResultStaged {
		if !snapshot.accept.DoAllSentriesAgreeWithStagedTournamentResult &&
			!snapshot.accept.IsClaimStagingPeriodOver {
			return nil, true, nil
		}
		return nil, false, s.broadcastAcceptTournamentResult(ctx, app, epoch, consensus, snapshot)
	}

	if epoch.Status == model.EpochStatus_ClaimStaged {
		s.Logger.Info("Local staged epoch is not staged in the pinned DaveConsensus snapshot; waiting for synchronization",
			"application", app.Name,
			"epoch_index", epoch.Index,
			"block", latestBlock)
		return nil, false, nil
	}
	if !snapshot.stage.IsFinished {
		return epoch, false, nil
	}
	return nil, false, s.broadcastStageTournamentResult(ctx, app, epoch, consensus)
}

func pendingTournamentResult(epoch *model.Epoch) bool {
	return epoch != nil && (epoch.Status == model.EpochStatus_ClaimComputed || epoch.Status == model.EpochStatus_ClaimStaged)
}

// recordTournamentResult consumes a snapshot at a published tournament block
// that is no newer than the configured head.
func (s *Service) recordTournamentResult(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	snapshot daveConsensusSnapshot,
) error {
	if err := s.validateResultEpoch(ctx, app, epoch, snapshot.sealed.EpochNumber); err != nil {
		return err
	}
	if epoch.InputIndexLowerBound != snapshot.sealed.InputIndexLowerBound ||
		epoch.InputIndexUpperBound != snapshot.sealed.InputIndexUpperBound {
		return s.setApplicationCorrupted(ctx, app,
			"epoch %d has input bounds inconsistent with DaveConsensus: local=[%d,%d] on-chain=[%d,%d]",
			epoch.Index, epoch.InputIndexLowerBound, epoch.InputIndexUpperBound,
			snapshot.sealed.InputIndexLowerBound, snapshot.sealed.InputIndexUpperBound)
	}
	if err := matchConsensusSnapshotToEpoch(epoch, snapshot); err != nil {
		return s.setApplicationDiverged(ctx, app, "%v", err)
	}
	if snapshot.stage.IsTournamentFailed {
		s.logFailedRootTournament(app, epoch.Index, snapshot.sealed.Tournament)
		return nil
	}
	if !snapshot.sealed.IsTournamentResultStaged {
		return nil
	}
	if epoch.Status == model.EpochStatus_ClaimStaged {
		if epoch.StagedAtBlock == nil || *epoch.StagedAtBlock != snapshot.sealed.StagingBlockNumber {
			return s.setApplicationCorrupted(ctx, app,
				"epoch %d has staging block inconsistent with DaveConsensus", epoch.Index)
		}
		return nil
	}
	if err := s.repository.UpdateEpochReconciledStaged(ctx, app.ID, epoch.Index, snapshot.sealed.StagingBlockNumber); err != nil {
		return fmt.Errorf("recording staged tournament result for epoch %d: %w", epoch.Index, err)
	}
	epoch.Status = model.EpochStatus_ClaimStaged
	epoch.StagedAtBlock = new(snapshot.sealed.StagingBlockNumber)
	s.Logger.Info("Recorded staged tournament result",
		"application", app.Name, "epoch_index", epoch.Index, "staging_block", snapshot.sealed.StagingBlockNumber)
	return nil
}

func (s *Service) validateResultEpoch(ctx context.Context, app *model.Application, epoch *model.Epoch, index uint64) error {
	if epoch.TournamentAddress == nil || epoch.Commitment == nil || epoch.MachineHash == nil || epoch.TxBufferDataBlock == nil {
		return s.setApplicationCorrupted(ctx, app,
			"epoch %d has missing required fields for tournament result processing", epoch.Index)
	}
	if epoch.Index != index {
		return s.setApplicationCorrupted(ctx, app,
			"loaded epoch %d for current sealed epoch %d", epoch.Index, index)
	}
	return nil
}

func matchConsensusSnapshotToEpoch(epoch *model.Epoch, snapshot daveConsensusSnapshot) error {
	if epoch.InputIndexLowerBound != snapshot.sealed.InputIndexLowerBound ||
		epoch.InputIndexUpperBound != snapshot.sealed.InputIndexUpperBound {
		return fmt.Errorf(
			"epoch %d has input bounds inconsistent with DaveConsensus: local=[%d,%d] on-chain=[%d,%d]",
			epoch.Index,
			epoch.InputIndexLowerBound,
			epoch.InputIndexUpperBound,
			snapshot.sealed.InputIndexLowerBound,
			snapshot.sealed.InputIndexUpperBound)
	}
	if *epoch.TournamentAddress != snapshot.sealed.Tournament {
		return fmt.Errorf(
			"epoch %d has inconsistent root tournament between off-chain (%s) and on-chain (%s)",
			epoch.Index, epoch.TournamentAddress, snapshot.sealed.Tournament)
	}
	if snapshot.stage.IsTournamentFailed {
		return nil
	}
	if !snapshot.stage.IsFinished {
		return nil
	}
	if *epoch.Commitment != snapshot.stage.WinnerCommitment {
		return fmt.Errorf(
			"epoch %d has inconsistent commitment between off-chain (%s) and on-chain (%s)",
			epoch.Index, epoch.Commitment, snapshot.stage.WinnerCommitment)
	}
	if *epoch.MachineHash != snapshot.stage.WinnerPostEpochMachineStateHash {
		return fmt.Errorf(
			"epoch %d has inconsistent machine hash between off-chain (%s) and on-chain (%s)",
			epoch.Index, epoch.MachineHash, snapshot.stage.WinnerPostEpochMachineStateHash)
	}
	if snapshot.sealed.IsTournamentResultStaged {
		if *epoch.MachineHash != snapshot.sealed.StagedPostEpochMachineStateHash {
			return fmt.Errorf(
				"epoch %d has inconsistent staged machine hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.MachineHash, snapshot.sealed.StagedPostEpochMachineStateHash)
		}
		if *epoch.TxBufferDataBlock != snapshot.sealed.StagedPostEpochOutputsMerkleRoot {
			return fmt.Errorf(
				"epoch %d has inconsistent staged outputs root between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.TxBufferDataBlock, snapshot.sealed.StagedPostEpochOutputsMerkleRoot)
		}
	}
	return nil
}

func (s *Service) broadcastStageTournamentResult(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	consensus DaveConsensusAdapter,
) error {
	proof, err := epoch.StateProof()
	if err != nil {
		return s.setApplicationCorrupted(ctx, app,
			"cannot stage tournament result for epoch %d: persisted machine state proof is incomplete: %v",
			epoch.Index, err)
	}
	txCtx, cancel := context.WithTimeout(ctx, s.submissionTimeout)
	defer cancel()
	txOpts, err := s.txOptsFactory.NewTransactOpts(txCtx)
	if err != nil {
		return fmt.Errorf("creating transaction options to stage epoch %d: %w", epoch.Index, err)
	}
	tx, err := consensus.StageTournamentResult(txOpts, epoch.Index, proof)
	if err != nil {
		return s.handleStageTournamentResultRevert(ctx, app, epoch, err)
	}
	if tx == nil {
		return errors.New("stage tournament result returned a nil transaction")
	}
	txHash := tx.Hash()
	s.pendingTransactions[app.ID] = pendingTournamentTransaction{
		Action: tournamentActionStage, Hash: txHash, EpochIndex: epoch.Index,
	}
	s.Logger.Info("Sent tournament result stage transaction",
		"application", app.Name,
		"epoch_index", epoch.Index,
		"tx", txHash)
	return nil
}

func (s *Service) broadcastAcceptTournamentResult(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	consensus DaveConsensusAdapter,
	snapshot daveConsensusSnapshot,
) error {
	if epoch.TournamentAddress == nil || *epoch.TournamentAddress != snapshot.sealed.Tournament {
		return s.setApplicationCorrupted(ctx, app,
			"epoch %d has no validated root tournament for result acceptance", epoch.Index)
	}
	txCtx, cancel := context.WithTimeout(ctx, s.submissionTimeout)
	defer cancel()
	txOpts, err := s.txOptsFactory.NewTransactOpts(txCtx)
	if err != nil {
		return fmt.Errorf("creating transaction options to accept epoch %d: %w", epoch.Index, err)
	}
	s.queueRootBondRecovery(app.ID, epoch.Index, snapshot.sealed.Tournament)
	tx, err := consensus.AcceptStagedTournamentResult(txOpts, epoch.Index)
	if err != nil {
		return s.handleAcceptTournamentResultRevert(ctx, app, epoch, err)
	}
	if tx == nil {
		return errors.New("accept staged tournament result returned a nil transaction")
	}
	txHash := tx.Hash()
	s.pendingTransactions[app.ID] = pendingTournamentTransaction{
		Action: tournamentActionAccept, Hash: txHash, EpochIndex: epoch.Index,
	}
	s.Logger.Info("Sent staged tournament result acceptance transaction",
		"application", app.Name,
		"epoch_index", epoch.Index,
		"tournament", snapshot.sealed.Tournament,
		"tx", txHash)
	return nil
}

func (s *Service) handleStageTournamentResultRevert(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	err error,
) error {
	if name := stageMachineValidationRevertName(err); name != "" {
		if name == "InvalidMachineMerkleProof" {
			// The contract checks this proof against its current winner, which
			// may differ from the latest precheck after a reorg or RPC race.
			// Only diagnose the proof once its target is confirmed.
			if confirmErr := s.confirmStageProofTarget(ctx, app, epoch); confirmErr != nil {
				return fmt.Errorf("stage proof rejected before its target could be confirmed: %w", confirmErr)
			}
		}
		guidance := "The node validated the proof when it collected it. " +
			"Check proof serialization, stored proof data, and node and contract versions before re-enabling."
		if name == "InvalidPostEpochMachineIflagsYRegister" || name == "InvalidPostEpochMachineHtifTohostRegister" {
			guidance = "The proven post-epoch machine state cannot finalize. " +
				"Check the selected post-epoch state, stored data, and node and contract versions. " +
				"If this state is unrecoverable, consider guardian foreclosure to permit emergency withdrawals and deposit refunds."
		}
		return s.setApplicationFailed(ctx, app,
			"StageTournamentResult rejected the machine state proof with %s for epoch %d. %s",
			name, epoch.Index, guidance)
	}
	switch {
	case isTournamentError(err, "TournamentFailedNoWinner"):
		s.Logger.Warn("Stage transaction observed a failed tournament; waiting for configured-block confirmation",
			"application", app.Name, "epoch_index", epoch.Index)
		return nil
	case isDaveConsensusError(err, "TournamentResultAlreadyStaged"):
		s.Logger.Info("Tournament result was already staged; waiting for a fresh chain snapshot",
			"application", app.Name, "epoch_index", epoch.Index)
		return nil
	case isDaveConsensusError(err, "TournamentNotFinishedYet"):
		s.Logger.Info("Tournament result is not ready to stage; waiting for a fresh chain snapshot",
			"application", app.Name, "epoch_index", epoch.Index)
		return nil
	case isDaveConsensusError(err, "IncorrectEpochNumber"):
		return s.handleResultEpochRace(app, "StageTournamentResult", epoch.Index, err)
	case isDaveConsensusError(err, "ApplicationForeclosed"):
		s.Logger.Warn("StageTournamentResult observed application foreclosure; waiting for the foreclosure observer",
			"application", app.Name, "epoch_index", epoch.Index)
		return nil
	case isDaveApplicationFailure(err):
		return s.failDaveApplicationCall(ctx, app, "StageTournamentResult", epoch.Index, err)
	case ethutil.IsNonceTooLowError(err):
		s.Logger.Info("StageTournamentResult broadcast was rejected with nonce too low; waiting for chain reconciliation",
			"application", app.Name, "epoch_index", epoch.Index)
		return nil
	default:
		s.logErrorUnlessShutdown(ctx, "failed to stage tournament result", err,
			"application", app.Name,
			"epoch_index", epoch.Index,
			"decoded_revert", describeKnownRevert(err))
		return err
	}
}

func (s *Service) confirmStageProofTarget(ctx context.Context, app *model.Application, epoch *model.Epoch) error {
	block, err := s.getDefaultBlockNumber(ctx)
	if err != nil {
		return err
	}
	consensus, err := s.adapterFactory.CreateDaveConsensusAdapter(app.IConsensusAddress)
	if err != nil {
		return err
	}
	snapshot, err := readDaveConsensusSnapshot(ctx, consensus, block)
	if err != nil {
		return err
	}
	if snapshot.sealed.EpochNumber != epoch.Index || !snapshot.stage.IsFinished || snapshot.stage.IsTournamentFailed {
		return fmt.Errorf("epoch %d winner is not confirmed at configured block %d", epoch.Index, block)
	}
	if err := s.validateResultEpoch(ctx, app, epoch, snapshot.sealed.EpochNumber); err != nil {
		return err
	}
	return matchConsensusSnapshotToEpoch(epoch, snapshot)
}

func stageMachineValidationRevertName(err error) string {
	for _, name := range []string{
		"InvalidSiblingsArrayLength",
		"InvalidMachineMerkleProof",
		"InvalidPostEpochMachineIflagsYRegister",
		"InvalidPostEpochMachineHtifTohostRegister",
	} {
		if isDaveConsensusError(err, name) {
			return name
		}
	}
	return ""
}

func (s *Service) handleAcceptTournamentResultRevert(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	err error,
) error {
	switch {
	case isDaveConsensusError(err, "TournamentResultNotStaged"):
		s.Logger.Info("Tournament result is no longer staged; waiting for a fresh chain snapshot",
			"application", app.Name, "epoch_index", epoch.Index)
		return nil
	case isDaveConsensusError(err, "ClaimStagingPeriodNotOverYet"):
		s.Logger.Info("Tournament result is not ready to accept; waiting for a fresh chain snapshot",
			"application", app.Name, "epoch_index", epoch.Index)
		return nil
	case isDaveConsensusError(err, "IncorrectEpochNumber"):
		return s.handleResultEpochRace(app, "AcceptStagedTournamentResult", epoch.Index, err)
	case isDaveConsensusError(err, "ApplicationForeclosed"):
		s.Logger.Warn("AcceptStagedTournamentResult observed application foreclosure; waiting for the foreclosure observer",
			"application", app.Name, "epoch_index", epoch.Index)
		return nil
	case isDaveApplicationFailure(err):
		return s.failDaveApplicationCall(ctx, app, "AcceptStagedTournamentResult", epoch.Index, err)
	case ethutil.IsNonceTooLowError(err):
		s.Logger.Info("AcceptStagedTournamentResult broadcast was rejected with nonce too low; waiting for chain reconciliation",
			"application", app.Name, "epoch_index", epoch.Index)
		return nil
	default:
		s.logErrorUnlessShutdown(ctx, "failed to accept staged tournament result", err,
			"application", app.Name,
			"epoch_index", epoch.Index,
			"decoded_revert", describeKnownRevert(err))
		return err
	}
}

func (s *Service) handleResultEpochRace(
	app *model.Application,
	action string,
	epochNumber uint64,
	err error,
) error {
	received, actual, ok := decodeIncorrectEpochNumber(err)
	if !ok {
		return err
	}
	if received.Cmp(actual) > 0 {
		// Simulation may use a lagging replica or an unfinalized fork. It is
		// not evidence that the local epoch is invalid.
		return err
	}
	s.Logger.Info("Tournament result transaction used a stale epoch; waiting for chain synchronization",
		"application", app.Name,
		"action", action,
		"epoch_index", epochNumber,
		"chain_epoch", actual)
	return nil
}

func isDaveApplicationFailure(err error) bool {
	return isDaveConsensusError(err, "ApplicationNotDeployed") ||
		isDaveConsensusError(err, "ApplicationReverted") ||
		isDaveConsensusError(err, "IllformedApplicationReturnData")
}

func (s *Service) failDaveApplicationCall(
	ctx context.Context,
	app *model.Application,
	action string,
	epochNumber uint64,
	err error,
) error {
	for _, name := range []string{"ApplicationNotDeployed", "ApplicationReverted", "IllformedApplicationReturnData"} {
		if !isDaveConsensusError(err, name) {
			continue
		}
		return s.setApplicationFailed(ctx, app,
			"%s reverted with %s for epoch %d. Verify the application and consensus contracts before re-enabling.%s",
			action, name, epochNumber, ethutil.ApplicationReturnDataSuffix(err, idaveconsensus.IDaveConsensusMetaData, name))
	}
	return err
}
