// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

// The published BIP-32 test vectors. They show that this derivation agrees with
// the standard. See https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki
const (
	vector1Seed = "000102030405060708090a0b0c0d0e0f"
	vector2Seed = "fffcf9f6f3f0edeae7e4e1dedbd8d5d2cfccc9c6c3c0bdbab7b4b1aeaba8a5a2" +
		"9f9c999693908d8a8784817e7b7875726f6c696663605d5a5754514e4b484542"
	vector3Seed = "4b381541583be4423346c643850da4b320e46a87ae3d2a4e6da11eba819cd4ac" +
		"ba45d239319ac14f863b8d5ab5a0d0c64d2e8a1e7d1457df2e5a3c51c73235be"
	vector4Seed = "3ddd5602285899a946114506157c7997e5444528f3003f6134712147db19b678"
)

type bip32Vector struct {
	name      string
	seed      string
	path      []uint32
	key       string
	chainCode string
}

// hardened returns the child index that derives a hardened child at index.
func hardened(index uint32) uint32 { return hardenedKeyStart + index }

var bip32Vectors = []bip32Vector{{
	name:      "vector 1, m",
	seed:      vector1Seed,
	key:       "e8f32e723decf4051aefac8e2c93c9c5b214313817cdb01a1494b917c8436b35",
	chainCode: "873dff81c02f525623fd1fe5167eac3a55a049de3d314bb42ee227ffed37d508",
}, {
	name:      "vector 1, m/0'",
	seed:      vector1Seed,
	path:      []uint32{hardenedKeyStart},
	key:       "edb2e14f9ee77d26dd93b4ecede8d16ed408ce149b6cd80b0715a2d911a0afea",
	chainCode: "47fdacbd0f1097043b78c63c20c34ef4ed9a111d980047ad16282c7ae6236141",
}, {
	name:      "vector 1, m/0'/1",
	seed:      vector1Seed,
	path:      []uint32{hardenedKeyStart, 1},
	key:       "3c6cb8d0f6a264c91ea8b5030fadaa8e538b020f0a387421a12de9319dc93368",
	chainCode: "2a7857631386ba23dacac34180dd1983734e444fdbf774041578e9b6adb37c19",
}, {
	name:      "vector 1, m/0'/1/2'",
	seed:      vector1Seed,
	path:      []uint32{hardenedKeyStart, 1, hardened(2)},
	key:       "cbce0d719ecf7431d88e6a89fa1483e02e35092af60c042b1df2ff59fa424dca",
	chainCode: "04466b9cc8e161e966409ca52986c584f07e9dc81f735db683c3ff6ec7b1503f",
}, {
	name:      "vector 1, m/0'/1/2'/2",
	seed:      vector1Seed,
	path:      []uint32{hardenedKeyStart, 1, hardened(2), 2},
	key:       "0f479245fb19a38a1954c5c7c0ebab2f9bdfd96a17563ef28a6a4b1a2a764ef4",
	chainCode: "cfb71883f01676f587d023cc53a35bc7f88f724b1f8c2892ac1275ac822a3edd",
}, {
	name:      "vector 1, m/0'/1/2'/2/1000000000",
	seed:      vector1Seed,
	path:      []uint32{hardenedKeyStart, 1, hardened(2), 2, 1000000000},
	key:       "471b76e389e528d6de6d816857e012c5455051cad6660850e58372a6c3e6e7c8",
	chainCode: "c783e67b921d2beb8f6b389cc646d7263b4145701dadd2161548a8b078e65e9e",
}, {
	// Vector 2 uses a 512-bit seed and the largest child indexes.
	name:      "vector 2, m",
	seed:      vector2Seed,
	key:       "4b03d6fc340455b363f51020ad3ecca4f0850280cf436c70c727923f6db46c3e",
	chainCode: "60499f801b896d83179a4374aeb7822aaeaceaa0db1f85ee3e904c4defbd9689",
}, {
	name:      "vector 2, m/0",
	seed:      vector2Seed,
	path:      []uint32{0},
	key:       "abe74a98f6c7eabee0428f53798f0ab8aa1bd37873999041703c742f15ac7e1e",
	chainCode: "f0909affaa7ee7abe5dd4e100598d4dc53cd709d5a5c2cac40e7412f232f7c9c",
}, {
	// The child index here is 0xffffffff, the largest possible value.
	name:      "vector 2, m/0/2147483647'",
	seed:      vector2Seed,
	path:      []uint32{0, hardened(2147483647)},
	key:       "877c779ad9687164e9c2f4f0f4ff0340814392330693ce95a58fe18fd52e6e93",
	chainCode: "be17a268474a6bb9c61e1d720cf6215e2a88c5406c4aee7b38547f585c9a37d9",
}, {
	name:      "vector 2, m/0/2147483647'/1",
	seed:      vector2Seed,
	path:      []uint32{0, hardened(2147483647), 1},
	key:       "704addf544a06e5ee4bea37098463c23613da32020d604506da8c0518e1da4b7",
	chainCode: "f366f48f1ea9f2d1d3fe958c95ca84ea18e4c4ddb9366c336c927eb246fb38cb",
}, {
	name:      "vector 2, m/0/2147483647'/1/2147483646'",
	seed:      vector2Seed,
	path:      []uint32{0, hardened(2147483647), 1, hardened(2147483646)},
	key:       "f1c7c871a54a804afe328b4c83a1c33b8e5ff48f5087273f04efa83b247d6a2d",
	chainCode: "637807030d55d01f9a0cb3a7839515d796bd07706386a6eddf06cc29a65a0e29",
}, {
	name:      "vector 2, m/0/2147483647'/1/2147483646'/2",
	seed:      vector2Seed,
	path:      []uint32{0, hardened(2147483647), 1, hardened(2147483646), 2},
	key:       "bb7d39bdb83ecf58f2fd82b6d918341cbef428661ef01ab97c28a4842125ac23",
	chainCode: "9452b549be8cea3ecb7a84bec10dcfd94afe4d129ebfd3b3cb58eedf394ed271",
}, {
	// Vector 3 shows that the master key keeps its leading zero.
	name:      "vector 3, m",
	seed:      vector3Seed,
	key:       "00ddb80b067e0d4993197fe10f2657a844a384589847602d56f0c629c81aae32",
	chainCode: "01d28a3e53cffa419ec122c968b3259e16b65076495494d97cae10bbfec3c36f",
}, {
	name:      "vector 3, m/0'",
	seed:      vector3Seed,
	path:      []uint32{hardenedKeyStart},
	key:       "491f7a2eebc7b57028e0d3faa0acda02e75c33b03c48fb288c41e2ea44e1daef",
	chainCode: "e5fea12a97b927fc9dc3d2cb0d1ea1cf50aa5a1fdc1f933e8906bb38df3377bd",
}, {
	// Vector 4 shows that a derived key also keeps its leading zero.
	name:      "vector 4, m",
	seed:      vector4Seed,
	key:       "12c0d59c7aa3a10973dbd3f478b65f2516627e3fe61e00c345be9a477ad2e215",
	chainCode: "d0c8a1f6edf2500798c3e0b54f1b56e45f6d03e6076abd36e5e2f54101e44ce6",
}, {
	name:      "vector 4, m/0'",
	seed:      vector4Seed,
	path:      []uint32{hardenedKeyStart},
	key:       "00d948e9261e41362a688b916f297121ba6bfb2274a3575ac0e456551dfd7f7e",
	chainCode: "cdc0f06456a14876c898790e0b3b1a41c531170aec69da44ff7b7265bfe7743b",
}, {
	name:      "vector 4, m/0'/1'",
	seed:      vector4Seed,
	path:      []uint32{hardenedKeyStart, hardened(1)},
	key:       "3a2086edd7d9df86c3487a5905a1712a9aa664bce8cc268141e07549eaa8661d",
	chainCode: "a48ee6674c5264a237703fd383bccd9fad4d9378ac98ab05e6e7029b06360c0d",
}}

// derive walks path from the master key of seed.
func derive(t *testing.T, seed string, path []uint32) *extendedKey {
	t.Helper()
	raw, err := hex.DecodeString(seed)
	require.Nil(t, err)

	key, err := newMasterKey(raw)
	require.Nil(t, err)
	for _, index := range path {
		key, err = key.childKey(index)
		require.Nil(t, err)
	}
	return key
}

func TestBIP32Vectors(t *testing.T) {
	for _, v := range bip32Vectors {
		t.Run(v.name, func(t *testing.T) {
			key := derive(t, v.seed, v.path)
			require.Equal(t, v.key, hex.EncodeToString(key.key))
			require.Equal(t, v.chainCode, hex.EncodeToString(key.chainCode))
			// A short key would still print the same hex, so check the length.
			require.Len(t, key.key, scalarSize)
			require.Len(t, key.chainCode, scalarSize)
		})
	}
}

// TestHardenedChildDiffers shows that the two branches of childKey do not
// derive the same child. Index n and index n' must give different keys.
func TestHardenedChildDiffers(t *testing.T) {
	master := derive(t, vector1Seed, nil)

	normal, err := master.childKey(1)
	require.Nil(t, err)
	hard, err := master.childKey(hardened(1))
	require.Nil(t, err)

	require.NotEqual(t, normal.key, hard.key)
	require.NotEqual(t, normal.chainCode, hard.chainCode)
}

// TestChildKeyRejectsInvalidParent shows that childKey does not derive a child
// from a parent key that is not on the curve.
func TestChildKeyRejectsInvalidParent(t *testing.T) {
	invalid := &extendedKey{
		key:       make([]byte, scalarSize),
		chainCode: make([]byte, scalarSize),
	}
	_, err := invalid.childKey(0)
	require.Error(t, err)
}
