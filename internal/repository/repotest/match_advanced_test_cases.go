// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"encoding/hex"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

type MatchAdvancedSuite struct {
	BaseSuite
}

func NewMatchAdvancedSuite(factory RepositoryFactory) *MatchAdvancedSuite {
	return &MatchAdvancedSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *MatchAdvancedSuite) createTournamentAndMatch() (
	*SeedResult, common.Address, common.Hash,
) {
	seed := Seed(s.Ctx, s.T(), s.Repo)
	tournAddr := UniqueAddress()
	t := NewTournamentBuilder(seed.App.ID).
		WithEpochIndex(0).WithAddress(tournAddr).Build()
	err := s.Repo.CreateTournament(s.Ctx, seed.App.IApplicationAddress.String(), t)
	s.Require().NoError(err)

	// Create commitments to satisfy match FK constraints
	c1 := NewCommitmentBuilder(seed.App.ID).
		WithEpochIndex(0).WithTournamentAddress(tournAddr).Build()
	err = s.Repo.CreateCommitment(s.Ctx, seed.App.IApplicationAddress.String(), c1)
	s.Require().NoError(err)

	c2 := NewCommitmentBuilder(seed.App.ID).
		WithEpochIndex(0).WithTournamentAddress(tournAddr).Build()
	err = s.Repo.CreateCommitment(s.Ctx, seed.App.IApplicationAddress.String(), c2)
	s.Require().NoError(err)

	matchIDHash := UniqueHash()
	match := NewMatchBuilder(seed.App.ID).
		WithEpochIndex(0).
		WithTournamentAddress(tournAddr).
		WithIDHash(matchIDHash).
		WithCommitmentOne(c1.Commitment).
		WithCommitmentTwo(c2.Commitment).
		Build()
	err = s.Repo.CreateMatch(s.Ctx, seed.App.IApplicationAddress.String(), match)
	s.Require().NoError(err)

	return seed, tournAddr, matchIDHash
}

func (s *MatchAdvancedSuite) TestCreateMatchAdvanced() {
	s.Run("CreatesSuccessfully", func() {
		seed, tournAddr, matchIDHash := s.createTournamentAndMatch()

		ma := NewMatchAdvancedBuilder(seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(tournAddr).
			WithIDHash(matchIDHash).
			Build()

		err := s.Repo.CreateMatchAdvanced(
			s.Ctx, seed.App.IApplicationAddress.String(), ma)
		s.Require().NoError(err)
	})
}

func (s *MatchAdvancedSuite) TestGetMatchAdvanced() {
	s.Run("ExistingMatchAdvanced", func() {
		seed, tournAddr, matchIDHash := s.createTournamentAndMatch()

		ma := NewMatchAdvancedBuilder(seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(tournAddr).
			WithIDHash(matchIDHash).
			Build()

		err := s.Repo.CreateMatchAdvanced(
			s.Ctx, seed.App.IApplicationAddress.String(), ma)
		s.Require().NoError(err)

		got, err := s.Repo.GetMatchAdvanced(
			s.Ctx, seed.App.IApplicationAddress.String(),
			0, tournAddr.String(), matchIDHash.Hex(),
			hex.EncodeToString(ma.OtherParent[:]))
		s.Require().NoError(err)
		s.Equal(ma.IDHash, got.IDHash)
		s.Equal(ma.OtherParent, got.OtherParent)
	})

	s.Run("NotFound", func() {
		seed, tournAddr, matchIDHash := s.createTournamentAndMatch()
		nonExistent := UniqueHash()
		got, err := s.Repo.GetMatchAdvanced(
			s.Ctx, seed.App.IApplicationAddress.String(),
			0, tournAddr.String(), matchIDHash.Hex(),
			hex.EncodeToString(nonExistent[:]))
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *MatchAdvancedSuite) TestListMatchAdvances() {
	s.Run("EmptyResult", func() {
		seed, tournAddr, matchIDHash := s.createTournamentAndMatch()
		advances, total, err := s.Repo.ListMatchAdvances(
			s.Ctx, seed.App.IApplicationAddress.String(),
			0, tournAddr.String(), matchIDHash.Hex(),
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(advances)
		s.Equal(uint64(0), total)
	})

	s.Run("ReturnsAll", func() {
		seed, tournAddr, matchIDHash := s.createTournamentAndMatch()
		for range 3 {
			ma := NewMatchAdvancedBuilder(seed.App.ID).
				WithEpochIndex(0).
				WithTournamentAddress(tournAddr).
				WithIDHash(matchIDHash).
				Build()
			err := s.Repo.CreateMatchAdvanced(
				s.Ctx, seed.App.IApplicationAddress.String(), ma)
			s.Require().NoError(err)
		}

		advances, total, err := s.Repo.ListMatchAdvances(
			s.Ctx, seed.App.IApplicationAddress.String(),
			0, tournAddr.String(), matchIDHash.Hex(),
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(advances, 3)
		s.Equal(uint64(3), total)
	})
}
