// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/events"
	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

type prtRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter,
		p repository.Pagination, descending bool) ([]*Application, uint64, error)
	UpdateApplicationHealth(ctx context.Context, appID int64, state ApplicationHealth, reason *string) error

	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter,
		p repository.Pagination, descending bool) ([]*Epoch, uint64, error)
	GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	UpdateEpochStatus(ctx context.Context, nameOrAddress string, e *Epoch) error

	CreateTournament(ctx context.Context, nameOrAddress string, t *Tournament) error
	GetTournament(ctx context.Context, nameOrAddress string, address string) (*Tournament, error)
	UpdateTournament(ctx context.Context, nameOrAddress string, t *Tournament) error
	ListTournaments(ctx context.Context, nameOrAddress string, f repository.TournamentFilter,
		p repository.Pagination, descending bool) ([]*Tournament, uint64, error)

	StoreTournamentEvents(ctx context.Context, appID int64, commitments []*Commitment, matches []*Match,
		matchAdvanced []*MatchAdvanced, matchDeleted []*Match, lastBlock uint64) error

	GetCommitment(ctx context.Context, nameOrAddress string, epochIndex uint64,
		tournamentAddress string, commitmentHex string) (*Commitment, error)

	AcknowledgeAppStopped(ctx context.Context, appID int64, serviceName string) error

	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)
}

// EthClientInterface defines the methods we need from ethclient.Client
type EthClientInterface interface {
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	ChainID(ctx context.Context) (*big.Int, error)
	BlockNumber(ctx context.Context) (uint64, error)
	TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error)
}

// DefaultAdapterFactory creates adapters using a concrete *ethclient.Client.
type DefaultAdapterFactory struct {
	client *ethclient.Client
	filter ethutil.Filter
}

// NewDefaultAdapterFactory creates a DefaultAdapterFactory.
func NewDefaultAdapterFactory(client *ethclient.Client, filter ethutil.Filter) *DefaultAdapterFactory {
	return &DefaultAdapterFactory{client: client, filter: filter}
}

func (f *DefaultAdapterFactory) CreateTournamentAdapter(addr common.Address) (TournamentAdapter, error) {
	return NewITournamentAdapter(addr, f.client, f.filter)
}

func (f *DefaultAdapterFactory) CreateDaveConsensusAdapter(addr common.Address) (DaveConsensusAdapter, error) {
	return NewDaveConsensusAdapter(addr, f.client)
}

func getAllRunningApplications(ctx context.Context, r prtRepository) ([]*Application, uint64, error) {
	f := repository.ApplicationFilter{Active: Pointer(true), ConsensusType: Pointer(Consensus_PRT)}
	return r.ListApplications(ctx, f, repository.Pagination{}, false)
}

func getAllClaimComputedEpochs(ctx context.Context, r prtRepository, nameOrAddress string) ([]*Epoch, uint64, error) {
	f := repository.EpochFilter{Status: []EpochStatus{EpochStatus_ClaimComputed}}
	return r.ListEpochs(ctx, nameOrAddress, f, repository.Pagination{}, false)
}

func getAllSubTournaments(
	ctx context.Context,
	r prtRepository,
	nameOrAddress string,
	epochIndex uint64,
	tournamentAddress *common.Address,
	level TournamentLevel,
) ([]*Tournament, uint64, error) {
	f := repository.TournamentFilter{EpochIndex: &epochIndex, ParentTournamentAddress: tournamentAddress, Level: (*uint64)(&level)}
	return r.ListTournaments(ctx, nameOrAddress, f, repository.Pagination{}, false)
}

func (s *Service) setApplicationInoperable(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	return appstatus.SetInoperablef(ctx, s.Logger, s.repository, app, reasonFmt, args...)
}

func (s *Service) saveTournamentEvents(ctx context.Context, app *Application, epoch *Epoch,
	tournamentAddress common.Address, events *TournamentEvents, lastBlock uint64) error {
	commitments := make([]*Commitment, 0, len(events.CommitmentJoined))
	for _, ev := range events.CommitmentJoined {
		c := Commitment{
			ApplicationID:     app.ID,
			EpochIndex:        epoch.Index,
			TournamentAddress: tournamentAddress,
			Commitment:        ev.Commitment,
			FinalStateHash:    ev.FinalStateHash,
			SubmitterAddress:  ev.Submitter,
			BlockNumber:       ev.Raw.BlockNumber,
			TxHash:            ev.Raw.TxHash,
		}
		s.Logger.Info("Found CommitmentJoined event",
			"application", app.Name,
			"epoch_index", epoch.Index,
			"tournament", tournamentAddress.Hex(),
			"commitment", c.Commitment.String())
		commitments = append(commitments, &c)
	}

	matches := make([]*Match, 0, len(events.MatchCreated))
	for _, ev := range events.MatchCreated {
		m := Match{
			ApplicationID:       app.ID,
			EpochIndex:          epoch.Index,
			TournamentAddress:   tournamentAddress,
			IDHash:              ev.MatchIdHash,
			CommitmentOne:       ev.One,
			CommitmentTwo:       ev.Two,
			LeftOfTwo:           ev.LeftOfTwo,
			BlockNumber:         ev.Raw.BlockNumber,
			TxHash:              ev.Raw.TxHash,
			Winner:              WinnerCommitment_NONE,
			DeletionReason:      MatchDeletionReason_NOT_DELETED,
			DeletionBlockNumber: 0,
			DeletionTxHash:      common.Hash{},
		}
		s.Logger.Info("Found MatchCreated event",
			"application", app.Name,
			"epoch_index", epoch.Index,
			"tournament", tournamentAddress.Hex(),
			"id_hash", m.IDHash.String(),
			"one", m.CommitmentOne.String(),
			"two", m.CommitmentTwo.String(),
			"leftOfTwo", m.LeftOfTwo.String())
		matches = append(matches, &m)
	}

	matchAdvanced := make([]*MatchAdvanced, 0, len(events.MatchAdvanced))
	for _, ev := range events.MatchAdvanced {
		m := &MatchAdvanced{
			ApplicationID:     app.ID,
			EpochIndex:        epoch.Index,
			TournamentAddress: tournamentAddress,
			IDHash:            ev.MatchIdHash,
			OtherParent:       ev.OtherParent,
			LeftNode:          ev.LeftNode,
			BlockNumber:       ev.Raw.BlockNumber,
			TxHash:            ev.Raw.TxHash,
		}
		s.Logger.Info("Found MatchAdvanced event",
			"application", app.Name,
			"epoch_index", epoch.Index,
			"tournament", tournamentAddress.Hex(),
			"id_hash", m.IDHash.String(),
			"other_parent", m.OtherParent.String(),
			"left_node", m.LeftNode.String())
		matchAdvanced = append(matchAdvanced, m)
	}

	matchDeleted := make([]*Match, 0, len(events.MatchDeleted))
	for _, ev := range events.MatchDeleted {
		m := Match{
			ApplicationID:       app.ID,
			EpochIndex:          epoch.Index,
			TournamentAddress:   tournamentAddress,
			IDHash:              ev.MatchIdHash,
			CommitmentOne:       ev.One,
			CommitmentTwo:       ev.Two,
			Winner:              WinnerCommitmentFromUint8(ev.WinnerCommitment),
			DeletionReason:      MatchDeletionReasonFromUint8(ev.Reason),
			DeletionBlockNumber: ev.Raw.BlockNumber,
			DeletionTxHash:      ev.Raw.TxHash,
		}
		s.Logger.Info("Found MatchDeleted event",
			"application", app.Name,
			"epoch_index", epoch.Index,
			"tournament", tournamentAddress.Hex(),
			"id_hash", ((common.Hash)(ev.MatchIdHash)).String(),
			"one", ((common.Hash)(ev.One)).String(),
			"two", ((common.Hash)(ev.Two)).String(),
			"winner", m.Winner.String(),
			"reason", m.DeletionReason.String(),
		)
		matchDeleted = append(matchDeleted, &m)
	}

	err := s.repository.StoreTournamentEvents(ctx, app.ID, commitments, matches, matchAdvanced, matchDeleted, lastBlock)
	if err != nil {
		s.Logger.Error("failed to save tournament events", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}
	return nil
}

func (s *Service) createTournament(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
	level TournamentLevel,
	parentMatchIDHash *common.Hash,
	parentTournamentAddress *common.Address,
	tournamentAddress common.Address,
) (*Tournament, error) {
	adapter, err := s.adapterFactory.CreateTournamentAdapter(tournamentAddress)
	if err != nil {
		s.Logger.Error("failed to create tournament adapter", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return nil, err
	}

	constants, err := adapter.Constants(nil)
	if err != nil {
		s.Logger.Error("failed to fetch tournament constants", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return nil, err
	}

	var winnerCommitmentPtr *common.Hash
	var finalStatePtr *common.Hash
	finishedAtBlock := uint64(0)
	if epoch.ClaimTransactionHash != nil {
		finished, timeFinished, err := adapter.TimeFinished(nil)
		if err != nil {
			s.Logger.Error("failed to fetch tournament finished at time", "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
			return nil, err
		}
		if !finished {
			err := fmt.Errorf("epoch %d: tournament %s should be finished but is not",
				epoch.Index, tournamentAddress.String())
			s.Logger.Error("tournament should be finished", "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
			return nil, err
		}
		finishedAtBlock = timeFinished

		_, winnerCommitment, finalState, err := adapter.Result(nil)
		if err != nil {
			s.Logger.Error("failed to fetch tournament result", "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
			return nil, err
		}

		// root tournament with no winner.
		if level == RootLevel && winnerCommitment == [32]byte{} {
			return nil, s.setApplicationInoperable(ctx, app,
				"Epoch %d root tournament %s has finished without winners. Setting application as inoperable.",
				epoch.Index, tournamentAddress.String())
		}

		if level == RootLevel && *epoch.Commitment != winnerCommitment {
			return nil, s.setApplicationInoperable(ctx, app,
				"Epoch %d has inconsistent commitment between off-chain (%s) and on-chain (%s). Setting application as inoperable.",
				epoch.Index, epoch.Commitment.String(), hexutil.Encode(winnerCommitment[:]))
		}
		winnerCommitmentPtr = new(common.Hash)
		*winnerCommitmentPtr = winnerCommitment

		finalStatePtr = new(common.Hash)
		*finalStatePtr = finalState
	} else {
		s.Logger.Info("Found open tournament", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String())
	}

	t := &Tournament{
		ApplicationID:           app.ID,
		EpochIndex:              epoch.Index,
		Address:                 tournamentAddress,
		ParentMatchIDHash:       parentMatchIDHash,
		ParentTournamentAddress: parentTournamentAddress,
		MaxLevel:                constants.MaxLevel,
		Level:                   constants.Level,
		Log2Step:                constants.Log2step,
		Height:                  constants.Height,
		WinnerCommitment:        winnerCommitmentPtr,
		FinalStateHash:          finalStatePtr,
		FinishedAtBlock:         finishedAtBlock,
	}

	err = s.repository.CreateTournament(ctx, app.IApplicationAddress.Hex(), t)
	if err != nil {
		s.Logger.Error("failed to create tournament in database", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return nil, err
	}
	return t, nil
}

func (s *Service) updateTournamentIfFinished(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
	level TournamentLevel,
	adapter TournamentAdapter,
	t *Tournament,
	mostRecentBlock uint64,
) error {
	callOpts := &bind.CallOpts{
		Context:     ctx,
		BlockNumber: new(big.Int).SetUint64(mostRecentBlock),
	}

	finished, timeFinished, err := adapter.TimeFinished(callOpts)
	if err != nil {
		s.Logger.Error("failed to fetch tournament finished at time", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", t.Address.String(), "error", err)
		return err
	}
	if !finished {
		return nil
	}
	t.FinishedAtBlock = timeFinished

	_, winnerCommitment, finalState, err := adapter.Result(callOpts)
	if err != nil {
		s.Logger.Error("failed to fetch tournament result", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", t.Address.String(), "error", err)
		return err
	}

	// root tournament with no winner.
	if level == RootLevel && winnerCommitment == [32]byte{} {
		return s.setApplicationInoperable(ctx, app,
			"Epoch %d root tournament %s has finished without winners. Setting application as inoperable.",
			epoch.Index, t.Address.String())
	}

	if level == RootLevel && *epoch.Commitment != winnerCommitment {
		return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent commitment between off-chain (%s) and on-chain (%s)",
			epoch.Index, epoch.Commitment.String(), hexutil.Encode(winnerCommitment[:]))
	}
	t.WinnerCommitment = new(common.Hash)
	*t.WinnerCommitment = winnerCommitment

	t.FinalStateHash = new(common.Hash)
	*t.FinalStateHash = finalState

	return s.repository.UpdateTournament(ctx, app.IApplicationAddress.Hex(), t)
}

func (s *Service) checkEpochs(ctx context.Context, app *Application, mostRecentBlock uint64) error {
	if app.LastTournamentCheckBlock >= mostRecentBlock {
		s.Logger.Debug("No new blocks since last tournament check", "application", app.Name,
			"last_tournament_check_block", app.LastTournamentCheckBlock, "most_recent_block", mostRecentBlock)
		return nil // nothing to do
	}

	epochs, _, err := getAllClaimComputedEpochs(ctx, s.repository, app.Name)
	if err != nil {
		s.Logger.Error("failed to list epochs", "application", app.Name, "error", err)
		return err
	}
	if len(epochs) == 0 {
		s.Logger.Debug("No epochs with claim computed status", "application", app.Name)
		return nil // nothing to do
	}

	consensus, err := s.adapterFactory.CreateDaveConsensusAdapter(app.IConsensusAddress)
	if err != nil {
		s.Logger.Error("failed to bind dave consensus contract", "application", app.Name,
			"consensus_address", app.IConsensusAddress.String(), "error", err)
		return err
	}

	for _, epoch := range epochs {
		if epoch.TournamentAddress == nil || epoch.Commitment == nil ||
			epoch.MachineHash == nil || epoch.OutputsMerkleRoot == nil {
			return s.setApplicationInoperable(ctx, app,
				"epoch %d has missing required fields for ClaimComputed status", epoch.Index)
		}

		if epoch.ClaimTransactionHash == nil { // epoch not claimed on-chain yet
			err = s.fetchTournamentData(ctx, app, epoch, RootLevel, nil, nil, *epoch.TournamentAddress, mostRecentBlock)
			if err != nil {
				s.Logger.Error("failed to fetch root tournament data", "application", app.Name,
					"epoch", epoch.Index, "tournament", epoch.TournamentAddress.String(), "error", err)
				return err
			}
			// if this epoch is not claimed on-chain yet, all other epochs with higher index should not be claimed either, so we can
			// stop processing here.
			break
		}

		receipt, err := s.client.TransactionReceipt(ctx, *epoch.ClaimTransactionHash)
		if err != nil {
			s.Logger.Error("failed to fetch transaction receipt for epoch", "application", app.Name,
				"epoch", epoch.Index, "tx", epoch.ClaimTransactionHash, "error", err)
			return err
		}

		if receipt.Status != 1 {
			return fmt.Errorf("epoch %d: EpochSealed transaction hash points to failed transaction", epoch.Index)
		}

		var event *idaveconsensus.IDaveConsensusEpochSealed
		for _, vLog := range receipt.Logs {
			event, err = consensus.ParseEpochSealed(*vLog)
			if err != nil {
				continue // Skip logs that don't match
			}
			break
		}
		if event == nil {
			return fmt.Errorf("epoch %d: failed to find EpochSealed event in receipt logs", epoch.Index)
		}

		if epoch.Index != event.EpochNumber.Uint64()-1 {
			return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent index between off-chain (%d) and on-chain (%d)",
				epoch.Index, epoch.Index, event.EpochNumber.Uint64()-1)
		}
		if *epoch.MachineHash != event.InitialMachineStateHash {
			return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent machine hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.MachineHash.String(), hexutil.Encode(event.InitialMachineStateHash[:]))
		}
		if *epoch.OutputsMerkleRoot != event.OutputsMerkleRoot {
			return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent claim hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.OutputsMerkleRoot.String(), hexutil.Encode(event.OutputsMerkleRoot[:]))
		}

		err = s.fetchTournamentData(ctx, app, epoch, RootLevel, nil, nil, *epoch.TournamentAddress, mostRecentBlock)
		if err != nil {
			s.Logger.Error("failed to fetch tournament data", "application", app.Name,
				"epoch", epoch.Index, "tournament", epoch.TournamentAddress.String(), "error", err)
			return err
		}

		s.Logger.Info("Found finalized epoch. OutputsMerkleRoot matched. Setting claim as accepted",
			"application", app.Name,
			"epoch", epoch.Index,
			"event_block_number", event.Raw.BlockNumber,
			"claim_hash", fmt.Sprintf("%x", event.OutputsMerkleRoot),
			"tx", epoch.ClaimTransactionHash,
		)

		epoch.Status = EpochStatus_ClaimAccepted
		err = s.repository.UpdateEpochStatus(ctx, app.Name, epoch)
		if err != nil {
			s.Logger.Error("failed to update epoch status to claim accepted", "application", app.Name, "epoch", epoch.Index, "error", err)
			return err
		}
		s.publisher.Publish(ctx, events.Notification{
			Channel:       events.ChannelClaimAccepted,
			ApplicationID: app.ID,
			EpochIndex:    epoch.Index,
		})
	}
	return nil
}

func (s *Service) fetchTournamentData(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
	level TournamentLevel,
	parentMatchIDHash *common.Hash,
	parentTournamentAddress *common.Address,
	tournamentAddress common.Address,
	mostRecentBlock uint64,
) error {
	s.Logger.Debug("Fetching tournament data", "level", level, "application", app.Name, "tournament", tournamentAddress.String())

	adapter, err := s.adapterFactory.CreateTournamentAdapter(tournamentAddress)
	if err != nil {
		s.Logger.Error("failed to create tournament adapter", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}

	t, err := s.repository.GetTournament(ctx, app.IApplicationAddress.Hex(), tournamentAddress.Hex())
	if err != nil {
		s.Logger.Error("failed to load tournament from database", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}
	if t == nil {
		t, err = s.createTournament(ctx, app, epoch, level,
			parentMatchIDHash, parentTournamentAddress, tournamentAddress)
		if err != nil {
			s.Logger.Error("failed to create new tournament", "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
			return err
		}
	} else if t.FinishedAtBlock == 0 {
		err = s.updateTournamentIfFinished(ctx, app, epoch, level, adapter, t, mostRecentBlock)
		if err != nil {
			s.Logger.Error("failed to check if tournament was finished", "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
			return err
		}
		if t.FinishedAtBlock != 0 {
			s.Logger.Info("Found finished tournament", "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", t.Address.String())
		}
	}

	nextSearchBlock := max(epoch.LastBlock, app.LastTournamentCheckBlock+1)
	var endBlock uint64
	if t.FinishedAtBlock != 0 {
		if nextSearchBlock > t.FinishedAtBlock {
			s.Logger.Debug("No new blocks to search for tournament events", "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", tournamentAddress.String(),
				"finished_at_block", t.FinishedAtBlock, "next_search_block", nextSearchBlock)
			return nil
		}
		endBlock = t.FinishedAtBlock
	} else {
		endBlock = mostRecentBlock
	}

	s.Logger.Debug("Searching for tournament events", "level", level, "application", app.Name,
		"epoch", epoch.Index, "tournament_address", tournamentAddress.String(),
		"next_search_block", nextSearchBlock, "end_block", endBlock)
	opts := &bind.FilterOpts{
		Context: ctx,
		Start:   nextSearchBlock,
		End:     &endBlock,
	}

	events, err := adapter.RetrieveAllEvents(opts)
	if err != nil {
		s.Logger.Error("failed to retrieve all events from tournament", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}

	s.Logger.Debug("Retrieved events for tournament", "level", level, "address", t.Address.String(),
		"epoch", epoch.Index,
		"commitmentJoined", len(events.CommitmentJoined),
		"matchCreated", len(events.MatchCreated),
		"matchAdvanced", len(events.MatchAdvanced),
		"matchDeleted", len(events.MatchDeleted),
		"newInnerTournament", len(events.NewInnerTournament))

	err = s.saveTournamentEvents(ctx, app, epoch, tournamentAddress, events, endBlock)
	if err != nil {
		s.Logger.Error("failed to save events for tournament", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", t.Address.String(), "error", err)
		return err
	}

	if level == BottomLevel {
		return nil // no inner tournaments
	}

	nextLevel := level + 1
	innerTournaments, _, err := getAllSubTournaments(ctx, s.repository, app.Name, epoch.Index, &tournamentAddress, level+1)
	if err != nil {
		s.Logger.Error("failed to list inner tournaments", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}

	for _, i := range innerTournaments {
		s.Logger.Debug("Fetching data for previous open tournament", "level", nextLevel,
			"parent_match_id_hash", i.ParentMatchIDHash.String(),
			"parent_tournament_address", i.ParentTournamentAddress.String(),
			"address", i.Address.String())

		if i.FinishedAtBlock != 0 {
			s.Logger.Debug("Skipping finished inner tournament", "address", i.Address.String())
			continue // already finished
		}

		err = s.fetchTournamentData(ctx, app, epoch, nextLevel, i.ParentMatchIDHash, &tournamentAddress, i.Address, mostRecentBlock)
		if err != nil {
			s.Logger.Error("failed to fetch tournament data", "level", nextLevel, "application", app.Name,
				"tournament", i.Address.String(), "error", err)
			return err
		}
	}

	for _, newInner := range events.NewInnerTournament {
		hashID := (common.Hash)(newInner.MatchIdHash)
		childAddress := newInner.ChildTournament

		s.Logger.Info("NewInnerTournament event", "id_hash", hashID.String(), "tournament_address", childAddress.String())

		err = s.fetchTournamentData(ctx, app, epoch, nextLevel, &hashID, &tournamentAddress, childAddress, mostRecentBlock)
		if err != nil {
			s.Logger.Error("failed to fetch tournament data", "level", nextLevel, "application", app.Name,
				"tournament", childAddress.String(), "error", err)
			return err
		}
	}

	return nil
}

func (s *Service) trySettle(ctx context.Context, app *Application, mostRecentBlock uint64) error {
	if _, exist := s.currentEpochIndex[app.ID]; !exist {
		s.currentEpochIndex[app.ID] = 0
	}
	currentEpochIndex := s.currentEpochIndex[app.ID]

	if tx, joinTxIsInFlight := s.joinInFlight[app.ID]; joinTxIsInFlight {
		s.Logger.Debug("Waiting for join tournament transaction to be mined", "application", app.Name,
			"epoch_index", currentEpochIndex, "tx", tx)
		return nil // wait for join to be mined before settling
	}

	if tx, settleTxIsInFlight := s.settleInFlight[app.ID]; settleTxIsInFlight {
		_, isPending, err := s.client.TransactionByHash(ctx, *tx)
		if err != nil {
			s.Logger.Error("failed to fetch last settle transaction status", "application", app.Name,
				"epoch_index", currentEpochIndex, "tx", tx, "error", err)
			return err
		}
		if isPending {
			s.Logger.Debug("Previous settle transaction is still pending", "application", app.Name,
				"epoch_index", currentEpochIndex, "tx", tx)
			return nil
		}
		s.Logger.Debug("Previous settle transaction has been mined", "application", app.Name,
			"epoch_index", currentEpochIndex, "tx", tx)
		delete(s.settleInFlight, app.ID)
		// Return so that the next tick's checkEpochs syncs the EpochSealed
		// event before we re-check CanSettle. Without this, a stale
		// mostRecentBlock snapshot could cause CanSettle to return true
		// and trigger a duplicate Settle that reverts on-chain.
		return nil
	}

	consensus, err := s.adapterFactory.CreateDaveConsensusAdapter(app.IConsensusAddress)
	if err != nil {
		s.Logger.Error("failed to bind dave consensus contract", "application", app.Name,
			"consensus_address", app.IConsensusAddress.String(), "error", err)
		return err
	}

	callOpts := &bind.CallOpts{
		Context:     ctx,
		BlockNumber: new(big.Int).SetUint64(mostRecentBlock),
	}

	result, err := consensus.CanSettle(callOpts)
	if err != nil {
		s.Logger.Error("failed to call CanSettle on DaveConsensus", "application", app.Name,
			"consensus", app.IConsensusAddress.String(), "error", err)
		return err
	}

	currentEpochIndex = result.EpochNumber.Uint64()
	s.currentEpochIndex[app.ID] = currentEpochIndex

	if !result.IsFinished {
		s.Logger.Debug("Epoch root tournament has not finished yet. Skipping Settle",
			"application", app.Name, "epoch_index", currentEpochIndex)
		return nil // nothing to do
	}

	epoch, err := s.repository.GetEpoch(ctx, app.IApplicationAddress.Hex(), currentEpochIndex)
	if err != nil {
		s.Logger.Error("failed to list epochs", "application", app.Name, "error", err)
		return err
	}
	if epoch == nil || epoch.Status != EpochStatus_ClaimComputed {
		s.Logger.Info("Application sync has not finished. Skipping Settle", "application", app.Name,
			"epoch_index", currentEpochIndex)
		return nil // nothing to do
	}

	if epoch.OutputsMerkleRoot == nil || epoch.OutputsMerkleProof == nil {
		return s.setApplicationInoperable(ctx, app,
			"epoch %d has missing required fields for settlement", epoch.Index)
	}

	s.Logger.Info("Sending Settle transaction", "application", app.Name, "epoch_index", epoch.Index,
		"outputs_merkle_root", epoch.OutputsMerkleRoot.String())

	tx, err := consensus.Settle(s.txOpts, result.EpochNumber,
		*epoch.OutputsMerkleRoot, hashSliceToByteSlice(epoch.OutputsMerkleProof))
	if err != nil {
		s.Logger.Error("failed to send Settle transaction", "application", app.Name,
			"epoch_index", result.EpochNumber.Uint64(), "error", err)
		return err
	}
	settleTx := tx.Hash()
	s.settleInFlight[app.ID] = &settleTx
	s.publisher.Publish(ctx, events.Notification{
		Channel:       events.ChannelSettleSubmitted,
		ApplicationID: app.ID,
	})

	return nil
}

func (s *Service) reactToTournament(ctx context.Context, app *Application, mostRecentBlock uint64) error {
	currentEpochIndex, exist := s.currentEpochIndex[app.ID]
	if !exist {
		errMsg := "current epoch index not found for application. Should not happen"
		s.Logger.Error(errMsg, "application", app.Name)
		return errors.New(errMsg)
	}
	if tx, settleTxIsInFlight := s.settleInFlight[app.ID]; settleTxIsInFlight {
		s.Logger.Debug("Waiting for settle transaction to be mined", "application", app.Name,
			"epoch_index", currentEpochIndex, "tx", tx)
		return nil // wait for settle to be mined
	}

	if tx, joinTxIsInFlight := s.joinInFlight[app.ID]; joinTxIsInFlight {
		_, isPending, err := s.client.TransactionByHash(ctx, *tx)
		if err != nil {
			s.Logger.Error("failed to fetch last join tournament transaction status", "application", app.Name,
				"epoch_index", currentEpochIndex, "tx", tx, "error", err)
			return err
		}
		if isPending {
			s.Logger.Debug("Previous join tournament transaction is still pending", "application", app.Name,
				"epoch_index", currentEpochIndex, "tx", tx)
			return nil
		}
		s.Logger.Debug("Previous join tournament transaction has been mined", "application", app.Name,
			"epoch_index", currentEpochIndex, "tx", tx)
		delete(s.joinInFlight, app.ID)
		// Return so that the next tick's checkEpochs syncs the CommitmentJoined
		// event before we re-check GetCommitment. Without this, a stale
		// mostRecentBlock snapshot could cause GetCommitment to return nil
		// and trigger a duplicate JoinTournament that reverts on-chain.
		return nil
	}

	epoch, err := s.repository.GetEpoch(ctx, app.IApplicationAddress.Hex(), currentEpochIndex)
	if err != nil {
		s.Logger.Error("failed to list epochs", "application", app.Name, "error", err)
		return err
	}
	if epoch == nil || epoch.Status != EpochStatus_ClaimComputed {
		s.Logger.Debug("Application sync has not finished. Skipping join tournament", "application", app.Name,
			"epoch_index", currentEpochIndex)
		return nil // nothing to do
	}

	if epoch.TournamentAddress == nil || epoch.Commitment == nil ||
		epoch.MachineHash == nil || epoch.CommitmentProof == nil {
		return s.setApplicationInoperable(ctx, app,
			"epoch %d has missing required fields for tournament reaction", epoch.Index)
	}

	commitment, err := s.repository.GetCommitment(ctx, app.IApplicationAddress.Hex(), epoch.Index,
		epoch.TournamentAddress.Hex(), epoch.Commitment.String())
	if err != nil {
		s.Logger.Error("failed to get commitment from repository", "application", app.Name,
			"epoch_index", currentEpochIndex, "tournament", epoch.TournamentAddress.Hex(),
			"commitment", epoch.Commitment.Hex(), "error", err)
		return err
	}
	if commitment != nil {
		s.Logger.Debug("Commitment already joined. Skipping JoinTournament", "application", app.Name,
			"epoch_index", currentEpochIndex, "tournament", epoch.TournamentAddress.Hex(), "commitment", epoch.Commitment.Hex())
		return nil
	}

	tournamentAdapter, err := s.adapterFactory.CreateTournamentAdapter(*epoch.TournamentAddress)
	if err != nil {
		s.Logger.Error("failed to create tournament adapter", "application", app.Name,
			"tournament", epoch.TournamentAddress.String(), "error", err)
		return err
	}

	callOpts := &bind.CallOpts{
		Context:     ctx,
		BlockNumber: new(big.Int).SetUint64(mostRecentBlock),
	}
	bondValue, err := tournamentAdapter.BondValue(callOpts)
	if err != nil {
		s.Logger.Error("failed to fetch tournament bond value", "application", app.Name,
			"epoch_index", currentEpochIndex, "tournament", epoch.TournamentAddress.Hex(),
			"error", err)
		return err
	}

	txOptsWithValue := *s.txOpts
	txOptsWithValue.Value = bondValue

	// FIXME move this to constants
	idx := uint64(1<<48) - 1 //nolint: mnd
	leftNode, rightNode, err := merkle.RootChildrenFromProof(*epoch.MachineHash, epoch.CommitmentProof, idx)
	if err != nil {
		s.Logger.Error("failed to compute left and right nodes from commitment proof",
			"application", app.Name, "epoch_index", currentEpochIndex, "error", err)
		return err
	}

	s.Logger.Info("Joining tournament", "application", app.Name, "epoch_index", epoch.Index,
		"commitment", epoch.Commitment, "left_node", leftNode.String(), "right_node", rightNode.String())

	tx, err := tournamentAdapter.JoinTournament(&txOptsWithValue, *epoch.MachineHash,
		hashSliceToByteSlice(epoch.CommitmentProof), leftNode, rightNode)
	if err != nil {
		s.Logger.Error("failed to send join tournament transaction", "application", app.Name,
			"epoch_index", currentEpochIndex, "error", err)
		return err
	}
	joinTx := tx.Hash()
	s.joinInFlight[app.ID] = &joinTx
	s.publisher.Publish(ctx, events.Notification{
		Channel:       events.ChannelJoinSubmitted,
		ApplicationID: app.ID,
	})

	return nil
}

func (s *Service) validateApplication(ctx context.Context, app *Application) error {
	s.Logger.Debug("Syncing PRT tournaments", "application", app.Name)
	mostRecentBlock, err := s.client.BlockNumber(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch latest block number", "application", app.Name, "error", err)
		return err
	}
	err = s.checkEpochs(ctx, app, mostRecentBlock)
	if err != nil {
		return err
	}
	if s.submissionEnabled {
		err = s.trySettle(ctx, app, mostRecentBlock)
		if err != nil {
			return err
		}
		err = s.reactToTournament(ctx, app, mostRecentBlock)
		if err != nil {
			return err
		}
	}
	return nil
}
