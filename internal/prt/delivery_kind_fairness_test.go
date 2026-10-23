// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type delayedPlanFactory struct {
	AdapterFactory
	application common.Address
	delay       time.Duration
}

func (f *delayedPlanFactory) CreateDaveConsensusAdapter(address common.Address) (DaveConsensusAdapter, error) {
	adapter, err := f.AdapterFactory.CreateDaveConsensusAdapter(address)
	if err != nil || (f.application != (common.Address{}) && address != f.application) {
		return adapter, err
	}
	return &delayedPlanConsensus{DaveConsensusAdapter: adapter, delay: f.delay}, nil
}

type delayedPlanConsensus struct {
	DaveConsensusAdapter
	delay time.Duration
}

func (a *delayedPlanConsensus) GetCurrentSealedEpoch(opts *bind.CallOpts) (CurrentSealedEpoch, error) {
	timer := time.NewTimer(a.delay)
	defer timer.Stop()
	select {
	case <-opts.Context.Done():
		return CurrentSealedEpoch{}, opts.Context.Err()
	case <-timer.C:
		return a.DaveConsensusAdapter.GetCurrentSealedEpoch(opts)
	}
}

func TestEarlierJoinScanHonorsParentCancellation(t *testing.T) {
	f := newDeliveryFixture(t)
	view := resultTestSnapshot(f.epoch, false)
	view.stage.IsFinished = false
	f.snapshot(view)
	deliveryJoinAdapter(f, f.epoch, 15)
	addSecondStageApplication(f)
	f.d.adapterFactory = &delayedPlanFactory{AdapterFactory: f.factory,
		application: f.app.IConsensusAddress, delay: 120 * time.Millisecond}
	ctx, done := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer done()
	urgent, err := f.d.candidates(ctx, f.block, observerFacts(100))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, urgent)
	require.Empty(t, f.backend.published, "scanning urgency only constructs a plan")
}
