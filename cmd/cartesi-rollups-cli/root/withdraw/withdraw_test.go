// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package withdraw

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// loadProof tests pin the JSON proof-file parser used by `withdraw`. The
// parser is the last sanity gate before a fund-moving tx is constructed —
// a malformed account or proof must abort with a clear error, never
// silently produce a self-consistent garbage proof.

const validWithdrawProofJSON = `{
  "account": "0xaabbccdd",
  "account_index": "0x7",
  "account_root_siblings": [
    "0x0000000000000000000000000000000000000000000000000000000000000001",
    "0x0000000000000000000000000000000000000000000000000000000000000002"
  ]
}`

func writeProofFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "proof.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoadProof_Valid(t *testing.T) {
	path := writeProofFile(t, validWithdrawProofJSON)
	account, proof, err := loadProof(path)
	require.NoError(t, err)
	require.Equal(t, []byte{0xaa, 0xbb, 0xcc, 0xdd}, account)
	require.Equal(t, uint64(7), proof.AccountIndex)
	require.Len(t, proof.AccountRootSiblings, 2)
	require.Equal(t, byte(0x01), proof.AccountRootSiblings[0][31])
	require.Equal(t, byte(0x02), proof.AccountRootSiblings[1][31])
}

func TestLoadProof_FileNotFound(t *testing.T) {
	_, _, err := loadProof("/nonexistent/path/proof.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read proof file")
}

func TestLoadProof_InvalidJSON(t *testing.T) {
	path := writeProofFile(t, `{not valid json`)
	_, _, err := loadProof(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse proof file")
}

func TestLoadProof_UnknownField(t *testing.T) {
	body := `{
	  "account": "0x00",
	  "account_index": "0x0",
	  "account_root_siblings": [],
	  "extra_field": "this should not be accepted"
	}`
	path := writeProofFile(t, body)
	_, _, err := loadProof(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse proof file")
}

func TestLoadProof_BadAccountHex(t *testing.T) {
	body := `{
	  "account": "not-hex",
	  "account_index": "0x0",
	  "account_root_siblings": []
	}`
	path := writeProofFile(t, body)
	_, _, err := loadProof(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid account")
}

func TestLoadProof_BadAccountIndex(t *testing.T) {
	body := `{
	  "account": "0xaa",
	  "account_index": "not-hex",
	  "account_root_siblings": []
	}`
	path := writeProofFile(t, body)
	_, _, err := loadProof(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid account_index")
}

func TestLoadProof_SiblingWrongLength(t *testing.T) {
	cases := []struct {
		name string
		hex  string
	}{
		{"31_bytes", "0x" + repeatHex("aa", 31)},
		{"33_bytes", "0x" + repeatHex("aa", 33)},
		{"empty", "0x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{
			  "account": "0x00",
			  "account_index": "0x0",
			  "account_root_siblings": ["` + tc.hex + `"]
			}`
			path := writeProofFile(t, body)
			_, _, err := loadProof(path)
			require.Error(t, err)
			require.Contains(t, err.Error(), "account_root_siblings")
			require.Contains(t, err.Error(), "32 bytes")
		})
	}
}

func TestLoadProof_BadSiblingHex(t *testing.T) {
	body := `{
	  "account": "0x00",
	  "account_index": "0x0",
	  "account_root_siblings": ["not-hex"]
	}`
	path := writeProofFile(t, body)
	_, _, err := loadProof(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "account_root_siblings[0]")
}

func repeatHex(b string, n int) string {
	out := make([]byte, 0, n*len(b))
	for range n {
		out = append(out, b...)
	}
	return string(out)
}
