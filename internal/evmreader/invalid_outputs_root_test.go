// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInvalidOutputsRootOutputObservation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mismatched bool
	}{
		{name: "matching output is indexed"},
		{name: "mismatching output preserves diagnosis and advances cursor", mismatched: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepository()
			contract := newMockApplicationContract()
			var logs bytes.Buffer
			reader := &Service{repository: repo}
			reader.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			app := &model.Application{
				ID: 1, Name: "invalid-root", Enabled: true, Status: model.ApplicationStatus_InvalidOutputsRoot,
				Reason: new("epoch 0: root rule failed"), ForecloseBlock: 0x12, LastOutputCheckBlock: 0x12,
			}
			output := &model.Output{
				Index: outputExecution0.OutputIndex, RawData: bytes.Clone(outputExecution0.Output),
			}
			if tc.mismatched {
				output.RawData = common.Hex2Bytes("FFBBCCDDEE")
			}
			contract.On("GetNumberOfExecutedOutputs", blockFrom(0x13)).Return(big.NewInt(1), nil)
			contract.On("RetrieveOutputExecutionEvents",
				mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x13 })).
				Return([]*iapplication.IApplicationOutputExecuted{outputExecution0}, nil).Once()
			repo.On("GetNumberOfExecutedOutputs", mock.Anything, app.IApplicationAddress.String()).Return(uint64(0), nil).Once()
			repo.On("GetOutput", mock.Anything, app.IApplicationAddress.Hex(), output.Index).Return(output, nil).Once()
			repo.On("UpdateOutputsExecution", mock.Anything, app.IApplicationAddress.Hex(),
				mock.MatchedBy(func(outputs []*model.Output) bool {
					if tc.mismatched {
						return len(outputs) == 0
					}
					return len(outputs) == 1 && outputs[0] == output &&
						outputs[0].ExecutionTransactionHash != nil && *outputs[0].ExecutionTransactionHash == outputExecution0.Raw.TxHash
				}), uint64(0x13)).Return(nil).Once()

			ok := reader.checkForOutputExecution(t.Context(), []appContracts{
				{application: app, applicationContract: contract},
			}, 0x13)

			require.True(t, ok, "terminal mismatch is handled under the existing event cursor policy")
			require.Equal(t, model.ApplicationStatus_InvalidOutputsRoot, app.Status)
			require.Equal(t, "epoch 0: root rule failed", *app.Reason)
			if tc.mismatched {
				require.Nil(t, output.ExecutionTransactionHash)
				require.Contains(t, logs.String(), "Output mismatch; preserving terminal application status")
				require.Contains(t, logs.String(), "application=invalid-root")
				require.Contains(t, logs.String(), "index=0")
				require.Contains(t, logs.String(), "status=INVALID_OUTPUTS_ROOT")
				require.NotContains(t, logs.String(), "unknown status")
			} else {
				require.Equal(t, outputExecution0.Raw.TxHash, *output.ExecutionTransactionHash)
			}
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
			contract.AssertExpectations(t)
		})
	}
}
