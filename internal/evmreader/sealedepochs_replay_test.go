// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSealedEpochTransitionValidatesCompletePageBeforeWrites(t *testing.T) {
	const block uint64 = 150
	tournament := common.HexToAddress("0x1234")
	for _, test := range []struct {
		name    string
		indices []int64
		change  func(*idaveconsensus.IDaveConsensusEpochSealed)
	}{
		{name: "empty"},
		{name: "missing first", indices: []int64{2, 3}},
		{name: "missing middle", indices: []int64{1, 3}},
		{name: "missing last", indices: []int64{1, 2}},
		{name: "duplicate", indices: []int64{1, 1, 3}},
		{name: "out of order", indices: []int64{2, 1, 3}},
		{name: "wrong block", indices: []int64{1, 2, 3}, change: func(e *idaveconsensus.IDaveConsensusEpochSealed) {
			e.Raw.BlockNumber++
		}},
		{name: "wrong lower bound", indices: []int64{1, 2, 3}, change: func(e *idaveconsensus.IDaveConsensusEpochSealed) {
			e.InputIndexLowerBound.SetUint64(1)
		}},
		{name: "wrong upper bound", indices: []int64{1, 2, 3}, change: func(e *idaveconsensus.IDaveConsensusEpochSealed) {
			e.InputIndexUpperBound.SetUint64(1)
		}},
		{name: "wrong tournament", indices: []int64{1, 2, 3}, change: func(e *idaveconsensus.IDaveConsensusEpochSealed) {
			e.Tournament = common.HexToAddress("0x5678")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockRepository()
			dave := newMockDaveConsensus()
			r := &Service{repository: repo}
			r.Logger = testLogger(t)
			app := newDaveAppContracts(nil, dave)
			app.application.LastEpochCheckBlock = block - 1
			app.application.Status = model.ApplicationStatus_OK
			events := make([]*idaveconsensus.IDaveConsensusEpochSealed, 0, len(test.indices))
			for _, index := range test.indices {
				events = append(events, makeSealedEpochEvent(index, 0, 0, block, tournament))
			}
			if test.change != nil {
				test.change(events[len(events)-1])
			}
			dave.On("GetCurrentSealedEpoch", blockRange(block-1, block)).
				Return(makeSealedEpochResult(0, 0, 0, tournament), nil).Once()
			dave.On("GetCurrentSealedEpoch", blockRange(block, block+1)).
				Return(makeSealedEpochResult(3, 0, 0, tournament), nil).Twice()
			dave.On("RetrieveSealedEpochs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
				return opts.Start == block && opts.End != nil && *opts.End == block
			})).Return(events, nil).Once()

			require.Error(t, r.processApplicationSealedEpochs(t.Context(), app, block))
			require.Empty(t, repo.Calls, "the entire page must be checked before any repository work")
			require.Equal(t, block-1, app.application.LastEpochCheckBlock)
			require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
			dave.AssertExpectations(t)
		})
	}
}

// This fake makes completed per-epoch writes survive a failed scan. Reads return
// copies, as PostgreSQL does; the test does not rely on shared model pointers.
type sealedEpochReplayRepository struct {
	*MockRepository
	epochs    map[uint64]*model.Epoch
	stores    []uint64
	failEpoch uint64
	storeErr  error
}

func (r *sealedEpochReplayRepository) GetEpoch(_ context.Context, _ string, index uint64) (*model.Epoch, error) {
	if epoch := r.epochs[index]; epoch != nil {
		copyEpoch := *epoch
		return &copyEpoch, nil
	}
	return nil, nil
}

func (r *sealedEpochReplayRepository) CreateEpochsAndInputs(
	_ context.Context, _ string, epochs map[*model.Epoch][]*model.Input, _ uint64,
) error {
	for epoch := range epochs {
		if epoch.Index == r.failEpoch && r.storeErr != nil {
			return r.storeErr
		}
		copyEpoch := *epoch
		r.epochs[epoch.Index] = &copyEpoch
		r.stores = append(r.stores, epoch.Index)
	}
	return nil
}

func TestSealedEpochScanReplaysCommittedPrefix(t *testing.T) {
	const (
		cursor      uint64 = 100
		firstBlock  uint64 = 150
		secondBlock uint64 = 175
		head        uint64 = 200
	)
	tournament := common.HexToAddress("0x1234")
	for _, sameBlockFailure := range []bool{true, false} {
		name := "later transition page omitted"
		if sameBlockFailure {
			name = "second seal in same block fails to store"
		}
		t.Run(name, func(t *testing.T) {
			repo := &sealedEpochReplayRepository{
				MockRepository: newMockRepository(),
				epochs: map[uint64]*model.Epoch{0: {
					Index: 0, FirstBlock: 10, LastBlock: cursor, Status: model.EpochStatus_Closed, TournamentAddress: &tournament,
				}},
			}
			if sameBlockFailure {
				repo.failEpoch = 2
				repo.storeErr = errors.New("second seal write interrupted")
			}
			dave := newMockDaveConsensus()
			r := &Service{repository: repo}
			r.Logger = testLogger(t)
			app := newDaveAppContracts(nil, dave)
			app.application.LastEpochCheckBlock = cursor
			app.application.IInputBoxBlock = 10
			app.application.Status = model.ApplicationStatus_OK
			dave.On("GetCurrentSealedEpoch", blockRange(cursor, firstBlock)).
				Return(makeSealedEpochResult(0, 0, 0, tournament), nil)
			dave.On("GetCurrentSealedEpoch", blockRange(firstBlock, secondBlock)).
				Return(makeSealedEpochResult(2, 0, 0, tournament), nil)
			dave.On("GetCurrentSealedEpoch", blockRange(secondBlock, head+1)).
				Return(makeSealedEpochResult(3, 0, 0, tournament), nil)
			dave.On("RetrieveSealedEpochs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
				return opts.Start == firstBlock && opts.End != nil && *opts.End == firstBlock
			})).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
				makeSealedEpochEvent(1, 0, 0, firstBlock, tournament),
				makeSealedEpochEvent(2, 0, 0, firstBlock, tournament),
			}, nil).Twice()
			secondPage := mock.MatchedBy(func(opts *bind.FilterOpts) bool {
				return opts.Start == secondBlock && opts.End != nil && *opts.End == secondBlock
			})
			if !sameBlockFailure {
				dave.On("RetrieveSealedEpochs", secondPage).Return([]*idaveconsensus.IDaveConsensusEpochSealed{}, nil).Once()
			}
			dave.On("RetrieveSealedEpochs", secondPage).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
				makeSealedEpochEvent(3, 0, 0, secondBlock, tournament),
			}, nil).Once()
			repo.On("UpdateEpochClaimTransactionHash", t.Context(), app1Addr.Hex(), mock.Anything).Return(nil)

			require.Error(t, r.processApplicationSealedEpochs(t.Context(), app, head))
			require.Equal(t, cursor, app.application.LastEpochCheckBlock)
			require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
			repo.AssertNotCalled(t, "UpdateEventLastCheckBlock", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			require.NotNil(t, repo.epochs[1], "the failed scan must have a committed prefix")
			// Other services can process that prefix before the scan retries.
			repo.epochs[1].Status = model.EpochStatus_ClaimComputed
			commitment := common.HexToHash("0x4567")
			repo.epochs[1].Commitment = &commitment
			repo.epochs[1].CommitmentProof = []common.Hash{common.HexToHash("0x89ab")}
			before := *repo.epochs[1]
			repo.storeErr = nil
			repo.On("UpdateEventLastCheckBlock", t.Context(), []int64{app.application.ID}, model.MonitoredEvent_EpochSealed, head).
				Return(nil).Once()

			require.NoError(t, r.processApplicationSealedEpochs(t.Context(), app, head))
			require.Equal(t, before, *repo.epochs[1], "replay must preserve claim and proof progress")
			require.Equal(t, []uint64{1, 2, 3}, repo.stores, "already sealed rows must not be re-stored")
			require.Equal(t, firstBlock, repo.epochs[2].LastBlock, "the second same-block seal must survive retry")
			require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
			dave.AssertExpectations(t)
		})
	}
}

func TestSealedEpochReplayRejectsConflictingStoredFields(t *testing.T) {
	const block uint64 = 100
	tournament := common.HexToAddress("0x1234")
	for _, field := range []string{"last block", "upper bound", "missing tournament", "different tournament"} {
		t.Run(field, func(t *testing.T) {
			repo := newMockRepository()
			r := &Service{repository: repo}
			r.Logger = testLogger(t)
			app := newDaveAppContracts(nil, nil)
			app.application.IInputBoxBlock = 10
			epoch := &model.Epoch{Index: 0, FirstBlock: 10, LastBlock: block,
				Status: model.EpochStatus_ClaimComputed, TournamentAddress: &tournament}
			switch field {
			case "last block":
				epoch.LastBlock++
			case "upper bound":
				epoch.InputIndexUpperBound++
			case "missing tournament":
				epoch.TournamentAddress = nil
			case "different tournament":
				other := common.HexToAddress("0x5678")
				epoch.TournamentAddress = &other
			}
			before := *epoch
			repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(0)).Return(epoch, nil).Once()
			err := r.processSealedEpochEvent(t.Context(), app, makeSealedEpochEvent(0, 0, 0, block, tournament))
			require.ErrorContains(t, err, "data mismatch with replayed event")
			require.Equal(t, before, *epoch)
			repo.AssertNotCalled(t, "CreateEpochsAndInputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}
