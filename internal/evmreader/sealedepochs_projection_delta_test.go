// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestOpenProjectionReadsOnlyNewPrefix pins that a growing OPEN projection
// reads and writes only [previousUpper, newUpper), not the whole epoch.
func TestOpenProjectionReadsOnlyNewPrefix(t *testing.T) {
	const (
		previousUpper = uint64(7)
		newUpper      = uint64(10)
		head          = uint64(200)
	)
	repo := newMockRepository()
	r := &Service{repository: repo}
	r.Logger = testLogger(t)
	app := appContracts{application: &Application{
		ID:                  1,
		IApplicationAddress: app1Addr,
		LastInputCheckBlock: 100,
	}}

	repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(&Epoch{
		Index:                0,
		LastBlock:            100,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: previousUpper,
		Status:               EpochStatus_Closed,
	}, nil).Once()
	repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).Return(&Epoch{
		Index:                1,
		FirstBlock:           100,
		LastBlock:            150,
		InputIndexLowerBound: previousUpper,
		InputIndexUpperBound: previousUpper,
		Status:               EpochStatus_Open,
	}, nil).Once()
	repo.On("GetEventLastCheckBlock", t.Context(), app.application.ID,
		MonitoredEvent_InputAdded).Return(uint64(100), nil).Once()

	delta := []*Input{
		{Index: 7, BlockNumber: 180, Status: InputCompletionStatus_None},
		{Index: 8, BlockNumber: 180, Status: InputCompletionStatus_None},
		{Index: 9, BlockNumber: 190, Status: InputCompletionStatus_None},
	}
	repo.On("ListObservedInputs", t.Context(), app1Addr.Hex(),
		previousUpper, newUpper).Return(delta, nil).Once()
	repo.On("ListInputs", t.Context(), app1Addr.Hex(),
		mock.MatchedBy(func(f repository.InputFilter) bool {
			return f.EpochIndex != nil && *f.EpochIndex == 1 &&
				f.IndexRange != nil &&
				f.IndexRange.Start == previousUpper &&
				f.IndexRange.End == newUpper-1
		}), repository.Pagination{}, false).Return(nil, uint64(0), nil).Once()

	var projected *Epoch
	repo.On("ProjectDaveOpenInputs", t.Context(), app1Addr.Hex(),
		mock.MatchedBy(func(epoch *Epoch) bool {
			return epoch.InputIndexLowerBound == previousUpper &&
				epoch.InputIndexUpperBound == newUpper &&
				epoch.LastBlock == head &&
				epoch.Status == EpochStatus_Open
		}),
		mock.MatchedBy(func(inputs []*Input) bool {
			return len(inputs) == len(delta)
		}), head).Run(func(args mock.Arguments) {
		projected = args.Get(2).(*Epoch)
	}).Return(nil).Once()

	require.NoError(t, r.projectDaveOpenEpoch(t.Context(), app, head,
		&InputObservation{ThroughBlock: head, NextInputIndex: newUpper}))
	require.NotNil(t, projected)
	repo.AssertExpectations(t)
}
