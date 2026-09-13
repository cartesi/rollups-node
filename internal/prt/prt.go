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
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

type prtRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter,
		p repository.Pagination, descending bool) ([]*Application, uint64, error)
	UpdateApplicationStatus(ctx context.Context, appID int64, status ApplicationStatus, reason *string) error
	HasUndrainedEpochsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)
	HasUnreconciledClaimsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)
	UpdateEpochWithForeclosedClaim(ctx context.Context, applicationID int64, index uint64) error

	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter,
		p repository.Pagination, descending bool) ([]*Epoch, uint64, error)
	GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	UpdateEpochReconciledStaged(ctx context.Context, applicationID int64, index uint64, stagedAtBlock uint64) error
	UpdateEpochWithAcceptedClaim(ctx context.Context, applicationID int64, index uint64, txHash *common.Hash) error

	GetTournament(ctx context.Context, nameOrAddress string, address string) (*Tournament, error)
	ListTournaments(ctx context.Context, nameOrAddress string, f repository.TournamentFilter,
		p repository.Pagination, descending bool) ([]*Tournament, uint64, error)

	StoreTournamentEvents(ctx context.Context, appID int64, batches []*repository.TournamentEventBatch, lastBlock uint64) error

	GetCommitment(ctx context.Context, nameOrAddress string, epochIndex uint64,
		tournamentAddress string, commitmentHex string) (*Commitment, error)
	ListCommitments(ctx context.Context, nameOrAddress string, filter repository.CommitmentFilter,
		pagination repository.Pagination, descending bool) ([]*Commitment, uint64, error)
	ListMatches(ctx context.Context, nameOrAddress string, filter repository.MatchFilter,
		pagination repository.Pagination, descending bool) ([]*Match, uint64, error)

	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)
}

// EthClientInterface defines the methods we need from ethclient.Client
type EthClientInterface interface {
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	ChainID(ctx context.Context) (*big.Int, error)
	BlockNumber(ctx context.Context) (uint64, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
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

func getObservableApplications(ctx context.Context, r prtRepository) ([]*Application, uint64, error) {
	filter := repository.ApplicationFilter{
		Enabled:       new(true),
		ConsensusType: new(Consensus_PRT),
	}
	return r.ListApplications(ctx, filter, repository.Pagination{}, false)
}

func getTournamentObservationEpochs(ctx context.Context, r prtRepository, nameOrAddress string) ([]*Epoch, uint64, error) {
	f := repository.EpochFilter{HasTournament: new(true)}
	return r.ListEpochs(ctx, nameOrAddress, f, repository.Pagination{}, false)
}

// getDefaultBlockNumber selects the block used for stored chain state. Live
// transaction checks use a separate latest block and cannot advance this view.
func (s *Service) getDefaultBlockNumber(ctx context.Context) (uint64, error) {
	var tag rpc.BlockNumber
	switch s.defaultBlock {
	case DefaultBlock_Pending:
		tag = rpc.PendingBlockNumber
	case DefaultBlock_Latest:
		tag = rpc.LatestBlockNumber
	case DefaultBlock_Finalized:
		tag = rpc.FinalizedBlockNumber
	case DefaultBlock_Safe:
		tag = rpc.SafeBlockNumber
	default:
		return 0, fmt.Errorf("default block %v not supported", s.defaultBlock)
	}
	header, err := s.client.HeaderByNumber(ctx, big.NewInt(tag.Int64()))
	if err != nil {
		return 0, fmt.Errorf("fetching %s block header: %w", tag, err)
	}
	if header == nil {
		return 0, fmt.Errorf("returned %s block header is nil", tag)
	}
	return checkedUint64(header.Number, "configured block number")
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

func (s *Service) setApplicationDiverged(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	return appstatus.SetDivergedf(ctx, s.Logger, s.repository, app, reasonFmt, args...)
}

func (s *Service) setApplicationCorrupted(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	return appstatus.SetCorruptedf(ctx, s.Logger, s.repository, app, reasonFmt, args...)
}

// setApplicationFailed marks the app FAILED (recoverable by operator action).
// Like appstatus.SetFailedf, it returns nil on success.
func (s *Service) setApplicationFailed(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	return appstatus.SetFailedf(ctx, s.Logger, s.repository, app, reasonFmt, args...)
}

func (s *Service) readTournament(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
	level TournamentLevel,
	parentMatchIDHash *common.Hash,
	parentTournamentAddress *common.Address,
	tournamentAddress common.Address,
	adapter TournamentAdapter,
	levelCount uint64,
	mostRecentBlock uint64,
) (*Tournament, error) {
	callOpts := pinnedCallOpts(ctx, mostRecentBlock)
	descriptor, err := adapter.Descriptor(callOpts)
	if err != nil {
		s.logErrorUnlessShutdown("failed to fetch tournament descriptor", err, "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String())
		return nil, err
	}
	if err := validateTournamentDescriptor(descriptor, level, levelCount); err != nil {
		return nil, fmt.Errorf("tournament %s: %w", tournamentAddress, err)
	}
	if descriptor.StartInstant > mostRecentBlock {
		return nil, fmt.Errorf("tournament %s starts at block %d after observed block %d",
			tournamentAddress, descriptor.StartInstant, mostRecentBlock)
	}
	baseCycle, err := Uint256FromBig(descriptor.BaseCycle)
	if err != nil {
		return nil, fmt.Errorf("tournament %s base cycle: %w", tournamentAddress, err)
	}

	t := &Tournament{
		ApplicationID:           app.ID,
		EpochIndex:              epoch.Index,
		Address:                 tournamentAddress,
		ParentMatchIDHash:       parentMatchIDHash,
		ParentTournamentAddress: parentTournamentAddress,
		MaxLevel:                levelCount,
		Level:                   descriptor.Level,
		Log2Step:                descriptor.Log2Stride,
		Height:                  descriptor.Height,
		InitialHash:             descriptor.InitialHash,
		BaseCycle:               baseCycle,
		Kind:                    descriptor.Kind,
		StartInstant:            descriptor.StartInstant,
		Allowance:               descriptor.Allowance,
	}
	if err := s.updateTournamentStanding(ctx, app, epoch, level, adapter, t, mostRecentBlock); err != nil {
		return nil, err
	}
	if t.Snapshot.FinishedAtBlock == 0 {
		s.Logger.Info("Found open tournament", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String())
	}
	return t, nil
}

func (s *Service) logFailedRootTournament(app *Application, epochIndex uint64, tournament common.Address) {
	s.Logger.Warn("Root tournament finished without a winner; this consensus cannot advance. "+
		"Ask the application guardian to consider foreclosure",
		"application", app.Name, "epoch_index", epochIndex, "tournament", tournament)
}

func validateTournamentDescriptor(descriptor TournamentDescriptor, expectedLevel TournamentLevel, levelCount uint64) error {
	if levelCount == 0 {
		return errors.New("tournament level count is zero")
	}
	if descriptor.BaseCycle == nil || descriptor.BaseCycle.Sign() < 0 {
		return errors.New("tournament descriptor has invalid base cycle")
	}
	if descriptor.Level != uint64(expectedLevel) {
		return fmt.Errorf("tournament descriptor level %d does not match expected level %d",
			descriptor.Level, expectedLevel)
	}
	if descriptor.Level >= levelCount {
		return fmt.Errorf("tournament descriptor level %d is outside level count %d",
			descriptor.Level, levelCount)
	}
	if descriptor.Kind != TournamentKindLeaf && descriptor.Kind != TournamentKindNonLeaf {
		return fmt.Errorf("tournament descriptor has unknown kind %s", descriptor.Kind)
	}
	expectedKind := TournamentKindNonLeaf
	if descriptor.Level+1 == levelCount {
		expectedKind = TournamentKindLeaf
	}
	if descriptor.Kind != expectedKind {
		return fmt.Errorf("tournament descriptor level %d has kind %s, expected %s",
			descriptor.Level, descriptor.Kind, expectedKind)
	}
	return nil
}

func validateTournamentStanding(standing TournamentStanding, mostRecentBlock uint64) error {
	switch standing.State {
	case TournamentStandingMatchesActive, TournamentStandingAwaitingClosure,
		TournamentStandingRootWinner, TournamentStandingRootFailed, TournamentStandingInnerWinner,
		TournamentStandingInnerEliminableNoWinner, TournamentStandingInnerEliminableWinnerExpired:
	default:
		return fmt.Errorf("tournament has unknown standing %s", standing.State)
	}
	if isTerminalTournamentStanding(standing.State) {
		if standing.FinishedAt == 0 {
			return errors.New("terminal tournament has zero finished block")
		}
		if standing.FinishedAt > mostRecentBlock {
			return fmt.Errorf("tournament finished at block %d after observed block %d",
				standing.FinishedAt, mostRecentBlock)
		}
	}
	return nil
}

func isTerminalTournamentStanding(state TournamentStandingState) bool {
	switch state {
	case TournamentStandingRootWinner, TournamentStandingRootFailed, TournamentStandingInnerWinner,
		TournamentStandingInnerEliminableNoWinner, TournamentStandingInnerEliminableWinnerExpired:
		return true
	case TournamentStandingMatchesActive, TournamentStandingAwaitingClosure:
		return false
	default:
		return false
	}
}

func (s *Service) refreshTournament(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
	level TournamentLevel,
	adapter TournamentAdapter,
	t *Tournament,
	mostRecentBlock uint64,
) error {
	callOpts := pinnedCallOpts(ctx, mostRecentBlock)

	descriptor, err := adapter.Descriptor(callOpts)
	if err != nil {
		s.logErrorUnlessShutdown("failed to fetch tournament descriptor", err, "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", t.Address.String())
		return err
	}
	if err := validateTournamentDescriptor(descriptor, level, t.MaxLevel); err != nil {
		return fmt.Errorf("tournament %s: %w", t.Address, err)
	}
	if t.Level != descriptor.Level || t.Log2Step != descriptor.Log2Stride || t.Height != descriptor.Height {
		return fmt.Errorf("tournament %s descriptor does not match stored geometry", t.Address)
	}
	if t.InitialHash != descriptor.InitialHash || t.BaseCycle.ToBig().Cmp(descriptor.BaseCycle) != 0 ||
		t.Kind != descriptor.Kind || t.StartInstant != descriptor.StartInstant || t.Allowance != descriptor.Allowance {
		return fmt.Errorf("tournament %s descriptor does not match stored configuration", t.Address)
	}
	return s.updateTournamentStanding(ctx, app, epoch, level, adapter, t, mostRecentBlock)
}

// updateTournamentStanding applies the current chain view. A prior winner can
// expire, so a finished tournament must still be refreshed.
func (s *Service) updateTournamentStanding(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
	level TournamentLevel,
	adapter TournamentAdapter,
	t *Tournament,
	mostRecentBlock uint64,
) error {
	callOpts := pinnedCallOpts(ctx, mostRecentBlock)
	standing, err := adapter.Standing(callOpts)
	if err != nil {
		s.logErrorUnlessShutdown("failed to fetch tournament standing", err, "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", t.Address.String())
		return err
	}
	if err := validateTournamentStanding(standing, mostRecentBlock); err != nil {
		return fmt.Errorf("tournament %s: %w", t.Address, err)
	}
	if standing.State == TournamentStandingMatchesActive {
		s.warnUnsupportedDispute(app, epoch.Index, t.Address)
	}
	if level == RootLevel && standing.State == TournamentStandingRootFailed {
		s.logFailedRootTournament(app, epoch.Index, t.Address)
	}
	snapshot := TournamentSnapshot{
		AsOfBlock: mostRecentBlock, Standing: standing.State, AcceptsJoins: standing.AcceptsJoins,
		FinishedAtBlock: standing.FinishedAt,
	}
	if standing.HasCandidate {
		snapshot.Candidate = new(standing.Candidate)
	}
	if standing.State == TournamentStandingRootWinner || standing.State == TournamentStandingInnerWinner {
		snapshot.WinnerCommitment = new(standing.Candidate)
		snapshot.FinalStateHash = new(standing.FinalState)
	}
	if standing.State == TournamentStandingInnerWinner {
		snapshot.ParentCommitment = new(standing.ParentCommitment)
		snapshot.WinnerExpiresAt = standing.WinnerExpiresAt
	}
	if level != RootLevel {
		result, err := adapter.InnerResult(callOpts)
		if err != nil {
			s.logErrorUnlessShutdown("failed to fetch inner tournament result", err,
				"application", app.Name, "tournament", t.Address)
			return err
		}
		snapshot.InnerResult = &TournamentInnerResult{Disposition: result.Disposition}
		switch result.Disposition {
		case InnerTournamentWinner:
			snapshot.InnerResult.ParentCommitment = new(result.ParentCommitment)
			snapshot.InnerResult.PausedAllowance = result.PausedAllowance
		case InnerTournamentUnsettled, InnerTournamentEliminable:
		default:
			return fmt.Errorf("tournament %s has unknown inner disposition %s", t.Address, result.Disposition)
		}
	}
	recovery, err := adapter.BondRecovery(callOpts)
	if err != nil {
		s.logErrorUnlessShutdown("failed to fetch tournament bond recovery", err,
			"application", app.Name, "tournament", t.Address)
		return err
	}
	snapshot.BondRecovery.Disposition = recovery.Disposition
	switch recovery.Disposition {
	case BondDispositionRecoverable:
		payment, err := Uint256FromBig(recovery.Payment)
		if err != nil {
			return fmt.Errorf("tournament %s bond payment: %w", t.Address, err)
		}
		snapshot.BondRecovery.Claimer = new(recovery.Claimer)
		snapshot.BondRecovery.Payment = &payment
	case BondDispositionTournamentRunning, BondDispositionNoWinner, BondDispositionRecovered:
	default:
		return fmt.Errorf("tournament %s has unknown bond disposition %s", t.Address, recovery.Disposition)
	}
	t.Snapshot = snapshot
	return nil
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
			epoch.MachineHash == nil || epoch.TxBufferDataBlock == nil {
			return s.setApplicationCorrupted(ctx, app,
				"epoch %d has missing required fields for ClaimComputed status", epoch.Index)
		}

		if epoch.ClaimTransactionHash == nil { // epoch not claimed on-chain yet
			err = s.fetchTournamentData(ctx, app, epoch, RootLevel, nil, nil, *epoch.TournamentAddress, mostRecentBlock)
			if err != nil {
				s.logErrorUnlessShutdown("failed to fetch root tournament data", err,
					"application", app.Name, "epoch", epoch.Index,
					"tournament", epoch.TournamentAddress.String())
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
			return s.setApplicationDiverged(ctx, app, "Epoch %d has inconsistent index between off-chain (%d) and on-chain (%d)",
				epoch.Index, epoch.Index, event.EpochNumber.Uint64()-1)
		}
		if *epoch.MachineHash != event.InitialMachineStateHash {
			return s.setApplicationDiverged(ctx, app, "Epoch %d has inconsistent machine hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.MachineHash.String(), hexutil.Encode(event.InitialMachineStateHash[:]))
		}
		if *epoch.TxBufferDataBlock != event.OutputsMerkleRoot {
			return s.setApplicationDiverged(ctx, app, "Epoch %d has inconsistent claim hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.TxBufferDataBlock.String(), hexutil.Encode(event.OutputsMerkleRoot[:]))
		}

		err = s.fetchTournamentData(ctx, app, epoch, RootLevel, nil, nil, *epoch.TournamentAddress, mostRecentBlock)
		if err != nil {
			s.logErrorUnlessShutdown("failed to fetch tournament data", err,
				"application", app.Name, "epoch", epoch.Index,
				"tournament", epoch.TournamentAddress.String())
			return err
		}

		s.Logger.Info("Found finalized epoch. OutputsMerkleRoot matched. Setting claim as accepted",
			"application", app.Name,
			"epoch", epoch.Index,
			"event_block_number", event.Raw.BlockNumber,
			"outputs_merkle_root", fmt.Sprintf("%x", event.OutputsMerkleRoot),
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

// observeApplicationTournaments indexes chain facts without consulting local
// claim readiness or changing application health. The returned roots are also
// used by the separate local claim reconciliation after publication.
func (s *Service) observeApplicationTournaments(
	ctx context.Context, app *Application, mostRecentBlock uint64,
) ([]*Epoch, DaveConsensusAdapter, error) {
	epochs, _, err := getTournamentObservationEpochs(ctx, s.repository, app.Name)
	if err != nil {
		s.logErrorUnlessShutdown("failed to list epochs", err, "application", app.Name)
		return nil, nil, err
	}
	if len(epochs) == 0 {
		s.Logger.Debug("No tournament roots to observe", "application", app.Name)
		return epochs, nil, nil
	}

	consensus, err := s.adapterFactory.CreateDaveConsensusAdapter(app.IConsensusAddress)
	if err != nil {
		s.Logger.Error("failed to bind dave consensus contract", "application", app.Name,
			"consensus_address", app.IConsensusAddress.String(), "error", err)
		return nil, nil, err
	}
	windowEnd := min(mostRecentBlock, app.LastEpochCheckBlock)
	if app.ForecloseBlock != 0 && app.LastEpochCheckBlock >= app.ForecloseBlock {
		// Foreclosure ends root creation, not the existing tournament trees.
		// Once every pre-foreclosure root is known, their events remain live.
		windowEnd = mostRecentBlock
	}

	if windowEnd > app.LastTournamentCheckBlock {
		if err := s.observeTournamentWindow(ctx, app, epochs, consensus, windowEnd); err != nil {
			s.recordTournamentObservationFailure(app, mostRecentBlock, windowEnd, err)
			return nil, nil, err
		}
		s.clearTournamentObservationFailure(app.ID)
	}

	return epochs, consensus, nil
}

// observeTournamentWindow publishes projections, events, and their cursor in
// one transaction. Acceptance reconciliation and transactions are not part of
// this operation or its readiness signal.
func (s *Service) observeTournamentWindow(
	ctx context.Context, app *Application, epochs []*Epoch, consensus DaveConsensusAdapter, windowEnd uint64,
) error {
	var roots []*Epoch
	for _, epoch := range epochs {
		if epoch.TournamentAddress != nil && epoch.LastBlock <= windowEnd {
			roots = append(roots, epoch)
		}
	}
	var levelCount uint64
	if len(roots) > 0 {
		var err error
		levelCount, err = consensus.TournamentLevelCount(pinnedCallOpts(ctx, windowEnd))
		if err != nil {
			return fmt.Errorf("fetching tournament level count: %w", err)
		}
		if levelCount == 0 {
			return errors.New("tournament level count is zero")
		}
	}
	var batches []*repository.TournamentEventBatch
	for _, epoch := range roots {
		rootBatches, err := s.gatherTournamentData(ctx, app, epoch, RootLevel, nil, nil,
			*epoch.TournamentAddress, levelCount, windowEnd)
		if err != nil {
			return fmt.Errorf("gathering epoch %d tournament events: %w", epoch.Index, err)
		}
		batches = append(batches, rootBatches...)
	}
	if err := s.repository.StoreTournamentEvents(ctx, app.ID, batches, windowEnd); err != nil {
		return fmt.Errorf("storing application tournament event window: %w", err)
	}
	app.LastTournamentCheckBlock = windowEnd
	return nil
}

func (s *Service) reconcileAcceptedEpochs(
	ctx context.Context, app *Application, epochs []*Epoch, consensus DaveConsensusAdapter, mostRecentBlock uint64,
) (bool, error) {
	if mostRecentBlock < app.LastTournamentCheckBlock {
		// A stored projection from a later head cannot decide local status at
		// this older configured head. Keep it and wait for the head to catch up.
		return true, nil
	}
	for _, epoch := range epochs {
		if epoch.Status != EpochStatus_ClaimComputed && epoch.Status != EpochStatus_ClaimStaged {
			continue
		}
		if epoch.TournamentAddress == nil || epoch.Commitment == nil ||
			epoch.MachineHash == nil || epoch.TxBufferDataBlock == nil {
			return true, s.setApplicationCorrupted(ctx, app,
				"epoch %d has missing required fields for pending claim processing", epoch.Index)
		}

		// Compare only after the complete root and descendant window is durable.
		// A losing local claim must not hide the on-chain winner from clients.
		tournament, err := s.repository.GetTournament(ctx, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex())
		if err != nil {
			return true, fmt.Errorf("loading epoch %d observed tournament: %w", epoch.Index, err)
		}
		if tournament == nil || tournament.Snapshot.FinishedAtBlock == 0 ||
			tournament.Snapshot.FinishedAtBlock > app.LastTournamentCheckBlock {
			return false, nil
		}
		if tournament.Snapshot.WinnerCommitment != nil && *tournament.Snapshot.WinnerCommitment != *epoch.Commitment {
			return true, s.setApplicationDiverged(ctx, app,
				"Epoch %d has inconsistent commitment between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.Commitment, tournament.Snapshot.WinnerCommitment)
		}
		if epoch.ClaimTransactionHash == nil { // no accepting EpochSealed event observed yet
			break
		}

		receipt, err := s.client.TransactionReceipt(ctx, *epoch.ClaimTransactionHash)
		if err != nil {
			s.logErrorUnlessShutdown("failed to fetch transaction receipt for epoch", err, "application", app.Name,
				"epoch", epoch.Index, "tx", epoch.ClaimTransactionHash)
			return true, err
		}
		if receipt == nil {
			return true, fmt.Errorf("epoch %d: acceptance transaction receipt is nil", epoch.Index)
		}
		if receipt.TxHash != *epoch.ClaimTransactionHash {
			return true, fmt.Errorf("epoch %d: acceptance receipt transaction hash %s differs from observed hash %s",
				epoch.Index, receipt.TxHash, epoch.ClaimTransactionHash)
		}
		receiptBlock, err := checkedUint64(receipt.BlockNumber, "acceptance receipt block")
		if err != nil {
			return true, fmt.Errorf("epoch %d: %w", epoch.Index, err)
		}
		observedBlock := min(mostRecentBlock, app.LastTournamentCheckBlock)
		if receiptBlock > observedBlock {
			s.Logger.Debug("Acceptance transaction is newer than the published tournament window",
				"application", app.Name,
				"epoch", epoch.Index,
				"tx", epoch.ClaimTransactionHash,
				"receipt_block", receiptBlock,
				"snapshot_block", observedBlock)
			return true, nil
		}

		if receipt.Status != types.ReceiptStatusSuccessful {
			return true, fmt.Errorf("epoch %d: EpochSealed transaction hash points to failed transaction", epoch.Index)
		}

		var event *idaveconsensus.IDaveConsensusEpochSealed
		expectedEventEpoch := new(big.Int).SetUint64(epoch.Index)
		expectedEventEpoch.Add(expectedEventEpoch, common.Big1)
		for _, vLog := range receipt.Logs {
			if vLog == nil || vLog.Address != app.IConsensusAddress ||
				vLog.TxHash != *epoch.ClaimTransactionHash || vLog.BlockNumber != receiptBlock {
				continue
			}
			candidate, parseErr := consensus.ParseEpochSealed(*vLog)
			if parseErr != nil || candidate == nil || candidate.EpochNumber == nil ||
				candidate.EpochNumber.Cmp(expectedEventEpoch) != 0 {
				continue // Skip logs that don't match
			}
			event = candidate
			break
		}
		if event == nil {
			return true, fmt.Errorf("epoch %d: failed to find EpochSealed event in receipt logs", epoch.Index)
		}

		if *epoch.MachineHash != event.InitialMachineStateHash {
			return true, s.setApplicationDiverged(ctx, app,
				"Epoch %d has inconsistent machine hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.MachineHash.String(), hexutil.Encode(event.InitialMachineStateHash[:]))
		}
		if *epoch.TxBufferDataBlock != event.OutputsMerkleRoot {
			return true, s.setApplicationDiverged(ctx, app,
				"Epoch %d has inconsistent claim hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.TxBufferDataBlock.String(), hexutil.Encode(event.OutputsMerkleRoot[:]))
		}

		s.Logger.Info("Found finalized epoch. OutputsMerkleRoot matched. Setting claim as accepted",
			"application", app.Name,
			"epoch", epoch.Index,
			"event_block_number", event.Raw.BlockNumber,
			"outputs_merkle_root", fmt.Sprintf("%x", event.OutputsMerkleRoot),
			"tx", epoch.ClaimTransactionHash,
		)

		if s.submissionEnabled {
			s.queueRootBondRecovery(app.ID, epoch.Index, *epoch.TournamentAddress)
		}
		err = s.repository.UpdateEpochWithAcceptedClaim(ctx, app.ID, epoch.Index, epoch.ClaimTransactionHash)
		if err != nil {
			s.logErrorUnlessShutdown("failed to update epoch status to claim accepted", err, "application", app.Name, "epoch", epoch.Index)
			return true, err
		}
	}
	return false, nil
}

// gatherTournamentData reads a complete subtree without storing projections,
// events, or cursors. The returned batches have parents before their children.
func (s *Service) gatherTournamentData(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
	level TournamentLevel,
	parentMatchIDHash *common.Hash,
	parentTournamentAddress *common.Address,
	tournamentAddress common.Address,
	levelCount uint64,
	mostRecentBlock uint64,
) ([]*repository.TournamentEventBatch, error) {
	s.Logger.Debug("Fetching tournament data", "level", level, "application", app.Name, "tournament", tournamentAddress.String())

	t, err := s.repository.GetTournament(ctx, app.IApplicationAddress.Hex(), tournamentAddress.Hex())
	if err != nil {
		s.logErrorUnlessShutdown("failed to load tournament from database", err, "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String())
		return nil, err
	}
	created := t == nil
	if !created {
		if t.MaxLevel != levelCount || t.Level != uint64(level) {
			return nil, fmt.Errorf("tournament %s database geometry does not match observed hierarchy", tournamentAddress)
		}
		if tournamentObservationComplete(t, app.LastTournamentCheckBlock, mostRecentBlock) {
			// Retirement belongs to this clone only. Its children can still
			// expire or recover bonds after the parent stops changing.
			return s.gatherInnerTournamentData(ctx, app, epoch, t, &TournamentEvents{}, mostRecentBlock)
		}
	}
	adapter, err := s.adapterFactory.CreateTournamentAdapter(tournamentAddress)
	if err != nil {
		s.Logger.Error("failed to create tournament adapter", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(), "error", err)
		return nil, err
	}
	if created {
		t, err = s.readTournament(ctx, app, epoch, level,
			parentMatchIDHash, parentTournamentAddress, tournamentAddress, adapter, levelCount, mostRecentBlock)
		if err != nil {
			s.logErrorUnlessShutdown("failed to create new tournament", err,
				"level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", tournamentAddress.String())
			return nil, err
		}
	}
	if !created {
		// Do not mutate a repository-owned projection before the window commits.
		projection := *t
		t = &projection
		previousFinish := t.Snapshot.FinishedAtBlock
		err = s.refreshTournament(ctx, app, epoch, level, adapter, t, mostRecentBlock)
		if err != nil {
			s.logErrorUnlessShutdown("failed to check if tournament was finished", err, "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", tournamentAddress.String())
			return nil, err
		}
		if previousFinish == 0 && t.Snapshot.FinishedAtBlock != 0 {
			s.Logger.Info("Found finished tournament", "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", t.Address.String())
		}
	}

	nextSearchBlock := max(t.StartInstant, app.LastTournamentCheckBlock+1)
	if created {
		// First observation must include the clone's history even if its
		// creation is below this application's existing checkpoint.
		nextSearchBlock = t.StartInstant
	}
	endBlock := mostRecentBlock
	events := &TournamentEvents{}
	if nextSearchBlock <= endBlock {
		s.Logger.Debug("Searching for tournament events", "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String(),
			"next_search_block", nextSearchBlock, "end_block", endBlock)
		opts := &bind.FilterOpts{Context: ctx, Start: nextSearchBlock, End: &endBlock}
		events, err = adapter.RetrieveAllEvents(opts)
		if err != nil {
			return nil, fmt.Errorf("retrieving tournament %s events: %w", tournamentAddress, err)
		}
		if events == nil {
			return nil, fmt.Errorf("tournament %s returned nil events", tournamentAddress)
		}
	}

	s.Logger.Debug("Retrieved events for tournament", "level", level, "address", t.Address.String(),
		"epoch", epoch.Index,
		"commitmentJoined", len(events.CommitmentJoined),
		"matchCreated", len(events.MatchCreated),
		"matchAdvanced", len(events.MatchAdvanced),
		"matchDeleted", len(events.MatchDeleted),
		"newInnerTournament", len(events.NewInnerTournament))

	previousCounts := zeroStructuralEventCounts()
	previousBondDisposition := BondDispositionTournamentRunning
	if !created && nextSearchBlock > t.StartInstant {
		previousOpts := pinnedCallOpts(ctx, nextSearchBlock-1)
		previousCounts, err = adapter.StructuralEventCounts(previousOpts)
		if err != nil {
			return nil, fmt.Errorf("reading previous tournament event counts: %w", err)
		}
		previousBond, err := adapter.BondRecovery(previousOpts)
		if err != nil {
			return nil, fmt.Errorf("reading previous tournament bond recovery: %w", err)
		}
		previousBondDisposition = previousBond.Disposition
	}
	currentCounts, err := adapter.StructuralEventCounts(pinnedCallOpts(ctx, endBlock))
	if err != nil {
		return nil, fmt.Errorf("reading current tournament event counts: %w", err)
	}
	if err := validateTournamentEventCounts(previousCounts, currentCounts, events); err != nil {
		return nil, fmt.Errorf("tournament %s: %w", tournamentAddress, err)
	}
	if err := validateTournamentFinancialEvents(previousBondDisposition, t.Snapshot.BondRecovery.Disposition, events); err != nil {
		return nil, fmt.Errorf("tournament %s: %w", tournamentAddress, err)
	}
	batch, err := s.tournamentEventBatch(ctx, app, epoch, t, adapter, events, endBlock)
	if err != nil {
		return nil, fmt.Errorf("projecting tournament %s events: %w", tournamentAddress, err)
	}
	children, err := s.gatherInnerTournamentData(ctx, app, epoch, t, events, mostRecentBlock)
	if err != nil {
		return nil, err
	}
	batches := make([]*repository.TournamentEventBatch, 0, 1+len(children))
	batches = append(batches, batch)
	return append(batches, children...), nil
}

// Finished clones reject joins and all match mutations. Once their winner can
// no longer expire and their bond cannot be recovered, their current views and
// events are immutable. Require a committed complete observation first.
func tournamentObservationComplete(t *Tournament, cursor, head uint64) bool {
	snapshot := t.Snapshot
	if snapshot.FinishedAtBlock == 0 || snapshot.FinishedAtBlock > snapshot.AsOfBlock ||
		snapshot.AsOfBlock > cursor || snapshot.AsOfBlock > head ||
		(t.Level != uint64(RootLevel) && t.CreationEvent == nil) {
		return false
	}
	switch snapshot.Standing {
	case TournamentStandingRootWinner, TournamentStandingRootFailed,
		TournamentStandingInnerEliminableNoWinner, TournamentStandingInnerEliminableWinnerExpired:
		return snapshot.BondRecovery.Disposition == BondDispositionNoWinner ||
			snapshot.BondRecovery.Disposition == BondDispositionRecovered
	case TournamentStandingMatchesActive, TournamentStandingAwaitingClosure, TournamentStandingInnerWinner:
		return false
	default:
		return false
	}
}

func (s *Service) gatherInnerTournamentData(
	ctx context.Context, app *Application, epoch *Epoch, t *Tournament, events *TournamentEvents, mostRecentBlock uint64,
) ([]*repository.TournamentEventBatch, error) {
	var batches []*repository.TournamentEventBatch

	if t.Level+1 >= t.MaxLevel {
		return batches, nil // no inner tournaments
	}

	level, levelCount, tournamentAddress := TournamentLevel(t.Level), t.MaxLevel, t.Address
	nextLevel := level + 1
	innerTournaments, _, err := getAllSubTournaments(ctx, s.repository, app.Name, epoch.Index, &tournamentAddress, level+1)
	if err != nil {
		s.logErrorUnlessShutdown("failed to list inner tournaments", err, "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String())
		return nil, err
	}

	seen := make(map[common.Address]*Tournament, len(innerTournaments)+len(events.NewInnerTournament))
	for _, i := range innerTournaments {
		if i.ParentMatchIDHash == nil || i.ParentTournamentAddress == nil || *i.ParentTournamentAddress != tournamentAddress {
			return nil, fmt.Errorf("child tournament %s has an invalid parent link", i.Address)
		}
		s.Logger.Debug("Fetching data for previous open tournament", "level", nextLevel,
			"parent_match_id_hash", i.ParentMatchIDHash.String(),
			"parent_tournament_address", i.ParentTournamentAddress.String(),
			"address", i.Address.String())

		childBatches, err := s.gatherTournamentData(ctx, app, epoch, nextLevel, i.ParentMatchIDHash,
			&tournamentAddress, i.Address, levelCount, mostRecentBlock)
		if err != nil {
			s.logErrorUnlessShutdown("failed to fetch tournament data", err,
				"level", nextLevel, "application", app.Name,
				"tournament", i.Address.String())
			return nil, err
		}
		batches = append(batches, childBatches...)
		storedChild := *i
		seen[i.Address] = &storedChild
		for _, childBatch := range childBatches {
			if childBatch.Tournament.Address == i.Address {
				seen[i.Address] = childBatch.Tournament
				break
			}
		}
	}

	for _, newInner := range events.NewInnerTournament {
		hashID := (common.Hash)(newInner.MatchIdHash)
		childAddress := newInner.ChildTournament
		if child := seen[childAddress]; child != nil {
			// A retired child already has this immutable creation tuple. Do
			// not modify its repository-owned projection during validation.
			projection := *child
			if err := applyTournamentCreationEvent(&projection, newInner); err != nil {
				return nil, err
			}
			child.CreationEvent = projection.CreationEvent
			continue
		}

		s.Logger.Info("NewInnerTournament event", "id_hash", hashID.String(), "tournament_address", childAddress.String())

		childBatches, err := s.gatherTournamentData(ctx, app, epoch, nextLevel, &hashID,
			&tournamentAddress, childAddress, levelCount, mostRecentBlock)
		if err != nil {
			s.logErrorUnlessShutdown("failed to fetch tournament data", err,
				"level", nextLevel, "application", app.Name,
				"tournament", childAddress.String())
			return nil, err
		}
		if len(childBatches) == 0 || childBatches[0].Tournament.Address != childAddress {
			return nil, fmt.Errorf("new child tournament %s has no initial observation", childAddress)
		}
		if err := applyTournamentCreationEvent(childBatches[0].Tournament, newInner); err != nil {
			return nil, err
		}
		batches = append(batches, childBatches...)
		seen[childAddress] = childBatches[0].Tournament
	}

	return batches, nil
}

func applyTournamentCreationEvent(child *Tournament, event *itournament.ITournamentNewInnerTournament) error {
	if child.Address != event.ChildTournament || child.ParentMatchIDHash == nil || *child.ParentMatchIDHash != event.MatchIdHash ||
		child.StartInstant != event.Raw.BlockNumber {
		return fmt.Errorf("child tournament %s descriptor does not match its creation event", child.Address)
	}
	creation := &TournamentCreationEvent{
		BlockNumber: event.Raw.BlockNumber, TxHash: event.Raw.TxHash, LogIndex: uint64(event.Raw.Index),
	}
	if child.CreationEvent != nil && *child.CreationEvent != *creation {
		return fmt.Errorf("child tournament %s has conflicting creation events", child.Address)
	}
	child.CreationEvent = creation
	return nil
}

// reactToTournament reports true only when the commitment is already joined.
// A deferred or newly broadcast join must not permit a new bond refund this tick.
func (s *Service) reactToTournament(ctx context.Context, app *Application, epoch *Epoch, mostRecentBlock uint64) (bool, error) {
	if epoch == nil || epoch.Status != EpochStatus_ClaimComputed {
		s.Logger.Debug("Application sync has not finished. Skipping join tournament", "application", app.Name)
		return false, nil
	}

	if epoch.TournamentAddress == nil || epoch.Commitment == nil ||
		epoch.MachineHash == nil || epoch.CommitmentProof == nil {
		return false, s.setApplicationCorrupted(ctx, app,
			"epoch %d has missing required fields for tournament reaction", epoch.Index)
	}

	commitment, err := s.repository.GetCommitment(ctx, app.IApplicationAddress.Hex(), epoch.Index,
		epoch.TournamentAddress.Hex(), epoch.Commitment.String())
	if err != nil {
		s.logErrorUnlessShutdown("failed to get commitment from repository", err, "application", app.Name,
			"epoch_index", epoch.Index, "tournament", epoch.TournamentAddress.Hex(),
			"commitment", epoch.Commitment.Hex())
		return false, err
	}
	if commitment != nil {
		s.Logger.Debug("Commitment already joined. Skipping JoinTournament", "application", app.Name,
			"epoch_index", epoch.Index, "tournament", epoch.TournamentAddress.Hex(), "commitment", epoch.Commitment.Hex())
		return true, nil
	}

	tournamentAdapter, err := s.adapterFactory.CreateTournamentAdapter(*epoch.TournamentAddress)
	if err != nil {
		s.Logger.Error("failed to create tournament adapter", "application", app.Name,
			"tournament", epoch.TournamentAddress.String(), "error", err)
		return false, err
	}

	callOpts := pinnedCallOpts(ctx, mostRecentBlock)

	// Check on-chain if the commitment was already joined (e.g., after a node
	// restart where the pending transaction was lost and the DB event sync hasn't caught up).
	commitmentStanding, err := tournamentAdapter.CommitmentStanding(callOpts, *epoch.Commitment)
	if err != nil {
		s.logErrorUnlessShutdown("failed to check commitment on-chain", err, "application", app.Name,
			"epoch_index", epoch.Index, "tournament", epoch.TournamentAddress.Hex(),
			"commitment", epoch.Commitment.Hex())
		return false, err
	}
	if commitmentStanding.Joined {
		if err := s.validateJoinedCommitment(ctx, app, epoch, commitmentStanding); err != nil {
			return false, err
		}
		s.Logger.Info("Commitment already joined on-chain, waiting for event sync",
			"application", app.Name, "epoch_index", epoch.Index,
			"tournament", epoch.TournamentAddress.Hex(), "commitment", epoch.Commitment.Hex())
		return true, nil
	}

	descriptor, err := tournamentAdapter.Descriptor(callOpts)
	if err != nil {
		return false, fmt.Errorf("reading root tournament geometry before joining: %w", err)
	}
	if descriptor.Level != uint64(RootLevel) || descriptor.Height != Log2EpochComputationHashLeafCount {
		return false, s.setApplicationFailed(ctx, app,
			"Cannot join tournament %s: root level %d and commitment height %d are required; got level %d and height %d. "+
				"Check the tournament factory and node versions before re-enabling.",
			epoch.TournamentAddress, RootLevel, Log2EpochComputationHashLeafCount, descriptor.Level, descriptor.Height)
	}

	bondValue, err := tournamentAdapter.BondValue(callOpts)
	if err != nil {
		s.logErrorUnlessShutdown("failed to fetch tournament bond value", err, "application", app.Name,
			"epoch_index", epoch.Index, "tournament", epoch.TournamentAddress.Hex())
		return false, err
	}

	txCtx, cancel := context.WithTimeout(ctx, s.submissionTimeout)
	defer cancel()
	txOpts, err := s.txOptsFactory.NewTransactOpts(txCtx)
	if err != nil {
		return false, fmt.Errorf("creating transaction options for joining tournament: %w", err)
	}
	txOptsWithValue := *txOpts
	txOptsWithValue.Value = bondValue

	idx := uint64(1<<Log2EpochComputationHashLeafCount) - 1
	leftNode, rightNode, err := merkle.RootChildrenFromProof(*epoch.MachineHash, epoch.CommitmentProof, idx)
	if err != nil {
		s.Logger.Error("failed to compute left and right nodes from commitment proof",
			"application", app.Name, "epoch_index", epoch.Index, "error", err)
		return false, err
	}

	s.Logger.Info("Joining tournament", "application", app.Name, "epoch_index", epoch.Index,
		"commitment", epoch.Commitment, "left_node", leftNode.String(), "right_node", rightNode.String())

	tx, err := tournamentAdapter.JoinTournament(&txOptsWithValue, *epoch.MachineHash,
		hashSliceToByteSlice(epoch.CommitmentProof), leftNode, rightNode)
	if err != nil {
		return false, s.handleJoinTournamentRevert(ctx, app, epoch, tournamentAdapter, err)
	}
	s.pendingTransactions[app.ID] = pendingTournamentTransaction{
		Action: tournamentActionJoin, Hash: tx.Hash(), EpochIndex: epoch.Index,
	}

	return false, nil
}

func (s *Service) validateJoinedCommitment(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
	standing CommitmentStanding,
) error {
	if epoch.MachineHash == nil {
		return s.setApplicationCorrupted(ctx, app,
			"epoch %d has no machine hash for joined commitment reconciliation", epoch.Index)
	}
	if standing.FinalState != *epoch.MachineHash {
		return fmt.Errorf(
			"epoch %d commitment has inconsistent final state between off-chain (%s) and on-chain (%s)",
			epoch.Index, epoch.MachineHash.String(), standing.FinalState.String())
	}
	return nil
}

// handleJoinTournamentRevert classifies a JoinTournament error and performs
// the matching state change. The known tournament reverts:
//
//   - ClockAlreadyInitialized: this commitment already joined — after a
//     restart when the IsCommitmentJoined pre-check used a slightly stale
//     block number. Wait for event sync.
//   - TournamentIsClosed: the join window is closed. The contract checks the
//     window before the already-joined clock check, so this also fires for a
//     commitment that DID join before the window closed; re-check
//     CommitmentStanding at the latest block to tell the two apart. Truly
//     unjoined must also be confirmed at the configured block before marking
//     the app FAILED. (TournamentIsFinished is handled identically as a
//     backstop, though join's window check fires first on a finished
//     tournament, so it should be unreachable from join.)
//   - CommitmentStateMismatch / CommitmentProofWrongSize: the locally stored
//     commitment proof does not reconstruct the commitment root — local data
//     corruption; CORRUPTED.
//
// JSON-RPC "nonce too low" broadcast rejections retry next tick; unknown
// errors are returned to the caller unchanged, with the decoded revert name
// in the log when one of the known ABIs declares it.
func (s *Service) handleJoinTournamentRevert(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
	tournamentAdapter TournamentAdapter,
	err error,
) error {
	switch {
	case isTournamentError(err, "ClockAlreadyInitialized"):
		s.Logger.Info("Commitment already joined on-chain (detected via revert), waiting for event sync",
			"application", app.Name, "epoch_index", epoch.Index,
			"tournament", epoch.TournamentAddress.Hex(), "commitment", epoch.Commitment.Hex())
		return nil

	case isTournamentError(err, "TournamentIsClosed"), isTournamentError(err, "TournamentIsFinished"):
		revertName := "TournamentIsClosed"
		if isTournamentError(err, "TournamentIsFinished") {
			revertName = "TournamentIsFinished"
		}
		// The window check precedes the already-joined check on chain, so a
		// commitment that joined just before the window closed reverts with
		// the window error on a rebroadcast (e.g. after a restart with a
		// stale CommitmentStanding read). Re-check at the latest block before
		// declaring the join missed.
		standing, joinedErr := tournamentAdapter.CommitmentStanding(
			&bind.CallOpts{Context: ctx}, *epoch.Commitment)
		if joinedErr != nil {
			s.Logger.Warn("JoinTournament reverted with "+revertName+" but the "+
				"already-joined re-check failed; retrying next tick",
				"application", app.Name, "epoch_index", epoch.Index,
				"tournament", epoch.TournamentAddress.Hex(),
				"check_error", joinedErr)
			return err
		}
		if standing.Joined {
			if err := s.validateJoinedCommitment(ctx, app, epoch, standing); err != nil {
				return err
			}
			s.Logger.Info("Commitment already joined on-chain (window closed after the join), "+
				"waiting for event sync",
				"application", app.Name, "epoch_index", epoch.Index,
				"tournament", epoch.TournamentAddress.Hex(), "commitment", epoch.Commitment.Hex())
			return nil
		}
		// The revert and latest read can refer to unfinalized state. Confirm
		// both closure and the missing join before stopping this application.
		confirmedBlock, confirmErr := s.getDefaultBlockNumber(ctx)
		if confirmErr != nil {
			return confirmErr
		}
		confirmedOpts := pinnedCallOpts(ctx, confirmedBlock)
		confirmedStanding, confirmErr := tournamentAdapter.Standing(confirmedOpts)
		if confirmErr != nil {
			return fmt.Errorf("confirming closed join window: %w", confirmErr)
		}
		if confirmedStanding.AcceptsJoins {
			return nil
		}
		confirmedCommitment, confirmErr := tournamentAdapter.CommitmentStanding(confirmedOpts, *epoch.Commitment)
		if confirmErr != nil {
			return fmt.Errorf("confirming missing commitment after join window closed: %w", confirmErr)
		}
		if confirmedCommitment.Joined {
			return nil
		}
		return s.setApplicationFailed(ctx, app,
			"JoinTournament reverted with %s for epoch %d "+
				"(tournament %s): the tournament no longer accepts new "+
				"commitments — the join window closed before this node joined, "+
				"so it cannot defend its claim. Investigate node downtime or a "+
				"lagging RPC endpoint before re-enabling.",
			revertName, epoch.Index, epoch.TournamentAddress.Hex())

	case isTournamentError(err, "CommitmentStateMismatch"):
		return s.setApplicationCorrupted(ctx, app,
			"JoinTournament reverted with CommitmentStateMismatch for epoch %d "+
				"(tournament %s) — the commitment proof stored locally does not "+
				"reconstruct the commitment root.",
			epoch.Index, epoch.TournamentAddress.Hex())

	case isTournamentError(err, "CommitmentProofWrongSize"):
		return s.setApplicationCorrupted(ctx, app,
			"JoinTournament reverted with CommitmentProofWrongSize for epoch %d "+
				"(tournament %s) — the commitment proof stored locally has the "+
				"wrong length for the tournament's tree height.",
			epoch.Index, epoch.TournamentAddress.Hex())

	case ethutil.IsNonceTooLowError(err):
		// Transient broadcast race: a tx with this EOA's nonce is already
		// mined. The next tick's CommitmentStanding check will reconcile
		// against the propagated chain state and short-circuit if our prior
		// JoinTournament landed; otherwise a new broadcast goes out with a
		// fresh nonce.
		s.Logger.Info(
			"JoinTournament broadcast rejected with 'nonce too low'; "+
				"deferring to the next tick's CommitmentStanding reconciliation",
			"application", app.Name,
			"epoch_index", epoch.Index,
			"tournament", epoch.TournamentAddress.Hex(),
			"commitment", epoch.Commitment.Hex())
		return nil
	}
	s.logErrorUnlessShutdown("failed to send join tournament transaction", err, "application", app.Name,
		"epoch_index", epoch.Index,
		"decoded_revert", describeKnownRevert(err))
	return err
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
		// trySettle may have marked the app FAILED (returning nil, per the
		// appstatus contract). Stop this tick's work instead of broadcasting
		// a bond-carrying JoinTournament for an app that was just halted.
		if app.Status != ApplicationStatus_OK {
			return nil
		}
		err = s.reactToTournament(ctx, app, mostRecentBlock)
		if err != nil {
			return err
		}
	}
	return nil
}

// isDaveConsensusError matches a typed Solidity error declared in the
// IDaveConsensus ABI against an RPC revert.
func isDaveConsensusError(err error, name string) bool {
	return ethutil.IsCustomError(err, idaveconsensus.IDaveConsensusMetaData, name)
}

// isTournamentError matches a typed Solidity error declared in the
// ITournament ABI against an RPC revert.
func isTournamentError(err error, name string) bool {
	return ethutil.IsCustomError(err, itournament.ITournamentMetaData, name)
}

// decodeIncorrectEpochNumber reads the arguments from an IncorrectEpochNumber
// revert:
//
//	error IncorrectEpochNumber(uint256 receivedEpochNumber, uint256 actualEpochNumber);
//
// received < actual means the chain accepted past us;
// received > actual means the local epoch index is ahead of the chain.
func decodeIncorrectEpochNumber(err error) (received, actual *big.Int, ok bool) {
	values, ok := ethutil.UnpackRevert(err, idaveconsensus.IDaveConsensusMetaData, "IncorrectEpochNumber")
	if !ok || len(values) < 2 {
		return nil, nil, false
	}
	received, okReceived := values[0].(*big.Int)
	actual, okActual := values[1].(*big.Int)
	if !okReceived || !okActual {
		return nil, nil, false
	}
	return received, actual, true
}

// describeKnownRevert renders the revert carried by err against the ABIs this
// service interacts with, for the unknown-error log lines. Returns "" when
// nothing matches.
func describeKnownRevert(err error) string {
	desc, ok := ethutil.DescribeRevert(err,
		idaveconsensus.IDaveConsensusMetaData, itournament.ITournamentMetaData)
	if !ok {
		return ""
	}
	return desc
}
