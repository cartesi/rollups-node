// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var zeroDigest = common.Hash{}

func TestTreeAndProofErrorSentinels(t *testing.T) {
	_, err := (&Tree{RootHash: zeroDigest, Height: 1}).ProveLeaf(big.NewInt(0))
	require.ErrorIs(t, err, ErrInvariant)
	_, _, err = (&Proof{}).BuildRootChildren()
	require.ErrorIs(t, err, ErrBadInput)
}

func TestBuildRootChildren1(t *testing.T) {
	p := Proof{
		Pos:  big.NewInt(1),
		Node: common.HexToHash("0x01"),
		Siblings: []common.Hash{
			common.HexToHash("0x02"),
		},
	}
	rootHash := p.BuildRoot()
	lhs, rhs, err := p.BuildRootChildren()

	require.NoError(t, err)
	assert.Equal(t, rootHash, crypto.Keccak256Hash(lhs[:], rhs[:]))
}

func TestBuildRootChildren2(t *testing.T) {
	p := Proof{
		Pos:  big.NewInt(1),
		Node: common.HexToHash("0x01"),
		Siblings: []common.Hash{
			common.HexToHash("0x02"),
			common.HexToHash("0x03"),
		},
	}
	rootHash := p.BuildRoot()
	lhs, rhs, err := p.BuildRootChildren()

	require.NoError(t, err)
	assert.Equal(t, rootHash, crypto.Keccak256Hash(lhs[:], rhs[:]))
}

func TestBuildRootChildrenAgainstReference(t *testing.T) {
	hash0 := common.HexToHash("0x976dc34e226f0c9803d556f26426aaa82ba7b5f96a5ed094f4f150c3c27aeaf5")
	hash1 := common.HexToHash("0xfffeb0e2d6fc065fdcf03c25e23e9730528ca7b890308765b0e6b07586db9c6e")
	hash2 := common.HexToHash("0x1588d343bd73f167bf4886b8ab7694b4d83b60087ddbdb445c427c16f26d2644")
	runs := []merkletest.Run{{Hash: hash0, Count: 1 << 24}, {Hash: hash1, Count: 1 << 24},
		{Hash: hash2, Count: (uint64(1) << 48) - (uint64(1) << 25)}}
	expectedRoot := merkletest.Root(runs, 48)
	expectedLeft, expectedRight := merkletest.RootChildren(runs, 48)

	proofSiblings := Proof{
		Pos:  new(big.Int).SetUint64((1 << 48) - 1),
		Node: common.HexToHash("0x1588d343bd73f167bf4886b8ab7694b4d83b60087ddbdb445c427c16f26d2644"),
		Siblings: []common.Hash{
			common.HexToHash("0x1588d343bd73f167bf4886b8ab7694b4d83b60087ddbdb445c427c16f26d2644"),
			common.HexToHash("0x6e94fed4ae1c88a9e09a36ad7b6a4e1cf16d3b9ca3af8cd2d7bb069451690e64"),
			common.HexToHash("0x546d5eed29c39ab4194c600a8534e3d3502478384eb3dc9add095703e422ed38"),
			common.HexToHash("0x38caf4f802a07958ed558995bab538a137d33b6172294362e6b21ac7b7121fd9"),
			common.HexToHash("0xb5daca74fbcfb5af2f8e3c38dc8ad05ec83114b7bba846b3330950266aa40eab"),
			common.HexToHash("0x020b890661da486ee8869b2b6109f4bd795f8bf4bbb88a8d3589a0603f3b824a"),
			common.HexToHash("0xc2c4c83bec36615b797be1407a8e55b08b5cfbeb11dfe6a1db7f942c58d379ed"),
			common.HexToHash("0x1998d07cf9e1006cc3b0f3c5fe08dda1119e471c20f7ceb29695cee940a7e75f"),
			common.HexToHash("0x443631cea815fc6298f029109fe1fda376d3bb185f8e21544829b4b09c947f9c"),
			common.HexToHash("0x159ad50b10bba4d6d2407ce012199523e3f45f2360ba288618c63bf8b91a56ab"),
			common.HexToHash("0x75e8815dec610854174bee1c387a04b78ac942f9f675c7ddce7ec3f6edd69e03"),
			common.HexToHash("0xca9bd9e206bd4e98e4443663dd4eef65c94cf02fd62abd30e36569c56191e9d2"),
			common.HexToHash("0xed97ed05a833b3f0615df43ee27fcc8dc742ccbf84702719747be4f93afce440"),
			common.HexToHash("0x228fddde650b389a339dab2088d3e1c9858989a1eecf1a424d0f4e1e70284047"),
			common.HexToHash("0xb1b6565d519dc4b85d75d074a37a56186f27b240977d2a2ec0e6860217745316"),
			common.HexToHash("0x32f63e77fd8f3762e6ae0efc82b62e0bd99430e656afde91792f4eb3c0eb2d0b"),
			common.HexToHash("0xe50e2d01372de278e92c28a75277bbd83c747b89ef9933cb55c41d4c25f4e043"),
			common.HexToHash("0x3b34305f34e7749600e503c11ad3bd739f3981a16470db39f5d9b324d0c9c2b3"),
			common.HexToHash("0xc62e8ec325b8c50fc32e54b456943f7e191181b38e8decae1d784b06ec331fcd"),
			common.HexToHash("0x176cd516c027b04ea15ef1bf3040f0295880ee9b06124a61707a1e95fd4d7032"),
			common.HexToHash("0xf0ee382cc41769e98ccd36d2996dca753af0dbdf13ea4e121f896fe46ab734c8"),
			common.HexToHash("0xc1ecfb62885ee2bb05bc1de974068c5e64f4f5a23dfdfb85d30dd47314ff3042"),
			common.HexToHash("0x3c8db9c54f3cdc333d266c021b7b82fdd97c6a7936741fe482fe8792206485a9"),
			common.HexToHash("0x599a4d9a455da52573f0a15a9716289e722411d814fffbf854ac0cf9c84f1b64"),
			common.HexToHash("0xb57a65f271d3f4bf499536bcb1158ac8a19ec0b9b87204b0cd12f171cfaf5b50"),
			common.HexToHash("0x66666cb6c0ba3ad91588a289c7207eeb68bc451840126b97aae50e8f9637ddae"),
			common.HexToHash("0x416826c4ee8d20ec51cf42390a2948ef8d15c1d4169143c07152583bdecc64a4"),
			common.HexToHash("0x759d0b439a3e16c4bcd780b724775279e4f6c20a1cee40c4c4452ce5444b8b53"),
			common.HexToHash("0x7888795e43e9f91907342bf8694756ad251bce7d6aeeea393a7da65598873b61"),
			common.HexToHash("0x21a9e92e15cd6e5d8c7b0f20416a7ced013f4f0dc7f42cce889cda91ea92d16a"),
			common.HexToHash("0x8e30f629baa35c5de738d64d562e0492623fc1b42aa3205711c3b740124f281c"),
			common.HexToHash("0xd3c85a40b8e5ecd91966a47acba3a6abb17b2995bb10fb61bf84b57ca25a7516"),
			common.HexToHash("0x76e076fb82921263e6cff4a5579419d3b8fb9c9398f147bbc7108f72912cd67a"),
			common.HexToHash("0x43bdd668fdaef54e7a315e68918095373b4d7538fb383535305265e29346e0c1"),
			common.HexToHash("0x19df614f8b4749c1a9ed4ed3949691d0c2d9209541e1fbf2c30dbc32e990bcd9"),
			common.HexToHash("0xfc2379f2a164ec902fff26ff3840383dcba0ea54f04f703b4f56844e4db1dbce"),
			common.HexToHash("0x1dd54410bdd1baaeafc1dbf9c04f5f05f698fdbd7d3209fa55aeec1c7d0fe19a"),
			common.HexToHash("0x721ad44b29a2c63e1ba75fd4774fa0e1a0a5bff806d4b441debfd6a95e75f5c9"),
			common.HexToHash("0xfa1144bd963f10d4a325184fbe43a36ec3f96a4212d71592e84679975117ef5c"),
			common.HexToHash("0xefef7b6fab7d64ac6f6764f932835af1f9015d11dc94ae1112956fd801cd8a30"),
			common.HexToHash("0xea1e381daf0fd8ecfd94994fcc88fb2874f08ab3bbc397d86389d433e7fc8368"),
			common.HexToHash("0xcfeff346f98bb48c38a0c650fe7fa2b9669ebdb37128cbe813a16a34e42fd7bf"),
			common.HexToHash("0x86835aafc70841ff671b492912db2407511b5a631823f02f8b2bdc0308103a82"),
			common.HexToHash("0xd86da47a2e5be3b6c43d25ae86977ce1db075ce9f82ead96d090d7a34a3bdc4f"),
			common.HexToHash("0x081d234561620d2bf78176378d004ed891749c04600d1abd053616ce4aed15b4"),
			common.HexToHash("0x85e7040a6c668e5fee700b3a3af4b9a0de0806a832f53458860c28ccd3ec92b8"),
			common.HexToHash("0xcbb7692c46db878f98bd4ed47f028397ca1508eb381b8eb6f08bc0c2c9844d29"),
			common.HexToHash("0x3dd5ebb22e39edc69e6259ef7f3fab49d4e73078e61c5a26737e8a387fa456e6"),
		},
	}

	rootHashProof := proofSiblings.BuildRoot()
	lhsProof, rhsProof, err := proofSiblings.BuildRootChildren()

	require.NoError(t, err)
	assert.Equal(t, expectedRoot, rootHashProof)
	assert.Equal(t, lhsProof, expectedLeft)
	assert.Equal(t, rhsProof, expectedRight)
	assert.Equal(t, expectedRoot, crypto.Keccak256Hash(expectedLeft[:], expectedRight[:]))
	assert.Equal(t, rootHashProof, crypto.Keccak256Hash(lhsProof[:], rhsProof[:]))
}
