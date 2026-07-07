// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDoNothing(t *testing.T) {
	m, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)

	prevEpochs := makeEpochMap()
	currEpochs := makeEpochMap()

	transitions, errs := m.submitClaimsAndUpdateDatabase(prevEpochs, currEpochs, makeApplicationMap(), big.NewInt(0))
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, transitions, "no transitions when no epochs to process")
}

func TestTickInterleavesStagesWithPinnedBlockAndReschedulesOnProgress(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	ctx := context.Background()
	err := service.Create(ctx, &service.CreateInfo{
		Name:             "claimer-test",
		Context:          ctx,
		Impl:             m,
		PollInterval:     time.Hour,
		EnableReschedule: true,
	}, &m.Service)
	require.NoError(t, err)
	t.Cleanup(func() {
		if m.Ticker != nil {
			m.Ticker.Stop()
		}
		if m.Cancel != nil {
			m.Cancel()
		}
	})

	tickBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	currEvent := makeSubmittedEvent(app, currEpoch)

	b.On("getDefaultBlockNumber", mock.Anything).
		Return(tickBlock, nil).Once()
	r.On("SelectClaimsToSubmitPerApp", mock.Anything).
		Return(makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), nil).Once()
	b.On("getConsensusAddress", mock.Anything, app, tickBlock).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, tickBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, tickBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{currEvent}, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, currEvent.Raw.TxHash).
		Return(nil).Once()

	r.On("SelectClaimsToStagePerApp", mock.Anything).
		Return(makeEpochMap(), makeEpochMap(), makeApplicationMap(), nil).Once()
	r.On("SelectClaimsToAcceptPerApp", mock.Anything).
		Return(makeEpochMap(), makeEpochMap(), makeApplicationMap(), nil).Once()
	r.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
		return f.Enabled != nil &&
			*f.Enabled &&
			f.ForeclosureRecorded != nil &&
			*f.ForeclosureRecorded &&
			assert.ElementsMatch(t,
				[]model.Consensus{model.Consensus_Authority, model.Consensus_Quorum},
				f.ConsensusTypes,
			)
	}), repository.Pagination{}, false).
		Return([]*model.Application{}, 0, nil).Once()

	errs := m.Tick()

	require.Empty(t, errs)
	assert.True(t, m.DrainReschedule(), "a successful stage transition should request an immediate follow-up tick")
}
