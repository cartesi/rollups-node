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

func TestDaveInputObservationChecksCounterBeforePublishing(t *testing.T) {
	const head uint64 = 100
	for _, test := range []struct {
		name        string
		frontier    *model.InputObservation
		endCount    uint64
		logIndices  []uint64
		wantIndices []uint64
		mismatch    bool
	}{
		{name: "missing first log", frontier: &model.InputObservation{ThroughBlock: head - 1, NextInputIndex: 0},
			endCount: 2, logIndices: []uint64{1}, mismatch: true},
		{name: "missing all logs", frontier: &model.InputObservation{ThroughBlock: head - 1, NextInputIndex: 0},
			endCount: 1, mismatch: true},
		{name: "no new inputs", frontier: &model.InputObservation{ThroughBlock: head - 1, NextInputIndex: 2},
			endCount: 2},
		{name: "new inputs", frontier: &model.InputObservation{ThroughBlock: head - 1, NextInputIndex: 2},
			endCount: 5, logIndices: []uint64{2, 3, 4}, wantIndices: []uint64{2, 3, 4}},
		{name: "multiple logs in the transition block are all kept",
			frontier: &model.InputObservation{ThroughBlock: head - 1, NextInputIndex: 0},
			endCount: 3, logIndices: []uint64{0, 1, 2}, wantIndices: []uint64{0, 1, 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockRepository()
			inputBox := newMockInputBox()
			reader := &Service{repository: repo}
			reader.Logger = testLogger(t)
			app := newDaveAppContracts(inputBox, nil)
			app.application.Status = model.ApplicationStatus_OK

			repo.On("GetInputObservation", t.Context(), app1Addr.Hex()).
				Return(test.frontier, nil).Once()
			inputBox.On("GetNumberOfInputs", blockRange(head, head+1), app1Addr).
				Return(new(big.Int).SetUint64(test.endCount), nil).Maybe()
			if test.endCount > test.frontier.NextInputIndex {
				events := make([]iinputbox.IInputBoxInputAdded, 0, len(test.logIndices))
				for _, index := range test.logIndices {
					events = append(events, makeInputEvent(app1Addr, index, head))
				}
				inputBox.On("RetrieveInputs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
					return opts.Start == head && opts.End != nil && *opts.End == head
				}), []common.Address{app1Addr}, mock.Anything).Return(events, nil).Once()
			}
			if !test.mismatch {
				repo.On("ObserveDaveInputs", t.Context(), app1Addr.Hex(), head, test.endCount,
					mock.MatchedBy(func(inputs []*model.Input) bool {
						if len(inputs) != len(test.wantIndices) {
							return false
						}
						for i, index := range test.wantIndices {
							if inputs[i].Index != index {
								return false
							}
						}
						return true
					})).Return(nil).Once()
			}

			observation, err := reader.observeDaveInputs(t.Context(), app, head)
			if test.mismatch {
				require.ErrorIs(t, err, errInconclusiveInputView)
				repo.AssertNotCalled(t, "ObserveDaveInputs", mock.Anything, mock.Anything,
					mock.Anything, mock.Anything, mock.Anything)
			} else {
				require.NoError(t, err)
				require.Equal(t, head, observation.ThroughBlock)
				require.Equal(t, test.endCount, observation.NextInputIndex)
			}
			repo.AssertExpectations(t)
			inputBox.AssertExpectations(t)
		})
	}
}
