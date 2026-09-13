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
