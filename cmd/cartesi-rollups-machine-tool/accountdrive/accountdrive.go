// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package accountdrive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	Log2AccountSize       = 5
	AccountSize           = 1 << Log2AccountSize
	DefaultLog2MaxAccount = 17
)

var (
	ErrUnsupportedLayout = errors.New("unsupported accounts-drive layout")
	ErrAccountNotFound   = errors.New("account not found")
)

type Proof struct {
	Account      [AccountSize]byte
	AccountIndex uint64
	AccountRoot  common.Hash
	DriveRoot    common.Hash
	Siblings     []common.Hash
}

func DriveSize(log2MaxNumOfAccounts uint8, log2LeavesPerAccount uint8) (uint64, error) {
	if log2LeavesPerAccount != 0 {
		return 0, fmt.Errorf("%w: log2_leaves_per_account=%d", ErrUnsupportedLayout, log2LeavesPerAccount)
	}
	if log2MaxNumOfAccounts >= 58 { // 2^(5+58) still fits in uint64; keep a margin for int conversion.
		return 0, fmt.Errorf("log2_max_num_of_accounts %d is too large", log2MaxNumOfAccounts)
	}
	return 1 << (Log2AccountSize + log2MaxNumOfAccounts), nil
}

func Encode(address common.Address, balance uint64) ([AccountSize]byte, error) {
	var account [AccountSize]byte
	if address == (common.Address{}) {
		return account, errors.New("account address must not be zero")
	}
	if balance == 0 {
		return account, errors.New("account balance must be positive")
	}
	if balance > math.MaxInt64 {
		return account, fmt.Errorf("account balance %d exceeds int64 accounts-drive limit", balance)
	}
	binary.LittleEndian.PutUint64(account[:8], balance)
	copy(account[8:28], address.Bytes())
	return account, nil
}

func Decode(account []byte) (common.Address, uint64, bool, error) {
	var zero [AccountSize]byte
	if len(account) != AccountSize {
		return common.Address{}, 0, false, fmt.Errorf("account record must be %d bytes, got %d", AccountSize, len(account))
	}
	if bytes.Equal(account, zero[:]) {
		return common.Address{}, 0, false, nil
	}
	balance := binary.LittleEndian.Uint64(account[:8])
	if balance == 0 {
		return common.Address{}, 0, false, errors.New("non-empty account has zero balance")
	}
	if balance > math.MaxInt64 {
		return common.Address{}, 0, false, fmt.Errorf("account balance %d exceeds int64 accounts-drive limit", balance)
	}
	address := common.BytesToAddress(account[8:28])
	if address == (common.Address{}) {
		return common.Address{}, 0, false, errors.New("non-empty account has zero address")
	}
	if !bytes.Equal(account[28:32], []byte{0, 0, 0, 0}) {
		return common.Address{}, 0, false, errors.New("non-empty account has non-zero padding")
	}
	return address, balance, true, nil
}

func BuildProof(
	drive []byte,
	address common.Address,
	log2MaxNumOfAccounts uint8,
	log2LeavesPerAccount uint8,
) (*Proof, error) {
	if address == (common.Address{}) {
		return nil, errors.New("account address must not be zero")
	}
	driveSize, err := DriveSize(log2MaxNumOfAccounts, log2LeavesPerAccount)
	if err != nil {
		return nil, err
	}
	if uint64(len(drive)) > driveSize {
		return nil, fmt.Errorf("accounts drive is too large: got %d bytes, want at most %d", len(drive), driveSize)
	}

	leafCount := 1 << log2MaxNumOfAccounts
	leaves := make([]common.Hash, leafCount)
	foundIndex := uint64(math.MaxUint64)
	var foundAccount [AccountSize]byte
	seenEnd := false

	for i := range leaves {
		var account [AccountSize]byte
		offset := i * AccountSize
		if offset < len(drive) {
			copy(account[:], drive[offset:min(offset+AccountSize, len(drive))])
		}

		decodedAddress, _, nonEmpty, err := Decode(account[:])
		if err != nil {
			return nil, fmt.Errorf("decode account %d: %w", i, err)
		}
		if nonEmpty {
			if seenEnd {
				return nil, fmt.Errorf("account %d appears after the first empty slot", i)
			}
			if decodedAddress == address {
				foundIndex = uint64(i)
				foundAccount = account
			}
		} else {
			seenEnd = true
		}
		leaves[i] = crypto.Keccak256Hash(account[:])
	}
	if foundIndex == math.MaxUint64 {
		return nil, fmt.Errorf("%w: %s", ErrAccountNotFound, address)
	}

	accountRoot := leaves[foundIndex]
	siblings := make([]common.Hash, log2MaxNumOfAccounts)
	nodeIndex := foundIndex
	level := leaves
	for height := range int(log2MaxNumOfAccounts) {
		siblings[height] = level[nodeIndex^1]
		parents := make([]common.Hash, len(level)/2)
		for i := 0; i < len(parents); i++ {
			parents[i] = crypto.Keccak256Hash(level[2*i].Bytes(), level[2*i+1].Bytes())
		}
		nodeIndex >>= 1
		level = parents
	}

	return &Proof{
		Account:      foundAccount,
		AccountIndex: foundIndex,
		AccountRoot:  accountRoot,
		DriveRoot:    level[0],
		Siblings:     siblings,
	}, nil
}

func RootFromProof(accountRoot common.Hash, accountIndex uint64, siblings []common.Hash) common.Hash {
	root := accountRoot
	for height, sibling := range siblings {
		if (accountIndex>>height)&1 == 0 {
			root = crypto.Keccak256Hash(root.Bytes(), sibling.Bytes())
		} else {
			root = crypto.Keccak256Hash(sibling.Bytes(), root.Bytes())
		}
	}
	return root
}
