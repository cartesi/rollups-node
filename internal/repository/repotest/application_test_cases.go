// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/crypto"
)

type ApplicationSuite struct {
	BaseSuite
}

func NewApplicationSuite(factory RepositoryFactory) *ApplicationSuite {
	return &ApplicationSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *ApplicationSuite) TestCreateApplication() {
	s.Run("ReturnsGeneratedID", func() {
		app := NewApplicationBuilder().Build()
		id, err := s.Repo.CreateApplication(s.Ctx, app, false)
		s.Require().NoError(err)
		s.Greater(id, int64(0))
	})

	s.Run("WithExecutionParameters", func() {
		ep := ExecutionParameters{
			SnapshotPolicy:        SnapshotPolicy_EveryEpoch,
			AdvanceIncCycles:      1000,
			AdvanceMaxCycles:      5000,
			InspectIncCycles:      1000,
			InspectMaxCycles:      5000,
			AdvanceIncDeadline:    10 * time.Second,
			AdvanceMaxDeadline:    60 * time.Second,
			InspectIncDeadline:    10 * time.Second,
			InspectMaxDeadline:    60 * time.Second,
			LoadDeadline:          30 * time.Second,
			StoreDeadline:         30 * time.Second,
			FastDeadline:          5 * time.Second,
			MaxConcurrentInspects: 10,
		}
		app := NewApplicationBuilder().
			WithExecutionParameters(ep).
			Create(s.Ctx, s.T(), s.Repo)
		s.Greater(app.ID, int64(0))

		got, err := s.Repo.GetExecutionParameters(s.Ctx, app.ID)
		s.Require().NoError(err)
		s.Equal(ep.SnapshotPolicy, got.SnapshotPolicy)
		s.Equal(ep.AdvanceIncCycles, got.AdvanceIncCycles)
		s.Equal(ep.AdvanceMaxCycles, got.AdvanceMaxCycles)
		s.Equal(ep.MaxConcurrentInspects, got.MaxConcurrentInspects)
	})
}

func (s *ApplicationSuite) TestGetApplication() {
	s.Run("ByName", func() {
		app := NewApplicationBuilder().
			WithName("my-unique-app").
			Create(s.Ctx, s.T(), s.Repo)

		got, err := s.Repo.GetApplication(s.Ctx, "my-unique-app")
		s.Require().NoError(err)
		s.Equal(app.ID, got.ID)
		s.Equal("my-unique-app", got.Name)
		s.Equal(app.IApplicationAddress, got.IApplicationAddress)
		s.Equal(app.IConsensusAddress, got.IConsensusAddress)
		s.Equal(app.IInputBoxAddress, got.IInputBoxAddress)
		s.Equal(app.TemplateHash, got.TemplateHash)
		s.Equal(app.EpochLength, got.EpochLength)
		s.Equal(app.ConsensusType, got.ConsensusType)
		s.Equal(app.State, got.State)
		s.Equal(app.DataAvailability, got.DataAvailability)
		s.False(got.CreatedAt.IsZero(), "CreatedAt should be set")
		s.False(got.UpdatedAt.IsZero(), "UpdatedAt should be set")
	})

	s.Run("ByAddress", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		got, err := s.Repo.GetApplication(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(app.ID, got.ID)
		s.Equal(app.Name, got.Name)
	})

	s.Run("NotFound", func() {
		got, err := s.Repo.GetApplication(s.Ctx, "nonexistent")
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *ApplicationSuite) TestListApplications() {
	s.Run("EmptyResult", func() {
		apps, total, err := s.Repo.ListApplications(
			s.Ctx, repository.ApplicationFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(apps)
		s.Equal(uint64(0), total)
	})

	s.Run("ReturnsAllApps", func() {
		for range 3 {
			NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		}
		apps, total, err := s.Repo.ListApplications(
			s.Ctx, repository.ApplicationFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(apps, 3)
		s.Equal(uint64(3), total)
	})

	s.Run("FilterByState", func() {
		NewApplicationBuilder().WithState(ApplicationState_Enabled).Create(s.Ctx, s.T(), s.Repo)
		NewApplicationBuilder().WithState(ApplicationState_Disabled).Create(s.Ctx, s.T(), s.Repo)

		state := ApplicationState_Enabled
		apps, total, err := s.Repo.ListApplications(
			s.Ctx,
			repository.ApplicationFilter{State: &state},
			repository.Pagination{Limit: 10},
			false,
		)
		s.Require().NoError(err)
		s.Len(apps, 1)
		s.Equal(uint64(1), total)
		s.Equal(ApplicationState_Enabled, apps[0].State)
	})

	s.Run("FilterByConsensus", func() {
		NewApplicationBuilder().WithConsensus(Consensus_Authority).Create(s.Ctx, s.T(), s.Repo)
		NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(s.Ctx, s.T(), s.Repo)

		consensus := Consensus_PRT
		apps, total, err := s.Repo.ListApplications(
			s.Ctx,
			repository.ApplicationFilter{ConsensusType: &consensus},
			repository.Pagination{Limit: 10},
			false,
		)
		s.Require().NoError(err)
		s.Len(apps, 1)
		s.Equal(uint64(1), total)
		s.Equal(Consensus_PRT, apps[0].ConsensusType)
	})

	s.Run("FilterByDataAvailability", func() {
		NewApplicationBuilder().
			WithDataAvailability(DataAvailability_InputBox[:]).
			Create(s.Ctx, s.T(), s.Repo)
		// Create another app with a different DA selector
		otherDA := DataAvailabilitySelector{0xaa, 0xbb, 0xcc, 0xdd}
		NewApplicationBuilder().
			WithDataAvailability(otherDA[:]).
			Create(s.Ctx, s.T(), s.Repo)

		da := DataAvailability_InputBox
		apps, total, err := s.Repo.ListApplications(
			s.Ctx,
			repository.ApplicationFilter{DataAvailability: &da},
			repository.Pagination{Limit: 10},
			false,
		)
		s.Require().NoError(err)
		s.Len(apps, 1)
		s.Equal(uint64(1), total)
		s.Equal(DataAvailability_InputBox[:], apps[0].DataAvailability[:4])
	})

	s.Run("Pagination", func() {
		for range 5 {
			NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		}

		apps, total, err := s.Repo.ListApplications(
			s.Ctx,
			repository.ApplicationFilter{},
			repository.Pagination{Limit: 2, Offset: 0},
			false,
		)
		s.Require().NoError(err)
		s.Len(apps, 2)
		s.Equal(uint64(5), total)

		apps2, total2, err := s.Repo.ListApplications(
			s.Ctx,
			repository.ApplicationFilter{},
			repository.Pagination{Limit: 2, Offset: 2},
			false,
		)
		s.Require().NoError(err)
		s.Len(apps2, 2)
		s.Equal(uint64(5), total2)
		// Pages should be different
		s.NotEqual(apps[0].ID, apps2[0].ID)
	})

	s.Run("Descending", func() {
		a1 := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		a2 := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		apps, _, err := s.Repo.ListApplications(
			s.Ctx, repository.ApplicationFilter{}, repository.Pagination{Limit: 10}, true)
		s.Require().NoError(err)
		s.Require().Len(apps, 2)
		// Descending: second created should be first
		s.Equal(a2.ID, apps[0].ID)
		s.Equal(a1.ID, apps[1].ID)
	})

	s.Run("CombinedFilters", func() {
		// Create apps with different combinations of state, consensus, and DA
		NewApplicationBuilder().
			WithState(ApplicationState_Enabled).
			WithConsensus(Consensus_Authority).
			WithDataAvailability(DataAvailability_InputBox[:]).
			Create(s.Ctx, s.T(), s.Repo)
		NewApplicationBuilder().
			WithState(ApplicationState_Enabled).
			WithConsensus(Consensus_PRT).
			WithDataAvailability(DataAvailability_InputBox[:]).
			Create(s.Ctx, s.T(), s.Repo)
		NewApplicationBuilder().
			WithState(ApplicationState_Disabled).
			WithConsensus(Consensus_Authority).
			WithDataAvailability(DataAvailability_InputBox[:]).
			Create(s.Ctx, s.T(), s.Repo)

		state := ApplicationState_Enabled
		consensus := Consensus_Authority
		apps, total, err := s.Repo.ListApplications(
			s.Ctx,
			repository.ApplicationFilter{
				State:         &state,
				ConsensusType: &consensus,
			},
			repository.Pagination{Limit: 10},
			false,
		)
		s.Require().NoError(err)
		s.Len(apps, 1)
		s.Equal(uint64(1), total)
		s.Equal(ApplicationState_Enabled, apps[0].State)
		s.Equal(Consensus_Authority, apps[0].ConsensusType)
	})

	s.Run("CombinedStateAndDataAvailability", func() {
		NewApplicationBuilder().
			WithState(ApplicationState_Enabled).
			WithDataAvailability(DataAvailability_InputBox[:]).
			Create(s.Ctx, s.T(), s.Repo)

		otherDA := DataAvailabilitySelector{0xaa, 0xbb, 0xcc, 0xdd}
		NewApplicationBuilder().
			WithState(ApplicationState_Enabled).
			WithDataAvailability(otherDA[:]).
			Create(s.Ctx, s.T(), s.Repo)

		state := ApplicationState_Enabled
		da := DataAvailability_InputBox
		apps, total, err := s.Repo.ListApplications(
			s.Ctx,
			repository.ApplicationFilter{
				State:            &state,
				DataAvailability: &da,
			},
			repository.Pagination{Limit: 10},
			false,
		)
		s.Require().NoError(err)
		s.Len(apps, 1)
		s.Equal(uint64(1), total)
		s.Equal(DataAvailability_InputBox[:], apps[0].DataAvailability[:4])
	})
}

func (s *ApplicationSuite) TestUpdateApplicationState() {
	s.Run("UpdatesState", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Equal(ApplicationState_Enabled, app.State)

		err := s.Repo.UpdateApplicationState(s.Ctx, app.ID, ApplicationState_Disabled, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Disabled, got.State)
		s.Nil(got.Reason)
	})

	s.Run("TriggerClearsReasonOnEnabled", func() {
		// Even if a reason is passed, the DB trigger clears it for ENABLED/DISABLED states
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// First set to FAILED with a reason
		reason := "machine crash"
		err := s.Repo.UpdateApplicationState(s.Ctx, app.ID, ApplicationState_Failed, &reason)
		s.Require().NoError(err)

		// Re-enable with a stale reason — trigger should clear it
		staleReason := "should be cleared"
		err = s.Repo.UpdateApplicationState(s.Ctx, app.ID, ApplicationState_Enabled, &staleReason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Enabled, got.State)
		s.Nil(got.Reason)
	})
}

func (s *ApplicationSuite) TestInoperableIsTerminal() {
	// helper: create an app and transition it to INOPERABLE via UpdateApplicationState.
	makeInoperable := func(reason string) *Application {
		s.T().Helper()
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Inoperable, &reason)
		s.Require().NoError(err)
		return app
	}

	s.Run("CannotChangeStateFromInoperable", func() {
		reason := "irrecoverable error"
		app := makeInoperable(reason)

		newReason := "re-enabling"
		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Enabled, &newReason)
		s.Require().Error(err)
		s.Contains(err.Error(), "INOPERABLE")

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Inoperable, got.State)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("CannotChangeReasonFromInoperable", func() {
		reason := "original reason"
		app := makeInoperable(reason)

		newReason := "different reason"
		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Inoperable, &newReason)
		s.Require().Error(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("CanSetToInoperableFromOtherStates", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		reason := "fatal error"
		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Inoperable, &reason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Inoperable, got.State)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("InoperableToSameStateAndReasonIsNoOp", func() {
		reason := "irrecoverable"
		app := makeInoperable(reason)

		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Inoperable, &reason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Inoperable, got.State)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})
}

func (s *ApplicationSuite) TestFailedStateLifecycle() {
	// helper: create an app and transition it to FAILED.
	makeFailed := func(reason string) *Application {
		s.T().Helper()
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Failed, &reason)
		s.Require().NoError(err)
		return app
	}

	s.Run("CanReEnableFromFailed", func() {
		reason := "machine crashed"
		app := makeFailed(reason)

		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Enabled, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Enabled, got.State)
		s.Nil(got.Reason)
	})

	s.Run("CanDisableFromFailed", func() {
		reason := "process crash"
		app := makeFailed(reason)

		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Disabled, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Disabled, got.State)
	})

	s.Run("CanEscalateFromFailedToInoperable", func() {
		app := makeFailed("machine error")

		reason := "data corruption detected"
		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Inoperable, &reason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Inoperable, got.State)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("ReasonClearedOnReEnable", func() {
		app := makeFailed("OOM kill")

		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Enabled, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Enabled, got.State)
		s.Nil(got.Reason)
	})

	s.Run("FailedToFailedUpdatesReason", func() {
		app := makeFailed("first crash")

		newReason := "second crash: different error"
		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Failed, &newReason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Failed, got.State)
		s.Require().NotNil(got.Reason)
		s.Equal(newReason, *got.Reason)
	})

	s.Run("FullRecoveryCycle", func() {
		// ENABLED -> FAILED -> ENABLED -> FAILED (verify full cycle works)
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// First failure
		reason1 := "crash 1"
		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Failed, &reason1)
		s.Require().NoError(err)

		// Re-enable
		err = s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Enabled, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Enabled, got.State)
		s.Nil(got.Reason)

		// Second failure
		reason2 := "crash 2"
		err = s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Failed, &reason2)
		s.Require().NoError(err)

		got, err = s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Failed, got.State)
		s.Require().NotNil(got.Reason)
		s.Equal(reason2, *got.Reason)
	})
}

func (s *ApplicationSuite) TestDisabledToFailedBlocked() {
	s.Run("CannotTransitionFromDisabledToFailed", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// First disable the app
		err := s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Disabled, nil)
		s.Require().NoError(err)

		// Attempt DISABLED -> FAILED should be blocked by trigger
		reason := "should not work"
		err = s.Repo.UpdateApplicationState(
			s.Ctx, app.ID, ApplicationState_Failed, &reason)
		s.Require().Error(err)
		s.Contains(err.Error(), "DISABLED")

		// Verify state unchanged
		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationState_Disabled, got.State)
	})
}

func (s *ApplicationSuite) TestDeleteApplication() {
	s.Run("DeletesExistingApp", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		err := s.Repo.DeleteApplication(s.Ctx, app.ID)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *ApplicationSuite) TestGetExecutionParameters() {
	s.Run("DefaultParameters", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		ep, err := s.Repo.GetExecutionParameters(s.Ctx, app.ID)
		s.Require().NoError(err)
		s.NotNil(ep)
	})
}

func (s *ApplicationSuite) TestUpdateExecutionParameters() {
	s.Run("UpdatesValues", func() {
		ep := ExecutionParameters{
			SnapshotPolicy:        SnapshotPolicy_EveryInput,
			AdvanceIncCycles:      2000,
			AdvanceMaxCycles:      10000,
			InspectIncCycles:      2000,
			InspectMaxCycles:      10000,
			AdvanceIncDeadline:    20 * time.Second,
			AdvanceMaxDeadline:    120 * time.Second,
			InspectIncDeadline:    20 * time.Second,
			InspectMaxDeadline:    120 * time.Second,
			LoadDeadline:          60 * time.Second,
			StoreDeadline:         60 * time.Second,
			FastDeadline:          10 * time.Second,
			MaxConcurrentInspects: 5,
		}
		app := NewApplicationBuilder().
			WithExecutionParameters(ep).
			Create(s.Ctx, s.T(), s.Repo)

		ep.ApplicationID = app.ID
		ep.AdvanceMaxCycles = 99999
		err := s.Repo.UpdateExecutionParameters(s.Ctx, &ep)
		s.Require().NoError(err)

		got, err := s.Repo.GetExecutionParameters(s.Ctx, app.ID)
		s.Require().NoError(err)
		s.Equal(uint64(99999), got.AdvanceMaxCycles)
	})
}

func (s *ApplicationSuite) TestEventLastCheckBlock() {
	s.Run("DefaultIsZero", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		block, err := s.Repo.GetEventLastCheckBlock(s.Ctx, app.ID, MonitoredEvent_InputAdded)
		s.Require().NoError(err)
		s.Equal(uint64(0), block)
	})

	s.Run("UpdateAndGet", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		err := s.Repo.UpdateEventLastCheckBlock(
			s.Ctx, []int64{app.ID}, MonitoredEvent_InputAdded, 42)
		s.Require().NoError(err)

		block, err := s.Repo.GetEventLastCheckBlock(s.Ctx, app.ID, MonitoredEvent_InputAdded)
		s.Require().NoError(err)
		s.Equal(uint64(42), block)
	})

	s.Run("AllMonitoredEventTypes", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// Events that map to the epoch check block column
		err := s.Repo.UpdateEventLastCheckBlock(
			s.Ctx, []int64{app.ID}, MonitoredEvent_EpochSealed, 10)
		s.Require().NoError(err)
		block, err := s.Repo.GetEventLastCheckBlock(
			s.Ctx, app.ID, MonitoredEvent_EpochSealed)
		s.Require().NoError(err)
		s.Equal(uint64(10), block)

		// Events that map to the output check block column
		err = s.Repo.UpdateEventLastCheckBlock(
			s.Ctx, []int64{app.ID}, MonitoredEvent_OutputExecuted, 20)
		s.Require().NoError(err)
		block, err = s.Repo.GetEventLastCheckBlock(
			s.Ctx, app.ID, MonitoredEvent_OutputExecuted)
		s.Require().NoError(err)
		s.Equal(uint64(20), block)

		err = s.Repo.UpdateEventLastCheckBlock(
			s.Ctx, []int64{app.ID}, MonitoredEvent_ClaimAccepted, 30)
		s.Require().NoError(err)
		block, err = s.Repo.GetEventLastCheckBlock(
			s.Ctx, app.ID, MonitoredEvent_ClaimAccepted)
		s.Require().NoError(err)
		s.Equal(uint64(30), block)

		err = s.Repo.UpdateEventLastCheckBlock(
			s.Ctx, []int64{app.ID}, MonitoredEvent_ClaimSubmitted, 40)
		s.Require().NoError(err)
		block, err = s.Repo.GetEventLastCheckBlock(
			s.Ctx, app.ID, MonitoredEvent_ClaimSubmitted)
		s.Require().NoError(err)
		s.Equal(uint64(40), block)

		// Tournament events all map to the tournament check block column
		tournamentEvents := []MonitoredEvent{
			MonitoredEvent_CommitmentJoined,
			MonitoredEvent_MatchAdvanced,
			MonitoredEvent_MatchCreated,
			MonitoredEvent_MatchDeleted,
			MonitoredEvent_NewInnerTournament,
		}
		for _, event := range tournamentEvents {
			err = s.Repo.UpdateEventLastCheckBlock(
				s.Ctx, []int64{app.ID}, event, 30)
			s.Require().NoError(err)
			block, err = s.Repo.GetEventLastCheckBlock(s.Ctx, app.ID, event)
			s.Require().NoError(err)
			s.Equal(uint64(30), block)
		}
	})

	s.Run("UpdateMultipleAppIDs", func() {
		app1 := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		app2 := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		err := s.Repo.UpdateEventLastCheckBlock(
			s.Ctx, []int64{app1.ID, app2.ID}, MonitoredEvent_InputAdded, 55)
		s.Require().NoError(err)

		block1, err := s.Repo.GetEventLastCheckBlock(
			s.Ctx, app1.ID, MonitoredEvent_InputAdded)
		s.Require().NoError(err)
		s.Equal(uint64(55), block1)

		block2, err := s.Repo.GetEventLastCheckBlock(
			s.Ctx, app2.ID, MonitoredEvent_InputAdded)
		s.Require().NoError(err)
		s.Equal(uint64(55), block2)
	})

	s.Run("EmptyAppIDsIsNoOp", func() {
		err := s.Repo.UpdateEventLastCheckBlock(
			s.Ctx, []int64{}, MonitoredEvent_InputAdded, 42)
		s.Require().NoError(err)
	})
}

func (s *ApplicationSuite) TestGetProcessedInputCount() {
	s.Run("ReturnsZeroInitially", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		count, err := s.Repo.GetProcessedInputCount(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(0), count)
	})

	s.Run("ReturnsCountAfterProcessing", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// StoreAdvanceResult increments ProcessedInputs
		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			OutputsProof: OutputsProof{
				MachineHash: crypto.Keccak256Hash([]byte("machine")),
			},
		}
		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		count, err := s.Repo.GetProcessedInputCount(
			s.Ctx, seed.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(1), count)
	})
}

func (s *ApplicationSuite) TestUpdateApplication() {
	s.Run("UpdatesFields", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		app.EpochLength = 20
		err := s.Repo.UpdateApplication(s.Ctx, app)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(20), got.EpochLength)
	})
}

func (s *ApplicationSuite) TestGetLastSnapshot() {
	s.Run("NotFound", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetLastSnapshot(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Nil(got)
	})

	s.Run("ReturnsInputWithSnapshot", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// GetLastSnapshot requires Status == Accepted AND SnapshotURI IS NOT NULL.
		// Use StoreAdvanceResult to set the input to Accepted first.
		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			OutputsProof: OutputsProof{
				MachineHash: crypto.Keccak256Hash([]byte("machine")),
			},
		}
		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		uri := "/snapshots/epoch-0-input-0"
		err = s.Repo.UpdateInputSnapshotURI(s.Ctx, seed.App.ID, 0, uri)
		s.Require().NoError(err)

		got, err := s.Repo.GetLastSnapshot(
			s.Ctx, seed.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Require().NotNil(got)
		s.Require().NotNil(got.SnapshotURI)
		s.Equal(uri, *got.SnapshotURI)
		s.Equal(uint64(0), got.Index)
	})
}
