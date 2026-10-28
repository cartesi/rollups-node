// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestPRTLostNonceRacesWarnWithoutRetainedBytesAndRecover(t *testing.T) {
	for _, beforePost := range []bool{true, false} {
		t.Run(map[bool]string{true: "stale before POST", false: "nonce-low at POST"}[beforePost], func(t *testing.T) {
			f, _, view := preparePRTAction(t, stageTournament)
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			interfere := true
			consumeNonce := func() {
				if interfere {
					f.backend.nonce++
					updatePRTView(f, f.block.Number+1, view)
				}
			}
			factory := f.d.txOptsFactory
			if beforePost {
				factory = deliverySigningFactory{TransactOptsFactory: factory, afterSign: consumeNonce}
			} else {
				f.backend.publishError = publicationRefusal("nonce too low")
				f.backend.onPublish = func(*types.Transaction) { consumeNonce() }
			}
			var err error
			f.d.sender, err = txsender.New[actionID](f.backend, factory, 31337,
				f.d.deliveryPolicy, f.d.guardPublication, f.d.Logger)
			require.NoError(t, err)
			conflict := func() {
				t.Helper()
				err := f.d.tick(t.Context())
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
			const warning = "PRT publication lost a race while signer nonce advanced; inspect other signer users"
			require.Equal(t, 1, strings.Count(logs.String(), warning), logs.String())
			require.Contains(t, logs.String(), f.app.IApplicationAddress.Hex())
			require.Contains(t, logs.String(), "stage tournament result")
			require.NotContains(t, logs.String(), "level=ERROR")
			f.d.diagnostics.warnedNonce = time.Now().Add(-publicationWarningInterval)
			conflict()
			require.Equal(t, 2, strings.Count(logs.String(), warning))
			if beforePost {
				require.Empty(t, f.backend.published, "a stale preparation must never POST")
			} else {
				require.Len(t, f.backend.published, 7, "one definite refusal ends each pass")
			}

			interfere, f.backend.publishError = false, nil
			logs.Reset()
			before := len(f.backend.published)
			require.NoError(t, f.d.tick(t.Context()), "exclusive use restores publication")
			require.Len(t, f.backend.published, before+1)
			require.Equal(t, f.backend.nonce, f.backend.published[before].Nonce())
			require.NotContains(t, logs.String(), "level=WARN", "normal publication after recovery is quiet")
		})
	}
}

func TestPRTIdleNonceProgressDoesNotWarnAboutSignerContention(t *testing.T) {
	f := newDeliveryFixture(t)
	f.repo.apps = nil
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	for range 4 {
		f.backend.nonce++
		f.backend.advance(f.backend.latest + 1)
		require.NoError(t, f.d.tick(t.Context()))
	}
	require.Empty(t, f.backend.published)
	require.Empty(t, logs.String())
}
