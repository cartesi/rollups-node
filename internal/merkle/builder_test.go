// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
)

var (
	oneDigest  = common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001")
	zeroDigest = common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000")
)

func TestIsCountPow2(t *testing.T) {
	assert.True(t, isCountPow2(big.NewInt(0)))
	assert.True(t, isCountPow2(big.NewInt(1)))
	assert.True(t, isCountPow2(big.NewInt(2)))
	assert.False(t, isCountPow2(big.NewInt(3)))
	assert.True(t, isCountPow2(big.NewInt(4)))
	assert.False(t, isCountPow2(big.NewInt(5)))
}

// repanicked
//func TestRepeatZero(t *testing.T) {
//	defer recover()
//
//	builder := Builder{}
//	builder.AppendRepeatedUint64(TreeLeaf(zeroHash), 0)
//}

func TestSimple0(t *testing.T) {
	builder := Builder{}
	builder.Append(TreeLeaf(oneDigest))
	treeRoot := builder.Build().RootHash
	expected := oneDigest

	assert.Equal(t, expected, treeRoot)
}

func TestSimple1(t *testing.T) {
	builder := Builder{}
	builder.Append(TreeLeaf(zeroDigest))
	builder.Append(TreeLeaf(oneDigest))
	treeRoot := builder.Build().RootHash

	expected := TreeLeaf(zeroDigest).Join(TreeLeaf(oneDigest)).RootHash

	assert.Equal(t, expected, treeRoot)
}

func TestSimple2(t *testing.T) {
	builder := Builder{}
	builder.AppendRepeatedUint64(TreeLeaf(oneDigest), 2)
	builder.AppendRepeatedUint64(TreeLeaf(zeroDigest), 2)
	treeRoot := builder.Build().RootHash

	lhs := TreeLeaf(oneDigest).Join(TreeLeaf(oneDigest))
	rhs := TreeLeaf(zeroDigest).Join(TreeLeaf(zeroDigest))
	expected := lhs.Join(rhs).RootHash

	assert.Equal(t, expected, treeRoot)
}

func TestSimple3(t *testing.T) {
	builder := Builder{}
	builder.Append(TreeLeaf(zeroDigest))
	builder.AppendRepeatedUint64(TreeLeaf(oneDigest), 2)
	builder.Append(TreeLeaf(zeroDigest))
	treeRoot := builder.Build().RootHash

	lhs := TreeLeaf(zeroDigest).Join(TreeLeaf(oneDigest))
	rhs := TreeLeaf(oneDigest).Join(TreeLeaf(zeroDigest))
	expected := lhs.Join(rhs).RootHash

	assert.Equal(t, expected, treeRoot)
}

func TestMerkleBuilder8(t *testing.T) {
	builder := Builder{}
	builder.AppendRepeatedUint64(TreeLeaf(zeroDigest), 2)
	builder.AppendRepeatedUint64(TreeLeaf(zeroDigest), 6)
	assert.True(t, builder.CanBuild())

	merkle := builder.Build()
	assert.Equal(t, merkle.RootHash, TreeLeaf(zeroDigest).Iterated(3).RootHash)
}

func TestMerkleBuilder64(t *testing.T) {
	one := big.NewInt(1)
	two := big.NewInt(2)
	reps := new(big.Int).Sub(new(big.Int).Lsh(one, 64), two)

	builder := Builder{}
	builder.AppendRepeatedUint64(TreeLeaf(zeroDigest), 2)
	builder.AppendRepeated(TreeLeaf(zeroDigest), reps)
	assert.True(t, builder.CanBuild())

	merkle := builder.Build()
	assert.Equal(t, merkle.RootHash, TreeLeaf(zeroDigest).Iterated(64).RootHash)
}

func TestMerkleBuilder256(t *testing.T) {
	one := big.NewInt(1)
	reps := new(big.Int).Lsh(one, 256)

	builder := Builder{}
	builder.AppendRepeated(TreeLeaf(zeroDigest), reps)
	assert.True(t, builder.CanBuild())

	merkle := builder.Build()
	assert.Equal(t, merkle.RootHash, TreeLeaf(zeroDigest).Iterated(256).RootHash)
}

func TestAppendAndRepeated(t *testing.T) {
	builder := Builder{}
	builder.Append(TreeLeaf(zeroDigest))
	assert.True(t, builder.CanBuild())
	tree1 := builder.Build()

	builder = Builder{}
	builder.AppendRepeatedUint64(TreeLeaf(zeroDigest), 1)
	tree2 := builder.Build()

	assert.Equal(t, tree1, tree2)
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

	assert.Nil(t, err)
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

	assert.Nil(t, err)
	assert.Equal(t, rootHash, crypto.Keccak256Hash(lhs[:], rhs[:]))
}

func TestBuildRootChildren3(t *testing.T) {
	p := Proof{
		Pos:  big.NewInt(1),
		Node: common.HexToHash("0x01"),
		Siblings: []common.Hash{
			common.HexToHash("0x02"),
			common.HexToHash("0x03"),
			common.HexToHash("0x04"),
		},
	}
	rootHash := p.BuildRoot()
	lhs, rhs, err := p.BuildRootChildren()

	assert.Nil(t, err)
	assert.Equal(t, rootHash, crypto.Keccak256Hash(lhs[:], rhs[:]))
}

// repanicked
//func TestBuildNotPow2(t *testing.T) {
//	defer recover()
//
//	builder := Builder{}
//	builder.Append(TreeLeaf(zeroDigest))
//	builder.Append(TreeLeaf(zeroDigest))
//	builder.Append(TreeLeaf(zeroDigest))
//	assert.False(t, builder.CanBuild())
//
//	builder.Build()
//}
