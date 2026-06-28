// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

func PrivateKeyToAddress(privateKey *ecdsa.PrivateKey) (common.Address, error) {
	if privateKey == nil {
		return common.Address{}, fmt.Errorf("private key is nil")
	}
	publicKey := privateKey.Public()
	publicKeyECDSA := publicKey.(*ecdsa.PublicKey)
	return crypto.PubkeyToAddress(*publicKeyECDSA), nil
}

// Create the private key from mnemonic and account index based on the BIP44 standard.
// For more info on BIP44, see https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki
func MnemonicToPrivateKey(mnemonic string, accountIndex uint32) (*ecdsa.PrivateKey, error) {
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, "")
	if err != nil {
		return nil, err
	}

	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to generate master key: %w", err)
	}

	// get key at path m/44'/60'/0'/0/account
	const hardenedKeyStart uint32 = 0x80000000
	levels := []uint32{
		hardenedKeyStart + 44,
		hardenedKeyStart + 60,
		hardenedKeyStart + 0,
		0,
		accountIndex,
	}
	key := masterKey
	for i, level := range levels {
		key, err = key.NewChildKey(level)
		if err != nil {
			return nil, fmt.Errorf("failed to get child %v: %w", i, err)
		}
	}

	return crypto.ToECDSA(key.Key)
}
