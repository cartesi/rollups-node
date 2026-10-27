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
	"slices"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type outputScanRange struct{ first, last uint64 }

type outputsRepository struct {
	*Mockrepo
	t                 *testing.T
	app               *model.Application
	epoch             *model.Epoch
	span              repository.EpochOutputRange
	raw               func(uint64) []byte
	scanFault         func(bool, uint64, uint64, repository.OutputScanFunc) (bool, error)
	onRow             func(bool, uint64, []byte)
	onBatch           func([]repository.OutputProof)
	writeErr          error
	scans             [2][]outputScanRange
	committed         []repository.OutputProof
	keepProofs        bool
	enforceBatchLimit bool
	inWrite, inScan   bool
	storedStatus      model.EpochStatus
}

func (repo *outputsRepository) GetEpochOutputRange(
	_ context.Context, appID int64, epochIndex uint64,
) (repository.EpochOutputRange, error) {
	require.Equal(repo.t, repo.app.ID, appID)
	require.Equal(repo.t, repo.epoch.Index, epochIndex)
	return repo.span, nil
}

func (repo *outputsRepository) GetOutput(ctx context.Context, address string, index uint64) (*model.Output, error) {
	require.Equal(repo.t, repo.app.IApplicationAddress.String(), address)
	require.Equal(repo.t, repo.span.First-1, index)
	return repo.Mockrepo.GetOutput(ctx, address, index)
}

func (repo *outputsRepository) ScanOutputs(
	_ context.Context, appID int64, first, last uint64, fn repository.OutputScanFunc,
) error {
	require.Equal(repo.t, repo.app.ID, appID)
	if repo.inWrite {
		return errors.New("pool scan inside claim transaction")
	}
	return repo.scan(false, first, last, fn)
}

func (repo *outputsRepository) scan(second bool, first, last uint64, fn repository.OutputScanFunc) error {
	if repo.inScan {
		return errors.New("connection busy: nested scan")
	}
	repo.inScan = true
	defer func() { repo.inScan = false }()
	pass := 0
	if second {
		pass = 1
	}
	repo.scans[pass] = append(repo.scans[pass], outputScanRange{first, last})
	if repo.scanFault != nil {
		handled, err := repo.scanFault(second, first, last, fn)
		if handled || err != nil {
			return err
		}
	}
	for index := first; index <= last; index++ {
		raw := repo.raw(index)
		if err := fn(index, raw); err != nil {
			return err
		}
		if repo.onRow != nil {
			repo.onRow(second, index, raw)
		}
	}
	return nil
}

type outputsWriter struct {
	repo   *outputsRepository
	staged []repository.OutputProof
}

func (writer *outputsWriter) ScanOutputs(
	_ context.Context, first, last uint64, fn repository.OutputScanFunc,
) error {
	return writer.repo.scan(true, first, last, fn)
}

func (writer *outputsWriter) StoreOutputProofs(_ context.Context, proofs []repository.OutputProof) error {
	if writer.repo.inScan {
		return errors.New("connection busy: proof write during scan")
	}
	// This literal pins the validator's configured chunk size independently.
	if writer.repo.enforceBatchLimit && len(proofs) > 2048 {
		return errors.New("proof batch exceeds 2048 outputs")
	}
	if writer.repo.onBatch != nil {
		writer.repo.onBatch(proofs)
	}
	if writer.repo.writeErr != nil && len(writer.staged) != 0 {
		return writer.repo.writeErr
	}
	if writer.repo.keepProofs {
		for _, proof := range proofs {
			proof.Siblings = slices.Clone(proof.Siblings)
			writer.staged = append(writer.staged, proof)
		}
	}
	return nil
}

func (repo *outputsRepository) StoreClaimAndProofs(
	ctx context.Context, epoch *model.Epoch, write repository.ClaimProofWriteFunc,
) error {
	require.Equal(repo.t, repo.app.ID, epoch.ApplicationID)
	require.Equal(repo.t, repo.epoch.Index, epoch.Index)
	if repo.inWrite {
		return errors.New("nested claim write")
	}
	repo.inWrite = true
	defer func() { repo.inWrite = false }()
	writer := &outputsWriter{repo: repo}
	if write != nil {
		if err := write(ctx, writer); err != nil {
			return err
		}
	}
	repo.committed = writer.staged
	repo.storedStatus = model.EpochStatus_ClaimComputed
	return nil
}

func generatedOutput(index uint64) []byte {
	raw := make([]byte, 132)
	binary.BigEndian.PutUint64(raw, index)
	return raw
}

func newOutputsFixture(t *testing.T, begin, count uint64) (*Service, *outputsRepository, []common.Hash) {
	t.Helper()
	post := merkle.CreatePostContext()
	app := &model.Application{ID: 73, Name: "streaming-output-fixture", Enabled: true, Status: model.ApplicationStatus_OK,
		ConsensusType: model.Consensus_Authority, IApplicationAddress: common.HexToAddress("0x1234")}
	epoch := &model.Epoch{ApplicationID: app.ID, Index: 5, VirtualIndex: 2,
		Status: model.EpochStatus_InputsProcessed, MachineHash: new(common.HexToHash("0x5678"))}
	repo := &outputsRepository{Mockrepo: newMockrepo(), t: t, app: app, epoch: epoch,
		span: repository.EpochOutputRange{Count: count, First: begin}, raw: generatedOutput,
		keepProofs: true, enforceBatchLimit: true, storedStatus: model.EpochStatus_InputsProcessed}
	if count > 0 {
		repo.span.Last = begin + count - 1
	}
	repo.application = app
	pre := post
	previousRoot := post[merkle.TREE_DEPTH]
	if begin != 0 {
		hash := crypto.Keccak256Hash([]byte("previous output"))
		siblings := make([]common.Hash, merkle.TREE_DEPTH)
		for i := range siblings {
			siblings[i] = crypto.Keccak256Hash([]byte{byte(i), 42})
		}
		previous := &model.Output{Index: begin - 1, Hash: &hash, OutputHashesSiblings: siblings}
		repo.On("GetOutput", mock.Anything, app.IApplicationAddress.String(), begin-1).Return(previous, nil).Once()
		pre = merkle.CreatePreContextFromProof(previous.Index, hash, siblings)
		previousRoot = merkletest.ComputeRootHashFromProof(previous.Index, hash, siblings)
	}
	previous := &model.Epoch{ApplicationID: app.ID, Index: 3, VirtualIndex: 1, TxBufferDataBlock: &previousRoot}
	repo.On("GetEpochByVirtualIndex", mock.Anything, app.IApplicationAddress.String(), uint64(1)).Return(previous, nil).Once()
	service := &Service{repository: repo, pristinePostContext: post, pristineRootHash: post[merkle.TREE_DEPTH]}
	service.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	return service, repo, pre
}

func recursiveOutputsOracle(t *testing.T, repo *outputsRepository, pre, post []common.Hash) (common.Hash, []repository.OutputProof) {
	t.Helper()
	leaves := make([]common.Hash, repo.span.Count)
	for i := range leaves {
		leaves[i] = crypto.Keccak256Hash(repo.raw(repo.span.First + uint64(i)))
	}
	siblings, err := merkletest.ComputeSiblingsMatrix(pre, leaves, post, repo.span.First)
	require.NoError(t, err)
	proofs := make([]repository.OutputProof, len(leaves))
	for i, leaf := range leaves {
		proofs[i] = repository.OutputProof{Index: repo.span.First + uint64(i), Hash: leaf,
			Siblings: slices.Clone(siblings[i*merkle.TREE_DEPTH : (i+1)*merkle.TREE_DEPTH])}
	}
	return merkletest.ComputeRootHashFromProof(repo.span.First, leaves[0], proofs[0].Siblings), proofs
}

func TestOutputsTwoPassesMatchRecursiveOracle(t *testing.T) {
	const page = uint64(2048)
	type testRange struct{ begin, count uint64 }
	cases := make([]testRange, 0, 32)
	cases = append(cases, []testRange{{0, 1}, {0, 2}, {0, page - 1}, {0, page}, {0, page + 1}, {0, 2*page + 7},
		{1, page + 9}, {page - 1, page + 9}, {3*page + 5, page + 9}, {1<<40 + 3, page + 9},
		{1<<63 - page - 9, page + 9}, {1<<63 - 1, 1}}...)
	for _, test := range cases {
		t.Run(fmt.Sprintf("%d_%d", test.begin, test.count), func(t *testing.T) {
			service, repo, pre := newOutputsFixture(t, test.begin, test.count)
			expectedRoot, expectedProofs := recursiveOutputsOracle(t, repo, pre, service.pristinePostContext)
			root, write, err := service.computeMerkleTreeAndProofs(t.Context(), repo.app, repo.epoch)
			require.NoError(t, err)
			require.Equal(t, expectedRoot, *root)
			require.NotNil(t, write)
			require.NoError(t, repo.StoreClaimAndProofs(t.Context(), repo.epoch, write))
			assertOutputProofs(t, expectedProofs, repo.committed)
			require.Equal(t, repo.scans[0], repo.scans[1])
			expectedChunks := (test.begin+test.count-1)/page - test.begin/page + 1
			require.Equal(t, expectedChunks, uint64(len(repo.scans[0])))
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}

func TestOutputsFences(t *testing.T) {
	const scanError = "scan error"
	const secondScanError = "second scan error"
	const storeError = "store error"
	for _, fault := range []string{
		"lost first", "lost second", "changed second", "reordered first", "reordered second",
		"extra first", "extra second", scanError, secondScanError, storeError,
	} {
		t.Run(fault, func(t *testing.T) {
			service, repo, _ := newOutputsFixture(t, 0, 4103)
			sentinel := errors.New("storage failed")
			repo.scanFault = func(second bool, first, last uint64, fn repository.OutputScanFunc) (bool, error) {
				switch {
				case fault == scanError || fault == secondScanError && second:
					return true, sentinel
				case fault == "lost first" && !second, fault == "lost second" && second:
					for index := first; index < last; index++ {
						if err := fn(index, repo.raw(index)); err != nil {
							return true, err
						}
					}
					return true, nil
				case fault == "changed second" && second:
					for index := first; index <= last; index++ {
						raw := repo.raw(index)
						raw[0] ^= 1
						if err := fn(index, raw); err != nil {
							return true, err
						}
					}
					return true, nil
				case fault == "reordered first" && !second, fault == "reordered second" && second:
					if last != repo.span.Last {
						return false, nil
					}
					for index := first; index <= last; index++ {
						swapped := index
						if index == first {
							swapped = first + 1
						}
						if index == first+1 {
							swapped = first
						}
						// Keep payload order unchanged so only the row-index check detects this fault.
						if err := fn(swapped, repo.raw(index)); err != nil {
							return true, err
						}
					}
					return true, nil
				case fault == "extra first" && !second, fault == "extra second" && second:
					if last != repo.span.Last {
						return false, nil
					}
					for index := first; index <= last; index++ {
						if err := fn(index, repo.raw(index)); err != nil {
							return true, err
						}
					}
					return true, fn(last+1, repo.raw(last+1))
				}
				return false, nil
			}
			if fault == storeError {
				repo.writeErr = sentinel
			}
			_, write, err := service.computeMerkleTreeAndProofs(t.Context(), repo.app, repo.epoch)
			if err == nil {
				err = repo.StoreClaimAndProofs(t.Context(), repo.epoch, write)
			}
			if fault == scanError || fault == secondScanError || fault == storeError {
				require.ErrorIs(t, err, sentinel)
			} else {
				require.ErrorIs(t, err, merkle.ErrOutputsChanged)
			}
			require.Empty(t, repo.committed)
			require.Equal(t, model.EpochStatus_InputsProcessed, repo.storedStatus)
			require.Equal(t, model.ApplicationStatus_OK, repo.app.Status)
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
	for _, span := range []repository.EpochOutputRange{{Count: 2, First: 0, Last: 2}, {Count: 1, First: 2, Last: 1}} {
		t.Run(fmt.Sprint(span), func(t *testing.T) {
			service, repo, _ := newOutputsFixture(t, 0, 1)
			repo.span = span
			reason := fmt.Sprintf("outputs of epoch 5 are not contiguous: %d outputs from index %d to %d",
				span.Count, span.First, span.Last)
			repo.On("UpdateApplicationStatus", mock.Anything, repo.app.ID, model.ApplicationStatus_Corrupted, &reason).Return(nil).Once()
			_, write, err := service.computeMerkleTreeAndProofs(t.Context(), repo.app, repo.epoch)
			require.ErrorContains(t, err, reason)
			require.Nil(t, write)
			require.Empty(t, repo.scans[0])
			repo.AssertExpectations(t)
		})
	}
	t.Run("invalid tree range is a code invariant", func(t *testing.T) {
		service, repo, _ := newOutputsFixture(t, 1<<63-1, 2)
		_, _, err := service.computeMerkleTreeAndProofs(t.Context(), repo.app, repo.epoch)
		require.ErrorIs(t, err, merkle.ErrInvariant)
		repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestOutputsValidateApplicationWritesProofs(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			service, repo, pre := newOutputsFixture(t, 0, 4103)
			root, proofs := recursiveOutputsOracle(t, repo, pre, service.pristinePostContext)
			repo.epoch.TxBufferDataBlock = &root
			repo.On("ListEpochs", mock.Anything, repo.app.IApplicationAddress.String(), mock.Anything, repository.Pagination{}, false).
				Return([]*model.Epoch{repo.epoch}, uint64(1), nil).Once()
			repo.On("GetLastInput", mock.Anything, repo.app.IApplicationAddress.String(), repo.epoch.Index).
				Return(&model.Input{MachineHash: repo.epoch.MachineHash, TxBufferDataBlock: &root}, nil).Once()
			sentinel := errors.New("second batch failed")
			if fail {
				repo.writeErr = sentinel
			}
			err := service.validateApplication(t.Context(), repo.app)
			if fail {
				require.ErrorIs(t, err, sentinel)
				require.Empty(t, repo.committed)
				require.Equal(t, model.EpochStatus_InputsProcessed, repo.storedStatus)
			} else {
				require.NoError(t, err)
				assertOutputProofs(t, proofs, repo.committed)
				require.Equal(t, model.EpochStatus_ClaimComputed, repo.storedStatus)
			}
			require.Equal(t, model.ApplicationStatus_OK, repo.app.Status)
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}

func TestOutputsBorrowedRowsAreHashedInsideCallback(t *testing.T) {
	service, repo, pre := newOutputsFixture(t, 0, 4103)
	expectedRoot, expectedProofs := recursiveOutputsOracle(t, repo, pre, service.pristinePostContext)
	repo.scanFault = func(_ bool, first, last uint64, fn repository.OutputScanFunc) (bool, error) {
		var borrowed [132]byte
		for index := first; index <= last; index++ {
			binary.BigEndian.PutUint64(borrowed[:], index)
			if err := fn(index, borrowed[:]); err != nil {
				return true, err
			}
			clear(borrowed[:])
		}
		return true, nil
	}
	root, write, err := service.computeMerkleTreeAndProofs(t.Context(), repo.app, repo.epoch)
	require.NoError(t, err)
	require.Equal(t, expectedRoot, *root)
	require.NoError(t, repo.StoreClaimAndProofs(t.Context(), repo.epoch, write))
	assertOutputProofs(t, expectedProofs, repo.committed)
}

// Compare bounded elements so a proof regression cannot dump an entire epoch.
func assertOutputProofs(t *testing.T, want, got []repository.OutputProof) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		require.Equal(t, want[i].Index, got[i].Index, "proof %d index", i)
		require.Equal(t, want[i].Hash, got[i].Hash, "proof %d hash", i)
		require.Len(t, got[i].Siblings, len(want[i].Siblings), "proof %d sibling count", i)
		for j := range want[i].Siblings {
			require.Equal(t, want[i].Siblings[j], got[i].Siblings[j], "proof %d sibling %d", i, j)
		}
	}
}

func TestOutputsWriterScanPreservesFullPayloadAndProofIdentity(t *testing.T) {
	service, repo, _ := newOutputsFixture(t, 0, 3)
	repo.raw = func(index uint64) []byte {
		raw := generatedOutput(index)
		raw[97], raw[131] = byte(index%3+11), byte(index%3+91)
		return raw
	}
	leaves := make([]common.Hash, 3)
	for i := range leaves {
		leaves[i] = crypto.Keccak256Hash(repo.raw(uint64(i)))
	}
	history := merkletest.NewOutputHistory(leaves)
	var rows [2]int
	repo.onRow = func(second bool, _ uint64, _ []byte) {
		if second {
			require.True(t, repo.inWrite)
			rows[1]++
		} else {
			require.False(t, repo.inWrite)
			rows[0]++
		}
	}
	root, write, err := service.computeMerkleTreeAndProofs(t.Context(), repo.app, repo.epoch)
	require.NoError(t, err)
	require.Equal(t, history.Root(), *root)
	require.NoError(t, repo.StoreClaimAndProofs(t.Context(), repo.epoch, write))
	require.Equal(t, [2]int{3, 3}, rows)
	want := make([]repository.OutputProof, len(leaves))
	for i, hash := range leaves {
		want[i] = repository.OutputProof{Index: uint64(i), Hash: hash, Siblings: history.Proof(uint64(i))}
	}
	assertOutputProofs(t, want, repo.committed)
}
