// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
)

// Minimal BIP-32 private key derivation over secp256k1, on top of go-ethereum's
// curve. The algorithm is one HMAC-SHA512 and one modular addition, and the node
// does not need an external module for it.
// The BIP-39 step continues to use go-bip39, because that standard needs the
// normative word lists.
// See https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki

// hardenedKeyStart is the first child index that requires the parent private key.
const hardenedKeyStart uint32 = 0x80000000

// scalarSize is the length in bytes of a serialized secp256k1 scalar.
const scalarSize = 32

// masterKeySalt is the HMAC key BIP-32 mandates for master key generation.
var masterKeySalt = []byte("Bitcoin seed")

// extendedKey is a BIP-32 extended private key: a secp256k1 scalar paired with
// the chain code that seeds the derivation of its children.
type extendedKey struct {
	key       []byte // scalarSize bytes, big endian
	chainCode []byte
}

// newMasterKey derives the master extended private key from a BIP-39 seed.
func newMasterKey(seed []byte) (*extendedKey, error) {
	mac := hmac.New(sha512.New, masterKeySalt)
	mac.Write(seed)
	sum := mac.Sum(nil)

	key := new(big.Int).SetBytes(sum[:scalarSize])
	if key.Sign() == 0 || key.Cmp(crypto.S256().Params().N) >= 0 {
		return nil, fmt.Errorf("seed produces an invalid master key")
	}
	return &extendedKey{key: sum[:scalarSize], chainCode: sum[scalarSize:]}, nil
}

// childKey derives the child of k at index, following BIP-32 CKDpriv.
// Indexes at or above hardenedKeyStart derive hardened children.
func (k *extendedKey) childKey(index uint32) (*extendedKey, error) {
	mac := hmac.New(sha512.New, k.chainCode)
	if index >= hardenedKeyStart {
		mac.Write([]byte{0})
		mac.Write(k.key)
	} else {
		parent, err := crypto.ToECDSA(k.key)
		if err != nil {
			return nil, fmt.Errorf("invalid parent key: %w", err)
		}
		mac.Write(crypto.CompressPubkey(&parent.PublicKey))
	}
	if err := binary.Write(mac, binary.BigEndian, index); err != nil {
		return nil, fmt.Errorf("failed to hash child index: %w", err)
	}
	sum := mac.Sum(nil)

	// The child key is (parse256(IL) + parent) mod n. BIP-32 requires the
	// caller to move on to the next index in the astronomically unlikely case
	// that IL is out of range or the sum is zero; report it instead, so that a
	// wrong result can never be mistaken for a valid key.
	order := crypto.S256().Params().N
	child := new(big.Int).SetBytes(sum[:scalarSize])
	if child.Cmp(order) >= 0 {
		return nil, fmt.Errorf("child %v is out of the curve order", index)
	}
	child.Add(child, new(big.Int).SetBytes(k.key))
	child.Mod(child, order)
	if child.Sign() == 0 {
		return nil, fmt.Errorf("child %v derives the zero key", index)
	}

	return &extendedKey{
		key:       child.FillBytes(make([]byte, scalarSize)),
		chainCode: sum[scalarSize:],
	}, nil
}
