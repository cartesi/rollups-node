// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStagedAcceptanceDoesNotDependOnComputedObservationCompletion(t *testing.T) {
	s, b, r := newDeliveryFixture(t)
	app, computed := queueDelivery(t, b, r, 1)
	staged := makeStagedEpoch(app, 1, 20)
	r.add(app, staged)
	b.claims[app.ID] = makeClaimStatus(claimStatusStaged, staged, 20)
	// The failed new submission cannot complete at live; an independently ready
	// acceptance remains selectable from committed rows.
	computed.Status = model.EpochStatus_ClaimSubmitted
	failClaimObservation(t, s)
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, b.published, 1)
	require.Equal(t, "acceptClaim", publishedClaimMethod(t, b.published[0].Data()))
}

func TestUnsupportedProtectedReadPausesOnlyTheCurrentPublicationIteration(t *testing.T) {
	s, b, r := newDeliveryFixture(t)
	app, _ := queueDelivery(t, b, r, 1)
	b.ownerErr = txsender.ErrUnsupported
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrUnsupported)
	require.Empty(t, b.published)
	require.Nil(t, s.failure.Load())
	b.ownerErr = nil
	clear(s.publisher.cooldowns)
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, b.published, 1)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
}
