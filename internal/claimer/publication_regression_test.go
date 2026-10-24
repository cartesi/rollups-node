// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

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
)

func capturePublicationLogs(s *Service) *bytes.Buffer {
	var output bytes.Buffer
	s.publisher.Logger = slog.New(slog.NewTextHandler(&output, nil))
	return &output
}

func TestClaimOperationalRefusalsRecoverWithoutApplicationPause(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, _ := queueDelivery(t, backend, lookup, 1)
	backend.balance = big.NewInt(1)
	for range 7 {
		clear(s.publisher.cooldowns)
		require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrLimit)
		require.Empty(t, backend.published)
		require.Equal(t, model.ApplicationStatus_OK, app.Status)
	}
	backend.balance = nil
	clear(s.publisher.cooldowns)
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 1, "funding restores publication without operator action")
}

func TestHeldClaimReadFailureAloneDoesNotJustifyReplacement(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, _ := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	queueDelivery(t, backend, lookup, 2)
	failure := errors.New("current application's epoch cannot be read")
	lookup.epochErrors = map[string]error{app.IApplicationAddress.Hex(): failure}
	err := s.publisher.tick(t.Context())
	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, err, "application "+app.IApplicationAddress.Hex())
	require.ErrorContains(t, err, "action authority-submit, epoch 3, nonce 0")
	require.Len(t, backend.published, 1, "read failure alone supplies no cancellation or replacement evidence")
}

func TestClaimSubmissionWaitsPastFinalBlockWithoutDelayingAcceptance(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		t.Run(consensus.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := deliveryApplication(1, consensus)
			lookup.add(app, epoch)
			backend.apps[app.IApplicationAddress] = app
			backend.last = epoch.LastBlock
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Empty(t, backend.published, "submitClaim reverts at the epoch's final block")
			backend.last++
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 1)
			backend.last = epoch.LastBlock
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 1, "a held submission waits rather than being cancelled at the lower head")
			backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 1)
			app.ClaimStagingPeriod = 0
			candidate, state, err := s.publisher.decide(t.Context(),
				txsender.BlockRef{Number: epoch.LastBlock}, repository.ClaimWork{Application: app, Epoch: epoch}, false)
			require.NoError(t, err)
			require.Equal(t, txsender.Useful, state)
			require.Equal(t, acceptClaimKind, candidate.action.Kind, "the submit-only boundary does not delay acceptance")
		})
	}
}

func TestHeldClaimIgnoresProvisionalAcceptedStatusWhenChainStillNeedsIt(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Latest)
	app, epoch := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	epoch.Status = model.EpochStatus_ClaimAccepted
	_, state, err := s.publisher.decide(t.Context(), txsender.BlockRef{Number: backend.last},
		repository.ClaimWork{Application: app, Epoch: epoch}, false)
	require.NoError(t, err)
	require.Equal(t, txsender.Obsolete, state, "fresh discovery still excludes accepted rows")
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 2)
	require.Equal(t, backend.published[0].Hash(), backend.published[1].Hash())
}

func TestHeldClaimLimitsWarnWithContextAndBackoff(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	logs := capturePublicationLogs(s)
	app, _ := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	backend.balance = big.NewInt(1)
	for range 7 {
		err := s.publisher.tick(t.Context())
		require.ErrorIs(t, err, txsender.ErrLimit)
		require.True(t, txsender.WaitOnly(err, txsender.ErrLimit), "run must not add a generic warning every poll")
	}
	const message = "Claim publication waiting for funding or a policy limit"
	require.Equal(t, 1, strings.Count(logs.String(), message))
	require.Contains(t, logs.String(), "address="+strings.ToLower(app.IApplicationAddress.Hex()))
	require.Contains(t, logs.String(), "action=authority-submit")
	require.Contains(t, logs.String(), "epoch=3")
	require.Contains(t, logs.String(), "nonce=0")
	clear(s.publisher.cooldowns)
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrLimit)
	require.Equal(t, 2, strings.Count(logs.String(), message))
	backend.balance = nil
	require.NoError(t, s.publisher.tick(t.Context()), "a held request retries immediately after funding returns")
	// Cancellation has no new lastChoice; its refusal still names the held action.
	clear(s.publisher.cooldowns)
	app.Enabled, backend.balance = false, big.NewInt(1)
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrLimit)
	require.Equal(t, 3, strings.Count(logs.String(), message))
	require.Contains(t, logs.String(), "action=\"cancel obsolete authority-submit\"")
}

func TestClaimHeldPublicationErrorsWarnWithBackoff(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, _ := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	logs := capturePublicationLogs(s)
	failure := errors.New("held claim read unavailable")
	lookup.epochErrors = map[string]error{app.IApplicationAddress.Hex(): failure}
	for range 7 {
		err := s.publisher.tick(t.Context())
		require.ErrorIs(t, err, failure)
		s.publisher.reportPublicationError(t.Context(), &s.publisher.deferredWarnedAt, err)
	}
	require.Equal(t, 1, strings.Count(logs.String(), "Claim publication deferred"))
	require.NotContains(t, logs.String(), "level=ERROR")
	require.Contains(t, logs.String(), app.IApplicationAddress.Hex())
	require.Contains(t, logs.String(), "action authority-submit")
}

func TestClaimPlanningWarningDoesNotHideAnotherApplicationsPublicationFailure(t *testing.T) {
	for _, sameSource := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_source=%t", sameSource), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			broken, stored := queueDelivery(t, backend, lookup, 1)
			stored.MachineHash = nil
			if !sameSource {
				stored.Status = model.EpochStatus_ClaimSubmitted
			}
			ready, _ := queueDelivery(t, backend, lookup, 2)
			failure := errors.New("ready application's exact simulation reverted")
			backend.fail[ready.IApplicationAddress] = failure
			logs := capturePublicationLogs(s)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			s.publisher.interval = time.Nanosecond
			passes, refused := 0, 0
			started := time.Now()
			backend.onHeader = func(string) {
				if s.publisher.prepared {
					return // Later preparation attempts remain in the same pass.
				}
				// Advance clocks at each pass's first account read. Warning and
				// cooldown clocks move together; candidates keep their waits.
				if passes == 200 {
					cancel()
					return
				}
				if passes > 0 {
					shiftClaimPublicationClock(s.publisher, 3*time.Second-time.Since(started))
				}
				started = time.Now()
				passes++
			}
			backend.onCall = func(method string) {
				if method == claimSubmitMethod {
					refused++
				}
			}
			require.ErrorIs(t, s.publisher.run(ctx), context.Canceled)
			planningWarnings, publicationWarnings := 0, 0
			for _, line := range strings.Split(logs.String(), "\n") {
				if !strings.Contains(line, "level=WARN") || !strings.Contains(line, "Claim publication deferred") {
					continue
				}
				if strings.Contains(line, "persisted claim is missing its machine or outputs root") {
					planningWarnings++
					require.Contains(t, line, broken.IApplicationAddress.Hex())
				}
				if strings.Contains(line, failure.Error()) {
					publicationWarnings++
					require.Contains(t, line, ready.IApplicationAddress.Hex())
				}
			}
			require.Equal(t, 200, passes)
			require.InDelta(t, 10, planningWarnings, 1, "planning warnings stay visible and rate-limited over ten virtual minutes")
			require.InDelta(t, 10, publicationWarnings, 1, "another application's failure must not take this warning slot")
			require.Greater(t, refused, publicationWarnings)
			require.Less(t, refused, passes/2, "the 30-second candidate cooldown remains active")
			require.Empty(t, backend.published)
			require.NotContains(t, logs.String(), "level=ERROR")
		})
	}
}

// Only these test clocks move. Selection and run-loop routing stay in production.
func shiftClaimPublicationClock(p *claimPublisher, elapsed time.Duration) {
	for _, warnedAt := range []*time.Time{&p.deferredWarnedAt, &p.planningWarnedAt, &p.warnedAt, &p.hardWarnedAt,
		&p.headChangedAt, &p.nonceChangedAt} {
		if !warnedAt.IsZero() {
			*warnedAt = warnedAt.Add(-elapsed)
		}
	}
	for key, until := range p.cooldowns {
		p.cooldowns[key] = until.Add(-elapsed)
	}
}

type publicationLogWriter func([]byte) (int, error)

func (write publicationLogWriter) Write(data []byte) (int, error) { return write(data) }

func TestClaimRunRoutesPlanningAndPublicationWarningsSeparately(t *testing.T) {
	for _, canceling := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceling=%t", canceling), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			useful, _ := queueDelivery(t, backend, lookup, 2)
			if canceling {
				require.NoError(t, s.publisher.tick(t.Context()))
				useful.Enabled = false
			}
			_, broken := queueDelivery(t, backend, lookup, 1)
			broken.Status, broken.MachineHash = model.EpochStatus_ClaimSubmitted, nil
			backend.publishErr = publicationRefusal("insufficient funds for gas * price + value")
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			var output bytes.Buffer
			writer := publicationLogWriter(func(data []byte) (int, error) {
				n, err := output.Write(data)
				if strings.Count(output.String(), "level=WARN msg=\"Claim publication deferred\"") == 2 {
					cancel()
				}
				return n, err
			})
			s.publisher.Logger = slog.New(slog.NewTextHandler(writer, nil))
			s.publisher.interval = time.Hour // Both warning channels must fire in the first pass.
			require.ErrorIs(t, s.publisher.run(ctx), context.Canceled, "both warnings must arrive before the test deadline")
			require.Equal(t, 2, strings.Count(output.String(), "level=WARN msg=\"Claim publication deferred\""))
			require.Contains(t, output.String(), "persisted claim is missing its machine or outputs root")
			require.Contains(t, output.String(), "insufficient funds")
			require.Equal(t, canceling, strings.Contains(output.String(), "before cancellation"))
		})
	}
}

func TestClaimRunShutdownKeepsRealFailuresVisible(t *testing.T) {
	failure := errors.New("provider returned malformed account state")
	for _, test := range []struct {
		name string
		err  error
		warn bool
	}{
		{"pure cancellation", context.Canceled, false},
		{"wait and cancellation", errors.Join(txsender.ErrInconclusive, context.Canceled), false},
		{"real failure and cancellation", errors.Join(failure, context.Canceled), true},
		{"operation deadline", context.DeadlineExceeded, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, backend, _ := newDeliveryFixture(t)
			logs := capturePublicationLogs(s)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			backend.onHeader = func(string) { cancel() }
			backend.headerErr = test.err
			require.ErrorIs(t, s.publisher.run(ctx), context.Canceled)
			require.Equal(t, test.warn, strings.Contains(logs.String(), "level=WARN"), logs.String())
			if test.warn {
				if errors.Is(test.err, failure) {
					require.Contains(t, logs.String(), failure.Error())
				} else {
					require.Contains(t, logs.String(), context.DeadlineExceeded.Error())
				}
			}
			require.NotContains(t, logs.String(), "level=ERROR")
		})
	}
}

func TestClaimFundingWarningNamesUsefulWorkWhileCancellationIsPending(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	first, _ := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	first.Enabled = false
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 2)
	require.Equal(t, s.publisher.signer, *backend.published[1].To())
	second, _ := queueDelivery(t, backend, lookup, 2)
	logs := capturePublicationLogs(s)
	backend.balance = big.NewInt(1)
	clear(s.publisher.cooldowns)
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrLimit)
	var warning string
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "Claim publication waiting for funding or a policy limit") &&
			strings.Contains(line, "address="+strings.ToLower(second.IApplicationAddress.Hex())) {
			warning = line
		}
	}
	require.NotEmpty(t, warning, logs.String())
	require.Contains(t, warning, "action=authority-submit")
	require.NotContains(t, warning, "cancel obsolete")
}

func TestClaimStalledNonceWarningIncludesOnlyThisPassPublicationReply(t *testing.T) {
	for _, attempted := range []bool{false, true} {
		t.Run(fmt.Sprintf("attempted=%t", attempted), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			queueDelivery(t, backend, lookup, 1)
			require.NoError(t, s.publisher.tick(t.Context()))
			logs := capturePublicationLogs(s)
			s.publisher.nonceChangedAt = time.Now().Add(-2 * claimWarningInterval)
			backend.publishErr = publicationRefusal("replacement transaction underpriced")
			report, err := s.publisher.sender.PublishNext(t.Context(), s.publisher)
			require.NoError(t, err)
			require.Equal(t, txsender.ReplacementUnderpriced, report.Class)
			report.Attempted = attempted
			s.publisher.reportStalls(report, nil)
			var warning string
			for _, line := range strings.Split(logs.String(), "\n") {
				if strings.Contains(line, "Claim publication nonce has not advanced") {
					warning = line
				}
			}
			require.NotEmpty(t, warning, logs.String())
			require.Equal(t, attempted, strings.Contains(warning, "reply=replacement-underpriced"))
		})
	}
}

func TestClaimIncompleteReportDoesNotResetProgress(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	logs := capturePublicationLogs(s)
	block, nonce := s.publisher.lastLive, s.publisher.lastNonce
	s.publisher.headChangedAt = time.Now().Add(-2 * claimWarningInterval)
	for range 3 {
		s.publisher.reportStalls(txsender.Report[claimAction]{}, nil)
	}
	require.Equal(t, block, s.publisher.lastLive)
	require.Equal(t, nonce, s.publisher.lastNonce)
	require.Empty(t, logs.String())
	s.publisher.reportStalls(txsender.Report[claimAction]{Block: block, Nonce: nonce}, nil)
	require.Contains(t, logs.String(), "Claim publication head has not advanced")
	require.NotContains(t, logs.String(), "block=0")
}

func TestHeldClaimOperationalErrorsKeepContextAndRemainVisible(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, _ := queueDelivery(t, backend, lookup, 1)
	require.NoError(t, s.publisher.tick(t.Context()))
	failure := errors.New("RPC execution read failed")
	backend.fail = map[common.Address]error{app.IApplicationAddress: errors.Join(txsender.ErrLimit, failure)}
	err := s.publisher.tick(t.Context())
	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, err, "application "+app.IApplicationAddress.Hex())
	require.ErrorContains(t, err, "action authority-submit, epoch 3, nonce 0")
	require.False(t, txsender.WaitOnly(err, txsender.ErrLimit), "a joined RPC error cannot be hidden as a policy wait")
}

func TestOrphanedProvisionalClaimAndRepeatedSelectionAreVisible(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Latest)
	logs := capturePublicationLogs(s)
	_, epoch := queueDelivery(t, backend, lookup, 1)
	epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(10))
	for range 3 {
		require.NoError(t, s.publisher.tick(t.Context()))
	}
	require.Equal(t, 1, strings.Count(logs.String(), "Re-publishing recorded claim absent from the current chain view"))
	backend.finalize(t) // The nonce advances, but this fake chain leaves the action useful.
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Contains(t, logs.String(), "Claim action selected again after signer nonce advanced")
	require.Contains(t, logs.String(), "other signer users")
	require.Equal(t, uint64(1), backend.published[len(backend.published)-1].Nonce())
	require.Equal(t, backend.published[0].Data(), backend.published[len(backend.published)-1].Data(),
		"nonce consumption cannot suppress the still-outstanding claim")
	require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status, "publication does not persist assumed protocol success")
	require.NotContains(t, logs.String(), "transaction reverted", "a repeated action does not prove a receipt result")
}

type deadlineClaimSigner struct {
	ethutil.TransactOptsFactory
	block   bool
	started chan struct{}
}

func (f *deadlineClaimSigner) NewTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	opts, err := f.TransactOptsFactory.NewTransactOpts(ctx)
	if err == nil && f.block {
		opts.Signer = func(common.Address, *types.Transaction) (*types.Transaction, error) {
			f.started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}
	return opts, err
}

func TestClaimSigningDeadlineReleasesBlockedSigner(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	queueDelivery(t, backend, lookup, 1)
	key, err := crypto.HexToECDSA("59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
	require.NoError(t, err)
	opts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(31337))
	require.NoError(t, err)
	factory := &deadlineClaimSigner{
		TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(opts), block: true, started: make(chan struct{}, 1),
	}
	s.publisher.sender, err = txsender.New[claimAction](backend, factory, 31337,
		txsender.Policy{SigningTimeout: 100 * time.Millisecond},
		func(context.Context) error { return nil }, s.Logger)
	require.NoError(t, err)
	require.ErrorIs(t, s.publisher.tick(t.Context()), context.DeadlineExceeded)
	require.Len(t, factory.started, 1, "the test must reach the blocked signer")
	require.Empty(t, backend.published)
	factory.block = false
	clear(s.publisher.cooldowns)
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 1, "a timed-out signing request cannot permanently hold the publisher")
	backend.finalize(t)
	backend.onHeader = func(string) { time.Sleep(75 * time.Millisecond) }
	started := time.Now()
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Greater(t, time.Since(started), 100*time.Millisecond,
		"ordinary reads may collectively take longer than the signing timeout")
	require.Len(t, backend.published, 2, "a slow discovery pass must still reach publication")
}

type cursorObservationChain struct {
	*eventChain
	visited map[uint64]bool
}

func (*cursorObservationChain) getDefaultBlockNumber(context.Context) (*big.Int, error) {
	return big.NewInt(10000), nil
}

func (c *cursorObservationChain) findClaimStagedEventAndSucc(
	_ context.Context, _ *model.Application, epoch *model.Epoch, _, _ uint64,
) (*iconsensus.IConsensus, *iconsensus.IConsensusClaimStaged, *iconsensus.IConsensusClaimStaged, error) {
	c.visited[epoch.Index] = true
	return nil, nil, nil, nil
}

func TestClaimObserverAdvancesPastUnchangedSubmittedWindow(t *testing.T) {
	s, _, lookup := newDeliveryFixture(t)
	app, _ := deliveryApplication(1, model.Consensus_Authority)
	for index := range uint64(32) {
		epoch := makeComputedEpoch(app, index)
		epoch.Status = model.EpochStatus_ClaimSubmitted
		lookup.add(app, epoch)
	}
	s.repository = &eventRepository{deliveryCandidateRepository: &deliveryCandidateRepository{lookup: lookup}}
	chain := &cursorObservationChain{eventChain: &eventChain{}, visited: map[uint64]bool{}}
	s.blockchain = chain
	for range 3 {
		_, err := s.Tick(t.Context())
		require.NoError(t, err)
	}
	require.Len(t, chain.visited, 32, "unchanged first-window rows cannot hide later submitted claims")
}
