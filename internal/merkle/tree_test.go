// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"fmt"
	"testing"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"
)

type MerkleTreeSuite struct {
	suite.Suite
}

func TestTreeSuite(t *testing.T) {
	suite.Run(t, new(MerkleTreeSuite))
}

func (s *MerkleTreeSuite) TestItCreatesATree() {
	failureTestCases := []uint{65, 100}
	for _, tc := range failureTestCases {
		s.Run(fmt.Sprintf("Fails_when_height_%d", tc), func() {
			_, err := NewTree(tc)
			s.NotNil(err)
		})
	}
	successTestCases := []uint{0, 16, 32, 64}
	for _, tc := range successTestCases {
		s.Run(fmt.Sprintf("Succeeds_when_height_%d", tc), func() {
			_, err := NewTree(tc)
			s.Nil(err)
		})
	}
}

func (s *MerkleTreeSuite) TestItCreatesATreeWithLeaves() {
	type TreeWithLeavesTestCase struct {
		height uint
		leaves []Hash
		name   string
	}
	failureTestCases := []TreeWithLeavesTestCase{
		{0, make([]Hash, 2), "height zero leaves above maximum"},
		{1, make([]Hash, 3), "height one leaf above maximum"},
		{65, make([]Hash, 3), "height above maximum with leaves"},
		{65, nil, "height above maximum no leaves"},
	}
	successTestCases := []TreeWithLeavesTestCase{
		{0, make([]Hash, 1), "single leaf with zero height"},
		{0, []Hash{}, "empty leaves slice"},
		{0, nil, "nil leaves slice"},
		{1, make([]Hash, 2), "maximum leaf count"},
		{1, make([]Hash, 1), "less leaves than maximum"},
		{64, nil, "maximum height no leaves"},
		{64, make([]Hash, 32), "maximum height with leaves"},
	}

	for _, tc := range failureTestCases {
		s.Run(fmt.Sprintf("Fails_when_%s", tc.name), func() {
			_, err := NewTreeFromLeaves(tc.height, tc.leaves)
			s.NotNil(err)
		})
	}
	for _, tc := range successTestCases {
		s.Run(fmt.Sprintf("Succeeds_when_%s", tc.name), func() {
			_, err := NewTreeFromLeaves(tc.height, tc.leaves)
			s.Nil(err)
		})
	}
}

func (s *MerkleTreeSuite) TestItPushesNewLeaves() {
	type PushLeafTestCase struct {
		height  uint
		leaves  []Hash
		newLeaf Hash
		name    string
	}
	failureTestCases := []PushLeafTestCase{
		{0, nil, Hash{}, "zero height tree"},
		{1, make([]Hash, 2), Hash{}, "reached max capacity"},
	}
	successTestCases := []PushLeafTestCase{
		{2, make([]Hash, 1), Hash{}, "odd number of leaves"},
		{2, make([]Hash, 2), Hash{}, "even number of leaves"},
		{2, make([]Hash, 3), Hash{}, "right below maximum capacity"},
	}

	for _, tc := range failureTestCases {
		s.Run(fmt.Sprintf("Fails_when_%s", tc.name), func() {
			tree, err := NewTreeFromLeaves(tc.height, tc.leaves)
			s.Require().Nil(err, "invalid test case")

			err = tree.Push(tc.newLeaf)
			s.NotNil(err)
		})
	}

	for _, tc := range successTestCases {
		s.Run(fmt.Sprintf("Succeeds_when_%s", tc.name), func() {
			tree, err := NewTreeFromLeaves(tc.height, tc.leaves)
			s.Require().Nil(err, "invalid test case")

			err = tree.Push(tc.newLeaf)
			s.Nil(err)
		})
	}
}

func (s *MerkleTreeSuite) TestItPushesData() {
	type PushDataTestCase struct {
		height  uint
		leaves  []Hash
		newLeaf []byte
		name    string
	}
	failureTestCases := []PushDataTestCase{
		{0, nil, []byte{}, "zero height tree"},
		{1, make([]Hash, 2), []byte{}, "reached max capacity"},
	}
	successTestCases := []PushDataTestCase{
		{2, make([]Hash, 1), []byte{}, "odd number of leaves"},
		{2, make([]Hash, 2), []byte{}, "even number of leaves"},
		{2, make([]Hash, 3), []byte{}, "right below maximum capacity"},
	}

	for _, tc := range failureTestCases {
		s.Run(fmt.Sprintf("Fails_when_%s", tc.name), func() {
			tree, err := NewTreeFromLeaves(tc.height, tc.leaves)
			s.Require().Nil(err, "invalid test case")

			err = tree.PushData(tc.newLeaf)
			s.NotNil(err)
		})
	}

	for _, tc := range successTestCases {
		s.Run(fmt.Sprintf("Succeeds_when_%s", tc.name), func() {
			tree, err := NewTreeFromLeaves(tc.height, tc.leaves)
			s.Require().Nil(err, "invalid test case")

			err = tree.PushData(tc.newLeaf)
			s.Nil(err)
		})
	}
}

func (s *MerkleTreeSuite) TestItCalculatesTheRootHash() {
	leaf := common.HexToHash("0xdeadbeef")
	testCases := []struct {
		height           uint
		leaves           []Hash
		expectedRootHash Hash
		name             string
	}{
		{0, nil, Hash{}, "height zero no leaf"},
		{0, make([]Hash, 1), Hash{}, "height zero single leaf"},
		{0, []Hash{leaf}, leaf, "height zero non-pristine leaf"},
		{
			1,
			[]Hash{leaf},
			crypto.Keccak256Hash(leaf[:], Hash{}.Bytes()),
			"height one single non-pristine leaf",
		},
		{
			1,
			make([]Hash, 2),
			common.HexToHash("0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"),
			"height one two leaves",
		},
		{
			3,
			[]Hash{leaf, leaf, leaf, leaf},
			common.HexToHash("0xaf8e3df4d71f4e778d599dd761c441f7f1ad517c4e3603aec59fa24bb9f63a87"),
			"height three even number of non-pristine leaves",
		},
		{
			3,
			[]Hash{leaf, leaf, leaf, leaf, leaf},
			common.HexToHash("0x7191aa62a7213586361a405d6a45906f9547fcf042f7d9b985da4775b32c0704"),
			"height three even odd of non-pristine leaves",
		},
		{
			5,
			make([]Hash, 32),
			common.HexToHash("0x0eb01ebfc9ed27500cd4dfc979272d1f0913cc9f66540d7e8005811109e1cf2d"),
			"height five maximum leaves",
		},
	}

	for _, tc := range testCases {
		s.Run(fmt.Sprintf("Succeeds_when_%s", tc.name), func() {
			tree, err := NewTreeFromLeaves(tc.height, tc.leaves)
			s.Require().Nil(err, "invalid test case")

			rootHash := tree.RootHash()
			s.Equal(tc.expectedRootHash, rootHash)
		})
	}
}

func (s *MerkleTreeSuite) TestRootHashWithBothInitializersMatch() {
	leaf := common.HexToHash("0xdeadbeef")
	treeInitializedWithLeaves, err := NewTreeFromLeaves(3, []common.Hash{leaf, leaf, leaf})
	s.Require().Nil(err)

	tree, err := NewTree(3)
	s.Require().Nil(err)
	_ = tree.Push(leaf)
	_ = tree.Push(leaf)
	_ = tree.Push(leaf)

	s.Equal(
		tree.RootHash(),
		treeInitializedWithLeaves.RootHash(),
	)
}

func (s *MerkleTreeSuite) TestRootHashWithBothPushMethodsMatch() {
	data := common.FromHex("0xdeadbeef")
	leaf := crypto.Keccak256Hash(data)

	hashTree, err := NewTreeFromLeaves(3, []common.Hash{leaf, leaf, leaf})
	s.Require().Nil(err)

	dataTree, err := NewTree(3)
	s.Require().Nil(err)
	_ = dataTree.PushData(data)
	_ = dataTree.PushData(data)
	_ = dataTree.PushData(data)

	s.Equal(
		dataTree.RootHash(),
		hashTree.RootHash(),
	)
}

func (s *MerkleTreeSuite) TestItGetsSiblings() {
	type GetSiblingsTestCase struct {
		height           uint
		leaves           []Hash
		leafIndex        uint
		expectedSiblings []Hash
		name             string
	}
	leaf := common.HexToHash("0xdeadbeef")
	successTestCases := []GetSiblingsTestCase{
		{0, nil, 0, nil, "height zero no siblings"},
		{0, make([]Hash, 1), 0, nil, "height zero non-pristine root no siblings"},
		{1, []Hash{{}, leaf}, 0, []Hash{leaf}, "height one right sibling"},
		{1, []Hash{{}, leaf}, 1, make([]Hash, 1), "height one left sibling"},
		{
			2,
			make([]Hash, 4),
			0,
			[]Hash{
				{},
				common.HexToHash(
					"0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
				),
			},
			"height two",
		},
		{
			3,
			make([]Hash, 8),
			0,
			[]Hash{
				{},
				common.HexToHash(
					"0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
				),
				common.HexToHash(
					"0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30",
				),
			},
			"height three",
		},
	}
	failureTestCases := []GetSiblingsTestCase{
		{0, nil, 1, nil, "height zero invalid leaf"},
		{0, make([]Hash, 1), 1, nil, "height zero invalid leaf"},
	}

	for _, tc := range successTestCases {
		s.Run(fmt.Sprintf("Succeeds_when_%s", tc.name), func() {
			tree, err := NewTreeFromLeaves(tc.height, tc.leaves)
			s.Require().Nil(err, "invalid test case")

			siblings, err := tree.SiblingsOfLeaf(tc.leafIndex)
			s.Require().Nil(err)

			s.Len(siblings, len(tc.expectedSiblings))
			for idx, sibling := range siblings {
				s.Equal(sibling, tc.expectedSiblings[idx])
			}
		})
	}

	for _, tc := range failureTestCases {
		s.Run(fmt.Sprintf("Fails_when_%s", tc.name), func() {
			tree, err := NewTreeFromLeaves(tc.height, tc.leaves)
			s.Require().Nil(err, "invalid test case")

			_, err = tree.SiblingsOfLeaf(tc.leafIndex)
			s.NotNil(err)
		})
	}
}

func (s *MerkleTreeSuite) TestItMatchesCMImplementation() {
	tree, err := NewTree(16)
	s.Require().Nil(err)

	_ = tree.PushData([]byte("Cartesi"))
	_ = tree.PushData([]byte("Merkle"))
	_ = tree.PushData([]byte("Tree"))

	s.Equal(
		common.HexToHash("0xe8e0477114cb630c4d14eea249eb2c63d84c9c685ddf35d137019e659ae20418"),
		tree.RootHash(),
	)
}
