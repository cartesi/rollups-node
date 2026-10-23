// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type acceptanceReceiptFixture struct {
	service   *Service
	repo      *prtRepositoryMock
	client    *ethClientMock
	consensus *daveConsensusAdapterMock
	app       *model.Application
	epoch     *model.Epoch
	header    *types.Header
	headerErr error
	receipt   *types.Receipt
	log       *types.Log
}

func newAcceptanceReceiptFixture(t *testing.T) *acceptanceReceiptFixture {
	t.Helper()
	s, repo := newPRTServiceMock()
	app := prtRevertTestApp()
	app.LastTournamentCheckBlock = 100
	epoch := checkpointEpoch(0, "0x100")
	epoch.ClaimTransactionHash = new(common.HexToHash("0x900"))
	header := &types.Header{Number: big.NewInt(100)}
	raw := &types.Log{Address: app.IConsensusAddress, TxHash: *epoch.ClaimTransactionHash,
		BlockNumber: 100, BlockHash: header.Hash()}
	receipt := &types.Receipt{TxHash: raw.TxHash, BlockNumber: big.NewInt(100), BlockHash: raw.BlockHash,
		Status: types.ReceiptStatusSuccessful, Logs: []*types.Log{raw}}
	client := &ethClientMock{}
	s.client = client
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
		Return(&model.Tournament{Snapshot: model.TournamentSnapshot{
			AsOfBlock: 100, FinishedAtBlock: 90, WinnerCommitment: epoch.Commitment,
		}}, nil)
	f := &acceptanceReceiptFixture{service: s, repo: repo, client: client,
		consensus: &daveConsensusAdapterMock{}, app: app, epoch: epoch, header: header, receipt: receipt, log: raw}
	t.Cleanup(func() {
		repo.AssertExpectations(t)
		client.AssertExpectations(t)
		f.consensus.AssertExpectations(t)
	})
	return f
}

func (f *acceptanceReceiptFixture) expectReceipt() {
	f.client.On("TransactionReceipt", mock.Anything, *f.epoch.ClaimTransactionHash).Return(f.receipt, nil).Once()
	if f.receipt.BlockHash != (common.Hash{}) {
		f.client.On("HeaderByNumber", mock.Anything, f.receipt.BlockNumber).Return(f.header, f.headerErr).Once()
	}
}

func (f *acceptanceReceiptFixture) expectAcceptance() {
	f.consensus.On("ParseEpochSealed", *f.log).Return(&idaveconsensus.IDaveConsensusEpochSealed{
		EpochNumber: big.NewInt(1), InitialMachineStateHash: *f.epoch.MachineHash,
		OutputsMerkleRoot: *f.epoch.TxBufferDataBlock, Raw: *f.log,
	}, nil).Once()
	f.repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, f.app.ID, f.epoch.Index, f.epoch.ClaimTransactionHash).
		Return(nil).Once()
}

func TestAcceptanceReceiptRequiresCanonicalBlockAndMatchingLog(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*acceptanceReceiptFixture)
		cause  error
	}{
		{"orphaned receipt", func(f *acceptanceReceiptFixture) {
			f.receipt.BlockHash = common.HexToHash("0xbad")
			f.log.BlockHash = f.receipt.BlockHash
		}, txsender.ErrInconclusive},
		{"missing receipt block hash", func(f *acceptanceReceiptFixture) {
			f.receipt.BlockHash = common.Hash{}
		}, txsender.ErrInconclusive},
		{"unavailable canonical header", func(f *acceptanceReceiptFixture) {
			f.header = nil
		}, txsender.ErrInconclusive},
		{"header has no number", func(f *acceptanceReceiptFixture) {
			f.header = &types.Header{}
		}, txsender.ErrInconclusive},
		{"header has another height", func(f *acceptanceReceiptFixture) {
			f.header = &types.Header{Number: big.NewInt(99)}
		}, txsender.ErrInconclusive},
		{"removed log", func(f *acceptanceReceiptFixture) {
			f.log.Removed = true
		}, txsender.ErrInconclusive},
		{"log has another block hash", func(f *acceptanceReceiptFixture) {
			f.log.BlockHash = common.HexToHash("0xbad")
		}, txsender.ErrInconclusive},
		{"log has no block hash", func(f *acceptanceReceiptFixture) {
			f.log.BlockHash = common.Hash{}
		}, txsender.ErrInconclusive},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newAcceptanceReceiptFixture(t)
			test.change(f)
			f.expectReceipt()
			deferActions, err := f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, 100)
			require.ErrorIs(t, err, test.cause)
			require.True(t, deferActions)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.consensus.AssertNotCalled(t, "ParseEpochSealed", mock.Anything)
			f.repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestAcceptanceReceiptCanonicalHeaderFailureCanRecover(t *testing.T) {
	f := newAcceptanceReceiptFixture(t)
	f.headerErr = errors.New("temporary canonical header outage")
	f.expectReceipt()
	deferActions, err := f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, 100)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.ErrorIs(t, err, f.headerErr)
	require.True(t, deferActions)
	f.repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.headerErr = nil
	f.expectReceipt()
	f.expectAcceptance()
	deferActions, err = f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, 100)
	require.NoError(t, err)
	require.False(t, deferActions)
}

func TestOrphanedAcceptanceReceiptCanRecoverOnCanonicalBranch(t *testing.T) {
	f := newAcceptanceReceiptFixture(t)
	canonicalHash := f.header.Hash()
	f.receipt.BlockHash = common.HexToHash("0xbad")
	f.log.BlockHash = f.receipt.BlockHash
	f.expectReceipt()
	// An orphaned event must never reach the decoder or a durable decision.
	// The same transaction can later execute on the canonical branch.
	deferActions, err := f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, 100)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.True(t, deferActions)
	f.consensus.AssertNotCalled(t, "ParseEpochSealed", mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.receipt.BlockHash, f.log.BlockHash = canonicalHash, canonicalHash
	f.expectReceipt()
	f.expectAcceptance()
	deferActions, err = f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, 100)
	require.NoError(t, err)
	require.False(t, deferActions)
}

func TestCanonicalAcceptanceReceiptStillDetectsDivergence(t *testing.T) {
	for _, field := range []string{"machine", "outputs"} {
		t.Run(field, func(t *testing.T) {
			f := newAcceptanceReceiptFixture(t)
			f.expectReceipt()
			event := &idaveconsensus.IDaveConsensusEpochSealed{
				EpochNumber: big.NewInt(1), InitialMachineStateHash: *f.epoch.MachineHash,
				OutputsMerkleRoot: *f.epoch.TxBufferDataBlock, Raw: *f.log,
			}
			if field == "machine" {
				event.InitialMachineStateHash = common.HexToHash("0xbad")
			} else {
				event.OutputsMerkleRoot = common.HexToHash("0xbad")
			}
			f.consensus.On("ParseEpochSealed", *f.log).Return(event, nil).Once()
			f.repo.On("UpdateApplicationStatus", mock.Anything, f.app.ID, model.ApplicationStatus_Diverged, mock.Anything).
				Return(nil).Once()
			deferActions, err := f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, 100)
			require.ErrorContains(t, err, "inconsistent")
			require.True(t, deferActions)
			require.Equal(t, model.ApplicationStatus_Diverged, f.app.Status)
			f.repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}
