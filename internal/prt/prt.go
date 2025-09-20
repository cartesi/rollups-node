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

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
)

type prtRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter,
		p repository.Pagination, descending bool) ([]*Application, uint64, error)
	UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error

	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter,
		p repository.Pagination, descending bool) ([]*Epoch, uint64, error)
	UpdateEpoch(ctx context.Context, nameOrAddress string, e *Epoch) error
	UpdateEpochStatus(ctx context.Context, nameOrAddress string, e *Epoch) error

	CreateTournament(ctx context.Context, nameOrAddress string, t *Tournament) error

	CreateCommitment(ctx context.Context, nameOrAddress string, c *Commitment) error
	UpdateMatch(ctx context.Context, nameOrAddress string, m *Match) error
	CreateMatch(ctx context.Context, nameOrAddress string, m *Match) error
	CreateMatchAdvanced(ctx context.Context, nameOrAddress string, m *MatchAdvanced) error

	StoreTournamentEvents(ctx context.Context, appID int64, commitments []*Commitment, matches []*Match,
		matchAdvanced []*MatchAdvanced, matchDeleted []*Match, lastBlock uint64) error

	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)
}

// EthClientInterface defines the methods we need from ethclient.Client
type EthClientInterface interface {
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	ChainID(ctx context.Context) (*big.Int, error)
}

func getAllRunningApplications(ctx context.Context, r prtRepository) ([]*Application, uint64, error) {
	f := repository.ApplicationFilter{State: Pointer(ApplicationState_Enabled), ConsensusType: Pointer(Consensus_PRT)}
	return r.ListApplications(ctx, f, repository.Pagination{}, false)
}

func getAllClaimComputedEpochs(ctx context.Context, r prtRepository, nameOrAddress string) ([]*Epoch, uint64, error) {
	f := repository.EpochFilter{Status: Pointer(EpochStatus_ClaimComputed)}
	return r.ListEpochs(ctx, nameOrAddress, f, repository.Pagination{}, false)
}

// setApplicationInoperable marks an application as inoperable with the given reason,
// logs any error that occurs during the update, and returns an error with the reason.
func (s *Service) setApplicationInoperable(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	reason := fmt.Sprintf(reasonFmt, args...)
	appAddress := app.IApplicationAddress.String()

	// Log the reason first
	s.Logger.Error(reason, "application", appAddress)

	// Update application state
	err := s.repository.UpdateApplicationState(ctx, app.ID, ApplicationState_Inoperable, &reason)
	if err != nil {
		s.Logger.Error("failed to update application state to inoperable", "app", appAddress, "err", err)
	}

	// Return the error with the reason
	return errors.New(reason)
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

func (s *Service) checkFinalizedEpochs(ctx context.Context, app *Application) error {
	epochs, _, err := getAllClaimComputedEpochs(ctx, s.repository, app.Name)
	if err != nil {
		s.Logger.Error("failed to list epochs", "application", app.Name, "error", err)
		return err
	}
	if len(epochs) == 0 {
		return nil // nothing to do
	}

	// TODO: use adapters instead of direct contract calls
	// Type assertion to get the concrete client if possible
	ethClient, ok := s.client.(*ethclient.Client)
	if !ok {
		return fmt.Errorf("client is not an *ethclient.Client, cannot create dave consensus bind")
	}

	consensus, err := idaveconsensus.NewIDaveConsensus(app.IConsensusAddress, ethClient)
	if err != nil {
		s.Logger.Error("failed to bind dave consensus contract", "application", app.Name,
			"consensus_address", app.IConsensusAddress.String(), "error", err)
		return err
	}

	for _, epoch := range epochs {
		if epoch.ClaimTransactionHash == nil { // epoch not claimed on-chain yet
			break
		}
		receipt, err := ethClient.TransactionReceipt(ctx, *epoch.ClaimTransactionHash)
		if err != nil {
			s.Logger.Error("failed to fetch transaction receipt for epoch", "application", app.Name,
				"epoch", epoch.Index, "tx", epoch.ClaimTransactionHash, "error", err)
			return err
		}

		if receipt.Status != 1 {
			return fmt.Errorf("EpochSealed transaction hash points to failed transaction")
		}

		var event *idaveconsensus.IDaveConsensusEpochSealed
		for _, vLog := range receipt.Logs {
			event, err = consensus.ParseEpochSealed(*vLog)
			if err != nil {
				continue // Skip logs that don't match
			}
		}
		if event == nil {
			return fmt.Errorf("failed to find EpochSealed event in receipt logs")

		}

		if epoch.Index != event.EpochNumber.Uint64()-1 {
			return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent index between off-chain (%d) and on-chain (%d)",
				epoch.Index, epoch.Index, event.EpochNumber.Uint64()-1)
		}
		if *epoch.MachineHash != event.InitialMachineStateHash {
			return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent machine hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.MachineHash.String(), hexutil.Encode(event.InitialMachineStateHash[:]))
		}
		if *epoch.ClaimHash != event.OutputsMerkleRoot {
			return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent claim hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.ClaimHash.String(), hexutil.Encode(event.OutputsMerkleRoot[:]))
		}

		err = s.fetchTournamentData(ctx, app, epoch, RootLevel, nil, nil, *epoch.TournamentAddress)
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
) error {
	s.Logger.Info("Fetching "+level.String()+" tournament data", "application", app.Name, "tournament", tournamentAddress.String())
	// TODO: use adapters instead of direct contract calls
	// Type assertion to get the concrete client if possible
	ethClient, ok := s.client.(*ethclient.Client)
	if !ok {
		return fmt.Errorf("client is not an *ethclient.Client, cannot create dave consensus bind")
	}

	adapter, err := NewITournamentAdapter(tournamentAddress, ethClient, s.filter)
	if err != nil {
		s.Logger.Error("failed to create "+level.String()+" tournament adapter", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}

	constants, err := adapter.Constants(nil)
	if err != nil {
		s.Logger.Error("failed to fetch "+level.String()+" tournament constants", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}

	finished, timeFinished, err := adapter.TimeFinished(nil)
	if err != nil {
		s.Logger.Error("failed to fetch "+level.String()+" tournament finished at time", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}
	if !finished {
		s.Logger.Error(level.String()+" tournament should be finished", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}

	_, winnerCommitment, finalState, err := adapter.Result(nil)
	if err != nil {
		s.Logger.Error("failed to fetch "+level.String()+" tournament result", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}

	if level == RootLevel && *epoch.Commitment != winnerCommitment {
		return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent commitment between off-chain (%s) and on-chain (%s)",
			epoch.Index, epoch.Commitment.String(), hexutil.Encode(winnerCommitment[:]))
	}

	t := Tournament{
		ApplicationID:           app.ID,
		EpochIndex:              epoch.Index,
		Address:                 tournamentAddress,
		ParentMatchIDHash:       parentMatchIDHash,
		ParentTournamentAddress: parentTournamentAddress,
		MaxLevel:                constants.MaxLevel,
		Level:                   constants.Level,
		Log2Step:                constants.Log2step,
		Height:                  constants.Height,
		WinnerCommitment:        (*common.Hash)(&winnerCommitment),
		FinalStateHash:          (*common.Hash)(&finalState),
		FinishedAtBlock:         timeFinished,
	}

	err = s.repository.CreateTournament(ctx, app.IApplicationAddress.Hex(), &t)
	if err != nil {
		s.Logger.Error("failed to create "+level.String()+" tournament in database", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}

	opts := &bind.FilterOpts{
		Context: ctx,
		Start:   epoch.LastBlock,
		End:     &timeFinished, // To latest block
	}

	events, err := adapter.RetrieveAllEvents(opts)
	if err != nil {
		s.Logger.Error("failed to retrieve all events from "+level.String()+" tournament", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return err
	}

	// Print summary of events found
	s.Logger.Info("Retrieved events for "+level.String()+" tournament", "address", t.Address.String(),
		"commitmentJoined", len(events.CommitmentJoined),
		"matchCreated", len(events.MatchCreated),
		"matchAdvanced", len(events.MatchAdvanced),
		"matchDeleted", len(events.MatchDeleted),
		"newInnerTournament", len(events.NewInnerTournament))

	err = s.saveTournamentEvents(ctx, app, epoch, tournamentAddress, events, t.FinishedAtBlock)
	if err != nil {
		s.Logger.Error("failed to save events for "+level.String()+" tournament", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", t.Address.String(), "error", err)
		return err
	}

	for _, newInner := range events.NewInnerTournament {
		hashID := (common.Hash)(newInner.MatchIdHash)
		childAddress := newInner.ChildTournament

		s.Logger.Info("NewInnerTournament event", "id_hash", hashID.String(), "tournament_address", childAddress.String())

		nextLevel := level + 1
		if nextLevel > BottomLevel {
			return fmt.Errorf("unexpected tournament level")
		}

		err = s.fetchTournamentData(ctx, app, epoch, nextLevel, &hashID, &tournamentAddress, childAddress)
		if err != nil {
			s.Logger.Error("failed to fetch "+TournamentLevel(nextLevel).String()+" tournament data", "application", app.Name,
				"tournament", childAddress.String(), "error", err)
			return err
		}
	}

	return nil
}

func (s *Service) validateApplication(ctx context.Context, app *Application) error {
	s.Logger.Debug("Syncing PTR tournaments", "application", app.Name)
	return s.checkFinalizedEpochs(ctx, app)
}
