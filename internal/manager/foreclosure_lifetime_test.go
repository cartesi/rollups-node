// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/replay"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// The repository supplies the durable duties. SQL tests establish which actual
// epoch rows satisfy those duties; these tests control the manager's schedule.
type foreclosureLifetimeRepository struct {
	MachineRepository
	application         *model.Application
	pendingInputs       bool
	pendingPublications int
}

func (r *foreclosureLifetimeRepository) ListApplications(
	_ context.Context, filter repository.ApplicationFilter, _ repository.Pagination, _ bool,
) ([]*model.Application, uint64, error) {
	if filter.ForeclosureRecorded == nil || !*filter.ForeclosureRecorded ||
		!r.application.Enabled || r.application.Status != model.ApplicationStatus_OK {
		return nil, 0, nil
	}
	return []*model.Application{r.application}, 1, nil
}

func (r *foreclosureLifetimeRepository) HasUndrainedEpochsBeforeBlock(context.Context, int64, uint64) (bool, error) {
	return r.pendingInputs, nil
}

func (r *foreclosureLifetimeRepository) HasUnpublishedEpochBeforeBlock(context.Context, int64, uint64) (bool, error) {
	return r.pendingPublications > 0, nil
}

func (*foreclosureLifetimeRepository) GetLastSnapshot(context.Context, string) (*model.Input, error) {
	return nil, nil
}

type foreclosureLifetimeInstances struct {
	created []*DummyMachineInstanceMock
	replays []replay.Options
}

func newForeclosureLifetimeManager(
	t *testing.T, repo *foreclosureLifetimeRepository,
) (*MachineManager, *foreclosureLifetimeInstances) {
	t.Helper()
	observed := &foreclosureLifetimeInstances{}
	factory := &MockMachineInstanceFactory{TemplateFn: func(app *model.Application) (MachineInstance, error) {
		instance := &DummyMachineInstanceMock{application: app}
		observed.created = append(observed.created, instance)
		return instance, nil
	}}
	m := NewMachineManager(repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 1,
		WithInstanceFactory(factory), withReplayRun(func(
			_ context.Context, _ repository.ReplayRepository, executor replay.Executor, options replay.Options,
		) (replay.Result, error) {
			observed.replays = append(observed.replays, options)
			instance := executor.(*DummyMachineInstanceMock)
			instance.processedInputs = options.ToInputExclusive
			return replay.Result{ReplayedInputs: options.ToInputExclusive - options.FromInput}, nil
		}))
	t.Cleanup(m.Close)
	return m, observed
}

func newForeclosureLifetimeRepository() *foreclosureLifetimeRepository {
	return &foreclosureLifetimeRepository{application: &model.Application{
		ID: 27, Name: "retained-prt-finalization", IApplicationAddress: common.HexToAddress("0x27"),
		ConsensusType: model.Consensus_PRT, Enabled: true, Status: model.ApplicationStatus_OK,
		ForecloseBlock: 80, LastInputCheckBlock: 80, LastEpochCheckBlock: 80, ProcessedInputs: 2,
	}}
}

func TestForeclosedPRTRetainsAllMachineDuties(t *testing.T) {
	repo := newForeclosureLifetimeRepository()
	repo.pendingPublications = 2
	m, observed := newForeclosureLifetimeManager(t, repo)
	app := repo.application

	require.NoError(t, m.UpdateMachines(t.Context()))
	require.True(t, m.HasMachine(app.ID))
	require.Len(t, observed.created, 1)
	instance := observed.created[0]

	// Completing one CLOSED target cannot release a second CLOSED target.
	repo.pendingPublications--
	require.NoError(t, m.UpdateMachines(t.Context()))
	require.True(t, m.HasMachine(app.ID))
	require.Zero(t, instance.closeCalls)

	// IP itself needs no machine, but bounded OPEN inputs still do.
	repo.pendingPublications = 0
	repo.pendingInputs = true
	require.NoError(t, m.UpdateMachines(t.Context()))
	require.True(t, m.HasMachine(app.ID))
	require.Zero(t, instance.closeCalls)

	repo.pendingInputs = false
	require.NoError(t, m.UpdateMachines(t.Context()))
	require.False(t, m.HasMachine(app.ID))
	require.Equal(t, 1, instance.closeCalls)
	require.Len(t, observed.created, 1, "keep one runtime through all eligible duties")
}

func TestForeclosedPRTReaderSealAfterReleaseRecreatesAndReplays(t *testing.T) {
	for _, restart := range []bool{false, true} {
		name := "same process"
		if restart {
			name = "after restart"
		}
		t.Run(name, func(t *testing.T) {
			repo := newForeclosureLifetimeRepository()
			before, observed := newForeclosureLifetimeManager(t, repo)
			old := &DummyMachineInstanceMock{application: repo.application, processedInputs: 2}
			before.addMachine(repo.application.ID, old)
			// Manager sees only a drained OPEN epoch. The reader has not sealed it yet.
			require.NoError(t, before.UpdateMachines(t.Context()))
			require.False(t, before.HasMachine(repo.application.ID))
			require.Equal(t, 1, old.closeCalls)

			// Reader commits its real seal before the next manager turn.
			repo.pendingPublications = 1
			after := before
			if restart {
				after, observed = newForeclosureLifetimeManager(t, repo)
			}
			require.NoError(t, after.UpdateMachines(t.Context()))
			require.True(t, after.HasMachine(repo.application.ID))
			require.Len(t, observed.created, 1)
			require.NotSame(t, old, observed.created[0])
			require.Len(t, observed.replays, 1)
			require.Equal(t, uint64(0), observed.replays[0].FromInput)
			require.Equal(t, uint64(2), observed.replays[0].ToInputExclusive)
			require.Equal(t, repository.ReplayVerificationCanonical, observed.replays[0].Verification)
			require.Equal(t, uint64(2), observed.created[0].ProcessedInputs())

			repo.pendingPublications = 0
			require.NoError(t, after.UpdateMachines(t.Context()))
			require.False(t, after.HasMachine(repo.application.ID))
			require.Equal(t, 1, observed.created[0].closeCalls)
		})
	}
}

func TestForeclosedPRTFailureRequiresOperatorRepairBeforeReplay(t *testing.T) {
	repo := newForeclosureLifetimeRepository()
	repo.pendingPublications = 1
	m, observed := newForeclosureLifetimeManager(t, repo)
	old := &DummyMachineInstanceMock{application: repo.application, processedInputs: 2}
	m.addMachine(repo.application.ID, old)
	require.NoError(t, m.UpdateMachines(t.Context()))
	require.True(t, m.HasMachine(repo.application.ID))

	// A durable failure suspends the duty; it does not discharge it.
	repo.application.Status = model.ApplicationStatus_Failed
	require.NoError(t, m.UpdateMachines(t.Context()))
	require.False(t, m.HasMachine(repo.application.ID))
	require.Equal(t, 1, old.closeCalls)
	require.Empty(t, observed.created)
	require.Equal(t, 1, repo.pendingPublications)
	require.NoError(t, m.UpdateMachines(t.Context()))
	require.Empty(t, observed.created, "ordinary turns cannot repair FAILED")

	// The operator has repaired the cause and restored OK. Create, do not revive.
	repo.application.Status = model.ApplicationStatus_OK
	require.NoError(t, m.UpdateMachines(t.Context()))
	require.True(t, m.HasMachine(repo.application.ID))
	require.Len(t, observed.created, 1)
	require.NotSame(t, old, observed.created[0])
	require.Len(t, observed.replays, 1)
	require.Equal(t, uint64(0), observed.replays[0].FromInput)
	require.Equal(t, repo.application.ProcessedInputs, observed.replays[0].ToInputExclusive)
	require.Equal(t, repository.ReplayVerificationCanonical, observed.replays[0].Verification)

	repo.pendingPublications = 0
	require.NoError(t, m.UpdateMachines(t.Context()))
	require.False(t, m.HasMachine(repo.application.ID))
	require.Equal(t, 1, old.closeCalls)
	require.Equal(t, 1, observed.created[0].closeCalls)
}
