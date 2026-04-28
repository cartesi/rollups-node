// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/lmittmann/tint"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type claimerRepositoryMock struct {
	mock.Mock
}

func (m *claimerRepositoryMock) SelectSubmittedClaimPairsPerApp(ctx context.Context) (
	map[int64]*model.Epoch,
	map[int64]*model.Epoch,
	map[int64]*model.Application,
	error,
) {
	args := m.Called(ctx)
	return args.Get(0).(map[int64]*model.Epoch),
		args.Get(1).(map[int64]*model.Epoch),
		args.Get(2).(map[int64]*model.Application),
		args.Error(3)
}

func (m *claimerRepositoryMock) SelectAcceptedClaimPairsPerApp(ctx context.Context) (
	map[int64]*model.Epoch,
	map[int64]*model.Epoch,
	map[int64]*model.Application,
	error,
) {
	args := m.Called(ctx)
	return args.Get(0).(map[int64]*model.Epoch),
		args.Get(1).(map[int64]*model.Epoch),
		args.Get(2).(map[int64]*model.Application),
		args.Error(3)
}
func (m *claimerRepositoryMock) UpdateEpochWithSubmittedClaim(
	ctx context.Context,
	appid int64,
	index uint64,
	txHash common.Hash,
) error {
	args := m.Called(ctx, appid, index, txHash)
	return args.Error(0)
}

func (m *claimerRepositoryMock) UpdateApplicationState(
	ctx context.Context,
	appID int64,
	state model.ApplicationState,
	reason *string,
) error {
	args := m.Called(ctx, appID, state, reason)
	return args.Error(0)
}

func (m *claimerRepositoryMock) UpdateEpochWithAcceptedClaim(
	ctx context.Context,
	appid int64,
	index uint64,
) error {
	args := m.Called(ctx, appid, index)
	return args.Error(0)
}

func (m *claimerRepositoryMock) SaveNodeConfigRaw(
	ctx context.Context,
	key string,
	rawJSON []byte,
) error {
	args := m.Called(ctx, key, rawJSON)
	return args.Error(0)
}

func (m *claimerRepositoryMock) LoadNodeConfigRaw(ctx context.Context, key string) (
	rawJSON []byte,
	createdAt, updatedAt time.Time,
	err error,
) {
	args := m.Called(ctx, key)
	return args.Get(0).([]byte), args.Get(1).(time.Time), args.Get(2).(time.Time), args.Error(3)
}

type claimerBlockchainMock struct {
	mock.Mock
}

func (m *claimerBlockchainMock) findClaimSubmittedEventAndSucc(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimSubmitted,
	*iconsensus.IConsensusClaimSubmitted,
	error,
) {
	args := m.Called(ctx, app, epoch, fromBlock, toBlock)
	return args.Get(0).(*iconsensus.IConsensus),
		args.Get(1).(*iconsensus.IConsensusClaimSubmitted),
		args.Get(2).(*iconsensus.IConsensusClaimSubmitted),
		args.Error(3)
}

func (m *claimerBlockchainMock) findClaimAcceptedEventAndSucc(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimAccepted,
	*iconsensus.IConsensusClaimAccepted,
	error,
) {
	args := m.Called(ctx, app, epoch, fromBlock, toBlock)
	return args.Get(0).(*iconsensus.IConsensus),
		args.Get(1).(*iconsensus.IConsensusClaimAccepted),
		args.Get(2).(*iconsensus.IConsensusClaimAccepted),
		args.Error(3)
}

func (m *claimerBlockchainMock) submitClaimToBlockchain(
	instance *iconsensus.IConsensus,
	app *model.Application,
	epoch *model.Epoch,
) (common.Hash, error) {
	args := m.Called(instance, app, epoch)
	return args.Get(0).(common.Hash), args.Error(1)
}
func (m *claimerBlockchainMock) pollTransaction(
	ctx context.Context,
	txHash common.Hash,
	endBlock *big.Int,
) (bool, *types.Receipt, error) {
	args := m.Called(ctx, txHash, endBlock)
	return args.Bool(0),
		args.Get(1).(*types.Receipt),
		args.Error(2)
}
func (m *claimerBlockchainMock) getDefaultBlockNumber(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	return args.Get(0).(*big.Int),
		args.Error(1)
}

func (m *claimerBlockchainMock) getConsensusAddress(
	ctx context.Context,
	app *model.Application,
) (common.Address, error) {
	args := m.Called(ctx, app)
	return args.Get(0).(common.Address),
		args.Error(1)
}

func newServiceMock() (*Service, *claimerRepositoryMock, *claimerBlockchainMock) {
	opts := &tint.Options{
		Level:     slog.LevelDebug,
		AddSource: true,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
	}
	handler := tint.NewHandler(os.Stdout, opts)
	repository := &claimerRepositoryMock{}
	blockchain := &claimerBlockchainMock{}

	claimer := &Service{
		TickServiceTemplate: service.TickServiceTemplate{
			ServiceTemplate: service.ServiceTemplate{
				Logger: slog.New(handler),
			},
		},
		submissionEnabled: true,
		claimsInFlight:    map[int64]common.Hash{},
		repository:        repository,
		blockchain:        blockchain,
	}
	return claimer, repository, blockchain
}

func makeApplication() *model.Application {
	return repotest.NewApplicationBuilder().
		WithEpochLength(10).
		Build()
}

func makeEpoch(id int64, status model.EpochStatus, i uint64) *model.Epoch {
	outputsMerkleRoot := common.HexToHash("0x01") // dummy value
	txHash := common.HexToHash("0x02")            // dummy value
	return repotest.NewEpochBuilder(id).
		WithIndex(i).
		WithBlocks(i*10, i*10+9).
		WithStatus(status).
		WithClaimTransactionHash(txHash).
		WithOutputsMerkleRoot(outputsMerkleRoot).
		Build()
}

func makeAcceptedEpoch(app *model.Application, i uint64) *model.Epoch {
	return makeEpoch(app.ID, model.EpochStatus_ClaimAccepted, i)
}

func makeSubmittedEpoch(app *model.Application, i uint64) *model.Epoch {
	return makeEpoch(app.ID, model.EpochStatus_ClaimSubmitted, i)
}

func makeComputedEpoch(app *model.Application, i uint64) *model.Epoch {
	return makeEpoch(app.ID, model.EpochStatus_ClaimComputed, i)
}
func makeEpochMap(epochs ...*model.Epoch) map[int64]*model.Epoch {
	result := map[int64]*model.Epoch{}
	for _, epoch := range epochs {
		result[epoch.ApplicationID] = epoch
	}
	return result
}
func makeApplicationMap(apps ...*model.Application) map[int64]*model.Application {
	result := map[int64]*model.Application{}
	for _, app := range apps {
		result[app.ID] = app
	}
	return result
}

func makeSubmittedEvent(app *model.Application, epoch *model.Epoch) *iconsensus.IConsensusClaimSubmitted {
	return &iconsensus.IConsensusClaimSubmitted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(epoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *epoch.OutputsMerkleRoot,
		Raw: types.Log{
			TxHash:      common.HexToHash(epoch.ClaimTransactionHash.Hex()),
			BlockNumber: epoch.LastBlock + 5,
		},
	}
}

// makeClaimAcceptedLog creates a types.Log that ParseClaimAccepted can decode.
// Used to build receipt logs for the Authority fast-accept path in tests.
func makeClaimAcceptedLog(app *model.Application, epoch *model.Epoch) types.Log {
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		panic(fmt.Sprintf("failed to get IConsensus ABI: %v", err))
	}
	event, ok := parsed.Events["ClaimAccepted"]
	if !ok {
		panic("IConsensus ABI does not define ClaimAccepted event")
	}
	data, err := event.Inputs.NonIndexed().Pack(
		new(big.Int).SetUint64(epoch.LastBlock),
		*epoch.OutputsMerkleRoot,
	)
	if err != nil {
		panic(fmt.Sprintf("failed to pack ClaimAccepted event data: %v", err))
	}
	return types.Log{
		Topics: []common.Hash{
			event.ID,
			common.BytesToHash(app.IApplicationAddress.Bytes()),
		},
		Data: data,
	}
}

func makeAcceptedEvent(app *model.Application, epoch *model.Epoch) *iconsensus.IConsensusClaimAccepted {
	return &iconsensus.IConsensusClaimAccepted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(epoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *epoch.OutputsMerkleRoot,
		Raw: types.Log{
			TxHash:      common.HexToHash(epoch.ClaimTransactionHash.Hex()),
			BlockNumber: epoch.LastBlock + 5,
		},
	}
}

// rpcDataError simulates an RPC error with revert data, as returned by
// eth_estimateGas when the contract reverts.
type rpcDataError struct {
	code int
	msg  string
	data any
}

func (e *rpcDataError) Error() string  { return e.msg }
func (e *rpcDataError) ErrorCode() int { return e.code }
func (e *rpcDataError) ErrorData() any { return e.data }

// notFirstClaimError creates an error that mimics a NotFirstClaim revert
// from eth_estimateGas, with the ABI error selector as revert data.
func notFirstClaimError() error {
	parsed, _ := iconsensus.IConsensusMetaData.GetAbi()
	id := parsed.Errors["NotFirstClaim"].ID
	selector := fmt.Sprintf("0x%x", id[:4])
	return &rpcDataError{
		code: 3,
		msg:  "execution reverted",
		data: selector + "000000000000000000000000" +
			"01000000000000000000000000000000000000000000000000000000000000" +
			"0000000000000000000000000000000000000000000000000000000000000027",
	}
}

// //////////////////////////////////////////////////////////////////////////////
// Success
// //////////////////////////////////////////////////////////////////////////////
func TestDoNothing(t *testing.T) {
	m, r, _ := newServiceMock()
	defer r.AssertExpectations(t)

	prevEpochs := makeEpochMap()
	currEpochs := makeEpochMap()

	transitions, errs := m.submitClaimsAndUpdateDatabase(context.Background(), prevEpochs, currEpochs, makeApplicationMap(), big.NewInt(0))
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, transitions, "no transitions when no epochs to process")
}

func TestSubmitFirstClaim(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	b.On("submitClaimToBlockchain", mock.Anything, app, currEpoch).
		Return(common.HexToHash("0x10"), nil).Once()

	transitions, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 1, len(m.claimsInFlight))
	assert.Equal(t, 1, transitions, "submitting a claim counts as a transition")
}

func TestSubmitClaimWithAntecessor(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil
	prevEvent := makeSubmittedEvent(app, prevEpoch)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	b.On("submitClaimToBlockchain", mock.Anything, app, currEpoch).
		Return(common.HexToHash("0x10"), nil).Once()

	transitions, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 1, len(m.claimsInFlight))
	assert.Equal(t, 1, transitions, "submitting a claim counts as a transition")
}

func TestSkipSubmitFirstClaim(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	m.submissionEnabled = false
	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()

	transitions, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 0, transitions, "no transition when submission is disabled")
}

func TestSkipSubmitClaimWithAntecessor(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	m.submissionEnabled = false
	endBlock := big.NewInt(40)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)
	prevEvent := makeSubmittedEvent(app, prevEpoch)
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, len(errs), 0)
	assert.Equal(t, len(m.claimsInFlight), 0)
}

func TestInFlightCompleted(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication() // default: Authority consensus
	currEpoch := makeComputedEpoch(app, 3)
	currEpoch.ClaimTransactionHash = &txHash

	m.claimsInFlight[app.ID] = *currEpoch.ClaimTransactionHash

	// Authority emits ClaimAccepted in the same tx. Include a matching
	// log in the receipt so the fast-accept path fires.
	acceptedLog := makeClaimAcceptedLog(app, currEpoch)
	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(true, &types.Receipt{
			ContractAddress: app.IApplicationAddress,
			TxHash:          txHash,
			BlockNumber:     new(big.Int).SetUint64(currEpoch.LastBlock + 1),
			Status:          1,
			Logs:            []*types.Log{&acceptedLog},
		}, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, txHash).
		Return(nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()

	transitions, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, len(m.claimsInFlight))
	// Authority fast path: submitted (1) + accepted (1) = 2 transitions.
	assert.Equal(t, 2, transitions)
}

func TestInFlightReverted(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	currEpoch.ClaimTransactionHash = &txHash

	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	m.claimsInFlight[app.ID] = *currEpoch.ClaimTransactionHash

	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(true, &types.Receipt{
			ContractAddress: app.IApplicationAddress,
			TxHash:          txHash,
			BlockNumber:     new(big.Int).SetUint64(currEpoch.LastBlock + 1),
			Status:          0,
		}, nil).Once()
	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	b.On("submitClaimToBlockchain", mock.Anything, app, currEpoch).
		Return(common.HexToHash("0x10"), nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, len(errs), 0)
	assert.Equal(t, len(m.claimsInFlight), 1)
}

func TestUpdateFirstClaim(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	currEvent := makeSubmittedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, prevEvent, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, currEvent.Raw.TxHash).
		Return(nil).Once()

	transitions, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 1, transitions, "finding on-chain event counts as a transition")
}

func TestUpdateClaimWithAntecessor(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)
	prevEvent := makeSubmittedEvent(app, prevEpoch)
	currEvent := makeSubmittedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, currEvent.Raw.TxHash).
		Return(nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, len(errs), 0)
	assert.Equal(t, len(m.claimsInFlight), 0)
}

func TestAcceptFirstClaim(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeSubmittedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimAccepted = nil
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()

	_, errs := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, len(errs), 0)
}

func TestAcceptClaimWithAntecessor(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeSubmittedEpoch(app, 3)
	prevEvent := makeAcceptedEvent(app, prevEpoch)
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()

	transitions, errs := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 1, transitions, "accepting a claim counts as a transition")
}

// //////////////////////////////////////////////////////////////////////////////
// Failure
// //////////////////////////////////////////////////////////////////////////////

func TestClaimInFlightMissingFromCurrClaims(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	reqHash := common.HexToHash("0x01")
	receipt := new(types.Receipt)

	app := makeApplication()
	m.claimsInFlight[app.ID] = reqHash

	b.On("pollTransaction", mock.Anything, reqHash, endBlock).
		Return(true, receipt, nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(), makeApplicationMap(app), endBlock)
	assert.Equal(t, len(errs), 0)
}

// submit again after pollTransaction failure
func TestSubmitFailedClaim(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	expectedErr := fmt.Errorf("not found")
	endBlock := big.NewInt(100)
	reqHash := common.HexToHash("0x01")
	var nilReceipt *types.Receipt

	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)
	prevEvent := makeSubmittedEvent(app, prevEpoch)
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	m.claimsInFlight[app.ID] = reqHash

	b.On("pollTransaction", mock.Anything, reqHash, endBlock).
		Return(false, nilReceipt, expectedErr).Once()
	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	b.On("submitClaimToBlockchain", mock.Anything, app, currEpoch).
		Return(common.HexToHash("0x10"), nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
}

// TestNotFirstClaimHandledGracefully verifies that when submitClaim reverts
// with NotFirstClaim (e.g., after a node restart where claimsInFlight was
// lost), the claimer handles it gracefully — no error, no claimsInFlight
// entry, and the claim is left for event sync to pick up.
func TestNotFirstClaimHandledGracefully(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimSubmitted
	var currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	// submitClaim reverts with NotFirstClaim (caught by eth_estimateGas).
	b.On("submitClaimToBlockchain", mock.Anything, app, currEpoch).
		Return(common.Hash{}, notFirstClaimError()).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, len(m.claimsInFlight))
}

// TestNotFirstClaimQuorumSetsInoperable verifies that when submitClaim reverts
// with NotFirstClaim for a Quorum app, the claimer marks the application as
// inoperable. In Quorum, NotFirstClaim means the validator previously submitted
// a different merkle root — a determinism violation.
func TestNotFirstClaimQuorumSetsInoperable(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimSubmitted
	var currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	b.On("submitClaimToBlockchain", mock.Anything, app, currEpoch).
		Return(common.Hash{}, notFirstClaimError()).Once()
	r.On("UpdateApplicationState", mock.Anything, int64(0), model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs))
	assert.Equal(t, 0, len(m.claimsInFlight))
}

// !claimSubmittedMatche(prevClaim, prevEvent)
func TestSubmitClaimWithAntecessorMismatch(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)

	// event has an incorrect LastProcessedBlockNumber field.
	prevEvent := &iconsensus.IConsensusClaimSubmitted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevEpoch.LastBlock + 1),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *prevEpoch.OutputsMerkleRoot,
	}
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).
		Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).
		Once()
	r.On("UpdateApplicationState", mock.Anything, int64(0), model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs))
}

// !claimMatchesEvent(currClaim, currEvent)
func TestSubmitClaimWithEventMismatch(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)
	prevEvent := makeSubmittedEvent(app, prevEpoch)
	wrongEvent := makeSubmittedEvent(app, makeComputedEpoch(app, 2))

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, wrongEvent, nil)
	r.On("UpdateApplicationState", mock.Anything, int64(0), model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs))
}

// !checkClaimsConstraint(prevClaim, currClaim) // epoch pair has its blocks out of order
func TestSubmitClaimWithAntecessorOutOfOrder(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	app := makeApplication()
	prevEpoch := makeSubmittedEpoch(app, 2)
	currEpoch := makeComputedEpoch(app, 1)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	r.On("UpdateApplicationState", mock.Anything, int64(0), model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), big.NewInt(0))
	assert.Equal(t, 1, len(errs))
}

func TestErrSubmittedMissingEvent(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeComputedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 2)
	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	currEvent := makeSubmittedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	r.On("UpdateApplicationState", mock.Anything, int64(0), model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs))
}

func TestConsensusAddressChangedOnSubmittedClaims(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	wrongConsensusAddress := app.IConsensusAddress
	wrongConsensusAddress[0]++

	b.On("getConsensusAddress", mock.Anything, app).
		Return(wrongConsensusAddress, nil).
		Once()
	r.On("UpdateApplicationState", mock.Anything, int64(0), model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, len(errs), 1)
}

////////////////////////////////////////////////////////////////////////////////

func TestFindClaimAcceptedEventAndSuccFailure0(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	expectedErr := fmt.Errorf("not found")
	endBlock := big.NewInt(100)

	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 2)
	var prevEvent *iconsensus.IConsensusClaimAccepted = nil
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, expectedErr).Once()

	_, errs := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs))
}

func TestFindClaimAcceptedEventAndSuccFailure1(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	expectedErr := fmt.Errorf("not found")
	endBlock := big.NewInt(100)

	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 2)
	prevEvent := makeAcceptedEvent(app, prevEpoch)
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, expectedErr).Once()

	_, errs := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs))
}

// !claimAcceptedMatch(prevClaim, prevEvent)
func TestAcceptClaimWithAntecessorMismatch(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)

	prevEvent := &iconsensus.IConsensusClaimAccepted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevEpoch.LastBlock + 1),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *prevEpoch.OutputsMerkleRoot,
	}
	var currEvent *iconsensus.IConsensusClaimAccepted = nil

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil)
	r.On("UpdateApplicationState", mock.Anything, mock.Anything, model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	_, errs := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs))
}

// !claimAcceptedMatch(currClaim, currEvent)
func TestAcceptClaimWithEventMismatch(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	wrongEpoch := makeComputedEpoch(app, 2)
	currEpoch := makeComputedEpoch(app, 3)
	wrongEvent := makeAcceptedEvent(app, wrongEpoch)
	prevEvent := makeAcceptedEvent(app, prevEpoch)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, wrongEvent, nil)
	r.On("UpdateApplicationState", mock.Anything, mock.Anything, model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	_, errs := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs))
}

// !checkClaimsConstraint(prevClaim, currClaim)
func TestAcceptClaimWithAntecessorOutOfOrder(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	app := makeApplication()
	wrongEpoch := makeComputedEpoch(app, 2)
	currEpoch := makeComputedEpoch(app, 1)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	r.On("UpdateApplicationState", mock.Anything, mock.Anything, model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).
		Once()

	_, errs := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(wrongEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), big.NewInt(0))
	assert.Equal(t, 1, len(errs))
}

func TestErrAcceptedMissingEvent(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeComputedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 2)
	var prevEvent *iconsensus.IConsensusClaimAccepted = nil
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	r.On("UpdateApplicationState", mock.Anything, mock.Anything, model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	_, errs := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs))
}

func TestUpdateEpochWithAcceptedClaimFailed(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	expectedErr := fmt.Errorf("not found")

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeSubmittedEpoch(app, 1)
	currEpoch := makeSubmittedEpoch(app, 2)
	prevEvent := makeAcceptedEvent(app, prevEpoch)
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(expectedErr).Once()

	_, errs := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs))
}

func TestConsensusAddressChangedOnAcceptedClaims(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	wrongConsensusAddress := app.IConsensusAddress
	wrongConsensusAddress[0]++

	b.On("getConsensusAddress", mock.Anything, app).
		Return(wrongConsensusAddress, nil).
		Once()
	r.On("UpdateApplicationState", mock.Anything, int64(0), model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).
		Once()

	_, errs := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, len(errs), 1)
}
