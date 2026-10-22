// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSealedEpochFetchesOnlyUnstoredInputs(t *testing.T) {
	const sealBlock uint64 = 101
	for _, test := range []struct {
		name     string
		upper    uint64
		indices  []uint64
		mismatch bool
	}{
		{name: "first unstored index is included", upper: 6, indices: []uint64{5}},
		{name: "several new inputs and next epoch input", upper: 8, indices: []uint64{5, 6, 7, 8}},
		{name: "all inputs already stored", upper: 5},
		{name: "missing input prevents sealing", upper: 7, indices: []uint64{6}, mismatch: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockRepository()
			inputBox := newMockInputBox()
			reader := &Service{repository: repo, maxInputsPerStep: 500}
			reader.Logger = testLogger(t)
			app := newDaveAppContracts(inputBox, nil)
			previous := &Epoch{Index: 0, LastBlock: 50, InputIndexUpperBound: 3, Status: EpochStatus_Closed}
			// Inputs 3 and 4 were stored while this epoch was open. LastBlock
			// represents a completely scanned block; index 5 is next, not 6.
			open := &Epoch{Index: 1, FirstBlock: 50, LastBlock: 100,
				InputIndexLowerBound: 3, InputIndexUpperBound: 5, Status: EpochStatus_Open}
			event := makeSealedEpochEvent(1, 3, test.upper, sealBlock, common.HexToAddress("0xaaaa"))
			repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(0)).Return(previous, nil).Once()
			repo.On("UpdateEpochClaimTransactionHash", t.Context(), app1Addr.Hex(), previous).Return(nil).Once()
			repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).Return(open, nil).Once()

			if test.upper > 5 {
				count := test.upper
				for _, index := range test.indices {
					count = max(count, index+1)
				}
				// Only the unscanned block may be queried. Fetching the full
				// epoch again fails these strict mock expectations.
				inputBox.On("GetNumberOfInputs", mock.MatchedBy(func(opts *bind.CallOpts) bool {
					return opts.BlockNumber.Uint64() == sealBlock
				}), app1Addr).Return(new(big.Int).SetUint64(count), nil).Once()
				events := make([]iinputbox.IInputBoxInputAdded, 0, len(test.indices))
				for _, index := range test.indices {
					if index < 5 || index >= test.upper {
						continue
					}
					events = append(events, makeInputEvent(app1Addr, index, sealBlock))
				}
				inputBox.On("RetrieveInputs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
					return opts.Start == sealBlock && opts.End != nil && *opts.End == sealBlock
				}), []common.Address{app1Addr}, inputIndexRange(5, test.upper)).Return(events, nil).Once()
			}
			if !test.mismatch {
				repo.On("CreateEpochsAndInputs", t.Context(), app1Addr.Hex(), mock.Anything, sealBlock-1).
					Run(func(args mock.Arguments) {
						epochs := args.Get(2).(map[*Epoch][]*Input)
						require.Len(t, epochs, 1)
						for epoch, inputs := range epochs {
							require.Equal(t, EpochStatus_Closed, epoch.Status)
							require.Equal(t, uint64(50), epoch.FirstBlock)
							require.Equal(t, sealBlock, epoch.LastBlock)
							require.Equal(t, uint64(3), epoch.InputIndexLowerBound)
							require.Equal(t, test.upper, epoch.InputIndexUpperBound)
							require.Equal(t, event.Tournament, *epoch.TournamentAddress)
							require.Len(t, inputs, int(test.upper-5))
							for i, input := range inputs {
								require.Equal(t, uint64(5+i), input.Index)
							}
						}
					}).Return(nil).Once()
			}
			err := reader.processSealedEpochEvent(t.Context(), app, event)
			if test.mismatch {
				require.ErrorContains(t, err, "input count mismatch")
				repo.AssertNotCalled(t, "CreateEpochsAndInputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			} else {
				require.NoError(t, err)
			}
			repo.AssertExpectations(t)
			inputBox.AssertExpectations(t)
		})
	}
}
