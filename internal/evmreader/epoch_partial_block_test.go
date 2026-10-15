// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"fmt"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDavePartialBlockPersistsValidEpochAndResumes(t *testing.T) {
	for _, mode := range []struct {
		name                    string
		firstSealed, lastSealed bool
	}{
		{name: "open"},
		{name: "sealed", firstSealed: true, lastSealed: true},
		{name: "open then sealed", lastSealed: true},
	} {
		for _, inputBlock := range []uint64{100, 101} {
			t.Run(fmt.Sprintf("%s/input_block_%d", mode.name, inputBlock), func(t *testing.T) {
				repo := newMockRepository()
				source := newMockInputBox()
				previous := &Epoch{Index: 0, FirstBlock: 10, LastBlock: 100, Status: EpochStatus_Closed}
				event := makeSealedEpochEvent(1, 0, 1000, 102, common.HexToAddress("0xaaaa"))
				source.On("GetNumberOfInputs", blockRange(100, inputBlock), app1Addr).
					Return(big.NewInt(0), nil).Maybe()
				source.On("GetNumberOfInputs", blockFrom(inputBlock), app1Addr).Return(big.NewInt(1000), nil)
				for _, first := range []uint64{0, 500} {
					source.On("RetrieveInputs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
						return opts.Start == inputBlock && opts.End != nil && *opts.End == inputBlock
					}), []common.Address{app1Addr}, inputIndexRange(first, first+500)).
						Return(batchInputEvents(first, first+500, inputBlock), nil).Once()
				}

				var persisted *Epoch
				cursor := uint64(99)
				stored := uint64(0)
				repo.On("CreateEpochsAndInputs", t.Context(), app1Addr.Hex(), mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						epochs := args.Get(2).(map[*Epoch][]*Input)
						require.Len(t, epochs, 1)
						for epoch, inputs := range epochs {
							// PostgreSQL enforces these bounds on every write.
							require.Equal(t, uint64(100), epoch.FirstBlock)
							require.GreaterOrEqual(t, epoch.LastBlock, epoch.FirstBlock)
							require.Len(t, inputs, 500)
							for i, input := range inputs {
								require.Equal(t, stored+uint64(i), input.Index)
								require.Equal(t, inputBlock, input.BlockNumber)
							}
							stored += uint64(len(inputs))
							require.Equal(t, stored, epoch.InputIndexUpperBound)
							copy := *epoch
							persisted = &copy
						}
						cursor = args.Get(3).(uint64)
					}).Return(nil).Twice()

				for batch, sealed := range []bool{mode.firstSealed, mode.lastSealed} {
					// Reconstruct the reader and epoch row between batches to ensure
					// resumption depends only on persisted state.
					reader := &Service{repository: repo, maxInputsPerStep: 500}
					reader.Logger = testLogger(t)
					app := newDaveAppContracts(source, nil)
					app.application.LastInputCheckBlock = cursor
					repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).Return(persisted, nil).Once()
					var err error
					if sealed {
						repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(0)).Return(previous, nil).Once()
						repo.On("UpdateEpochClaimTransactionHash", t.Context(), app1Addr.Hex(), previous).Return(nil).Once()
						err = reader.processSealedEpochEvent(t.Context(), app, event)
					} else {
						repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(previous, nil).Once()
						repo.On("GetEventLastCheckBlock", t.Context(), app.application.ID, MonitoredEvent_InputAdded).
							Return(cursor, nil).Once()
						repo.On("GetNumberOfInputs", t.Context(), app1Addr.Hex()).Return(stored, nil).Once()
						err = reader.processApplicationOpenEpoch(t.Context(), app, 102)
					}
					if batch == 0 && sealed {
						require.ErrorIs(t, err, ethutil.ErrIncompleteProcessing)
					} else {
						require.NoError(t, err)
					}
					if batch == 0 {
						require.Equal(t, inputBlock, persisted.LastBlock)
						require.Equal(t, inputBlock-1, cursor)
						require.Equal(t, EpochStatus_Open, persisted.Status)
					}
				}
				require.Equal(t, uint64(1000), stored)
				if mode.lastSealed {
					require.Equal(t, EpochStatus_Closed, persisted.Status)
					require.Equal(t, uint64(102), persisted.LastBlock)
					require.Equal(t, uint64(101), cursor)
				} else {
					require.Equal(t, EpochStatus_Open, persisted.Status)
					require.Equal(t, inputBlock, persisted.LastBlock)
					require.Equal(t, inputBlock, cursor)
				}
				repo.AssertExpectations(t)
				source.AssertExpectations(t)
			})
		}
	}
}
