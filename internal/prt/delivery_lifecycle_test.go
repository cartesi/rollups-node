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
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type lifecycleBackend struct {
	*deliveryBackendFake
	armed   bool
	started chan struct{}
	release chan struct{}
	exited  chan struct{}
}

func (b *lifecycleBackend) Publish(ctx context.Context, raw []byte) (common.Hash, error) {
	if !b.armed {
		return b.deliveryBackendFake.Publish(ctx, raw)
	}
	close(b.started)
	<-ctx.Done()
	<-b.release
	close(b.exited)
	return common.Hash{}, ctx.Err()
}

func waitForDeliverySignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for delivery lifecycle transition")
	}
}

func TestSubmittingLoopsMaintainWhileObserverBlocksAndWaitForBothToExit(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	base := f.backend
	submitPRTAction(t, f, call)
	f.backend.foreclosure = true
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Current.Neutral)
	backend := &lifecycleBackend{deliveryBackendFake: base, started: make(chan struct{}),
		release: make(chan struct{}), exited: make(chan struct{})}
	// Rebuild the sender with the same key. Its empty cache reconstructs fresh
	// work; choose a useful stage and then block that publication at POST.
	f.backend.foreclosure = false
	sender, err := txsender.New[actionID](backend, f.d.txOptsFactory, 31337, f.d.deliveryPolicy, f.d.guardPublication, f.d.Logger)
	require.NoError(t, err)
	f.d.sender, f.d.deliveryBackend = sender, backend

	observerBlocked, releaseObserver, observerExited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Run(func(args mock.Arguments) {
			close(observerBlocked)
			<-args.Get(0).(context.Context).Done()
			<-releaseObserver
			close(observerExited)
		}).Return([]*model.Epoch{}, uint64(0), context.Canceled).Once()
	client := &ethClientMock{}
	f.d.client = client
	f.d.observedFacts = txsender.BlockRef{}
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(base.headers[100], nil)
	client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(base.headers[100], nil)
	f.d.delivery, f.d.submissionEnabled, f.d.defaultBlock = f.d, true, model.DefaultBlock_Finalized
	f.d.pollInterval = time.Millisecond
	require.NoError(t, service.InitTickServiceTemplate(&f.d.TickServiceTemplate, &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{Name: PrtConfigKey, Logger: f.d.Logger}, PollInterval: time.Hour,
	}, f.d.Service))
	backend.armed = true
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- f.d.Serve(ctx) }()
	waitForDeliverySignal(t, observerBlocked)
	waitForDeliverySignal(t, backend.started)
	cancel()
	close(backend.release)
	waitForDeliverySignal(t, backend.exited)
	select {
	case err = <-done:
		t.Fatalf("Serve returned before observer cleanup completed: %v", err)
	default:
	}
	close(releaseObserver)
	select {
	case err = <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("submitting service did not stop")
	}
	require.False(t, f.d.Ready(), "shutdown clears publication readiness")
}

func TestSubmittingLoopsRateLimitRecoverablePublicationWarnings(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	submitPRTAction(t, f, call)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	f.d.delivery, f.d.pollInterval = f.d, time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	observerStarted := make(chan struct{})
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Run(func(args mock.Arguments) {
			close(observerStarted)
			<-args.Get(0).(context.Context).Done()
		}).Return([]*model.Epoch{}, uint64(0), context.Canceled).Once()
	client := &ethClientMock{}
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(f.backend.headers[100], nil)
	client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(f.backend.headers[100], nil)
	f.d.client = client
	require.NoError(t, service.InitTickServiceTemplate(&f.d.TickServiceTemplate, &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{Name: PrtConfigKey, Logger: f.d.Logger}, PollInterval: time.Hour,
	}, f.d.Service))
	failure := errors.New("provider execution read unavailable")
	attempts := 0
	f.backend.onPreflight = func(ctx context.Context, _ ethereum.CallMsg, _ txsender.BlockRef) error {
		select {
		case <-observerStarted:
		case <-ctx.Done():
			return ctx.Err()
		}
		attempts++
		if attempts == 3 {
			cancel()
		}
		return failure
	}
	require.ErrorIs(t, f.d.serveLoops(ctx), context.Canceled)
	require.Equal(t, 3, attempts, "the actual publisher loop must retry the recoverable failure")
	require.Equal(t, 1, strings.Count(logs.String(), "PRT publication deferred"), logs.String())
	require.Equal(t, 1, strings.Count(logs.String(), "PRT transaction could not publish"), logs.String())
	require.NotContains(t, logs.String(), "level=ERROR")
	require.Contains(t, logs.String(), failure.Error())
	require.Len(t, f.backend.published, 1, "all subsequent attempts failed before publication")
	f.repo.AssertExpectations(t)
}
