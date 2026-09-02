// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

type TournamentSuite struct {
	BaseSuite
}

func NewTournamentSuite(factory RepositoryFactory) *TournamentSuite {
	return &TournamentSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *TournamentSuite) seedWithEpoch() *SeedResult {
	return Seed(s.Ctx, s.T(), s.Repo)
}

func (s *TournamentSuite) TestCreateTournament() {
	s.Run("CreatesSuccessfully", func() {
		seed := s.seedWithEpoch()
		tournament := NewTournamentBuilder(seed.App.ID).
			WithEpochIndex(0).Build()

		err := s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), tournament)
		s.Require().NoError(err)
	})

	s.Run("ExactReplayIsIdempotent", func() {
		seed := s.seedWithEpoch()
		first := NewTournamentBuilder(seed.App.ID).
			WithEpochIndex(0).Build()
		s.Require().NoError(s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), first))

		replay := *first
		replay.MaxLevel++
		err := s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), &replay)
		s.Require().NoError(err)

		got, err := s.Repo.GetTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), first.Address.String())
		s.Require().NoError(err)
		s.Require().NotNil(got)
		s.Equal(first.MaxLevel, got.MaxLevel, "an exact replay must not overwrite the first observation")
	})

	s.Run("DifferentRootForSameEpochIsAnError", func() {
		seed := s.seedWithEpoch()
		first := NewTournamentBuilder(seed.App.ID).
			WithEpochIndex(0).Build()
		s.Require().NoError(s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), first))

		conflicting := NewTournamentBuilder(seed.App.ID).
			WithEpochIndex(0).Build()
		err := s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), conflicting)
		s.Require().Error(err, "only the exact tournament identity may be replayed")
	})
}

func (s *TournamentSuite) TestGetTournament() {
	s.Run("ExistingTournament", func() {
		seed := s.seedWithEpoch()
		tournament := NewTournamentBuilder(seed.App.ID).
			WithEpochIndex(0).Build()

		err := s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), tournament)
		s.Require().NoError(err)

		got, err := s.Repo.GetTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), tournament.Address.String())
		s.Require().NoError(err)
		s.Equal(tournament.Address, got.Address)
		s.Equal(tournament.Level, got.Level)
	})

	s.Run("NotFound", func() {
		seed := s.seedWithEpoch()
		got, err := s.Repo.GetTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), UniqueAddress().String())
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *TournamentSuite) TestUpdateTournament() {
	s.Run("UpdatesFields", func() {
		seed := s.seedWithEpoch()
		tournament := NewTournamentBuilder(seed.App.ID).
			WithEpochIndex(0).Build()

		err := s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), tournament)
		s.Require().NoError(err)

		winnerHash := UniqueHash()
		tournament.WinnerCommitment = &winnerHash
		err = s.Repo.UpdateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), tournament)
		s.Require().NoError(err)

		got, err := s.Repo.GetTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), tournament.Address.String())
		s.Require().NoError(err)
		s.Require().NotNil(got.WinnerCommitment)
		s.Equal(winnerHash, *got.WinnerCommitment)
	})
}

func (s *TournamentSuite) TestListTournaments() {
	s.Run("EmptyResult", func() {
		seed := s.seedWithEpoch()
		tournaments, total, err := s.Repo.ListTournaments(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.TournamentFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(tournaments)
		s.Equal(uint64(0), total)
	})

	s.Run("ReturnsAll", func() {
		// Create 3 root tournaments in different epochs
		// (unique_root_per_epoch_idx allows only 1 root per epoch)
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epochInputMap := make(map[*Epoch][]*Input)
		for i := range uint64(3) {
			e := NewEpochBuilder(app.ID).
				WithIndex(i).WithStatus(EpochStatus_Closed).
				WithBlocks(i*10, i*10+9).WithInputBounds(i, i).Build()
			inp := NewInputBuilder().WithIndex(i).WithEpochIndex(i).
				WithBlockNumber(i*10 + 5).Build()
			epochInputMap[e] = []*Input{inp}
		}
		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(), epochInputMap, 30)
		s.Require().NoError(err)

		for i := range uint64(3) {
			t := NewTournamentBuilder(app.ID).WithEpochIndex(i).Build()
			err := s.Repo.CreateTournament(
				s.Ctx, app.IApplicationAddress.String(), t)
			s.Require().NoError(err)
		}

		tournaments, total, err := s.Repo.ListTournaments(
			s.Ctx, app.IApplicationAddress.String(),
			repository.TournamentFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(tournaments, 3)
		s.Equal(uint64(3), total)
	})

	s.Run("FilterByEpochIndex", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch0 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		epoch1 := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Closed).
			WithBlocks(10, 19).WithInputBounds(1, 1).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithEpochIndex(1).WithBlockNumber(15).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch0: {input0}, epoch1: {input1}}, 20)
		s.Require().NoError(err)

		t0 := NewTournamentBuilder(app.ID).WithEpochIndex(0).Build()
		err = s.Repo.CreateTournament(s.Ctx, app.IApplicationAddress.String(), t0)
		s.Require().NoError(err)

		t1 := NewTournamentBuilder(app.ID).WithEpochIndex(1).Build()
		err = s.Repo.CreateTournament(s.Ctx, app.IApplicationAddress.String(), t1)
		s.Require().NoError(err)

		epochIdx := uint64(0)
		tournaments, total, err := s.Repo.ListTournaments(
			s.Ctx, app.IApplicationAddress.String(),
			repository.TournamentFilter{EpochIndex: &epochIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(tournaments, 1)
		s.Equal(uint64(1), total)
	})

	s.Run("FilterByLevel", func() {
		seed := s.seedWithEpoch()

		// Root tournament at level 0
		t0 := NewTournamentBuilder(seed.App.ID).
			WithEpochIndex(0).WithLevel(0).Build()
		err := s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), t0)
		s.Require().NoError(err)

		// Create commitments required by match FK constraints
		c1 := NewCommitmentBuilder(seed.App.ID).
			WithEpochIndex(0).WithTournamentAddress(t0.Address).Build()
		err = s.Repo.CreateCommitment(
			s.Ctx, seed.App.IApplicationAddress.String(), c1)
		s.Require().NoError(err)

		c2 := NewCommitmentBuilder(seed.App.ID).
			WithEpochIndex(0).WithTournamentAddress(t0.Address).Build()
		err = s.Repo.CreateCommitment(
			s.Ctx, seed.App.IApplicationAddress.String(), c2)
		s.Require().NoError(err)

		// Create a match in the root tournament so the FK constraint is satisfied
		matchIDHash := UniqueHash()
		match := NewMatchBuilder(seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(t0.Address).
			WithIDHash(matchIDHash).
			WithCommitmentOne(c1.Commitment).
			WithCommitmentTwo(c2.Commitment).
			Build()
		err = s.Repo.CreateMatch(
			s.Ctx, seed.App.IApplicationAddress.String(), match)
		s.Require().NoError(err)

		// Sub-tournament at level 1 with parent referencing the match
		t1 := NewTournamentBuilder(seed.App.ID).
			WithEpochIndex(0).WithLevel(1).
			WithParent(t0.Address, matchIDHash).Build()
		err = s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), t1)
		s.Require().NoError(err)

		level := uint64(1)
		tournaments, total, err := s.Repo.ListTournaments(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.TournamentFilter{Level: &level},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(tournaments, 1)
		s.Equal(uint64(1), total)
		s.Equal(uint64(1), tournaments[0].Level)
	})

	s.Run("FilterByParentTournamentAddress", func() {
		seed := s.seedWithEpoch()

		// Root tournament
		rootAddr := UniqueAddress()
		t0 := NewTournamentBuilder(seed.App.ID).
			WithEpochIndex(0).WithAddress(rootAddr).Build()
		err := s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), t0)
		s.Require().NoError(err)

		// Create commitments required by match FK constraints
		c1 := NewCommitmentBuilder(seed.App.ID).
			WithEpochIndex(0).WithTournamentAddress(rootAddr).Build()
		err = s.Repo.CreateCommitment(
			s.Ctx, seed.App.IApplicationAddress.String(), c1)
		s.Require().NoError(err)

		c2 := NewCommitmentBuilder(seed.App.ID).
			WithEpochIndex(0).WithTournamentAddress(rootAddr).Build()
		err = s.Repo.CreateCommitment(
			s.Ctx, seed.App.IApplicationAddress.String(), c2)
		s.Require().NoError(err)

		// Create a match in the root tournament so the FK constraint is satisfied
		matchIDHash := UniqueHash()
		match := NewMatchBuilder(seed.App.ID).
			WithEpochIndex(0).
			WithTournamentAddress(rootAddr).
			WithIDHash(matchIDHash).
			WithCommitmentOne(c1.Commitment).
			WithCommitmentTwo(c2.Commitment).
			Build()
		err = s.Repo.CreateMatch(
			s.Ctx, seed.App.IApplicationAddress.String(), match)
		s.Require().NoError(err)

		// Sub-tournament with parent referencing the match
		t1 := NewTournamentBuilder(seed.App.ID).
			WithEpochIndex(0).WithLevel(1).
			WithParent(rootAddr, matchIDHash).Build()
		err = s.Repo.CreateTournament(
			s.Ctx, seed.App.IApplicationAddress.String(), t1)
		s.Require().NoError(err)

		tournaments, total, err := s.Repo.ListTournaments(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.TournamentFilter{ParentTournamentAddress: &rootAddr},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(tournaments, 1)
		s.Equal(uint64(1), total)
		s.Require().NotNil(tournaments[0].ParentTournamentAddress)
		s.Equal(rootAddr, *tournaments[0].ParentTournamentAddress)
	})
}
