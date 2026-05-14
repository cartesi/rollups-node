// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package provedriveroot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// loadProof tests pin the JSON proof-file parser used by `prove-drive-root`.
// A malformed root or sibling must abort with a clear error before the tx
// is constructed; the on-chain `proveAccountsDriveMerkleRoot` reverts with
// less context.

const validDriveRootProofJSON = `{
  "accounts_drive_merkle_root": "0x0000000000000000000000000000000000000000000000000000000000000042",
  "proof": [
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
	path := writeProofFile(t, validDriveRootProofJSON)
	root, proof, err := loadProof(path)
	require.NoError(t, err)
	require.Equal(t, byte(0x42), root[31])
	require.Len(t, proof, 2)
	require.Equal(t, byte(0x01), proof[0][31])
	require.Equal(t, byte(0x02), proof[1][31])
}

func TestLoadProof_EmptyProofArray(t *testing.T) {
	body := `{
	  "accounts_drive_merkle_root": "0x0000000000000000000000000000000000000000000000000000000000000042",
	  "proof": []
	}`
	path := writeProofFile(t, body)
	_, proof, err := loadProof(path)
	require.NoError(t, err)
	require.Len(t, proof, 0,
		"empty proof array is structurally valid here; the contract validates depth")
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
	  "accounts_drive_merkle_root": "0x0000000000000000000000000000000000000000000000000000000000000042",
	  "proof": [],
	  "extra_field": "rejected"
	}`
	path := writeProofFile(t, body)
	_, _, err := loadProof(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse proof file")
}

func TestLoadProof_RootWrongLength(t *testing.T) {
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
			  "accounts_drive_merkle_root": "` + tc.hex + `",
			  "proof": []
			}`
			path := writeProofFile(t, body)
			_, _, err := loadProof(path)
			require.Error(t, err)
			require.Contains(t, err.Error(), "accounts_drive_merkle_root")
			require.Contains(t, err.Error(), "32 bytes")
		})
	}
}

func TestLoadProof_BadRootHex(t *testing.T) {
	body := `{
	  "accounts_drive_merkle_root": "not-hex",
	  "proof": []
	}`
	path := writeProofFile(t, body)
	_, _, err := loadProof(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid accounts_drive_merkle_root")
}

func TestLoadProof_SiblingWrongLength(t *testing.T) {
	body := `{
	  "accounts_drive_merkle_root": "0x0000000000000000000000000000000000000000000000000000000000000042",
	  "proof": ["0x` + repeatHex("aa", 31) + `"]
	}`
	path := writeProofFile(t, body)
	_, _, err := loadProof(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "proof[0]")
	require.Contains(t, err.Error(), "32 bytes")
}

func TestLoadProof_BadSiblingHex(t *testing.T) {
	body := `{
	  "accounts_drive_merkle_root": "0x0000000000000000000000000000000000000000000000000000000000000042",
	  "proof": ["not-hex"]
	}`
	path := writeProofFile(t, body)
	_, _, err := loadProof(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid proof[0]")
}

func repeatHex(b string, n int) string {
	out := make([]byte, 0, n*len(b))
	for range n {
		out = append(out, b...)
	}
	return string(out)
}
