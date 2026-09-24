// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

type applicationInputBoxCallerStub struct {
	inputBox common.Address
	err      error
}

func (s applicationInputBoxCallerStub) GetInputBox(*bind.CallOpts) (common.Address, error) {
	return s.inputBox, s.err
}

func TestReadApplicationInputBox(t *testing.T) {
	want := common.HexToAddress("0x1234567890123456789012345678901234567890")
	got, err := readApplicationInputBox(applicationInputBoxCallerStub{inputBox: want}, nil)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestReadApplicationInputBoxRejectsZeroAddress(t *testing.T) {
	_, err := readApplicationInputBox(applicationInputBoxCallerStub{}, nil)
	require.ErrorContains(t, err, "zero address")
}

func TestReadApplicationInputBoxReturnsCallError(t *testing.T) {
	wantErr := errors.New("call failed")
	_, err := readApplicationInputBox(applicationInputBoxCallerStub{err: wantErr}, nil)
	require.ErrorIs(t, err, wantErr)
}
