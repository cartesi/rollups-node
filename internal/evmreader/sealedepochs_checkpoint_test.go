// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// A sealed batch contains only inputs before the seal transaction, not the
// later inputs in that block. A failed open scan must not publish the block as
// complete, even if the sealed scan has finished and the reader restarts.
func TestDaveInputCheckpointAfterFailedOpenScan(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	ctx := context.Background()
	release, err := db.LockTestPostgres(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(release)

	for _, test := range []struct {
		name        string
		sealedSizes []uint64
		cancelWrite bool
	}{
		{name: "empty seal then RPC failure", sealedSizes: []uint64{0}},
		{name: "input seal input then RPC failure", sealedSizes: []uint64{1}},
		{name: "two seals then cancelled write", sealedSizes: []uint64{1, 1}, cancelWrite: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			const (
				previousSealBlock uint64 = 50
				forecloseBlock    uint64 = 100
			)
			require.NoError(t, db.SetupTestPostgres(endpoint))
			repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
			require.NoError(t, err)
			t.Cleanup(func() { repo.Close() })
			app := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(ctx, t, repo)
			address := app.IApplicationAddress.Hex()
			previousEpoch := &Epoch{Index: 0, FirstBlock: 10, LastBlock: previousSealBlock, Status: EpochStatus_Closed}
			require.NoError(t, repo.CreateEpochsAndInputs(ctx, address,
				map[*Epoch][]*Input{previousEpoch: nil}, forecloseBlock-2))
			require.NoError(t, repo.UpdateEventLastCheckBlock(ctx, []int64{app.ID},
				MonitoredEvent_EpochSealed, forecloseBlock-2))
			app, err = repo.GetApplication(ctx, address)
			require.NoError(t, err)

			var sealedInputs uint64
			var events []*idaveconsensus.IDaveConsensusEpochSealed
			for i, size := range test.sealedSizes {
				events = append(events, makeSealedEpochEvent(int64(i+1), sealedInputs, sealedInputs+size,
					forecloseBlock, repotest.UniqueAddress()))
				sealedInputs += size
			}
			lastEvent := events[len(events)-1]
			dave := newMockDaveConsensus()
			dave.On("GetCurrentSealedEpoch", blockRange(0, forecloseBlock)).
				Return(makeSealedEpochResult(0, 0, 0, common.Address{}), nil)
			dave.On("GetCurrentSealedEpoch", blockFrom(forecloseBlock)).Return(makeSealedEpochResult(
				lastEvent.EpochNumber.Int64(), lastEvent.InputIndexLowerBound.Uint64(), sealedInputs, lastEvent.Tournament), nil)
			dave.On("RetrieveSealedEpochs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
				return opts.Start == forecloseBlock && opts.End != nil && *opts.End == forecloseBlock
			})).Return(events, nil).Once()

			inputBox := newMockInputBox()
			inputBox.On("GetNumberOfInputs", blockRange(0, forecloseBlock), app.IApplicationAddress).
				Return(big.NewInt(0), nil).Maybe()
			inputBox.On("GetNumberOfInputs", blockFrom(forecloseBlock), app.IApplicationAddress).
				Return(new(big.Int).SetUint64(sealedInputs+1), nil)
			inputEvents := make([]iinputbox.IInputBoxInputAdded, 0, sealedInputs+1)
			for index := uint64(0); index <= sealedInputs; index++ {
				inputEvents = append(inputEvents, makeInputEvent(app.IApplicationAddress, index, forecloseBlock))
			}
			inputBlock := mock.MatchedBy(func(opts *bind.FilterOpts) bool {
				return opts.Start == forecloseBlock && opts.End != nil && *opts.End == forecloseBlock
			})
			inputBox.On("RetrieveInputs", inputBlock, []common.Address{app.IApplicationAddress}, mock.Anything).
				Return(inputEvents, nil)
			contracts := appContracts{application: app, inputSource: inputBox, daveConsensus: dave}
			reader := &Service{repository: repo}
			reader.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			require.NoError(t, reader.processApplicationSealedEpochs(ctx, contracts, forecloseBlock))

			// Interrupt the open scan either at its RPC read or before the input
			// transaction starts. No open epoch or final input may be published.
			inputBox.Unset("RetrieveInputs")
			openCtx, cancel := context.WithCancel(ctx)
			t.Cleanup(cancel)
			failedRead := inputBox.On("RetrieveInputs", inputBlock, []common.Address{app.IApplicationAddress}, mock.Anything).Once()
			if test.cancelWrite {
				failedRead.Run(func(mock.Arguments) { cancel() }).Return(inputEvents, nil)
			} else {
				failedRead.Return([]iinputbox.IInputBoxInputAdded{}, context.Canceled)
			}
			require.ErrorIs(t, reader.processApplicationOpenEpoch(openCtx, contracts, forecloseBlock), context.Canceled)
			count, err := repo.GetNumberOfInputs(ctx, address)
			require.NoError(t, err)
			require.Equal(t, sealedInputs, count)
			openEpochIndex := lastEvent.EpochNumber.Uint64() + 1
			openEpoch, err := repo.GetEpoch(ctx, address, openEpochIndex)
			require.NoError(t, err)
			require.Nil(t, openEpoch)

			// Recreate both the repository connection and reader, then discover
			// foreclosure in the same block. No tick-start snapshot survives.
			repo.Close()
			repo, err = factory.NewRepositoryFromConnectionString(ctx, endpoint)
			require.NoError(t, err)
			require.NoError(t, repo.UpdateApplicationForeclosure(ctx, app.ID, forecloseBlock,
				repotest.UniqueHash(), forecloseBlock))
			app, err = repo.GetApplication(ctx, address)
			require.NoError(t, err)
			assert.Equal(t, forecloseBlock, app.LastEpochCheckBlock)
			assert.Equal(t, forecloseBlock-1, app.LastInputCheckBlock, "the seal does not cover the whole block")
			assert.False(t, app.ForeclosureScanCaughtUp(), "drain must wait for the missing open input")

			inputBox.Unset("RetrieveInputs")
			inputBox.On("RetrieveInputs", inputBlock, []common.Address{app.IApplicationAddress}, mock.Anything).
				Return(inputEvents, nil).Once()
			contracts.application = app
			reader = &Service{repository: repo}
			reader.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			reader.scanDaveConsensusEpochsAndInputs(ctx, []appContracts{contracts}, forecloseBlock+10)

			input, err := repo.GetInput(ctx, address, sealedInputs)
			require.NoError(t, err)
			require.NotNil(t, input, "retry must fetch the input after the last seal")
			assert.Equal(t, openEpochIndex, input.EpochIndex)
			assert.Equal(t, forecloseBlock, input.BlockNumber)
			openEpoch, err = repo.GetEpoch(ctx, address, openEpochIndex)
			require.NoError(t, err)
			require.NotNil(t, openEpoch)
			assert.Equal(t, forecloseBlock, openEpoch.FirstBlock)
			assert.Equal(t, forecloseBlock, openEpoch.LastBlock)
			assert.Equal(t, sealedInputs, openEpoch.InputIndexLowerBound)
			assert.Equal(t, sealedInputs+1, openEpoch.InputIndexUpperBound)
			assert.Equal(t, EpochStatus_Open, openEpoch.Status)
			app, err = repo.GetApplication(ctx, address)
			require.NoError(t, err)
			assert.Equal(t, forecloseBlock, app.LastInputCheckBlock)
			assert.True(t, app.ForeclosureScanCaughtUp())
			undrained, err := repo.HasUndrainedEpochsBeforeBlock(ctx, app.ID, forecloseBlock)
			require.NoError(t, err)
			assert.True(t, undrained, "the stored input must remain visible to the machine drain")
			dave.AssertExpectations(t)
			inputBox.AssertExpectations(t)
		})
	}
}
