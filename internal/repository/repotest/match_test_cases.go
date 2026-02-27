// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

type MatchSuite struct {
	BaseSuite
}

func NewMatchSuite(factory RepositoryFactory) *MatchSuite {
	return &MatchSuite{BaseSuite: BaseSuite{factory: factory}}
}

// matchSetup holds a tournament and two commitments needed for match FK constraints.
type matchSetup struct {
	seed        *SeedResult
	tournAddr   common.Address
	commitHash1 common.Hash
	commitHash2 common.Hash
}

func (s *MatchSuite) setupTournamentWithCommitments() *matchSetup {
	seed := Seed(s.Ctx, s.T(), s.Repo)
	tournAddr := UniqueAddress()
	t := NewTournamentBuilder(seed.App.ID).
		WithEpochIndex(0).WithAddress(tournAddr).Build()
	err := s.Repo.CreateTournament(s.Ctx, seed.App.IApplicationAddress.String(), t)
	s.Require().NoError(err)

	c1 := NewCommitmentBuilder(seed.App.ID).
		WithEpochIndex(0).WithTournamentAddress(tournAddr).Build()
	err = s.Repo.CreateCommitment(s.Ctx, seed.App.IApplicationAddress.String(), c1)
	s.Require().NoError(err)

	c2 := NewCommitmentBuilder(seed.App.ID).
		WithEpochIndex(0).WithTournamentAddress(tournAddr).Build()
	err = s.Repo.CreateCommitment(s.Ctx, seed.App.IApplicationAddress.String(), c2)
	s.Require().NoError(err)

	return &matchSetup{
		seed:        seed,
		tournAddr:   tournAddr,
		commitHash1: c1.Commitment,
		commitHash2: c2.Commitment,
	}
}

func (s *MatchSuite) TestCreateMatch() {
	s.Run("CreatesSuccessfully", func() {
		setup := s.setupTournamentWithCommitments()
		match := NewMatchBuilder(setup.seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(setup.tournAddr).
			WithCommitmentOne(setup.commitHash1).
			WithCommitmentTwo(setup.commitHash2).
			Build()

		err := s.Repo.CreateMatch(
			s.Ctx, setup.seed.App.IApplicationAddress.String(), match)
		s.Require().NoError(err)
	})
}

func (s *MatchSuite) TestGetMatch() {
	s.Run("ExistingMatch", func() {
		setup := s.setupTournamentWithCommitments()
		match := NewMatchBuilder(setup.seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(setup.tournAddr).
			WithCommitmentOne(setup.commitHash1).
			WithCommitmentTwo(setup.commitHash2).
			Build()

		err := s.Repo.CreateMatch(
			s.Ctx, setup.seed.App.IApplicationAddress.String(), match)
		s.Require().NoError(err)

		got, err := s.Repo.GetMatch(
			s.Ctx, setup.seed.App.IApplicationAddress.String(),
			0, setup.tournAddr.String(), match.IDHash.Hex())
		s.Require().NoError(err)
		s.Equal(match.IDHash, got.IDHash)
		s.Equal(match.CommitmentOne, got.CommitmentOne)
	})

	s.Run("NotFound", func() {
		setup := s.setupTournamentWithCommitments()
		got, err := s.Repo.GetMatch(
			s.Ctx, setup.seed.App.IApplicationAddress.String(),
			0, setup.tournAddr.String(), UniqueHash().Hex())
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *MatchSuite) TestUpdateMatch() {
	s.Run("UpdatesWinner", func() {
		setup := s.setupTournamentWithCommitments()
		match := NewMatchBuilder(setup.seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(setup.tournAddr).
			WithCommitmentOne(setup.commitHash1).
			WithCommitmentTwo(setup.commitHash2).
			Build()

		err := s.Repo.CreateMatch(
			s.Ctx, setup.seed.App.IApplicationAddress.String(), match)
		s.Require().NoError(err)

		match.Winner = WinnerCommitment_ONE
		err = s.Repo.UpdateMatch(
			s.Ctx, setup.seed.App.IApplicationAddress.String(), match)
		s.Require().NoError(err)

		got, err := s.Repo.GetMatch(
			s.Ctx, setup.seed.App.IApplicationAddress.String(),
			0, setup.tournAddr.String(), match.IDHash.Hex())
		s.Require().NoError(err)
		s.Equal(WinnerCommitment_ONE, got.Winner)
	})
}

func (s *MatchSuite) TestListMatches() {
	s.Run("EmptyResult", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		matches, total, err := s.Repo.ListMatches(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.MatchFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(matches)
		s.Equal(uint64(0), total)
	})

	s.Run("ReturnsAll", func() {
		setup := s.setupTournamentWithCommitments()

		// Each match needs unique commitment pairs (matches_unique_pair_idx).
		// Create additional commitments for more matches.
		for range 3 {
			c1 := NewCommitmentBuilder(setup.seed.App.ID).
				WithEpochIndex(0).WithTournamentAddress(setup.tournAddr).Build()
			err := s.Repo.CreateCommitment(
				s.Ctx, setup.seed.App.IApplicationAddress.String(), c1)
			s.Require().NoError(err)

			c2 := NewCommitmentBuilder(setup.seed.App.ID).
				WithEpochIndex(0).WithTournamentAddress(setup.tournAddr).Build()
			err = s.Repo.CreateCommitment(
				s.Ctx, setup.seed.App.IApplicationAddress.String(), c2)
			s.Require().NoError(err)

			m := NewMatchBuilder(setup.seed.App.ID).
				WithEpochIndex(0).
				WithTournamentAddress(setup.tournAddr).
				WithCommitmentOne(c1.Commitment).
				WithCommitmentTwo(c2.Commitment).
				Build()
			err = s.Repo.CreateMatch(
				s.Ctx, setup.seed.App.IApplicationAddress.String(), m)
			s.Require().NoError(err)
		}

		matches, total, err := s.Repo.ListMatches(
			s.Ctx, setup.seed.App.IApplicationAddress.String(),
			repository.MatchFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(matches, 3)
		s.Equal(uint64(3), total)
	})

	s.Run("FilterByEpochIndex", func() {
		setup := s.setupTournamentWithCommitments()
		m := NewMatchBuilder(setup.seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(setup.tournAddr).
			WithCommitmentOne(setup.commitHash1).
			WithCommitmentTwo(setup.commitHash2).
			Build()
		err := s.Repo.CreateMatch(
			s.Ctx, setup.seed.App.IApplicationAddress.String(), m)
		s.Require().NoError(err)

		epochIdx := uint64(0)
		matches, total, err := s.Repo.ListMatches(
			s.Ctx, setup.seed.App.IApplicationAddress.String(),
			repository.MatchFilter{EpochIndex: &epochIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(matches, 1)
		s.Equal(uint64(1), total)
	})

	s.Run("FilterByTournamentAddress", func() {
		setup := s.setupTournamentWithCommitments()
		m := NewMatchBuilder(setup.seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(setup.tournAddr).
			WithCommitmentOne(setup.commitHash1).
			WithCommitmentTwo(setup.commitHash2).
			Build()
		err := s.Repo.CreateMatch(
			s.Ctx, setup.seed.App.IApplicationAddress.String(), m)
		s.Require().NoError(err)

		addrStr := setup.tournAddr.String()
		matches, total, err := s.Repo.ListMatches(
			s.Ctx, setup.seed.App.IApplicationAddress.String(),
			repository.MatchFilter{TournamentAddress: &addrStr},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(matches, 1)
		s.Equal(uint64(1), total)
	})
}
