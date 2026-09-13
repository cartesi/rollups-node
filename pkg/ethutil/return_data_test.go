// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestApplicationReturnDataSuffix(t *testing.T) {
	for _, contract := range []struct {
		name     string
		metadata *bind.MetaData
	}{
		{"IConsensus", iconsensus.IConsensusMetaData},
		{"IDaveConsensus", idaveconsensus.IDaveConsensusMetaData},
	} {
		for _, name := range []string{"ApplicationReverted", "IllformedApplicationReturnData"} {
			t.Run(contract.name+"/"+name, func(t *testing.T) {
				parsed, err := contract.metadata.GetAbi()
				require.NoError(t, err)
				abiError, ok := parsed.Errors[name]
				require.True(t, ok)
				for _, data := range [][]byte{nil, {0, 0xff, '\n', '\r', 0x1b, '[', '2', 'J'}} {
					packed, err := abiError.Inputs.Pack(common.HexToAddress("0x1234"), data)
					require.NoError(t, err)
					payload := append(append([]byte{}, abiError.ID[:revertSelectorLength]...), packed...)
					rpcErr := &rpcDataError{code: 3, msg: executionRevertedMessage, data: payload}
					for _, wrapped := range []error{rpcErr, fmt.Errorf("estimate: %w", rpcErr)} {
						require.Equal(t, fmt.Sprintf(" Application return data: 0x%x.", data),
							ApplicationReturnDataSuffix(wrapped, contract.metadata, name))
					}
				}
				truncated := &rpcDataError{data: abiError.ID[:revertSelectorLength]}
				require.Empty(t, ApplicationReturnDataSuffix(truncated, contract.metadata, name))
				require.Empty(t, ApplicationReturnDataSuffix(nil, contract.metadata, name))
				require.Empty(t, ApplicationReturnDataSuffix(errors.New("unavailable"), contract.metadata, name))
			})
		}
	}
}

func TestApplicationReturnDataSuffixRejectsUnrelatedShapes(t *testing.T) {
	// These errors do not have the (address, bytes) return-data shape.
	for _, test := range []struct {
		name string
		args []any
	}{
		{"AppReverted", []any{[]byte{1}}},
		{"InsufficientFunds", []any{common.Big1, common.Big2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := describeTestRevert(t, test.name, test.args...)
			require.Empty(t, ApplicationReturnDataSuffix(err, describeTestMetaData, test.name))
			require.Empty(t, ApplicationReturnDataSuffix(err, nil, test.name))
			require.Empty(t, ApplicationReturnDataSuffix(err, &bind.MetaData{ABI: "invalid"}, test.name))
			require.Empty(t, ApplicationReturnDataSuffix(err, describeTestMetaData, "Unknown"))
			require.Empty(t, ApplicationReturnDataSuffix(err, describeTestMetaData, "Closed"))
		})
	}
}
