// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPRTPassPlansOneWindowAndAttemptsEachRefusedCandidateOnce(t *testing.T) {
	for _, interval := range []time.Duration{time.Second, time.Nanosecond} {
		t.Run(interval.String(), func(t *testing.T) {
			f, _, _ := preparePRTAction(t, stageTournament)
			for id := int64(8); id < 15; id++ {
				addDeliveryJoinCandidate(f, id, 25)
			}
			f.d.pollInterval = interval
			f.backend.balance = big.NewInt(1)
			before := len(f.factory.Calls)
			err := f.d.tick(t.Context())
			require.ErrorIs(t, err, txsender.ErrLimit)
			require.Empty(t, f.backend.published)
			require.Len(t, f.repo.pages, 4, "fallthrough must not advance another application window")
			require.Len(t, f.d.candidateCooldown, 8, "all eight candidates get an independent preparation attempt")
			plans := 0
			for _, call := range f.factory.Calls[before:] {
				if call.Method == "CreateDaveConsensusAdapter" {
					plans++
				}
			}
			require.LessOrEqual(t, plans, 4*8, "eight refused candidates must not rebuild all eight plans")
		})
	}
}

func failDeliveryEpochRead(f *deliveryFixture, app *model.Application, cause error) {
	for _, call := range f.repo.ExpectedCalls {
		if call.Method == "GetEpoch" && call.Arguments.Get(1) == app.IApplicationAddress.Hex() {
			call.ReturnArguments = mock.Arguments{(*model.Epoch)(nil), cause}
		}
	}
}

func TestPRTPlanningFailureIsVisibleWhileAnotherApplicationPublishes(t *testing.T) {
	f, _, _ := preparePRTAction(t, stageTournament)
	other := addSecondStageApplication(f)
	cause := errors.New("provider could not read the application's epoch")
	failDeliveryEpochRead(f, other, cause)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	for range 2 {
		require.NoError(t, f.d.tick(t.Context()))
	}
	require.Len(t, f.backend.published, 2, "unrelated failed reads cannot suppress useful current work")
	require.Contains(t, logs.String(), other.Name)
	require.Contains(t, logs.String(), other.IApplicationAddress.Hex())
	require.Contains(t, logs.String(), cause.Error())
	require.Equal(t, 3, strings.Count(logs.String(), "PRT application action could not be planned"),
		"each affected action kind warns once, rather than once per poll or fallthrough")
}

func TestPRTHeldReadFailureAloneDoesNotJustifySubstitution(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	submitPRTAction(t, f, call)
	// This pass has no separately verified priority evidence after the failed read.
	addDeliveryJoinCandidate(f, 8, 25)
	cause := errors.New("held application's epoch could not be read")
	failDeliveryEpochRead(f, f.app, cause)
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.ErrorIs(t, err, cause)
	require.False(t, report.Attempted)
	require.NotNil(t, report.Current)
	require.Equal(t, call.Key, report.Current.Action)
	require.False(t, report.Current.Neutral)
	require.Len(t, f.backend.published, 1)
}

func TestPRTReadyJoinPreemptsUntimedCurrentWork(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	submitPRTAction(t, f, call)
	other := addDeliveryJoinCandidate(f, 8, 25)
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.Equal(t, other.ID, report.Current.Action.ApplicationID)
	require.Equal(t, joinTournament, report.Current.Action.Kind)
	require.Equal(t, uint64(7), f.backend.published[1].Nonce())
}

func TestPRTFallthroughRechecksAtTheNewCanonicalHead(t *testing.T) {
	for _, reorg := range []bool{false, true} {
		t.Run(map[bool]string{false: "head advances", true: "preparation branch orphaned"}[reorg], func(t *testing.T) {
			f, _, view := preparePRTAction(t, stageTournament)
			other := addDeliveryJoinCandidate(f, 8, 25)
			old := f.block
			refused := errors.New("first candidate's exact simulation reverted")
			f.backend.onPreflight = func(_ context.Context, msg ethereum.CallMsg, block txsender.BlockRef) error {
				if *msg.To == common.BigToAddress(big.NewInt(other.ID*10+3)) {
					require.Equal(t, old, block)
					if reorg {
						f.backend.headers[old.Number].Extra = []byte("orphaned preparation")
						view.sealed.EpochNumber++
						view.stage.EpochNumber++
						view.accept.EpochNumber++
					}
					updatePRTView(f, old.Number+1, view)
					return refused
				}
				require.Equal(t, f.block, block, "fallback simulation must use the fresh canonical block hash")
				return nil
			}
			err := f.d.tick(t.Context())
			require.ErrorIs(t, err, refused)
			if reorg {
				require.ErrorIs(t, err, txsender.ErrIneligible)
				require.Empty(t, f.backend.published, "fresh Recheck must reject the old branch's cached action")
			} else {
				require.Len(t, f.backend.published, 1)
				require.Equal(t, f.app.IConsensusAddress, *f.backend.published[0].To())
			}
			require.Equal(t, old.Number+1, f.block.Number)
			require.Len(t, f.repo.pages, 4, "the head change does not rebuild the discovery window")
		})
	}
}

func TestPRTHeldStageWaitsForTournamentToFinishAgainAfterReorg(t *testing.T) {
	f, call, view := preparePRTAction(t, stageTournament)
	submitPRTAction(t, f, call)
	joined := &tournamentAdapterMock{}
	joined.On("CommitmentStanding", mock.Anything, [32]byte(*f.epoch.Commitment)).
		Return(CommitmentStanding{Joined: true, FinalState: *f.epoch.MachineHash}, nil)
	f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(joined, nil)
	view.stage.IsFinished = false
	f.backend.headers[f.block.Number].Extra = []byte("orphaned tournament completion")
	updatePRTView(f, f.block.Number-1, view)
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.False(t, report.Attempted)
	require.Equal(t, call.Key, report.Current.Action)
	require.False(t, report.Current.Neutral)
	require.Len(t, f.backend.published, 1, "a temporarily unfinished tournament does not justify cancellation")
	view.stage.IsFinished = true
	updatePRTView(f, 111, view)
	report, err = f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.Equal(t, call.Key, report.Current.Action)
	require.Equal(t, f.backend.raw[0], f.backend.raw[1])
}

func TestPRTRepeatedActionAfterNonceProgressWarnsWithoutAReceiptVerdict(t *testing.T) {
	f, _, _ := preparePRTAction(t, stageTournament)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	for range 3 {
		require.NoError(t, f.d.tick(t.Context()))
		f.backend.nonce++
	}
	require.Contains(t, logs.String(), "PRT action is still required after the signer nonce advanced")
	require.Contains(t, logs.String(), f.app.IApplicationAddress.Hex())
	require.Contains(t, logs.String(), "stage tournament result")
	require.Contains(t, logs.String(), "previous_nonce=7 nonce=8")
	require.Equal(t, 1, strings.Count(logs.String(), "PRT action is still required"))
	require.Contains(t, logs.String(), "other signer users")
	require.Len(t, f.backend.published, 3)
	for i, tx := range f.backend.published {
		require.Equal(t, uint64(7+i), tx.Nonce(), "externally consumed nonces cause reconstruction at the next nonce")
		require.Equal(t, f.backend.published[0].Data(), tx.Data(), "nonce consumption does not prove protocol success")
	}
	require.False(t, f.d.publicationStopped())
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
}

func TestPRTPendingRefusalWarningsRetainContextWithoutANewCandidate(t *testing.T) {
	for _, cancellation := range []bool{false, true} {
		t.Run(map[bool]string{false: "held read unavailable", true: "cancellation lacks funds"}[cancellation], func(t *testing.T) {
			f, call, _ := preparePRTAction(t, stageTournament)
			var cause error
			submitPRTAction(t, f, call)
			if cancellation {
				f.app.Enabled = false
				f.backend.balance = big.NewInt(1)
				cause = txsender.ErrLimit
			} else {
				cause = errors.New("held epoch read unavailable")
				failDeliveryEpochRead(f, f.app, cause)
			}
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			for range 2 {
				require.ErrorIs(t, f.d.tick(t.Context()), cause)
			}
			require.Len(t, f.backend.published, 1)
			require.Equal(t, 1, strings.Count(logs.String(), "PRT transaction could not publish"))
			require.Contains(t, logs.String(), f.app.IApplicationAddress.Hex())
			require.Contains(t, logs.String(), "stage tournament result")
			if cancellation {
				require.Contains(t, logs.String(), "cancel obsolete stage tournament result")
			}
			require.Contains(t, logs.String(), "nonce=7")
		})
	}
}

type deadlineSigningFactory struct {
	ethutil.TransactOptsFactory
	blocked bool
	entered bool
}

func (f *deadlineSigningFactory) NewTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	opts, err := f.TransactOptsFactory.NewTransactOpts(ctx)
	if err != nil || !f.blocked {
		return opts, err
	}
	result := *opts
	result.Signer = func(common.Address, *types.Transaction) (*types.Transaction, error) {
		f.entered = true
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &result, nil
}

func TestPRTSigningDeadlineBoundsSigningAndTheNextPassCanProgress(t *testing.T) {
	f, _, _ := preparePRTAction(t, stageTournament)
	factory := &deadlineSigningFactory{TransactOptsFactory: f.d.txOptsFactory, blocked: true}
	f.d.deliveryPolicy.SigningTimeout = 100 * time.Millisecond
	var err error
	f.d.sender, err = txsender.New[actionID](f.backend, factory, 31337, f.d.deliveryPolicy, f.d.guardPublication, f.d.Logger)
	require.NoError(t, err)
	err = f.d.tick(t.Context())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.True(t, factory.entered, "the signer must receive its own operation deadline")
	require.Empty(t, f.backend.published)
	require.False(t, f.d.publicationStopped())
	factory.blocked = false
	clear(f.d.candidateCooldown)
	require.NoError(t, f.d.tick(t.Context()))
	require.Len(t, f.backend.published, 1)
	f.backend.nonce++
	f.backend.onHeader = func(context.Context) error {
		time.Sleep(75 * time.Millisecond)
		return nil
	}
	started := time.Now()
	require.NoError(t, f.d.tick(t.Context()))
	require.Greater(t, time.Since(started), 100*time.Millisecond,
		"the signing budget does not expire during ordinary discovery reads")
	require.Len(t, f.backend.published, 2)
}

func TestPRTPublicationShutdownDuringAccountReadDoesNotLogAnError(t *testing.T) {
	f, _, _ := preparePRTAction(t, stageTournament)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f.backend.onHeader = func(ctx context.Context) error {
		cancel()
		return ctx.Err()
	}
	err := f.d.tick(ctx)
	require.ErrorIs(t, err, context.Canceled)
	f.d.reportPublicationError(ctx, err)
	require.NotContains(t, logs.String(), "level=ERROR")
	require.Empty(t, f.backend.published)
}
