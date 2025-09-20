// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"

	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
)

type TournamentConstants struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}

// Interface for Tournament reading
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

const TournamentFailedNoWinner string = "0xb3045ef8"
