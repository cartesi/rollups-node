// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"
)

type ComputeProofSuite struct {
	suite.Suite
	pristine model.Hash
}

func TestComputeProofSuite(t *testing.T) {
	suite.Run(t, new(ComputeProofSuite))
}

func (s *ComputeProofSuite) TestZeroHeight() {
	s.Run("no leaves", func() {
		root, siblings, err := CreateProofs(nil, 0)
		s.Require().Nil(err)

		s.Equal(s.pristine, root)
		s.Equal(len(siblings), 1)
		s.Equal(len(siblings[0]), 0)
	})

	s.Run("one leaf", func() {
		leaf := crypto.Keccak256Hash([]byte("Cartesi"))

		root, siblings, err := CreateProofs([]model.Hash{leaf}, 0)
		s.Require().Nil(err)

		s.Equal(leaf, root)
		s.Equal(len(siblings), 1)
		s.Equal(len(siblings[0]), 0)
	})

	s.Run("two leafs", func() {
		leaves := make([]model.Hash, 2)

		_, _, err := CreateProofs(leaves, 0)

		s.ErrorContains(err, "too many leaves for height")
	})
}
func (s *ComputeProofSuite) TestHeightOne() {
	height := 1
	leaf1 := crypto.Keccak256Hash([]byte("Cartesi"))
	leaf2 := crypto.Keccak256Hash([]byte("Merkle"))
	s.Run("no leaves", func() {
		root, siblings, err := CreateProofs(nil, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(s.pristine[:], s.pristine[:]),
			root,
		)
		s.Equal(len(siblings), height)
		s.Equal(len(siblings[0]), 1)
		s.Equal(siblings[0][0], s.pristine)
	})

	s.Run("one leaf", func() {
		root, siblings, err := CreateProofs([]model.Hash{leaf1}, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(leaf1[:], s.pristine[:]),
			root,
		)
		s.Equal(len(siblings), height)
		s.Equal(len(siblings[0]), 1)
		s.Equal(siblings[0][0], s.pristine)
	})

	s.Run("two leaves", func() {
		leaves := []model.Hash{leaf1, leaf2}

		root, siblings, err := CreateProofs(leaves, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(leaf1[:], leaf2[:]),
			root,
		)
		s.Equal(len(siblings), len(leaves))
		for idx := range siblings {
			s.Equal(len(siblings[idx]), height)
		}
		s.Equal(siblings[0][0], leaf2)
		s.Equal(siblings[1][0], leaf1)
	})

	s.Run("three leafs", func() {
		leaves := make([]model.Hash, 3)

		_, _, err := CreateProofs(leaves, 1)

		s.ErrorContains(err, "too many leaves for height")
	})
}

func (s *ComputeProofSuite) TestHeightTwo() {
	height := 2
	leaf1 := crypto.Keccak256Hash([]byte("Merkle"))
	leaf2 := crypto.Keccak256Hash([]byte("trees"))
	leaf3 := crypto.Keccak256Hash([]byte("are"))
	leaf4 := crypto.Keccak256Hash([]byte("cool"))

	s.Run("two leaves", func() {
		leaves := []model.Hash{leaf1, leaf2}

		root, siblings, err := CreateProofs(leaves, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(s.pristine[:], s.pristine[:]).Bytes(),
			),
			root,
		)

		s.Equal(len(siblings), len(leaves))
		for idx := range siblings {
			s.Equal(len(siblings[idx]), height)
		}
		s.Equal(siblings[0][0], leaf2)
		s.Equal(siblings[1][0], leaf1)
		s.Equal(siblings[0][1], crypto.Keccak256Hash(s.pristine[:], s.pristine[:]))
		s.Equal(siblings[1][1], crypto.Keccak256Hash(s.pristine[:], s.pristine[:]))
	})

	s.Run("three leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3}

		root, siblings, err := CreateProofs(leaves, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], s.pristine[:]).Bytes(),
			),
			root,
		)

		s.Equal(len(siblings), len(leaves))
		for idx := range siblings {
			s.Equal(len(siblings[idx]), height)
		}
		s.Equal(siblings[0][0], leaf2)
		s.Equal(siblings[1][0], leaf1)
		s.Equal(siblings[2][0], s.pristine)
		s.Equal(siblings[0][1], crypto.Keccak256Hash(leaf3[:], s.pristine[:]))
		s.Equal(siblings[1][1], crypto.Keccak256Hash(leaf3[:], s.pristine[:]))
		s.Equal(siblings[2][1], crypto.Keccak256Hash(leaf1[:], leaf2[:]))
	})

	s.Run("four leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3, leaf4}

		root, siblings, err := CreateProofs(leaves, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			root,
		)

		s.Equal(len(siblings), len(leaves))
		for idx := range siblings {
			s.Equal(len(siblings[idx]), height)
		}
		s.Equal(siblings[0][0], leaf2)
		s.Equal(siblings[1][0], leaf1)
		s.Equal(siblings[2][0], leaf4)
		s.Equal(siblings[3][0], leaf3)
		s.Equal(siblings[0][1], crypto.Keccak256Hash(leaf3[:], leaf4[:]))
		s.Equal(siblings[1][1], crypto.Keccak256Hash(leaf3[:], leaf4[:]))
		s.Equal(siblings[2][1], crypto.Keccak256Hash(leaf1[:], leaf2[:]))
		s.Equal(siblings[3][1], crypto.Keccak256Hash(leaf1[:], leaf2[:]))
	})
}

func (s *ComputeProofSuite) TestHeightThree() {
	height := 3
	leaf1 := crypto.Keccak256Hash([]byte("Merkle"))
	leaf2 := crypto.Keccak256Hash([]byte("trees"))
	leaf3 := crypto.Keccak256Hash([]byte("are"))
	leaf4 := crypto.Keccak256Hash([]byte("so"))
	leaf5 := crypto.Keccak256Hash([]byte("much"))
	leaf6 := crypto.Keccak256Hash([]byte("fun"))
	leaf7 := crypto.Keccak256Hash([]byte("wow"))

	s.Run("six leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3, leaf4, leaf5, leaf6}

		root, siblings, err := CreateProofs(leaves, uint(height))
		s.Require().Nil(err)

		s.Equal(
			common.HexToHash("0xfc16527248afe9934764bccf38a45bd6e3bd2fc094ab8649a2c81a6ef9f2c4b2"),
			root,
		)
		s.Equal(len(siblings), len(leaves))
		s.Equal(len(siblings[0]), 3)
	})

	s.Run("seven leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3, leaf4, leaf5, leaf6, leaf7}

		root, siblings, err := CreateProofs(leaves, 3)
		s.Require().Nil(err)

		s.Equal(
			common.HexToHash("0x111eeb9da43f29ed8482602b2db72385a2780537e25ef6352d609de59da73ea1"),
			root,
		)
		s.Equal(len(siblings), len(leaves))
		s.Equal(len(siblings[0]), 3)
	})
}

// This test was taken from the libcmt suite
// as a method to compare both implementations
func (s *ComputeProofSuite) TestItMatchesMachineImplementation() {
	leaves := []model.Hash{
		crypto.Keccak256Hash([]byte("Cartesi")),
		crypto.Keccak256Hash([]byte("Merkle")),
		crypto.Keccak256Hash([]byte("Tree")),
	}

	root, _, err := CreateProofs(leaves, 16)
	s.Require().Nil(err)

	s.Equal(
		common.HexToHash("0xe8e0477114cb630c4d14eea249eb2c63d84c9c685ddf35d137019e659ae20418"),
		root,
	)
}
