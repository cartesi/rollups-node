// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildIConsensusInputScanUnits_GroupsByInputBoxAndCursor(t *testing.T) {
	ctx := context.Background()
	reader := &Service{
		Service: service.Service{Logger: testLogger(t)},
	}
	inputBoxA := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	inputBoxB := common.HexToAddress("0x00000000000000000000000000000000000000b1")

	tests := []struct {
		name  string
		apps  []appContracts
		units map[common.Address]map[iConsensusInputScanRange][]int64
	}{
		{
			name: "same input box and cursor share one unit",
			apps: []appContracts{
				inputUnitApp(1, inputBoxA, 10, true),
				inputUnitApp(2, inputBoxA, 10, true),
			},
			units: map[common.Address]map[iConsensusInputScanRange][]int64{
				inputBoxA: {{10, 20}: {1, 2}},
			},
		},
		{
			name: "same input box and different cursor produce separate units",
			apps: []appContracts{
				inputUnitApp(1, inputBoxA, 10, true),
				inputUnitApp(2, inputBoxA, 11, true),
			},
			units: map[common.Address]map[iConsensusInputScanRange][]int64{
				inputBoxA: {{10, 20}: {1}, {11, 20}: {2}},
			},
		},
		{
			name: "different input boxes produce separate units",
			apps: []appContracts{
				inputUnitApp(1, inputBoxA, 10, true),
				inputUnitApp(2, inputBoxB, 10, true),
			},
			units: map[common.Address]map[iConsensusInputScanRange][]int64{
				inputBoxA: {{10, 20}: {1}},
				inputBoxB: {{10, 20}: {2}},
			},
		},
		{
			name: "non-InputBox data availability apps are excluded",
			apps: []appContracts{
				inputUnitApp(1, inputBoxA, 10, true),
				inputUnitApp(2, inputBoxA, 10, false),
			},
			units: map[common.Address]map[iConsensusInputScanRange][]int64{
				inputBoxA: {{10, 20}: {1}},
			},
		},
		{
			name: "foreclosed app scans only through the foreclose block",
			apps: []appContracts{
				inputUnitAppWithForeclose(1, inputBoxA, 10, true, 15),
			},
			units: map[common.Address]map[iConsensusInputScanRange][]int64{
				inputBoxA: {{10, 15}: {1}},
			},
		},
		{
			name: "foreclosed app already checked through foreclosure is excluded",
			apps: []appContracts{
				inputUnitAppWithForeclose(1, inputBoxA, 15, true, 15),
			},
			units: map[common.Address]map[iConsensusInputScanRange][]int64{},
		},
		{
			name: "same input box and cursor split when foreclosure changes end block",
			apps: []appContracts{
				inputUnitApp(1, inputBoxA, 10, true),
				inputUnitAppWithForeclose(2, inputBoxA, 10, true, 15),
			},
			units: map[common.Address]map[iConsensusInputScanRange][]int64{
				inputBoxA: {{10, 20}: {1}, {10, 15}: {2}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			units := reader.buildIConsensusInputScanUnits(ctx, tt.apps, 20)
			require.Equal(t, tt.units, inputScanUnitIDs(units))
		})
	}
}

func TestBuildIConsensusInputScanUnits_InitializesBeforeGrouping(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.On("UpdateEventLastCheckBlock", mock.Anything, []int64{int64(1)}, MonitoredEvent_InputAdded, uint64(6)).
		Return(nil).Once()
	reader := &Service{
		Service:    service.Service{Logger: testLogger(t)},
		repository: repo,
	}
	inputBox := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	app := inputUnitApp(1, inputBox, 0, true)
	app.application.IInputBoxBlock = 7

	units := reader.buildIConsensusInputScanUnits(ctx, []appContracts{app}, 20)

	require.Equal(t, map[common.Address]map[iConsensusInputScanRange][]int64{
		inputBox: {{6, 20}: {1}},
	}, inputScanUnitIDs(units))
	require.Equal(t, uint64(6), app.application.LastInputCheckBlock)
	repo.AssertExpectations(t)
}

func TestBuildIConsensusInputScanUnits_FailedInitializationExcludesOnlyThatApp(t *testing.T) {
	ctx := context.Background()
	reader := &Service{
		Service: service.Service{Logger: testLogger(t)},
	}
	inputBox := common.HexToAddress("0x00000000000000000000000000000000000000a1")
	broken := inputUnitApp(1, inputBox, 0, true)
	broken.application.IInputBoxBlock = 0
	good := inputUnitApp(2, inputBox, 10, true)

	units := reader.buildIConsensusInputScanUnits(ctx, []appContracts{broken, good}, 20)

	require.Equal(t, map[common.Address]map[iConsensusInputScanRange][]int64{
		inputBox: {{10, 20}: {2}},
	}, inputScanUnitIDs(units))
}

func inputUnitApp(id int64, inputBox common.Address, cursor uint64, hasInputBoxDA bool) appContracts {
	return inputUnitAppWithForeclose(id, inputBox, cursor, hasInputBoxDA, 0)
}

func inputUnitAppWithForeclose(
	id int64,
	inputBox common.Address,
	cursor uint64,
	hasInputBoxDA bool,
	forecloseBlock uint64,
) appContracts {
	dataAvailability := []byte{0xff}
	if hasInputBoxDA {
		dataAvailability = DataAvailability_InputBox[:]
	}
	return appContracts{application: &Application{
		ID:                  id,
		Name:                "app",
		IApplicationAddress: common.BigToAddress(big.NewInt(id)),
		IInputBoxAddress:    inputBox,
		IInputBoxBlock:      1,
		DataAvailability:    dataAvailability,
		Enabled:             true,
		Status:              ApplicationStatus_OK,
		LastInputCheckBlock: cursor,
		ForecloseBlock:      forecloseBlock,
	}}
}

func inputScanUnitIDs(units []iConsensusInputScanUnit) map[common.Address]map[iConsensusInputScanRange][]int64 {
	result := map[common.Address]map[iConsensusInputScanRange][]int64{}
	for _, unit := range units {
		byCursor := result[unit.inputBoxAddress]
		if byCursor == nil {
			byCursor = map[iConsensusInputScanRange][]int64{}
			result[unit.inputBoxAddress] = byCursor
		}
		byCursor[iConsensusInputScanRange{
			lastInputCheckBlock: unit.lastInputCheckBlock,
			endBlock:            unit.endBlock,
		}] = planTargetIDs(unit.apps)
	}
	return result
}
