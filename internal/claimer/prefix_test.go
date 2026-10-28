// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

const (
	missingBoundaryCase      = "missing boundary"
	snapshotReadFailedCase   = "snapshot read failed"
	divergedApplicationCase  = "diverged"
	failedApplicationCase    = "failed"
	corruptedApplicationCase = "corrupted"
)

func setClaimTestStop(lookup *deliveryLookup, app *model.Application, cutoff uint64, kind model.ApplicationStatus) {
	app.Status = kind
	if lookup.problems == nil {
		lookup.problems = map[int64]*model.ApplicationProblem{}
	}
	problem := &model.ApplicationProblem{EpochIndex: cutoff, Kind: kind}
	if kind.IsExecutionTerminal() {
		problem.InputIndex = new(uint64(99))
	}
	lookup.problems[app.ID] = problem
}

func TestClaimPublicationCompletesTerminalPrefix(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		for _, status := range []model.ApplicationStatus{model.ApplicationStatus_GuestException,
			model.ApplicationStatus_MachineHalted, model.ApplicationStatus_McycleOverflow,
			model.ApplicationStatus_UnexpectedYield, model.ApplicationStatus_InvalidOutputsRoot} {
			for _, accept := range []bool{false, true} {
				name := consensus.String() + "/" + status.String() + "/submit"
				if accept {
					name = consensus.String() + "/" + status.String() + "/accept"
				}
				t.Run(name, func(t *testing.T) {
					s, backend, lookup := newDeliveryFixture(t)
					app, epoch := deliveryApplication(1, consensus)
					lookup.add(app, epoch)
					backend.apps[app.IApplicationAddress] = app
					setClaimTestStop(lookup, app, epoch.Index+1, status)
					if accept {
						epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
						backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 30)
					}
					require.NoError(t, s.publisher.tick(t.Context()))
					require.Len(t, backend.published, 1)
					method := "submitClaim"
					if accept {
						method = "acceptClaim"
					}
					require.Equal(t, method, publishedClaimMethod(t, backend.published[0].Data()))
					// Retained same-nonce work remains useful after the global
					// status changes; Next and post-sign Recheck agree.
					require.NoError(t, s.publisher.tick(t.Context()))
					require.Len(t, backend.published, 2)
					require.Equal(t, backend.published[0].Hash(), backend.published[1].Hash())
					// Defensive case: tightening to an already computed epoch
					// is unreachable with the ordered, single validator. If
					// that premise is violated, positive cutoff evidence must
					// still veto another resend.
					lookup.problems[app.ID].EpochIndex = epoch.Index
					require.NoError(t, s.publisher.tick(t.Context()))
					require.Len(t, backend.published, 3)
					require.Empty(t, backend.published[2].Data())
					require.Equal(t, backend.published[0].Nonce(), backend.published[2].Nonce())
				})
			}
		}
	}
}

func TestClaimMissingBoundaryIsInconclusiveForRetainedWork(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, epoch := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	app.Status = model.ApplicationStatus_MachineHalted
	queueDelivery(t, backend, lookup, 2) // No independently established priority replacement.

	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrInconclusive)
	require.Len(t, backend.published, 1, "missing metadata alone cannot justify cancelling or replacing retained work")
	setClaimTestStop(lookup, app, epoch.Index+1, app.Status)
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 2)
	require.Equal(t, backend.published[0].Hash(), backend.published[1].Hash())
}

func TestClaimPostSignRechecksTerminalPrefix(t *testing.T) {
	for _, change := range []string{"later stop", "cutoff", failedApplicationCase, divergedApplicationCase,
		corruptedApplicationCase, missingBoundaryCase, snapshotReadFailedCase} {
		t.Run(change, func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := queueDelivery(t, backend, lookup, 1)
			key, err := crypto.HexToECDSA("59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
			require.NoError(t, err)
			opts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(31337))
			require.NoError(t, err)
			readFailure := errors.New("application epoch snapshot unavailable")
			factory := claimSigningHookFactory{TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(opts), after: func() {
				setClaimTestStop(lookup, app, epoch.Index+1, model.ApplicationStatus_MachineHalted)
				switch change {
				case "cutoff":
					lookup.problems[app.ID].EpochIndex = epoch.Index
				case failedApplicationCase:
					app.Status = model.ApplicationStatus_Failed
				case divergedApplicationCase:
					app.Status = model.ApplicationStatus_Diverged
				case corruptedApplicationCase:
					app.Status = model.ApplicationStatus_Corrupted
				case missingBoundaryCase:
					delete(lookup.problems, app.ID)
				case snapshotReadFailedCase:
					lookup.epochErrors = map[string]error{app.IApplicationAddress.Hex(): readFailure}
				}
			}}
			s.publisher.sender, err = txsender.New[claimAction](backend, factory, 31337,
				txsender.Policy{MaxFee: big.NewInt(1000)}, func(context.Context) error { return nil }, s.Logger)
			require.NoError(t, err)

			err = s.publisher.tick(t.Context())

			if change == "later stop" {
				require.NoError(t, err)
				require.Len(t, backend.published, 1)
			} else {
				require.Error(t, err)
				require.Empty(t, backend.published, "post-sign state is checked before POST")
				if change == missingBoundaryCase {
					require.ErrorIs(t, err, txsender.ErrInconclusive)
				}
				if change == snapshotReadFailedCase {
					require.ErrorIs(t, err, readFailure)
				}
			}
		})
	}
}
