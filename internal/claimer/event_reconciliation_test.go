// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

// This fixture stores the domain rows only. No receipt or sender state can
// advance an epoch. Each page is selected again after its preceding DB writes.
type eventRepository struct {
	*deliveryCandidateRepository
	failStageOnce bool
}

func (r *eventRepository) SelectClaimsToStagePage(_ context.Context, after repository.ClaimCursor, limit uint64) (
	repository.ClaimPage, error,
) {
	return r.page(after, limit, model.EpochStatus_ClaimSubmitted)
}
func (*eventRepository) ListApplicationsByID(context.Context, repository.ApplicationFilter, int64, int64, uint64) (
	[]*model.Application, error,
) {
	return nil, nil
}
func (r *eventRepository) epoch(id int64, index uint64) *model.Epoch {
	for address, app := range r.lookup.apps {
		if app.ID == id {
			return r.lookup.epochs[address][index]
		}
	}
	return nil
}
func (r *eventRepository) UpdateEpochWithSubmittedClaim(_ context.Context, id int64, index uint64, hash common.Hash) error {
	e := r.epoch(id, index)
	e.Status, e.ClaimTransactionHash = model.EpochStatus_ClaimSubmitted, new(hash)
	return nil
}
func (r *eventRepository) UpdateEpochToStaged(_ context.Context, id int64, index, block uint64) error {
	if r.failStageOnce {
		r.failStageOnce = false
		return errors.New("stage commit unavailable")
	}
	e := r.epoch(id, index)
	e.Status, e.StagedAtBlock = model.EpochStatus_ClaimStaged, new(block)
	return nil
}
func (r *eventRepository) UpdateEpochWithAcceptedClaim(_ context.Context, id int64, index uint64, hash *common.Hash) error {
	e := r.epoch(id, index)
	e.Status = model.EpochStatus_ClaimAccepted
	if hash != nil {
		e.ClaimTransactionHash = new(*hash)
	}
	return nil
}

type eventChain struct {
	accept bool
}

func (*eventChain) getDefaultBlockNumber(context.Context) (*big.Int, error) {
	return big.NewInt(100), nil
}
func (*eventChain) getConsensusAddress(_ context.Context, app *model.Application, _ *big.Int) (common.Address, error) {
	return app.IConsensusAddress, nil
}
func (*eventChain) claimSubmitterAddress() (common.Address, bool) { return common.Address{}, false }
func (*eventChain) findClaimSubmittedEventAndSucc(_ context.Context, app *model.Application, epoch *model.Epoch, _, _ uint64) (
	*iconsensus.IConsensus, []*iconsensus.IConsensusClaimSubmitted, error,
) {
	return nil, []*iconsensus.IConsensusClaimSubmitted{makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0xabc"))}, nil
}
func (*eventChain) findClaimStagedEventAndSucc(_ context.Context, app *model.Application, epoch *model.Epoch, _, _ uint64) (
	*iconsensus.IConsensus, *iconsensus.IConsensusClaimStaged, *iconsensus.IConsensusClaimStaged, error,
) {
	return nil, makeStagedEvent(app, epoch), nil, nil
}
func (c *eventChain) findClaimAcceptedEventAndSucc(_ context.Context, app *model.Application, epoch *model.Epoch, _, _ uint64) (
	*iconsensus.IConsensus, *iconsensus.IConsensusClaimAccepted, *iconsensus.IConsensusClaimAccepted, error,
) {
	if !c.accept {
		return nil, nil, nil, nil
	}
	event := makeAcceptedEvent(app, epoch)
	event.Raw.TxHash = common.HexToHash("0xdef")
	return nil, event, nil, nil
}
func (*eventChain) getClaimStatus(_ context.Context, _ *model.Application, epoch *model.Epoch, _ *big.Int) (
	iconsensus.IConsensusClaim, error,
) {
	return makeClaimStatus(claimStatusStaged, epoch, 45), nil
}

func TestEventOnlyReconciliationSurvivesPagingAndRestartAfterStageWriteFailure(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		t.Run(consensus.String(), func(t *testing.T) {
			s, _, lookup := newDeliveryFixture(t)
			repo := &eventRepository{deliveryCandidateRepository: &deliveryCandidateRepository{lookup: lookup}, failStageOnce: true}
			chain := &eventChain{}
			s.repository, s.blockchain = repo, chain
			for id := int64(1); id <= 17; id++ {
				app, epoch := deliveryApplication(id, consensus)
				lookup.add(app, epoch)
			}
			_, err := s.Tick(t.Context())
			require.ErrorContains(t, err, "stage commit unavailable")
			var pending int
			for address, app := range lookup.apps {
				epoch := lookup.epochs[address][3]
				if epoch.Status == model.EpochStatus_ClaimSubmitted {
					pending++
					require.Equal(t, common.HexToHash("0xabc"), *epoch.ClaimTransactionHash)
				}
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
			}
			require.Equal(t, 1, pending, "only the failed stage write remains submitted")
			// A new observer has no volatile hint, receipt or page cursor to recover.
			restarted, _, _ := newServiceMock(t)
			restarted.repository, restarted.blockchain = repo, chain
			chain.accept = true
			for range 18 {
				_, err = restarted.Tick(t.Context())
				require.NoError(t, err)
			}
			for address, app := range lookup.apps {
				epoch := lookup.epochs[address][3]
				require.Equal(t, model.EpochStatus_ClaimAccepted, epoch.Status)
				require.Equal(t, common.HexToHash("0xdef"), *epoch.ClaimTransactionHash)
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
			}
		})
	}
}
