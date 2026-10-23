// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func (m *claimerRepositoryMock) RefreshClaimObservation(
	ctx context.Context, expected *model.Epoch, hash *common.Hash, block *uint64,
) error {
	return m.Called(ctx, expected, hash, block).Error(0)
}

func (m *claimerBlockchainMock) claimTransactionCanonical(ctx context.Context, hash common.Hash, block uint64) (bool, error) {
	args := m.Called(ctx, hash, block)
	return args.Bool(0), args.Error(1)
}

func (*eventChain) claimTransactionCanonical(context.Context, common.Hash, uint64) (bool, error) {
	return true, nil
}

func (r *eventRepository) RefreshClaimObservation(
	_ context.Context, expected *model.Epoch, hash *common.Hash, block *uint64,
) error {
	stored := r.epoch(expected.ApplicationID, expected.Index)
	if stored == nil || stored.Status != expected.Status ||
		(stored.Status != model.EpochStatus_ClaimSubmitted && stored.Status != model.EpochStatus_ClaimStaged) {
		return repository.ErrNoUpdate
	}
	stored.ClaimTransactionHash = hash
	if block != nil {
		stored.StagedAtBlock = new(*block)
	}
	return nil
}

func TestRecordedQuorumVoteMaintenanceUsesLiveVoteEligibility(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe} {
		t.Run(policy.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t, policy)
			app, epoch := deliveryApplication(1, model.Consensus_Quorum)
			lookup.add(app, epoch)
			backend.apps[app.IApplicationAddress] = app
			require.NoError(t, s.publisher.tick(t.Context()))
			first := backend.published[0]
			epoch.Status = model.EpochStatus_ClaimSubmitted
			backend.last, backend.safe = 52, 51
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 2)
			require.Equal(t, first.Data(), backend.published[1].Data())
			require.Equal(t, first.Nonce(), backend.published[1].Nonce())
		})
	}
}

func TestVanishedClaimStageRepairsHeldAcceptanceButImmaturityWaits(t *testing.T) {
	for _, absent := range []bool{false, true} {
		t.Run(map[bool]string{false: "immature", true: "absent"}[absent], func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Latest)
			app, epoch := deliveryApplication(1, model.Consensus_Authority)
			epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
			app.ClaimStagingPeriod = 10
			lookup.add(app, epoch)
			backend.apps[app.IApplicationAddress] = app
			backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 30)
			require.NoError(t, s.publisher.tick(t.Context()))
			if absent {
				delete(backend.claims, app.ID)
			} else {
				backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 49)
			}
			backend.last = 51
			err := s.publisher.tick(t.Context())
			if !absent {
				require.NoError(t, err)
				require.Len(t, backend.published, 1)
				return
			}
			require.NoError(t, err)
			require.Len(t, backend.published, 2)
			require.Equal(t, claimSubmitMethod, publishedClaimMethod(t, backend.published[1].Data()))
			require.Equal(t, backend.published[0].Nonce(), backend.published[1].Nonce())
		})
	}
}
