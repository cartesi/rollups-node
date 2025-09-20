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
	if info, ok := ExtractJsonErrorInfo(err); ok && info.HasData && info.Data == TournamentFailedNoWinner {
		return true, [32]byte{}, [32]byte{}, nil
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

func buildCommitmentJoinedFilterQuery(
	opts *bind.FilterOpts,
	tournamentAddress common.Address,
) (q ethereum.FilterQuery, err error) {
	c, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	topics, err := abi.MakeTopics(
		[]any{c.Events[MonitoredEvent_CommitmentJoined.String()].ID},
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

func (a *ITournamentAdapterImpl) RetrieveCommitmentJoinedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentCommitmentJoined, error) {
	q, err := buildCommitmentJoinedFilterQuery(opts, a.tournamentAddress)
	if err != nil {
		return nil, err
	}

	itr, err := a.filter.ChunkedFilterLogs(opts.Context, a.client, q)
	if err != nil {
		return nil, err
	}

	var events []*itournament.ITournamentCommitmentJoined
	for log, err := range itr {
		if err != nil {
			return nil, err
		}
		ev, err := a.tournament.ParseCommitmentJoined(*log)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func buildMatchAdvancedFilterQuery(
	opts *bind.FilterOpts,
	tournamentAddress common.Address,
) (q ethereum.FilterQuery, err error) {
	c, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	topics, err := abi.MakeTopics(
		[]any{c.Events[MonitoredEvent_MatchAdvanced.String()].ID},
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

func (a *ITournamentAdapterImpl) RetrieveMatchAdvancedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentMatchAdvanced, error) {
	q, err := buildMatchAdvancedFilterQuery(opts, a.tournamentAddress)
	if err != nil {
		return nil, err
	}

	itr, err := a.filter.ChunkedFilterLogs(opts.Context, a.client, q)
	if err != nil {
		return nil, err
	}

	var events []*itournament.ITournamentMatchAdvanced
	for log, err := range itr {
		if err != nil {
			return nil, err
		}
		ev, err := a.tournament.ParseMatchAdvanced(*log)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func buildMatchCreatedFilterQuery(
	opts *bind.FilterOpts,
	tournamentAddress common.Address,
) (q ethereum.FilterQuery, err error) {
	c, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	topics, err := abi.MakeTopics(
		[]any{c.Events[MonitoredEvent_MatchCreated.String()].ID},
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

func (a *ITournamentAdapterImpl) RetrieveMatchCreatedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentMatchCreated, error) {
	q, err := buildMatchCreatedFilterQuery(opts, a.tournamentAddress)
	if err != nil {
		return nil, err
	}

	itr, err := a.filter.ChunkedFilterLogs(opts.Context, a.client, q)
	if err != nil {
		return nil, err
	}

	var events []*itournament.ITournamentMatchCreated
	for log, err := range itr {
		if err != nil {
			return nil, err
		}
		ev, err := a.tournament.ParseMatchCreated(*log)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func buildMatchDeletedFilterQuery(
	opts *bind.FilterOpts,
	tournamentAddress common.Address,
) (q ethereum.FilterQuery, err error) {
	c, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	topics, err := abi.MakeTopics(
		[]any{c.Events[MonitoredEvent_MatchDeleted.String()].ID},
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

func (a *ITournamentAdapterImpl) RetrieveMatchDeletedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentMatchDeleted, error) {
	q, err := buildMatchDeletedFilterQuery(opts, a.tournamentAddress)
	if err != nil {
		return nil, err
	}

	itr, err := a.filter.ChunkedFilterLogs(opts.Context, a.client, q)
	if err != nil {
		return nil, err
	}

	var events []*itournament.ITournamentMatchDeleted
	for log, err := range itr {
		if err != nil {
			return nil, err
		}
		ev, err := a.tournament.ParseMatchDeleted(*log)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func buildNewInnerTournamentFilterQuery(
	opts *bind.FilterOpts,
	tournamentAddress common.Address,
) (q ethereum.FilterQuery, err error) {
	c, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	topics, err := abi.MakeTopics(
		[]any{c.Events[MonitoredEvent_NewInnerTournament.String()].ID},
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

func (a *ITournamentAdapterImpl) RetrieveNewInnerTournamentEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentNewInnerTournament, error) {
	q, err := buildNewInnerTournamentFilterQuery(opts, a.tournamentAddress)
	if err != nil {
		return nil, err
	}

	itr, err := a.filter.ChunkedFilterLogs(opts.Context, a.client, q)
	if err != nil {
		return nil, err
	}

	var events []*itournament.ITournamentNewInnerTournament
	for log, err := range itr {
		if err != nil {
			return nil, err
		}
		ev, err := a.tournament.ParseNewInnerTournament(*log)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
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
