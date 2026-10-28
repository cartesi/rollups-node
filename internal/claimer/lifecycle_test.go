// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type claimSibling struct {
	started chan struct{}
	stopped atomic.Bool
}

func (*claimSibling) String() string { return "sibling" }
func (*claimSibling) Ready() bool    { return true }
func (s *claimSibling) Serve(ctx context.Context) error {
	close(s.started)
	<-ctx.Done()
	s.stopped.Store(true)
	return ctx.Err()
}

func TestClaimHardFailureKeepsObserverAndSiblingServingUntilShutdown(t *testing.T) {
	for _, origin := range []string{"observer", "publisher"} {
		t.Run(origin, func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			chain := s.blockchain.(*claimerBlockchainMock)
			cause := errors.Join(txsender.ErrFinality, errors.New("finalized block 20 changed hash"))
			if origin == "observer" {
				chain.On("getDefaultBlockNumber", mock.Anything).Return((*big.Int)(nil), cause)
			} else {
				queueDelivery(t, backend, lookup, 1)
				backend.ownerErr = cause
				chain.On("getDefaultBlockNumber", mock.Anything).
					Run(func(args mock.Arguments) { <-args.Get(0).(context.Context).Done() }).
					Return((*big.Int)(nil), context.Canceled).Once()
			}
			require.NoError(t, service.InitTickServiceTemplate(&s.TickServiceTemplate, &service.TickServiceConfigs{
				BaseConfigs: service.BaseConfigs{Name: "claimer", Logger: s.Logger}, PollInterval: 5 * time.Millisecond,
			}, s))
			s.publisher.interval = 5 * time.Millisecond
			sibling := &claimSibling{started: make(chan struct{})}
			supervisor, err := service.NewSupervisor(t.Context(), &service.SupervisorConfigs{
				BaseConfigs: service.BaseConfigs{Name: "node", Logger: s.Logger},
				Factories: []service.FactoryFunction{
					func(context.Context, service.Supervisor) (service.SupervisedService, error) { return s, nil },
					func(context.Context, service.Supervisor) (service.SupervisedService, error) { return sibling, nil },
				},
			})
			require.NoError(t, err)
			finished := make(chan error, 1)
			go func() { finished <- supervisor.Serve() }()
			defer supervisor.Stop()
			<-sibling.started
			require.Eventually(t, func() bool { return s.failure.Load() != nil }, time.Second, time.Millisecond)
			require.False(t, s.Ready())
			require.ErrorIs(t, *s.failure.Load(), txsender.ErrFinality)
			select {
			case err := <-finished:
				t.Fatalf("hard latch stopped the node: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			require.False(t, sibling.stopped.Load())
			supervisor.Stop()
			require.NoError(t, <-finished)
			require.True(t, sibling.stopped.Load())
			require.Empty(t, backend.published)
			chain.AssertExpectations(t)
		})
	}
}

func TestClaimShutdownWaitsForBothLoops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, backend, lookup := newDeliveryFixture(t)
		app, _ := queueDelivery(t, backend, lookup, 1)
		backend.blockExecution = map[common.Address]bool{app.IApplicationAddress: true}
		backend.executionStarted = make(chan struct{}, 1)
		observerStarted, observerCanceled, releaseObserver := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var observerExited atomic.Bool
		tick := &claimTickFixture{fn: func(ctx context.Context) (bool, error) {
			close(observerStarted)
			<-ctx.Done()
			close(observerCanceled)
			<-releaseObserver
			observerExited.Store(true)
			return false, ctx.Err()
		}}
		require.NoError(t, service.InitTickServiceTemplate(&s.TickServiceTemplate, &service.TickServiceConfigs{
			BaseConfigs: service.BaseConfigs{Name: "claimer", Logger: s.Logger}, PollInterval: time.Hour,
		}, tick))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		finished := make(chan error, 1)
		go func() { finished <- s.Serve(ctx) }()
		<-observerStarted
		<-backend.executionStarted
		cancel()
		<-observerCanceled
		synctest.Wait()
		select {
		case err := <-finished:
			t.Fatalf("Serve returned before observer cleanup completed: %v", err)
		default:
		}
		close(releaseObserver)
		require.ErrorIs(t, <-finished, context.Canceled)
		require.True(t, observerExited.Load())
		require.False(t, s.Ready())
		require.Empty(t, backend.published, "the canceled publisher completed without broadcasting")

	})
}
