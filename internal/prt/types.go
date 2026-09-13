// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
)

// TournamentDescriptor is the immutable tournament configuration exposed by
// the Dave tournament contract.
type TournamentDescriptor struct {
	InitialHash  common.Hash
	BaseCycle    *big.Int
	Log2Stride   uint64
	Height       uint64
	Level        uint64
	Kind         model.TournamentKind
	StartInstant uint64
	Allowance    uint64
}

// TournamentStanding is the current tournament result and timing projection.
type TournamentStanding struct {
	State            model.TournamentStandingState
	AcceptsJoins     bool
	HasCandidate     bool
	Candidate        common.Hash
	FinalState       common.Hash
	ParentCommitment common.Hash
	FinishedAt       uint64
	WinnerExpiresAt  uint64
}

// CommitmentStanding is the current on-chain record for one commitment.
type CommitmentStanding struct {
	Joined         bool
	FinalState     common.Hash
	Claimer        common.Address
	ClockRunning   bool
	ClockDeadline  uint64
	ClockAllowance uint64
}

// BisectingMatch is the current divergence frontier before the final opening.
type BisectingMatch struct {
	RevealingParent      common.Hash
	WaitingLeft          common.Hash
	WaitingRight         common.Hash
	SegmentStartPosition *big.Int
	SegmentStartCycle    *big.Int
	CurrentHeight        uint64
	Responder            model.CommitmentSide
}

// ReadyToSealMatch is the final divergence frontier, before sealing.
type ReadyToSealMatch struct {
	RevealingParent      common.Hash
	WaitingLeft          common.Hash
	WaitingRight         common.Hash
	SegmentStartPosition *big.Int
	SegmentStartCycle    *big.Int
	Responder            model.CommitmentSide
}

// SealedMatch stores states in commitment-side order, not reveal order.
type SealedMatch struct {
	AgreeState         common.Hash
	DivergencePosition *big.Int
	DivergenceCycle    *big.Int
	FinalStateOne      common.Hash
	FinalStateTwo      common.Hash
}

// ObservedMatchSnapshot contains only the payload selected by Phase. Deleted matches
// have phase UNINITIALIZED and no payload; their events remain historical data.
type ObservedMatchSnapshot struct {
	Phase          model.MatchPhase
	TimeoutOutcome model.MatchTimeoutOutcome
	DeferredCharge uint64
	Bisecting      *BisectingMatch
	ReadyToSeal    *ReadyToSealMatch
	Sealed         *SealedMatch
}

// InnerResult includes the carryover allowance at the observation block.
type InnerResult struct {
	Disposition      model.InnerTournamentDisposition
	ParentCommitment common.Hash
	PausedAllowance  uint64
}

// StructuralEventCounts contains six independent on-chain counters. Financial
// events have no dedicated counter in this contract version.
type StructuralEventCounts struct {
	CommitmentJoined   *big.Int
	MatchCreated       *big.Int
	MatchAdvanced      *big.Int
	LeafMatchSealed    *big.Int
	MatchDeleted       *big.Int
	NewInnerTournament *big.Int
}

// BondRecovery is the current recovery state of a tournament bond.
type BondRecovery struct {
	Disposition model.BondDisposition
	Claimer     common.Address
	Payment     *big.Int
}

// TournamentAdapter provides read and write access to tournament contracts.
type TournamentAdapter interface {
	RetrieveAllEvents(opts *bind.FilterOpts) (*TournamentEvents, error)
	Descriptor(opts *bind.CallOpts) (TournamentDescriptor, error)
	Standing(opts *bind.CallOpts) (TournamentStanding, error)
	CommitmentStanding(opts *bind.CallOpts, commitmentRoot [32]byte) (CommitmentStanding, error)
	MatchSnapshot(opts *bind.CallOpts, one, two [32]byte) (ObservedMatchSnapshot, error)
	InnerResult(opts *bind.CallOpts) (InnerResult, error)
	StructuralEventCounts(opts *bind.CallOpts) (StructuralEventCounts, error)
	BondValue(opts *bind.CallOpts) (*big.Int, error)
	BondRecovery(opts *bind.CallOpts) (BondRecovery, error)
	JoinTournament(opts *bind.TransactOpts, finalState [32]byte, proof [][32]byte,
		leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error)
	TryRecoveringBond(opts *bind.TransactOpts) (*types.Transaction, error)
}

// DaveConsensusAdapter wraps access to the IDaveConsensus contract.
type DaveConsensusAdapter interface {
	ParseEpochSealed(log types.Log) (*idaveconsensus.IDaveConsensusEpochSealed, error)
	TournamentLevelCount(opts *bind.CallOpts) (uint64, error)
	GetCurrentSealedEpoch(opts *bind.CallOpts) (CurrentSealedEpoch, error)
	CanStageTournamentResult(opts *bind.CallOpts) (CanStageTournamentResult, error)
	CanAcceptStagedTournamentResult(opts *bind.CallOpts) (CanAcceptStagedTournamentResult, error)
	StageTournamentResult(opts *bind.TransactOpts, epochNumber uint64,
		proof model.StateProof) (*types.Transaction, error)
	AcceptStagedTournamentResult(opts *bind.TransactOpts, epochNumber uint64) (*types.Transaction, error)
}

// CurrentSealedEpoch is the complete state of the current sealed epoch.
type CurrentSealedEpoch struct {
	EpochNumber                      uint64
	InputIndexLowerBound             uint64
	InputIndexUpperBound             uint64
	Tournament                       common.Address
	IsTournamentResultStaged         bool
	StagingBlockNumber               uint64
	StagedPostEpochMachineStateHash  common.Hash
	StagedPostEpochOutputsMerkleRoot common.Hash
}

// CanStageTournamentResult is the stage readiness view for the current sealed epoch.
type CanStageTournamentResult struct {
	IsFinished                      bool
	IsTournamentFailed              bool
	IsTournamentResultStaged        bool
	EpochNumber                     uint64
	WinnerCommitment                common.Hash
	WinnerPostEpochMachineStateHash common.Hash
}

// CanAcceptStagedTournamentResult is the accept readiness view for the current sealed epoch.
type CanAcceptStagedTournamentResult struct {
	IsTournamentResultStaged                     bool
	DoAllSentriesAgreeWithStagedTournamentResult bool
	IsClaimStagingPeriodOver                     bool
	EpochNumber                                  uint64
	StagedPostEpochMachineStateHash              common.Hash
	StagedPostEpochOutputsMerkleRoot             common.Hash
}

// AdapterFactory creates contract adapters from on-chain addresses.
type AdapterFactory interface {
	CreateTournamentAdapter(addr common.Address) (TournamentAdapter, error)
	CreateDaveConsensusAdapter(addr common.Address) (DaveConsensusAdapter, error)
}

// Struct to hold all events retrieved at once
type TournamentEvents struct {
	CommitmentJoined   []*itournament.ITournamentCommitmentJoined
	MatchAdvanced      []*itournament.ITournamentMatchAdvanced
	MatchCreated       []*itournament.ITournamentMatchCreated
	MatchDeleted       []*itournament.ITournamentMatchDeleted
	NewInnerTournament []*itournament.ITournamentNewInnerTournament
	LeafMatchSealed    []*itournament.ITournamentLeafMatchSealed
	PartialBondRefund  []*itournament.ITournamentPartialBondRefund
	BondRecovered      []*itournament.ITournamentBondRecovered
}

type TournamentLevel uint64

const (
	RootLevel TournamentLevel = iota
)

func (l TournamentLevel) String() string {
	switch l {
	case RootLevel:
		return "root"
	default:
		return "inner"
	}
}
