// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"log/slog"
	"math/big"
	"os"
	"testing"

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
	map[address]claimRow,
	map[address]claimRow,
	error,
) {
	args := m.Called()
	return args.Get(0).(map[address]claimRow),
		args.Get(1).(map[address]claimRow),
		args.Error(2)
}
func (m *serviceMock) updateEpochWithSubmittedClaim(
	claim *claimRow,
	txHash Hash,
) error {
	args := m.Called(claim, txHash)
	return args.Error(0)
}

func (m *serviceMock) findClaimSubmissionEventAndSucc(
	claim *claimRow,
) (
	*iconsensus.IConsensus,
	*claimSubmissionEvent,
	*claimSubmissionEvent,
	error,
) {
	args := m.Called(claim)
	return args.Get(0).(*iconsensus.IConsensus),
		args.Get(1).(*claimSubmissionEvent),
		args.Get(2).(*claimSubmissionEvent),
		args.Error(3)
}
func (m *serviceMock) submitClaimToBlockchain(
	instance *iconsensus.IConsensus,
	claim *claimRow,
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
			claimsInFlight:    map[address]hash{},
		},
	}
}

// //////////////////////////////////////////////////////////////////////////////
// Success
// //////////////////////////////////////////////////////////////////////////////
func TestDoNothing(t *testing.T) {
	m := newServiceMock()
	prevClaims := map[address]claimRow{}
	currClaims := map[address]claimRow{}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)

	errs := m.submitClaimsAndUpdateDatabase(m)
	assert.Equal(t, len(errs), 0)
}

func TestSubmitFirstClaim(t *testing.T) {
	m := newServiceMock()
	appContractAddress := common.HexToAddress("0x01")
	claimTransactionHash := common.HexToHash("0x10")
	currClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         3,
		EpochFirstBlock:    30,
		EpochLastBlock:     39,
	}

	var prevEvent *claimSubmissionEvent = nil
	var currEvent *claimSubmissionEvent = nil
	prevClaims := map[address]claimRow{}
	currClaims := map[address]claimRow{
		appContractAddress: currClaim,
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
	m := newServiceMock()
	appContractAddress := common.HexToAddress("0x01")
	claimTransactionHash := common.HexToHash("0x10")
	prevClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         1,
		EpochFirstBlock:    10,
		EpochLastBlock:     19,
	}
	currClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         3,
		EpochFirstBlock:    30,
		EpochLastBlock:     39,
	}

	prevClaims := map[address]claimRow{
		appContractAddress: prevClaim,
	}
	var currEvent *claimSubmissionEvent = nil
	prevEvent := &claimSubmissionEvent{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.EpochLastBlock),
		AppContract:              appContractAddress,
	}
	currClaims := map[address]claimRow{
		appContractAddress: currClaim,
	}

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("findClaimSubmissionEventAndSucc", &prevClaim).
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

func TestSkipSubmitFirstClaim(t *testing.T) {
	m := newServiceMock()
	m.submissionEnabled = false
	appContractAddress := common.HexToAddress("0x01")
	claimTransactionHash := common.HexToHash("0x10")
	currClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         3,
		EpochFirstBlock:    30,
		EpochLastBlock:     39,
	}

	var prevEvent *claimSubmissionEvent = nil
	var currEvent *claimSubmissionEvent = nil
	prevClaims := map[address]claimRow{}
	currClaims := map[address]claimRow{
		appContractAddress: currClaim,
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
	prevClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         1,
		EpochFirstBlock:    10,
		EpochLastBlock:     19,
	}
	currClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         3,
		EpochFirstBlock:    30,
		EpochLastBlock:     39,
	}

	prevClaims := map[address]claimRow{
		appContractAddress: prevClaim,
	}
	var currEvent *claimSubmissionEvent = nil
	prevEvent := &claimSubmissionEvent{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.EpochLastBlock),
		AppContract:              appContractAddress,
	}
	currClaims := map[address]claimRow{
		appContractAddress: currClaim,
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
	currClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         1,
		EpochFirstBlock:    10,
		EpochLastBlock:     19,
	}
	prevClaims := map[address]claimRow{}
	currClaims := map[address]claimRow{
		appContractAddress: currClaim,
	}
	m.claimsInFlight[appContractAddress] = reqHash

	m.On("selectClaimPairsPerApp").
		Return(prevClaims, currClaims, nil)
	m.On("pollTransaction", reqHash).
		Return(true, &types.Receipt{
			ContractAddress: appContractAddress,
			TxHash: txHash,
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
	currClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         3,
		EpochFirstBlock:    30,
		EpochLastBlock:     39,
	}

	var nilEvent *claimSubmissionEvent = nil
	currEvent := claimSubmissionEvent{
		AppContract:              appContractAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(currClaim.EpochLastBlock),
	}
	prevClaims := map[address]claimRow{}
	currClaims := map[address]claimRow{
		appContractAddress: currClaim,
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
	prevClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         1,
		EpochFirstBlock:    10,
		EpochLastBlock:     19,
	}
	currClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         3,
		EpochFirstBlock:    30,
		EpochLastBlock:     39,
	}

	prevEvent := claimSubmissionEvent{
		AppContract:              appContractAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.EpochLastBlock),
	}
	currEvent := claimSubmissionEvent{
		AppContract:              appContractAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(currClaim.EpochLastBlock),
	}
	prevClaims := map[address]claimRow{
		appContractAddress: prevClaim,
	}
	currClaims := map[address]claimRow{
		appContractAddress: currClaim,
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
	prevClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         1,
		EpochFirstBlock:    10,
		EpochLastBlock:     19,
	}
	currClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         3,
		EpochFirstBlock:    30,
		EpochLastBlock:     39,
	}

	prevClaims := map[address]claimRow{
		appContractAddress: prevClaim,
	}
	var currEvent *claimSubmissionEvent = nil
	prevEvent := &claimSubmissionEvent{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.EpochLastBlock + 1),
		AppContract:              appContractAddress,
	}
	currClaims := map[address]claimRow{
		appContractAddress: currClaim,
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
	prevClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         1,
		EpochFirstBlock:    10,
		EpochLastBlock:     19,
	}
	currClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         3,
		EpochFirstBlock:    30,
		EpochLastBlock:     39,
	}

	prevEvent := claimSubmissionEvent{
		AppContract:              appContractAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.EpochLastBlock),
	}
	currEvent := claimSubmissionEvent{
		AppContract:              appContractAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.EpochLastBlock + 1),
	}
	prevClaims := map[address]claimRow{
		appContractAddress: prevClaim,
	}
	currClaims := map[address]claimRow{
		appContractAddress: currClaim,
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
	prevClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         2,
		EpochFirstBlock:    20,
		EpochLastBlock:     29,
	}
	currClaim := claimRow{
		AppContractAddress: appContractAddress,
		AppIConsensusAddress: appContractAddress,
		EpochIndex:         1,
		EpochFirstBlock:    10,
		EpochLastBlock:     19,
	}

	prevClaims := map[address]claimRow{
		appContractAddress: prevClaim,
	}
	var currEvent *claimSubmissionEvent = nil
	prevEvent := &claimSubmissionEvent{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevClaim.EpochLastBlock + 1),
		AppContract:              appContractAddress,
	}
	currClaims := map[address]claimRow{
		appContractAddress: currClaim,
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

