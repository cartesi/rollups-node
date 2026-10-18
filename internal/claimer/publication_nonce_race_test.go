// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/require"
)

func TestClaimLostNonceRacesWarnWithoutRetainedBytesAndRecover(t *testing.T) {
	for _, beforePost := range []bool{true, false} {
		t.Run(map[bool]string{true: "stale before POST", false: "nonce-low at POST"}[beforePost], func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, _ := queueDelivery(t, backend, lookup, 1)
			logs := capturePublicationLogs(s)
			reads, simulations := 0, 0
			consumeNonce := func() {
				backend.last++
				backend.consumedAt = backend.last
				backend.nonce++
			}
			if beforePost {
				backend.onHeader = func(tag string) {
					if tag == "latest" {
						reads++
						if reads == 2 {
							consumeNonce()
						}
					}
				}
			} else {
				backend.publishErr = publicationRefusal("nonce too low")
				backend.onCall = func(method string) {
					if method == claimSubmitMethod {
						simulations++
						if simulations == 2 {
							consumeNonce()
						}
					}
				}
			}
			conflict := func() {
				t.Helper()
				reads, simulations = 0, 0
				err := s.publisher.tick(t.Context())
				require.Error(t, err)
				require.True(t, txsender.WaitOnly(err), "%v", err)
				if beforePost {
					require.ErrorIs(t, err, txsender.ErrStale)
				} else {
					var reply *txsender.PublicationError
					require.ErrorAs(t, err, &reply)
					require.True(t, reply.NonceLowOnly())
				}
			}
			conflict()
			require.NotContains(t, logs.String(), "level=WARN", "one race is not a repeated nonce-progress symptom")
			for range 5 {
				conflict()
			}
			const warning = "Claim publication lost a race while signer nonce advanced; inspect other signer users"
			require.Equal(t, 1, strings.Count(logs.String(), warning), logs.String())
			require.Contains(t, logs.String(), app.IApplicationAddress.Hex())
			require.Contains(t, logs.String(), "authority-submit")
			require.NotContains(t, logs.String(), "level=ERROR")
			s.publisher.warnedAt = time.Now().Add(-claimWarningInterval)
			conflict()
			require.Equal(t, 2, strings.Count(logs.String(), warning))
			if beforePost {
				require.Empty(t, backend.published, "a stale preparation must never POST")
			} else {
				require.Len(t, backend.published, 7, "one definite refusal ends each pass")
			}

			backend.onHeader, backend.onCall, backend.publishErr = nil, nil, nil
			logs.Reset()
			before := len(backend.published)
			require.NoError(t, s.publisher.tick(t.Context()), "exclusive use restores publication")
			require.Len(t, backend.published, before+1)
			require.Equal(t, backend.nonce, backend.published[before].Nonce())
			require.NotContains(t, logs.String(), "level=WARN", "normal publication after recovery is quiet")
		})
	}
}

func TestClaimIdleNonceProgressDoesNotWarnAboutSignerContention(t *testing.T) {
	s, backend, _ := newDeliveryFixture(t)
	logs := capturePublicationLogs(s)
	for range 4 {
		backend.last++
		backend.consumedAt, backend.nonce = backend.last, backend.nonce+1
		require.NoError(t, s.publisher.tick(t.Context()))
	}
	require.Empty(t, backend.published)
	require.Empty(t, logs.String())
}
