// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
)

func applyTournamentCreationEvent(t *model.Tournament, creation *TournamentCreation) error {
	if creation == nil {
		return fmt.Errorf("tournament %s has no creation event", t.Address)
	}
	d := creation.Descriptor
	if t.Address != creation.Address || t.InitialHash != d.InitialHash || d.BaseCycle == nil || t.BaseCycle.ToBig().Cmp(d.BaseCycle) != 0 ||
		t.Log2Step != d.Log2Stride || t.Height != d.Height || t.Level != d.Level || t.Kind != d.Kind ||
		t.StartInstant != d.StartInstant || t.Allowance != d.Allowance || t.BondValue != creation.BondValue ||
		creation.BondValue.ToBig().Sign() == 0 || creation.Event.BlockNumber != t.StartInstant || creation.Event.TxHash == (common.Hash{}) {
		return fmt.Errorf("tournament %s descriptor or bond does not match its creation event", t.Address)
	}
	if t.CreationEvent != nil && *t.CreationEvent != creation.Event {
		return fmt.Errorf("tournament %s has conflicting creation events", t.Address)
	}
	t.CreationEvent = new(creation.Event)
	return nil
}

func childTournamentCreation(t *model.Tournament, parent *model.Match,
	event *itournament.ITournamentNewInnerTournament) (*TournamentCreation, error) {
	if parent.CommitmentOne != event.One || parent.CommitmentTwo != event.Two ||
		event.Raw.Address != t.Address {
		return nil, fmt.Errorf("child creation does not match parent tournament and creation pair")
	}
	d, err := tournamentDescriptorFromBinding(event.Descriptor)
	if err != nil {
		return nil, err
	}
	bond, err := model.Uint256FromBig(event.BondValue)
	if err != nil {
		return nil, err
	}
	return &TournamentCreation{Address: event.ChildTournament, Descriptor: d, BondValue: bond,
		ContestedFinalStateOne: new(common.Hash(event.ContestedFinalStateOne)),
		ContestedFinalStateTwo: new(common.Hash(event.ContestedFinalStateTwo)),
		Event: model.TournamentCreationEvent{BlockNumber: event.Raw.BlockNumber, TxHash: event.Raw.TxHash,
			LogIndex: uint64(event.Raw.Index)}}, nil
}

func validateChildContestedStates(child *model.Tournament, parent *model.Match) error {
	if child.ContestedFinalStateOne == nil || child.ContestedFinalStateTwo == nil {
		return fmt.Errorf("child contested states are absent")
	}
	if parent.Snapshot.Phase == model.MatchPhaseSealed && (parent.Snapshot.Sealed == nil ||
		*child.ContestedFinalStateOne != parent.Snapshot.Sealed.FinalStateOne ||
		*child.ContestedFinalStateTwo != parent.Snapshot.Sealed.FinalStateTwo) {
		return fmt.Errorf("child contested states do not match current sealed parent")
	}
	return nil
}
