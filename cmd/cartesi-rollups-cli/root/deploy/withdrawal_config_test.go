// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const validInlineJSON = `{
  "guardian":                   "0x1111111111111111111111111111111111111111",
  "log2_leaves_per_account":    0,
  "log2_max_num_of_accounts":   20,
  "accounts_drive_start_index": 33554432,
  "withdrawal_output_builder":  "0x2222222222222222222222222222222222222222"
}`

func TestParseWithdrawalConfig_BothEmpty(t *testing.T) {
	wc, err := parseWithdrawalConfig("", "")
	require.NoError(t, err)
	require.Equal(t, common.Address{}, wc.Guardian)
	require.Equal(t, common.Address{}, wc.WithdrawalOutputBuilder)
}

func TestParseWithdrawalConfig_BothSet(t *testing.T) {
	_, err := parseWithdrawalConfig(validInlineJSON, "some/file.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "mutually exclusive")
}

func TestParseWithdrawalConfig_ValidInline(t *testing.T) {
	wc, err := parseWithdrawalConfig(validInlineJSON, "")
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress("0x1111111111111111111111111111111111111111"), wc.Guardian)
	require.Equal(t, common.HexToAddress("0x2222222222222222222222222222222222222222"), wc.WithdrawalOutputBuilder)
	require.Equal(t, uint8(0), wc.Log2LeavesPerAccount)
	require.Equal(t, uint8(20), wc.Log2MaxNumOfAccounts)
	require.Equal(t, uint64(33554432), wc.AccountsDriveStartIndex)
}

func TestParseWithdrawalConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wc.json")
	require.NoError(t, os.WriteFile(path, []byte(validInlineJSON), 0o600))

	wc, err := parseWithdrawalConfig("", path)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress("0x1111111111111111111111111111111111111111"), wc.Guardian)
}

func TestParseWithdrawalConfig_FileNotFound(t *testing.T) {
	_, err := parseWithdrawalConfig("", "/nonexistent/path/wc.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read")
}

func TestParseWithdrawalConfig_BadJSON(t *testing.T) {
	_, err := parseWithdrawalConfig("not json", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid JSON")
}

func TestParseWithdrawalConfig_UnknownField(t *testing.T) {
	bad := `{
  "guardian":                   "0x1111111111111111111111111111111111111111",
  "log2_leaves_per_account":    0,
  "log2_max_num_of_accounts":   20,
  "accounts_drive_start_index": 33554432,
  "withdrawal_output_builder":  "0x2222222222222222222222222222222222222222",
  "gardian":                    "0x0"
}`
	_, err := parseWithdrawalConfig(bad, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid JSON")
}

func TestParseWithdrawalConfig_MissingKey(t *testing.T) {
	bad := `{
  "log2_leaves_per_account":    0,
  "log2_max_num_of_accounts":   20,
  "accounts_drive_start_index": 33554432,
  "withdrawal_output_builder":  "0x2222222222222222222222222222222222222222"
}`
	_, err := parseWithdrawalConfig(bad, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing required keys")
	require.Contains(t, err.Error(), "guardian")
}

func TestParseWithdrawalConfig_BadGuardianAddress(t *testing.T) {
	bad := `{
  "guardian":                   "not-an-address",
  "log2_leaves_per_account":    0,
  "log2_max_num_of_accounts":   20,
  "accounts_drive_start_index": 33554432,
  "withdrawal_output_builder":  "0x2222222222222222222222222222222222222222"
}`
	_, err := parseWithdrawalConfig(bad, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid guardian address")
}

func TestParseWithdrawalConfig_FailsIsValid(t *testing.T) {
	// log2_max + log2_leaves = 60 + 60 -> drive > 64
	bad := `{
  "guardian":                   "0x1111111111111111111111111111111111111111",
  "log2_leaves_per_account":    60,
  "log2_max_num_of_accounts":   60,
  "accounts_drive_start_index": 0,
  "withdrawal_output_builder":  "0x2222222222222222222222222222222222222222"
}`
	_, err := parseWithdrawalConfig(bad, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "larger than machine memory")
}

func TestParseWithdrawalConfig_ZeroGuardianRejected(t *testing.T) {
	bad := `{
  "guardian":                   "0x0000000000000000000000000000000000000000",
  "log2_leaves_per_account":    0,
  "log2_max_num_of_accounts":   20,
  "accounts_drive_start_index": 33554432,
  "withdrawal_output_builder":  "0x2222222222222222222222222222222222222222"
}`
	_, err := parseWithdrawalConfig(bad, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "guardian address must not be the zero address")
}

func TestParseWithdrawalConfig_ZeroBuilderRejected(t *testing.T) {
	bad := `{
  "guardian":                   "0x1111111111111111111111111111111111111111",
  "log2_leaves_per_account":    0,
  "log2_max_num_of_accounts":   20,
  "accounts_drive_start_index": 33554432,
  "withdrawal_output_builder":  "0x0000000000000000000000000000000000000000"
}`
	_, err := parseWithdrawalConfig(bad, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "withdrawal_output_builder address must not be the zero address")
}
