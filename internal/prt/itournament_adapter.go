// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"cmp"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	errInvalidBondRecovery         = errors.New("invalid tournament bond recovery state")
	errTournamentMatchPhaseChanged = errors.New("tournament match phase changed within pinned observation")
	errUnpinnedTournamentRead      = errors.New("tournament observation requires a pinned block")
)

// ITournament Wrapper
type ITournamentAdapterImpl struct {
	tournament        *itournament.ITournament
	client            *ethclient.Client
	tournamentAddress common.Address
	filter            ethutil.Filter
}

func NewITournamentAdapter(
	tournamentAddress common.Address,
	client *ethclient.Client,
	filter ethutil.Filter,
) (TournamentAdapter, error) {
	tournamentContract, err := itournament.NewITournament(tournamentAddress, client)
	if err != nil {
		return nil, err
	}
	return &ITournamentAdapterImpl{
		tournament:        tournamentContract,
		tournamentAddress: tournamentAddress,
		client:            client,
		filter:            filter,
	}, nil
}

func (a *ITournamentAdapterImpl) Descriptor(opts *bind.CallOpts) (TournamentDescriptor, error) {
	descriptor, err := a.tournament.TournamentDescriptor(opts)
	if err != nil {
		return TournamentDescriptor{}, err
	}
	return tournamentDescriptorFromBinding(descriptor)
}

func tournamentDescriptorFromBinding(descriptor itournament.ITournamentTournamentDescriptor) (TournamentDescriptor, error) {
	kind, err := tournamentEnum("kind", descriptor.Kind, model.TournamentKindLeaf, model.TournamentKindNonLeaf)
	if err != nil {
		return TournamentDescriptor{}, err
	}
	baseCycle, err := tournamentUint256("base cycle", descriptor.BaseCycle)
	if err != nil {
		return TournamentDescriptor{}, err
	}
	return TournamentDescriptor{
		InitialHash:  descriptor.InitialHash,
		BaseCycle:    baseCycle,
		Log2Stride:   descriptor.Log2Stride,
		Height:       descriptor.Height,
		Level:        descriptor.Level,
		Kind:         kind,
		StartInstant: descriptor.StartInstant,
		Allowance:    descriptor.Allowance,
	}, nil
}

func (a *ITournamentAdapterImpl) Standing(opts *bind.CallOpts) (TournamentStanding, error) {
	standing, err := a.tournament.TournamentStanding(opts)
	if err != nil {
		return TournamentStanding{}, err
	}
	return tournamentStandingFromBinding(standing)
}

func tournamentStandingFromBinding(standing itournament.ITournamentTournamentStandingView) (TournamentStanding, error) {
	state, err := tournamentEnum("standing", standing.Standing,
		model.TournamentStandingMatchesActive, model.TournamentStandingAwaitingClosure,
		model.TournamentStandingRootWinner, model.TournamentStandingRootFailed, model.TournamentStandingInnerWinner,
		model.TournamentStandingInnerEliminableNoWinner, model.TournamentStandingInnerEliminableWinnerExpired)
	if err != nil {
		return TournamentStanding{}, err
	}
	return TournamentStanding{
		State:            state,
		AcceptsJoins:     standing.AcceptsJoins,
		HasCandidate:     standing.HasCandidate,
		Candidate:        standing.Candidate,
		FinalState:       standing.FinalState,
		ParentCommitment: standing.ParentCommitment,
		FinishedAt:       standing.FinishedAt,
		WinnerExpiresAt:  standing.WinnerExpiresAt,
	}, nil
}

func (a *ITournamentAdapterImpl) BondValue(opts *bind.CallOpts) (*big.Int, error) {
	value, err := a.tournament.BondValue(opts)
	if err != nil {
		return nil, err
	}
	return tournamentUint256("bond value", value)
}

func (a *ITournamentAdapterImpl) BondRecovery(opts *bind.CallOpts) (BondRecovery, error) {
	recovery, err := a.tournament.BondRecovery(opts)
	if err != nil {
		return BondRecovery{}, err
	}
	return bondRecoveryFromBinding(recovery.Disposition, recovery.Claimer, recovery.Payment)
}

func bondRecoveryFromBinding(
	disposition uint8,
	claimer common.Address,
	payment *big.Int,
) (BondRecovery, error) {
	d, err := tournamentEnum("bond disposition", disposition, model.BondDispositionTournamentRunning,
		model.BondDispositionNoWinner, model.BondDispositionRecoverable, model.BondDispositionRecovered)
	if err != nil {
		return BondRecovery{}, fmt.Errorf("%w: %w", errInvalidBondRecovery, err)
	}
	amount, err := tournamentUint256("payment", payment)
	if err != nil {
		return BondRecovery{}, fmt.Errorf("%w: %w", errInvalidBondRecovery, err)
	}

	if d == model.BondDispositionRecoverable {
		if claimer == (common.Address{}) {
			return BondRecovery{}, fmt.Errorf("%w: recoverable bond has zero claimer", errInvalidBondRecovery)
		}
	} else if claimer != (common.Address{}) || payment.Sign() != 0 {
		return BondRecovery{}, fmt.Errorf(
			"%w: inactive disposition %s has claimer %s or payment %s",
			errInvalidBondRecovery,
			d,
			claimer,
			payment,
		)
	}

	return BondRecovery{
		Disposition: d,
		Claimer:     claimer,
		Payment:     amount,
	}, nil
}

func (a *ITournamentAdapterImpl) CommitmentStanding(
	opts *bind.CallOpts, commitmentRoot [32]byte,
) (CommitmentStanding, error) {
	standing, err := a.tournament.CommitmentStanding(opts, commitmentRoot)
	if err != nil {
		return CommitmentStanding{}, err
	}
	return commitmentStandingFromBinding(standing), nil
}

func commitmentStandingFromBinding(standing itournament.ITournamentCommitmentStandingView) CommitmentStanding {
	return CommitmentStanding{
		Joined:         standing.Joined,
		FinalState:     standing.FinalState,
		Claimer:        standing.Claimer,
		ClockRunning:   standing.ClockRunning,
		ClockDeadline:  standing.ClockDeadline,
		ClockAllowance: standing.ClockAllowance,
	}
}

// tournamentEnum maps ABI ordinal values to the shared model vocabulary. The
// explicit order at each call site follows the deployed Solidity declaration.
func tournamentEnum[T ~string](name string, raw uint8, values ...T) (T, error) {
	if int(raw) >= len(values) {
		var zero T
		return zero, fmt.Errorf("tournament has unknown %s %d", name, raw)
	}
	return values[raw], nil
}

const tournamentIntegerBits = 256

func tournamentUint256(name string, value *big.Int) (*big.Int, error) {
	if value == nil {
		return nil, fmt.Errorf("tournament %s is nil", name)
	}
	if value.Sign() < 0 {
		return nil, fmt.Errorf("tournament %s is negative", name)
	}
	if value.BitLen() > tournamentIntegerBits {
		return nil, fmt.Errorf("tournament %s exceeds uint256", name)
	}
	return new(big.Int).Set(value), nil
}

func requirePinnedTournamentRead(opts *bind.CallOpts) error {
	if opts == nil || opts.Pending {
		return errUnpinnedTournamentRead
	}
	if opts.BlockHash != (common.Hash{}) && opts.BlockNumber == nil {
		return nil
	}
	if opts.BlockNumber == nil || !opts.BlockNumber.IsUint64() || opts.BlockHash != (common.Hash{}) {
		return errUnpinnedTournamentRead
	}
	return nil
}

func (a *ITournamentAdapterImpl) MatchSnapshot(opts *bind.CallOpts, one, two [32]byte) (ObservedMatchSnapshot, error) {
	if err := requirePinnedTournamentRead(opts); err != nil {
		return ObservedMatchSnapshot{}, err
	}
	timeout, err := a.tournament.ClassifyMatchTimeout(opts, itournament.MatchId{CommitmentOne: one, CommitmentTwo: two})
	if err != nil {
		return ObservedMatchSnapshot{}, err
	}
	phase, err := tournamentEnum("match phase", timeout.ActualPhase,
		model.MatchPhaseUninitialized, model.MatchPhaseBisecting, model.MatchPhaseReadyToSeal, model.MatchPhaseSealed)
	if err != nil {
		return ObservedMatchSnapshot{}, err
	}
	outcome, err := tournamentEnum("match timeout outcome", timeout.Outcome,
		model.MatchTimeoutNone, model.MatchTimeoutOneWins, model.MatchTimeoutTwoWins, model.MatchTimeoutEliminateBoth)
	if err != nil {
		return ObservedMatchSnapshot{}, err
	}
	if (outcome == model.MatchTimeoutNone || outcome == model.MatchTimeoutEliminateBoth) && timeout.DeferredCharge != 0 {
		return ObservedMatchSnapshot{}, errors.New("tournament timeout has an inactive deferred charge")
	}
	snapshot := ObservedMatchSnapshot{Phase: phase, TimeoutOutcome: outcome, DeferredCharge: timeout.DeferredCharge}
	matchHash := crypto.Keccak256Hash(one[:], two[:])
	switch phase {
	case model.MatchPhaseUninitialized:
		if outcome != model.MatchTimeoutNone {
			return ObservedMatchSnapshot{}, errors.New("absent tournament match has a timeout outcome")
		}
	case model.MatchPhaseBisecting:
		view, err := a.tournament.BisectingMatch(opts, matchHash)
		if err != nil {
			return ObservedMatchSnapshot{}, err
		}
		if view.ActualPhase != timeout.ActualPhase {
			return ObservedMatchSnapshot{}, errTournamentMatchPhaseChanged
		}
		value := view.Value
		position, cycle, side, err := bisectionFields(value.SegmentStartPosition, value.SegmentStartCycle, value.Responder)
		if err != nil {
			return ObservedMatchSnapshot{}, err
		}
		if value.CurrentHeight <= 1 {
			return ObservedMatchSnapshot{}, errors.New("bisecting tournament match has height below two")
		}
		snapshot.Bisecting = &BisectingMatch{RevealingParent: value.RevealingParent,
			WaitingLeft: value.WaitingLeft, WaitingRight: value.WaitingRight, SegmentStartPosition: position,
			SegmentStartCycle: cycle, CurrentHeight: value.CurrentHeight, Responder: side}
	case model.MatchPhaseReadyToSeal:
		view, err := a.tournament.ReadyToSealMatch(opts, matchHash)
		if err != nil {
			return ObservedMatchSnapshot{}, err
		}
		if view.ActualPhase != timeout.ActualPhase {
			return ObservedMatchSnapshot{}, errTournamentMatchPhaseChanged
		}
		value := view.Value
		position, cycle, side, err := bisectionFields(value.SegmentStartPosition, value.SegmentStartCycle, value.Responder)
		if err != nil {
			return ObservedMatchSnapshot{}, err
		}
		snapshot.ReadyToSeal = &ReadyToSealMatch{RevealingParent: value.RevealingParent,
			WaitingLeft: value.WaitingLeft, WaitingRight: value.WaitingRight, SegmentStartPosition: position,
			SegmentStartCycle: cycle, Responder: side}
	case model.MatchPhaseSealed:
		view, err := a.tournament.SealedMatch(opts, matchHash)
		if err != nil {
			return ObservedMatchSnapshot{}, err
		}
		if view.ActualPhase != timeout.ActualPhase {
			return ObservedMatchSnapshot{}, errTournamentMatchPhaseChanged
		}
		value := view.Value
		position, err := tournamentUint256("divergence position", value.DivergencePosition)
		if err != nil {
			return ObservedMatchSnapshot{}, err
		}
		cycle, err := tournamentUint256("divergence cycle", value.DivergenceCycle)
		if err != nil {
			return ObservedMatchSnapshot{}, err
		}
		if timeout.DeferredCharge != 0 {
			return ObservedMatchSnapshot{}, errors.New("sealed tournament match has a deferred charge")
		}
		snapshot.Sealed = &SealedMatch{AgreeState: value.AgreeState, DivergencePosition: position,
			DivergenceCycle: cycle, FinalStateOne: value.FinalStateOne, FinalStateTwo: value.FinalStateTwo}
	}
	return snapshot, nil
}

func bisectionFields(position, cycle *big.Int, rawSide uint8) (*big.Int, *big.Int, model.CommitmentSide, error) {
	positionCopy, err := tournamentUint256("segment start position", position)
	if err != nil {
		return nil, nil, "", err
	}
	cycleCopy, err := tournamentUint256("segment start cycle", cycle)
	if err != nil {
		return nil, nil, "", err
	}
	side, err := tournamentEnum("responder", rawSide, model.CommitmentSideOne, model.CommitmentSideTwo)
	return positionCopy, cycleCopy, side, err
}

func (a *ITournamentAdapterImpl) InnerResult(opts *bind.CallOpts) (InnerResult, error) {
	value, err := a.tournament.InnerResult(opts)
	if err != nil {
		return InnerResult{}, err
	}
	disposition, err := tournamentEnum("inner disposition", value.Disposition,
		model.InnerTournamentUnsettled, model.InnerTournamentWinner, model.InnerTournamentEliminable)
	if err != nil {
		return InnerResult{}, err
	}
	if disposition != model.InnerTournamentWinner && (value.ParentCommitment != [32]byte{} || value.PausedAllowance != 0) {
		return InnerResult{}, errors.New("inactive tournament inner result has winner fields")
	}
	return InnerResult{Disposition: disposition, ParentCommitment: value.ParentCommitment, PausedAllowance: value.PausedAllowance}, nil
}

func (a *ITournamentAdapterImpl) StructuralEventCounts(opts *bind.CallOpts) (StructuralEventCounts, error) {
	if err := requirePinnedTournamentRead(opts); err != nil {
		return StructuralEventCounts{}, err
	}
	var counts StructuralEventCounts
	for _, counter := range []struct {
		name  string
		read  func(*bind.CallOpts) (*big.Int, error)
		value **big.Int
	}{
		{tournamentEventCommitmentJoined, a.tournament.GetCommitmentJoinedCount, &counts.CommitmentJoined},
		{tournamentEventMatchCreated, a.tournament.GetMatchCreatedCount, &counts.MatchCreated},
		{tournamentEventMatchAdvanced, a.tournament.GetMatchAdvancedCount, &counts.MatchAdvanced},
		{tournamentEventLeafMatchSealed, a.tournament.GetLeafMatchSealedCount, &counts.LeafMatchSealed},
		{tournamentEventMatchDeleted, a.tournament.GetMatchDeletedCount, &counts.MatchDeleted},
		{tournamentEventNewInnerTournament, a.tournament.GetNewInnerTournamentCount, &counts.NewInnerTournament},
	} {
		value, err := counter.read(opts)
		if err != nil {
			return StructuralEventCounts{}, fmt.Errorf("reading %s count: %w", counter.name, err)
		}
		*counter.value, err = tournamentUint256(counter.name+" count", value)
		if err != nil {
			return StructuralEventCounts{}, err
		}
	}
	return counts, nil
}

func (a *ITournamentAdapterImpl) JoinTournament(
	opts *bind.TransactOpts, finalState [32]byte, proof [][32]byte,
	leftNode [32]byte, rightNode [32]byte,
) (*types.Transaction, error) {
	return a.tournament.JoinTournament(opts, finalState, proof, leftNode, rightNode)
}

func (a *ITournamentAdapterImpl) TryRecoveringBond(opts *bind.TransactOpts) (*types.Transaction, error) {
	return a.tournament.TryRecoveringBond(opts)
}

var tournamentEventNames = [...]string{
	"CommitmentJoined", "MatchAdvanced", "MatchCreated", "MatchDeleted",
	"NewInnerTournament", "LeafMatchSealed", "PartialBondRefund", "BondRecovered",
}

func buildAllEventsFilterQuery(
	opts *bind.FilterOpts,
	tournamentAddress common.Address,
	contractABI *abi.ABI,
) (ethereum.FilterQuery, error) {
	if opts == nil {
		return ethereum.FilterQuery{}, errors.New("tournament event filter is nil")
	}
	topics := make([]common.Hash, 0, len(tournamentEventNames))
	for _, name := range tournamentEventNames {
		event, ok := contractABI.Events[name]
		if !ok {
			return ethereum.FilterQuery{}, fmt.Errorf("ITournament ABI is missing monitored event %s", name)
		}
		topics = append(topics, event.ID)
	}
	q := ethereum.FilterQuery{
		Addresses: []common.Address{tournamentAddress},
		FromBlock: new(big.Int).SetUint64(opts.Start),
		Topics:    [][]common.Hash{topics},
	}
	if opts.End != nil {
		q.ToBlock = new(big.Int).SetUint64(*opts.End)
	}
	return q, nil
}

func (a *ITournamentAdapterImpl) RetrieveAllEvents(opts *bind.FilterOpts) (*TournamentEvents, error) {
	if opts == nil || opts.End == nil || opts.Start > *opts.End {
		return nil, errors.New("tournament events require a fixed, nonempty block range")
	}
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	query, err := buildAllEventsFilterQuery(opts, a.tournamentAddress, contractABI)
	if err != nil {
		return nil, err
	}
	// The filter owns its chunk sizing. A local copy prevents concurrent reads
	// through one adapter from changing each other's filter state.
	filter := a.filter
	iterator, err := filter.ChunkedFilterLogs(opts.Context, a.client, query)
	if err != nil {
		return nil, err
	}
	type logPosition struct {
		block uint64
		index uint
	}
	type eventIdentity struct {
		transaction common.Hash
		index       uint
	}
	positions := make(map[logPosition]struct{})
	identities := make(map[eventIdentity]struct{})
	blockHashes := make(map[uint64]common.Hash)
	var logs []types.Log
	for entry, err := range iterator {
		if err != nil {
			return nil, err
		}
		if entry == nil || entry.Address != a.tournamentAddress || entry.Removed {
			return nil, errors.New("tournament event has an invalid address or removed flag")
		}
		if entry.BlockNumber < opts.Start || entry.BlockNumber > *opts.End {
			return nil, errors.New("tournament event is outside the requested block range")
		}
		if entry.TxHash == (common.Hash{}) || entry.BlockHash == (common.Hash{}) {
			return nil, errors.New("tournament event has an incomplete log identity")
		}
		position := logPosition{entry.BlockNumber, entry.Index}
		identity := eventIdentity{entry.TxHash, entry.Index}
		if _, exists := positions[position]; exists {
			return nil, errors.New("tournament event repeats a block log position")
		}
		if _, exists := identities[identity]; exists {
			return nil, errors.New("tournament event repeats a transaction log identity")
		}
		if hash, exists := blockHashes[entry.BlockNumber]; exists && hash != entry.BlockHash {
			return nil, errors.New("tournament events disagree on the block hash")
		}
		positions[position] = struct{}{}
		identities[identity] = struct{}{}
		blockHashes[entry.BlockNumber] = entry.BlockHash
		logs = append(logs, *entry)
	}
	slices.SortFunc(logs, func(one, two types.Log) int {
		if order := cmp.Compare(one.BlockNumber, two.BlockNumber); order != 0 {
			return order
		}
		return cmp.Compare(one.Index, two.Index)
	})
	events := &TournamentEvents{}
	for _, entry := range logs {
		if err := a.appendTournamentEvent(contractABI, entry, events); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func (a *ITournamentAdapterImpl) appendTournamentEvent(contractABI *abi.ABI, entry types.Log, events *TournamentEvents) error {
	if len(entry.Topics) == 0 {
		return errors.New("tournament event has no topic")
	}
	definition, err := contractABI.EventByID(entry.Topics[0])
	if err != nil {
		return fmt.Errorf("tournament event has unexpected topic %s: %w", entry.Topics[0], err)
	}
	// Every current tournament event field occupies one static ABI word.
	// UnpackLog skips decoding empty data, so validate its exact size first.
	const abiWordBytes = 32
	nonIndexed := len(definition.Inputs.NonIndexed())
	if len(entry.Data) != nonIndexed*abiWordBytes || len(entry.Topics) != len(definition.Inputs)-nonIndexed+1 {
		return fmt.Errorf("tournament %s event has invalid ABI field lengths", definition.Name)
	}
	switch entry.Topics[0] {
	case contractABI.Events["CommitmentJoined"].ID:
		event, err := a.tournament.ParseCommitmentJoined(entry)
		if err != nil {
			return err
		}
		events.CommitmentJoined = append(events.CommitmentJoined, event)
	case contractABI.Events["MatchAdvanced"].ID:
		event, err := a.tournament.ParseMatchAdvanced(entry)
		if err != nil {
			return err
		}
		events.MatchAdvanced = append(events.MatchAdvanced, event)
	case contractABI.Events["MatchCreated"].ID:
		event, err := a.tournament.ParseMatchCreated(entry)
		if err != nil {
			return err
		}
		events.MatchCreated = append(events.MatchCreated, event)
	case contractABI.Events["MatchDeleted"].ID:
		event, err := a.tournament.ParseMatchDeleted(entry)
		if err != nil {
			return err
		}
		if _, err := model.MatchDeletionReasonFromUint8(event.Reason); err != nil {
			return fmt.Errorf("tournament deletion reason: %w", err)
		}
		if _, err := model.WinnerCommitmentFromUint8(event.WinnerCommitment); err != nil {
			return fmt.Errorf("tournament deletion winner: %w", err)
		}
		events.MatchDeleted = append(events.MatchDeleted, event)
	case contractABI.Events["NewInnerTournament"].ID:
		event, err := a.tournament.ParseNewInnerTournament(entry)
		if err != nil {
			return err
		}
		events.NewInnerTournament = append(events.NewInnerTournament, event)
	case contractABI.Events["LeafMatchSealed"].ID:
		event, err := a.tournament.ParseLeafMatchSealed(entry)
		if err != nil {
			return err
		}
		events.LeafMatchSealed = append(events.LeafMatchSealed, event)
	case contractABI.Events["PartialBondRefund"].ID:
		event, err := a.tournament.ParsePartialBondRefund(entry)
		if err != nil {
			return err
		}
		events.PartialBondRefund = append(events.PartialBondRefund, event)
	case contractABI.Events["BondRecovered"].ID:
		event, err := a.tournament.ParseBondRecovered(entry)
		if err != nil {
			return err
		}
		events.BondRecovered = append(events.BondRecovered, event)
	default:
		return fmt.Errorf("tournament event has unexpected topic %s", entry.Topics[0])
	}
	return nil
}
