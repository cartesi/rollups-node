// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaimSelectionFailureDoesNotBlockHeldMaintenanceOrCompletion(t *testing.T) {
	for _, completed := range []bool{false, true} {
		name := "maintenance"
		if completed {
			name = "completion"
		}
		t.Run(name, func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := queueDelivery(t, backend, lookup, 1)
			require.NoError(t, s.publisher.tick(t.Context()))
			lookup.pageErr = errors.New("selection unavailable")
			backend.last++
			if completed {
				backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 40)
			}
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 2)
			require.Equal(t, backend.published[0].Nonce(), backend.published[1].Nonce())
			if completed {
				require.Empty(t, backend.published[1].Data())
			} else {
				require.Equal(t, backend.raw[0], backend.raw[1])
			}
		})
	}
}

func TestClaimSelectionFailureIsVisibleAndDoesNotLoseCommittedCandidate(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	queueDelivery(t, backend, lookup, 1)
	lookup.pageErr = errors.New("database selection unavailable")
	require.ErrorIs(t, s.publisher.tick(t.Context()), lookup.pageErr)
	require.Empty(t, backend.published)
	lookup.pageErr = nil
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 1)
}
