// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"errors"
	"io"
	"log/slog"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestApplicationAdapterResolver_ReusesAdaptersWithLatestApplicationPointer(t *testing.T) {
	factory := &countingAdapterFactory{}
	resolver := newApplicationAdapterResolver(testLogger(t), factory)

	app := resolverApp(1)
	contracts := resolver.buildAppContracts([]*Application{app})
	require.Len(t, contracts, 1)
	require.Same(t, app, contracts[0].application)
	require.Len(t, factory.calls, 1)

	refreshedApp := resolverApp(1)
	refreshedApp.LastInputCheckBlock = 100
	contracts = resolver.buildAppContracts([]*Application{refreshedApp})
	require.Len(t, contracts, 1)
	require.Same(t, refreshedApp, contracts[0].application)
	require.Len(t, factory.calls, 1)
}

func TestApplicationAdapterResolver_InvalidatesStaleAdapters(t *testing.T) {
	tests := []struct {
		name   string
		change func(app *Application)
	}{
		{
			name: "consensus address changed",
			change: func(app *Application) {
				app.IConsensusAddress = common.HexToAddress("0x00000000000000000000000000000000000000c2")
			},
		},
		{
			name: "input box address changed",
			change: func(app *Application) {
				app.IInputBoxAddress = common.HexToAddress("0x00000000000000000000000000000000000000b2")
			},
		},
		{
			name: "consensus type changed",
			change: func(app *Application) {
				app.ConsensusType = Consensus_PRT
			},
		},
		{
			name: "InputBox data availability changed",
			change: func(app *Application) {
				app.DataAvailability = []byte{0xff}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := &countingAdapterFactory{}
			resolver := newApplicationAdapterResolver(testLogger(t), factory)

			contracts := resolver.buildAppContracts([]*Application{resolverApp(1)})
			require.Len(t, contracts, 1)

			changed := resolverApp(1)
			tt.change(changed)
			contracts = resolver.buildAppContracts([]*Application{changed})
			require.Len(t, contracts, 1)
			require.Len(t, factory.calls, 2)
		})
	}
}

func TestApplicationAdapterResolver_EvictsRemovedApplications(t *testing.T) {
	factory := &countingAdapterFactory{}
	resolver := newApplicationAdapterResolver(testLogger(t), factory)

	contracts := resolver.buildAppContracts([]*Application{resolverApp(1), resolverApp(2)})
	require.Len(t, contracts, 2)
	require.Len(t, factory.calls, 2)

	contracts = resolver.buildAppContracts([]*Application{resolverApp(2)})
	require.Len(t, contracts, 1)
	require.Len(t, factory.calls, 2)

	contracts = resolver.buildAppContracts([]*Application{resolverApp(1), resolverApp(2)})
	require.Len(t, contracts, 2)
	require.Len(t, factory.calls, 3)
}

func TestApplicationAdapterResolver_DoesNotCacheCreationErrors(t *testing.T) {
	factory := &countingAdapterFactory{
		results: []adapterFactoryResult{
			{err: errors.New("boom")},
			{},
		},
	}
	resolver := newApplicationAdapterResolver(testLogger(t), factory)

	contracts := resolver.buildAppContracts([]*Application{resolverApp(1)})
	require.Empty(t, contracts)
	require.Len(t, factory.calls, 1)

	contracts = resolver.buildAppContracts([]*Application{resolverApp(1)})
	require.Len(t, contracts, 1)
	require.Len(t, factory.calls, 2)
}

type adapterFactoryResult struct {
	applicationContract ApplicationContractAdapter
	inputSource         InputSourceAdapter
	daveConsensus       DaveConsensusAdapter
	err                 error
}

type countingAdapterFactory struct {
	calls   []*Application
	results []adapterFactoryResult
}

func (f *countingAdapterFactory) CreateAdapters(
	app *Application,
) (ApplicationContractAdapter, InputSourceAdapter, DaveConsensusAdapter, error) {
	f.calls = append(f.calls, app)
	result := adapterFactoryResult{}
	if len(f.results) >= len(f.calls) {
		result = f.results[len(f.calls)-1]
	}
	if result.err != nil {
		return nil, nil, nil, result.err
	}
	if result.applicationContract == nil {
		result.applicationContract = newMockApplicationContract()
	}
	if result.inputSource == nil {
		result.inputSource = newMockInputBox()
	}
	return result.applicationContract, result.inputSource, result.daveConsensus, nil
}

func resolverApp(id int64) *Application {
	return &Application{
		ID:                  id,
		Name:                "app",
		IApplicationAddress: common.BigToAddress(big.NewInt(id)),
		IConsensusAddress:   common.HexToAddress("0x00000000000000000000000000000000000000c1"),
		IInputBoxAddress:    common.HexToAddress("0x00000000000000000000000000000000000000b1"),
		DataAvailability:    DataAvailability_InputBox[:],
		ConsensusType:       Consensus_Authority,
		Enabled:             true,
		Status:              ApplicationStatus_OK,
	}
}

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
