// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"context"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

type ClaimerSuite struct {
	BaseSuite
}

func NewClaimerSuite(factory RepositoryFactory) *ClaimerSuite {
	return &ClaimerSuite{BaseSuite: BaseSuite{factory: factory}}
}

// createAppWithClaimComputedEpoch creates an app with one epoch at ClaimComputed status.
func (s *ClaimerSuite) createAppWithClaimComputedEpoch() *Application {
	s.T().Helper()
	app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

	epoch := NewEpochBuilder(app.ID).
		WithIndex(0).WithStatus(EpochStatus_Closed).
		WithBlocks(0, 9).WithInputBounds(0, 0).Build()
	input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

	err := s.Repo.CreateEpochsAndInputs(
		s.Ctx, app.IApplicationAddress.String(),
		map[*Epoch][]*Input{epoch: {input}}, 10)
	s.Require().NoError(err)

	epoch.Status = EpochStatus_ClaimComputed
	err = s.Repo.UpdateEpochStatus(s.Ctx, app.IApplicationAddress.String(), epoch)
	s.Require().NoError(err)

	return app
}

func (s *ClaimerSuite) TestSelectSubmittedClaimPairsPerApp() {
	s.Run("EmptyWhenNoClaimComputed", func() {
		NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// Returns: (acceptedOrSubmitted, computed, applications, error)
		_, computed, apps, err := s.Repo.SelectSubmittedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Empty(computed)
		s.Empty(apps)
	})

	s.Run("ReturnsPairWhenClaimComputed", func() {
		app := s.createAppWithClaimComputedEpoch()

		// Returns: (acceptedOrSubmitted, computed, applications, error)
		_, computed, apps, err := s.Repo.SelectSubmittedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.NotEmpty(computed)
		s.NotEmpty(apps)
		s.Contains(computed, app.ID)
		s.Contains(apps, app.ID)
	})

	s.Run("MultipleAppsReturnsSeparateEntries", func() {
		app1 := s.createAppWithClaimComputedEpoch()
		app2 := s.createAppWithClaimComputedEpoch()

		_, computed, apps, err := s.Repo.SelectSubmittedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Len(computed, 2)
		s.Len(apps, 2)
		s.Contains(computed, app1.ID)
		s.Contains(computed, app2.ID)
		s.Contains(apps, app1.ID)
		s.Contains(apps, app2.ID)
	})

	s.Run("IncludesAcceptedOrSubmittedForMultipleApps", func() {
		// Create two apps, each with a submitted epoch.
		// SelectSubmittedClaimPairsPerApp returns acceptedOrSubmitted
		// via selectNewestAcceptedClaimPerApp(includeSubmitted=true).
		app1 := s.createAppWithClaimComputedEpoch()
		app2 := s.createAppWithClaimComputedEpoch()

		// Move both to ClaimSubmitted
		txHash1 := UniqueHash()
		err := s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app1.ID, 0, txHash1)
		s.Require().NoError(err)

		txHash2 := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app2.ID, 0, txHash2)
		s.Require().NoError(err)

		acceptedOrSubmitted, _, _, err := s.Repo.SelectSubmittedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Len(acceptedOrSubmitted, 2)
		s.Contains(acceptedOrSubmitted, app1.ID)
		s.Contains(acceptedOrSubmitted, app2.ID)
	})

	// Regression guard: verify map keys are actual application IDs
	// and that each epoch is stored under the correct key.
	s.Run("MultiAppMapKeysMatchEpochApplicationIDs", func() {
		app1 := s.createAppWithClaimComputedEpoch()
		app2 := s.createAppWithClaimComputedEpoch()

		_, computed, apps, err := s.Repo.SelectSubmittedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)

		for appID, epoch := range computed {
			s.NotEqual(int64(0), appID, "map key must not be zero")
			s.Equal(appID, epoch.ApplicationID,
				"epoch stored under wrong key")
		}
		for appID, app := range apps {
			s.NotEqual(int64(0), appID, "map key must not be zero")
			s.Equal(appID, app.ID,
				"application stored under wrong key")
		}

		// Verify specific app data integrity
		s.Equal(app1.IApplicationAddress, apps[app1.ID].IApplicationAddress)
		s.Equal(app2.IApplicationAddress, apps[app2.ID].IApplicationAddress)
	})

	s.Run("ExcludesPRTApps", func() {
		app := NewApplicationBuilder().
			WithConsensus(Consensus_PRT).
			Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		epoch.Status = EpochStatus_ClaimComputed
		err = s.Repo.UpdateEpochStatus(s.Ctx, app.IApplicationAddress.String(), epoch)
		s.Require().NoError(err)

		_, computed, apps, err := s.Repo.SelectSubmittedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Empty(computed)
		s.Empty(apps)
	})

	s.Run("ExcludesDisabledApps", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		epoch.Status = EpochStatus_ClaimComputed
		err = s.Repo.UpdateEpochStatus(s.Ctx, app.IApplicationAddress.String(), epoch)
		s.Require().NoError(err)

		reason := "test disabled"
		err = s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Disabled, &reason)
		s.Require().NoError(err)

		_, computed, apps, err := s.Repo.SelectSubmittedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Empty(computed)
		s.Empty(apps)
	})

	s.Run("ContextCancellation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, _, err := s.Repo.SelectSubmittedClaimPairsPerApp(ctx)
		s.Require().Error(err)
	})
}

func (s *ClaimerSuite) TestSelectAcceptedClaimPairsPerApp() {
	s.Run("EmptyWhenNoClaimSubmitted", func() {
		NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		accepted, submitted, apps, err := s.Repo.SelectAcceptedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Empty(accepted)
		s.Empty(submitted)
		s.Empty(apps)
	})

	s.Run("ReturnsPairWhenClaimAccepted", func() {
		app := s.createAppWithClaimComputedEpoch()

		// Move to ClaimSubmitted
		txHash := UniqueHash()
		err := s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
		s.Require().NoError(err)

		// Move to ClaimAccepted
		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0)
		s.Require().NoError(err)

		accepted, _, _, err := s.Repo.SelectAcceptedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Len(accepted, 1)
		s.Contains(accepted, app.ID)
	})

	s.Run("MultipleAppsReturnsSeparateEntries", func() {
		app1 := s.createAppWithClaimComputedEpoch()
		app2 := s.createAppWithClaimComputedEpoch()

		// Move both through submitted -> accepted
		for _, app := range []*Application{app1, app2} {
			txHash := UniqueHash()
			err := s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
			s.Require().NoError(err)
			err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0)
			s.Require().NoError(err)
		}

		accepted, _, _, err := s.Repo.SelectAcceptedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Len(accepted, 2)
		s.Contains(accepted, app1.ID)
		s.Contains(accepted, app2.ID)
	})

	// Regression guard: verify accepted map keys match the actual
	// epoch.ApplicationID, not a zero-value from an unscanned field.
	s.Run("MultiAppMapKeysMatchEpochApplicationIDs", func() {
		app1 := s.createAppWithClaimComputedEpoch()
		app2 := s.createAppWithClaimComputedEpoch()

		for _, app := range []*Application{app1, app2} {
			txHash := UniqueHash()
			err := s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
			s.Require().NoError(err)
			err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0)
			s.Require().NoError(err)
		}

		accepted, submitted, apps, err := s.Repo.SelectAcceptedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)

		for appID, epoch := range accepted {
			s.NotEqual(int64(0), appID, "accepted map key must not be zero")
			s.Equal(appID, epoch.ApplicationID,
				"accepted epoch stored under wrong key")
		}
		for appID, epoch := range submitted {
			s.NotEqual(int64(0), appID, "submitted map key must not be zero")
			s.Equal(appID, epoch.ApplicationID,
				"submitted epoch stored under wrong key")
		}
		for appID, app := range apps {
			s.NotEqual(int64(0), appID, "apps map key must not be zero")
			s.Equal(appID, app.ID,
				"application stored under wrong key")
		}

		s.Contains(accepted, app1.ID)
		s.Contains(accepted, app2.ID)
	})

	s.Run("ExcludesPRTApps", func() {
		app := NewApplicationBuilder().
			WithConsensus(Consensus_PRT).
			Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		epoch.Status = EpochStatus_ClaimComputed
		err = s.Repo.UpdateEpochStatus(s.Ctx, app.IApplicationAddress.String(), epoch)
		s.Require().NoError(err)

		txHash := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
		s.Require().NoError(err)

		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0)
		s.Require().NoError(err)

		accepted, submitted, apps, err := s.Repo.SelectAcceptedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Empty(accepted)
		s.Empty(submitted)
		s.Empty(apps)
	})

	s.Run("ExcludesDisabledApps", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		epoch.Status = EpochStatus_ClaimComputed
		err = s.Repo.UpdateEpochStatus(s.Ctx, app.IApplicationAddress.String(), epoch)
		s.Require().NoError(err)

		txHash := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
		s.Require().NoError(err)

		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0)
		s.Require().NoError(err)

		reason := "test disabled"
		err = s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Disabled, &reason)
		s.Require().NoError(err)

		accepted, submitted, apps, err := s.Repo.SelectAcceptedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Empty(accepted)
		s.Empty(submitted)
		s.Empty(apps)
	})

	s.Run("ReturnsSubmittedMapWithBothEpochStates", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// Create two epochs with inputs
		epoch0 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		epoch1 := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Closed).
			WithBlocks(10, 19).WithInputBounds(1, 1).Build()
		input1 := NewInputBuilder().WithIndex(1).WithBlockNumber(15).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch0: {input0}, epoch1: {input1}}, 20)
		s.Require().NoError(err)

		// Move epoch 0 to ClaimAccepted
		epoch0.Status = EpochStatus_ClaimComputed
		err = s.Repo.UpdateEpochStatus(
			s.Ctx, app.IApplicationAddress.String(), epoch0)
		s.Require().NoError(err)

		txHash0 := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash0)
		s.Require().NoError(err)

		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0)
		s.Require().NoError(err)

		// Move epoch 1 to ClaimSubmitted
		epoch1.Status = EpochStatus_ClaimComputed
		err = s.Repo.UpdateEpochStatus(
			s.Ctx, app.IApplicationAddress.String(), epoch1)
		s.Require().NoError(err)

		txHash1 := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 1, txHash1)
		s.Require().NoError(err)

		accepted, submitted, apps, err := s.Repo.SelectAcceptedClaimPairsPerApp(s.Ctx)
		s.Require().NoError(err)

		// accepted contains epoch 0 (newest accepted)
		s.Len(accepted, 1)
		s.Contains(accepted, app.ID)
		s.Equal(uint64(0), accepted[app.ID].Index)
		s.Equal(EpochStatus_ClaimAccepted, accepted[app.ID].Status)

		// submitted contains epoch 1 (oldest submitted)
		s.Len(submitted, 1)
		s.Contains(submitted, app.ID)
		s.Equal(uint64(1), submitted[app.ID].Index)
		s.Equal(EpochStatus_ClaimSubmitted, submitted[app.ID].Status)

		// apps contains the application
		s.Len(apps, 1)
		s.Contains(apps, app.ID)
		s.Equal(app.IApplicationAddress, apps[app.ID].IApplicationAddress)
	})

	s.Run("ContextCancellation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, _, err := s.Repo.SelectAcceptedClaimPairsPerApp(ctx)
		s.Require().Error(err)
	})
}

func (s *ClaimerSuite) TestUpdateEpochWithSubmittedClaim() {
	s.Run("SetsClaimSubmitted", func() {
		app := s.createAppWithClaimComputedEpoch()

		txHash := common.HexToHash("0xdeadbeef")
		err := s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimSubmitted, got.Status)
		s.Require().NotNil(got.ClaimTransactionHash)
		s.Equal(txHash, *got.ClaimTransactionHash)
	})

	s.Run("ErrorWhenEpochNotClaimComputed", func() {
		// Create an app with an epoch still in Closed status (not ClaimComputed)
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		txHash := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
		s.Require().Error(err)
	})
}

func (s *ClaimerSuite) TestUpdateEpochWithAcceptedClaim() {
	s.Run("SetsClaimAccepted", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		epoch.Status = EpochStatus_ClaimSubmitted
		err = s.Repo.UpdateEpochStatus(s.Ctx, app.IApplicationAddress.String(), epoch)
		s.Require().NoError(err)

		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimAccepted, got.Status)
	})

	s.Run("ErrorWhenEpochNotClaimSubmitted", func() {
		// Create an app with an epoch in ClaimComputed status (not ClaimSubmitted)
		app := s.createAppWithClaimComputedEpoch()

		err := s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0)
		s.Require().Error(err)
	})
}
