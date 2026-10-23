// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestClaimPlannerPublishesThroughRealSender(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		t.Run(consensus.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := deliveryApplication(1, consensus)
			lookup.add(app, epoch)
			backend.apps[app.IApplicationAddress] = app
			candidate, state, err := s.publisher.decide(t.Context(), txsender.BlockRef{Number: backend.last},
				repository.ClaimWork{Application: app, Epoch: epoch}, false)
			require.NoError(t, err)
			require.Equal(t, txsender.Useful, state)
			require.Equal(t, app.IApplicationAddress, candidate.action.Address)
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 1)
			require.Equal(t, candidate.request.Data, backend.published[0].Data())
			require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status, "publication does not write observed status")
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			if consensus == model.Consensus_Quorum {
				require.Equal(t, quorumSubmitGasLimit, backend.published[0].Gas())
			}
		})
	}
}

func TestClaimPlannerUnknownReadDoesNotCancelOrSwitch(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	queueDelivery(t, backend, lookup, 2)
	failure := errors.New("owner RPC unavailable")
	backend.ownerErr = failure
	require.ErrorIs(t, s.publisher.tick(t.Context()), failure)
	require.Len(t, backend.published, 1)
}

func TestClaimPlannerRetainsBytesAcrossLostReply(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	queueDelivery(t, backend, lookup, 1)
	backend.publishErr = errors.New("transport reply lost")
	require.Error(t, s.publisher.tick(t.Context()))
	backend.publishErr = nil
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.raw, 2)
	require.Equal(t, backend.raw[0], backend.raw[1])
}

func TestClaimPlannerAuthorizationAndPermissionlessAcceptance(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		t.Run(consensus.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := deliveryApplication(1, consensus)
			lookup.add(app, epoch)
			backend.apps[app.IApplicationAddress] = app
			backend.owner = common.HexToAddress("0xdead")
			backend.validatorID = new(big.Int)
			require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrIneligible)
			require.Empty(t, backend.published)
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
			backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 30)
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 1, "acceptance does not require ownership or validator membership")
		})
	}
}

func TestClaimPlannerFreshAcceptedEpochIsExcluded(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	_, epoch := queueDelivery(t, backend, lookup, 1)
	epoch.Status = model.EpochStatus_ClaimAccepted
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Empty(t, backend.published)
}

func TestClaimPlannerReadFailureHasNoHealthWrite(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, _ := queueDelivery(t, backend, lookup, 1)
	backend.claimReadErr = context.DeadlineExceeded
	require.ErrorIs(t, s.publisher.tick(t.Context()), context.DeadlineExceeded)
	require.Empty(t, backend.published)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
}

func publishedClaimMethod(t *testing.T, data []byte) string {
	t.Helper()
	contractABI, err := iconsensus.IConsensusMetaData.GetAbi()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(data), 4)
	method, err := contractABI.MethodById(data[:4])
	require.NoError(t, err)
	return method.Name
}

func TestClaimPlannerPositiveObsolescenceReleasesCurrentNonce(t *testing.T) {
	for _, reason := range []string{"disabled", "deleted", "foreclosed", "health", "accepted", "owner-changed"} {
		t.Run(reason, func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := queueDelivery(t, backend, lookup, 1)
			require.NoError(t, s.publisher.tick(t.Context()))
			switch reason {
			case "disabled":
				app.Enabled = false
			case "deleted":
				delete(lookup.apps, app.IApplicationAddress.Hex())
			case "foreclosed":
				app.ForecloseBlock = backend.last
			case "health":
				app.Status = model.ApplicationStatus_Failed
			case "accepted":
				backend.claims[app.ID] = makeClaimStatus(claimStatusAccepted, epoch, 30)
			case "owner-changed":
				backend.owner = common.HexToAddress("0xdead")
			}
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 2)
			cancel := backend.published[1]
			require.Empty(t, cancel.Data())
			require.Equal(t, s.signer, *cancel.To())
			require.Zero(t, cancel.Value().Sign())
			require.Equal(t, backend.published[0].Nonce(), cancel.Nonce())
		})
	}
}

func TestClaimPlannerImmaturePeerStageCancelsCompletedSubmit(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		t.Run(consensus.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := deliveryApplication(1, consensus)
			app.ClaimStagingPeriod = 100
			lookup.add(app, epoch)
			backend.apps[app.IApplicationAddress] = app
			require.NoError(t, s.publisher.tick(t.Context()))
			backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 49)
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 2)
			require.Empty(t, backend.published[1].Data(), "matching staging has completed the held submit action")
		})
	}
}

func TestClaimPlannerFutureObservationCannotAuthorizeCancellation(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, _ := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	app.ForecloseBlock = backend.last + 1
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrInconclusive)
	require.Len(t, backend.published, 1)
}

func TestClaimPlannerDoesNotHoldAlternativeBehindObsoleteCache(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	old, _ := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	old.Enabled = false
	next, bad := queueDelivery(t, backend, lookup, 2)
	good := makeComputedEpoch(next, 4)
	lookup.add(next, good)
	failure := errors.New("bad candidate simulation reverted")
	backend.failEpochs = map[uint64]error{bad.LastBlock: failure}
	require.ErrorIs(t, s.publisher.tick(t.Context()), failure)
	require.Len(t, backend.published, 2, "failed candidate falls through while obsolete cached bytes still exist")
	require.Equal(t, claimSubmitMethod, publishedClaimMethod(t, backend.published[1].Data()))
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	require.NoError(t, err)
	values, err := parsed.Methods[claimSubmitMethod].Inputs.Unpack(backend.published[1].Data()[4:])
	require.NoError(t, err)
	require.Equal(t, new(big.Int).SetUint64(good.LastBlock), values[1])
	require.Equal(t, backend.published[0].Nonce(), backend.published[1].Nonce())
}

func TestClaimPlannerInspectsLaterWindowBeforeReadyOldestSend(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, oldest := queueDelivery(t, backend, lookup, 1)
	var tail []*model.Epoch
	for index := uint64(4); index < 44; index++ {
		epoch := makeComputedEpoch(app, index)
		lookup.add(app, epoch)
		tail = append(tail, epoch)
	}
	backend.last = 1000
	// Every pass deliberately leaves the oldest computed row eligible. Cache
	// retirement is modeled by latest mined nonce advancement, not receipts.
	for range 12 {
		require.NoError(t, s.publisher.tick(t.Context()))
		require.NotEmpty(t, backend.published)
		backend.finalize(t)
	}
	require.Contains(t, backend.claimReads, oldest.LastBlock)
	for _, epoch := range tail {
		require.Contains(t, backend.claimReads, epoch.LastBlock, "ready oldest rows cannot hide later read attempts")
		require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
	}
	require.Len(t, backend.published, 12, "one publication ends each pass after every window row was considered")
}

func TestClaimPlannerReconstructsAfterInclusionReorgUnderEveryPolicy(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe, model.DefaultBlock_Finalized} {
		t.Run(policy.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t, policy)
			app, epoch := queueDelivery(t, backend, lookup, 1)
			var tags []string
			backend.onHeader = func(tag string) { tags = append(tags, tag) }
			require.NoError(t, s.publisher.tick(t.Context()))
			original := backend.published[0]
			backend.finalize(t)
			epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(45))
			backend.claims[app.ID] = makeClaimStatus(claimStatusAccepted, epoch, 45)
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 1)
			// The old nonce has become available again after a reorg. The
			// already stored STAGED row remains a reconstruction candidate.
			backend.nonce, backend.consumedAt = 0, 0
			delete(backend.claims, app.ID)
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 2)
			require.Equal(t, original.Nonce(), backend.published[1].Nonce())
			require.Equal(t, original.Data(), backend.published[1].Data())
			for _, tag := range tags {
				require.Equal(t, "latest", tag, "sender policy has no hidden finality dependency")
			}
		})
	}
}

func TestClaimPlannerIncompleteSavedContentIsAnActionableError(t *testing.T) {
	for _, missing := range []string{"machine-root", "outputs-root", "proof"} {
		t.Run(missing, func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := queueDelivery(t, backend, lookup, 1)
			switch missing {
			case "machine-root":
				epoch.MachineHash = nil
			case "outputs-root":
				epoch.TxBufferDataBlock = nil
			case "proof":
				epoch.TxBufferProof = nil
			}
			err := s.publisher.tick(t.Context())
			require.Error(t, err)
			require.False(t, txsender.WaitOnly(err))
			require.Contains(t, err.Error(), app.IApplicationAddress.Hex())
			require.Contains(t, err.Error(), "epoch 3")
			require.Empty(t, backend.published)
			require.Equal(t, model.ApplicationStatus_OK, app.Status, "only Observe persists health")
		})
	}
}

func TestClaimPlannerRotatesApplicationsAfterEachPublication(t *testing.T) {
	for _, count := range []int{0, 1, 16, 17, 40} {
		t.Run(fmt.Sprintf("apps-%d", count), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			for index := range count {
				queueDelivery(t, backend, lookup, int64(index+1))
			}
			seen := make(map[common.Address]bool)
			for range max(count, 1) {
				before := len(backend.published)
				require.NoError(t, s.publisher.tick(t.Context()))
				if count == 0 {
					require.Empty(t, backend.published)
					continue
				}
				require.Len(t, backend.published, before+1)
				parsed, err := iconsensus.IConsensusMetaData.GetAbi()
				require.NoError(t, err)
				values, err := parsed.Methods[claimSubmitMethod].Inputs.Unpack(backend.published[before].Data()[4:])
				require.NoError(t, err)
				app := values[0].(common.Address)
				require.False(t, seen[app], "another app must get a turn before this oldest is retried")
				seen[app] = true
				backend.finalize(t)
			}
			require.Len(t, seen, count)
		})
	}
}

func TestClaimPlannerOlderQuorumVoteDoesNotHideMatureAcceptance(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, old := deliveryApplication(1, model.Consensus_Quorum)
	old.Status = model.EpochStatus_ClaimSubmitted
	ready := makeComputedEpoch(app, 4)
	ready.Status, ready.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(51))
	backend.last = 60
	require.GreaterOrEqual(t, backend.last, *ready.StagedAtBlock+app.ClaimStagingPeriod)
	lookup.add(app, old)
	lookup.add(app, ready)
	backend.apps[app.IApplicationAddress] = app
	backend.claimsAt = map[uint64]iconsensus.IConsensusClaim{ready.LastBlock: makeClaimStatus(claimStatusStaged, ready, 51)}
	backend.votesAt = map[uint64]bool{old.LastBlock: true}
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 1)
	require.Equal(t, "acceptClaim", publishedClaimMethod(t, backend.published[0].Data()))
	require.Equal(t, model.EpochStatus_ClaimSubmitted, old.Status)
}

func TestClaimPlannerMissingHeldEpochIsUnusableEvidence(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, epoch := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	delete(lookup.epochs[app.IApplicationAddress.Hex()], epoch.Index)
	lookup.missingNil = true
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrInconclusive)
	require.Len(t, backend.published, 1, "missing content does not authorize cancellation")
}

func TestClaimPlannerNeutralRequestCanReturnToUsefulProtocolWork(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, _ := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	app.Enabled = false
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Empty(t, backend.published[1].Data())
	app.Enabled = true
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 3)
	require.Equal(t, claimSubmitMethod, publishedClaimMethod(t, backend.published[2].Data()))
	require.Equal(t, backend.published[0].Nonce(), backend.published[2].Nonce())
}

func TestClaimPlannerSimulationDeadlineFallsThroughWithoutHealthWrite(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, bad := queueDelivery(t, backend, lookup, 1)
	good := makeComputedEpoch(app, 4)
	lookup.add(app, good)
	backend.failEpochs = map[uint64]error{bad.LastBlock: context.DeadlineExceeded}
	require.ErrorIs(t, s.publisher.tick(t.Context()), context.DeadlineExceeded)
	require.Len(t, backend.published, 1)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
}

type claimSigningHookFactory struct {
	ethutil.TransactOptsFactory
	after func()
}

func (f claimSigningHookFactory) NewTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	opts, err := f.TransactOptsFactory.NewTransactOpts(ctx)
	if err != nil {
		return nil, err
	}
	sign := opts.Signer
	opts.Signer = func(address common.Address, tx *types.Transaction) (*types.Transaction, error) {
		signed, err := sign(address, tx)
		if err == nil {
			f.after()
		}
		return signed, err
	}
	return opts, nil
}

const (
	claimPostSignReadFailure  = "failed-read"
	claimPostSignHeadAdvances = "head-advances"
)

func TestClaimPlannerPostSignRechecksTheChosenAction(t *testing.T) {
	for _, change := range []string{"owner", "foreclosure", "deleted-app", claimPostSignReadFailure, claimPostSignHeadAdvances} {
		t.Run(change, func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, _ := queueDelivery(t, backend, lookup, 1)
			key, err := crypto.HexToECDSA("59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
			require.NoError(t, err)
			opts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(31337))
			require.NoError(t, err)
			readFailure := errors.New("post-sign owner RPC failed")
			factory := claimSigningHookFactory{TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(opts), after: func() {
				switch change {
				case "owner":
					backend.owner = common.HexToAddress("0xdead")
				case "foreclosure":
					app.ForecloseBlock = backend.last
				case "deleted-app":
					delete(lookup.apps, app.IApplicationAddress.Hex())
				case claimPostSignReadFailure:
					backend.ownerErr = readFailure
				case claimPostSignHeadAdvances:
					backend.last++
				}
			}}
			s.publisher.sender, err = txsender.New[claimAction](backend, factory, 31337,
				txsender.Policy{MaxFee: big.NewInt(1000)}, func(context.Context) error { return nil }, s.Logger)
			require.NoError(t, err)
			err = s.publisher.tick(t.Context())
			if change == claimPostSignHeadAdvances {
				require.NoError(t, err)
				require.Len(t, backend.published, 1, "ordinary forward heads do not starve KMS publication")
			} else {
				require.Error(t, err)
				if change == claimPostSignReadFailure {
					require.ErrorIs(t, err, readFailure)
				}
				require.Empty(t, backend.published, "unknown or changed chosen-action eligibility blocks POST")
			}
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
		})
	}
}
