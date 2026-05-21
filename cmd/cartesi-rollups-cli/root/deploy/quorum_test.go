// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestParseValidatorAddresses_ValidRepeatedFlags(t *testing.T) {
	got, err := parseValidatorAddresses([]string{
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
	})

	require.NoError(t, err)
	require.Equal(t, []common.Address{
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
	}, got)
}

func TestParseValidatorAddresses_RequiresAtLeastOneValidator(t *testing.T) {
	_, err := parseValidatorAddresses(nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one")
}

func TestParseValidatorAddresses_RejectsInvalidAddress(t *testing.T) {
	_, err := parseValidatorAddresses([]string{"not-an-address"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse")
}

func TestParseValidatorAddresses_RejectsZeroAddress(t *testing.T) {
	_, err := parseValidatorAddresses([]string{"0x0000000000000000000000000000000000000000"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "zero address")
}

func TestParseValidatorAddresses_RejectsDuplicates(t *testing.T) {
	_, err := parseValidatorAddresses([]string{
		"0x1111111111111111111111111111111111111111",
		"0x1111111111111111111111111111111111111111",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
}
