// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"context"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
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
	return s.createAppWithClaimComputedEpochForConsensus(Consensus_Authority)
}

func (s *ClaimerSuite) createAppWithClaimComputedEpochForConsensus(c Consensus) *Application {
	s.T().Helper()
	app := NewApplicationBuilder().WithConsensus(c).Create(s.Ctx, s.T(), s.Repo)

	epoch := NewEpochBuilder(app.ID).
		WithIndex(0).WithStatus(EpochStatus_Closed).
		WithBlocks(0, 9).WithInputBounds(0, 0).Build()
	input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

	err := s.Repo.CreateEpochsAndInputs(
		s.Ctx, app.IApplicationAddress.String(),
		map[*Epoch][]*Input{epoch: {input}}, 10)
	s.Require().NoError(err)

	AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
		app.IApplicationAddress.String(), epoch, EpochStatus_ClaimComputed)

	return app
}

func (s *ClaimerSuite) TestSelectClaimsToSubmitPerApp() {
	s.Run("EmptyWhenNoClaimComputed", func() {
		NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// Returns: (acceptedOrSubmitted, computed, applications, error)
		_, computed, apps, err := s.Repo.SelectClaimsToSubmitPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Empty(computed)
		s.Empty(apps)
	})

	s.Run("ReturnsPairWhenClaimComputed", func() {
		app := s.createAppWithClaimComputedEpoch()

		// Returns: (acceptedOrSubmitted, computed, applications, error)
		_, computed, apps, err := s.Repo.SelectClaimsToSubmitPerApp(s.Ctx)
		s.Require().NoError(err)
		s.NotEmpty(computed)
		s.NotEmpty(apps)
		s.Contains(computed, app.ID)
		s.Contains(apps, app.ID)
	})

	s.Run("IncludesForeclosedComputedAppForTerminalization", func() {
		app := s.createAppWithClaimComputedEpoch()
		err := s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, 100, UniqueHash(), 100)
		s.Require().NoError(err)

		_, computed, apps, err := s.Repo.SelectClaimsToSubmitPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Require().Contains(computed, app.ID)
		s.Equal(EpochStatus_ClaimComputed, computed[app.ID].Status)
		s.Require().Contains(apps, app.ID)
		s.NotZero(apps[app.ID].ForecloseBlock)
	})

	s.Run("MultipleAppsReturnsSeparateEntries", func() {
		app1 := s.createAppWithClaimComputedEpoch()
		app2 := s.createAppWithClaimComputedEpoch()

		_, computed, apps, err := s.Repo.SelectClaimsToSubmitPerApp(s.Ctx)
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
		// SelectClaimsToSubmitPerApp returns the submit barriers:
		// accepted, submitted, and staged predecessors.
		app1 := s.createAppWithClaimComputedEpoch()
		app2 := s.createAppWithClaimComputedEpoch()

		// Move both to ClaimSubmitted
		txHash1 := UniqueHash()
		err := s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app1.ID, 0, txHash1)
		s.Require().NoError(err)

		txHash2 := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app2.ID, 0, txHash2)
		s.Require().NoError(err)

		acceptedOrSubmitted, _, _, err := s.Repo.SelectClaimsToSubmitPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Len(acceptedOrSubmitted, 2)
		s.Contains(acceptedOrSubmitted, app1.ID)
		s.Contains(acceptedOrSubmitted, app2.ID)
	})

	s.Run("IncludesStagedPredecessorAsSubmitBarrier", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

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

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch0, EpochStatus_ClaimComputed)

		txHash := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
		s.Require().NoError(err)

		err = s.Repo.UpdateEpochToStaged(s.Ctx, app.ID, 0, 30)
		s.Require().NoError(err)

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch1, EpochStatus_ClaimComputed)

		barriers, computed, apps, err := s.Repo.SelectClaimsToSubmitPerApp(s.Ctx)
		s.Require().NoError(err)

		s.Require().Contains(barriers, app.ID)
		s.Equal(uint64(0), barriers[app.ID].Index)
		s.Equal(EpochStatus_ClaimStaged, barriers[app.ID].Status)

		s.Require().Contains(computed, app.ID)
		s.Equal(uint64(1), computed[app.ID].Index)
		s.Contains(apps, app.ID)
	})

	// Regression guard: verify map keys are actual application IDs
	// and that each epoch is stored under the correct key.
	s.Run("MultiAppMapKeysMatchEpochApplicationIDs", func() {
		app1 := s.createAppWithClaimComputedEpoch()
		app2 := s.createAppWithClaimComputedEpoch()

		_, computed, apps, err := s.Repo.SelectClaimsToSubmitPerApp(s.Ctx)
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

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch, EpochStatus_ClaimComputed)

		_, computed, apps, err := s.Repo.SelectClaimsToSubmitPerApp(s.Ctx)
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

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch, EpochStatus_ClaimComputed)

		err = s.Repo.UpdateApplicationEnabled(s.Ctx, app.ID, false)
		s.Require().NoError(err)

		_, computed, apps, err := s.Repo.SelectClaimsToSubmitPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Empty(computed)
		s.Empty(apps)
	})

	s.Run("ContextCancellation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, _, err := s.Repo.SelectClaimsToSubmitPerApp(ctx)
		s.Require().Error(err)
	})
}

func (s *ClaimerSuite) TestSelectClaimsToStagePerApp() {
	s.Run("EmptyWhenNoClaimSubmitted", func() {
		NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		accepted, submitted, apps, err := s.Repo.SelectClaimsToStagePerApp(s.Ctx)
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
		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, nil)
		s.Require().NoError(err)

		accepted, _, _, err := s.Repo.SelectClaimsToStagePerApp(s.Ctx)
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
			err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, nil)
			s.Require().NoError(err)
		}

		accepted, _, _, err := s.Repo.SelectClaimsToStagePerApp(s.Ctx)
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
			err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, nil)
			s.Require().NoError(err)
		}

		accepted, submitted, apps, err := s.Repo.SelectClaimsToStagePerApp(s.Ctx)
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

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch, EpochStatus_ClaimComputed)

		txHash := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
		s.Require().NoError(err)

		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, nil)
		s.Require().NoError(err)

		accepted, submitted, apps, err := s.Repo.SelectClaimsToStagePerApp(s.Ctx)
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

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch, EpochStatus_ClaimComputed)

		txHash := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
		s.Require().NoError(err)

		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, nil)
		s.Require().NoError(err)

		err = s.Repo.UpdateApplicationEnabled(s.Ctx, app.ID, false)
		s.Require().NoError(err)

		accepted, submitted, apps, err := s.Repo.SelectClaimsToStagePerApp(s.Ctx)
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
		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch0, EpochStatus_ClaimComputed)

		txHash0 := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash0)
		s.Require().NoError(err)

		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, nil)
		s.Require().NoError(err)

		// Move epoch 1 to ClaimSubmitted
		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch1, EpochStatus_ClaimComputed)

		txHash1 := UniqueHash()
		err = s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 1, txHash1)
		s.Require().NoError(err)

		accepted, submitted, apps, err := s.Repo.SelectClaimsToStagePerApp(s.Ctx)
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

	s.Run("IncludesForeclosedSubmittedAppForTerminalization", func() {
		app := s.createAppWithClaimComputedEpoch()
		txHash := UniqueHash()
		err := s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
		s.Require().NoError(err)
		err = s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, 100, UniqueHash(), 100)
		s.Require().NoError(err)

		accepted, submitted, apps, err := s.Repo.SelectClaimsToStagePerApp(s.Ctx)
		s.Require().NoError(err)
		s.Empty(accepted)
		s.Require().Contains(submitted, app.ID)
		s.Equal(EpochStatus_ClaimSubmitted, submitted[app.ID].Status)
		s.Require().Contains(apps, app.ID)
		s.NotZero(apps[app.ID].ForecloseBlock)
	})

	s.Run("ContextCancellation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, _, err := s.Repo.SelectClaimsToStagePerApp(ctx)
		s.Require().Error(err)
	})
}

func (s *ClaimerSuite) TestSelectClaimsToAcceptPerApp() {
	s.Run("IncludesForeclosedStagedAppForTerminalization", func() {
		app := s.createAppWithClaimComputedEpoch()
		txHash := UniqueHash()
		err := s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash)
		s.Require().NoError(err)
		err = s.Repo.UpdateEpochToStaged(s.Ctx, app.ID, 0, 42)
		s.Require().NoError(err)
		err = s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, 100, UniqueHash(), 100)
		s.Require().NoError(err)

		accepted, staged, apps, err := s.Repo.SelectClaimsToAcceptPerApp(s.Ctx)
		s.Require().NoError(err)
		s.Empty(accepted)
		s.Require().Contains(staged, app.ID)
		s.Equal(EpochStatus_ClaimStaged, staged[app.ID].Status)
		s.Require().Contains(apps, app.ID)
		s.NotZero(apps[app.ID].ForecloseBlock)
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

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch, EpochStatus_ClaimSubmitted)

		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimAccepted, got.Status)
	})

	s.Run("ComputedToAcceptedIsAllowed", func() {
		// In v3, CLAIM_COMPUTED → CLAIM_ACCEPTED is a legal transition
		// (deep reader-mode catch-up and PRT). The trigger permits it and
		// UpdateEpochWithAcceptedClaim's WHERE clause accepts COMPUTED as
		// a valid source. This test pins that behavior.
		app := s.createAppWithClaimComputedEpoch()

		err := s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimAccepted, got.Status)
	})

	s.Run("ComputedToAcceptedWithNilTxHashLeavesColumnNull", func() {
		app := s.createAppWithClaimComputedEpoch()

		err := s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimAccepted, got.Status)
		s.Nil(got.ClaimTransactionHash,
			"getClaim-driven COMPUTED -> ACCEPTED reconciliation has no event tx hash to record")
	})

	// Catch-up reconciliation path: an epoch coming from CLAIM_COMPUTED
	// (the read-only scan caught a matching ClaimAccepted on chain) needs
	// to record the observed event's tx hash, because the epoch never went
	// through the CLAIM_SUBMITTED transition that normally populates the
	// column. Pass a non-nil txHash and assert it lands on the row.
	s.Run("ComputedToAcceptedRecordsTxHashWhenProvided", func() {
		app := s.createAppWithClaimComputedEpoch()
		txHash := UniqueHash()

		err := s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, &txHash)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimAccepted, got.Status)
		s.Require().NotNil(got.ClaimTransactionHash,
			"catch-up reconciliation with a known event tx must populate claim_transaction_hash")
		s.Equal(txHash, *got.ClaimTransactionHash)
	})

	// Symmetric to the above for the normal flow: when txHash is nil,
	// claim_transaction_hash is left untouched. The submit-flow caller
	// relies on this: the column was set during CLAIM_SUBMITTED and must
	// carry through the CLAIM_STAGED → CLAIM_ACCEPTED steps unchanged.
	s.Run("NilTxHashPreservesExistingColumn", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		// Drive through INPUTS_PROCESSED → CLAIM_COMPUTED (which seeds the
		// proof fields via the test helper) then submit via the real
		// repository method that records the submit-tx hash. This mirrors
		// the production submit flow exactly.
		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch, EpochStatus_ClaimComputed)
		submitTx := UniqueHash()
		s.Require().NoError(s.Repo.UpdateEpochWithSubmittedClaim(
			s.Ctx, app.ID, 0, submitTx))

		err = s.Repo.UpdateEpochWithAcceptedClaim(s.Ctx, app.ID, 0, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimAccepted, got.Status)
		s.Require().NotNil(got.ClaimTransactionHash,
			"nil txHash must NOT clear an existing claim_transaction_hash")
		s.Equal(submitTx, *got.ClaimTransactionHash,
			"the value seeded during CLAIM_SUBMITTED must carry through to CLAIM_ACCEPTED")
	})
}

func (s *ClaimerSuite) TestRejectEpochAndSetApplicationDiverged() {
	assertRejected := func(app *Application, reason string) {
		gotEpoch, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimRejected, gotEpoch.Status)

		gotApp, err := s.Repo.GetApplication(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Diverged, gotApp.Status)
		s.Require().NotNil(gotApp.Reason)
		s.Equal(reason, *gotApp.Reason)
	}

	s.Run("RejectsComputedEpoch", func() {
		app := s.createAppWithClaimComputedEpoch()
		reason := "quorum_divergence_at_acceptance: rejected computed epoch"

		result, err := s.Repo.RejectEpochAndSetApplicationDiverged(s.Ctx, app.ID, 0, reason)
		s.Require().NoError(err)
		s.True(result.EpochRejected)
		s.True(result.ApplicationDiverged)

		assertRejected(app, reason)
	})

	s.Run("RejectsSubmittedEpoch", func() {
		app := s.createAppWithClaimComputedEpoch()
		err := s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, UniqueHash())
		s.Require().NoError(err)

		reason := "quorum_divergence_at_staging: rejected submitted epoch"
		result, err := s.Repo.RejectEpochAndSetApplicationDiverged(s.Ctx, app.ID, 0, reason)
		s.Require().NoError(err)
		s.True(result.EpochRejected)
		s.True(result.ApplicationDiverged)

		assertRejected(app, reason)
	})

	for _, terminalStatus := range []ApplicationStatus{
		ApplicationStatus_MachineHalted,
		ApplicationStatus_Corrupted,
	} {
		s.Run("Preserves"+terminalStatus.String()+"WhileRejectingEpoch", func() {
			app := s.createAppWithClaimComputedEpoch()
			originalReason := "earlier terminal cause"
			s.Require().NoError(s.Repo.UpdateApplicationStatus(
				s.Ctx, app.ID, terminalStatus, &originalReason))

			result, err := s.Repo.RejectEpochAndSetApplicationDiverged(
				s.Ctx, app.ID, 0, "later claim disagreement")
			s.Require().NoError(err)
			s.True(result.EpochRejected)
			s.False(result.ApplicationDiverged)

			gotEpoch, err := s.Repo.GetEpoch(
				s.Ctx, app.IApplicationAddress.String(), 0)
			s.Require().NoError(err)
			s.Equal(EpochStatus_ClaimRejected, gotEpoch.Status)

			gotApp, err := s.Repo.GetApplication(
				s.Ctx, app.IApplicationAddress.String())
			s.Require().NoError(err)
			s.Equal(terminalStatus, gotApp.Status)
			s.Require().NotNil(gotApp.Reason)
			s.Equal(originalReason, *gotApp.Reason)
		})
	}

	// A CLAIM_STAGED epoch is outside the COMPUTED/SUBMITTED set, so the
	// best-effort epoch reject matches no row and the epoch stays CLAIM_STAGED.
	// The application halt is unconditional: it still becomes DIVERGED so a
	// detected divergence can never leave the application runnable.
	s.Run("MarksDivergedButLeavesStagedEpoch", func() {
		app := s.createAppWithClaimComputedEpoch()
		err := s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, UniqueHash())
		s.Require().NoError(err)
		err = s.Repo.UpdateEpochToStaged(s.Ctx, app.ID, 0, 42)
		s.Require().NoError(err)

		reason := "quorum_divergence_at_acceptance: staged epoch is not a normal rejection source"
		result, err := s.Repo.RejectEpochAndSetApplicationDiverged(s.Ctx, app.ID, 0, reason)
		s.Require().NoError(err)
		s.False(result.EpochRejected)
		s.True(result.ApplicationDiverged)

		gotEpoch, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimStaged, gotEpoch.Status)

		gotApp, err := s.Repo.GetApplication(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Diverged, gotApp.Status)
		s.Require().NotNil(gotApp.Reason)
		s.Equal(reason, *gotApp.Reason)
	})

	// The epoch reject is best-effort: even an epoch that cannot be rejected
	// (here CLOSED) does not block the unconditional application halt.
	s.Run("MarksApplicationEvenWhenEpochCannotBeRejected", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		reason := "divergence detected against a non-rejectable epoch"
		result, err := s.Repo.RejectEpochAndSetApplicationDiverged(
			s.Ctx, app.ID, 0, reason)
		s.Require().NoError(err)
		s.False(result.EpochRejected)
		s.True(result.ApplicationDiverged)

		gotEpoch, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_Closed, gotEpoch.Status)

		gotApp, err := s.Repo.GetApplication(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Diverged, gotApp.Status)
		s.Require().NotNil(gotApp.Reason)
		s.Equal(reason, *gotApp.Reason)
	})

	// A missing application row surfaces ErrNotFound, distinguishing a genuine
	// "no such app" from the best-effort epoch reject matching no row.
	s.Run("ReturnsNotFoundWhenApplicationMissing", func() {
		_, err := s.Repo.RejectEpochAndSetApplicationDiverged(
			s.Ctx, int64(99_999_999), 0, "missing application")
		s.Require().ErrorIs(err, repository.ErrNotFound)
	})
}

func (s *ClaimerSuite) TestUpdateEpochWithForeclosedClaim() {
	markForeclosed := func(app *Application) {
		s.T().Helper()
		s.Require().NoError(s.Repo.UpdateApplicationForeclosure(
			s.Ctx, app.ID, 100, UniqueHash(), 100))
	}

	s.Run("ForeclosesComputedEpoch", func() {
		app := s.createAppWithClaimComputedEpoch()
		markForeclosed(app)

		err := s.Repo.UpdateEpochWithForeclosedClaim(s.Ctx, app.ID, 0)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimForeclosed, got.Status)
	})

	s.Run("ForeclosesSubmittedEpoch", func() {
		app := s.createAppWithClaimComputedEpoch()
		markForeclosed(app)
		txHash := UniqueHash()
		s.Require().NoError(s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, txHash))

		err := s.Repo.UpdateEpochWithForeclosedClaim(s.Ctx, app.ID, 0)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimForeclosed, got.Status)
		s.Require().NotNil(got.ClaimTransactionHash)
		s.Equal(txHash, *got.ClaimTransactionHash)
	})

	s.Run("ForeclosesStagedEpoch", func() {
		app := s.createAppWithClaimComputedEpoch()
		markForeclosed(app)
		s.Require().NoError(s.Repo.UpdateEpochWithSubmittedClaim(s.Ctx, app.ID, 0, UniqueHash()))
		stagedAt := uint64(42)
		s.Require().NoError(s.Repo.UpdateEpochToStaged(s.Ctx, app.ID, 0, stagedAt))

		err := s.Repo.UpdateEpochWithForeclosedClaim(s.Ctx, app.ID, 0)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimForeclosed, got.Status)
		s.Require().NotNil(got.StagedAtBlock)
		s.Equal(stagedAt, *got.StagedAtBlock)
	})

	s.Run("RequiresApplicationForeclosure", func() {
		app := s.createAppWithClaimComputedEpoch()

		err := s.Repo.UpdateEpochWithForeclosedClaim(s.Ctx, app.ID, 0)
		s.Require().Error(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimComputed, got.Status)
	})

	// The PRT drain terminalizes pre-foreclosure epochs the tournament never
	// accepted through this same call, so it must apply to PRT apps too.
	s.Run("ForeclosesPRTComputedEpoch", func() {
		app := s.createAppWithClaimComputedEpochForConsensus(Consensus_PRT)
		markForeclosed(app)

		err := s.Repo.UpdateEpochWithForeclosedClaim(s.Ctx, app.ID, 0)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimForeclosed, got.Status)
	})
}
