// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// validatedObservationReference validates the selected head itself and the
// previously accepted finalized hash. Callers own their separate watermarks.
func (s *Service) validatedObservationReference(ctx context.Context, previous txsender.BlockRef) (txsender.BlockRef, error) {
	header, err := s.getDefaultBlockHeader(ctx)
	if err != nil {
		return txsender.BlockRef{}, err
	}
	candidate := txsender.BlockRef{Number: header.Number.Uint64(), Hash: header.Hash()}
	canonical, err := s.client.HeaderByNumber(ctx, new(big.Int).SetUint64(candidate.Number))
	if err != nil {
		return txsender.BlockRef{}, err
	}
	if canonical == nil || canonical.Hash() != candidate.Hash {
		return txsender.BlockRef{}, txsender.ErrInconclusive
	}
	if s.defaultBlock == model.DefaultBlock_Finalized && previous.Hash != (common.Hash{}) {
		if candidate.Number < previous.Number {
			return txsender.BlockRef{}, txsender.ErrInconclusive
		}
		old, readErr := s.client.HeaderByNumber(ctx, new(big.Int).SetUint64(previous.Number))
		if readErr != nil {
			return txsender.BlockRef{}, readErr
		}
		if old == nil {
			return txsender.BlockRef{}, txsender.ErrInconclusive
		}
		if old.Hash() != previous.Hash {
			return txsender.BlockRef{}, fmt.Errorf("%w: block %d saved_hash=%s canonical_hash=%s",
				txsender.ErrFinality, previous.Number, previous.Hash, old.Hash())
		}
	}
	return candidate, nil
}

// observe owns traversal and domain writes at the selected policy. A failed tournament
// rolls back its own interval, without discarding another tournament's progress.
func (s *Service) observe(ctx context.Context) error {
	filter := repository.ApplicationFilter{ConsensusType: new(model.Consensus_PRT), Enabled: new(true)}
	through, err := s.repository.GetApplicationIDBoundary(ctx, filter)
	if err != nil || through == 0 {
		if err == nil {
			s.protectionNotices = nil
		}
		return err
	}
	wasStopped := s.publicationStopped()
	defer func() {
		if !wasStopped && s.publicationStopped() {
			s.reportGlobalProtectionStop(ctx, through, s.publicationFailure())
		}
	}()
	facts, err := s.validatedObservationReference(ctx, s.observerFacts)
	if err != nil {
		if errors.Is(err, txsender.ErrFinality) {
			s.stopPublication(err)
		}
		return err
	}
	s.observerFacts = facts
	var errs []error
	active := make(map[int64]struct{})
	for after := int64(0); after < through; {
		apps, listErr := s.repository.ListApplicationsByID(ctx, filter, after, through, deliveryApplicationPageSize)
		if listErr != nil {
			return errors.Join(append(errs, listErr)...)
		}
		if len(apps) == 0 {
			break
		}
		for _, app := range apps {
			active[app.ID] = struct{}{}
			after = app.ID
			if err = s.observeApplication(ctx, app, facts); err != nil {
				errs = append(errs, fmt.Errorf("observing PRT application %s: %w", app.Name, err))
			}
			if ctx.Err() != nil {
				return errors.Join(append(errs, ctx.Err())...)
			}
		}
	}
	for id := range s.protectionNotices {
		if _, ok := active[id]; !ok {
			delete(s.protectionNotices, id)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) observeApplication(ctx context.Context, app *model.Application, facts txsender.BlockRef) (resultErr error) {
	if !app.Enabled {
		s.clearTournamentObservationFailure(app.ID)
		delete(s.protectionNotices, app.ID)
		return nil
	}
	var deploymentErr error
	defer func() { resultErr = errors.Join(resultErr, deploymentErr) }()
	s.warnZeroStagingPeriod(app)
	epochs, _, epochErr := getTournamentObservationEpochs(ctx, s.repository, app.Name)
	consensus, err := s.adapterFactory.CreateDaveConsensusAdapter(app.IConsensusAddress)
	if epochErr == nil && err == nil && len(epochs) == 0 &&
		app.Status == model.ApplicationStatus_OK && app.LastTournamentCheckBlock == 0 {
		// Registration at latest does not establish visibility at this observation block.
		opts := pinnedCallOpts(withLiveBlock(ctx, facts.Number, facts.Hash), facts.Number)
		if _, visibilityErr := consensus.GetCurrentSealedEpoch(opts); errors.Is(visibilityErr, bind.ErrNoCode) {
			return nil
		}
	}
	clocks := s.observeProtection(ctx, app, epochs, consensus, facts, errors.Join(epochErr, err))
	var deployment deploymentSnapshot
	if err == nil {
		deployment, deploymentErr = s.observeDeployment(ctx, app, consensus, facts)
	}
	state, generation, admissionErr := s.deploymentVerdictState(ctx, app)
	deploymentErr = errors.Join(deploymentErr, admissionErr)
	s.reportProtectionStops(ctx, app, clocks, facts, state, generation, deploymentErr)
	if ctx.Err() != nil {
		return errors.Join(epochErr, err, ctx.Err())
	}
	if epochErr != nil || err != nil {
		return errors.Join(epochErr, err)
	}
	var view *daveConsensusSnapshot
	var eligibilityErr error
	if completionStatusAllowed(app.Status) && !app.IsForeclosed() {
		for _, epoch := range epochs {
			if !pendingTournamentResult(epoch) || epoch.LastBlock > facts.Number {
				continue
			}
			eligible, readErr := s.observerCompletionEpoch(ctx, app, epoch)
			if readErr != nil {
				eligibilityErr = errors.Join(eligibilityErr, readErr)
				continue
			}
			if eligible == nil {
				continue
			}
			snapshot, readErr := readDaveConsensusSnapshot(withLiveBlock(ctx, facts.Number, facts.Hash), consensus, facts.Number)
			if errors.Is(readErr, bind.ErrNoCode) && app.Status == model.ApplicationStatus_OK {
				return nil
			}
			if readErr != nil {
				s.recordTournamentObservationFailure(ctx, app, facts.Number, facts.Number, readErr)
				readErr = fmt.Errorf("application %s (%s), consensus snapshot block %d hash %s: %w",
					app.Name, app.IApplicationAddress, facts.Number, facts.Hash, readErr)
				if app.Status == model.ApplicationStatus_OK {
					return readErr
				}
				eligibilityErr = errors.Join(eligibilityErr, readErr)
				break
			}
			view = &snapshot
			break
		}
	}
	observeErr := eligibilityErr
	if deployment.Alpha6Shape {
		observeErr = errors.Join(observeErr, s.observeTournamentState(ctx, app, epochs, consensus, facts))
	}
	if ctx.Err() != nil {
		return errors.Join(observeErr, ctx.Err())
	}
	// Existing committed roots remain independently usable even if another root
	// failed. Reconciliation uses each owning root's durable endpoint below.
	if eligibilityErr != nil || !completionStatusAllowed(app.Status) {
		return observeErr
	}
	if app.IsForeclosed() {
		return errors.Join(observeErr, s.drainForeclosedClaims(ctx, app, epochs, consensus, facts.Number))
	}
	deferred, reconcileErr := s.reconcileAcceptedEpochs(ctx, app, epochs, consensus, facts.Number)
	if reconcileErr != nil || deferred || len(epochs) == 0 {
		return errors.Join(observeErr, reconcileErr)
	}
	hasCurrentRoot := false
	for _, epoch := range epochs {
		if !pendingTournamentResult(epoch) || epoch.TournamentAddress == nil {
			continue
		}
		root, loadErr := s.repository.GetTournament(ctx, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex())
		if loadErr != nil {
			return errors.Join(observeErr, loadErr)
		}
		if rootObservationCovers(root, facts.Number) {
			hasCurrentRoot = true
			break
		}
	}
	if !hasCurrentRoot {
		return observeErr
	}
	if view == nil {
		return observeErr
	}
	epoch, err := s.repository.GetEpoch(ctx, app.IApplicationAddress.Hex(), view.sealed.EpochNumber)
	if err != nil {
		return errors.Join(observeErr, err)
	}
	if pendingTournamentResult(epoch) && (epoch.StagedAtBlock == nil || *epoch.StagedAtBlock <= facts.Number) {
		epoch, err = s.observerCompletionEpoch(ctx, app, epoch)
		if err != nil || epoch == nil {
			return errors.Join(observeErr, err)
		}
		root, readErr := s.repository.GetTournament(ctx, app.IApplicationAddress.Hex(), view.sealed.Tournament.Hex())
		if readErr != nil {
			return errors.Join(observeErr, readErr)
		}
		if rootObservationCovers(root, facts.Number) {
			err = s.recordTournamentResult(ctx, app, epoch, *view)
		}
	}
	return errors.Join(observeErr, err)
}

func (s *Service) observeTournamentState(
	ctx context.Context, app *model.Application, epochs []*model.Epoch, consensus DaveConsensusAdapter, facts txsender.BlockRef,
) error {
	stored, _, err := s.repository.ListTournaments(ctx, app.Name, repository.TournamentFilter{}, repository.Pagination{}, false)
	if err != nil {
		return err
	}
	known := make(map[common.Address]bool, len(stored))
	var targets []*model.Tournament
	for _, tournament := range stored {
		known[tournament.Address] = true
		if tournament.Snapshot.AsOfBlock < facts.Number &&
			!tournamentObservationComplete(tournament, tournament.Snapshot.AsOfBlock, facts.Number) {
			targets = append(targets, tournament)
		}
	}
	for _, epoch := range epochs {
		if epoch.TournamentAddress != nil && epoch.LastBlock <= facts.Number && !known[*epoch.TournamentAddress] {
			targets = append(targets, &model.Tournament{ApplicationID: app.ID, EpochIndex: epoch.Index,
				Address: *epoch.TournamentAddress, StartInstant: epoch.LastBlock})
		}
	}
	if len(targets) == 0 {
		if failure, exists := s.observationFailures[app.ID]; exists && facts.Number >= failure.lastHead {
			s.clearTournamentObservationFailure(app.ID)
		}
		return nil
	}
	if err := s.validateObservationBlock(ctx, facts); err != nil {
		return err
	}
	ctx = withLiveBlock(ctx, facts.Number, facts.Hash)
	levels, err := consensus.TournamentLevelCount(pinnedCallOpts(ctx, facts.Number))
	if err == nil && levels == 0 {
		err = errors.New("tournament level count is zero")
	}
	if err != nil {
		s.recordTournamentObservationFailure(ctx, app, facts.Number, tournamentObservationStart(targets[0]), err)
		return err
	}
	epochsByIndex := make(map[uint64]*model.Epoch, len(epochs))
	for _, epoch := range epochs {
		epochsByIndex[epoch.Index] = epoch
	}
	var errs []error
	var failedFrom *uint64
	completed := false
	for _, target := range targets {
		epoch := epochsByIndex[target.EpochIndex]
		var loadErr error
		if epoch == nil {
			epoch, loadErr = s.repository.GetEpoch(ctx, app.IApplicationAddress.Hex(), target.EpochIndex)
		}
		if loadErr != nil || epoch == nil {
			errs = append(errs, errors.Join(loadErr, fmt.Errorf("missing epoch for %s", target.Address)))
			if failedFrom == nil {
				failedFrom = new(tournamentObservationStart(target))
			}
			continue
		}
		var expected *uint64
		if known[target.Address] {
			expected = new(target.Snapshot.AsOfBlock)
		}
		batches, readErr := s.gatherTournamentData(ctx, app, epoch, TournamentLevel(target.Level), target.ParentMatchIDHash,
			target.ParentTournamentAddress, target.Address, levels, facts.Number, nil, nil)
		if readErr == nil && len(batches) > 0 {
			// Revalidate after all reads and before publishing this complete unit.
			readErr = s.validateObservationBlock(ctx, facts)
			if readErr == nil {
				readErr = s.repository.StoreTournamentInterval(ctx, app.ID, expected, batches)
				if readErr == nil {
					app.LastTournamentCheckBlock = max(app.LastTournamentCheckBlock, facts.Number)
				}
			}
		}
		if readErr != nil {
			errs = append(errs, fmt.Errorf("tournament %s interval: %w", target.Address, readErr))
			if !errors.Is(readErr, repository.ErrTournamentIntervalStale) && failedFrom == nil {
				failedFrom = new(tournamentObservationStart(target))
			}
		} else if len(batches) > 0 {
			completed = true
		}
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
	}
	passErr := errors.Join(errs...)
	if failedFrom != nil {
		s.recordTournamentObservationFailure(ctx, app, facts.Number, *failedFrom, passErr)
	} else if completed {
		s.clearTournamentObservationFailure(app.ID)
	}
	return passErr
}

func tournamentObservationStart(tournament *model.Tournament) uint64 {
	return max(tournament.StartInstant, tournament.Snapshot.AsOfBlock+1)
}

func (s *Service) validateObservationBlock(ctx context.Context, facts txsender.BlockRef) error {
	header, err := s.client.HeaderByNumber(ctx, new(big.Int).SetUint64(facts.Number))
	if err != nil {
		return err
	}
	if header == nil || header.Number == nil || !header.Number.IsUint64() || header.Number.Uint64() != facts.Number {
		return txsender.ErrInconclusive
	}
	if header.Hash() == facts.Hash {
		return nil
	}
	if s.defaultBlock != model.DefaultBlock_Finalized {
		return txsender.ErrStale
	}
	err = fmt.Errorf("%w: observation block %d selected_hash=%s canonical_hash=%s",
		txsender.ErrFinality, facts.Number, facts.Hash, header.Hash())
	s.stopPublication(err)
	return err
}

func completionStatusAllowed(status model.ApplicationStatus) bool {
	return status == model.ApplicationStatus_OK || status.IsExecutionTerminal() || status == model.ApplicationStatus_InvalidOutputsRoot
}

// Healthy observation retains its existing reconciliation path. Continuing it
// after ET/IOR requires positive, coherent evidence that this epoch precedes the
// stop. Tournament interval observation itself remains independent of that gate.
func (s *Service) observerCompletionEpoch(ctx context.Context, app *model.Application, epoch *model.Epoch) (*model.Epoch, error) {
	if !completionStatusAllowed(app.Status) {
		return nil, txsender.ErrIneligible
	}
	if app.Status == model.ApplicationStatus_OK {
		return epoch, nil
	}
	state, err := s.repository.GetEpochCompletionState(ctx, app.IApplicationAddress.Hex(), epoch.Index)
	if err != nil {
		return nil, err
	}
	if state == nil || state.Application == nil || state.Application.ID != app.ID || !state.Application.Enabled ||
		!completionStatusAllowed(state.Application.Status) {
		return nil, txsender.ErrIneligible
	}
	allowed, err := state.AllowsEpochCompletion()
	if err != nil {
		return nil, fmt.Errorf("%w: application %s epoch %d: %w", txsender.ErrInconclusive, app.Name, epoch.Index, err)
	}
	if !allowed {
		return nil, nil
	}
	return state.Epoch, nil
}
