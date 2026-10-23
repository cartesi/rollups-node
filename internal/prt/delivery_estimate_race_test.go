// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type advancingPRTEstimator struct {
	*deliveryBackendFake
	duringEstimate func()
	raceBlock      txsender.BlockRef
}

func (b *advancingPRTEstimator) Estimate(ctx context.Context, msg ethereum.CallMsg,
	block txsender.BlockRef) (uint64, error) {
	if advance := b.duringEstimate; advance != nil {
		b.raceBlock = block
		b.duringEstimate = nil
		advance()
	}
	// The protected historical estimate still sees the pre-completion state.
	// Estimating against latest would instead revert on the completed join or epoch.
	return b.deliveryBackendFake.Estimate(ctx, msg, block)
}

func TestPRTPinnedEstimateSurvivesConcurrentActionCompletion(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe, model.DefaultBlock_Finalized} {
		for _, kind := range []actionKind{joinTournament, acceptTournament} {
			for _, ownInclusion := range []bool{false, true} {
				name := policy.String() + "/" + actionName(kind) + "/" +
					map[bool]string{false: "competitor completes action", true: "our transaction is included"}[ownInclusion]
				t.Run(name, func(t *testing.T) {
					f, call, view := preparePRTAction(t, kind)
					f.d.defaultBlock = policy
					backend := &advancingPRTEstimator{deliveryBackendFake: f.backend}
					var err error
					f.d.sender, err = txsender.New[actionID](backend, f.d.txOptsFactory, 31337,
						f.d.deliveryPolicy, f.d.guardPublication, f.d.Logger)
					require.NoError(t, err)
					submitPRTAction(t, f, call)
					original := f.block
					backend.duringEstimate = func() {
						if ownInclusion {
							f.backend.nonce++
						}
						if kind == joinTournament {
							adapter, loadErr := f.factory.CreateTournamentAdapter(*f.epoch.TournamentAddress)
							require.NoError(t, loadErr)
							tournament, ok := adapter.(*tournamentAdapterMock)
							require.True(t, ok)
							for _, expectation := range tournament.ExpectedCalls {
								if expectation.Method == "CommitmentStanding" {
									expectation.ReturnArguments = mock.Arguments{
										CommitmentStanding{Joined: true, FinalState: *f.epoch.MachineHash}, nil,
									}
								}
							}
						} else {
							view.sealed.EpochNumber++
							view.stage.EpochNumber++
							view.accept.EpochNumber++
						}
						updatePRTView(f, original.Number+1, view)
					}
					var logs bytes.Buffer
					f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
					err = f.d.tick(t.Context())
					want := txsender.ErrIneligible
					if ownInclusion {
						want = txsender.ErrStale
					}
					require.ErrorIs(t, err, want)
					require.True(t, deliveryWaitOnly(err), "completed work must produce an ordinary publication wait: %v", err)
					require.Equal(t, original, backend.raceBlock, "sizing must stay pinned when latest advances")
					require.Equal(t, original.Number+1, f.backend.latest)
					if ownInclusion {
						require.Len(t, f.backend.published, 1, "a spent nonce must not produce another POST")
					} else {
						// The competitor made our held action obsolete without spending
						// our nonce. Resolving it with a neutral replacement is valid.
						require.Len(t, f.backend.published, 2)
						neutral := f.backend.published[1]
						require.Equal(t, f.repo.signer, *neutral.To())
						require.Empty(t, neutral.Data(), "fresh checks must stop redundant protocol calldata")
						require.Zero(t, neutral.Value().Sign())
						require.Equal(t, f.backend.published[0].Nonce(), neutral.Nonce())
					}
					require.NotContains(t, logs.String(), "PRT transaction could not publish")
					require.NotContains(t, logs.String(), "level=ERROR")
					require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
					require.Empty(t, f.repo.windows, "only observation can persist protocol completion")
				})
			}
		}
	}
}
