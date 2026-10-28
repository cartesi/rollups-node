// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/stretchr/testify/require"
)

func TestClaimPublisherSkipsStoppedApplicationsBeforePagination(t *testing.T) {
	// Exercise the repair and acceptance sources as well as fresh submission.
	for _, status := range []model.EpochStatus{
		model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted, model.EpochStatus_ClaimStaged,
	} {
		t.Run(status.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			for id := int64(1); id <= 40; id++ {
				app, _ := queueDelivery(t, backend, lookup, id)
				app.Status = model.ApplicationStatus_Diverged
			}
			foreclosed, _ := queueDelivery(t, backend, lookup, 41)
			foreclosed.ForecloseBlock = 1
			ready, epoch := queueDelivery(t, backend, lookup, 42)
			for _, epochs := range lookup.epochs {
				for _, stored := range epochs {
					stored.Status = status
				}
			}
			backend.claims[ready.ID] = makeClaimStatus(claimStatusUnstaged, epoch, 0)
			if status == model.EpochStatus_ClaimStaged {
				ready.ClaimStagingPeriod = 0
				backend.claims[ready.ID] = makeClaimStatus(claimStatusStaged, epoch, 1)
			}
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 1, "healthy work must publish in the first pass")
			contractABI, err := iconsensus.IConsensusMetaData.GetAbi()
			require.NoError(t, err)
			method, err := contractABI.MethodById(backend.published[0].Data()[:4])
			require.NoError(t, err)
			args, err := method.Inputs.Unpack(backend.published[0].Data()[4:])
			require.NoError(t, err)
			require.Equal(t, ready.IApplicationAddress, args[0])
		})
	}
}
