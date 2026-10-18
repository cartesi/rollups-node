// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/common"
)

// These reads use the observer's selected view. Publication at latest cannot
// replace audit evidence saved by a safe/finalized observer.
func (s *Service) refreshSubmittedClaim(ctx context.Context, app *model.Application, epoch *model.Epoch, block uint64) error {
	if epoch.ClaimTransactionHash != nil {
		canonical, err := s.blockchain.claimTransactionCanonical(ctx, *epoch.ClaimTransactionHash, block)
		if err != nil || canonical {
			return err
		}
	}
	_, events, err := s.blockchain.findClaimSubmittedEventAndSucc(ctx, app, epoch, epoch.LastBlock+1, block)
	if err != nil {
		return err
	}
	signer, known := s.blockchain.claimSubmitterAddress()
	for _, event := range events {
		matches, complete := claimSubmittedEventMatches(app, epoch, event)
		if !complete || !matches {
			continue
		}
		if s.submissionEnabled && app.ConsensusType == model.Consensus_Quorum && known && event.Submitter != signer {
			continue
		}
		if event.Raw.BlockNumber > block {
			return fmt.Errorf("%w: epoch %d submission event block %d exceeds observation block %d",
				txsender.ErrInconclusive, epoch.Index, event.Raw.BlockNumber, block)
		}
		return s.refreshClaimMetadata(ctx, epoch, new(event.Raw.TxHash), nil)
	}
	return nil
}

func (s *Service) refreshStagedClaim(
	ctx context.Context, app *model.Application, epoch *model.Epoch, claim iconsensus.IConsensusClaim, block uint64,
) error {
	if claim.StagingBlockNumber == nil || !claim.StagingBlockNumber.IsUint64() || claim.StagingBlockNumber.Uint64() > block {
		return txsender.ErrInconclusive
	}
	if epoch.StagedAtBlock != nil && *epoch.StagedAtBlock == claim.StagingBlockNumber.Uint64() && epoch.ClaimTransactionHash != nil {
		canonical, err := s.blockchain.claimTransactionCanonical(ctx, *epoch.ClaimTransactionHash, block)
		if err != nil || canonical {
			return err
		}
	}
	_, event, _, err := s.blockchain.findClaimStagedEventAndSucc(ctx, app, epoch, epoch.LastBlock+1, block)
	if err != nil {
		return err
	}
	matches, complete := claimStagedEventMatches(app, epoch, event)
	if !complete || !matches || claim.StagingBlockNumber == nil || !claim.StagingBlockNumber.IsUint64() ||
		event.Raw.BlockNumber != claim.StagingBlockNumber.Uint64() || event.Raw.BlockNumber > block {
		return fmt.Errorf("%w: application %s (%s), epoch %d staging state and event disagree at block %d",
			txsender.ErrInconclusive, app.Name, app.IApplicationAddress, epoch.Index, block)
	}
	hash, err := s.stagedClaimAuditHash(ctx, app, epoch, event.Raw.TxHash, block)
	if err != nil {
		return err
	}
	return s.refreshClaimMetadata(ctx, epoch, hash, new(event.Raw.BlockNumber))
}

// A submitting Quorum node records its own matching vote before acceptance.
// The deciding vote belongs to another validator in the usual case. Reader
// mode has no local vote identity and records the matching staging event.
func (s *Service) stagedClaimAuditHash(
	ctx context.Context, app *model.Application, epoch *model.Epoch, stagedHash common.Hash, block uint64,
) (*common.Hash, error) {
	if app.ConsensusType != model.Consensus_Quorum || !s.submissionEnabled {
		return new(stagedHash), nil
	}
	signer, known := s.blockchain.claimSubmitterAddress()
	if !known {
		return nil, txsender.ErrInconclusive
	}
	if epoch.ClaimTransactionHash != nil {
		canonical, err := s.blockchain.claimTransactionCanonical(ctx, *epoch.ClaimTransactionHash, block)
		if err != nil || canonical {
			return epoch.ClaimTransactionHash, err
		}
	}
	_, events, err := s.blockchain.findClaimSubmittedEventAndSucc(ctx, app, epoch, epoch.LastBlock+1, block)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		matches, complete := claimSubmittedEventMatches(app, epoch, event)
		if !complete || !matches || event.Submitter != signer {
			continue
		}
		if event.Raw.BlockNumber > block || event.Raw.TxHash == (common.Hash{}) {
			return nil, txsender.ErrInconclusive
		}
		return new(event.Raw.TxHash), nil
	}
	// Peers can stage a claim after our vote was orphaned. The staging clock
	// remains observable; a missing local vote must not become a peer's hash.
	return nil, nil
}

func (s *Service) refreshClaimMetadata(
	ctx context.Context, epoch *model.Epoch, hash *common.Hash, stagingBlock *uint64,
) error {
	sameHash := epoch.ClaimTransactionHash == nil && hash == nil ||
		epoch.ClaimTransactionHash != nil && hash != nil && *epoch.ClaimTransactionHash == *hash
	if sameHash &&
		(stagingBlock == nil || epoch.StagedAtBlock != nil && *epoch.StagedAtBlock == *stagingBlock) {
		return nil
	}
	if hash != nil && *hash == (common.Hash{}) {
		return fmt.Errorf("%w: epoch %d matching claim event has no transaction hash", txsender.ErrInconclusive, epoch.Index)
	}
	if err := s.repository.RefreshClaimObservation(ctx, epoch, hash, stagingBlock); err != nil {
		return fmt.Errorf("refreshing epoch %d claim audit metadata: %w", epoch.Index, err)
	}
	epoch.ClaimTransactionHash = hash
	if stagingBlock != nil {
		epoch.StagedAtBlock = new(*stagingBlock)
	}
	return nil
}
