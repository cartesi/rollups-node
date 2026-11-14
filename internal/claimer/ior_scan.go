// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/common"
)

type iorScanKey struct {
	applicationID int64
	epochIndex    uint64
	phase         int
}

type iorClaimIdentity struct {
	application, consensus common.Address
	lastBlock              uint64
	machine, outputs       common.Hash
}

type iorScanProgress struct {
	through  txsender.BlockRef
	identity iorClaimIdentity
	seen     bool
}

// This read belongs to one visit. Event evidence keeps precedence on a complete
// scan; a scan failure can still use the point evidence without hiding its error.
type iorClaimObservation struct {
	key     iorScanKey
	claim   iconsensus.IConsensusClaim
	at      txsender.BlockRef
	readErr error
	from    uint64
	covered bool
}

func iorIdentity(work claimWork) iorClaimIdentity {
	identity := iorClaimIdentity{
		application: work.app.IApplicationAddress, consensus: work.app.IConsensusAddress,
		lastBlock: work.epoch.LastBlock,
	}
	if work.epoch.MachineHash != nil {
		identity.machine = *work.epoch.MachineHash
	}
	if work.epoch.TxBufferDataBlock != nil {
		identity.outputs = *work.epoch.TxBufferDataBlock
	}
	return identity
}

func (work claimWork) scanFromBlock() uint64 {
	if work.ior != nil {
		return work.ior.from
	}
	return work.epoch.LastBlock + 1
}

func (work claimWork) scanCovered() bool {
	return work.ior != nil && work.ior.covered
}

func (s *Service) prepareIORObservation(
	ctx context.Context, work claimWork, phase int, head uint64,
) (claimWork, error) {
	if err := ctx.Err(); err != nil {
		return work, err
	}
	observation := &iorClaimObservation{
		key: iorScanKey{work.app.ID, work.epoch.Index, phase}, from: work.epoch.LastBlock + 1,
	}
	work.ior = observation
	var referenceErr error
	observation.at, referenceErr = s.blockchain.claimReference(ctx, head)
	if referenceErr == nil && (observation.at.Number != head || observation.at.Hash == (common.Hash{})) {
		referenceErr = txsender.ErrInconclusive
	}
	observation.claim, observation.readErr = s.blockchain.getClaimStatus(ctx, work.app, work.epoch, new(big.Int).SetUint64(head))
	observation.readErr = errors.Join(referenceErr, observation.readErr)
	if observation.readErr != nil {
		observation.readErr = fmt.Errorf("reading IOR claim before event scan (epoch %d): %w", work.epoch.Index, observation.readErr)
	}
	// Provisional reads can cross branches during a visit. Do not retain
	// coverage from them; the next visit must read the history again.
	if s.defaultBlock != model.DefaultBlock_Finalized {
		return work, nil
	}
	previous, ok := s.iorScans[observation.key]
	if !ok {
		return work, nil
	}
	if previous.identity != iorIdentity(work) || previous.through.Hash == (common.Hash{}) {
		delete(s.iorScans, observation.key)
		return work, nil
	}
	if err := s.blockchain.checkClaimReference(ctx, previous.through); err != nil {
		if errors.Is(err, txsender.ErrNonCanonical) {
			delete(s.iorScans, observation.key)
			return work, fmt.Errorf("%w: IOR scan prefix at block %d is no longer canonical", txsender.ErrFinality, previous.through.Number)
		}
		// Unavailable is not non-canonical. Keep this prefix for a retry,
		// but do not reuse it or replay history while its check has failed.
		return work, err
	}
	observation.covered = previous.through.Number == head
	if !observation.covered {
		observation.from = previous.through.Number + 1
	}
	return work, nil
}

func (s *Service) completeIORObservation(
	ctx context.Context, work claimWork, result claimStepResult,
) claimStepResult {
	result.err = errors.Join(result.err, work.ior.readErr, ctx.Err())
	if result.progress > 0 {
		delete(s.iorScans, work.ior.key)
		return result
	}
	if result.err != nil {
		return result
	}
	if s.defaultBlock != model.DefaultBlock_Finalized {
		return result
	}
	through := work.ior.at
	err := s.blockchain.checkClaimReference(ctx, through)
	err = errors.Join(err, ctx.Err())
	if err != nil {
		result.err = err
		return result
	}
	if s.iorScans == nil {
		s.iorScans = make(map[iorScanKey]iorScanProgress)
	}
	s.iorScans[work.ior.key] = iorScanProgress{through: through, identity: iorIdentity(work), seen: true}
	return result
}

func (s *Service) projectIORObservation(
	ctx context.Context, work claimWork, block *big.Int,
) (claimStepResult, bool) {
	if err := ctx.Err(); err != nil {
		return claimRetryLater(err), true
	}
	if work.ior.readErr != nil {
		// The deferred completion retains the point-read error. Do not treat
		// an unknown point state as UNSTAGED or foreclose it.
		return claimNoProgress(), true
	}
	if work.ior.claim.Status != claimStatusUnstaged || work.app.ForecloseBlock != 0 {
		// The point read precedes potentially long, fallible historical reads.
		// Validate its original hash, not a newly resolved hash at this height.
		if err := s.blockchain.checkClaimReference(ctx, work.ior.at); err != nil {
			return claimRetryLater(err), true
		}
		if err := ctx.Err(); err != nil {
			return claimRetryLater(err), true
		}
	}
	reconciled, err := s.reconcileObservedClaimStatus(ctx, work.app, work.epoch, work.ior.claim, block)
	if err != nil {
		return claimDropped(err), true
	}
	if !reconciled {
		return claimNoProgress(), false
	}
	result := claimWorkCompleted(1)
	if s.defaultBlock.Provisional() && work.ior.claim.Status == claimStatusStaged {
		// Projection and its diagnostic are already committed. Audit repair
		// must not prevent the operator from seeing the verified stage clock.
		staged := *work.epoch
		staged.Status = model.EpochStatus_ClaimStaged
		staged.StagedAtBlock = new(work.ior.claim.StagingBlockNumber.Uint64())
		result.err = s.refreshStagedClaim(ctx, work.app, &staged, work.ior.claim, block.Uint64())
	}
	return result, true
}

func (s *Service) fallbackIORObservation(
	ctx context.Context, work claimWork, block *big.Int, scanErr error,
) claimStepResult {
	if txsender.HardFailure(scanErr) {
		return claimDropped(scanErr)
	}
	if work.ior != nil {
		if result, done := s.projectIORObservation(ctx, work, block); done {
			result.err = errors.Join(scanErr, result.err)
			return result
		}
	}
	return claimDropped(scanErr)
}

// Existing per-status application rotation supplies the lifetime boundary.
// Mark every cached epoch of that app, including tails outside this page.
func (s *Service) visitIORScanApplication(phase int, after, current int64) {
	for key, progress := range s.iorScans {
		if key.phase != phase {
			continue
		}
		if current == 0 {
			delete(s.iorScans, key)
			continue
		}
		if current <= after {
			if !progress.seen {
				delete(s.iorScans, key)
				continue
			}
			progress.seen = false
		}
		if key.applicationID == current {
			progress.seen = true
		}
		s.iorScans[key] = progress
	}
}
