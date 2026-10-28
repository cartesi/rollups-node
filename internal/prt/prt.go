// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/cartesi/rollups-node/internal/appstatus"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

type prtRepository interface {
	repository.DeliveryRepository
	GetApplication(ctx context.Context, address string) (*Application, error)
	ListApplications(ctx context.Context, f repository.ApplicationFilter,
		p repository.Pagination, descending bool) ([]*Application, uint64, error)
	UpdateApplicationStatus(ctx context.Context, appID int64, status ApplicationStatus, reason *string) error
	HasUndrainedEpochsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)
	HasUnreconciledClaimsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)
	UpdateEpochWithForeclosedClaim(ctx context.Context, applicationID int64, index uint64) error

	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter,
		p repository.Pagination, descending bool) ([]*Epoch, uint64, error)
	GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	GetEpochCompletionState(ctx context.Context, nameOrAddress string, index uint64) (*repository.EpochCompletionState, error)
	GetLastNonOpenEpochIndex(ctx context.Context, nameOrAddress string) (*uint64, error)
	UpdateEpochReconciledStaged(ctx context.Context, applicationID int64, index uint64, stagedAtBlock uint64) error
	UpdateEpochStagingBlock(ctx context.Context, applicationID int64, index, expectedOldBlock, newBlock uint64) error
	UpdateEpochWithAcceptedClaim(ctx context.Context, applicationID int64, index uint64, txHash *common.Hash) error

	GetTournament(ctx context.Context, nameOrAddress string, address string) (*Tournament, error)
	ListTournaments(ctx context.Context, nameOrAddress string, f repository.TournamentFilter,
		p repository.Pagination, descending bool) ([]*Tournament, uint64, error)
	GetCommitment(ctx context.Context, nameOrAddress string, epochIndex uint64,
		tournamentAddress string, commitmentHex string) (*Commitment, error)
	ListCommitments(ctx context.Context, nameOrAddress string, filter repository.CommitmentFilter,
		pagination repository.Pagination, descending bool) ([]*Commitment, uint64, error)
	ListMatches(ctx context.Context, nameOrAddress string, filter repository.MatchFilter,
		pagination repository.Pagination, descending bool) ([]*Match, uint64, error)

	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)
}

// EthClientInterface defines the methods we need from ethclient.Client
type EthClientInterface interface {
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
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

func getTournamentObservationEpochs(ctx context.Context, r prtRepository, nameOrAddress string) ([]*Epoch, uint64, error) {
	f := repository.EpochFilter{HasTournament: new(true)}
	return r.ListEpochs(ctx, nameOrAddress, f, repository.Pagination{}, false)
}

// getDefaultBlockNumber selects the block used for stored chain state. Live
// transaction checks use a separate latest block and cannot advance this view.
func (s *Service) getDefaultBlockNumber(ctx context.Context) (uint64, error) {
	header, err := s.getDefaultBlockHeader(ctx)
	if err != nil {
		return 0, err
	}
	return header.Number.Uint64(), nil
}

func (s *Service) getDefaultBlockHeader(ctx context.Context) (*types.Header, error) {
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
		return nil, fmt.Errorf("default block %v not supported", s.defaultBlock)
	}
	header, err := s.client.HeaderByNumber(ctx, big.NewInt(tag.Int64()))
	if err != nil {
		return nil, fmt.Errorf("fetching %s block header: %w", tag, err)
	}
	if header == nil || header.Number == nil || !header.Number.IsUint64() {
		return nil, fmt.Errorf("returned %s block header is invalid", tag)
	}
	return header, nil
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
		s.logErrorUnlessShutdown(ctx, "failed to fetch tournament descriptor", err, "level", level, "application", app.Name,
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
		s.logErrorUnlessShutdown(ctx, "failed to fetch tournament descriptor", err, "level", level, "application", app.Name,
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
		s.logErrorUnlessShutdown(ctx, "failed to fetch tournament standing", err, "level", level, "application", app.Name,
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
			s.logErrorUnlessShutdown(ctx, "failed to fetch inner tournament result", err,
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
		s.logErrorUnlessShutdown(ctx, "failed to fetch tournament bond recovery", err,
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

// A failed RPC read or database write leaves the shared cursor unchanged.
func (s *Service) reconcileAcceptedEpochs(
	ctx context.Context, app *Application, epochs []*Epoch, consensus DaveConsensusAdapter, mostRecentBlock uint64,
) (bool, error) {
	for _, epoch := range epochs {
		if epoch.Status != EpochStatus_ClaimComputed && epoch.Status != EpochStatus_ClaimStaged {
			continue
		}
		var err error
		epoch, err = s.observerCompletionEpoch(ctx, app, epoch)
		if err != nil {
			return true, err
		}
		if epoch == nil {
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
		if app.IsForeclosed() && epoch.LastBlock < app.ForecloseBlock && !rootObservationCovers(tournament, mostRecentBlock) {
			return true, nil
		}
		if tournament == nil || tournament.Snapshot.FinishedAtBlock == 0 ||
			tournament.Snapshot.FinishedAtBlock > tournament.Snapshot.AsOfBlock ||
			tournament.Snapshot.AsOfBlock > mostRecentBlock {
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
			s.logErrorUnlessShutdown(ctx, "failed to fetch transaction receipt for epoch", err, "application", app.Name,
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
		observedBlock := min(mostRecentBlock, tournament.Snapshot.AsOfBlock)
		// A retired root's winner is immutable. Its older certificate remains
		// sufficient while DaveConsensus stages and accepts the result later.
		if tournamentObservationComplete(tournament, tournament.Snapshot.AsOfBlock, mostRecentBlock) {
			observedBlock = mostRecentBlock
		}
		if receiptBlock > observedBlock {
			s.Logger.Debug("Acceptance transaction is newer than the published tournament window",
				"application", app.Name,
				"epoch", epoch.Index,
				"tx", epoch.ClaimTransactionHash,
				"receipt_block", receiptBlock,
				"snapshot_block", observedBlock)
			return true, nil
		}
		if err := s.validateAcceptanceReceipt(ctx, receipt); err != nil {
			return true, fmt.Errorf("epoch %d: %w", epoch.Index, err)
		}

		if receipt.Status != types.ReceiptStatusSuccessful {
			return true, fmt.Errorf("epoch %d: EpochSealed transaction hash points to failed transaction", epoch.Index)
		}

		var event *idaveconsensus.IDaveConsensusEpochSealed
		expectedEventEpoch := new(big.Int).SetUint64(epoch.Index)
		expectedEventEpoch.Add(expectedEventEpoch, common.Big1)
		for _, vLog := range receipt.Logs {
			if vLog == nil || vLog.Address != app.IConsensusAddress ||
				vLog.TxHash != *epoch.ClaimTransactionHash || vLog.BlockNumber != receiptBlock ||
				vLog.BlockHash != receipt.BlockHash || vLog.Removed {
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
			return true, fmt.Errorf("%w: epoch %d: failed to find EpochSealed event in receipt logs", txsender.ErrInconclusive, epoch.Index)
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

		s.Logger.Info("Observed accepted epoch at selected block policy; outputs root matched",
			"application", app.Name,
			"epoch", epoch.Index,
			"event_block_number", event.Raw.BlockNumber,
			"outputs_merkle_root", fmt.Sprintf("%x", event.OutputsMerkleRoot),
			"tx", epoch.ClaimTransactionHash,
		)

		err = s.repository.UpdateEpochWithAcceptedClaim(ctx, app.ID, epoch.Index, epoch.ClaimTransactionHash)
		if err != nil {
			s.logErrorUnlessShutdown(ctx, "failed to update epoch status to claim accepted", err,
				"application", app.Name, "epoch", epoch.Index)
			return true, err
		}
	}
	return false, nil
}

// gatherTournamentData reads one tournament and only newly discovered
// descendants. Already stored children receive their own independent turn.
func (s *Service) gatherTournamentData(
	ctx context.Context, app *Application, epoch *Epoch, level TournamentLevel,
	parentMatchIDHash *common.Hash, parentTournamentAddress *common.Address,
	tournamentAddress common.Address, levelCount, mostRecentBlock uint64,
) ([]*repository.TournamentEventBatch, error) {
	t, err := s.repository.GetTournament(ctx, app.IApplicationAddress.Hex(), tournamentAddress.Hex())
	if err != nil {
		s.logErrorUnlessShutdown(ctx, "failed to load tournament from database", err, "level", level, "application", app.Name,
			"epoch", epoch.Index, "tournament_address", tournamentAddress.String())
		return nil, err
	}
	created := t == nil
	var previousAsOf uint64
	previousBondDisposition := BondDispositionTournamentRunning
	if !created {
		previousAsOf = t.Snapshot.AsOfBlock
		previousBondDisposition = t.Snapshot.BondRecovery.Disposition
	}
	if !created {
		if t.MaxLevel != levelCount || t.Level != uint64(level) {
			return nil, fmt.Errorf("tournament %s database geometry does not match observed hierarchy", tournamentAddress)
		}
		if t.Snapshot.AsOfBlock >= mostRecentBlock || tournamentObservationComplete(t, t.Snapshot.AsOfBlock, mostRecentBlock) {
			return nil, nil
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
			s.logErrorUnlessShutdown(ctx, "failed to create new tournament", err,
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
			s.logErrorUnlessShutdown(ctx, "failed to check if tournament was finished", err, "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", tournamentAddress.String())
			return nil, err
		}
		if previousFinish == 0 && t.Snapshot.FinishedAtBlock != 0 {
			s.Logger.Info("Found finished tournament", "level", level, "application", app.Name,
				"epoch", epoch.Index, "tournament_address", t.Address.String())
		}
	}

	nextSearchBlock := t.StartInstant
	if !created {
		if previousAsOf == math.MaxUint64 {
			return nil, errors.New("tournament cursor overflow")
		}
		nextSearchBlock = max(t.StartInstant, previousAsOf+1)
	}

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
		events, err = adapter.RetrieveAllEvents(&bind.FilterOpts{Context: ctx, Start: nextSearchBlock, End: &endBlock})
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
	if !created && nextSearchBlock > t.StartInstant {
		// Committed event identities preserve the cumulative baseline even when
		// the provider has pruned state at the previous observation block.
		counts, countErr := s.repository.GetTournamentEventCounts(ctx, app.ID, epoch.Index, tournamentAddress, previousAsOf)
		if countErr != nil {
			return nil, fmt.Errorf("reading committed tournament event counts: %w", countErr)
		}
		previousCounts = structuralEventCounts(counts)
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
	return append([]*repository.TournamentEventBatch{batch}, children...), nil
}

// rootObservationCovers separates immutable tournament facts from later
// DaveConsensus stage/accept transitions. Retirement never advances a cursor.
func rootObservationCovers(root *Tournament, head uint64) bool {
	if root == nil || root.Snapshot.AsOfBlock > head {
		return false
	}
	return root.Snapshot.AsOfBlock == head || tournamentObservationComplete(root, root.Snapshot.AsOfBlock, head)
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
	ctx context.Context, app *Application, epoch *Epoch, t *Tournament, events *TournamentEvents, head uint64,
) ([]*repository.TournamentEventBatch, error) {
	if t.Level+1 >= t.MaxLevel {
		return nil, nil
	}
	seen := make(map[common.Address]bool)
	var batches []*repository.TournamentEventBatch
	for _, event := range events.NewInnerTournament {
		if seen[event.ChildTournament] {
			return nil, fmt.Errorf("duplicate child creation for %s", event.ChildTournament)
		}
		seen[event.ChildTournament] = true
		stored, err := s.repository.GetTournament(ctx, app.IApplicationAddress.Hex(), event.ChildTournament.Hex())
		if err != nil {
			return nil, err
		}
		if stored != nil {
			projection := *stored
			if err = applyTournamentCreationEvent(&projection, event); err != nil {
				return nil, err
			}
			continue
		}
		hash := common.Hash(event.MatchIdHash)
		children, err := s.gatherTournamentData(ctx, app, epoch, TournamentLevel(t.Level+1), &hash, &t.Address,
			event.ChildTournament, t.MaxLevel, head)
		if err != nil {
			return nil, err
		}
		if len(children) == 0 {
			return nil, fmt.Errorf("new child tournament %s has no initial observation", event.ChildTournament)
		}
		if err = applyTournamentCreationEvent(children[0].Tournament, event); err != nil {
			return nil, err
		}
		batches = append(batches, children...)
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
