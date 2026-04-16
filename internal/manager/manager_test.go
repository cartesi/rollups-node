// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/replay"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestMachineManager(t *testing.T) {
	suite.Run(t, new(MachineManagerSuite))
}

type MachineManagerSuite struct {
	suite.Suite
}

func newForkableMock() *MockRollupsMachine {
	runtime := &MockRollupsMachine{
		CompletionStatusReturn: machine.CompletionStatusAccepted,
		HashReturn:             newHash(1),
		OutputsHashReturn:      newHash(2),
	}
	runtime.ForkFunc = func(context.Context) (machine.Machine, error) {
		return newForkableMock(), nil
	}
	return runtime
}

func newTestMachineManager(
	repo MachineRepository,
	logger *slog.Logger,
	checkTemplateHash bool,
	inputBatchSize uint64,
	opts ...Option,
) *MachineManager {
	testRun := withReplayRun(func(
		_ context.Context,
		_ repository.ReplayRepository,
		executor replay.Executor,
		_ replay.Options,
	) (replay.Result, error) {
		if instance, ok := executor.(*DummyMachineInstanceMock); ok {
			instance.replayCalls++
			return replay.Result{}, instance.replayErr
		}
		return replay.Result{}, nil
	})
	return NewMachineManager(
		repo, logger, checkTemplateHash, inputBatchSize, append(opts, testRun)...,
	)
}

type nilSingleUnwrapperError struct{}

func (nilSingleUnwrapperError) Error() string { return "nil single unwrapper" }
func (nilSingleUnwrapperError) Unwrap() error { return nil }

type emptyMultiUnwrapperError struct{}

func (emptyMultiUnwrapperError) Error() string   { return "empty multi unwrapper" }
func (emptyMultiUnwrapperError) Unwrap() []error { return []error{} }

func (s *MachineManagerSuite) TestNewMachineManager() {
	require := s.Require()
	repo := &MockMachineRepository{}
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := newTestMachineManager(repo, testLogger, false, 500)
	require.NotNil(manager)
	require.Empty(manager.machines)
	require.Equal(repo, manager.repository)
}

func (s *MachineManagerSuite) TestUpdateMachinesUsesCanonicalReplayPolicy() {
	require := s.Require()
	app := &model.Application{
		ID:                  47,
		Name:                "ReplayPolicy",
		IApplicationAddress: common.HexToAddress("0x47"),
		Status:              model.ApplicationStatus_OK,
		ProcessedInputs:     7,
	}
	repo := &MockMachineRepository{}
	repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
		return f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
	}), repository.Pagination{}, false).Return([]*model.Application{app}, uint64(1), nil).Once()
	repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
		return f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
	}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
	repo.On("GetLastSnapshot", mock.Anything, app.IApplicationAddress.String()).Return(nil, nil).Once()

	instance := &DummyMachineInstanceMock{application: app, processedInputs: 3}
	factory := &MockMachineInstanceFactory{Instance: instance}
	var capturedSource repository.ReplayRepository
	var capturedExecutor replay.Executor
	var capturedOptions replay.Options
	run := func(
		_ context.Context,
		source repository.ReplayRepository,
		executor replay.Executor,
		options replay.Options,
	) (replay.Result, error) {
		capturedSource = source
		capturedExecutor = executor
		capturedOptions = options
		return replay.Result{ReplayedInputs: options.ToInputExclusive - options.FromInput}, nil
	}
	manager := NewMachineManager(
		repo,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		false,
		23,
		WithInstanceFactory(factory),
		withReplayRun(run),
	)

	require.NoError(manager.UpdateMachines(context.Background()))
	require.Same(repo, capturedSource)
	require.Same(instance, capturedExecutor)
	require.Same(app, capturedOptions.Application)
	require.Equal(uint64(3), capturedOptions.FromInput)
	require.Equal(uint64(7), capturedOptions.ToInputExclusive)
	require.Equal(uint64(23), capturedOptions.BatchSize)
	require.Equal(repository.ReplayVerificationCanonical, capturedOptions.Verification)
	require.True(manager.HasMachine(app.ID))
	repo.AssertExpectations(s.T())
	manager.Close()
}

func (s *MachineManagerSuite) TestIsOnlyApplicationFailurePersistenceErrors() {
	localFailure := func(applicationID int64) error {
		return &ApplicationFailurePersistenceError{
			ApplicationID: applicationID,
			WriteErr:      errors.New("repository unavailable"),
		}
	}
	wrap := func(message string, err error) error {
		return fmt.Errorf("%s: %w", message, err)
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "plain non-wrapping error",
			err:  errors.New("global repository failure"),
			want: false,
		},
		{
			name: "single unwrapper returning nil",
			err:  nilSingleUnwrapperError{},
			want: false,
		},
		{
			name: "multi unwrapper returning empty slice",
			err:  emptyMultiUnwrapperError{},
			want: false,
		},
		{
			name: "single wrapper",
			err:  wrap("update machines", localFailure(1)),
			want: true,
		},
		{
			name: "multiple wrappers",
			err:  wrap("advancer step", wrap("update machines", localFailure(2))),
			want: true,
		},
		{
			name: "nested joins of wrapped local failures",
			err: errors.Join(
				wrap("first application", localFailure(3)),
				errors.Join(
					wrap("second application", wrap("status persistence", localFailure(4))),
					wrap("third application", localFailure(5)),
				),
			),
			want: true,
		},
		{
			name: "mixed join with global failure",
			err: errors.Join(
				wrap("application", localFailure(6)),
				errors.Join(
					wrap("another application", localFailure(7)),
					errors.New("global repository failure"),
				),
			),
			want: false,
		},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			s.Equal(test.want, IsOnlyApplicationFailurePersistenceErrors(test.err))
		})
	}
}

func (s *MachineManagerSuite) TestUpdateMachines() {
	s.Run("ReplayContradictionClosesAndDoesNotRegisterMachine", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		app := &model.Application{
			ID:                  71,
			Name:                "ReplayMismatch",
			IApplicationAddress: common.HexToAddress("0x71"),
			Enabled:             true,
			Status:              model.ApplicationStatus_OK,
		}
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{app}, uint64(1), nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
		repo.On("GetLastSnapshot", mock.Anything, app.IApplicationAddress.String()).Return(nil, nil).Once()
		const expectedReason = "replay contradicts persisted canonical result: " +
			"app=\"ReplayMismatch\" epoch=<unknown> input=0 " +
			"field=machine_hash expected= actual="
		repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
			mock.MatchedBy(func(reason *string) bool {
				return reason != nil && *reason == expectedReason
			})).Return(nil).Once()

		instance := &DummyMachineInstanceMock{
			application: app,
			replayErr:   &replay.ContradictionError{Application: app.Name, Field: "machine_hash"},
		}
		factory := &MockMachineInstanceFactory{Instance: instance}
		manager := newTestMachineManager(
			repo,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			false,
			10,
			WithInstanceFactory(factory),
		)

		require.NoError(manager.UpdateMachines(context.Background()))
		require.Equal(1, instance.closeCalls)
		require.Equal(1, factory.TemplateCalls)
		require.False(manager.HasMachine(app.ID))
		require.Empty(manager.Applications())
		require.Equal(model.ApplicationStatus_Failed, app.Status)
		require.NotNil(app.Reason)
		require.Empty(manager.pendingApplicationFailures)
		require.False(manager.HasPendingApplicationFailures())
		repo.AssertExpectations(s.T())

		// A fresh manager sees no executable row once FAILED is durable, so a
		// process restart cannot replay the contradictory history again.
		restartRepo := &MockMachineRepository{}
		restartRepo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
			Return([]*model.Application{}, uint64(0), nil).Twice()
		restartFactory := &MockMachineInstanceFactory{}
		restarted := newTestMachineManager(
			restartRepo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
			WithInstanceFactory(restartFactory),
		)
		require.NoError(restarted.UpdateMachines(context.Background()))
		require.Zero(restartFactory.TemplateCalls)
		restartRepo.AssertExpectations(s.T())
	})

	s.Run("ApplicationFailureStatusWriteRetriesWithoutReplay", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		app := &model.Application{
			ID: 75, Name: "RetryFailedStatus", IApplicationAddress: common.HexToAddress("0x75"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{app}, uint64(1), nil).Twice()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Twice()
		repo.On("GetLastSnapshot", mock.Anything, app.IApplicationAddress.String()).Return(nil, nil).Once()
		writeErr := errors.New("temporary status write failure")
		repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(writeErr).Once()
		repo.On("GetApplication", mock.Anything, app.IApplicationAddress.String()).Return(app, nil).Once()
		repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(nil).Once()

		instance := &DummyMachineInstanceMock{
			application: app,
			replayErr: &replay.ContradictionError{
				Application: app.Name, InputIndex: 9, Field: "outputs_hash",
				Expected: "0x01", Actual: "0x02",
			},
		}
		factory := &MockMachineInstanceFactory{Instance: instance}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
			WithInstanceFactory(factory),
		)

		err := manager.UpdateMachines(context.Background())
		require.ErrorIs(err, ErrApplicationFailureNotDurable)
		var persistenceErr *ApplicationFailurePersistenceError
		require.ErrorAs(err, &persistenceErr)
		require.ErrorIs(err, writeErr)
		require.Equal(app.ID, persistenceErr.ApplicationID)
		require.Equal(model.ApplicationStatus_OK, app.Status)
		require.Len(manager.pendingApplicationFailures, 1)
		require.True(manager.HasPendingApplicationFailures())
		require.Equal(1, factory.TemplateCalls)
		require.Equal(1, instance.closeCalls)
		require.False(manager.HasMachine(app.ID))

		// The next update retries only the FAILED write. The app remains fenced
		// even though this mock returns its stale executable row.
		require.NoError(manager.UpdateMachines(context.Background()))
		require.Equal(model.ApplicationStatus_Failed, app.Status)
		require.Empty(manager.pendingApplicationFailures)
		require.False(manager.HasPendingApplicationFailures())
		require.Equal(1, factory.TemplateCalls)
		require.Equal(1, instance.closeCalls)
		require.False(manager.HasMachine(app.ID))
		repo.AssertExpectations(s.T())
	})

	s.Run("ApplicationFailureFenceClearsWhenTerminalStatusWins", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		app := &model.Application{
			ID: 76, Name: "TerminalStatusWins", IApplicationAddress: common.HexToAddress("0x76"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		terminalApp := *app
		terminalApp.Status = model.ApplicationStatus_Corrupted
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{app}, uint64(1), nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
		repo.On("GetLastSnapshot", mock.Anything, app.IApplicationAddress.String()).Return(nil, nil).Once()
		repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(errors.New("terminal status transition rejected")).Once()
		repo.On("GetApplication", mock.Anything, app.IApplicationAddress.String()).
			Return(&terminalApp, nil).Once()

		instance := &DummyMachineInstanceMock{
			application: app,
			replayErr:   &replay.ContradictionError{Application: app.Name, Field: "machine_hash"},
		}
		factory := &MockMachineInstanceFactory{Instance: instance}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
			WithInstanceFactory(factory),
		)

		require.NoError(manager.UpdateMachines(context.Background()))
		require.Empty(manager.pendingApplicationFailures)
		require.False(manager.HasPendingApplicationFailures())
		require.Equal(1, factory.TemplateCalls)
		require.Equal(1, instance.closeCalls)
		require.False(manager.HasMachine(app.ID))
		repo.AssertExpectations(s.T())
	})

	s.Run("ApplicationFailureFenceRetainsUnrelatedFailedStatus", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		app := &model.Application{
			ID: 77, Name: "UnrelatedFailure", IApplicationAddress: common.HexToAddress("0x77"),
			Status: model.ApplicationStatus_OK,
		}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
		)
		manager.FenceApplicationFailure(app, replayContradictionReason(&replay.ContradictionError{
			Application: app.Name, Field: "machine_hash",
		}))

		unrelatedReason := "machine process crashed"
		durableApp := *app
		durableApp.Status = model.ApplicationStatus_Failed
		durableApp.Reason = &unrelatedReason
		repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(errors.New("status transition rejected")).Once()
		repo.On("GetApplication", mock.Anything, app.IApplicationAddress.String()).
			Return(&durableApp, nil).Once()

		err := manager.persistApplicationFailure(context.Background(), app.ID)

		require.ErrorIs(err, ErrApplicationFailureNotDurable)
		require.Len(manager.pendingApplicationFailures, 1)
		require.True(manager.HasPendingApplicationFailures())
		repo.AssertExpectations(s.T())
	})

	s.Run("AmbiguousWriteClearsFenceWhenExactFailedStatusIsDurable", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		app := &model.Application{
			ID: 78, Name: "MatchingFailure", IApplicationAddress: common.HexToAddress("0x78"),
			Status: model.ApplicationStatus_OK,
		}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
		)
		manager.FenceApplicationFailure(app, replayContradictionReason(&replay.ContradictionError{
			Application: app.Name, Field: "machine_hash",
		}))
		pending := manager.pendingApplicationFailures[app.ID]

		durableApp := *app
		durableApp.Status = model.ApplicationStatus_Failed
		durableApp.Reason = &pending.reason
		repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(errors.New("write result lost")).Once()
		repo.On("GetApplication", mock.Anything, app.IApplicationAddress.String()).
			Return(&durableApp, nil).Once()

		require.NoError(manager.persistApplicationFailure(context.Background(), app.ID))

		require.Empty(manager.pendingApplicationFailures)
		require.False(manager.HasPendingApplicationFailures())
		repo.AssertExpectations(s.T())
	})

	s.Run("AmbiguousWriteClearsFenceForNormalizedLongReason", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		app := &model.Application{
			ID: 81, Name: "LongReason", IApplicationAddress: common.HexToAddress("0x81"),
			Status: model.ApplicationStatus_OK,
		}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
		)
		detail := &replay.ContradictionError{
			Application: app.Name,
			Field:       "machine_hash",
			Expected:    strings.Repeat("e", 5000),
			Actual:      "different",
		}
		expectedReason := appstatus.NormalizeReason(detail.Error())
		manager.FenceApplicationFailure(app, replayContradictionReason(detail))
		pending := manager.pendingApplicationFailures[app.ID]
		require.Equal(expectedReason, pending.reason)

		durableApp := *app
		durableApp.Status = model.ApplicationStatus_Failed
		durableApp.Reason = &expectedReason
		repo.On(
			"UpdateApplicationStatus",
			mock.Anything,
			app.ID,
			model.ApplicationStatus_Failed,
			mock.MatchedBy(func(reason *string) bool {
				return reason != nil && *reason == expectedReason
			}),
		).Return(errors.New("write result lost")).Once()
		repo.On("GetApplication", mock.Anything, app.IApplicationAddress.String()).
			Return(&durableApp, nil).Once()

		require.NoError(manager.persistApplicationFailure(context.Background(), app.ID))
		require.False(manager.HasPendingApplicationFailures())
		repo.AssertExpectations(s.T())
	})

	s.Run("ApplicationFailureFenceClearsWhenStatusWriteFindsDeletedApplication", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		app := &model.Application{
			ID: 79, Name: "DeletedApplication", IApplicationAddress: common.HexToAddress("0x79"),
			Status: model.ApplicationStatus_OK,
		}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
		)
		manager.FenceApplicationFailure(app, replayContradictionReason(&replay.ContradictionError{
			Application: app.Name, Field: "machine_hash",
		}))

		repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(repository.ErrNotFound).Once()

		require.NoError(manager.persistApplicationFailure(context.Background(), app.ID))

		require.Empty(manager.pendingApplicationFailures)
		require.False(manager.HasPendingApplicationFailures())
		repo.AssertNotCalled(s.T(), "GetApplication", mock.Anything, mock.Anything)
		repo.AssertExpectations(s.T())
	})

	s.Run("ApplicationFailureFenceClearsWhenReadFindsDeletedApplication", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		app := &model.Application{
			ID: 82, Name: "DeletedApplicationReadback", IApplicationAddress: common.HexToAddress("0x82"),
			Status: model.ApplicationStatus_OK,
		}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
		)
		manager.FenceApplicationFailure(app, "machine execution failed")
		repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(errors.New("write result unavailable")).Once()
		repo.On("GetApplication", mock.Anything, app.IApplicationAddress.String()).
			Return(nil, nil).Once()

		require.NoError(manager.persistApplicationFailure(context.Background(), app.ID))
		require.False(manager.HasPendingApplicationFailures())
		repo.AssertExpectations(s.T())
	})

	s.Run("ApplicationFailureFenceClearsWhenAddressBelongsToReplacement", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		oldApp := &model.Application{
			ID: 82, Name: "OldApplication", IApplicationAddress: common.HexToAddress("0x82"),
			Status: model.ApplicationStatus_OK,
		}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
		)
		manager.FenceApplicationFailure(oldApp, replayContradictionReason(&replay.ContradictionError{
			Application: oldApp.Name, Field: "machine_hash",
		}))
		replacement := *oldApp
		replacement.ID = 83
		replacement.Name = "ReplacementApplication"

		repo.On(
			"UpdateApplicationStatus",
			mock.Anything,
			oldApp.ID,
			model.ApplicationStatus_Failed,
			mock.Anything,
		).Return(errors.New("write result unavailable")).Once()
		repo.On("GetApplication", mock.Anything, oldApp.IApplicationAddress.String()).
			Return(&replacement, nil).Once()

		require.NoError(manager.persistApplicationFailure(context.Background(), oldApp.ID))
		require.False(manager.HasPendingApplicationFailures())
		require.Equal(model.ApplicationStatus_OK, replacement.Status)
		require.Nil(replacement.Reason)
		repo.AssertNotCalled(
			s.T(),
			"UpdateApplicationStatus",
			mock.Anything,
			replacement.ID,
			mock.Anything,
			mock.Anything,
		)
		repo.AssertExpectations(s.T())
	})

	s.Run("ApplicationFailureFencePropagatesWriteAndReadFailure", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		app := &model.Application{
			ID: 80, Name: "UnconfirmedFailure", IApplicationAddress: common.HexToAddress("0x80"),
			Status: model.ApplicationStatus_OK,
		}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
		)
		manager.FenceApplicationFailure(app, replayContradictionReason(&replay.ContradictionError{
			Application: app.Name, Field: "machine_hash",
		}))
		writeErr := errors.New("status write unavailable")
		readErr := errors.New("status read unavailable")
		repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(writeErr).Once()
		repo.On("GetApplication", mock.Anything, app.IApplicationAddress.String()).
			Return(nil, readErr).Once()

		err := manager.persistApplicationFailure(context.Background(), app.ID)

		require.ErrorIs(err, ErrApplicationFailureNotDurable)
		require.ErrorIs(err, writeErr)
		require.ErrorIs(err, readErr)
		require.True(manager.HasPendingApplicationFailures())
		repo.AssertExpectations(s.T())
	})

	s.Run("ApplicationFailureLocalDeadlineDoesNotBlockHealthySibling", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		failed := &model.Application{
			ID: 91, Name: "LocalDeadline", IApplicationAddress: common.HexToAddress("0x91"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		healthy := &model.Application{
			ID: 92, Name: "HealthyAfterDeadline", IApplicationAddress: common.HexToAddress("0x92"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
			WithInstanceFactory(&MockMachineInstanceFactory{TemplateFn: func(app *model.Application) (MachineInstance, error) {
				return &DummyMachineInstanceMock{application: app}, nil
			}}),
		)
		manager.FenceApplicationFailure(failed, "machine execution failed")
		localDeadline := fmt.Errorf("repository statement timeout: %w", context.DeadlineExceeded)
		repo.On("UpdateApplicationStatus", mock.Anything, failed.ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(localDeadline).Once()
		repo.On("GetApplication", mock.Anything, failed.IApplicationAddress.String()).
			Return(failed, nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{failed, healthy}, uint64(2), nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
		repo.On("GetLastSnapshot", mock.Anything, healthy.IApplicationAddress.String()).Return(nil, nil).Once()

		err := manager.UpdateMachines(context.Background())

		require.ErrorIs(err, context.DeadlineExceeded)
		require.True(IsOnlyApplicationFailurePersistenceErrors(err))
		require.True(manager.HasPendingApplicationFailures())
		require.False(manager.HasMachine(failed.ID))
		require.True(manager.HasMachine(healthy.ID), "a repository-local timeout must not block the sibling")
		repo.AssertExpectations(s.T())
	})

	s.Run("PendingApplicationFailureCancellationPreservesErrorChainAndStops", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		manager := newTestMachineManager(repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10)
		apps := []*model.Application{
			{ID: 101, Name: "First", IApplicationAddress: common.HexToAddress("0x101")},
			{ID: 102, Name: "Canceled", IApplicationAddress: common.HexToAddress("0x102")},
			{ID: 103, Name: "MustNotRun", IApplicationAddress: common.HexToAddress("0x103")},
		}
		for _, app := range apps {
			manager.FenceApplicationFailure(app, "machine execution failed")
		}
		firstWriteErr := errors.New("first write unavailable")
		firstReadErr := errors.New("first read unavailable")
		repo.On("UpdateApplicationStatus", mock.Anything, apps[0].ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(firstWriteErr).Once()
		repo.On("GetApplication", mock.Anything, apps[0].IApplicationAddress.String()).
			Return(nil, firstReadErr).Once()
		ctx, cancel := context.WithCancel(context.Background())
		repo.On("UpdateApplicationStatus", mock.Anything, apps[1].ID, model.ApplicationStatus_Failed, mock.Anything).
			Run(func(mock.Arguments) { cancel() }).Return(context.Canceled).Once()
		repo.On("GetApplication", mock.Anything, apps[1].IApplicationAddress.String()).
			Return(nil, context.Canceled).Once()

		_, err := manager.persistPendingApplicationFailures(ctx)

		require.ErrorIs(err, firstWriteErr)
		require.ErrorIs(err, firstReadErr)
		require.ErrorIs(err, context.Canceled)
		var persistenceIDs []int64
		var collect func(error)
		collect = func(current error) {
			if current == nil {
				return
			}
			if typed, ok := current.(*ApplicationFailurePersistenceError); ok {
				persistenceIDs = append(persistenceIDs, typed.ApplicationID)
				return
			}
			if joined, ok := current.(interface{ Unwrap() []error }); ok {
				for _, child := range joined.Unwrap() {
					collect(child)
				}
			}
		}
		collect(err)
		require.ElementsMatch([]int64{apps[0].ID, apps[1].ID}, persistenceIDs)
		repo.AssertNotCalled(s.T(), "UpdateApplicationStatus", mock.Anything, apps[2].ID, mock.Anything, mock.Anything)
		require.Len(manager.pendingApplicationFailures, 3)
		repo.AssertExpectations(s.T())
	})

	s.Run("QueuedLiveFailureRetryClearsFenceAndRemovesOnlyAffectedMachine", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		failed := &model.Application{
			ID: 111, Name: "LiveFailure", IApplicationAddress: common.HexToAddress("0x111"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		healthy := &model.Application{
			ID: 112, Name: "Healthy", IApplicationAddress: common.HexToAddress("0x112"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		manager := newTestMachineManager(repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10)
		failedMachine := &DummyMachineInstanceMock{application: failed}
		healthyMachine := &DummyMachineInstanceMock{application: healthy}
		require.True(manager.addMachine(failed.ID, failedMachine))
		require.True(manager.addMachine(healthy.ID, healthyMachine))
		const reason = "advance execution reached configured cycle limit"
		manager.FenceApplicationFailure(failed, reason)
		repo.On("UpdateApplicationStatus", mock.Anything, failed.ID, model.ApplicationStatus_Failed,
			mock.MatchedBy(func(got *string) bool { return got != nil && *got == reason })).Return(nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{failed, healthy}, uint64(2), nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()

		require.NoError(manager.UpdateMachines(context.Background()))

		require.False(manager.HasPendingApplicationFailures())
		require.False(manager.HasMachine(failed.ID))
		require.True(manager.HasMachine(healthy.ID))
		require.Equal(1, failedMachine.closeCalls)
		require.Zero(healthyMachine.closeCalls)
		repo.AssertExpectations(s.T())
	})

	s.Run("TransientReplayErrorRetries", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		app := &model.Application{
			ID: 72, Name: "TransientReplay", IApplicationAddress: common.HexToAddress("0x72"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{app}, uint64(1), nil).Twice()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Twice()
		repo.On("GetLastSnapshot", mock.Anything, app.IApplicationAddress.String()).Return(nil, nil).Twice()

		var instances []*DummyMachineInstanceMock
		factory := &MockMachineInstanceFactory{TemplateFn: func(got *model.Application) (MachineInstance, error) {
			instance := &DummyMachineInstanceMock{application: got}
			if len(instances) == 0 {
				instance.replayErr = errors.New("temporary repository outage")
			}
			instances = append(instances, instance)
			return instance, nil
		}}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
			WithInstanceFactory(factory),
		)

		require.NoError(manager.UpdateMachines(context.Background()))
		require.Len(instances, 1)
		require.Equal(1, instances[0].closeCalls)
		require.Empty(manager.pendingApplicationFailures)
		require.False(manager.HasMachine(app.ID))

		require.NoError(manager.UpdateMachines(context.Background()))
		require.Len(instances, 2)
		require.Equal(2, factory.TemplateCalls)
		require.True(manager.HasMachine(app.ID))
		require.Empty(manager.pendingApplicationFailures)
		repo.AssertExpectations(s.T())
	})

	s.Run("ReplayContradictionDoesNotBlockHealthySibling", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		badApp := &model.Application{
			ID: 73, Name: "BadReplay", IApplicationAddress: common.HexToAddress("0x73"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		goodApp := &model.Application{
			ID: 74, Name: "GoodReplay", IApplicationAddress: common.HexToAddress("0x74"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).
			Return([]*model.Application{badApp, goodApp}, uint64(2), nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).Return(nil, nil).Twice()
		repo.On("UpdateApplicationStatus", mock.Anything, badApp.ID, model.ApplicationStatus_Failed,
			mock.Anything).Return(nil).Once()

		instances := map[int64]*DummyMachineInstanceMock{}
		factory := &MockMachineInstanceFactory{TemplateFn: func(app *model.Application) (MachineInstance, error) {
			instance := &DummyMachineInstanceMock{application: app}
			if app.ID == badApp.ID {
				instance.replayErr = &replay.ContradictionError{
					Application: app.Name, InputIndex: 4, Field: "outputs_hash",
					Expected: "0x01", Actual: "0x02",
				}
			}
			instances[app.ID] = instance
			return instance, nil
		}}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
			WithInstanceFactory(factory),
		)

		require.NoError(manager.UpdateMachines(context.Background()))
		require.False(manager.HasMachine(badApp.ID))
		require.True(manager.HasMachine(goodApp.ID))
		require.Equal(1, instances[badApp.ID].closeCalls)
		require.Zero(instances[goodApp.ID].closeCalls)
		require.Equal(model.ApplicationStatus_Failed, badApp.Status)
		require.Equal(model.ApplicationStatus_OK, goodApp.Status)
		require.Empty(manager.pendingApplicationFailures)
		require.False(manager.HasPendingApplicationFailures())
		repo.AssertExpectations(s.T())
	})

	s.Run("UnconfirmedApplicationFailureDoesNotBlockHealthySibling", func() {
		require := s.Require()
		repo := &MockMachineRepository{}
		badApp := &model.Application{
			ID: 83, Name: "BadUndurableReplay", IApplicationAddress: common.HexToAddress("0x83"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		goodApp := &model.Application{
			ID: 84, Name: "HealthyReplay", IApplicationAddress: common.HexToAddress("0x84"),
			Enabled: true, Status: model.ApplicationStatus_OK,
		}
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).
			Return([]*model.Application{badApp, goodApp}, uint64(2), nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).Return(nil, nil).Twice()
		writeErr := errors.New("status write unavailable")
		readErr := errors.New("status read unavailable")
		repo.On("UpdateApplicationStatus", mock.Anything, badApp.ID, model.ApplicationStatus_Failed, mock.Anything).
			Return(writeErr).Once()
		repo.On("GetApplication", mock.Anything, badApp.IApplicationAddress.String()).
			Return(nil, readErr).Once()

		instances := map[int64]*DummyMachineInstanceMock{}
		factory := &MockMachineInstanceFactory{TemplateFn: func(app *model.Application) (MachineInstance, error) {
			instance := &DummyMachineInstanceMock{application: app}
			if app.ID == badApp.ID {
				instance.replayErr = &replay.ContradictionError{
					Application: app.Name, InputIndex: 4, Field: "outputs_hash",
				}
			}
			instances[app.ID] = instance
			return instance, nil
		}}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
			WithInstanceFactory(factory),
		)

		err := manager.UpdateMachines(context.Background())
		require.ErrorIs(err, ErrApplicationFailureNotDurable)
		require.ErrorIs(err, writeErr)
		require.ErrorIs(err, readErr)
		require.False(manager.HasMachine(badApp.ID))
		require.True(manager.HasMachine(goodApp.ID))
		require.Equal(1, instances[badApp.ID].closeCalls)
		require.Zero(instances[goodApp.ID].closeCalls)
		require.True(manager.HasPendingApplicationFailures())
		repo.AssertExpectations(s.T())
	})

	replayFailures := []struct {
		name           string
		err            error
		reasonContains string
	}{
		{"PayloadLengthLimit", machine.ErrPayloadLengthLimitExceeded, "execution limit"},
		{"OutputsLimit", machine.ErrOutputsLimitExceeded, "execution limit"},
		{"ReportsLimit", machine.ErrReportsLimitExceeded, "execution limit"},
		{"McycleLimit", machine.ErrReachedLimitMcycle, "execution limit"},
		{"Deadline", machine.ErrDeadlineExceeded, "execution deadline"},
		{"IncompleteAdvance", ErrIncompleteAdvance, "incomplete advance result"},
		{"MachineInternal", machine.ErrMachineInternal, "failed internally"},
	}
	for index, failure := range replayFailures {
		s.Run("ReplayFailure/"+failure.name, func() {
			require := s.Require()
			repo := &MockMachineRepository{}
			failedID := int64(85 + index*2)
			failed := &model.Application{
				ID: failedID, Name: "FailedReplay",
				IApplicationAddress: common.BigToAddress(big.NewInt(failedID)),
				Enabled:             true, Status: model.ApplicationStatus_OK,
			}
			healthy := &model.Application{
				ID: failedID + 1, Name: "HealthyReplay",
				IApplicationAddress: common.BigToAddress(big.NewInt(failedID + 1)),
				Enabled:             true, Status: model.ApplicationStatus_OK,
			}
			repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
				return f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
			}), repository.Pagination{}, false).
				Return([]*model.Application{failed, healthy}, uint64(2), nil).Once()
			repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
				return f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
			}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
			repo.On("GetLastSnapshot", mock.Anything, mock.Anything).Return(nil, nil).Twice()
			repo.On("UpdateApplicationStatus", mock.Anything, failed.ID, model.ApplicationStatus_Failed,
				mock.MatchedBy(func(reason *string) bool {
					return reason != nil && strings.Contains(*reason, failure.reasonContains)
				})).Return(nil).Once()

			instances := map[int64]*DummyMachineInstanceMock{}
			factory := &MockMachineInstanceFactory{TemplateFn: func(app *model.Application) (MachineInstance, error) {
				instance := &DummyMachineInstanceMock{application: app}
				if app.ID == failed.ID {
					instance.replayErr = fmt.Errorf("replay input: %w", failure.err)
				}
				instances[app.ID] = instance
				return instance, nil
			}}
			manager := newTestMachineManager(
				repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
				WithInstanceFactory(factory),
			)

			require.NoError(manager.UpdateMachines(context.Background()))
			require.False(manager.HasMachine(failed.ID))
			require.True(manager.HasMachine(healthy.ID))
			require.Equal(model.ApplicationStatus_Failed, failed.Status)
			require.Equal(model.ApplicationStatus_OK, healthy.Status)
			require.Equal(1, instances[failed.ID].closeCalls)
			require.Zero(instances[healthy.ID].closeCalls)
			require.False(manager.HasPendingApplicationFailures())
			repo.AssertExpectations(s.T())
		})
	}

	s.Run("AddNewMachines", func() {
		require := s.Require()

		// Setup repository with enabled applications
		repo := &MockMachineRepository{}
		app1 := &model.Application{
			ID:                  1,
			Name:                "App1",
			IApplicationAddress: common.HexToAddress("0x1"),
			Status:              model.ApplicationStatus_OK,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Application{app1}, uint64(1), nil)

		// Mock GetLastSnapshot to return nil (no snapshot available)
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(nil, nil)

		// Create manager with a mock instance factory
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		mockInstance := &DummyMachineInstanceMock{application: app1}
		factory := &MockMachineInstanceFactory{Instance: mockInstance}
		manager := newTestMachineManager(repo, testLogger, false, 500, WithInstanceFactory(factory))

		err := manager.UpdateMachines(context.Background())
		require.NoError(err)
		require.True(manager.HasMachine(1))

		repo.AssertCalled(s.T(), "ListApplications", mock.Anything, mock.Anything, mock.Anything, false)
	})

	s.Run("AddsMachineForForeclosedAppWithUndrainedInputs", func() {
		require := s.Require()

		repo := &MockMachineRepository{}
		app := &model.Application{
			ID:                  1,
			Name:                "ForeclosedApp",
			IApplicationAddress: common.HexToAddress("0x1"),
			Enabled:             true,
			Status:              model.ApplicationStatus_OK,
			ForecloseBlock:      100,
			// Caught up (last_input_check_block >= foreclose_block), so the
			// drain query below is trustworthy and gets consulted.
			LastInputCheckBlock: 100,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		// The executable filter selects OK apps that have not foreclosed; the
		// drain filter selects OK apps that have. They are distinguished by
		// ForeclosureRecorded, since a foreclosed app keeps status OK.
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Status != nil && *f.Status == model.ApplicationStatus_OK &&
				f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Status != nil && *f.Status == model.ApplicationStatus_OK &&
				f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{app}, uint64(1), nil).Once()
		repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).
			Return(true, nil).Once()
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).Return(nil, nil)

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		mockInstance := &DummyMachineInstanceMock{application: app}
		factory := &MockMachineInstanceFactory{Instance: mockInstance}
		manager := newTestMachineManager(repo, testLogger, false, 500, WithInstanceFactory(factory))

		err := manager.UpdateMachines(context.Background())
		require.NoError(err)
		require.True(manager.HasMachine(app.ID))
		repo.AssertExpectations(s.T())
	})

	s.Run("KeepsMachineForForeclosedAppBeforeScanCaughtUp", func() {
		require := s.Require()

		repo := &MockMachineRepository{}
		// Foreclosed app whose historical scan has NOT yet reached
		// foreclose_block (last_input_check_block < foreclose_block). The
		// inputs/epochs table is still incomplete, so the drain query would
		// answer "nothing to drain" on an empty table. The manager must keep
		// the machine and must NOT consult HasUndrainedEpochsBeforeBlock yet.
		app := &model.Application{
			ID:                  1,
			Name:                "ForeclosedAppBootstrapping",
			IApplicationAddress: common.HexToAddress("0x1"),
			Enabled:             true,
			Status:              model.ApplicationStatus_OK,
			ForecloseBlock:      100,
			LastInputCheckBlock: 99,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Status != nil && *f.Status == model.ApplicationStatus_OK &&
				f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Status != nil && *f.Status == model.ApplicationStatus_OK &&
				f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{app}, uint64(1), nil).Once()
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).Return(nil, nil)

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		mockInstance := &DummyMachineInstanceMock{application: app}
		factory := &MockMachineInstanceFactory{Instance: mockInstance}
		manager := newTestMachineManager(repo, testLogger, false, 500, WithInstanceFactory(factory))

		err := manager.UpdateMachines(context.Background())
		require.NoError(err)
		require.True(manager.HasMachine(app.ID), "machine kept while the scan catches up")
		repo.AssertExpectations(s.T())
		repo.AssertNotCalled(s.T(), "HasUndrainedEpochsBeforeBlock", mock.Anything, mock.Anything, mock.Anything)
	})

	s.Run("RemoveDisabledMachines", func() {
		require := s.Require()

		// Create a mock repository
		repo := &MockMachineRepository{}
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(nil, nil)

		// Create a test logger
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		manager := newTestMachineManager(repo, testLogger, false, 500)

		// Add mock machines
		app1 := &model.Application{ID: 1, Name: "App1"}
		app2 := &model.Application{ID: 2, Name: "App2"}
		app3 := &model.Application{ID: 3, Name: "App3"}

		mockMachine1 := &DummyMachineInstanceMock{application: app1}
		mockMachine2 := &DummyMachineInstanceMock{application: app2}
		mockMachine3 := &DummyMachineInstanceMock{application: app3}

		manager.addMachine(1, mockMachine1)
		manager.addMachine(2, mockMachine2)
		manager.addMachine(3, mockMachine3)

		// Remove machines not in the active list
		manager.removeMachines([]*model.Application{app1, app3})

		// Verify machine2 was removed
		require.Len(manager.machines, 2)
		require.True(manager.HasMachine(1))
		require.False(manager.HasMachine(2))
		require.True(manager.HasMachine(3))
	})
}

func (s *MachineManagerSuite) TestSnapshotStartingStateVerification() {
	newApp := func(id int64, processed uint64) *model.Application {
		return &model.Application{
			ID:                  id,
			Name:                "SnapshotApp",
			IApplicationAddress: common.BigToAddress(new(big.Int).SetInt64(id)),
			Enabled:             true,
			Status:              model.ApplicationStatus_OK,
			ProcessedInputs:     processed,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    time.Second,
				InspectMaxDeadline:    time.Second,
				LoadDeadline:          time.Second,
				MaxConcurrentInspects: 3,
			},
		}
	}
	prepareRepo := func(
		app *model.Application,
		snapshot *model.Input,
		snapshotErr error,
	) *MockMachineRepository {
		repo := &MockMachineRepository{
			replayApplicationID: app.ID,
			replayConsensus:     app.ConsensusType,
		}
		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Application{app}, uint64(1), nil)
		repo.On("GetLastSnapshot", mock.Anything, app.IApplicationAddress.String()).
			Return(snapshot, snapshotErr).Once()
		return repo
	}
	configureReplay := func(repo *MockMachineRepository, app *model.Application) common.Hash {
		machineHash := newHash(1)
		outputsHash := newHash(2)
		repo.replayCount = app.ProcessedInputs
		repo.replayRecords = []*model.ReplayRecord{{
			Input: model.ReplayInput{
				ApplicationID: app.ID,
				EpochIndex:    0,
				InputIndex:    0,
				RawData:       []byte("replayed input"),
				Status:        model.InputCompletionStatus_Accepted,
				MachineHash:   &machineHash,
				OutputsHash:   &outputsHash,
			},
		}}
		return machineHash
	}
	newReplayTemplate := func(app *model.Application) MachineInstance {
		base := &MockRollupsMachine{
			HashReturn:        newHash(0),
			OutputsHashReturn: newHash(0),
		}
		base.ForkReturn = newForkableMock()
		instance, err := NewMachineInstanceWithFactory(
			context.Background(), app, 0,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			&MockMachineRuntimeFactory{RuntimeToReturn: base},
		)
		s.Require().NoError(err)
		return instance
	}
	assertTemplateReplay := func(
		require *require.Assertions,
		manager *MachineManager,
		repo *MockMachineRepository,
		app *model.Application,
		expectedHash common.Hash,
	) {
		machineInstance, exists := manager.GetMachine(app.ID)
		require.True(exists)
		require.Equal(app.ProcessedInputs, machineInstance.ProcessedInputs())
		actualHash, err := machineInstance.Hash(context.Background())
		require.NoError(err)
		require.Equal([32]byte(expectedHash), actualHash)
		require.Positive(repo.replayListCalls, "template fallback must replay canonical inputs")
	}

	s.Run("MatchingCaughtUpSnapshotIsAccepted", func() {
		require := s.Require()
		app := newApp(81, 3)
		path := s.T().TempDir()
		hash := common.HexToHash("0x81")
		snapshot := &model.Input{Index: 2, SnapshotURI: &path, MachineHash: &hash}
		repo := prepareRepo(app, snapshot, nil)
		repo.replayCount = 3
		snapshotInstance := &DummyMachineInstanceMock{
			application: app, processedInputs: 3, hashReturn: [32]byte(hash),
		}
		factory := &MockMachineInstanceFactory{SnapshotInstance: snapshotInstance}
		manager := newTestMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
			WithInstanceFactory(factory),
		)

		require.NoError(manager.UpdateMachines(context.Background()))
		require.Equal(1, factory.SnapshotCalls)
		require.Zero(factory.TemplateCalls)
		require.Equal(1, snapshotInstance.hashCalls)
		require.Equal(1, snapshotInstance.replayCalls)
		require.True(manager.HasMachine(app.ID))
		manager.Close()
	})

	fallbackTests := []struct {
		name          string
		snapshot      func(path string, expectedHash common.Hash) *model.Input
		repositoryErr error
		candidate     func(app *model.Application, expectedHash common.Hash) *DummyMachineInstanceMock
		factoryErr    error
		wantCalls     int
		wantHashCalls int
	}{
		{
			name: "missing persisted hash",
			snapshot: func(path string, _ common.Hash) *model.Input {
				return &model.Input{Index: 0, SnapshotURI: &path}
			},
			wantCalls: 0,
		},
		{
			name: "repository result accompanied by error",
			snapshot: func(path string, expectedHash common.Hash) *model.Input {
				return &model.Input{Index: 0, SnapshotURI: &path, MachineHash: &expectedHash}
			},
			repositoryErr: errors.New("snapshot query failed"),
			wantCalls:     0,
		},
		{
			name: "snapshot input index cannot be incremented",
			snapshot: func(path string, expectedHash common.Hash) *model.Input {
				return &model.Input{Index: math.MaxUint64, SnapshotURI: &path, MachineHash: &expectedHash}
			},
			wantCalls: 0,
		},
		{
			name: "snapshot is beyond application progress",
			snapshot: func(path string, expectedHash common.Hash) *model.Input {
				return &model.Input{Index: 1, SnapshotURI: &path, MachineHash: &expectedHash}
			},
			wantCalls: 0,
		},
		{
			name: "matching hash with wrong processed input count",
			snapshot: func(path string, expectedHash common.Hash) *model.Input {
				return &model.Input{Index: 0, SnapshotURI: &path, MachineHash: &expectedHash}
			},
			candidate: func(app *model.Application, expectedHash common.Hash) *DummyMachineInstanceMock {
				return &DummyMachineInstanceMock{
					application: app, processedInputs: 0, hashReturn: [32]byte(expectedHash),
				}
			},
			wantCalls: 1,
		},
		{
			name: "manager boundary hash mismatch",
			snapshot: func(path string, expectedHash common.Hash) *model.Input {
				return &model.Input{Index: 0, SnapshotURI: &path, MachineHash: &expectedHash}
			},
			candidate: func(app *model.Application, _ common.Hash) *DummyMachineInstanceMock {
				return &DummyMachineInstanceMock{application: app, processedInputs: 1, hashReturn: newHash(99)}
			},
			wantCalls:     1,
			wantHashCalls: 1,
		},
		{
			name: "manager boundary hash read error",
			snapshot: func(path string, expectedHash common.Hash) *model.Input {
				return &model.Input{Index: 0, SnapshotURI: &path, MachineHash: &expectedHash}
			},
			candidate: func(app *model.Application, expectedHash common.Hash) *DummyMachineInstanceMock {
				return &DummyMachineInstanceMock{
					application: app, processedInputs: 1, hashReturn: [32]byte(expectedHash),
					hashError: errors.New("snapshot hash unavailable"),
				}
			},
			wantCalls:     1,
			wantHashCalls: 1,
		},
		{
			name: "factory returns partial candidate with error",
			snapshot: func(path string, expectedHash common.Hash) *model.Input {
				return &model.Input{Index: 0, SnapshotURI: &path, MachineHash: &expectedHash}
			},
			candidate: func(app *model.Application, expectedHash common.Hash) *DummyMachineInstanceMock {
				return &DummyMachineInstanceMock{
					application: app, processedInputs: 1, hashReturn: [32]byte(expectedHash),
				}
			},
			factoryErr: errors.New("factory failed after creating candidate"),
			wantCalls:  1,
		},
	}

	for i, tt := range fallbackTests {
		s.Run(tt.name, func() {
			require := s.Require()
			app := newApp(82+int64(i), 1)
			path := s.T().TempDir()
			expectedHash := newHash(1)
			snapshot := tt.snapshot(path, expectedHash)
			repo := prepareRepo(app, snapshot, tt.repositoryErr)
			require.Equal(expectedHash, configureReplay(repo, app))
			candidate := (*DummyMachineInstanceMock)(nil)
			if tt.candidate != nil {
				candidate = tt.candidate(app, expectedHash)
			}
			template := newReplayTemplate(app)
			factory := &MockMachineInstanceFactory{
				Instance: template, SnapshotInstance: candidate, SnapshotErr: tt.factoryErr,
			}
			manager := NewMachineManager(
				repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
				WithInstanceFactory(factory),
			)

			require.NoError(manager.UpdateMachines(context.Background()))
			require.Equal(tt.wantCalls, factory.SnapshotCalls)
			require.Equal(1, factory.TemplateCalls)
			require.Equal([]bool{false}, factory.TemplateChecks)
			if candidate != nil {
				require.Equal(1, candidate.closeCalls)
				require.Equal(tt.wantHashCalls, candidate.hashCalls)
				require.Zero(candidate.replayCalls)
			}
			assertTemplateReplay(require, manager, repo, app, expectedHash)
			manager.Close()
		})
	}

	s.Run("SnapshotFactoryNilResultFallsBackToTemplate", func() {
		require := s.Require()
		app := newApp(90, 1)
		path := s.T().TempDir()
		expectedHash := common.Hash{}
		snapshot := &model.Input{Index: 0, SnapshotURI: &path, MachineHash: &expectedHash}
		repo := prepareRepo(app, snapshot, nil)
		expectedHash = configureReplay(repo, app)
		snapshot.MachineHash = &expectedHash
		template := newReplayTemplate(app)
		factory := &MockMachineInstanceFactory{Instance: template, ReturnNilSnapshot: true}
		manager := NewMachineManager(
			repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
			WithInstanceFactory(factory),
		)

		require.NoError(manager.UpdateMachines(context.Background()))
		require.Equal(1, factory.SnapshotCalls)
		require.Equal(1, factory.TemplateCalls)
		assertTemplateReplay(require, manager, repo, app, expectedHash)
		manager.Close()
	})
}

func (s *MachineManagerSuite) TestTemplateFactoryResultHandling() {
	tests := []struct {
		name       string
		candidate  *DummyMachineInstanceMock
		factoryErr error
	}{
		{name: "nil instance without error"},
		{
			name: "partial instance with error",
			candidate: &DummyMachineInstanceMock{
				closeError: errors.New("partial template close failed"),
			},
			factoryErr: errors.New("template factory failed"),
		},
	}

	for i, tt := range tests {
		s.Run(tt.name, func() {
			require := s.Require()
			app := &model.Application{
				ID:                  91 + int64(i),
				Name:                "TemplateFactoryResultApp",
				IApplicationAddress: common.BigToAddress(big.NewInt(91 + int64(i))),
				Enabled:             true,
				Status:              model.ApplicationStatus_OK,
			}
			if tt.candidate != nil {
				tt.candidate.application = app
			}
			repo := &MockMachineRepository{}
			repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
				Return([]*model.Application{app}, uint64(1), nil)
			repo.On("GetLastSnapshot", mock.Anything, app.IApplicationAddress.String()).
				Return(nil, nil).Once()
			factory := &MockMachineInstanceFactory{Err: tt.factoryErr}
			if tt.candidate != nil {
				factory.Instance = tt.candidate
			}
			manager := newTestMachineManager(
				repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 10,
				WithInstanceFactory(factory),
			)

			require.NoError(manager.UpdateMachines(context.Background()))
			require.Equal(1, factory.TemplateCalls)
			require.False(manager.HasMachine(app.ID))
			if tt.candidate != nil {
				require.Equal(1, tt.candidate.closeCalls)
				require.Zero(tt.candidate.replayCalls)
			}
			manager.Close()
		})
	}
}

func (s *MachineManagerSuite) TestGetMachine() {
	require := s.Require()

	repo := &MockMachineRepository{}
	repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
		Return(nil, nil)

	manager := newTestMachineManager(repo, nil, false, 500)
	machine := &DummyMachineInstanceMock{application: &model.Application{ID: 1}}

	// Add a machine
	manager.addMachine(1, machine)

	// Test retrieval
	retrieved, exists := manager.GetMachine(1)
	require.True(exists)
	require.Same(machine, retrieved)

	// Test non-existent machine
	_, exists = manager.GetMachine(2)
	require.False(exists)
}

func (s *MachineManagerSuite) TestHasMachine() {
	require := s.Require()

	repo := &MockMachineRepository{}
	repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
		Return(nil, nil)

	manager := newTestMachineManager(repo, nil, false, 500)
	machine := &DummyMachineInstanceMock{application: &model.Application{ID: 1}}

	// Add a machine
	manager.addMachine(1, machine)

	// Test has machine
	require.True(manager.HasMachine(1))

	// Test doesn't have machine
	require.False(manager.HasMachine(2))
}

func (s *MachineManagerSuite) TestAddMachine() {
	require := s.Require()

	repo := &MockMachineRepository{}
	repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
		Return(nil, nil)

	manager := newTestMachineManager(repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 500)
	machine1 := &DummyMachineInstanceMock{application: &model.Application{ID: 1}}
	machine2 := &DummyMachineInstanceMock{application: &model.Application{ID: 2}}

	// Add first machine
	added := manager.addMachine(1, machine1)
	require.True(added)
	require.Len(manager.machines, 1)

	// Add second machine
	added = manager.addMachine(2, machine2)
	require.True(added)
	require.Len(manager.machines, 2)

	// Try to add duplicate
	added = manager.addMachine(1, machine1)
	require.False(added)
	require.Len(manager.machines, 2)

	// Close the manager and try to add a new machine
	manager.Close()

	machine3 := &DummyMachineInstanceMock{application: &model.Application{ID: 3}}
	added = manager.addMachine(3, machine3)
	require.False(added, "addMachine must reject additions after Close")
}

func (s *MachineManagerSuite) TestRemoveDisabledMachines() {
	require := s.Require()

	manager := newTestMachineManager(nil, nil, false, 500)

	// Add machines
	app1 := &model.Application{ID: 1}
	app2 := &model.Application{ID: 2}
	app3 := &model.Application{ID: 3}

	machine1 := &DummyMachineInstanceMock{application: app1}
	machine2 := &DummyMachineInstanceMock{application: app2}
	machine3 := &DummyMachineInstanceMock{application: app3}

	manager.addMachine(1, machine1)
	manager.addMachine(2, machine2)
	manager.addMachine(3, machine3)

	// Remove machines not in the active list
	manager.removeMachines([]*model.Application{app1, app3})

	// Verify machine2 was removed
	require.Len(manager.machines, 2)
	require.True(manager.HasMachine(1))
	require.False(manager.HasMachine(2))
	require.True(manager.HasMachine(3))
}

func (s *MachineManagerSuite) TestUpdateMachinesErrors() {
	s.Run("GetExecutableApplicationsError", func() {
		require := s.Require()

		repo := &MockMachineRepository{}
		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return(([]*model.Application)(nil), uint64(0), errors.New("db error"))

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		manager := newTestMachineManager(repo, testLogger, false, 500)

		err := manager.UpdateMachines(context.Background())
		require.Error(err)
		require.Contains(err.Error(), "db error")
	})

	s.Run("SnapshotCreationFailureFallsBackToTemplate", func() {
		require := s.Require()

		app := &model.Application{
			ID:                  1,
			Name:                "App1",
			IApplicationAddress: common.HexToAddress("0x1"),
			Status:              model.ApplicationStatus_OK,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		snapshotPath := "/fake/snapshot/path"
		snapshotInput := &model.Input{
			Index:       2,
			SnapshotURI: &snapshotPath,
		}

		repo := &MockMachineRepository{}
		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Application{app}, uint64(1), nil)
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(snapshotInput, nil)
		// The snapshot path doesn't exist, so it should fall back to template
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		mockInstance := &DummyMachineInstanceMock{application: app}
		factory := &MockMachineInstanceFactory{Instance: mockInstance}
		manager := newTestMachineManager(repo, testLogger, false, 500, WithInstanceFactory(factory))

		err := manager.UpdateMachines(context.Background())
		require.NoError(err)
		require.True(manager.HasMachine(1))
	})

	s.Run("TemplateCreationFailureSkipsApp", func() {
		require := s.Require()

		app := &model.Application{
			ID:                  1,
			Name:                "App1",
			IApplicationAddress: common.HexToAddress("0x1"),
			Status:              model.ApplicationStatus_OK,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		repo := &MockMachineRepository{}
		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Application{app}, uint64(1), nil)
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(nil, nil)

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		factory := &MockMachineInstanceFactory{Err: errors.New("machine creation failed")}
		manager := newTestMachineManager(repo, testLogger, false, 500, WithInstanceFactory(factory))

		err := manager.UpdateMachines(context.Background())
		// UpdateMachines should not return an error; it logs and skips
		require.NoError(err)
		require.False(manager.HasMachine(1))
	})

	s.Run("SynchronizeFailureClosesAndSkipsApp", func() {
		require := s.Require()

		app := &model.Application{
			ID:                  1,
			Name:                "App1",
			IApplicationAddress: common.HexToAddress("0x1"),
			Status:              model.ApplicationStatus_OK,
			ProcessedInputs:     3,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		repo := &MockMachineRepository{}
		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Application{app}, uint64(1), nil)
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(nil, nil)
		repo.replayCountError = errors.New("db connection lost")

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

		// Use a factory that builds a real MachineInstanceImpl (with a mock runtime)
		// so that Synchronize actually runs and hits the repo.
		mockRuntime := &MockRollupsMachine{}
		runtimeFactory := &MockMachineRuntimeFactory{
			RuntimeToReturn: mockRuntime,
			ErrorToReturn:   nil,
		}
		realFactory := &realMachineInstanceFactory{runtimeFactory: runtimeFactory}
		manager := NewMachineManager(repo, testLogger, false, 500, WithInstanceFactory(realFactory))

		err := manager.UpdateMachines(context.Background())
		require.NoError(err)
		// Machine should NOT have been added due to sync failure
		require.False(manager.HasMachine(1))
	})
}

// captureLogger returns a logger whose output is written to buf. Level is set
// to debug so every call is recorded.
func captureLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func (s *MachineManagerSuite) TestCloseAggregatesErrors() {
	require := s.Require()

	var buf bytes.Buffer
	manager := newTestMachineManager(nil, captureLogger(&buf), false, 500)

	machine1 := &DummyMachineInstanceMock{application: &model.Application{ID: 1}}
	machine2 := &DummyMachineInstanceMock{
		application: &model.Application{ID: 2},
		closeError:  errors.New("close error 2"),
	}
	machine3 := &DummyMachineInstanceMock{
		application: &model.Application{ID: 3},
		closeError:  errors.New("close error 3"),
	}

	manager.addMachine(1, machine1)
	manager.addMachine(2, machine2)
	manager.addMachine(3, machine3)

	manager.Close()

	logContents := buf.String()
	require.Contains(logContents, "close error 2")
	require.Contains(logContents, "close error 3")
	require.Empty(manager.machines)
}

func (s *MachineManagerSuite) TestApplications() {
	require := s.Require()

	repo := &MockMachineRepository{}
	repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
		Return(nil, nil)

	manager := newTestMachineManager(repo, nil, false, 500)

	// Add machines
	app1 := &model.Application{ID: 1, Name: "App1"}
	app2 := &model.Application{ID: 2, Name: "App2"}

	machine1 := &DummyMachineInstanceMock{application: app1}
	machine2 := &DummyMachineInstanceMock{application: app2}

	manager.addMachine(1, machine1)
	manager.addMachine(2, machine2)

	// Get applications
	apps := manager.Applications()
	require.Len(apps, 2)

	// Verify apps are in the list (order not guaranteed)
	appMap := make(map[int64]*model.Application)
	for _, app := range apps {
		appMap[app.ID] = app
	}

	require.Contains(appMap, int64(1))
	require.Contains(appMap, int64(2))
	require.Equal("App1", appMap[1].Name)
	require.Equal("App2", appMap[2].Name)
}

// Mock repository for testing
type MockMachineRepository struct {
	mock.Mock
	replayApplicationID int64
	replayConsensus     model.Consensus
	replayCount         uint64
	replayCountError    error
	replayRecords       []*model.ReplayRecord
	replayListCalls     int
}

func (m *MockMachineRepository) ReplaySummary(
	_ context.Context,
	_ common.Address,
	_ repository.ReplayVerificationLevel,
) (model.ReplaySummary, error) {
	return model.ReplaySummary{
		ApplicationID:   m.replayApplicationID,
		ProcessedInputs: m.replayCount,
		Consensus:       m.replayConsensus,
	}, m.replayCountError
}

func (m *MockMachineRepository) ReplayPage(
	_ context.Context,
	request repository.ReplayPageRequest,
) ([]*model.ReplayRecord, error) {
	m.replayListCalls++
	records := make([]*model.ReplayRecord, 0, min(request.Limit, uint64(len(m.replayRecords))))
	for _, record := range m.replayRecords {
		index := record.Input.InputIndex
		if index < request.FromInput || index >= request.ToInputExclusive {
			continue
		}
		records = append(records, record)
		if uint64(len(records)) == request.Limit {
			break
		}
	}
	return records, nil
}

func (m *MockMachineRepository) ListApplications(
	ctx context.Context,
	f repository.ApplicationFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Application, uint64, error) {
	args := m.Called(ctx, f, p, descending)
	return args.Get(0).([]*model.Application), args.Get(1).(uint64), args.Error(2)
}

func (m *MockMachineRepository) HasUndrainedEpochsBeforeBlock(
	ctx context.Context,
	appID int64,
	blockBound uint64,
) (bool, error) {
	args := m.Called(ctx, appID, blockBound)
	return args.Bool(0), args.Error(1)
}

func (m *MockMachineRepository) GetLastSnapshot(
	ctx context.Context,
	nameOrAddress string) (*model.Input, error) {
	args := m.Called(ctx, nameOrAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Input), args.Error(1)
}

func (m *MockMachineRepository) GetApplication(
	ctx context.Context,
	nameOrAddress string,
) (*model.Application, error) {
	args := m.Called(ctx, nameOrAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Application), args.Error(1)
}

func (m *MockMachineRepository) UpdateApplicationStatus(
	ctx context.Context,
	appID int64,
	status model.ApplicationStatus,
	reason *string,
) error {
	return m.Called(ctx, appID, status, reason).Error(0)
}

// ------------------------------------------------------------------------------------------------

// MockMachineInstanceFactory implements MachineInstanceFactory for testing.
// It returns the same instance for every call, ignoring the app/path arguments.
type MockMachineInstanceFactory struct {
	Instance          MachineInstance
	SnapshotInstance  MachineInstance
	Err               error
	SnapshotErr       error
	TemplateFn        func(*model.Application) (MachineInstance, error)
	ReturnNilSnapshot bool
	TemplateCalls     int
	SnapshotCalls     int
	TemplateChecks    []bool
}

func (f *MockMachineInstanceFactory) NewFromTemplate(
	_ context.Context, app *model.Application, _ *slog.Logger, checkTemplateHash bool,
) (MachineInstance, error) {
	f.TemplateCalls++
	f.TemplateChecks = append(f.TemplateChecks, checkTemplateHash)
	if f.TemplateFn != nil {
		return f.TemplateFn(app)
	}
	return f.Instance, f.Err
}

func (f *MockMachineInstanceFactory) NewFromSnapshot(
	_ context.Context, _ *model.Application, _ *slog.Logger,
	_ string, _ common.Hash, _ uint64,
) (MachineInstance, error) {
	f.SnapshotCalls++
	if f.ReturnNilSnapshot {
		return nil, nil
	}
	if f.SnapshotErr != nil {
		return f.SnapshotInstance, f.SnapshotErr
	}
	if f.SnapshotInstance != nil {
		return f.SnapshotInstance, nil
	}
	return f.Instance, f.Err
}

// realMachineInstanceFactory builds real MachineInstanceImpl values using the
// provided MachineRuntimeFactory. This lets tests exercise the real Synchronize
// path while still mocking the machine runtime. Unlike MockMachineInstanceFactory,
// snapshot path and hash are ignored — it always creates from the runtime factory.
type realMachineInstanceFactory struct {
	runtimeFactory MachineRuntimeFactory
}

func (f *realMachineInstanceFactory) NewFromTemplate(
	ctx context.Context, app *model.Application, logger *slog.Logger, _ bool,
) (MachineInstance, error) {
	return NewMachineInstanceWithFactory(ctx, app, 0, logger, f.runtimeFactory)
}

func (f *realMachineInstanceFactory) NewFromSnapshot(
	ctx context.Context, app *model.Application, logger *slog.Logger,
	_ string, _ common.Hash, inputIndex uint64,
) (MachineInstance, error) {
	if inputIndex == math.MaxUint64 {
		return nil, ErrInvalidSnapshotPoint
	}
	return NewMachineInstanceWithFactory(ctx, app, inputIndex+1, logger, f.runtimeFactory)
}

// ------------------------------------------------------------------------------------------------

// DummyMachineInstanceMock implements the MachineInstance interface for testing
type DummyMachineInstanceMock struct {
	application     *model.Application
	closeError      error
	hashError       error
	hashReturn      [32]byte
	replayErr       error
	closeCalls      int
	hashCalls       int
	processedInputs uint64
	replayCalls     int
}

func (m *DummyMachineInstanceMock) Application() *model.Application {
	return m.application
}

func (m *DummyMachineInstanceMock) ProcessedInputs() uint64 {
	return m.processedInputs
}

func (m *DummyMachineInstanceMock) OutputsProof(ctx context.Context) (*model.OutputsProof, error) {
	return nil, nil
}

func (m *DummyMachineInstanceMock) Advance(_ context.Context, _ []byte, _ uint64, _ uint64, _ bool) (*model.AdvanceResult, error) {
	return nil, nil
}

func (m *DummyMachineInstanceMock) Inspect(_ context.Context, _ []byte) (*InspectResult, error) {
	return nil, nil
}

func (m *DummyMachineInstanceMock) CreateSnapshot(_ context.Context, _ uint64, _ string) error {
	return nil
}

func (m *DummyMachineInstanceMock) Hash(_ context.Context) ([32]byte, error) {
	m.hashCalls++
	return m.hashReturn, m.hashError
}

func (m *DummyMachineInstanceMock) Close() error {
	m.closeCalls++
	return m.closeError
}
