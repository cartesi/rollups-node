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
			AdvanceMaxCycles:      1500, //nolint:mnd
			InspectIncCycles:      2000,
			InspectMaxCycles:      2500, //nolint:mnd
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
		s.Equal(ep.InspectIncCycles, got.InspectIncCycles)
		s.Equal(ep.InspectMaxCycles, got.InspectMaxCycles)
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
		s.Equal(app.ClaimStagingPeriod, got.ClaimStagingPeriod)
		s.Equal(app.WithdrawalConfig, got.WithdrawalConfig)
		s.Equal(app.ConsensusType, got.ConsensusType)
		s.Equal(app.Enabled, got.Enabled)
		s.Equal(app.Status, got.Status)
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

	s.Run("FilterByStatus", func() {
		NewApplicationBuilder().WithStatus(ApplicationStatus_OK).Create(s.Ctx, s.T(), s.Repo)
		failed := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		reason := "machine crashed"
		s.Require().NoError(s.Repo.UpdateApplicationStatus(s.Ctx, failed.ID, ApplicationStatus_Failed, &reason))

		status := ApplicationStatus_OK
		apps, total, err := s.Repo.ListApplications(
			s.Ctx,
			repository.ApplicationFilter{Status: &status},
			repository.Pagination{Limit: 10},
			false,
		)
		s.Require().NoError(err)
		s.Len(apps, 1)
		s.Equal(uint64(1), total)
		s.Equal(ApplicationStatus_OK, apps[0].Status)
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
		// Create apps with different combinations of enabled flag, status, consensus, and DA.
		NewApplicationBuilder().
			WithStatus(ApplicationStatus_OK).
			WithConsensus(Consensus_Authority).
			WithDataAvailability(DataAvailability_InputBox[:]).
			Create(s.Ctx, s.T(), s.Repo)
		NewApplicationBuilder().
			WithStatus(ApplicationStatus_OK).
			WithConsensus(Consensus_PRT).
			WithDataAvailability(DataAvailability_InputBox[:]).
			Create(s.Ctx, s.T(), s.Repo)
		NewApplicationBuilder().
			WithEnabled(false).
			WithConsensus(Consensus_Authority).
			WithDataAvailability(DataAvailability_InputBox[:]).
			Create(s.Ctx, s.T(), s.Repo)

		enabled := true
		status := ApplicationStatus_OK
		consensus := Consensus_Authority
		apps, total, err := s.Repo.ListApplications(
			s.Ctx,
			repository.ApplicationFilter{
				Enabled:       &enabled,
				Status:        &status,
				ConsensusType: &consensus,
			},
			repository.Pagination{Limit: 10},
			false,
		)
		s.Require().NoError(err)
		s.Len(apps, 1)
		s.Equal(uint64(1), total)
		s.Equal(ApplicationStatus_OK, apps[0].Status)
		s.Equal(Consensus_Authority, apps[0].ConsensusType)
	})

	// FilterByForeclosureRecorded pins the SQL behind the
	// listEnabledForeclosedNonPRTApps query: ForecloseBlock > 0 selects only
	// apps the evmreader has observed as foreclosed. An IS_NULL/IS_NOT_NULL
	// swap or a GT/EQ swap in the SQL would silently disable the drain-from-
	// idle path; the assertions here catch both directions.
	s.Run("FilterByForeclosureRecorded", func() {
		foreclosed := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationForeclosure(
			s.Ctx, foreclosed.ID, 1234, UniqueHash(), 1234))
		_ = NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo) // not foreclosed

		yes := true
		got, total, err := s.Repo.ListApplications(s.Ctx,
			repository.ApplicationFilter{ForeclosureRecorded: &yes},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(got, 1)
		s.Equal(uint64(1), total)
		s.Equal(foreclosed.ID, got[0].ID)

		no := false
		got, total, err = s.Repo.ListApplications(s.Ctx,
			repository.ApplicationFilter{ForeclosureRecorded: &no},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(got, 1)
		s.Equal(uint64(1), total)
		s.NotEqual(foreclosed.ID, got[0].ID)
	})

	s.Run("CombinedStateAndDataAvailability", func() {
		NewApplicationBuilder().
			WithStatus(ApplicationStatus_OK).
			WithDataAvailability(DataAvailability_InputBox[:]).
			Create(s.Ctx, s.T(), s.Repo)

		otherDA := DataAvailabilitySelector{0xaa, 0xbb, 0xcc, 0xdd}
		NewApplicationBuilder().
			WithStatus(ApplicationStatus_OK).
			WithDataAvailability(otherDA[:]).
			Create(s.Ctx, s.T(), s.Repo)

		status := ApplicationStatus_OK
		da := DataAvailability_InputBox
		apps, total, err := s.Repo.ListApplications(
			s.Ctx,
			repository.ApplicationFilter{
				Status:           &status,
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

func (s *ApplicationSuite) TestUpdateApplicationStatus() {
	s.Run("UpdatesStatus", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Equal(ApplicationStatus_OK, app.Status)

		reason := "machine crashed"
		err := s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Failed, &reason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Failed, got.Status)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("TriggerClearsReasonOnOK", func() {
		// Even if a reason is passed, the DB trigger clears it for OK status.
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// First set to FAILED with a reason
		reason := "machine crash"
		err := s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Failed, &reason)
		s.Require().NoError(err)

		// Recover to OK with a stale reason — trigger should clear it.
		staleReason := "should be cleared"
		err = s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_OK, &staleReason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_OK, got.Status)
		s.Nil(got.Reason)
	})

	s.Run("MissingApplicationReturnsNotFound", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		err := s.Repo.DeleteApplication(s.Ctx, app.ID)
		s.Require().NoError(err)

		reason := "missing app"
		err = s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Failed, &reason)
		s.Require().ErrorIs(err, repository.ErrNotFound)
	})
}

func (s *ApplicationSuite) TestUpdateApplicationEnabled() {
	s.Run("UpdatesOnlyEnabledFlag", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		err := s.Repo.UpdateApplicationEnabled(s.Ctx, app.ID, false)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.False(got.Enabled)
		s.Equal(ApplicationStatus_OK, got.Status)
	})

	s.Run("ReturnsNotFoundWhenRowMissing", func() {
		err := s.Repo.UpdateApplicationEnabled(s.Ctx, int64(99_999_999), false)
		s.Require().ErrorIs(err, repository.ErrNotFound)
	})
}

func (s *ApplicationSuite) TestEnableApplicationAndClearFailed() {
	s.Run("ClearsFailedStatusAndReason", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationEnabled(s.Ctx, app.ID, false))
		reason := "machine crashed"
		s.Require().NoError(s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Failed, &reason))

		err := s.Repo.EnableApplicationAndClearFailed(s.Ctx, app.ID)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.True(got.Enabled)
		s.Equal(ApplicationStatus_OK, got.Status)
		s.Nil(got.Reason)
	})

	s.Run("DoesNotClearCorrupted", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationEnabled(s.Ctx, app.ID, false))
		reason := "corruption"
		s.Require().NoError(s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Corrupted, &reason))

		err := s.Repo.EnableApplicationAndClearFailed(s.Ctx, app.ID)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.True(got.Enabled)
		s.Equal(ApplicationStatus_Corrupted, got.Status)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("DoesNotClearDiverged", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationEnabled(s.Ctx, app.ID, false))
		reason := "claim disagreement"
		s.Require().NoError(s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Diverged, &reason))

		err := s.Repo.EnableApplicationAndClearFailed(s.Ctx, app.ID)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.True(got.Enabled)
		s.Equal(ApplicationStatus_Diverged, got.Status)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("ReturnsNotFoundWhenRowMissing", func() {
		err := s.Repo.EnableApplicationAndClearFailed(s.Ctx, int64(99_999_999))
		s.Require().ErrorIs(err, repository.ErrNotFound)
	})
}

func (s *ApplicationSuite) TestTerminalStatusIsTerminal() {
	// helper: create an app and transition it to the given terminal status via
	// UpdateApplicationStatus.
	makeTerminal := func(status ApplicationStatus, reason string) *Application {
		s.T().Helper()
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		err := s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, status, &reason)
		s.Require().NoError(err)
		return app
	}

	terminalStatuses := []ApplicationStatus{
		ApplicationStatus_Diverged,
		ApplicationStatus_Corrupted,
	}

	for _, status := range terminalStatuses {
		s.Run(string(status)+"CannotChangeStatus", func() {
			reason := "irrecoverable error"
			app := makeTerminal(status, reason)

			newReason := "re-enabling"
			err := s.Repo.UpdateApplicationStatus(
				s.Ctx, app.ID, ApplicationStatus_OK, &newReason)
			s.Require().Error(err)
			s.Contains(err.Error(), string(status))

			got, err := s.Repo.GetApplication(s.Ctx, app.Name)
			s.Require().NoError(err)
			s.Equal(status, got.Status)
			s.Require().NotNil(got.Reason)
			s.Equal(reason, *got.Reason)
		})

		s.Run(string(status)+"CannotChangeReason", func() {
			reason := "original reason"
			app := makeTerminal(status, reason)

			newReason := "different reason"
			err := s.Repo.UpdateApplicationStatus(
				s.Ctx, app.ID, status, &newReason)
			s.Require().Error(err)

			got, err := s.Repo.GetApplication(s.Ctx, app.Name)
			s.Require().NoError(err)
			s.Require().NotNil(got.Reason)
			s.Equal(reason, *got.Reason)
		})

		s.Run(string(status)+"CanBeSetFromOtherStates", func() {
			app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

			reason := "fatal error"
			err := s.Repo.UpdateApplicationStatus(
				s.Ctx, app.ID, status, &reason)
			s.Require().NoError(err)

			got, err := s.Repo.GetApplication(s.Ctx, app.Name)
			s.Require().NoError(err)
			s.Equal(status, got.Status)
			s.Require().NotNil(got.Reason)
			s.Equal(reason, *got.Reason)
		})

		s.Run(string(status)+"ToSameStateAndReasonIsNoOp", func() {
			reason := "irrecoverable"
			app := makeTerminal(status, reason)

			err := s.Repo.UpdateApplicationStatus(
				s.Ctx, app.ID, status, &reason)
			s.Require().NoError(err)

			got, err := s.Repo.GetApplication(s.Ctx, app.Name)
			s.Require().NoError(err)
			s.Equal(status, got.Status)
			s.Require().NotNil(got.Reason)
			s.Equal(reason, *got.Reason)
		})
	}
}

func (s *ApplicationSuite) TestForeclosedCanBecomeTerminal() {
	s.Run("Diverged", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		block := uint64(1234)
		s.Require().NoError(s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, block, UniqueHash(), block))

		reason := "post-foreclosure claim disagreement"
		err := s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Diverged, &reason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Diverged, got.Status)
		s.True(got.IsForeclosed())
		s.Equal(block, got.ForecloseBlock)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("Corrupted", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		block := uint64(1234)
		s.Require().NoError(s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, block, UniqueHash(), block))

		reason := "post-foreclosure replay mismatch"
		err := s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Corrupted, &reason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Corrupted, got.Status)
		s.True(got.IsForeclosed())
		s.Equal(block, got.ForecloseBlock)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})
}

// TestForeclosedCanBecomeFailed verifies that a foreclosed application (one
// with a non-zero foreclose_block) can still transition to FAILED with a
// reason; the row then reads FAILED with foreclose_block preserved. Health
// status and foreclosure live in independent columns.
func (s *ApplicationSuite) TestForeclosedCanBecomeFailed() {
	s.Run("Ok", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		block := uint64(1234)
		s.Require().NoError(s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, block, UniqueHash(), block))

		reason := "machine crashed after foreclosure"
		err := s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Failed, &reason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Failed, got.Status)
		s.True(got.IsForeclosed())
		s.Equal(block, got.ForecloseBlock)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})
}

// TestApplicationStatusTriggerInvariants pins the application trigger
// invariants exercised through the Repository interface: foreclosure is
// one-way (a repeated call with block 0 is an idempotent no-op that leaves the
// recorded block intact), and returning health to OK clears a stale reason.
func (s *ApplicationSuite) TestApplicationStatusTriggerInvariants() {
	s.Run("ForecloseBlockSurvivesIdempotentZeroCall", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		block := uint64(4242)
		s.Require().NoError(s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, block, UniqueHash(), block))

		// A second foreclosure call is guarded on foreclose_block = 0, so it
		// cannot overwrite the recorded block back to zero.
		s.Require().NoError(s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, 0, UniqueHash(), block))

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(block, got.ForecloseBlock)
		s.True(got.IsForeclosed())
	})

	s.Run("StatusToOKClearsReason", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		reason := "machine crashed"
		s.Require().NoError(s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Failed, &reason))

		err := s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_OK, &reason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_OK, got.Status)
		s.Nil(got.Reason)
	})
}

func (s *ApplicationSuite) TestFailedStatusLifecycle() {
	// helper: create an app and transition it to FAILED.
	makeFailed := func(reason string) *Application {
		s.T().Helper()
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		err := s.Repo.UpdateApplicationStatus(
			s.Ctx, app.ID, ApplicationStatus_Failed, &reason)
		s.Require().NoError(err)
		return app
	}

	s.Run("CanRecoverFromFailed", func() {
		reason := "machine crashed"
		app := makeFailed(reason)

		err := s.Repo.UpdateApplicationStatus(
			s.Ctx, app.ID, ApplicationStatus_OK, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_OK, got.Status)
		s.Nil(got.Reason)
	})

	s.Run("CanEscalateFromFailedToCorrupted", func() {
		app := makeFailed("machine error")

		reason := "data corruption detected"
		err := s.Repo.UpdateApplicationStatus(
			s.Ctx, app.ID, ApplicationStatus_Corrupted, &reason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Corrupted, got.Status)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("CanEscalateFromFailedToDiverged", func() {
		app := makeFailed("machine error")

		reason := "claim disagreement detected"
		err := s.Repo.UpdateApplicationStatus(
			s.Ctx, app.ID, ApplicationStatus_Diverged, &reason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Diverged, got.Status)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("ReasonClearedOnReEnable", func() {
		app := makeFailed("OOM kill")

		err := s.Repo.UpdateApplicationStatus(
			s.Ctx, app.ID, ApplicationStatus_OK, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_OK, got.Status)
		s.Nil(got.Reason)
	})

	s.Run("FailedToFailedUpdatesReason", func() {
		app := makeFailed("first crash")

		newReason := "second crash: different error"
		err := s.Repo.UpdateApplicationStatus(
			s.Ctx, app.ID, ApplicationStatus_Failed, &newReason)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Failed, got.Status)
		s.Require().NotNil(got.Reason)
		s.Equal(newReason, *got.Reason)
	})

	s.Run("FullRecoveryCycle", func() {
		// OK -> FAILED -> OK -> FAILED (verify full cycle works)
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// First failure
		reason1 := "crash 1"
		err := s.Repo.UpdateApplicationStatus(
			s.Ctx, app.ID, ApplicationStatus_Failed, &reason1)
		s.Require().NoError(err)

		// Recover to OK.
		err = s.Repo.UpdateApplicationStatus(
			s.Ctx, app.ID, ApplicationStatus_OK, nil)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_OK, got.Status)
		s.Nil(got.Reason)

		// Second failure
		reason2 := "crash 2"
		err = s.Repo.UpdateApplicationStatus(
			s.Ctx, app.ID, ApplicationStatus_Failed, &reason2)
		s.Require().NoError(err)

		got, err = s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(ApplicationStatus_Failed, got.Status)
		s.Require().NotNil(got.Reason)
		s.Equal(reason2, *got.Reason)
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
		s.Zero(ep.AdvanceMaxCycles)
		s.Zero(ep.InspectMaxCycles)
	})
}

func (s *ApplicationSuite) TestUpdateExecutionParameters() {
	s.Run("UpdatesValues", func() {
		ep := ExecutionParameters{
			SnapshotPolicy:        SnapshotPolicy_EveryInput,
			AdvanceIncCycles:      2000,
			AdvanceMaxCycles:      2500, //nolint:mnd
			InspectIncCycles:      3000,
			InspectMaxCycles:      3500, //nolint:mnd
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
		ep.AdvanceIncCycles = 99999
		ep.AdvanceMaxCycles = MaxExecutionCycleSpan
		err := s.Repo.UpdateExecutionParameters(s.Ctx, &ep)
		s.Require().NoError(err)

		got, err := s.Repo.GetExecutionParameters(s.Ctx, app.ID)
		s.Require().NoError(err)
		s.Equal(uint64(99999), got.AdvanceIncCycles)
		s.Equal(MaxExecutionCycleSpan, got.AdvanceMaxCycles)
		s.Equal(ep.InspectIncCycles, got.InspectIncCycles)
		s.Equal(ep.InspectMaxCycles, got.InspectMaxCycles)
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

		// ClaimSubmitted and ClaimAccepted should return errors
		_, err = s.Repo.GetEventLastCheckBlock(
			s.Ctx, app.ID, MonitoredEvent_ClaimSubmitted)
		s.Require().Error(err)

		_, err = s.Repo.GetEventLastCheckBlock(
			s.Ctx, app.ID, MonitoredEvent_ClaimAccepted)
		s.Require().Error(err)
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

	// UpdateApplication must not touch status or foreclosure columns. Those
	// fields are owned by UpdateApplicationStatus and the atomic foreclosure
	// marker+cursor write. If UpdateApplication's column list ever re-includes
	// them, a caller with a stale in-memory app could silently clear the marker
	// or move the app back to OK while changing unrelated configuration.
	s.Run("DoesNotClobberStatusOrForecloseColumns", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		block := uint64(12345)
		txHash := crypto.Keccak256Hash([]byte("foreclose-tx"))
		err := s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, block, txHash, block)
		s.Require().NoError(err)

		// Mutate an unrelated field on an in-memory copy whose
		// ForecloseBlock / ForecloseTransaction are zero (simulating a caller
		// that reads, modifies, and writes back without first refreshing the
		// foreclosure status). UpdateApplication must leave the persisted
		// foreclose columns alone.
		app.EpochLength = 77
		s.Require().Zero(app.ForecloseBlock)
		s.Require().Nil(app.ForecloseTransaction)
		err = s.Repo.UpdateApplication(s.Ctx, app)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(77), got.EpochLength)
		s.Require().NotZero(got.ForecloseBlock, "foreclose_block must not be cleared by UpdateApplication")
		s.Equal(block, got.ForecloseBlock)
		s.Require().NotNil(got.ForecloseTransaction)
		s.Equal(txHash, *got.ForecloseTransaction)
		s.True(got.IsForeclosed())
	})

	s.Run("DoesNotClobberServiceProgressOrEnabled", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		app := seed.App
		s.Require().NoError(s.Repo.UpdateApplicationEnabled(s.Ctx, app.ID, false))
		s.Require().NoError(s.Repo.UpdateEventLastCheckBlock(
			s.Ctx, []int64{app.ID}, MonitoredEvent_InputAdded, 42))
		s.Require().NoError(s.Repo.UpdateEventLastCheckBlock(
			s.Ctx, []int64{app.ID}, MonitoredEvent_OutputExecuted, 43))
		s.Require().NoError(s.Repo.StoreAdvanceResult(s.Ctx, app.ID, &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			OutputsProof: OutputsProof{
				MachineHash: crypto.Keccak256Hash([]byte("machine")),
			},
		}))

		app.EpochLength = 33
		app.Enabled = true
		app.LastInputCheckBlock = 0
		app.LastOutputCheckBlock = 0
		app.ProcessedInputs = 0
		err := s.Repo.UpdateApplication(s.Ctx, app)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(33), got.EpochLength)
		s.False(got.Enabled)
		s.Equal(uint64(42), got.LastInputCheckBlock)
		s.Equal(uint64(43), got.LastOutputCheckBlock)
		s.Equal(uint64(1), got.ProcessedInputs)
	})

	s.Run("ReturnsNotFoundWhenRowMissing", func() {
		app := NewApplicationBuilder().Build()
		app.ID = int64(99_999_999)
		err := s.Repo.UpdateApplication(s.Ctx, app)
		s.Require().ErrorIs(err, repository.ErrNotFound)
	})
}

func (s *ApplicationSuite) TestUpdateApplicationForeclosure() {
	s.Run("WritesMarkerAndCursor", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		block := uint64(1234)
		head := uint64(1500)
		txHash := UniqueHash()

		err := s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, block, txHash, head)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(block, got.ForecloseBlock)
		s.Require().NotNil(got.ForecloseTransaction)
		s.Equal(txHash, *got.ForecloseTransaction)
		s.Equal(head, got.LastForecloseCheckBlock)
		s.True(got.IsForeclosed())
		s.Equal(ApplicationStatus_OK, got.Status)
		s.Nil(got.Reason)
	})

	s.Run("PreservesTerminalStatusAndReason", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		reason := "divergent claim observed"
		err := s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, ApplicationStatus_Diverged, &reason)
		s.Require().NoError(err)

		block := uint64(1234)
		head := uint64(1500)
		txHash := UniqueHash()
		err = s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, block, txHash, head)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(block, got.ForecloseBlock)
		s.Equal(head, got.LastForecloseCheckBlock)
		s.True(got.IsForeclosed())
		s.Equal(ApplicationStatus_Diverged, got.Status)
		s.Require().NotNil(got.Reason)
		s.Equal(reason, *got.Reason)
	})

	s.Run("DoesNotRegressCursor", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationLastForecloseCheckBlock(s.Ctx, app.ID, 500))

		err := s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, 300, UniqueHash(), 400)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(500), got.LastForecloseCheckBlock)
		s.Equal(uint64(300), got.ForecloseBlock)
	})

	s.Run("IdempotentWhenAlreadyForeclosed", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		firstBlock := uint64(1234)
		firstHead := uint64(1500)
		firstTx := UniqueHash()
		s.Require().NoError(
			s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, firstBlock, firstTx, firstHead))

		err := s.Repo.UpdateApplicationForeclosure(s.Ctx, app.ID, 9999, UniqueHash(), 2000)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(firstBlock, got.ForecloseBlock)
		s.Require().NotNil(got.ForecloseTransaction)
		s.Equal(firstTx, *got.ForecloseTransaction)
		s.Equal(firstHead, got.LastForecloseCheckBlock)
	})

	s.Run("ReturnsNotFoundWhenRowMissing", func() {
		err := s.Repo.UpdateApplicationForeclosure(
			s.Ctx, int64(99_999_999), 1, UniqueHash(), 2)
		s.Require().ErrorIs(err, repository.ErrNotFound)
	})
}

// TestUpdateApplicationLastForecloseCheckBlock pins the strictly monotonic
// semantics of the write. Out-of-order or duplicate observations from a
// slow tick must not rewind last_foreclose_check_block and re-cause a
// full [deployment, head] rescan on the next tick.
func (s *ApplicationSuite) TestUpdateApplicationLastForecloseCheckBlock() {
	s.Run("AdvancesFromZero", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		err := s.Repo.UpdateApplicationLastForecloseCheckBlock(s.Ctx, app.ID, 1234)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(1234), got.LastForecloseCheckBlock)
	})

	s.Run("AdvancesForward", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationLastForecloseCheckBlock(s.Ctx, app.ID, 100))
		s.Require().NoError(s.Repo.UpdateApplicationLastForecloseCheckBlock(s.Ctx, app.ID, 200))

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(200), got.LastForecloseCheckBlock)
	})

	// Out-of-order ticks: a stale call carrying a lower block number must
	// be a silent no-op, not an error and not a regression of the stored
	// value. The repo returns nil (matches LastInputCheckBlock-style
	// conventions); the caller cannot distinguish "I was stale" from
	// "I was current". That is intentional — the next tick's read will
	// surface the true value.
	s.Run("RejectsRegressionSilently", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationLastForecloseCheckBlock(s.Ctx, app.ID, 500))

		err := s.Repo.UpdateApplicationLastForecloseCheckBlock(s.Ctx, app.ID, 100)
		s.Require().NoError(err, "regression attempts return nil; the WHERE guard makes it a no-op")

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(500), got.LastForecloseCheckBlock,
			"last_foreclose_check_block must not regress below its previous value")
	})

	// Equal-value writes are also no-ops, mirroring the strict-less-than
	// guard. Useful when two ticks happen to land on the same head block.
	s.Run("RejectsEqualSilently", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationLastForecloseCheckBlock(s.Ctx, app.ID, 777))

		err := s.Repo.UpdateApplicationLastForecloseCheckBlock(s.Ctx, app.ID, 777)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(777), got.LastForecloseCheckBlock)
	})
}

// TestUpdateApplicationLastAccountsDriveProvedCheckBlock mirrors the
// LastForecloseCheckBlock contract: strictly monotonic, regression and
// equal-value writes are silent no-ops. Out-of-order ticks must not rewind
// the cursor and re-cause a full [foreclose_block, head] rescan.
func (s *ApplicationSuite) TestUpdateApplicationLastAccountsDriveProvedCheckBlock() {
	s.Run("AdvancesFromZero", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationLastAccountsDriveProvedCheckBlock(s.Ctx, app.ID, 1234))
		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(1234), got.LastAccountsDriveProvedCheckBlock)
	})

	s.Run("AdvancesForward", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationLastAccountsDriveProvedCheckBlock(s.Ctx, app.ID, 100))
		s.Require().NoError(s.Repo.UpdateApplicationLastAccountsDriveProvedCheckBlock(s.Ctx, app.ID, 200))
		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(200), got.LastAccountsDriveProvedCheckBlock)
	})

	s.Run("RejectsRegressionSilently", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationLastAccountsDriveProvedCheckBlock(s.Ctx, app.ID, 500))
		s.Require().NoError(s.Repo.UpdateApplicationLastAccountsDriveProvedCheckBlock(s.Ctx, app.ID, 100))
		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(500), got.LastAccountsDriveProvedCheckBlock,
			"last_accounts_drive_proved_check_block must not regress below its previous value")
	})

	s.Run("RejectsEqualSilently", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationLastAccountsDriveProvedCheckBlock(s.Ctx, app.ID, 777))
		s.Require().NoError(s.Repo.UpdateApplicationLastAccountsDriveProvedCheckBlock(s.Ctx, app.ID, 777))
		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(777), got.LastAccountsDriveProvedCheckBlock)
	})
}

func (s *ApplicationSuite) TestUpdateAccountsDriveProved() {
	s.Run("WritesMarkerAndCursor", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		block := uint64(4242)
		head := uint64(4300)
		txHash := UniqueHash()
		root := UniqueHash()

		err := s.Repo.UpdateAccountsDriveProved(s.Ctx, app.ID, block, txHash, root, head)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(block, got.AccountsDriveProvedBlock)
		s.Require().NotNil(got.AccountsDriveProvedTransaction)
		s.Equal(txHash, *got.AccountsDriveProvedTransaction)
		s.Require().NotNil(got.AccountsDriveMerkleRoot)
		s.Equal(root, *got.AccountsDriveMerkleRoot)
		s.Equal(head, got.LastAccountsDriveProvedCheckBlock)
	})

	s.Run("DoesNotRegressCursor", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.UpdateApplicationLastAccountsDriveProvedCheckBlock(s.Ctx, app.ID, 500))

		err := s.Repo.UpdateAccountsDriveProved(
			s.Ctx, app.ID, 300, UniqueHash(), UniqueHash(), 400)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(500), got.LastAccountsDriveProvedCheckBlock)
		s.Equal(uint64(300), got.AccountsDriveProvedBlock)
	})

	s.Run("IdempotentWhenAlreadyProved", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		firstBlock := uint64(4242)
		firstHead := uint64(4300)
		firstTx := UniqueHash()
		firstRoot := UniqueHash()
		s.Require().NoError(s.Repo.UpdateAccountsDriveProved(
			s.Ctx, app.ID, firstBlock, firstTx, firstRoot, firstHead))

		err := s.Repo.UpdateAccountsDriveProved(
			s.Ctx, app.ID, 9999, UniqueHash(), UniqueHash(), 5000)
		s.Require().NoError(err)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(firstBlock, got.AccountsDriveProvedBlock)
		s.Require().NotNil(got.AccountsDriveProvedTransaction)
		s.Equal(firstTx, *got.AccountsDriveProvedTransaction)
		s.Require().NotNil(got.AccountsDriveMerkleRoot)
		s.Equal(firstRoot, *got.AccountsDriveMerkleRoot)
		s.Equal(firstHead, got.LastAccountsDriveProvedCheckBlock)
	})

	s.Run("ReturnsNotFoundWhenRowMissing", func() {
		err := s.Repo.UpdateAccountsDriveProved(
			s.Ctx, int64(99_999_999), 1, UniqueHash(), UniqueHash(), 2)
		s.Require().ErrorIs(err, repository.ErrNotFound)
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
