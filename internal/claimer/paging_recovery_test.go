// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestOlderUnreadyClaimCannotHideLaterAcceptanceOrVoteRepair(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_ClaimStaged, model.EpochStatus_ClaimSubmitted} {
		t.Run(status.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Latest)
			app, older := deliveryApplication(1, model.Consensus_Quorum)
			app.ClaimStagingPeriod = 10
			older.Status, older.StagedAtBlock = status, new(uint64(49))
			later := makeComputedEpoch(app, older.Index+1)
			later.Status, later.StagedAtBlock = status, new(uint64(30))
			lookup.add(app, older)
			lookup.add(app, later)
			backend.apps[app.IApplicationAddress] = app
			backend.claimsAt = map[uint64]iconsensus.IConsensusClaim{}
			if status == model.EpochStatus_ClaimStaged {
				backend.claimsAt[older.LastBlock] = makeClaimStatus(claimStatusStaged, older, 49)
				backend.claimsAt[later.LastBlock] = makeClaimStatus(claimStatusStaged, later, 30)
			} else {
				older.StagedAtBlock, later.StagedAtBlock = nil, nil
				backend.votesAt = map[uint64]bool{older.LastBlock: true, later.LastBlock: false}
				backend.matchingVotesAt = map[uint64]bool{older.LastBlock: true}
			}
			for range 3 {
				_ = s.publisher.tick(t.Context())
				if len(backend.published) != 0 {
					break
				}
			}
			require.Len(t, backend.published, 1)
			contractABI, err := iconsensus.IConsensusMetaData.GetAbi()
			require.NoError(t, err)
			method, err := contractABI.MethodById(backend.published[0].Data()[:4])
			require.NoError(t, err)
			args, err := method.Inputs.Unpack(backend.published[0].Data()[4:])
			require.NoError(t, err)
			require.Equal(t, later.LastBlock, args[1].(*big.Int).Uint64())
			if status == model.EpochStatus_ClaimStaged {
				require.Equal(t, "acceptClaim", method.Name)
			} else {
				require.Equal(t, "submitClaim", method.Name)
			}
		})
	}
}
