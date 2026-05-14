// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
)

type TournamentConstants struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}

// TournamentAdapter provides read and write access to tournament contracts.
type TournamentAdapter interface {
	RetrieveCommitmentJoinedEvents(opts *bind.FilterOpts) ([]*itournament.ITournamentCommitmentJoined, error)
	RetrieveMatchAdvancedEvents(opts *bind.FilterOpts) ([]*itournament.ITournamentMatchAdvanced, error)
	RetrieveMatchCreatedEvents(opts *bind.FilterOpts) ([]*itournament.ITournamentMatchCreated, error)
	RetrieveMatchDeletedEvents(opts *bind.FilterOpts) ([]*itournament.ITournamentMatchDeleted, error)
	RetrieveNewInnerTournamentEvents(opts *bind.FilterOpts) ([]*itournament.ITournamentNewInnerTournament, error)
	RetrieveAllEvents(opts *bind.FilterOpts) (*TournamentEvents, error)
	Result(opts *bind.CallOpts) (bool, [32]byte, [32]byte, error)
	Constants(opts *bind.CallOpts) (TournamentConstants, error)
	TimeFinished(opts *bind.CallOpts) (bool, uint64, error)
	BondValue(opts *bind.CallOpts) (*big.Int, error)
	IsCommitmentJoined(opts *bind.CallOpts, commitmentRoot [32]byte) (bool, error)
	JoinTournament(opts *bind.TransactOpts, finalState [32]byte, proof [][32]byte,
		leftNode [32]byte, rightNode [32]byte) (*types.Transaction, error)
}

// DaveConsensusAdapter wraps access to the IDaveConsensus contract.
type DaveConsensusAdapter interface {
	ParseEpochSealed(log types.Log) (*idaveconsensus.IDaveConsensusEpochSealed, error)
	CanSettle(opts *bind.CallOpts) (CanSettleResult, error)
	IsEpochSettled(opts *bind.CallOpts, epochNumber uint64) (bool, error)
	Settle(opts *bind.TransactOpts, epochNumber *big.Int,
		outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error)
}

// CanSettleResult holds the result of a CanSettle call.
type CanSettleResult struct {
	IsFinished       bool
	EpochNumber      *big.Int
	WinnerCommitment [32]byte
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
}

type TournamentLevel uint64

const (
	RootLevel TournamentLevel = iota
	MiddleLevel
	BottomLevel
)

func (l TournamentLevel) String() string {
	switch l {
	case RootLevel:
		return "root"
	case MiddleLevel:
		return "middle"
	case BottomLevel:
		return "bottom"
	default:
		return "unknown"
	}
}
