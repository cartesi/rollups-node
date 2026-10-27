// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// The fake keeps durable epochs separate from mutable completion snapshots.
// A failed publication cannot silently advance the next discovery result.
const (
	claimForeclosedExemption = "foreclosed"
	claimRootExemption       = "root boundary"
	claimStatusExemption     = "status"
)

type claimBackoffRepository struct {
	repository.Repository
	apps        []*model.Application
	epochs      map[int64][]*model.Epoch
	problems    map[int64]*model.ApplicationProblem
	stores      []claimRetryKey
	reads       []claimRetryKey
	statusCalls int
	rootCalls   int
	statusError error
	rootError   error
	read        func(claimRetryKey) error
	rangeRead   func(claimRetryKey) error
	store       func(context.Context, claimRetryKey) error
	count       func() (uint64, error)
}

func (r *claimBackoffRepository) ListApplications(
	context.Context, repository.ApplicationFilter, repository.Pagination, bool,
) ([]*model.Application, uint64, error) {
	return r.apps, uint64(len(r.apps)), nil
}

func (r *claimBackoffRepository) app(address string) *model.Application {
	for _, app := range r.apps {
		if app.IApplicationAddress.Hex() == address {
			return app
		}
	}
	panic("unexpected application address")
}

func (r *claimBackoffRepository) epoch(key claimRetryKey) *model.Epoch {
	for _, epoch := range r.epochs[key.applicationID] {
		if epoch.Index == key.epochIndex {
			return epoch
		}
	}
	panic("unexpected epoch key")
}

func (r *claimBackoffRepository) ListEpochs(
	_ context.Context, address string, filter repository.EpochFilter, _ repository.Pagination, _ bool,
) ([]*model.Epoch, uint64, error) {
	if filter.HasTournament != nil {
		return nil, 0, nil
	}
	var epochs []*model.Epoch
	for _, epoch := range r.epochs[r.app(address).ID] {
		if epoch.Status == model.EpochStatus_InputsProcessed {
			snapshot := *epoch
			epochs = append(epochs, &snapshot)
		}
	}
	return epochs, uint64(len(epochs)), nil
}

func (r *claimBackoffRepository) GetEpochCompletionState(
	_ context.Context, address string, index uint64,
) (*repository.EpochCompletionState, error) {
	app := r.app(address)
	key := claimRetryKey{app.ID, index}
	r.reads = append(r.reads, key)
	if r.read != nil {
		if err := r.read(key); err != nil {
			return nil, err
		}
	}
	snapshot := *r.epoch(key)
	appCopy := *app
	return &repository.EpochCompletionState{Application: &appCopy, Epoch: &snapshot, Problem: r.problems[app.ID]}, nil
}

func (r *claimBackoffRepository) GetEpochOutputRange(
	_ context.Context, appID int64, index uint64,
) (repository.EpochOutputRange, error) {
	if r.rangeRead != nil {
		return repository.EpochOutputRange{}, r.rangeRead(claimRetryKey{appID, index})
	}
	return repository.EpochOutputRange{}, nil
}

func (r *claimBackoffRepository) GetLastInput(_ context.Context, address string, index uint64) (*model.Input, error) {
	epoch := r.epoch(claimRetryKey{r.app(address).ID, index})
	return &model.Input{MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock}, nil
}

func (r *claimBackoffRepository) GetEpochByVirtualIndex(
	_ context.Context, address string, index uint64,
) (*model.Epoch, error) {
	for _, epoch := range r.epochs[r.app(address).ID] {
		if epoch.VirtualIndex == index {
			snapshot := *epoch
			return &snapshot, nil
		}
	}
	return nil, nil
}

func (r *claimBackoffRepository) StoreClaimAndProofs(
	ctx context.Context, epoch *model.Epoch, _ repository.ClaimProofWriteFunc,
) error {
	key := claimRetryKey{epoch.ApplicationID, epoch.Index}
	r.stores = append(r.stores, key)
	if r.store != nil {
		if err := r.store(ctx, key); err != nil {
			return err
		}
	}
	r.epoch(key).Status = model.EpochStatus_ClaimComputed
	return nil
}

func (r *claimBackoffRepository) UpdateApplicationStatus(
	_ context.Context, appID int64, status model.ApplicationStatus, reason *string,
) error {
	r.statusCalls++
	if r.statusError != nil {
		return r.statusError
	}
	for _, app := range r.apps {
		if app.ID == appID {
			app.Status, app.Reason = status, reason
		}
	}
	return nil
}

func (r *claimBackoffRepository) StoreInvalidOutputsRoot(context.Context, int64, uint64, string) error {
	r.rootCalls++
	return r.rootError
}

func (r *claimBackoffRepository) CountEpochStateHashes(context.Context, int64, uint64) (uint64, error) {
	return r.count()
}

func (r *claimBackoffRepository) ListEpochStateHashes(
	context.Context, int64, uint64, *uint64, uint64,
) ([]model.EpochStateHash, error) {
	return nil, nil
}

func newClaimBackoffFixture(t *testing.T, poll time.Duration) (*Service, *claimBackoffRepository, *time.Time, *bytes.Buffer) {
	t.Helper()
	post := merkle.CreatePostContext()
	repo := &claimBackoffRepository{epochs: make(map[int64][]*model.Epoch), problems: make(map[int64]*model.ApplicationProblem)}
	for _, id := range []int64{73, 91} {
		app := &model.Application{ID: id, Name: fmt.Sprintf("app-%d", id), Enabled: true,
			IApplicationAddress: common.BigToAddress(big.NewInt(id)), Status: model.ApplicationStatus_OK,
			ConsensusType: model.Consensus_Authority, TemplateHash: common.HexToHash("0xcafe")}
		repo.apps = append(repo.apps, app)
		indexes := []uint64{2, 7}
		if id == 91 {
			indexes = indexes[:1]
		}
		for virtual, index := range indexes {
			repo.epochs[id] = append(repo.epochs[id], &model.Epoch{ApplicationID: id, Index: index,
				VirtualIndex: uint64(virtual), FirstBlock: index * 10, LastBlock: index*10 + 9,
				Status: model.EpochStatus_InputsProcessed, MachineHash: &app.TemplateHash, TxBufferDataBlock: &post[merkle.TREE_DEPTH]})
		}
	}
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	logs := new(bytes.Buffer)
	created, err := Create(t.Context(), &CreateInfo{Config: config.ValidatorConfig{ValidatorPollingInterval: poll},
		Repository: repo, Logger: slog.New(slog.NewJSONHandler(logs, nil))})
	require.NoError(t, err)
	s := created.(*Service)
	s.now = func() time.Time { return now }
	return s, repo, &now, logs
}

func backoffTick(t *testing.T, s *Service, cause error) error {
	t.Helper()
	reschedule, err := s.Tick(t.Context())
	require.False(t, reschedule, "backoff must preserve the polling schedule")
	if cause == nil {
		require.NoError(t, err)
	} else {
		require.ErrorIs(t, err, cause)
	}
	return err
}

func TestClaimBackoffWaitsForThirdAttemptAndKeepsSiblingProgress(t *testing.T) {
	s, repo, now, _ := newClaimBackoffFixture(t, 100*time.Second)
	key := claimRetryKey{73, 2}
	failure := errors.New("disk full")
	repo.store = func(_ context.Context, current claimRetryKey) error {
		if current == key {
			return failure
		}
		return nil
	}
	_ = backoffTick(t, s, failure)
	require.Equal(t, []claimRetryKey{{73, 2}, {91, 2}}, repo.stores)
	require.Equal(t, model.ApplicationStatus_OK, repo.apps[0].Status)
	require.Zero(t, repo.statusCalls)
	*now = now.Add(100 * time.Second)
	_ = backoffTick(t, s, failure)
	require.Equal(t, uint64(2), s.claimRetries[key].attempts)
	require.Equal(t, 200*time.Second, s.claimRetries[key].delay)
	*now = now.Add(100 * time.Second)
	_ = backoffTick(t, s, nil)
	require.Len(t, repo.stores, 3, "third poll is still cooling and cannot enter its successor")
	*now = now.Add(100 * time.Second) // Exact equality is eligible.
	repo.store = nil
	_ = backoffTick(t, s, nil)
	require.Equal(t, []claimRetryKey{{73, 2}, {91, 2}, {73, 2}, {73, 2}, {73, 7}}, repo.stores)
	require.Empty(t, s.claimRetries)
}

func TestClaimBackoffStartsAtFailureCompletionAndPreservesCoolingReadFailure(t *testing.T) {
	s, repo, now, _ := newClaimBackoffFixture(t, 100*time.Second)
	key := claimRetryKey{73, 2}
	failure := errors.New("slow failed claim")
	repo.store = func(_ context.Context, current claimRetryKey) error {
		if current == key {
			*now = now.Add(201 * time.Second)
			return failure
		}
		return nil
	}
	start := *now
	_ = backoffTick(t, s, failure)
	prior := s.claimRetries[key]
	require.Equal(t, start.Add(301*time.Second), prior.nextAttempt)
	_ = backoffTick(t, s, nil) // The buffered tick at the same completion time.
	require.Len(t, repo.stores, 2)
	readError := errors.New("cheap coherent read unavailable")
	repo.read = func(current claimRetryKey) error {
		if current == key {
			return readError
		}
		return nil
	}
	_ = backoffTick(t, s, readError)
	require.Equal(t, prior, s.claimRetries[key], "a cooling read failure is not another claim attempt")
	*now = prior.nextAttempt
	_ = backoffTick(t, s, readError)
	retry := s.claimRetries[key]
	require.Equal(t, uint64(2), retry.attempts)
	require.Equal(t, 200*time.Second, retry.delay)
	require.Equal(t, now.Add(200*time.Second), retry.nextAttempt)
}

func TestClaimBackoffOperationalClasses(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
		read  bool
	}{
		{"read", errors.New("read failed"), true},
		{"store", errors.New("store failed"), false},
		{"changed", merkle.ErrOutputsChanged, false},
		{"invariant", merkle.ErrInvariant, false},
		{"operation deadline", context.DeadlineExceeded, false},
		{"joined cancellation", errors.Join(context.Canceled, errors.New("independent write error")), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, repo, now, _ := newClaimBackoffFixture(t, time.Minute)
			key := claimRetryKey{73, 2}
			fail := func(current claimRetryKey) error {
				if current == key {
					return test.cause
				}
				return nil
			}
			if test.read {
				repo.rangeRead = fail
			} else {
				repo.store = func(_ context.Context, k claimRetryKey) error { return fail(k) }
			}
			_ = backoffTick(t, s, test.cause)
			retry := s.claimRetries[key]
			require.Equal(t, uint64(1), retry.attempts)
			require.Equal(t, time.Minute, retry.delay)
			require.Equal(t, now.Add(time.Minute), retry.nextAttempt)
			require.Equal(t, model.ApplicationStatus_OK, repo.apps[0].Status)
			require.Zero(t, repo.statusCalls)
			require.Equal(t, model.EpochStatus_ClaimComputed, repo.epochs[91][0].Status)
		})
	}
}

func TestClaimBackoffStateHashRecheck(t *testing.T) {
	s, repo, _, _ := newClaimBackoffFixture(t, time.Minute)
	repo.apps[0].ConsensusType = model.Consensus_PRT
	repo.epochs[73][0].InputIndexUpperBound = 1
	failure := errors.New("count recheck failed")
	var counts int
	repo.count = func() (uint64, error) {
		counts++
		if counts == 1 {
			return 1, nil
		}
		return 0, failure
	}
	_ = backoffTick(t, s, failure)
	require.Equal(t, uint64(1), s.claimRetries[claimRetryKey{73, 2}].attempts)
	require.Zero(t, repo.statusCalls)
}

func TestClaimBackoffExemptionsPreserveExistingRetries(t *testing.T) {
	for _, exemption := range []string{claimForeclosedExemption, "canceled", claimStatusExemption, claimRootExemption} {
		for _, priorFailure := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/prior=%t", exemption, priorFailure), func(t *testing.T) {
				s, repo, now, _ := newClaimBackoffFixture(t, time.Minute)
				key := claimRetryKey{73, 2}
				if priorFailure {
					failure := errors.New("first write failed")
					repo.store = func(_ context.Context, k claimRetryKey) error {
						if k == key {
							return failure
						}
						return nil
					}
					_ = backoffTick(t, s, failure)
					*now = s.claimRetries[key].nextAttempt
				}
				prior := s.claimRetries[key]
				repo.store = nil
				var cause error
				switch exemption {
				case claimForeclosedExemption:
					repo.store = func(_ context.Context, k claimRetryKey) error {
						if k == key {
							repo.epoch(k).Status = model.EpochStatus_ClaimForeclosed
							return fmt.Errorf("late: %w", repository.ErrEpochForeclosed)
						}
						return nil
					}
				case "canceled":
					cause = context.Canceled
					repo.store = func(_ context.Context, k claimRetryKey) error {
						if k == key {
							return fmt.Errorf("write: %w", cause)
						}
						return nil
					}
				case claimStatusExemption:
					repo.epochs[73][0].MachineHash = nil
				case claimRootExemption:
					// The successful transaction can keep execution status and only
					// record an earlier boundary. app.Status cannot classify it.
					repo.apps[0].Status = model.ApplicationStatus_MachineHalted
					repo.problems[73] = &model.ApplicationProblem{EpochIndex: 9, Kind: model.ApplicationStatus_MachineHalted,
						InputIndex: new(uint64(4))}
					repo.epochs[73][0].TxBufferDataBlock = new(common.Hash)
				}
				if exemption == claimStatusExemption || exemption == claimRootExemption {
					reschedule, err := s.Tick(t.Context())
					require.False(t, reschedule)
					require.Error(t, err)
				} else {
					_ = backoffTick(t, s, cause)
				}
				require.Equal(t, prior, s.claimRetries[key])
				if exemption == claimForeclosedExemption {
					require.Equal(t, model.EpochStatus_ClaimComputed, repo.epochs[73][1].Status)
				}
				if exemption == claimRootExemption {
					require.Equal(t, 1, repo.rootCalls)
					require.Equal(t, model.ApplicationStatus_MachineHalted, repo.apps[0].Status)
				}
			})
		}
	}
}

func TestClaimBackoffCapKeysRestartAndExternalCommit(t *testing.T) {
	s, repo, now, _ := newClaimBackoffFixture(t, 100*time.Second)
	failure := errors.New("permanent disk failure")
	key := claimRetryKey{73, 2}
	repo.store = func(_ context.Context, k claimRetryKey) error {
		if k == key {
			return failure
		}
		return nil
	}
	for _, expected := range []time.Duration{100, 200, 300, 300, 300} {
		_ = backoffTick(t, s, failure)
		require.Equal(t, expected*time.Second, s.claimRetries[key].delay)
		*now = s.claimRetries[key].nextAttempt
	}
	other := claimRetryKey{91, 999}
	s.claimRetries[other] = claimRetry{attempts: 7, delay: time.Minute, nextAttempt: now.Add(time.Hour)}
	repo.store = nil
	_ = backoffTick(t, s, nil)
	require.Equal(t, uint64(7), s.claimRetries[other].attempts, "success clears only its own key")
	// A new service has no volatile retries, even with an old future deadline.
	fresh, _, _, _ := newClaimBackoffFixture(t, time.Minute)
	fresh.repository = repo
	repo.epochs[73][0].Status = model.EpochStatus_InputsProcessed
	_ = backoffTick(t, fresh, nil)
	require.Empty(t, fresh.claimRetries)
	// A late valid commit can remove a cooling epoch from the work set.
	s2, repo2, now2, _ := newClaimBackoffFixture(t, time.Minute)
	s2.claimRetries = map[claimRetryKey]claimRetry{key: {attempts: 2, delay: time.Minute, nextAttempt: now2.Add(time.Hour)}}
	repo2.read = func(k claimRetryKey) error {
		if k == key {
			repo2.epoch(k).Status = model.EpochStatus_ClaimComputed
		}
		return nil
	}
	_ = backoffTick(t, s2, nil)
	require.Equal(t, model.EpochStatus_ClaimComputed, repo2.epochs[73][1].Status)
	require.Equal(t, uint64(2), s2.claimRetries[key].attempts)
}

// Create only stores this interface. No method, database, or SQL is called.
type claimCreateRepository struct{ repository.Repository }

func TestClaimBackoffResolvedPoll(t *testing.T) {
	for _, poll := range []time.Duration{0, 100 * time.Second, 5 * time.Minute, 10 * time.Minute} {
		t.Run(poll.String(), func(t *testing.T) {
			created, err := Create(t.Context(), &CreateInfo{Config: config.ValidatorConfig{ValidatorPollingInterval: poll},
				Logger: slog.New(slog.NewJSONHandler(new(bytes.Buffer), nil)), Repository: &claimCreateRepository{}})
			require.NoError(t, err)
			s := created.(*Service)
			want := poll
			if want == 0 {
				want = time.Minute
			}
			require.Equal(t, min(want, 5*time.Minute), s.claimRetryBase)
			_, repo, now, _ := newClaimBackoffFixture(t, poll)
			s.repository = repo
			s.now = func() time.Time { return *now }
			failure := errors.New("first attempt failed")
			repo.store = func(_ context.Context, _ claimRetryKey) error { return failure }
			_ = backoffTick(t, s, failure)
			require.Equal(t, min(want, 5*time.Minute), s.claimRetries[claimRetryKey{73, 2}].delay)
		})
	}
}

func TestClaimBackoffErrorNamesTheRetry(t *testing.T) {
	s, repo, now, logs := newClaimBackoffFixture(t, time.Minute)
	failures := map[int64]error{73: errors.New("app-A failure"), 91: errors.New("app-B failure")}
	repo.store = func(_ context.Context, k claimRetryKey) error { return failures[k.applicationID] }
	err := backoffTick(t, s, failures[73])
	require.ErrorIs(t, err, failures[91])
	require.Contains(t, err.Error(), failures[73].Error())
	require.Contains(t, err.Error(), failures[91].Error())
	s.Logger.Error("Tick", "error", err)
	require.Equal(t, 1, strings.Count(logs.String(), `"level":"ERROR"`))
	next := now.Add(time.Minute).Format(time.RFC3339)
	for i := range 2 {
		require.Contains(t, err.Error(), fmt.Sprintf("application %s epoch 2: claim attempt 1 failed, next attempt at %s",
			repo.apps[i].IApplicationAddress.String(), next))
	}
}

func TestClaimBackoffPreservedHealthIsNotRecordedFailure(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
		t.Run(status.String(), func(t *testing.T) {
			s, repo, _, _ := newClaimBackoffFixture(t, time.Minute)
			repo.apps[0].Status = status
			repo.problems[73] = &model.ApplicationProblem{EpochIndex: 9, Kind: status}
			if status.IsExecutionTerminal() {
				repo.problems[73].InputIndex = new(uint64(4))
			}
			failure := errors.New("operational error before the stop boundary")
			repo.rangeRead = func(k claimRetryKey) error {
				if k.applicationID == 73 {
					return failure
				}
				return nil
			}
			_ = backoffTick(t, s, failure)
			require.Equal(t, uint64(1), s.claimRetries[claimRetryKey{73, 2}].attempts)
			require.Zero(t, repo.statusCalls)
			require.Equal(t, status, repo.apps[0].Status)
		})
	}
}

func TestClaimBackoffClassifiesActualStatusWrite(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     model.ApplicationStatus
		writeError error
		wantRetry  bool
	}{
		{"FAILED recorded", model.ApplicationStatus_OK, nil, false},
		{"FAILED write failed", model.ApplicationStatus_OK, errors.New("status failed"), true},
		{"FAILED write canceled while live", model.ApplicationStatus_OK, context.Canceled, true},
		{"execution terminal preserved", model.ApplicationStatus_MachineHalted, nil, true},
		{"root stop preserved", model.ApplicationStatus_InvalidOutputsRoot, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, repo, _, _ := newClaimBackoffFixture(t, time.Minute)
			app := repo.apps[0]
			app.Status = test.status
			repo.statusError = test.writeError
			epoch := repo.epochs[73][0]
			_, _, err := s.proveCommitment(t.Context(), app, epoch, &merkle.Tree{Height: uint32(model.Log2EpochComputationHashLeafCount)})
			require.ErrorIs(t, err, merkle.ErrInvariant)
			if test.writeError != nil {
				require.ErrorIs(t, err, test.writeError)
			}
			_ = s.failedClaim(claimRetryKey{73, 2}, "app", err)
			_, retried := s.claimRetries[claimRetryKey{73, 2}]
			require.Equal(t, test.wantRetry, retried)
			if test.status == model.ApplicationStatus_OK {
				require.Equal(t, 1, repo.statusCalls)
			} else {
				require.Zero(t, repo.statusCalls)
			}
		})
	}
}

func TestClaimBackoffFailedStatusPersistenceStillRetries(t *testing.T) {
	for _, rootBoundary := range []bool{false, true} {
		t.Run(fmt.Sprintf("rootBoundary=%t", rootBoundary), func(t *testing.T) {
			s, repo, _, _ := newClaimBackoffFixture(t, time.Minute)
			failure := errors.New("status transaction failed")
			if rootBoundary {
				repo.epochs[73][0].TxBufferDataBlock = new(common.Hash)
				repo.rootError = failure
			} else {
				repo.epochs[73][0].MachineHash = nil
				repo.statusError = failure
			}
			_ = backoffTick(t, s, failure)
			require.Equal(t, uint64(1), s.claimRetries[claimRetryKey{73, 2}].attempts)
			require.Equal(t, model.ApplicationStatus_OK, repo.apps[0].Status)
		})
	}
}

func TestClaimBackoffServeEmitsSingleError(t *testing.T) {
	s, repo, _, logs := newClaimBackoffFixture(t, time.Minute)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, service.InitTickServiceTemplate(&s.TickServiceTemplate, &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{Name: config.ServiceValidator, Logger: s.Logger}, PollInterval: time.Hour,
	}, s))
	cause := errors.New("failed transaction")
	repo.store = func(_ context.Context, key claimRetryKey) error {
		if key.applicationID == 91 {
			cancel()
		}
		return cause
	}
	require.ErrorIs(t, s.Serve(ctx), context.Canceled)
	require.Equal(t, 1, strings.Count(logs.String(), `"level":"ERROR"`))
	require.Equal(t, 2, strings.Count(logs.String(), "claim attempt 1 failed"))
}
