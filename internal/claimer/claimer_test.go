// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"log/slog"
	"math/big"
	"os"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/lmittmann/tint"

	"github.com/ethereum/go-ethereum/common"
	. "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type serviceMock struct {
	mock.Mock
	Service
}

func (m *serviceMock) selectClaimPairsPerApp() (
	map[common.Address]*model.ClaimRow,
	map[common.Address]*model.ClaimRow,
	error,
) {
	args := m.Called()
	return args.Get(0).(map[common.Address]*model.ClaimRow),
		args.Get(1).(map[common.Address]*model.ClaimRow),
		args.Error(2)
}
func (m *serviceMock) updateEpochWithSubmittedClaim(
	claim *model.ClaimRow,
	txHash Hash,
) error {
	args := m.Called(claim, txHash)
	return args.Error(0)
}

func (m *serviceMock) findClaimSubmissionEventAndSucc(
	claim *model.ClaimRow,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimSubmission,
	*iconsensus.IConsensusClaimSubmission,
	error,
) {
	args := m.Called(claim)
	return args.Get(0).(*iconsensus.IConsensus),
		args.Get(1).(*iconsensus.IConsensusClaimSubmission),
		args.Get(2).(*iconsensus.IConsensusClaimSubmission),
		args.Error(3)
}
func (m *serviceMock) submitClaimToBlockchain(
	instance *iconsensus.IConsensus,
	claim *model.ClaimRow,
) (Hash, error) {
	args := m.Called(nil, claim)
	return args.Get(0).(Hash), args.Error(1)
}
func (m *serviceMock) pollTransaction(txHash Hash) (bool, *types.Receipt, error) {
	args := m.Called(txHash)
	return args.Bool(0),
		args.Get(1).(*types.Receipt),
		args.Error(2)
}

func newServiceMock() *serviceMock {
	opts := &tint.Options{
		Level:     slog.LevelDebug,
		AddSource: true,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
	}
	handler := tint.NewHandler(os.Stdout, opts)

	return &serviceMock{
		Service: Service{
			Service: service.Service{
				Logger: slog.New(handler),
			},
			submissionEnabled: true,
			claimsInFlight:    map[common.Address]common.Hash{},
		},
	}
}

// //////////////////////////////////////////////////////////////////////////////
// Success
// //////////////////////////////////////////////////////////////////////////////
func TestDoNothing(t *testing.T) {
	m := newServiceMock()
	prevClaims := map[common.Address]*model.ClaimRow{}
	currClaims := map[common.Address]*model.ClaimRow{}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 0)
}

func TestSubmitFirstClaim(t *testing.T) {
	m := newServiceMock()
	appContractAddress := common.HexToAddress("0x01")
	claimTransactionHash := common.HexToHash("0x10")
	currClaim := model.ClaimRow{
		IApplicationAddress: appContractAddress,
		IConsensusAddress:   appContractAddress,
		Epoch: model.Epoch{
			Index:           3,
			FirstBlock:      30,
			LastBlock:       39,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}

	var prevEvent *iconsensus.IConsensusClaimSubmission = nil
	var currEvent *iconsensus.IConsensusClaimSubmission = nil
	prevClaims := map[common.Address]*model.ClaimRow{}
	currClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &currClaim,
	}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("findClaimSubmissionEventAndSucc", &currClaim).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil)
	m.On("submitClaimToBlockchain", nil, &currClaim).
		Return(claimTransactionHash, nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 0)
	assert.Equal(t, len(m.claimsInFlight), 1)
	m.AssertNumberOfCalls(t, "findClaimSubmissionEventAndSucc", 1)
	m.AssertNumberOfCalls(t, "pollTransaction", 0)
	m.AssertNumberOfCalls(t, "selectClaimPairsPerApp", 1)
	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 1)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 0)
}

func TestSubmitClaimWithAntecessor(t *testing.T) {

	// There can be only one submitted claim at a time.
	// Skip submission if prev epoch status is ClaimSubmitted.
	testCases := []struct{
		epochStatus model.EpochStatus
		submit bool
	}{
		{
			model.EpochStatus_ClaimAccepted,
			true,
		},
		{
			model.EpochStatus_ClaimSubmitted,
			false,
		},
	}

	for _, test := range testCases {
		m := newServiceMock()
		appContractAddress := common.HexToAddress("0x01")
		claimTransactionHash := common.HexToHash("0x10")
		prevClaim := model.ClaimRow{
			IApplicationAddress:   appContractAddress,
			IConsensusAddress: appContractAddress,
			Epoch: model.Epoch{
				Index:           1,
				FirstBlock:      10,
				LastBlock:       19,
				ClaimHash: &common.Hash{},
				ClaimTransactionHash: &common.Hash{},
				Status: test.epochStatus,
			},
		}
		currClaim := model.ClaimRow{
			IApplicationAddress:   appContractAddress,
			IConsensusAddress: appContractAddress,
			Epoch: model.Epoch{
				Index:           3,
				FirstBlock:      30,
				LastBlock:       39,
				ClaimHash: &common.Hash{},
				ClaimTransactionHash: &common.Hash{},
				Status: model.EpochStatus_ClaimComputed,
			},
		}

		prevClaims := map[common.Address]*model.ClaimRow{
			appContractAddress: &prevClaim,
		}
		var currEvent *iconsensus.IConsensusClaimSubmission = nil
		prevEvent := &iconsensus.IConsensusClaimSubmission{
			LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.LastBlock),
			AppContract:              appContractAddress,
		}
		currClaims := map[common.Address]*model.ClaimRow{
			appContractAddress: &currClaim,
		}

		m.On("selectClaimPairsPerApp").
			Return(prevClaims, currClaims, nil)
		m.On("findClaimSubmissionEventAndSucc", &prevClaim).
			Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil)
		m.On("submitClaimToBlockchain", nil, &currClaim).
			Return(claimTransactionHash, nil)

		errs := m.submitClaimsAndUpdateDatabase(m)
		assert.Equal(t, len(errs), 0)
		m.AssertNumberOfCalls(t, "findClaimSubmissionEventAndSucc", 1)
		m.AssertNumberOfCalls(t, "pollTransaction", 0)
		m.AssertNumberOfCalls(t, "selectClaimPairsPerApp", 1)
		m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 0)

		if test.submit {
			assert.Equal(t, len(m.claimsInFlight), 1)
			m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 1)
		} else {
			assert.Equal(t, len(m.claimsInFlight), 0)
			m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 0)
		}
	}
}

func TestSkipSubmitFirstClaim(t *testing.T) {
	m := newServiceMock()
	m.submissionEnabled = false
	appContractAddress := common.HexToAddress("0x01")
	claimTransactionHash := common.HexToHash("0x10")
	currClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           3,
			FirstBlock:      30,
			LastBlock:       39,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}

	var prevEvent *iconsensus.IConsensusClaimSubmission = nil
	var currEvent *iconsensus.IConsensusClaimSubmission = nil
	prevClaims := map[common.Address]*model.ClaimRow{}
	currClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &currClaim,
	}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("findClaimSubmissionEventAndSucc", &currClaim).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil)
	m.On("submitClaimToBlockchain", nil, &currClaim).
		Return(claimTransactionHash, nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 0)
	assert.Equal(t, len(m.claimsInFlight), 0)
	m.AssertNumberOfCalls(t, "findClaimSubmissionEventAndSucc", 1)
	m.AssertNumberOfCalls(t, "pollTransaction", 0)
	m.AssertNumberOfCalls(t, "selectClaimPairsPerApp", 1)
	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 0)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 0)
}

func TestSkipSubmitClaimWithAntecessor(t *testing.T) {
	m := newServiceMock()
	m.submissionEnabled = false
	appContractAddress := common.HexToAddress("0x01")
	claimTransactionHash := common.HexToHash("0x10")
	prevClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           1,
			FirstBlock:      10,
			LastBlock:       19,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}
	currClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           3,
			FirstBlock:      30,
			LastBlock:       39,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}

	prevClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &prevClaim,
	}
	var currEvent *iconsensus.IConsensusClaimSubmission = nil
	prevEvent := &iconsensus.IConsensusClaimSubmission{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.LastBlock),
		AppContract:              appContractAddress,
	}
	currClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &currClaim,
	}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("findClaimSubmissionEventAndSucc", &prevClaim).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil)
	m.On("submitClaimToBlockchain", nil, &currClaim).
		Return(claimTransactionHash, nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 0)
	assert.Equal(t, len(m.claimsInFlight), 0)
	m.AssertNumberOfCalls(t, "findClaimSubmissionEventAndSucc", 1)
	m.AssertNumberOfCalls(t, "pollTransaction", 0)
	m.AssertNumberOfCalls(t, "selectClaimPairsPerApp", 1)
	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 0)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 0)
}

func TestInFlightCompleted(t *testing.T) {
	m := newServiceMock()
	appContractAddress := common.HexToAddress("0x01")
	reqHash := common.HexToHash("0x10")
	txHash := common.HexToHash("0x100")
	currClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           1,
			FirstBlock:      10,
			LastBlock:       19,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}
	prevClaims := map[common.Address]*model.ClaimRow{}
	currClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &currClaim,
	}
	m.claimsInFlight[appContractAddress] = reqHash

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("pollTransaction", reqHash).
		Return(true, &types.Receipt{
			ContractAddress: appContractAddress,
			TxHash:          txHash,
		}, nil)
	m.On("updateEpochWithSubmittedClaim", &currClaim, txHash).
		Return(nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 0)
	assert.Equal(t, len(m.claimsInFlight), 0)
	m.AssertNumberOfCalls(t, "findClaimSubmissionEventAndSucc", 0)
	m.AssertNumberOfCalls(t, "pollTransaction", 1)
	m.AssertNumberOfCalls(t, "selectClaimPairsPerApp", 1)
	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 0)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 1)
}

func TestUpdateFirstClaim(t *testing.T) {
	m := newServiceMock()
	appContractAddress := common.HexToAddress("0x01")
	currClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           3,
			FirstBlock:      30,
			LastBlock:       39,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}

	var nilEvent *iconsensus.IConsensusClaimSubmission = nil
	currEvent := iconsensus.IConsensusClaimSubmission{
		AppContract:              appContractAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(currClaim.LastBlock),
	}
	prevClaims := map[common.Address]*model.ClaimRow{}
	currClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &currClaim,
	}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("findClaimSubmissionEventAndSucc", &currClaim).
		Return(&iconsensus.IConsensus{}, &currEvent, nilEvent, nil)
	m.On("updateEpochWithSubmittedClaim", &currClaim, currEvent.Raw.TxHash).
		Return(nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 0)
	assert.Equal(t, len(m.claimsInFlight), 0)
	m.AssertNumberOfCalls(t, "findClaimSubmissionEventAndSucc", 1)
	m.AssertNumberOfCalls(t, "pollTransaction", 0)
	m.AssertNumberOfCalls(t, "selectClaimPairsPerApp", 1)
	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 0)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 1)
}

func TestUpdateClaimWithAntecessor(t *testing.T) {
	m := newServiceMock()
	appContractAddress := common.HexToAddress("0x01")
	prevClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           1,
			FirstBlock:      10,
			LastBlock:       19,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}
	currClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           3,
			FirstBlock:      30,
			LastBlock:       39,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}

	prevEvent := iconsensus.IConsensusClaimSubmission{
		AppContract:              appContractAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.LastBlock),
	}
	currEvent := iconsensus.IConsensusClaimSubmission{
		AppContract:              appContractAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(currClaim.LastBlock),
	}
	prevClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &prevClaim,
	}
	currClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &currClaim,
	}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("findClaimSubmissionEventAndSucc", &prevClaim).
		Return(&iconsensus.IConsensus{}, &prevEvent, &currEvent, nil)
	m.On("updateEpochWithSubmittedClaim", &currClaim, currEvent.Raw.TxHash).
		Return(nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 0)
	assert.Equal(t, len(m.claimsInFlight), 0)
	m.AssertNumberOfCalls(t, "findClaimSubmissionEventAndSucc", 1)
	m.AssertNumberOfCalls(t, "pollTransaction", 0)
	m.AssertNumberOfCalls(t, "selectClaimPairsPerApp", 1)
	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 0)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 1)
}

// //////////////////////////////////////////////////////////////////////////////
// Failure
// //////////////////////////////////////////////////////////////////////////////

// !claimMatchesEvent(prevClaim, prevEvent)
func TestSubmitClaimWithAntecessorMismatch(t *testing.T) {
	m := newServiceMock()
	appContractAddress := common.HexToAddress("0x01")
	claimTransactionHash := common.HexToHash("0x10")
	prevClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           1,
			FirstBlock:      10,
			LastBlock:       19,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}
	currClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           3,
			FirstBlock:      30,
			LastBlock:       39,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}

	prevClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &prevClaim,
	}
	var currEvent *iconsensus.IConsensusClaimSubmission = nil
	prevEvent := &iconsensus.IConsensusClaimSubmission{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.LastBlock + 1),
		AppContract:              appContractAddress,
	}
	currClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &currClaim,
	}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("findClaimSubmissionEventAndSucc", &prevClaim).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil)
	m.On("submitClaimToBlockchain", nil, &currClaim).
		Return(claimTransactionHash, nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 1)
	assert.Equal(t, errs[0], ErrEventMismatch)
}

// !claimMatchesEvent(currClaim, currEvent)
func TestSubmitClaimWithEventMismatch(t *testing.T) {
	m := newServiceMock()
	appContractAddress := common.HexToAddress("0x01")
	prevClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           1,
			FirstBlock:      10,
			LastBlock:       19,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}
	currClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           3,
			FirstBlock:      30,
			LastBlock:       39,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}

	prevEvent := iconsensus.IConsensusClaimSubmission{
		AppContract:              appContractAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.LastBlock),
	}
	currEvent := iconsensus.IConsensusClaimSubmission{
		AppContract:              appContractAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.LastBlock + 1),
	}
	prevClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &prevClaim,
	}
	currClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &currClaim,
	}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("findClaimSubmissionEventAndSucc", &prevClaim).
		Return(&iconsensus.IConsensus{}, &prevEvent, &currEvent, nil)
	m.On("updateEpochWithSubmittedClaim", &currClaim, currEvent.Raw.TxHash).
		Return(nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 1)
	assert.Equal(t, errs[0], ErrEventMismatch)
}

// !checkClaimsConstraint(prevClaim, currClaim)
func TestSubmitClaimWithAntecessorOutOfOrder(t *testing.T) {
	m := newServiceMock()
	appContractAddress := common.HexToAddress("0x01")
	claimTransactionHash := common.HexToHash("0x10")
	prevClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           2,
			FirstBlock:      20,
			LastBlock:       29,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},
		},
	}
	currClaim := model.ClaimRow{
		IApplicationAddress:   appContractAddress,
		IConsensusAddress: appContractAddress,
		Epoch: model.Epoch{
			Index:           1,
			FirstBlock:      10,
			LastBlock:       19,
			ClaimHash: &common.Hash{},
			ClaimTransactionHash: &common.Hash{},

		},
	}

	prevClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &prevClaim,
	}
	var currEvent *iconsensus.IConsensusClaimSubmission = nil
	prevEvent := &iconsensus.IConsensusClaimSubmission{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.LastBlock + 1),
		AppContract:              appContractAddress,
	}
	currClaims := map[common.Address]*model.ClaimRow{
		appContractAddress: &currClaim,
	}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("findClaimSubmissionEventAndSucc", &prevClaim).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil)
	m.On("submitClaimToBlockchain", nil, &currClaim).
		Return(claimTransactionHash, nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 1)
	assert.Equal(t, errs[0], ErrClaimMismatch)
}
