// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

const (
	activeApplicationName     = "active application"
	foreclosedApplicationName = "foreclosed application"
	secondApplicationName     = "second"
	boundaryReadName          = "application boundary"
	applicationPageName       = "application page"
	configuredHeadName        = "configured head"
)

func tickFixture(t *testing.T) (*deliveryFixture, *ethClientMock) {
	t.Helper()
	f := newDeliveryFixture(t)
	f.d.delivery = f.d
	f.d.defaultBlock = model.DefaultBlock_Finalized
	client := &ethClientMock{}
	f.d.client = client
	expectNumericWindowHeaders(client)
	return f, client
}

func tickHead(client *ethClientMock, head uint64) {
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: new(big.Int).SetUint64(head)}, nil).Once()
}

func TestPRTTickOnlyObservesAndDoesNotCreateEmptyCoverage(t *testing.T) {
	f, client := tickFixture(t)
	f.d.submissionEnabled = true
	f.repo.publishedRoots = nil
	f.repo.storedTournaments = nil
	f.app.Status = model.ApplicationStatus_Failed
	second := *f.app
	second.ID++
	second.Name = secondApplicationName
	second.IApplicationAddress = common.HexToAddress("0x901")
	second.IConsensusAddress = common.HexToAddress("0x902")
	f.repo.apps = append(f.repo.apps, &second)
	secondConsensus := &daveConsensusAdapterMock{}
	f.factory.On("CreateDaveConsensusAdapter", second.IConsensusAddress).Return(secondConsensus, nil)
	f.repo.On("ListEpochs", mock.Anything, mock.Anything, repository.EpochFilter{HasTournament: new(true)},
		repository.Pagination{}, false).Return([]*model.Epoch{}, uint64(0), nil)
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, mock.Anything).Return((*uint64)(nil), nil)
	for tick := range 2 {
		head := uint64(101 + tick)
		tickHead(client, head)
		f.block.Number = uint64(120 + tick)
		expectProtectionReads(t, f.factory, f.consensus, observerFacts(head))
		expectProtectionReads(t, f.factory, secondConsensus, observerFacts(head))
		reschedule, err := f.d.Tick(t.Context())
		require.False(t, reschedule)
		require.NoError(t, err)
		require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
		require.Equal(t, uint64(100), second.LastTournamentCheckBlock)
	}
	require.Empty(t, f.backend.headerReads, "the observer never calls the sender")
	require.Empty(t, f.repo.windows, "no tournament means no fabricated interval")
	require.Empty(t, f.backend.published)
	client.AssertNotCalled(t, "BlockNumber", mock.Anything)
	client.AssertExpectations(t)
}

func TestPRTTickWithoutApplicationsDoesNotReadHead(t *testing.T) {
	f, client := tickFixture(t)
	f.repo.apps = nil
	reschedule, err := f.d.Tick(t.Context())
	require.False(t, reschedule)
	require.NoError(t, err)
	require.Empty(t, client.Calls)
}

func TestPRTTickSuppressesHeadReadCancellationDuringShutdown(t *testing.T) {
	f, client := tickFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Run(func(mock.Arguments) { cancel() }).Return((*types.Header)(nil), context.Canceled).Once()
	_, err := f.d.Tick(ctx)
	require.NoError(t, err)
	require.Empty(t, f.backend.published)
}

func TestPRTTickPreservesDependencyCancellationAndDeadlines(t *testing.T) {
	dbError := errors.New("database unavailable")
	branches := []string{boundaryReadName, applicationPageName, configuredHeadName, activeApplicationName, foreclosedApplicationName}
	for _, branch := range branches {
		for _, test := range []struct {
			name     string
			shutdown bool
			cause    error
			quiet    bool
		}{
			{"shutdown cancellation", true, context.Canceled, true},
			{"wrapped shutdown", true, fmt.Errorf("read: %w", context.Canceled), true},
			{"joined shutdown", true, errors.Join(context.Canceled, fmt.Errorf("read: %w", context.Canceled)), true},
			{"dependency cancellation", false, context.Canceled, false},
			{"database during shutdown", true, dbError, false},
			{"mixed database during shutdown", true, errors.Join(context.Canceled, dbError), false},
			{"deadline during shutdown", true, errors.Join(context.Canceled, context.DeadlineExceeded), false},
		} {
			t.Run(branch+"/"+test.name, func(t *testing.T) {
				f, client := tickFixture(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if branch == foreclosedApplicationName {
					f.app.ForecloseBlock = 100
				}
				switch branch {
				case boundaryReadName:
					f.repo.boundaryError = test.cause
				case applicationPageName:
					f.repo.pageError = test.cause
					tickHead(client, 100)
				case configuredHeadName:
					client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
						Return((*types.Header)(nil), test.cause).Once()
				default:
					tickHead(client, 100)
					f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
						Return([]*model.Epoch(nil), uint64(0), test.cause).Run(func(mock.Arguments) {
						if test.shutdown {
							cancel()
						}
					}).Once()
				}
				if test.shutdown && branch != activeApplicationName && branch != foreclosedApplicationName {
					// Cancellation happens at the failing dependency, after Tick's entry check.
					f.repo.onBoundary = func() {
						if branch == boundaryReadName {
							cancel()
						}
					}
					f.repo.onPage = func() {
						if branch == applicationPageName {
							cancel()
						}
					}
					if branch == configuredHeadName {
						client.ExpectedCalls[len(client.ExpectedCalls)-1].Run(func(mock.Arguments) { cancel() })
					}
				}
				if !test.shutdown && (branch == activeApplicationName || branch == foreclosedApplicationName) {
					expectProtectionReads(t, f.factory, f.consensus, observerFacts(100))
				}
				_, err := f.d.Tick(ctx)
				if test.quiet {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, test.cause)
				}
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
				require.Empty(t, f.backend.published)
			})
		}
	}
}

func TestPRTTickRetainsEarlierApplicationErrorDuringShutdown(t *testing.T) {
	f, client := tickFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	firstError := errors.New("first observation failed")
	second := *f.app
	second.ID++
	second.Name = "second"
	second.ForecloseBlock = 100
	f.repo.apps = append(f.repo.apps, &second)
	tickHead(client, 100)
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch(nil), uint64(0), firstError).Once()
	f.repo.On("ListEpochs", mock.Anything, second.Name, mock.Anything, repository.Pagination{}, false).
		Run(func(mock.Arguments) { cancel() }).Return([]*model.Epoch(nil), uint64(0), context.Canceled).Once()
	expectProtectionReads(t, f.factory, f.consensus, observerFacts(100))
	_, err := f.d.Tick(ctx)
	require.ErrorIs(t, err, firstError)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, f.backend.published)
}
