// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestOpenEpochSkipsAlreadyScannedFreshCursor(t *testing.T) {
	const head uint64 = 200
	for _, test := range []struct {
		name   string
		cursor uint64
	}{
		{name: "equal to head", cursor: head},
		{name: "above head", cursor: head + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockRepository()
			inputBox := newMockInputBox()
			r := &Service{
				Service:    service.Service{Logger: testLogger(t)},
				repository: repo,
			}
			app := appContracts{
				application: &model.Application{
					ID: 1, IApplicationAddress: app1Addr, LastInputCheckBlock: head - 1,
				},
				inputSource: inputBox,
			}
			sealed := &model.Epoch{Index: 0, LastBlock: head - 1, Status: model.EpochStatus_Closed}
			open := &model.Epoch{Index: 1, FirstBlock: sealed.LastBlock, LastBlock: test.cursor, Status: model.EpochStatus_Open}
			before := *open
			repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(sealed, nil).Once()
			repo.On("GetEpoch", t.Context(), app1Addr.Hex(), open.Index).Return(open, nil).Once()
			repo.On("GetEventLastCheckBlock", t.Context(), app.application.ID, model.MonitoredEvent_InputAdded).
				Return(test.cursor, nil).Once()

			require.NoError(t, r.processApplicationOpenEpoch(t.Context(), app, head))
			require.Equal(t, before, *open, "the stored epoch must not be rewritten to an older head")
			require.Empty(t, inputBox.Calls)
			repo.AssertNotCalled(t, "GetNumberOfInputs", mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "CreateEpochsAndInputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}

func TestForeclosureInputDrainRunsBelowSealedCursor(t *testing.T) {
	const (
		sealedCursor uint64 = 200
		foreclosure  uint64 = 180
		inputCursor  uint64 = 150
	)
	repo := newMockRepository()
	inputBox := newMockInputBox()
	dave := newMockDaveConsensus()
	r := &Service{
		Service: service.Service{Logger: testLogger(t)}, repository: repo,
	}
	app := newDaveAppContracts(inputBox, dave)
	app.application.Status = model.ApplicationStatus_MachineHalted
	app.application.ForecloseBlock = foreclosure
	app.application.LastEpochCheckBlock = sealedCursor
	app.application.LastInputCheckBlock = inputCursor
	repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(&model.Epoch{
		Index: 0, LastBlock: 100, Status: model.EpochStatus_Closed,
	}, nil).Once()
	repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).Return(nil, nil).Once()
	repo.On("GetEventLastCheckBlock", t.Context(), app.application.ID, model.MonitoredEvent_InputAdded).
		Return(inputCursor, nil).Once()
	repo.On("GetNumberOfInputs", t.Context(), app1Addr.Hex()).Return(uint64(0), nil).Once()
	inputBox.On("GetNumberOfInputs", blockRange(inputCursor+1, foreclosure+1), app1Addr).
		Return(new(big.Int), nil).Twice()
	repo.On("CreateEpochsAndInputs", t.Context(), app1Addr.Hex(), mock.MatchedBy(func(epochs map[*model.Epoch][]*model.Input) bool {
		for epoch, inputs := range epochs {
			return len(epochs) == 1 && epoch.LastBlock == foreclosure && epoch.Status == model.EpochStatus_Open && len(inputs) == 0
		}
		return false
	}), foreclosure).Return(nil).Once()

	r.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, sealedCursor+10)
	require.Equal(t, model.ApplicationStatus_MachineHalted, app.application.Status)
	require.Empty(t, dave.Calls, "the sealed cursor already covers foreclosure")
	repo.AssertNotCalled(t, "UpdateEventLastCheckBlock", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	inputBox.AssertExpectations(t)
}
