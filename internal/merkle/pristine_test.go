// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type PristineTreeSuite struct {
	suite.Suite
}

func TestPristineTree(t *testing.T) {
	suite.Run(t, new(PristineTreeSuite))
}

func (s *PristineTreeSuite) TestItCreatesAPristineTree() {
	testCases := []uint{0, 3, 32, 64}
	for _, tc := range testCases {
		s.Run(fmt.Sprintf("Succeeds_when_height_%d", tc), func() {
			s.NotPanics(func() { NewPristineTree(tc) })
		})
	}
}

func (s *PristineTreeSuite) TestItGetsHashAtLevel() {
	expectedHash := "0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"
	tree := NewPristineTree(3)

	hash, err := tree.NodeAt(2)
	s.Require().Nil(err)
	s.Equal(expectedHash, hash.String())
}

func (s *PristineTreeSuite) TestItFailsToGetHashAtInexistentHeight() {
	tree := NewPristineTree(3)

	_, err := tree.NodeAt(4)
	s.Require().NotNil(err)
}
func (s *PristineTreeSuite) TestItCreatesAPristineTreeWithTheCorrectHashes() {
	tree := NewPristineTree(5)

	hash, err := tree.NodeAt(0)
	s.Require().Nil(err)
	s.Equal(
		"0x0eb01ebfc9ed27500cd4dfc979272d1f0913cc9f66540d7e8005811109e1cf2d",
		hash.String(),
	)

	hash, err = tree.NodeAt(1)
	s.Require().Nil(err)
	s.Equal(
		"0xe58769b32a1beaf1ea27375a44095a0d1fb664ce2dd358e7fcbfb78c26a19344",
		hash.String(),
	)

	hash, err = tree.NodeAt(2)
	s.Require().Nil(err)
	s.Equal(
		"0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85",
		hash.String(),
	)

	hash, err = tree.NodeAt(3)
	s.Require().Nil(err)
	s.Equal(
		"0xb4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30",
		hash.String(),
	)

	hash, err = tree.NodeAt(4)
	s.Require().Nil(err)
	s.Equal(
		"0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
		hash.String(),
	)

	hash, err = tree.NodeAt(5)
	s.Require().Nil(err)
	s.Equal(
		"0x0000000000000000000000000000000000000000000000000000000000000000",
		hash.String(),
	)
}
