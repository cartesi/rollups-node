// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

type CommitmentSuite struct {
	BaseSuite
}

func NewCommitmentSuite(factory RepositoryFactory) *CommitmentSuite {
	return &CommitmentSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *CommitmentSuite) createTournament() (*SeedResult, common.Address) {
	seed := Seed(s.Ctx, s.T(), s.Repo)
	tournAddr := UniqueAddress()
	t := NewTournamentBuilder(seed.App.ID).
		WithEpochIndex(0).WithAddress(tournAddr).Build()
	err := s.Repo.CreateTournament(s.Ctx, seed.App.IApplicationAddress.String(), t)
	s.Require().NoError(err)
	return seed, tournAddr
}

func (s *CommitmentSuite) TestCreateCommitment() {
	s.Run("CreatesSuccessfully", func() {
		seed, tournAddr := s.createTournament()

		c := NewCommitmentBuilder(seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(tournAddr).
			Build()

		err := s.Repo.CreateCommitment(
			s.Ctx, seed.App.IApplicationAddress.String(), c)
		s.Require().NoError(err)
	})
}

func (s *CommitmentSuite) TestGetCommitment() {
	s.Run("ExistingCommitment", func() {
		seed, tournAddr := s.createTournament()

		c := NewCommitmentBuilder(seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(tournAddr).
			Build()

		err := s.Repo.CreateCommitment(
			s.Ctx, seed.App.IApplicationAddress.String(), c)
		s.Require().NoError(err)

		got, err := s.Repo.GetCommitment(
			s.Ctx, seed.App.IApplicationAddress.String(),
			0, tournAddr.String(), c.Commitment.Hex())
		s.Require().NoError(err)
		s.Equal(c.Commitment, got.Commitment)
		s.Equal(c.FinalStateHash, got.FinalStateHash)
	})

	s.Run("NotFound", func() {
		seed, tournAddr := s.createTournament()

		got, err := s.Repo.GetCommitment(
			s.Ctx, seed.App.IApplicationAddress.String(),
			0, tournAddr.String(), UniqueHash().Hex())
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *CommitmentSuite) TestListCommitments() {
	s.Run("EmptyResult", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		commitments, total, err := s.Repo.ListCommitments(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.CommitmentFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(commitments)
		s.Equal(uint64(0), total)
	})

	s.Run("ReturnsAll", func() {
		seed, tournAddr := s.createTournament()
		for range 3 {
			c := NewCommitmentBuilder(seed.App.ID).
				WithEpochIndex(0).
				WithTournamentAddress(tournAddr).
				Build()
			err := s.Repo.CreateCommitment(
				s.Ctx, seed.App.IApplicationAddress.String(), c)
			s.Require().NoError(err)
		}

		commitments, total, err := s.Repo.ListCommitments(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.CommitmentFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(commitments, 3)
		s.Equal(uint64(3), total)
	})

	s.Run("FilterByEpochIndex", func() {
		seed, tournAddr := s.createTournament()

		c := NewCommitmentBuilder(seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(tournAddr).
			Build()
		err := s.Repo.CreateCommitment(
			s.Ctx, seed.App.IApplicationAddress.String(), c)
		s.Require().NoError(err)

		epochIdx := uint64(0)
		commitments, total, err := s.Repo.ListCommitments(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.CommitmentFilter{EpochIndex: &epochIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(commitments, 1)
		s.Equal(uint64(1), total)
	})

	s.Run("FilterByTournamentAddress", func() {
		seed, tournAddr := s.createTournament()

		c := NewCommitmentBuilder(seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(tournAddr).
			Build()
		err := s.Repo.CreateCommitment(
			s.Ctx, seed.App.IApplicationAddress.String(), c)
		s.Require().NoError(err)

		addrStr := tournAddr.String()
		commitments, total, err := s.Repo.ListCommitments(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.CommitmentFilter{TournamentAddress: &addrStr},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(commitments, 1)
		s.Equal(uint64(1), total)
	})
}
