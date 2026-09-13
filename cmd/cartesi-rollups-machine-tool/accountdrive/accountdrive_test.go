// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package accountdrive

import (
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestEncodeMatchesLibUsdAccountLayout(t *testing.T) {
	address := common.HexToAddress("0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266")
	balance, ok := new(big.Int).SetString("0123456789abcdef01234567", 16)
	require.True(t, ok)

	account, err := Encode(address, balance)
	require.NoError(t, err)

	require.Equal(t,
		"67452301efcdab8967452301f39fd6e51aad88f6f4ce6ab8827279cfffb92266",
		hex.EncodeToString(account[:]),
	)

	decodedAddress, decodedBalance, nonEmpty, err := Decode(account[:])
	require.NoError(t, err)
	require.True(t, nonEmpty)
	require.Equal(t, address, decodedAddress)
	require.Zero(t, balance.Cmp(decodedBalance))
}

func TestEncodeSupportsUint96(t *testing.T) {
	maxUint96 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 96), big.NewInt(1))
	address := common.HexToAddress("0x1111111111111111111111111111111111111111")

	account, err := Encode(address, maxUint96)
	require.NoError(t, err)
	require.Equal(t, "ffffffffffffffffffffffff1111111111111111111111111111111111111111",
		hex.EncodeToString(account[:]))

	decodedAddress, decodedBalance, nonEmpty, err := Decode(account[:])
	require.NoError(t, err)
	require.True(t, nonEmpty)
	require.Equal(t, address, decodedAddress)
	require.Zero(t, maxUint96.Cmp(decodedBalance))
}

func TestEncodeRejectsInvalidBalances(t *testing.T) {
	address := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tooLarge := new(big.Int).Lsh(big.NewInt(1), 96)
	for _, balance := range []*big.Int{nil, big.NewInt(0), big.NewInt(-1), tooLarge} {
		_, err := Encode(address, balance)
		require.Error(t, err)
	}
}

func TestDecodeRejectsCorruptRecords(t *testing.T) {
	t.Run("wrong record size is invalid", func(t *testing.T) {
		for _, size := range []int{AccountSize - 1, AccountSize + 1} {
			_, _, _, err := Decode(make([]byte, size))
			require.ErrorContains(t, err, "must be 32 bytes")
		}
	})

	t.Run("zero record is empty", func(t *testing.T) {
		var zero [AccountSize]byte
		_, _, ok, err := Decode(zero[:])
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("zero balance is invalid", func(t *testing.T) {
		var account [AccountSize]byte
		copy(account[usdBalanceSize:], common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes())

		_, _, _, err := Decode(account[:])
		require.ErrorContains(t, err, "zero balance")
	})

	t.Run("zero address is invalid", func(t *testing.T) {
		var account [AccountSize]byte
		account[0] = 1

		_, _, _, err := Decode(account[:])
		require.ErrorContains(t, err, "zero address")
	})
}

func TestBuildProof_BuildsVerifiableAccountProof(t *testing.T) {
	addr1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	account1, err := Encode(addr1, big.NewInt(10))
	require.NoError(t, err)
	account2, err := Encode(addr2, big.NewInt(20))
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
	account, err := Encode(addr, big.NewInt(10))
	require.NoError(t, err)
	drive := make([]byte, 3*AccountSize)
	copy(drive[2*AccountSize:3*AccountSize], account[:])

	_, err = BuildProof(drive, addr, 2, 0)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrAccountNotFound))
	require.ErrorContains(t, err, "after the first empty slot")
}
