// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

func nullableAddressBytes(value *common.Address) any {
	if value == nil {
		return nil
	}
	return value.Bytes()
}

var tournamentColumns = postgres.ColumnList{
	table.Tournaments.ApplicationID,
	table.Tournaments.EpochIndex,
	table.Tournaments.Address,
	table.Tournaments.ParentTournamentAddress,
	table.Tournaments.ParentMatchIDHash,
	table.Tournaments.MaxLevel,
	table.Tournaments.Level,
	table.Tournaments.Log2step,
	table.Tournaments.Height,
	table.Tournaments.InitialHash,
	table.Tournaments.BaseCycle,
	table.Tournaments.Kind,
	table.Tournaments.StartInstant,
	table.Tournaments.Allowance,
	table.Tournaments.CreationBlockNumber,
	table.Tournaments.CreationTxHash,
	table.Tournaments.CreationLogIndex,
	table.Tournaments.AsOfBlock,
	table.Tournaments.Standing,
	table.Tournaments.AcceptsJoins,
	table.Tournaments.Candidate,
	table.Tournaments.WinnerCommitment,
	table.Tournaments.FinalStateHash,
	table.Tournaments.FinishedAtBlock,
	table.Tournaments.ParentCommitment,
	table.Tournaments.WinnerExpiresAt,
	table.Tournaments.InnerDisposition,
	table.Tournaments.InnerParentCommitment,
	table.Tournaments.InnerPausedAllowance,
	table.Tournaments.BondDisposition,
	table.Tournaments.BondClaimer,
	table.Tournaments.BondPayment,
}

func tournamentValues(v *Tournament) []any {
	var creationBlock, creationTx, creationLog any
	if v.CreationEvent != nil {
		creationBlock = uint64Expr(v.CreationEvent.BlockNumber)
		creationTx = v.CreationEvent.TxHash.Bytes()
		creationLog = uint64Expr(v.CreationEvent.LogIndex)
	}
	var innerDisposition, innerParent, innerAllowance any
	if v.Snapshot.InnerResult != nil {
		innerDisposition = v.Snapshot.InnerResult.Disposition
		innerParent = hashToBytes(v.Snapshot.InnerResult.ParentCommitment)
		innerAllowance = uint64Expr(v.Snapshot.InnerResult.PausedAllowance)
	}
	var bondPayment any
	if v.Snapshot.BondRecovery.Payment != nil {
		bondPayment = *v.Snapshot.BondRecovery.Payment
	}
	return []any{
		v.ApplicationID,
		uint64Expr(v.EpochIndex),
		v.Address.Bytes(),
		nullableAddressBytes(v.ParentTournamentAddress),
		hashToBytes(v.ParentMatchIDHash),
		uint64Expr(v.MaxLevel),
		uint64Expr(v.Level),
		uint64Expr(v.Log2Step),
		uint64Expr(v.Height),
		v.InitialHash.Bytes(),
		v.BaseCycle,
		v.Kind,
		uint64Expr(v.StartInstant),
		uint64Expr(v.Allowance),
		creationBlock,
		creationTx,
		creationLog,
		uint64Expr(v.Snapshot.AsOfBlock),
		v.Snapshot.Standing,
		v.Snapshot.AcceptsJoins,
		hashToBytes(v.Snapshot.Candidate),
		hashToBytes(v.Snapshot.WinnerCommitment),
		hashToBytes(v.Snapshot.FinalStateHash),
		uint64Expr(v.Snapshot.FinishedAtBlock),
		hashToBytes(v.Snapshot.ParentCommitment),
		uint64Expr(v.Snapshot.WinnerExpiresAt),
		innerDisposition,
		innerParent,
		innerAllowance,
		v.Snapshot.BondRecovery.Disposition,
		nullableAddressBytes(v.Snapshot.BondRecovery.Claimer),
		bondPayment,
	}
}

func scanTournament(row pgx.Row) (*Tournament, error) {
	var v Tournament
	var creationBlock, creationLog *uint64
	var creationTx *common.Hash
	var innerDisposition *InnerTournamentDisposition
	var innerParent *common.Hash
	var innerAllowance *uint64
	if err := row.Scan(
		&v.ApplicationID,
		&v.EpochIndex,
		&v.Address,
		&v.ParentTournamentAddress,
		&v.ParentMatchIDHash,
		&v.MaxLevel,
		&v.Level,
		&v.Log2Step,
		&v.Height,
		&v.InitialHash,
		&v.BaseCycle,
		&v.Kind,
		&v.StartInstant,
		&v.Allowance,
		&creationBlock,
		&creationTx,
		&creationLog,
		&v.Snapshot.AsOfBlock,
		&v.Snapshot.Standing,
		&v.Snapshot.AcceptsJoins,
		&v.Snapshot.Candidate,
		&v.Snapshot.WinnerCommitment,
		&v.Snapshot.FinalStateHash,
		&v.Snapshot.FinishedAtBlock,
		&v.Snapshot.ParentCommitment,
		&v.Snapshot.WinnerExpiresAt,
		&innerDisposition,
		&innerParent,
		&innerAllowance,
		&v.Snapshot.BondRecovery.Disposition,
		&v.Snapshot.BondRecovery.Claimer,
		&v.Snapshot.BondRecovery.Payment,
		&v.CreatedAt, &v.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if creationBlock != nil {
		v.CreationEvent = &TournamentCreationEvent{BlockNumber: *creationBlock, TxHash: *creationTx, LogIndex: *creationLog}
	}
	if innerDisposition != nil {
		v.Snapshot.InnerResult = &TournamentInnerResult{
			Disposition: *innerDisposition, ParentCommitment: innerParent, PausedAllowance: *innerAllowance,
		}
	}
	return &v, nil
}

var commitmentColumns = postgres.ColumnList{
	table.Commitments.ApplicationID,
	table.Commitments.EpochIndex,
	table.Commitments.TournamentAddress,
	table.Commitments.Commitment,
	table.Commitments.FinalStateHash,
	table.Commitments.SubmitterAddress,
	table.Commitments.BlockNumber,
	table.Commitments.TxHash,
	table.Commitments.LogIndex,
	table.Commitments.AsOfBlock,
	table.Commitments.Claimer,
	table.Commitments.ClockRunning,
	table.Commitments.ClockDeadline,
	table.Commitments.ClockAllowance,
}

func commitmentValues(v *Commitment) []any {
	return []any{
		v.ApplicationID,
		uint64Expr(v.EpochIndex),
		v.TournamentAddress.Bytes(),
		v.Commitment.Bytes(),
		v.FinalStateHash.Bytes(),
		v.SubmitterAddress.Bytes(),
		uint64Expr(v.BlockNumber),
		v.TxHash.Bytes(),
		uint64Expr(v.LogIndex),
		uint64Expr(v.Snapshot.AsOfBlock),
		v.Snapshot.Claimer.Bytes(),
		v.Snapshot.ClockRunning,
		uint64Expr(v.Snapshot.ClockDeadline),
		uint64Expr(v.Snapshot.ClockAllowance),
	}
}

func scanCommitment(row pgx.Row) (*Commitment, error) {
	var v Commitment

	if err := row.Scan(
		&v.ApplicationID,
		&v.EpochIndex,
		&v.TournamentAddress,
		&v.Commitment,
		&v.FinalStateHash,
		&v.SubmitterAddress,
		&v.BlockNumber,
		&v.TxHash,
		&v.LogIndex,
		&v.Snapshot.AsOfBlock,
		&v.Snapshot.Claimer,
		&v.Snapshot.ClockRunning,
		&v.Snapshot.ClockDeadline,
		&v.Snapshot.ClockAllowance,
		&v.CreatedAt, &v.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return &v, nil
}

var matchColumns = postgres.ColumnList{
	table.Matches.ApplicationID,
	table.Matches.EpochIndex,
	table.Matches.TournamentAddress,
	table.Matches.IDHash,
	table.Matches.CommitmentOne,
	table.Matches.CommitmentTwo,
	table.Matches.LeftOfTwo,
	table.Matches.BlockNumber,
	table.Matches.TxHash,
	table.Matches.LogIndex,
	table.Matches.EliminableAt,
	table.Matches.SealEliminableAt,
	table.Matches.SealBlockNumber,
	table.Matches.SealTxHash,
	table.Matches.SealLogIndex,
	table.Matches.AsOfBlock,
	table.Matches.Phase,
	table.Matches.TimeoutOutcome,
	table.Matches.DeferredCharge,
	table.Matches.RevealingParent,
	table.Matches.WaitingLeft,
	table.Matches.WaitingRight,
	table.Matches.SegmentStartPosition,
	table.Matches.SegmentStartCycle,
	table.Matches.CurrentHeight,
	table.Matches.Responder,
	table.Matches.AgreeState,
	table.Matches.DivergencePosition,
	table.Matches.DivergenceCycle,
	table.Matches.FinalStateOne,
	table.Matches.FinalStateTwo,
	table.Matches.Winner,
	table.Matches.DeletionReason,
	table.Matches.DeletionBlockNumber,
	table.Matches.DeletionTxHash,
	table.Matches.DeletionLogIndex,
}

func matchValues(v *Match) []any {
	var sealEliminable, sealBlock, sealTx, sealLog any
	if v.LeafSeal != nil {
		sealEliminable = uint64Expr(v.LeafSeal.EliminableAt)
		sealBlock = uint64Expr(v.LeafSeal.BlockNumber)
		sealTx = v.LeafSeal.TxHash.Bytes()
		sealLog = uint64Expr(v.LeafSeal.LogIndex)
	}
	var revealingParent, waitingLeft, waitingRight, segmentPosition, segmentCycle, currentHeight, responder any
	if s := v.Snapshot.Bisection; s != nil {
		revealingParent, waitingLeft, waitingRight = s.RevealingParent.Bytes(), s.WaitingLeft.Bytes(), s.WaitingRight.Bytes()
		segmentPosition, segmentCycle, responder = s.SegmentStartPosition, s.SegmentStartCycle, s.Responder
		if s.CurrentHeight != nil {
			currentHeight = uint64Expr(*s.CurrentHeight)
		}
	}
	var agreeState, divergencePosition, divergenceCycle, finalStateOne, finalStateTwo any
	if s := v.Snapshot.Sealed; s != nil {
		agreeState, finalStateOne, finalStateTwo = s.AgreeState.Bytes(), s.FinalStateOne.Bytes(), s.FinalStateTwo.Bytes()
		divergencePosition, divergenceCycle = s.DivergencePosition, s.DivergenceCycle
	}
	var deletionLog any
	if v.DeletionLogIndex != nil {
		deletionLog = uint64Expr(*v.DeletionLogIndex)
	}
	return []any{
		v.ApplicationID,
		uint64Expr(v.EpochIndex),
		v.TournamentAddress.Bytes(),
		v.IDHash.Bytes(),
		v.CommitmentOne.Bytes(),
		v.CommitmentTwo.Bytes(),
		v.LeftOfTwo.Bytes(),
		uint64Expr(v.BlockNumber),
		v.TxHash.Bytes(),
		uint64Expr(v.LogIndex),
		uint64Expr(v.EliminableAt),
		sealEliminable,
		sealBlock,
		sealTx,
		sealLog,
		uint64Expr(v.Snapshot.AsOfBlock),
		v.Snapshot.Phase,
		v.Snapshot.TimeoutOutcome,
		uint64Expr(v.Snapshot.DeferredCharge),
		revealingParent,
		waitingLeft,
		waitingRight,
		segmentPosition,
		segmentCycle,
		currentHeight,
		responder,
		agreeState,
		divergencePosition,
		divergenceCycle,
		finalStateOne,
		finalStateTwo,
		v.Winner,
		v.DeletionReason,
		uint64Expr(v.DeletionBlockNumber),
		hashToBytes(v.DeletionTxHash),
		deletionLog,
	}
}

func scanMatch(row pgx.Row) (*Match, error) {
	var v Match
	var sealEliminable, sealBlock, sealLog *uint64
	var sealTx *common.Hash
	var revealingParent, waitingLeft, waitingRight *common.Hash
	var segmentPosition, segmentCycle *Uint256
	var currentHeight *uint64
	var responder *CommitmentSide
	var agreeState, finalStateOne, finalStateTwo *common.Hash
	var divergencePosition, divergenceCycle *Uint256
	if err := row.Scan(
		&v.ApplicationID,
		&v.EpochIndex,
		&v.TournamentAddress,
		&v.IDHash,
		&v.CommitmentOne,
		&v.CommitmentTwo,
		&v.LeftOfTwo,
		&v.BlockNumber,
		&v.TxHash,
		&v.LogIndex,
		&v.EliminableAt,
		&sealEliminable,
		&sealBlock,
		&sealTx,
		&sealLog,
		&v.Snapshot.AsOfBlock,
		&v.Snapshot.Phase,
		&v.Snapshot.TimeoutOutcome,
		&v.Snapshot.DeferredCharge,
		&revealingParent,
		&waitingLeft,
		&waitingRight,
		&segmentPosition,
		&segmentCycle,
		&currentHeight,
		&responder,
		&agreeState,
		&divergencePosition,
		&divergenceCycle,
		&finalStateOne,
		&finalStateTwo,
		&v.Winner,
		&v.DeletionReason,
		&v.DeletionBlockNumber,
		&v.DeletionTxHash,
		&v.DeletionLogIndex,
		&v.CreatedAt, &v.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if sealBlock != nil {
		v.LeafSeal = &LeafMatchSeal{EliminableAt: *sealEliminable, BlockNumber: *sealBlock, TxHash: *sealTx, LogIndex: *sealLog}
	}
	if revealingParent != nil {
		v.Snapshot.Bisection = &MatchBisectionSnapshot{
			RevealingParent: *revealingParent, WaitingLeft: *waitingLeft, WaitingRight: *waitingRight,
			SegmentStartPosition: *segmentPosition, SegmentStartCycle: *segmentCycle, CurrentHeight: currentHeight, Responder: *responder,
		}
	}
	if agreeState != nil {
		v.Snapshot.Sealed = &MatchSealedSnapshot{
			AgreeState: *agreeState, DivergencePosition: *divergencePosition, DivergenceCycle: *divergenceCycle,
			FinalStateOne: *finalStateOne, FinalStateTwo: *finalStateTwo,
		}
	}
	return &v, nil
}

var matchAdvanceColumns = postgres.ColumnList{
	table.MatchAdvances.ApplicationID,
	table.MatchAdvances.EpochIndex,
	table.MatchAdvances.TournamentAddress,
	table.MatchAdvances.IDHash,
	table.MatchAdvances.OtherParent,
	table.MatchAdvances.LeftNode,
	table.MatchAdvances.SegmentStartPosition,
	table.MatchAdvances.EliminableAt,
	table.MatchAdvances.BlockNumber,
	table.MatchAdvances.TxHash,
	table.MatchAdvances.LogIndex,
}

func matchAdvanceValues(v *MatchAdvanced) []any {

	return []any{
		v.ApplicationID,
		uint64Expr(v.EpochIndex),
		v.TournamentAddress.Bytes(),
		v.IDHash.Bytes(),
		v.OtherParent.Bytes(),
		v.LeftNode.Bytes(),
		v.SegmentStartPosition,
		uint64Expr(v.EliminableAt),
		uint64Expr(v.BlockNumber),
		v.TxHash.Bytes(),
		uint64Expr(v.LogIndex),
	}
}

func scanMatchAdvanced(row pgx.Row) (*MatchAdvanced, error) {
	var v MatchAdvanced

	if err := row.Scan(
		&v.ApplicationID,
		&v.EpochIndex,
		&v.TournamentAddress,
		&v.IDHash,
		&v.OtherParent,
		&v.LeftNode,
		&v.SegmentStartPosition,
		&v.EliminableAt,
		&v.BlockNumber,
		&v.TxHash,
		&v.LogIndex,
		&v.CreatedAt, &v.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return &v, nil
}

var bondEventColumns = postgres.ColumnList{
	table.BondEvents.ApplicationID,
	table.BondEvents.EpochIndex,
	table.BondEvents.TournamentAddress,
	table.BondEvents.Type,
	table.BondEvents.BlockNumber,
	table.BondEvents.TxHash,
	table.BondEvents.LogIndex,
	table.BondEvents.Recipient,
	table.BondEvents.Value,
	table.BondEvents.Success,
	table.BondEvents.Commitment,
	table.BondEvents.Claimer,
	table.BondEvents.Payment,
	table.BondEvents.Burned,
}

func bondEventValues(v *BondEvent) []any {
	var recipient, value, success, commitment, claimer, payment, burned any
	if v.Refund != nil {
		recipient, value, success = v.Refund.Recipient.Bytes(), v.Refund.Value, v.Refund.Success
	}
	if v.Recovery != nil {
		commitment, claimer = v.Recovery.Commitment.Bytes(), v.Recovery.Claimer.Bytes()
		payment, burned = v.Recovery.Payment, v.Recovery.Burned
	}
	return []any{
		v.ApplicationID,
		uint64Expr(v.EpochIndex),
		v.TournamentAddress.Bytes(),
		v.Type,
		uint64Expr(v.BlockNumber),
		v.TxHash.Bytes(),
		uint64Expr(v.LogIndex),
		recipient,
		value,
		success,
		commitment,
		claimer,
		payment,
		burned,
	}
}

func scanBondEvent(row pgx.Row) (*BondEvent, error) {
	var v BondEvent
	var recipient, claimer *common.Address
	var value, payment, burned *Uint256
	var success *bool
	var commitment *common.Hash
	if err := row.Scan(
		&v.ApplicationID,
		&v.EpochIndex,
		&v.TournamentAddress,
		&v.Type,
		&v.BlockNumber,
		&v.TxHash,
		&v.LogIndex,
		&recipient,
		&value,
		&success,
		&commitment,
		&claimer,
		&payment,
		&burned,
		&v.CreatedAt, &v.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if recipient != nil {
		v.Refund = &PartialBondRefund{Recipient: *recipient, Value: *value, Success: *success}
	}
	if commitment != nil {
		v.Recovery = &BondRecovered{Commitment: *commitment, Claimer: *claimer, Payment: *payment, Burned: *burned}
	}
	return &v, nil
}
