// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDaveOpenEpochChecksInputCountBeforePublication(t *testing.T) {
	const head uint64 = 100
	for _, foreclosed := range []bool{false, true} {
		name := "active"
		if foreclosed {
			name = "foreclosed"
		}
		for _, test := range []struct {
			name        string
			storedCount uint64
			lowerBound  uint64
			endCount    uint64
			logIndices  []uint64
			wantIndices []uint64
			mismatch    bool
		}{
			{name: "missing first log", endCount: 2, logIndices: []uint64{1}, mismatch: true},
			{name: "missing all logs", endCount: 1, mismatch: true},
			{name: "existing open epoch misses a log", storedCount: 4, lowerBound: 2, endCount: 6,
				logIndices: []uint64{5}, mismatch: true},
			{name: "overlap block includes sealed inputs", storedCount: 2, lowerBound: 2, endCount: 3,
				logIndices: []uint64{0, 1, 2}, wantIndices: []uint64{2}},
			{name: "no new inputs", storedCount: 2, lowerBound: 2, endCount: 2},
			{name: "existing open inputs", storedCount: 4, lowerBound: 2, endCount: 5,
				logIndices: []uint64{4}, wantIndices: []uint64{4}},
		} {
			t.Run(name+"/"+test.name, func(t *testing.T) {
				repo := newMockRepository()
				inputBox := newMockInputBox()
				r := &Service{repository: repo}
				r.Logger = testLogger(t)
				app := newDaveAppContracts(inputBox, nil)
				app.application.Status = model.ApplicationStatus_OK
				app.application.LastEpochCheckBlock = head
				app.application.LastInputCheckBlock = head - 1
				observedHead := head
				if foreclosed {
					app.application.ForecloseBlock = head
					observedHead += 10
				}
				sealed := &model.Epoch{Index: 0, LastBlock: head,
					InputIndexUpperBound: test.lowerBound, Status: model.EpochStatus_Closed}
				var open *model.Epoch
				if test.storedCount > test.lowerBound {
					sealed.LastBlock = head - 2
					open = &model.Epoch{Index: 1, FirstBlock: sealed.LastBlock, LastBlock: head - 1,
						InputIndexLowerBound: test.lowerBound, InputIndexUpperBound: test.storedCount,
						Status: model.EpochStatus_Open}
				}
				repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(sealed, nil).Once()
				repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).Return(open, nil).Once()
				repo.On("GetEventLastCheckBlock", t.Context(), app.application.ID, model.MonitoredEvent_InputAdded).
					Return(head-1, nil).Once()
				repo.On("GetNumberOfInputs", t.Context(), app1Addr.Hex()).Return(test.storedCount, nil).Once()
				inputBox.On("GetNumberOfInputs", blockRange(head, head+1), app1Addr).
					Return(new(big.Int).SetUint64(test.endCount), nil).Once()
				if test.endCount > test.storedCount {
					events := make([]iinputbox.IInputBoxInputAdded, 0, len(test.logIndices))
					for _, index := range test.logIndices {
						events = append(events, makeInputEvent(app1Addr, index, head))
					}
					inputBox.On("RetrieveInputs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
						return opts.Start == head && opts.End != nil && *opts.End == head
					}), []common.Address{app1Addr}, mock.Anything).Return(events, nil).Once()
				}
				if !test.mismatch {
					repo.On("CreateEpochsAndInputs", t.Context(), app1Addr.Hex(), mock.Anything, head).
						Run(func(args mock.Arguments) {
							epochs := args.Get(2).(map[*model.Epoch][]*model.Input)
							require.Len(t, epochs, 1)
							for epoch, inputs := range epochs {
								require.Equal(t, uint64(1), epoch.Index)
								require.Equal(t, sealed.LastBlock, epoch.FirstBlock)
								require.Equal(t, head, epoch.LastBlock)
								require.Equal(t, test.lowerBound, epoch.InputIndexLowerBound)
								require.Equal(t, test.endCount, epoch.InputIndexUpperBound)
								require.Equal(t, model.EpochStatus_Open, epoch.Status)
								require.Len(t, inputs, len(test.wantIndices))
								for i, index := range test.wantIndices {
									require.Equal(t, index, inputs[i].Index)
								}
							}
							app.application.LastInputCheckBlock = head
						}).Return(nil).Once()
				}

				require.Equal(t, !test.mismatch, r.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, observedHead))
				if test.mismatch {
					require.Equal(t, head-1, app.application.LastInputCheckBlock)
					if open != nil {
						require.Equal(t, test.storedCount, open.InputIndexUpperBound)
						require.Equal(t, head-1, open.LastBlock)
					}
					repo.AssertNotCalled(t, "CreateEpochsAndInputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				} else {
					require.Equal(t, head, app.application.LastInputCheckBlock)
				}
				require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
				repo.AssertNotCalled(t, "UpdateEventLastCheckBlock", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				repo.AssertExpectations(t)
				inputBox.AssertExpectations(t)
			})
		}
	}
}
