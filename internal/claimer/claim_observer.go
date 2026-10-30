// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
)

type claimObservationRepository interface {
	ListApplicationsByID(context.Context, repository.ApplicationFilter, int64, int64, uint64) ([]*model.Application, error)
	GetApplication(context.Context, string) (*model.Application, error)
	GetEventLastCheckBlock(context.Context, int64, model.MonitoredEvent) (uint64, error)
	UpdateEventLastCheckBlock(context.Context, []int64, model.MonitoredEvent, uint64) error
	ReadClaimRoots(context.Context, int64, uint64) (repository.ClaimRoots, error)
	appstatus.Repository
}

type claimObservationBlockchain interface {
	scanClaims(context.Context, *model.Application, uint64, *claimRead, uint64) (*claimScan, error)
	checkClaimRead(context.Context, txsender.BlockRef) error
	claimForeclosureFinalized(context.Context, *model.Application, txsender.BlockRef) (bool, error)
}

type pendingObservedClaim struct {
	observedClaim
	element *list.Element
	reason  string
	cause   string
}

type claimObservationPhase uint8

const (
	claimObservationOld claimObservationPhase = iota
	claimObservationFetch
	claimObservationFresh
	claimObservationCheckpoint
)

// All volatile facts belong to this unit. Eviction discards the unit; its next
// visit reloads the durable safe prefix and reconstructs every later claim.
type claimObservation struct {
	safe          uint64
	loaded        bool
	read          *claimRead
	pending       map[observedClaimKey]*pendingObservedClaim
	order         list.List
	fresh         []observedClaimKey
	verdict       *pendingObservedClaim
	phase         claimObservationPhase
	foreclosureAt txsender.BlockRef
	foreclosed    bool
	warnings      [claimWarningKinds]claimWarning
}

type claimWarningKind uint8

const (
	claimWarningPending claimWarningKind = iota
	claimWarningObservation
	claimWarningCheckpoint
	claimWarningKinds
)

type claimWarning struct {
	cause string
	at    time.Time
}

type claimVisitProgress struct {
	advanced  bool
	completed bool
}

type independentClaimObserver struct {
	repository        claimObservationRepository
	blockchain        claimObservationBlockchain
	logger            *slog.Logger
	policy            model.DefaultBlock
	maxRange          uint64
	budget            time.Duration
	after             int64
	apps              map[int64]*claimObservation
	selectionWarnedAt time.Time
}

var errClaimObservationInactive = errors.New("claim observation application is inactive")

func (o *independentClaimObserver) observe(ctx context.Context, head uint64) (bool, error) {
	started := time.Now()
	visited := map[int64]bool{}
	reschedule := false
	for len(visited) < int(claimPageSize) && (len(visited) == 0 || time.Since(started) < o.budget) {
		filter := repository.ApplicationFilter{Enabled: new(true),
			ConsensusTypes: []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum}}
		apps, err := o.repository.ListApplicationsByID(ctx, filter, o.after, math.MaxInt64, 1)
		if err == nil && len(apps) == 0 && o.after != 0 {
			apps, err = o.repository.ListApplicationsByID(ctx, filter, 0, math.MaxInt64, 1)
		}
		if err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			if time.Since(o.selectionWarnedAt) >= claimWarningInterval {
				o.logger.Warn("Cannot select applications for claim observation", "error", err)
				o.selectionWarnedAt = time.Now()
			}
			return reschedule, nil
		}
		if len(apps) == 0 {
			o.apps = nil
			o.after = 0
			break
		}
		app := apps[0]
		// An eligible-ID gap proves these cached owners were removed or disabled.
		// Dropping the whole unit is safe because the durable R has not moved.
		for id := range o.apps {
			passed := o.after < id && id <= app.ID
			if app.ID <= o.after {
				passed = o.after < id || id <= app.ID
			}
			if passed && id != app.ID {
				delete(o.apps, id)
			}
		}
		if visited[app.ID] {
			break
		}
		visited[app.ID] = true
		o.after = app.ID
		state := o.apps[app.ID]
		if state == nil {
			state = &claimObservation{pending: map[observedClaimKey]*pendingObservedClaim{}}
			if o.apps == nil {
				o.apps = map[int64]*claimObservation{}
			}
			o.apps[app.ID] = state
		}
		if !state.loaded {
			safe, readErr := o.repository.GetEventLastCheckBlock(ctx, app.ID, model.MonitoredEvent_ClaimAccepted)
			if readErr != nil {
				if ctx.Err() != nil {
					return false, ctx.Err()
				}
				if errors.Is(readErr, repository.ErrNotFound) {
					delete(o.apps, app.ID)
					continue
				}
				o.warn(app, state, claimWarningCheckpoint, "checkpoint_read", readErr.Error())
				continue
			}
			state.safe, state.loaded = safe, true
		}
		progressed, visitErr := o.visit(ctx, app, state)
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if txsender.HardFailure(visitErr) {
			return false, visitErr
		}
		if errors.Is(visitErr, errClaimObservationInactive) || errors.Is(visitErr, repository.ErrNotFound) {
			delete(o.apps, app.ID)
			continue
		}
		if errors.Is(visitErr, txsender.ErrNonCanonical) {
			delete(o.apps, app.ID)
		}
		if visitErr != nil {
			if errors.Is(visitErr, errClaimDeploymentPending) {
				o.logger.Debug("Claim observation waits for deployment", "application", app.Name)
			} else {
				o.warn(app, state, claimWarningObservation, claimErrorCause(visitErr), visitErr.Error())
			}
		} else if len(state.pending) > 0 {
			claim := state.example()
			reason, cause := claim.reason, claim.cause
			if reason == "" {
				reason = "input coverage or local validation is incomplete"
			}
			o.warn(app, state, claimWarningPending, cause, reason)
		}
		reschedule = reschedule || progressed.advanced && state.read != nil && state.read.At.Number < head ||
			progressed.completed && len(state.pending) > 0
	}
	return reschedule, nil
}

// Admission is soft and applies between app visits. Complete this finite unit
// without a poll-derived child deadline. Ordinary I/O timeouts and shutdown apply.
func (o *independentClaimObserver) visit(ctx context.Context, app *model.Application, s *claimObservation) (claimVisitProgress, error) {
	progressed := claimVisitProgress{}
	if s.read != nil {
		if err := o.blockchain.checkClaimRead(ctx, s.read.At); err != nil {
			return progressed, err
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return progressed, err
		}
		if s.verdict != nil {
			if err := o.persistMismatch(ctx, app, s.verdict.reason); err != nil {
				return progressed, err
			}
			s.remove(s.verdict)
			s.verdict = nil
			progressed.completed = true
		}
		switch s.phase {
		case claimObservationOld:
			if first := s.order.Front(); first != nil {
				claim := first.Value.(*pendingObservedClaim)
				before := len(s.pending)
				if err := o.evaluate(ctx, app, s, claim); err != nil {
					s.order.MoveToBack(claim.element)
					s.phase = claimObservationFetch
					return progressed, err
				}
				progressed.completed = progressed.completed || len(s.pending) < before
			}
			s.phase = claimObservationFetch
		case claimObservationFetch:
			cursor := s.safe
			if s.read != nil {
				cursor = s.read.At.Number
			}
			batch, err := o.blockchain.scanClaims(ctx, app, cursor, s.read, o.maxRange)
			if err != nil {
				s.phase = claimObservationOld
				return progressed, fmt.Errorf("scan claim events: %w", err)
			}
			if batch != nil {
				if err = s.adopt(batch); err != nil {
					s.phase = claimObservationOld
					return progressed, err
				}
				progressed.advanced = progressed.advanced || batch.Read.At.Number > cursor
			}
			s.phase = claimObservationFresh
		case claimObservationFresh:
			if len(s.fresh) == 0 {
				s.phase = claimObservationCheckpoint
				continue
			}
			key := s.fresh[0]
			if claim := s.pending[key]; claim != nil {
				before := len(s.pending)
				if err := o.evaluate(ctx, app, s, claim); err != nil {
					s.order.MoveToBack(claim.element)
					s.fresh = s.fresh[1:]
					if len(s.fresh) == 0 {
						s.phase = claimObservationCheckpoint
					}
					return progressed, err
				}
				progressed.completed = progressed.completed || len(s.pending) < before
			}
			s.fresh = s.fresh[1:]
			if len(s.fresh) == 0 {
				s.phase = claimObservationCheckpoint
			}
		case claimObservationCheckpoint:
			safe := s.safe
			if s.read != nil {
				safe = s.read.At.Number
			}
			for _, claim := range s.pending {
				if claim.Stage == nil || claim.Stage.BlockNumber == 0 {
					return progressed, txsender.ErrInconclusive
				}
				if claim.Stage.BlockNumber <= safe {
					safe = claim.Stage.BlockNumber - 1
				}
			}
			if safe > s.safe {
				if err := o.repository.UpdateEventLastCheckBlock(
					ctx, []int64{app.ID}, model.MonitoredEvent_ClaimAccepted, safe); err != nil {
					s.phase = claimObservationOld
					return progressed, fmt.Errorf("save claim checkpoint: %w", err)
				}
				s.safe = safe
			}
			s.phase = claimObservationOld
			return progressed, nil
		}
	}
}

func (s *claimObservation) adopt(batch *claimScan) error {
	// Validate all cross-range merges before changing any cached fact or read.
	for _, fact := range batch.Claims {
		if old := s.pending[fact.Key]; old != nil && (old.Outputs != fact.Outputs || fact.Stage != nil ||
			fact.Acceptance != nil && old.Acceptance != nil) {
			return txsender.ErrInconclusive
		}
	}
	s.fresh = nil
	for _, fact := range batch.Claims {
		claim := s.pending[fact.Key]
		if claim == nil && fact.Stage != nil {
			claim = &pendingObservedClaim{observedClaim: fact}
			claim.element = s.order.PushBack(claim)
			s.pending[fact.Key] = claim
		}
		if claim == nil {
			continue
		} // completed before this acceptance; still counted by scanner
		if fact.Acceptance != nil {
			claim.Acceptance = fact.Acceptance
		}
		s.fresh = append(s.fresh, fact.Key)
	}
	read := batch.Read
	s.read = &read
	return nil
}

func (s *claimObservation) remove(claim *pendingObservedClaim) {
	delete(s.pending, claim.Key)
	s.order.Remove(claim.element)
}

func (o *independentClaimObserver) evaluate(ctx context.Context, app *model.Application, s *claimObservation,
	claim *pendingObservedClaim) error {
	covered, ok := new(big.Int).SetString(claim.Key.Covered, claimDecimalRadix)
	if !ok {
		return txsender.ErrInconclusive
	}
	roots := repository.ClaimRoots{ReasonClass: "covered_domain", Reason: "covered block exceeds the local uint64 input domain"}
	var err error
	if covered.IsUint64() {
		roots, err = o.repository.ReadClaimRoots(ctx, app.ID, covered.Uint64())
	}
	if err != nil {
		return fmt.Errorf("read local roots for covered block %s: %w", claim.Key.Covered, err)
	}
	claim.reason, claim.cause = roots.Reason, roots.ReasonClass
	switch {
	case roots.Ready:
		if roots.MachineHash == claim.Key.Machine && roots.OutputsRoot == claim.Outputs {
			s.remove(claim)
			return nil
		}
		claim.reason = fmt.Sprintf("independent_claim_mismatch: covered block %s, on-chain machine=%s outputs=%s, "+
			"validated local machine=%s outputs=%s",
			claim.Key.Covered, claim.Key.Machine, claim.Outputs, roots.MachineHash, roots.OutputsRoot)
		s.verdict = claim
		phase := "staged"
		if claim.Acceptance != nil {
			phase = "accepted"
		}
		o.logger.Error("Divergent on-chain claim observed", "source", "independent_claim_observer", "application", app.Name,
			"address", app.IApplicationAddress, "consensus", app.IConsensusAddress,
			"covered_block", claim.Key.Covered, "machine_merkle_root", claim.Key.Machine, "outputs_merkle_root", claim.Outputs,
			"comparison", "mismatch", "reason", claim.reason, "phase", phase, "observation_policy", o.policy,
			"source_block", claim.Stage.BlockNumber, "source_block_hash", claim.Stage.BlockHash,
			"source_transaction", claim.Stage.TxHash, "staging_contract_block", claim.StagingBlock,
			"staging_period", claim.Period, "acceptance_from_contract_block", claim.acceptanceFrom())
	case claim.Acceptance == nil && o.policy == model.DefaultBlock_Finalized && app.ForecloseBlock != 0 && s.read != nil:
		// Only the flag at the certified read excludes a later acceptance. The
		// evmreader's stored foreclosure height is a hint, not a semantic bound.
		if s.foreclosureAt != s.read.At {
			foreclosed, readErr := o.blockchain.claimForeclosureFinalized(ctx, app, s.read.At)
			if readErr != nil {
				return fmt.Errorf("read foreclosure at block %d: %w", s.read.At.Number, readErr)
			}
			s.foreclosureAt, s.foreclosed = s.read.At, foreclosed
		}
		if s.foreclosed {
			s.remove(claim)
			return nil
		}
	} // A pending cause alone never authorizes a status write.
	s.order.MoveToBack(claim.element)
	return nil
}

func (o *independentClaimObserver) persistMismatch(ctx context.Context, app *model.Application, reason string) error {
	current, err := o.repository.GetApplication(ctx, app.IApplicationAddress.Hex())
	if err != nil {
		return err
	}
	if current == nil || current.ID != app.ID || !current.Enabled {
		return errClaimObservationInactive
	}
	if current.Status == model.ApplicationStatus_Diverged || current.Status == model.ApplicationStatus_Corrupted {
		return nil
	}
	writeErr := appstatus.SetDiverged(ctx, o.logger, o.repository, current, reason)
	// A concurrent integrity writer or a committed write with a lost response
	// is success only after current durable state confirms its integrity effect.
	confirmed, readErr := o.repository.GetApplication(ctx, app.IApplicationAddress.Hex())
	if readErr != nil {
		return errors.Join(writeErr, readErr)
	}
	if confirmed == nil || confirmed.ID != app.ID || !confirmed.Enabled {
		return errClaimObservationInactive
	}
	if confirmed.Status == model.ApplicationStatus_Diverged || confirmed.Status == model.ApplicationStatus_Corrupted {
		return nil
	}
	return writeErr
}

// Choose the oldest source independently of FIFO rotation and map iteration.
func (s *claimObservation) example() *pendingObservedClaim {
	var first *pendingObservedClaim
	for _, claim := range s.pending {
		if first == nil || claim.Stage.BlockNumber < first.Stage.BlockNumber ||
			claim.Stage.BlockNumber == first.Stage.BlockNumber && (claim.Key.Covered < first.Key.Covered ||
				claim.Key.Covered == first.Key.Covered && claim.Key.Machine.Hex() < first.Key.Machine.Hex()) {
			first = claim
		}
	}
	return first
}

func claimErrorCause(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, txsender.ErrNonCanonical):
		return "noncanonical"
	case errors.Is(err, txsender.ErrInconclusive):
		return "inconclusive"
	default:
		return "dependency"
	}
}

func (o *independentClaimObserver) warn(app *model.Application, s *claimObservation,
	kind claimWarningKind, cause, reason string) {
	warning := &s.warnings[kind]
	if !warning.at.IsZero() && warning.cause == cause && time.Since(warning.at) < claimWarningInterval {
		return
	}
	warning.cause, warning.at = cause, time.Now()
	var read, safe any = "unscanned", "unloaded"
	if s.loaded {
		safe = s.safe
	}
	if s.read != nil {
		read = s.read.At.Number
	}
	fields := []any{"source", "independent_claim_observer", "application", app.Name,
		"address", app.IApplicationAddress, "safe_claim_block", safe, "read_claim_block", read,
		"pending_claims", len(s.pending), "observation_policy", o.policy, "cause_class", cause, "cause", reason}
	if claim := s.example(); claim != nil {
		phase := "staged"
		if claim.Acceptance != nil {
			phase = "accepted"
		}
		fields = append(fields, "example_covered_block", claim.Key.Covered, "example_machine_root", claim.Key.Machine,
			"example_outputs_root", claim.Outputs, "example_phase", phase, "acceptance_from_contract_block", claim.acceptanceFrom())
	}
	o.logger.Warn("Independent claim verification is incomplete", fields...)
}
