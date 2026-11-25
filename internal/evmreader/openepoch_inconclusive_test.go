// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDaveInputObservationRetriesFromSameFrontierWithoutPublishingIncompleteInputs(t *testing.T) {
	const head uint64 = 100
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing all logs", true: "missing last log"}[partial], func(t *testing.T) {
			repo, inputBox := newMockRepository(), newMockInputBox()
			app := newDaveAppContracts(inputBox, nil)
			app.application.LastInputCheckBlock = head - 1
			app.application.Status = model.ApplicationStatus_OK

			prevIndex, total := uint64(0), uint64(1)
			var firstLogs []iinputbox.IInputBoxInputAdded
			if partial {
				prevIndex, total = 1, 3
				firstLogs = []iinputbox.IInputBoxInputAdded{makeInputEvent(app1Addr, prevIndex, head)}
			}
			var output bytes.Buffer
			reader := &Service{repository: repo}
			reader.Logger = slog.New(slog.NewJSONHandler(&output, nil))
			repo.On("GetInputObservation", t.Context(), app1Addr.Hex()).
				Return(&model.InputObservation{ThroughBlock: head - 1, NextInputIndex: prevIndex}, nil)
			inputBox.On("GetNumberOfInputs", inputCountBlock(head), app1Addr).
				Return(new(big.Int).SetUint64(total), nil).Twice()
			inputBox.On("RetrieveInputs", inputLogBlock(head), []common.Address{app1Addr}, mock.Anything).
				Return(firstLogs, nil).Once()

			_, err := reader.observeDaveInputs(t.Context(), app, head)
			require.ErrorIs(t, err, errInconclusiveInputView)
			require.ErrorContains(t, err, "input count mismatch")
			repo.AssertNotCalled(t, "ObserveDaveInputs", mock.Anything, mock.Anything,
				mock.Anything, mock.Anything, mock.Anything)

			// The next response is complete at the same head. The persisted
			// frontier still selects the original block, including the missing
			// input.
			inputBox.Unset("RetrieveInputs")
			complete := make([]iinputbox.IInputBoxInputAdded, 0, total-prevIndex)
			for index := prevIndex; index < total; index++ {
				complete = append(complete, makeInputEvent(app1Addr, index, head))
			}
			inputBox.On("RetrieveInputs", inputLogBlock(head), []common.Address{app1Addr}, mock.Anything).
				Return(complete, nil).Once()
			repo.On("ObserveDaveInputs", t.Context(), app1Addr.Hex(), head, total,
				mock.MatchedBy(func(inputs []*model.Input) bool {
					return len(inputs) == len(complete)
				})).Return(nil).Once()

			observation, err := reader.observeDaveInputs(t.Context(), app, head)
			require.NoError(t, err)
			require.Equal(t, head, observation.ThroughBlock)
			require.Equal(t, total, observation.NextInputIndex)
			require.NotContains(t, output.String(), `"level":"ERROR"`)
			repo.AssertExpectations(t)
			inputBox.AssertExpectations(t)
		})
	}
}
