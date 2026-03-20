// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
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

func (a *ITournamentAdapterImpl) Result(opts *bind.CallOpts) (bool, [32]byte, [32]byte, error) {
	result, err := a.tournament.ArbitrationResult(opts)
	// ArbitrationResult reverts when it has finished with no winners
	if info, ok := ExtractJSONErrorInfo(err); ok && info.HasData {
		if ethutil.MatchesSelector(info.Data, TournamentFailedNoWinner) {
			return true, [32]byte{}, [32]byte{}, nil
		}
	}
	return result.Finished, result.WinnerCommitment, result.FinalState, err
}

func (a *ITournamentAdapterImpl) Constants(opts *bind.CallOpts) (TournamentConstants, error) {
	c, err := a.tournament.TournamentLevelConstants(opts)
	return TournamentConstants{
		MaxLevel: c.MaxLevel,
		Level:    c.Level,
		Log2step: c.Log2step,
		Height:   c.Height,
	}, err
}

func (a *ITournamentAdapterImpl) TimeFinished(opts *bind.CallOpts) (bool, uint64, error) {
	return a.tournament.TimeFinished(opts)
}

func (a *ITournamentAdapterImpl) BondValue(opts *bind.CallOpts) (*big.Int, error) {
	return a.tournament.BondValue(opts)
}

// IsCommitmentJoined checks on-chain whether a commitment has already been
// joined to this tournament. It calls the contract's getCommitment method
// and checks if the returned finalState is non-zero (indicating the commitment
// exists). This prevents duplicate JoinTournament calls after a node restart.
func (a *ITournamentAdapterImpl) IsCommitmentJoined(
	opts *bind.CallOpts, commitmentRoot [32]byte,
) (bool, error) {
	result, err := a.tournament.GetCommitment(opts, commitmentRoot)
	if err != nil {
		return false, err
	}
	return result.FinalState != [32]byte{}, nil
}

func (a *ITournamentAdapterImpl) JoinTournament(
	opts *bind.TransactOpts, finalState [32]byte, proof [][32]byte,
	leftNode [32]byte, rightNode [32]byte,
) (*types.Transaction, error) {
	return a.tournament.JoinTournament(opts, finalState, proof, leftNode, rightNode)
}

// buildFilterQuery creates a filter query for a specific tournament event
func buildFilterQuery(
	opts *bind.FilterOpts,
	tournamentAddress common.Address,
	eventName string,
) (q ethereum.FilterQuery, err error) {
	c, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	topics, err := abi.MakeTopics(
		[]any{c.Events[eventName].ID},
	)
	if err != nil {
		return q, err
	}

	q = ethereum.FilterQuery{
		Addresses: []common.Address{tournamentAddress},
		FromBlock: new(big.Int).SetUint64(opts.Start),
		Topics:    topics,
	}
	if opts.End != nil {
		q.ToBlock = new(big.Int).SetUint64(*opts.End)
	}
	return q, err
}

// retrieveEvents retrieves and parses events of a specific type
func retrieveEvents[T any](
	a *ITournamentAdapterImpl,
	opts *bind.FilterOpts,
	eventName string,
	parseFunc func(types.Log) (T, error),
) ([]T, error) {
	q, err := buildFilterQuery(opts, a.tournamentAddress, eventName)
	if err != nil {
		return nil, err
	}

	itr, err := a.filter.ChunkedFilterLogs(opts.Context, a.client, q)
	if err != nil {
		return nil, err
	}

	var events []T
	for log, err := range itr {
		if err != nil {
			return nil, err
		}
		ev, err := parseFunc(*log)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func (a *ITournamentAdapterImpl) RetrieveCommitmentJoinedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentCommitmentJoined, error) {
	return retrieveEvents(a, opts, MonitoredEvent_CommitmentJoined.String(), a.tournament.ParseCommitmentJoined)
}

func (a *ITournamentAdapterImpl) RetrieveMatchAdvancedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentMatchAdvanced, error) {
	return retrieveEvents(a, opts, MonitoredEvent_MatchAdvanced.String(), a.tournament.ParseMatchAdvanced)
}

func (a *ITournamentAdapterImpl) RetrieveMatchCreatedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentMatchCreated, error) {
	return retrieveEvents(a, opts, MonitoredEvent_MatchCreated.String(), a.tournament.ParseMatchCreated)
}

func (a *ITournamentAdapterImpl) RetrieveMatchDeletedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentMatchDeleted, error) {
	return retrieveEvents(a, opts, MonitoredEvent_MatchDeleted.String(), a.tournament.ParseMatchDeleted)
}

func (a *ITournamentAdapterImpl) RetrieveNewInnerTournamentEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentNewInnerTournament, error) {
	return retrieveEvents(a, opts, MonitoredEvent_NewInnerTournament.String(), a.tournament.ParseNewInnerTournament)
}

func buildAllEventsFilterQuery(
	opts *bind.FilterOpts,
	tournamentAddress common.Address,
) (q ethereum.FilterQuery, err error) {
	c, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	topics, err := abi.MakeTopics(
		[]any{
			c.Events[MonitoredEvent_CommitmentJoined.String()].ID,
			c.Events[MonitoredEvent_MatchAdvanced.String()].ID,
			c.Events[MonitoredEvent_MatchCreated.String()].ID,
			c.Events[MonitoredEvent_MatchDeleted.String()].ID,
			c.Events[MonitoredEvent_NewInnerTournament.String()].ID,
		},
	)
	if err != nil {
		return q, err
	}

	q = ethereum.FilterQuery{
		Addresses: []common.Address{tournamentAddress},
		FromBlock: new(big.Int).SetUint64(opts.Start),
		Topics:    topics,
	}
	if opts.End != nil {
		q.ToBlock = new(big.Int).SetUint64(*opts.End)
	}
	return q, err
}

func (a *ITournamentAdapterImpl) RetrieveAllEvents(
	opts *bind.FilterOpts,
) (*TournamentEvents, error) {
	q, err := buildAllEventsFilterQuery(opts, a.tournamentAddress)
	if err != nil {
		return nil, err
	}

	itr, err := a.filter.ChunkedFilterLogs(opts.Context, a.client, q)
	if err != nil {
		return nil, err
	}

	var commitmentJoined []*itournament.ITournamentCommitmentJoined
	var matchAdvanced []*itournament.ITournamentMatchAdvanced
	var matchCreated []*itournament.ITournamentMatchCreated
	var matchDeleted []*itournament.ITournamentMatchDeleted
	var newInnerTournament []*itournament.ITournamentNewInnerTournament

	c, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	for log, err := range itr {
		if err != nil {
			return nil, err
		}

		switch log.Topics[0] {
		case c.Events[MonitoredEvent_CommitmentJoined.String()].ID:
			ev, err := a.tournament.ParseCommitmentJoined(*log)
			if err != nil {
				return nil, err
			}
			commitmentJoined = append(commitmentJoined, ev)
		case c.Events[MonitoredEvent_MatchAdvanced.String()].ID:
			ev, err := a.tournament.ParseMatchAdvanced(*log)
			if err != nil {
				return nil, err
			}
			matchAdvanced = append(matchAdvanced, ev)
		case c.Events[MonitoredEvent_MatchCreated.String()].ID:
			ev, err := a.tournament.ParseMatchCreated(*log)
			if err != nil {
				return nil, err
			}
			matchCreated = append(matchCreated, ev)
		case c.Events[MonitoredEvent_MatchDeleted.String()].ID:
			ev, err := a.tournament.ParseMatchDeleted(*log)
			if err != nil {
				return nil, err
			}
			matchDeleted = append(matchDeleted, ev)
		case c.Events[MonitoredEvent_NewInnerTournament.String()].ID:
			ev, err := a.tournament.ParseNewInnerTournament(*log)
			if err != nil {
				return nil, err
			}
			newInnerTournament = append(newInnerTournament, ev)
		}
	}

	return &TournamentEvents{
		CommitmentJoined:   commitmentJoined,
		MatchAdvanced:      matchAdvanced,
		MatchCreated:       matchCreated,
		MatchDeleted:       matchDeleted,
		NewInnerTournament: newInnerTournament,
	}, nil
}
