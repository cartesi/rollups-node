// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const countReadError = "count error"

func expectStateHashRows(repo *Mockrepo, app *model.Application, epoch *model.Epoch, rows []*model.StateHash, total uint64) {
	calls := 1
	if uint64(len(rows)) != total {
		calls++
		repo.application = app
		epoch.Status = model.EpochStatus_InputsProcessed
		repo.epochs = []*model.Epoch{epoch}
	}
	repo.On("CountEpochStateHashes", mock.Anything, app.ID, epoch.Index).Return(total, nil).Times(calls)
	page := make([]model.EpochStateHash, len(rows))
	for i, row := range rows {
		page[i] = model.EpochStateHash{Index: uint64(i), InputIndex: row.InputIndex,
			MachineHash: row.MachineHash, Repetitions: row.Repetitions}
	}
	repo.On("ListEpochStateHashes", mock.Anything, app.ID, epoch.Index, (*uint64)(nil), stateHashPageSize).
		Return(page, nil).Once()
}

type stateHashPageRepository struct {
	*Mockrepo
	t                     *testing.T
	appID                 int64
	epochIndex            uint64
	total, inputs         uint64
	final                 common.Hash
	countCalls, pageCalls int
	cursors               []*uint64
	countFn               func(context.Context, int) (uint64, error)
	pageFn                func(context.Context, *uint64, uint64) ([]model.EpochStateHash, error)
	observePage           func()
}

func (r *stateHashPageRepository) CountEpochStateHashes(ctx context.Context, appID int64, epochIndex uint64) (uint64, error) {
	require.Equal(r.t, r.appID, appID)
	require.Equal(r.t, r.epochIndex, epochIndex)
	r.countCalls++
	if r.countFn != nil {
		return r.countFn(ctx, r.countCalls)
	}
	return r.total, ctx.Err()
}

func (r *stateHashPageRepository) ListEpochStateHashes(
	ctx context.Context, appID int64, epochIndex uint64, after *uint64, limit uint64,
) ([]model.EpochStateHash, error) {
	require.Equal(r.t, r.appID, appID)
	require.Equal(r.t, r.epochIndex, epochIndex)
	require.Equal(r.t, stateHashPageSize, limit)
	r.pageCalls++
	if after == nil {
		r.cursors = append(r.cursors, nil)
	} else {
		value := *after
		r.cursors = append(r.cursors, &value)
	}
	if r.pageFn != nil {
		return r.pageFn(ctx, after, limit)
	}
	var start uint64
	if after != nil {
		start = (*after-17)/2 + 1
	}
	count := min(limit, r.total-start)
	page := make([]model.EpochStateHash, count)
	for i := range page {
		page[i] = generatedStateHash(start+uint64(i), r.total, r.inputs, r.final)
	}
	if r.observePage != nil {
		r.observePage()
	}
	return page, nil
}

func generatedStateHash(index, rows, inputs uint64, final common.Hash) model.EpochStateHash {
	base, extra := rows/inputs, rows%inputs
	wideEnd := (base + 1) * extra
	var input, offset, size uint64
	if index < wideEnd {
		input, offset, size = index/(base+1), index%(base+1), base+1
	} else {
		input, offset, size = extra+(index-wideEnd)/base, (index-wideEnd)%base, base
	}
	var hash common.Hash
	binary.BigEndian.PutUint64(hash[common.HashLength-8:], index+1)
	repetitions := uint64(1)
	if offset+1 == size {
		hash = model.FinalStateEntry(final)
		repetitions = model.InputHashCollectionCapacity - (size - 1)
	}
	return model.EpochStateHash{Index: 17 + 2*index, InputIndex: input, MachineHash: hash, Repetitions: repetitions}
}

func newPagedCommitmentFixture(t *testing.T, rows, inputs uint64) (*stateHashPageRepository, *Service, *model.Application, *model.Epoch) {
	t.Helper()
	final := common.HexToHash("0x1234")
	app := &model.Application{
		ID: 47, Name: "paged-prt", IApplicationAddress: common.HexToAddress("0x47"),
		ConsensusType: model.Consensus_PRT, Enabled: true, Status: model.ApplicationStatus_OK,
		TemplateHash: common.HexToHash("0x9876"),
	}
	epoch := &model.Epoch{
		ApplicationID: app.ID, Index: 83, VirtualIndex: 12, Status: model.EpochStatus_InputsProcessed,
		InputIndexLowerBound: 700, InputIndexUpperBound: 700 + inputs, MachineHash: &final,
	}
	r := &stateHashPageRepository{Mockrepo: newMockrepo(), t: t, appID: app.ID, epochIndex: epoch.Index,
		total: rows, inputs: inputs, final: final}
	r.application, r.epochs = app, []*model.Epoch{epoch}
	s := &Service{repository: r}
	s.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	return r, s, app, epoch
}

func expectedPagedCommitment(t *testing.T, r *stateHashPageRepository) (common.Hash, *merkle.Proof) {
	t.Helper()
	runs := make([]merkletest.Run, 0, r.total+1)
	for index := range r.total {
		row := generatedStateHash(index, r.total, r.inputs, r.final)
		runs = append(runs, merkletest.Run{Hash: row.MachineHash, Count: row.Repetitions})
	}
	runs = append(runs, merkletest.Run{Hash: model.FinalStateEntry(r.final),
		Count: (model.MaxAdvanceStatesPerEpoch - r.inputs) << model.Log2InputHashCollectionCapacity})
	const entryHeight = uint32(48)
	root := merkletest.Root(runs, entryHeight)
	_, upper := merkletest.LastProof(runs, entryHeight)
	siblings := make([]common.Hash, 0, 54)
	hash := r.final
	for range 6 {
		siblings = append(siblings, hash)
		hash = crypto.Keccak256Hash(hash[:], hash[:])
	}
	siblings = append(siblings, upper...)
	return root, &merkle.Proof{Pos: new(big.Int).SetUint64((uint64(1) << 54) - 1), Node: r.final, Siblings: siblings}
}

func TestBuildCommitmentPagesMatchFullBuilder(t *testing.T) {
	for _, test := range []struct{ rows, inputs uint64 }{
		{1, 1}, {2, 1}, {stateHashPageSize - 1, 1}, {stateHashPageSize, 1},
		{stateHashPageSize + 1, 1}, {2 * stateHashPageSize, 1}, {2*stateHashPageSize + 7, 1}, {2*stateHashPageSize + 7, 300},
	} {
		t.Run(fmt.Sprintf("%d rows %d inputs", test.rows, test.inputs), func(t *testing.T) {
			r, s, app, epoch := newPagedCommitmentFixture(t, test.rows, test.inputs)
			wantRoot, wantProof := expectedPagedCommitment(t, r)
			commitment, proof, err := s.buildCommitment(t.Context(), app, epoch)
			require.NoError(t, err)
			require.Equal(t, wantRoot, *commitment)
			require.Equal(t, wantProof, proof)
			require.True(t, proof.VerifyRoot(*commitment))
			require.Equal(t, 1, r.countCalls)
			require.Equal(t, int(test.rows/stateHashPageSize)+1, r.pageCalls) //nolint:gosec // Small fixture counts.
			require.Nil(t, r.cursors[0])
			for page := 1; page < len(r.cursors); page++ {
				expected := uint64(page)*stateHashPageSize - 1
				require.Equal(t, 17+2*expected, *r.cursors[page])
			}
			r.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func expectPagedCorruption(r *stateHashPageRepository, app *model.Application, text string) {
	expectNoStoredProtectionRoots(r.Mockrepo, app)
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted,
		mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
		require.Contains(r.t, *args.Get(3).(*string), text)
	})
}

func TestBuildCommitmentLostRowPrecedesAppendError(t *testing.T) {
	for _, zeroRepetitions := range []bool{false, true} {
		t.Run(fmt.Sprint(zeroRepetitions), func(t *testing.T) {
			r, s, app, epoch := newPagedCommitmentFixture(t, 2, 1)
			r.pageFn = func(context.Context, *uint64, uint64) ([]model.EpochStateHash, error) {
				row := generatedStateHash(1, 2, 1, r.final)
				if zeroRepetitions {
					row.Repetitions = 0
				}
				return []model.EpochStateHash{row}, nil
			}
			expectPagedCorruption(r, app, "inconsistent number of state hashes")
			_, _, err := s.buildCommitment(t.Context(), app, epoch)
			require.ErrorContains(t, err, "inconsistent number of state hashes")
			require.NotContains(t, err.Error(), "invalid repetitions")
			require.Equal(t, 2, r.countCalls)
			r.AssertExpectations(t)
		})
	}
}

func TestBuildCommitmentReadErrorsAreOperational(t *testing.T) {
	sentinel := errors.New("read unavailable")
	for _, test := range []struct {
		name      string
		configure func(*stateHashPageRepository, *model.Epoch)
		want      string
	}{
		{countReadError, func(r *stateHashPageRepository, _ *model.Epoch) {
			r.countFn = func(context.Context, int) (uint64, error) { return 0, sentinel }
		}, "failed to count state hashes"},
		{"page error", func(r *stateHashPageRepository, _ *model.Epoch) {
			r.pageFn = func(context.Context, *uint64, uint64) ([]model.EpochStateHash, error) { return nil, sentinel }
		}, "failed to list state hashes"},
		{"duplicate index", func(r *stateHashPageRepository, _ *model.Epoch) {
			r.pageFn = func(context.Context, *uint64, uint64) ([]model.EpochStateHash, error) {
				row := generatedStateHash(0, 2, 1, r.final)
				return []model.EpochStateHash{row, row}, nil
			}
		}, "not strictly increasing"},
		{"oversized page", func(r *stateHashPageRepository, _ *model.Epoch) {
			r.pageFn = func(context.Context, *uint64, uint64) ([]model.EpochStateHash, error) {
				return make([]model.EpochStateHash, stateHashPageSize+1), nil
			}
		}, "above limit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, s, app, epoch := newPagedCommitmentFixture(t, 2*stateHashPageSize, 1)
			test.configure(r, epoch)
			_, _, err := s.buildCommitment(t.Context(), app, epoch)
			require.ErrorContains(t, err, test.want)
			if test.name == countReadError || test.name == "page error" {
				require.ErrorIs(t, err, sentinel)
			}
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			r.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestBuildCommitmentRejectsPageCursorRegression(t *testing.T) {
	r, s, app, epoch := newPagedCommitmentFixture(t, 2*stateHashPageSize, 1)
	r.pageFn = func(context.Context, *uint64, uint64) ([]model.EpochStateHash, error) {
		page := make([]model.EpochStateHash, stateHashPageSize)
		for i := range page {
			page[i] = generatedStateHash(uint64(i), r.total, r.inputs, r.final)
		}
		return page, nil
	}
	_, _, err := s.buildCommitment(t.Context(), app, epoch)
	require.ErrorContains(t, err, "not strictly increasing")
	require.Equal(t, 2, r.pageCalls)
	require.Equal(t, uint64(17+2*(stateHashPageSize-1)), *r.cursors[1])
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	r.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestBuildCommitmentStopsUnboundedPagesAtCount(t *testing.T) {
	r, s, app, epoch := newPagedCommitmentFixture(t, stateHashPageSize, 1)
	r.pageFn = func(context.Context, *uint64, uint64) ([]model.EpochStateHash, error) {
		// The malformed provider advances its own index but ignores the supplied cursor.
		page := make([]model.EpochStateHash, stateHashPageSize)
		start := uint64(r.pageCalls-1) * stateHashPageSize //nolint:gosec // Positive bounded test call count.
		for i := range page {
			page[i] = model.EpochStateHash{Index: start + uint64(i), Repetitions: 1}
		}
		return page, nil
	}
	expectPagedCorruption(r, app, "inconsistent number of state hashes")
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, _, err := s.buildCommitment(ctx, app, epoch)
	require.ErrorContains(t, err, "inconsistent number of state hashes")
	require.Equal(t, 2, r.pageCalls)
	r.AssertExpectations(t)
}

func TestBuildCommitmentRechecksBeforeCorruption(t *testing.T) {
	sentinel := errors.New("recheck unavailable")
	for _, kind := range []string{
		"count changed", "epoch changed", "application gone", "wrong application", "wrong epoch", countReadError, "epoch error",
	} {
		t.Run(kind, func(t *testing.T) {
			r, s, app, epoch := newPagedCommitmentFixture(t, 2, 1)
			r.On("UpdateApplicationStatus", mock.Anything, app.ID, mock.Anything, mock.Anything).Return(nil).Maybe()
			r.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(), repository.EpochFilter{
				HasTournament: new(true), Status: model.NonTerminalEpochStatuses(),
			}, repository.Pagination{}, false).Return([]*model.Epoch(nil), uint64(0), nil).Maybe()
			r.pageFn = func(context.Context, *uint64, uint64) ([]model.EpochStateHash, error) {
				row := generatedStateHash(1, 2, 1, r.final)
				row.Repetitions = 0
				return []model.EpochStateHash{row}, nil
			}
			r.countFn = func(_ context.Context, call int) (uint64, error) {
				if call > 1 && kind == "count changed" {
					return 1, nil
				}
				if call > 1 && kind == countReadError {
					return 0, sentinel
				}
				return 2, nil
			}
			stateReads := 0
			r.readEpochCompletionState = func(_ context.Context, address string, index uint64) (*repository.EpochCompletionState, error) {
				require.Equal(t, app.IApplicationAddress.Hex(), address)
				require.Equal(t, epoch.Index, index)
				stateReads++
				if kind == "application gone" {
					return nil, nil
				}
				if kind == "epoch error" {
					return nil, sentinel
				}
				currentApp, currentEpoch := *app, *epoch
				if kind == "epoch changed" {
					currentEpoch.Status = model.EpochStatus_ClaimComputed
				}
				if kind == "wrong application" {
					currentApp.ID++
				}
				if kind == "wrong epoch" {
					currentEpoch.Index++
				}
				return &repository.EpochCompletionState{Application: &currentApp, Epoch: &currentEpoch}, nil
			}
			_, _, err := s.buildCommitment(t.Context(), app, epoch)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "inconsistent number of state hashes")
			require.NotContains(t, err.Error(), "invalid repetitions")
			if strings.Contains(kind, "error") {
				require.ErrorIs(t, err, sentinel)
			}
			require.Equal(t, 2, r.countCalls)
			if kind == countReadError {
				require.Zero(t, stateReads)
			} else {
				require.Equal(t, 1, stateReads)
			}
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			r.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestBuildCommitmentEmptyLaterEpoch(t *testing.T) {
	r, s, app, epoch := newPagedCommitmentFixture(t, 0, 0)
	commitment, proof, err := s.buildCommitment(t.Context(), app, epoch)
	require.NoError(t, err)
	want := merkle.TreeLeaf(*epoch.MachineHash).Iterated(model.Log2EpochComputationHashLeafCount)
	require.Equal(t, want.RootHash, *commitment)
	require.Equal(t, *epoch.MachineHash, proof.Node)
	require.NotEqual(t, app.TemplateHash, proof.Node)
	require.Zero(t, r.countCalls)
	require.Zero(t, r.pageCalls)
}

func TestBuildCommitmentCancellationDoesNotWriteStatus(t *testing.T) {
	r, s, app, epoch := newPagedCommitmentFixture(t, 1, 1)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := s.buildCommitment(ctx, app, epoch)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, r.pageCalls)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	r.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestValidatorStreamsNonemptyEpochBeforeTerminalBoundary(t *testing.T) {
	r, s, app, epoch := newPagedCommitmentFixture(t, stateHashPageSize+1, 1)
	s.pristinePostContext = merkle.CreatePostContext()
	s.pristineRootHash = s.pristinePostContext[merkle.TREE_DEPTH]
	epoch.TxBufferDataBlock = &s.pristineRootHash
	app.Status = model.ApplicationStatus_UnexpectedYield
	r.problem = &model.ApplicationProblem{EpochIndex: epoch.Index + 1, Kind: app.Status,
		InputIndex: new(epoch.InputIndexUpperBound)}
	later := &model.Epoch{ApplicationID: app.ID, Index: epoch.Index + 1, VirtualIndex: epoch.VirtualIndex + 1,
		Status: model.EpochStatus_InputsProcessed}
	r.On("ListApplications", mock.Anything, validationApplicationsFilter(), repository.Pagination{}, false).
		Return([]*model.Application{app}, uint64(1), nil).Once()
	r.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(), mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{epoch, later}, uint64(2), nil).Once()
	expectValidPrefixEpoch(r.Mockrepo, app, epoch, false)
	r.On("GetEpochByVirtualIndex", mock.Anything, app.IApplicationAddress.Hex(), epoch.VirtualIndex-1).
		Return(&model.Epoch{TxBufferDataBlock: &s.pristineRootHash}, nil).Once()
	r.On("StoreClaimAndProofs", mock.Anything, epoch, mock.Anything).Return(nil).Once()
	_, err := s.Tick(t.Context())
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
	require.Equal(t, model.EpochStatus_InputsProcessed, later.Status)
	require.Equal(t, model.ApplicationStatus_UnexpectedYield, app.Status)
	require.Equal(t, 2, r.pageCalls)
	r.AssertExpectations(t)
}

func TestOpaqueCommitmentProofNamesBothCauses(t *testing.T) {
	r, s, app, epoch := newPagedCommitmentFixture(t, 0, 0)
	const height = model.Log2EpochComputationHashLeafCount
	tree := merkle.OpaqueTree(common.HexToHash("0xabcd"), 1).Iterated(height - 1)
	expectNoStoredProtectionRoots(r.Mockrepo, app)
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).Return(nil).Once()
	_, _, err := s.proveCommitment(t.Context(), app, epoch, tree)
	require.ErrorIs(t, err, merkle.ErrOpaqueTree)
	require.ErrorContains(t, err, "last input fills its span")
	require.ErrorContains(t, err, "stored padding row is not the epoch's final entry")
	require.Equal(t, model.ApplicationStatus_Failed, app.Status)
	r.AssertExpectations(t)
}
