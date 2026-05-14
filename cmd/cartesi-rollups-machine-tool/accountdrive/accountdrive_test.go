// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package accountdrive

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestEncode_MatchesEwtoolsLayout(t *testing.T) {
	account, err := Encode(common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), 7)
	require.NoError(t, err)

	require.Equal(t,
		"0700000000000000bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb00000000",
		hex.EncodeToString(account[:]),
	)
}

func TestDecode_RejectsCorruptRecords(t *testing.T) {
	t.Run("zero record is empty", func(t *testing.T) {
		var zero [AccountSize]byte
		_, _, ok, err := Decode(zero[:])
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("non-zero padding is invalid", func(t *testing.T) {
		account, err := Encode(common.HexToAddress("0x1111111111111111111111111111111111111111"), 1)
		require.NoError(t, err)
		account[31] = 1

		_, _, _, err = Decode(account[:])
		require.ErrorContains(t, err, "padding")
	})
}

func TestBuildProof_BuildsVerifiableAccountProof(t *testing.T) {
	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	account1, err := Encode(addr1, 10)
	require.NoError(t, err)
	account2, err := Encode(addr2, 20)
	require.NoError(t, err)

	drive := make([]byte, 1<<(Log2AccountSize+DefaultLog2MaxAccount))
	copy(drive[0:AccountSize], account1[:])
	copy(drive[AccountSize:2*AccountSize], account2[:])

	proof, err := BuildProof(drive, addr2, DefaultLog2MaxAccount, 0)
	require.NoError(t, err)

	require.Equal(t, uint64(1), proof.AccountIndex)
	require.Equal(t, account2, proof.Account)
	require.Len(t, proof.Siblings, DefaultLog2MaxAccount)
	require.Equal(t, proof.DriveRoot, RootFromProof(proof.AccountRoot, proof.AccountIndex, proof.Siblings))
}

func TestBuildProof_ReturnsClearErrorForMissingAccount(t *testing.T) {
	drive := make([]byte, 1<<(Log2AccountSize+DefaultLog2MaxAccount))
	_, err := BuildProof(drive, common.HexToAddress("0x3333333333333333333333333333333333333333"), DefaultLog2MaxAccount, 0)
	require.ErrorIs(t, err, ErrAccountNotFound)
}

func TestBuildProof_RejectsUnsupportedLayout(t *testing.T) {
	_, err := BuildProof(nil, common.HexToAddress("0x1111111111111111111111111111111111111111"), DefaultLog2MaxAccount, 1)
	require.ErrorIs(t, err, ErrUnsupportedLayout)
}

func TestBuildProof_RejectsNonCompactAccountTable(t *testing.T) {
	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	account, err := Encode(addr, 10)
	require.NoError(t, err)
	drive := make([]byte, 3*AccountSize)
	copy(drive[2*AccountSize:3*AccountSize], account[:])

	_, err = BuildProof(drive, addr, 2, 0)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrAccountNotFound))
	require.ErrorContains(t, err, "after the first empty slot")
}
