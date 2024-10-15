// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"math/rand"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"
)

type CreateProofsSuite struct {
	suite.Suite
	pristine []model.Hash
}

func TestCreateProofsSuite(t *testing.T) {
	suite.Run(t, new(CreateProofsSuite))
}

func (s *CreateProofsSuite) SetupSuite() {
	maxHeight := 4
	s.pristine = make([]model.Hash, maxHeight)

	for height := 1; height < maxHeight; height++ {
		s.pristine[height] = crypto.Keccak256Hash(
			s.pristine[height-1][:],
			s.pristine[height-1][:],
		)
	}
}

func (s *CreateProofsSuite) TestZeroHeight() {
	s.Run("no leaves", func() {
		root, siblings, err := CreateProofs(nil, 0)
		s.Require().Nil(err)

		s.Equal(s.pristine[0], root)
		// if there are no leaves, there is nothing to prove
		s.Equal(0, len(siblings))
	})

	s.Run("one leaf", func() {
		leaf := crypto.Keccak256Hash([]byte("Cartesi"))

		root, siblings, err := CreateProofs([]model.Hash{leaf}, 0)
		s.Require().Nil(err)

		s.Equal(leaf, root)
		s.Equal(0, len(siblings))
	})

	s.Run("two leaves", func() {
		leaves := make([]model.Hash, 2)

		_, _, err := CreateProofs(leaves, 0)
		s.Require().NotNil(err)

		s.ErrorContains(err, "too many leaves")
	})
}
func (s *CreateProofsSuite) TestHeightOne() {
	height := 1
	leaf1 := crypto.Keccak256Hash([]byte("Cartesi"))
	leaf2 := crypto.Keccak256Hash([]byte("Merkle"))

	s.Run("no leaves", func() {
		root, siblings, err := CreateProofs(nil, uint(height))
		s.Require().Nil(err)

		s.Equal(s.pristine[1], root)

		s.Equal(0, len(siblings))
	})

	s.Run("one leaf", func() {
		leaves := []model.Hash{leaf1}

		root, siblings, err := CreateProofs(leaves, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(leaf1[:], s.pristine[0][:]),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(s.pristine[0], siblings[0])

		leafSiblings := siblings[0 : 0+height]
		s.Equal(root, rootFromSiblings(leaf1, 0, leafSiblings))
	})

	s.Run("two leaves", func() {
		leaves := []model.Hash{leaf1, leaf2}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(leaf1[:], leaf2[:]),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0*height+0])
		s.Equal(leaf1, siblings[1*height+0])

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})

	s.Run("three leaves", func() {
		leaves := make([]model.Hash, 3)

		_, _, err := CreateProofs(leaves, 1)
		s.Require().NotNil(err)

		s.ErrorContains(err, "too many leaves")
	})
}

func (s *CreateProofsSuite) TestHeightTwo() {
	height := 2
	leaf1 := crypto.Keccak256Hash([]byte("Merkle"))
	leaf2 := crypto.Keccak256Hash([]byte("trees"))
	leaf3 := crypto.Keccak256Hash([]byte("are"))
	leaf4 := crypto.Keccak256Hash([]byte("cool"))

	s.Run("no leaves", func() {
		root, siblings, err := CreateProofs(nil, uint(height))
		s.Require().Nil(err)

		s.Equal(s.pristine[2], root)

		s.Equal(0, len(siblings))
	})

	s.Run("one leaf", func() {
		leaves := []model.Hash{leaf1}

		root, siblings, err := CreateProofs(leaves, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], s.pristine[0][:]).Bytes(),
				s.pristine[1][:],
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(s.pristine[0], siblings[0])

		leafSiblings := siblings[0 : 0+height]
		s.Equal(root, rootFromSiblings(leaf1, 0, leafSiblings))
	})

	s.Run("two leaves", func() {
		leaves := []model.Hash{leaf1, leaf2}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				s.pristine[1][:],
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0*height])
		s.Equal(leaf1, siblings[1*height])
		s.Equal(s.pristine[1], siblings[0*height+1])
		s.Equal(s.pristine[1], siblings[1*height+1])

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})

	s.Run("three leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], s.pristine[0][:]).Bytes(),
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0*height])
		s.Equal(leaf1, siblings[1*height])
		s.Equal(s.pristine[0], siblings[2*height])
		s.Equal(crypto.Keccak256Hash(leaf3[:], s.pristine[0][:]), siblings[0*height+1])
		s.Equal(crypto.Keccak256Hash(leaf3[:], s.pristine[0][:]), siblings[1*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[2*height+1])

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})

	s.Run("four leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3, leaf4}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0*height])
		s.Equal(leaf1, siblings[1*height])
		s.Equal(leaf4, siblings[2*height])
		s.Equal(leaf3, siblings[3*height])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[0*height+1])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[1*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[2*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[3*height+1])

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})

	s.Run("five leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3, leaf4, leaf1}

		_, _, err := CreateProofs(leaves, uint(height))
		s.Require().NotNil(err)

		s.ErrorContains(err, "too many leaves")
	})
}

func (s *CreateProofsSuite) TestHeightThree() {
	height := 3
	leaf1 := crypto.Keccak256Hash([]byte("Merkle"))
	leaf2 := crypto.Keccak256Hash([]byte("trees"))
	leaf3 := crypto.Keccak256Hash([]byte("are"))
	leaf4 := crypto.Keccak256Hash([]byte("so"))
	leaf5 := crypto.Keccak256Hash([]byte("much"))
	leaf6 := crypto.Keccak256Hash([]byte("fun"))
	leaf7 := crypto.Keccak256Hash([]byte("wow"))
	leaf8 := crypto.Keccak256Hash([]byte("!"))

	s.Run("no leaves", func() {
		root, siblings, err := CreateProofs(nil, uint(height))
		s.Require().Nil(err)

		s.Equal(s.pristine[height], root)

		s.Equal(0, len(siblings))
	})

	s.Run("one leaf", func() {
		leaves := []model.Hash{leaf1}

		root, siblings, err := CreateProofs(leaves, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf1[:], s.pristine[0][:]).Bytes(),
					s.pristine[1][:],
				).Bytes(),
				s.pristine[2][:],
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(s.pristine[0], siblings[0])
		s.Equal(s.pristine[1], siblings[1])
		s.Equal(s.pristine[2], siblings[2])

		leafSiblings := siblings[0 : 0+height]
		s.Equal(root, rootFromSiblings(leaf1, 0, leafSiblings))
	})

	s.Run("two leaves", func() {
		leaves := []model.Hash{leaf1, leaf2}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
					s.pristine[1][:],
				).Bytes(),
				s.pristine[2][:],
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0])
		s.Equal(leaf1, siblings[1*height])
		s.Equal(s.pristine[1], siblings[0*height+1])
		s.Equal(s.pristine[1], siblings[1*height+1])
		s.Equal(s.pristine[2], siblings[0*height+2])
		s.Equal(s.pristine[2], siblings[1*height+2])

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})

	s.Run("three leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
					crypto.Keccak256Hash(leaf3[:], s.pristine[0][:]).Bytes(),
				).Bytes(),
				s.pristine[2][:],
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0])
		s.Equal(leaf1, siblings[1*height])
		s.Equal(s.pristine[0], siblings[2*height])
		s.Equal(crypto.Keccak256Hash(leaf3[:], s.pristine[0][:]), siblings[0*height+1])
		s.Equal(crypto.Keccak256Hash(leaf3[:], s.pristine[0][:]), siblings[1*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[2*height+1])
		s.Equal(s.pristine[2], siblings[0*height+2])
		s.Equal(s.pristine[2], siblings[1*height+2])
		s.Equal(s.pristine[2], siblings[2*height+2])

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})

	s.Run("four leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3, leaf4}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
					crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
				).Bytes(),
				s.pristine[2][:],
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0])
		s.Equal(leaf1, siblings[1*height])
		s.Equal(leaf4, siblings[2*height])
		s.Equal(leaf3, siblings[3*height])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[0*height+1])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[1*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[2*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[3*height+1])
		s.Equal(s.pristine[2], siblings[0*height+2])
		s.Equal(s.pristine[2], siblings[1*height+2])
		s.Equal(s.pristine[2], siblings[2*height+2])
		s.Equal(s.pristine[2], siblings[3*height+2])

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})

	s.Run("five leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3, leaf4, leaf5}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
					crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
				).Bytes(),
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf5[:], s.pristine[0][:]).Bytes(),
					s.pristine[1][:],
				).Bytes(),
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0])
		s.Equal(leaf1, siblings[1*height])
		s.Equal(leaf4, siblings[2*height])
		s.Equal(leaf3, siblings[3*height])
		s.Equal(s.pristine[0], siblings[4*height])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[0*height+1])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[1*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[2*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[3*height+1])
		s.Equal(s.pristine[1], siblings[4*height+1])
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], s.pristine[0][:]).Bytes(),
				s.pristine[1][:],
			),
			siblings[0*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], s.pristine[0][:]).Bytes(),
				s.pristine[1][:],
			),
			siblings[1*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], s.pristine[0][:]).Bytes(),
				s.pristine[1][:],
			),
			siblings[2*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], s.pristine[0][:]).Bytes(),
				s.pristine[1][:],
			),
			siblings[3*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			siblings[4*height+2],
		)

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})

	s.Run("six leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3, leaf4, leaf5, leaf6}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
					crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
				).Bytes(),
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
					s.pristine[1][:],
				).Bytes(),
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0])
		s.Equal(leaf1, siblings[1*height])
		s.Equal(leaf4, siblings[2*height])
		s.Equal(leaf3, siblings[3*height])
		s.Equal(leaf6, siblings[4*height])
		s.Equal(leaf5, siblings[5*height])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[0*height+1])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[1*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[2*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[3*height+1])
		s.Equal(s.pristine[1], siblings[4*height+1])
		s.Equal(s.pristine[1], siblings[5*height+1])
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				s.pristine[1][:],
			),
			siblings[0*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				s.pristine[1][:],
			),
			siblings[1*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				s.pristine[1][:],
			),
			siblings[2*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				s.pristine[1][:],
			),
			siblings[3*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			siblings[4*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			siblings[5*height+2],
		)

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})

	s.Run("seven leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3, leaf4, leaf5, leaf6, leaf7}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
					crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
				).Bytes(),
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
					crypto.Keccak256Hash(leaf7[:], s.pristine[0][:]).Bytes(),
				).Bytes(),
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0])
		s.Equal(leaf1, siblings[1*height])
		s.Equal(leaf4, siblings[2*height])
		s.Equal(leaf3, siblings[3*height])
		s.Equal(leaf6, siblings[4*height])
		s.Equal(leaf5, siblings[5*height])
		s.Equal(s.pristine[0], siblings[6*height])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[0*height+1])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[1*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[2*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[3*height+1])
		s.Equal(crypto.Keccak256Hash(leaf7[:], s.pristine[0][:]), siblings[4*height+1])
		s.Equal(crypto.Keccak256Hash(leaf7[:], s.pristine[0][:]), siblings[5*height+1])
		s.Equal(crypto.Keccak256Hash(leaf5[:], leaf6[:]), siblings[6*height+1])
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				crypto.Keccak256Hash(leaf7[:], s.pristine[0][:]).Bytes(),
			),
			siblings[0*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				crypto.Keccak256Hash(leaf7[:], s.pristine[0][:]).Bytes(),
			),
			siblings[1*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				crypto.Keccak256Hash(leaf7[:], s.pristine[0][:]).Bytes(),
			),
			siblings[2*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				crypto.Keccak256Hash(leaf7[:], s.pristine[0][:]).Bytes(),
			),
			siblings[3*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			siblings[4*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			siblings[5*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			siblings[6*height+2],
		)

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})

	s.Run("eight leaves", func() {
		leaves := []model.Hash{leaf1, leaf2, leaf3, leaf4, leaf5, leaf6, leaf7, leaf8}
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, uint(height))
		s.Require().Nil(err)

		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
					crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
				).Bytes(),
				crypto.Keccak256Hash(
					crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
					crypto.Keccak256Hash(leaf7[:], leaf8[:]).Bytes(),
				).Bytes(),
			),
			root,
		)

		s.Equal(len(leaves)*height, len(siblings))
		s.Equal(leaf2, siblings[0])
		s.Equal(leaf1, siblings[1*height])
		s.Equal(leaf4, siblings[2*height])
		s.Equal(leaf3, siblings[3*height])
		s.Equal(leaf6, siblings[4*height])
		s.Equal(leaf5, siblings[5*height])
		s.Equal(leaf8, siblings[6*height])
		s.Equal(leaf7, siblings[7*height])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[0*height+1])
		s.Equal(crypto.Keccak256Hash(leaf3[:], leaf4[:]), siblings[1*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[2*height+1])
		s.Equal(crypto.Keccak256Hash(leaf1[:], leaf2[:]), siblings[3*height+1])
		s.Equal(crypto.Keccak256Hash(leaf7[:], leaf8[:]), siblings[4*height+1])
		s.Equal(crypto.Keccak256Hash(leaf7[:], leaf8[:]), siblings[5*height+1])
		s.Equal(crypto.Keccak256Hash(leaf5[:], leaf6[:]), siblings[6*height+1])
		s.Equal(crypto.Keccak256Hash(leaf5[:], leaf6[:]), siblings[7*height+1])
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				crypto.Keccak256Hash(leaf7[:], leaf8[:]).Bytes(),
			),
			siblings[0*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				crypto.Keccak256Hash(leaf7[:], leaf8[:]).Bytes(),
			),
			siblings[1*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				crypto.Keccak256Hash(leaf7[:], leaf8[:]).Bytes(),
			),
			siblings[2*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf5[:], leaf6[:]).Bytes(),
				crypto.Keccak256Hash(leaf7[:], leaf8[:]).Bytes(),
			),
			siblings[3*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			siblings[4*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			siblings[5*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			siblings[6*height+2],
		)
		s.Equal(
			crypto.Keccak256Hash(
				crypto.Keccak256Hash(leaf1[:], leaf2[:]).Bytes(),
				crypto.Keccak256Hash(leaf3[:], leaf4[:]).Bytes(),
			),
			siblings[7*height+2],
		)

		for idx := range leaves {
			leafSiblings := siblings[idx*height : idx*height+height]
			s.Equal(root, rootFromSiblings(leaves[idx], idx, leafSiblings))
		}
	})
}

// This test was taken from the libcmt suite as a method to compare
// both implementations
func (s *CreateProofsSuite) TestItMatchesMachineImplementation() {
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

func FuzzVerifyProofs(f *testing.F) {
	f.Add(uint(10), uint(10), uint(5), uint(4))
	f.Fuzz(func(t *testing.T, height, leafCount, leafIdx, siblingIdx uint) {
		height = bound(height, 3, 20)
		leafCount = bound(leafCount, 1, 1<<height)
		leafIdx = bound(leafIdx, 0, leafCount-1)
		leaves := generateRandomLeaves(leafCount)
		leavesCopy := make([]model.Hash, len(leaves))
		copy(leavesCopy, leaves)

		root, siblings, err := CreateProofs(leavesCopy, height)
		if err != nil {
			t.Error(err)
		}

		leafSiblings := siblings[leafIdx*height : leafIdx*height+height]
		newRoot := rootFromSiblings(leaves[leafIdx], int(leafIdx), leafSiblings)
		if root != newRoot {
			t.Errorf("expected: %v, got: %v\n", root, newRoot)
		}

		randomLeaf := generateRandomLeaves(1)[0]
		// safety check to avoid a false positive
		if leaves[leafIdx] != randomLeaf {
			newRoot = rootFromSiblings(randomLeaf, int(leafIdx), leafSiblings)
			if root == newRoot {
				t.Errorf("expected root to be different when replacing the leaf")
			}
		}

		siblingIdx = bound(siblingIdx, 0, height-1)
		leafSiblings[siblingIdx] = crypto.Keccak256Hash([]byte("wrong_sibling"))
		newRoot = rootFromSiblings(leaves[leafIdx], int(leafIdx), leafSiblings)
		if root == newRoot {
			t.Errorf("expected root to be different when replacing a sibling")
		}
	})

}

// rootFromSiblings returns a Merkle root hash calculated by hashing a leaf with
// its siblings. It panics if leafIdx is out of bounds.
func rootFromSiblings(leaf model.Hash, leafIdx int, leafSiblings []common.Hash) model.Hash {
	root := leaf
	height := len(leafSiblings)
	for siblingIdx := uint(0); siblingIdx < uint(height); siblingIdx++ {
		if leafIdx&1 == 0 {
			root = crypto.Keccak256Hash(root[:], leafSiblings[siblingIdx][:])
		} else {
			root = crypto.Keccak256Hash(leafSiblings[siblingIdx][:], root[:])
		}
		leafIdx >>= 1
	}
	if leafIdx != 0 {
		panic("index out of bounds")
	}
	return root
}

// bound ensures n is always between the min and max values, inclusive.
// If max < min, it panics.
func bound(n, min, max uint) uint {
	if max < min {
		panic("max should be equal or greater than min")
	}
	return min + (n % (1 + max - min))
}

// generateRandomLeaves generates random byte slices and hash them with
// Keccak256 to return leafCount hashes.
func generateRandomLeaves(leafCount uint) []model.Hash {
	leaves := make([]model.Hash, leafCount)
	leaf := model.Hash{}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for idx := uint(0); idx < leafCount; idx++ {
		r.Read(leaf[:])
		leaves[idx] = crypto.Keccak256Hash(leaf[:])
	}

	return leaves
}
