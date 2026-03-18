// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/lmittmann/tint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

// ////////////////////////////////////////////////////////////////////////////
// Mocks
// ////////////////////////////////////////////////////////////////////////////

// repositoryMock implements iRepository via testify/mock.
type repositoryMock struct {
	mock.Mock
}

// ApplicationRepository methods

func (m *repositoryMock) CreateApplication(ctx context.Context, app *model.Application, withExecutionParameters bool) (int64, error) {
	args := m.Called(ctx, app, withExecutionParameters)
	return args.Get(0).(int64), args.Error(1)
}

func (m *repositoryMock) GetApplication(ctx context.Context, nameOrAddress string) (*model.Application, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(*model.Application), args.Error(1)
}

func (m *repositoryMock) GetProcessedInputCount(ctx context.Context, nameOrAddress string) (uint64, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *repositoryMock) UpdateApplication(ctx context.Context, app *model.Application) error {
	args := m.Called(ctx, app)
	return args.Error(0)
}

func (m *repositoryMock) UpdateApplicationState(ctx context.Context, appID int64, state model.ApplicationState, reason *string) error {
	args := m.Called(ctx, appID, state, reason)
	return args.Error(0)
}

func (m *repositoryMock) DeleteApplication(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *repositoryMock) ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool) ([]*model.Application, uint64, error) {
	args := m.Called(ctx, f, p, descending)
	return args.Get(0).([]*model.Application), args.Get(1).(uint64), args.Error(2)
}

func (m *repositoryMock) GetExecutionParameters(ctx context.Context, applicationID int64) (*model.ExecutionParameters, error) {
	args := m.Called(ctx, applicationID)
	return args.Get(0).(*model.ExecutionParameters), args.Error(1)
}

func (m *repositoryMock) UpdateExecutionParameters(ctx context.Context, ep *model.ExecutionParameters) error {
	args := m.Called(ctx, ep)
	return args.Error(0)
}

func (m *repositoryMock) GetEventLastCheckBlock(ctx context.Context, appID int64, event model.MonitoredEvent) (uint64, error) {
	args := m.Called(ctx, appID, event)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *repositoryMock) UpdateEventLastCheckBlock(ctx context.Context, appIDs []int64, event model.MonitoredEvent, blockNumber uint64) error {
	args := m.Called(ctx, appIDs, event, blockNumber)
	return args.Error(0)
}

func (m *repositoryMock) GetLastSnapshot(ctx context.Context, nameOrAddress string) (*model.Input, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(*model.Input), args.Error(1)
}

// EpochRepository methods

func (m *repositoryMock) CreateEpochsAndInputs(ctx context.Context, nameOrAddress string, epochInputMap map[*model.Epoch][]*model.Input, blockNumber uint64) error {
	args := m.Called(ctx, nameOrAddress, epochInputMap, blockNumber)
	return args.Error(0)
}

func (m *repositoryMock) GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*model.Epoch, error) {
	args := m.Called(ctx, nameOrAddress, index)
	return args.Get(0).(*model.Epoch), args.Error(1)
}

func (m *repositoryMock) GetLastAcceptedEpochIndex(ctx context.Context, nameOrAddress string) (uint64, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *repositoryMock) GetLastNonOpenEpoch(ctx context.Context, nameOrAddress string) (*model.Epoch, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(*model.Epoch), args.Error(1)
}

func (m *repositoryMock) GetEpochByVirtualIndex(ctx context.Context, nameOrAddress string, index uint64) (*model.Epoch, error) {
	args := m.Called(ctx, nameOrAddress, index)
	return args.Get(0).(*model.Epoch), args.Error(1)
}

func (m *repositoryMock) UpdateEpochClaimTransactionHash(ctx context.Context, nameOrAddress string, e *model.Epoch) error {
	args := m.Called(ctx, nameOrAddress, e)
	return args.Error(0)
}

func (m *repositoryMock) UpdateEpochClaimSubmittedTransactionHash(ctx context.Context, nameOrAddress string, e *model.Epoch) error {
	args := m.Called(ctx, nameOrAddress, e)
	return args.Error(0)
}

func (m *repositoryMock) UpdateEpochClaimAcceptedTransactionHash(ctx context.Context, nameOrAddress string, e *model.Epoch) error {
	args := m.Called(ctx, nameOrAddress, e)
	return args.Error(0)
}

func (m *repositoryMock) UpdateEpochStatus(ctx context.Context, nameOrAddress string, e *model.Epoch) error {
	args := m.Called(ctx, nameOrAddress, e)
	return args.Error(0)
}

func (m *repositoryMock) UpdateEpochInputsProcessed(ctx context.Context, nameOrAddress string, epochIndex uint64) error {
	args := m.Called(ctx, nameOrAddress, epochIndex)
	return args.Error(0)
}

func (m *repositoryMock) UpdateEpochOutputsProof(ctx context.Context, appID int64, epochIndex uint64, proof *model.OutputsProof) error {
	args := m.Called(ctx, appID, epochIndex, proof)
	return args.Error(0)
}

func (m *repositoryMock) RepeatPreviousEpochOutputsProof(ctx context.Context, appID int64, epochIndex uint64) error {
	args := m.Called(ctx, appID, epochIndex)
	return args.Error(0)
}

func (m *repositoryMock) ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter, p repository.Pagination, descending bool) ([]*model.Epoch, uint64, error) {
	args := m.Called(ctx, nameOrAddress, f, p, descending)
	return args.Get(0).([]*model.Epoch), args.Get(1).(uint64), args.Error(2)
}

// NodeConfigRepository methods

func (m *repositoryMock) SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error {
	args := m.Called(ctx, key, rawJSON)
	return args.Error(0)
}

func (m *repositoryMock) LoadNodeConfigRaw(ctx context.Context, key string) ([]byte, time.Time, time.Time, error) {
	args := m.Called(ctx, key)
	return args.Get(0).([]byte), args.Get(1).(time.Time), args.Get(2).(time.Time), args.Error(3)
}

// blockchainMock implements iBlockchain via testify/mock.
// Only HeaderByNumber and TransactionReceipt are exercised by claimer logic;
// the ContractBackend methods are present to satisfy the interface but panic
// on unexpected calls.
type blockchainMock struct {
	mock.Mock
}

func (m *blockchainMock) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	args := m.Called(ctx, number)
	return args.Get(0).(*types.Header), args.Error(1)
}

func (m *blockchainMock) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	args := m.Called(ctx, txHash)
	return args.Get(0).(*types.Receipt), args.Error(1)
}

func (m *blockchainMock) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	panic("unexpected call to CodeAt")
}
func (m *blockchainMock) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	panic("unexpected call to CallContract")
}
func (m *blockchainMock) PendingCodeAt(ctx context.Context, contract common.Address) ([]byte, error) {
	panic("unexpected call to PendingCodeAt")
}
func (m *blockchainMock) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	panic("unexpected call to PendingNonceAt")
}
func (m *blockchainMock) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	panic("unexpected call to SuggestGasPrice")
}
func (m *blockchainMock) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	panic("unexpected call to SuggestGasTipCap")
}
func (m *blockchainMock) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	panic("unexpected call to EstimateGas")
}
func (m *blockchainMock) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	panic("unexpected call to SendTransaction")
}
func (m *blockchainMock) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	panic("unexpected call to FilterLogs")
}
func (m *blockchainMock) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	panic("unexpected call to SubscribeFilterLogs")
}

// consensusMock implements iConsensus via testify/mock.
type consensusMock struct {
	mock.Mock
}

func (m *consensusMock) GetNumberOfSubmittedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	args := m.Called(opts, appContract)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *consensusMock) GetNumberOfAcceptedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	args := m.Called(opts, appContract)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *consensusMock) FilterClaimSubmitted(opts *bind.FilterOpts, submitter []common.Address, appContract []common.Address) ([]*iconsensus.IConsensusClaimSubmitted, error) {
	args := m.Called(opts, submitter, appContract)
	return args.Get(0).([]*iconsensus.IConsensusClaimSubmitted), args.Error(1)
}

func (m *consensusMock) FilterClaimAccepted(opts *bind.FilterOpts, appContract []common.Address) ([]*iconsensus.IConsensusClaimAccepted, error) {
	args := m.Called(opts, appContract)
	return args.Get(0).([]*iconsensus.IConsensusClaimAccepted), args.Error(1)
}

func (m *consensusMock) SubmitClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	args := m.Called(opts, appContract, lastProcessedBlockNumber, outputsMerkleRoot)
	return args.Get(0).(*types.Transaction), args.Error(1)
}

// ////////////////////////////////////////////////////////////////////////////
// Helpers
// ////////////////////////////////////////////////////////////////////////////

func newLogger() *slog.Logger {
	opts := &tint.Options{
		Level:      slog.LevelDebug,
		AddSource:  true,
		TimeFormat: "2006-01-02T15:04:05.000",
	}
	return slog.New(tint.NewHandler(os.Stdout, opts))
}

var (
	merkleRoot = common.HexToHash("0xDEAD")
	txHashA    = common.HexToHash("0xAAAA")
	txHashB    = common.HexToHash("0xBBBB")
)

func makeApp() *model.Application {
	return repotest.NewApplicationBuilder().Build()
}

func makeDaveApp() *model.Application {
	return repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Build()
}

// makeComputedEpoch returns a ClaimComputed epoch with blocks [index*10, index*10+9]
// and VirtualIndex == Index (the common case).
func makeComputedEpoch(appID int64, index uint64) *model.Epoch {
	e := repotest.NewEpochBuilder(appID).
		WithIndex(index).
		WithStatus(model.EpochStatus_ClaimComputed).
		WithBlocks(index*10, index*10+9).
		WithClaimHash(merkleRoot).
		Build()
	e.VirtualIndex = index
	return e
}

// makeSubmittedEpoch returns a ClaimSubmitted epoch with blocks [index*10, index*10+9]
// and VirtualIndex == Index.
func makeSubmittedEpoch(appID int64, index uint64) *model.Epoch {
	e := repotest.NewEpochBuilder(appID).
		WithIndex(index).
		WithStatus(model.EpochStatus_ClaimSubmitted).
		WithBlocks(index*10, index*10+9).
		WithClaimHash(merkleRoot).
		Build()
	e.VirtualIndex = index
	return e
}

func makeSubmittedEvent(app *model.Application, epoch *model.Epoch, txHash common.Hash) *iconsensus.IConsensusClaimSubmitted {
	return &iconsensus.IConsensusClaimSubmitted{
		AppContract:              app.IApplicationAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(epoch.LastBlock),
		OutputsMerkleRoot:        *epoch.OutputsMerkleRoot,
		Raw: types.Log{
			TxHash:      txHash,
			BlockNumber: epoch.LastBlock + 5,
		},
	}
}

func makeAcceptedEvent(app *model.Application, epoch *model.Epoch, txHash common.Hash) *iconsensus.IConsensusClaimAccepted {
	return &iconsensus.IConsensusClaimAccepted{
		AppContract:              app.IApplicationAddress,
		LastProcessedBlockNumber: new(big.Int).SetUint64(epoch.LastBlock),
		OutputsMerkleRoot:        *epoch.OutputsMerkleRoot,
		Raw: types.Log{
			TxHash:      txHash,
			BlockNumber: epoch.LastBlock + 5,
		},
	}
}

// makeTx creates a minimal signed transaction whose Hash() is deterministic
// for a given nonce value.
func makeTx(nonce uint64) *types.Transaction {
	return types.NewTx(&types.LegacyTx{Nonce: nonce})
}

// epochFilter returns the EpochFilter the claimer passes to ListEpochs.
func epochFilter() repository.EpochFilter {
	return repository.EpochFilter{
		Status: []model.EpochStatus{
			model.EpochStatus_ClaimComputed,
			model.EpochStatus_ClaimSubmitted,
		},
	}
}

// appFilter returns the ApplicationFilter the claimer passes to ListApplications.
func appFilter() repository.ApplicationFilter {
	return repository.ApplicationFilter{
		State: model.Pointer(model.ApplicationState_Enabled),
	}
}

// ////////////////////////////////////////////////////////////////////////////
// checkClaimSubmittedEvent
// ////////////////////////////////////////////////////////////////////////////

func TestCheckSubmitted_Valid(t *testing.T) {
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	ev := makeSubmittedEvent(app, epoch, txHashA)
	assert.NoError(t, checkClaimSubmittedEvent(app, epoch, ev))
}

func TestCheckSubmitted_AddressMismatch(t *testing.T) {
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	ev := makeSubmittedEvent(app, epoch, txHashA)
	ev.AppContract = common.HexToAddress("0xDEAD")
	assert.Error(t, checkClaimSubmittedEvent(app, epoch, ev))
}

func TestCheckSubmitted_MerkleRootMismatch(t *testing.T) {
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	ev := makeSubmittedEvent(app, epoch, txHashA)
	ev.OutputsMerkleRoot = common.HexToHash("0xBAD")
	assert.Error(t, checkClaimSubmittedEvent(app, epoch, ev))
}

func TestCheckSubmitted_BlockMismatch(t *testing.T) {
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	ev := makeSubmittedEvent(app, epoch, txHashA)
	ev.LastProcessedBlockNumber = new(big.Int).SetUint64(epoch.LastBlock + 1)
	assert.Error(t, checkClaimSubmittedEvent(app, epoch, ev))
}

// ////////////////////////////////////////////////////////////////////////////
// checkClaimAcceptedEvent
// ////////////////////////////////////////////////////////////////////////////

func TestCheckAccepted_Valid(t *testing.T) {
	app := makeApp()
	epoch := makeSubmittedEpoch(app.ID, 0)
	ev := makeAcceptedEvent(app, epoch, txHashA)
	assert.NoError(t, checkClaimAcceptedEvent(app, epoch, ev))
}

func TestCheckAccepted_AddressMismatch(t *testing.T) {
	app := makeApp()
	epoch := makeSubmittedEpoch(app.ID, 0)
	ev := makeAcceptedEvent(app, epoch, txHashA)
	ev.AppContract = common.HexToAddress("0xDEAD")
	assert.Error(t, checkClaimAcceptedEvent(app, epoch, ev))
}

func TestCheckAccepted_MerkleRootMismatch(t *testing.T) {
	app := makeApp()
	epoch := makeSubmittedEpoch(app.ID, 0)
	ev := makeAcceptedEvent(app, epoch, txHashA)
	ev.OutputsMerkleRoot = common.HexToHash("0xBAD")
	assert.Error(t, checkClaimAcceptedEvent(app, epoch, ev))
}

func TestCheckAccepted_BlockMismatch(t *testing.T) {
	app := makeApp()
	epoch := makeSubmittedEpoch(app.ID, 0)
	ev := makeAcceptedEvent(app, epoch, txHashA)
	ev.LastProcessedBlockNumber = new(big.Int).SetUint64(epoch.LastBlock + 1)
	assert.Error(t, checkClaimAcceptedEvent(app, epoch, ev))
}

// ////////////////////////////////////////////////////////////////////////////
// getDefaultBlockNumber
// ////////////////////////////////////////////////////////////////////////////

func TestGetDefaultBlock_Finalized(t *testing.T) {
	b := &blockchainMock{}
	defer b.AssertExpectations(t)
	b.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(42)}, nil).Once()

	nr, err := getDefaultBlockNumber(context.Background(), b, model.DefaultBlock_Finalized)
	assert.NoError(t, err)
	assert.Equal(t, uint64(42), nr)
}

func TestGetDefaultBlock_Latest(t *testing.T) {
	b := &blockchainMock{}
	defer b.AssertExpectations(t)
	b.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.LatestBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Once()

	nr, err := getDefaultBlockNumber(context.Background(), b, model.DefaultBlock_Latest)
	assert.NoError(t, err)
	assert.Equal(t, uint64(100), nr)
}

func TestGetDefaultBlock_Pending(t *testing.T) {
	b := &blockchainMock{}
	defer b.AssertExpectations(t)
	b.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.PendingBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(101)}, nil).Once()

	nr, err := getDefaultBlockNumber(context.Background(), b, model.DefaultBlock_Pending)
	assert.NoError(t, err)
	assert.Equal(t, uint64(101), nr)
}

func TestGetDefaultBlock_Safe(t *testing.T) {
	b := &blockchainMock{}
	defer b.AssertExpectations(t)
	b.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.SafeBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(99)}, nil).Once()

	nr, err := getDefaultBlockNumber(context.Background(), b, model.DefaultBlock_Safe)
	assert.NoError(t, err)
	assert.Equal(t, uint64(99), nr)
}

func TestGetDefaultBlock_Invalid(t *testing.T) {
	b := &blockchainMock{}
	_, err := getDefaultBlockNumber(context.Background(), b, model.DefaultBlock("INVALID"))
	assert.Error(t, err)
}

// ////////////////////////////////////////////////////////////////////////////
// trySubmitClaim
// ////////////////////////////////////////////////////////////////////////////

// No previous in-flight tx, submission enabled → SubmitClaim called, hash persisted.
func TestTrySubmit_NilTxHash_Submits(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	// epoch.ClaimSubmittedTransactionHash is nil

	tx := makeTx(1)
	txOpts := &bind.TransactOpts{}

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	b := &blockchainMock{}
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	ic.On("SubmitClaim", txOpts, app.IApplicationAddress,
		new(big.Int).SetUint64(epoch.LastBlock), [32]byte(*epoch.OutputsMerkleRoot)).
		Return(tx, nil).Once()
	r.On("UpdateEpochClaimSubmittedTransactionHash", ctx, app.Name, epoch).
		Return(nil).Once()

	err := trySubmitClaim(ctx, logger, app, epoch, r, ic, b, txOpts, 100)
	assert.NoError(t, err)
	assert.Equal(t, tx.Hash(), *epoch.ClaimSubmittedTransactionHash)
	r.AssertExpectations(t)
}

// No previous in-flight tx, submission disabled (txOpts == nil) → silent no-op.
func TestTrySubmit_NilTxHash_Disabled(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)

	r := &repositoryMock{}
	b := &blockchainMock{}
	ic := &consensusMock{}

	err := trySubmitClaim(ctx, logger, app, epoch, r, ic, b, nil, 100)
	assert.NoError(t, err)
	assert.Nil(t, epoch.ClaimSubmittedTransactionHash)
	ic.AssertNotCalled(t, "SubmitClaim")
}

// In-flight tx found, receipt block > endBlock → wait, return nil.
func TestTrySubmit_InFlight_TooEarly(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	epoch.ClaimSubmittedTransactionHash = &txHashA
	endBlock := uint64(50)

	b := &blockchainMock{}
	defer b.AssertExpectations(t)
	r := &repositoryMock{}
	ic := &consensusMock{}

	b.On("TransactionReceipt", ctx, txHashA).
		Return(&types.Receipt{
			Status:      1,
			BlockNumber: big.NewInt(int64(endBlock + 1)), // beyond endBlock
		}, nil).Once()

	err := trySubmitClaim(ctx, logger, app, epoch, r, ic, b, &bind.TransactOpts{}, endBlock)
	assert.NoError(t, err)
	r.AssertNotCalled(t, "UpdateEpochStatus")
}

// In-flight tx confirmed (Status=1, block <= endBlock) → UpdateEpochStatus called.
func TestTrySubmit_InFlight_Confirmed(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	epoch.ClaimSubmittedTransactionHash = &txHashA
	endBlock := uint64(100)

	b := &blockchainMock{}
	defer b.AssertExpectations(t)
	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}

	b.On("TransactionReceipt", ctx, txHashA).
		Return(&types.Receipt{
			Status:      1,
			TxHash:      txHashA,
			BlockNumber: big.NewInt(50),
		}, nil).Once()
	r.On("UpdateEpochStatus", ctx, app.Name, epoch).
		Return(nil).Once()

	err := trySubmitClaim(ctx, logger, app, epoch, r, ic, b, &bind.TransactOpts{}, endBlock)
	assert.NoError(t, err)
	assert.Equal(t, model.EpochStatus_ClaimSubmitted, epoch.Status)
	ic.AssertNotCalled(t, "SubmitClaim")
}

// In-flight tx reverted (Status=0) → falls through to submit label → resubmits.
func TestTrySubmit_InFlight_Reverted_Resubmits(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	epoch.ClaimSubmittedTransactionHash = &txHashA
	endBlock := uint64(100)
	txOpts := &bind.TransactOpts{}
	newTx := makeTx(2)

	b := &blockchainMock{}
	defer b.AssertExpectations(t)
	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	b.On("TransactionReceipt", ctx, txHashA).
		Return(&types.Receipt{
			Status:      0,
			BlockNumber: big.NewInt(50),
		}, nil).Once()
	ic.On("SubmitClaim", txOpts, app.IApplicationAddress,
		new(big.Int).SetUint64(epoch.LastBlock), [32]byte(*epoch.OutputsMerkleRoot)).
		Return(newTx, nil).Once()
	r.On("UpdateEpochClaimSubmittedTransactionHash", ctx, app.Name, epoch).
		Return(nil).Once()

	err := trySubmitClaim(ctx, logger, app, epoch, r, ic, b, txOpts, endBlock)
	assert.NoError(t, err)
	assert.Equal(t, newTx.Hash(), *epoch.ClaimSubmittedTransactionHash)
}

// In-flight tx not found (ethereum.NotFound) → falls through to submit label → resubmits.
func TestTrySubmit_InFlight_NotFound_Resubmits(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	epoch.ClaimSubmittedTransactionHash = &txHashA
	endBlock := uint64(100)
	txOpts := &bind.TransactOpts{}
	newTx := makeTx(3)

	b := &blockchainMock{}
	defer b.AssertExpectations(t)
	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	b.On("TransactionReceipt", ctx, txHashA).
		Return((*types.Receipt)(nil), ethereum.NotFound).Once()
	ic.On("SubmitClaim", txOpts, app.IApplicationAddress,
		new(big.Int).SetUint64(epoch.LastBlock), [32]byte(*epoch.OutputsMerkleRoot)).
		Return(newTx, nil).Once()
	r.On("UpdateEpochClaimSubmittedTransactionHash", ctx, app.Name, epoch).
		Return(nil).Once()

	err := trySubmitClaim(ctx, logger, app, epoch, r, ic, b, txOpts, endBlock)
	assert.NoError(t, err)
	assert.Equal(t, newTx.Hash(), *epoch.ClaimSubmittedTransactionHash)
}

// ////////////////////////////////////////////////////////////////////////////
// updateClaimSubmitted
// ////////////////////////////////////////////////////////////////////////////

// collectClaimSubmittedEvents calls GetNumberOfSubmittedClaims twice (startBlock-1
// and startBlock as part of FindTransitions). When the count does not change,
// no FilterClaimSubmitted is called and only UpdateEventLastCheckBlock is persisted.
func TestUpdateSubmitted_NoEvents(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	epochs := []*model.Epoch{epoch}
	endBlock := uint64(100)
	// startBlock = max(0, epoch.LastBlock) + 1 = epoch.LastBlock + 1 = 10
	// FindTransitions calls oracle at startBlock-1, startBlock, and endBlock (3 total)

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	count := big.NewInt(0)
	// oracle called at startBlock-1, startBlock, and endBlock (FindTransitions)
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(count, nil).Times(3)
	r.On("UpdateEventLastCheckBlock", ctx, []int64{app.ID}, model.MonitoredEvent_ClaimSubmitted, endBlock).
		Return(nil).Once()

	err := updateClaimSubmitted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.NoError(t, err)
	// epoch status should be unchanged
	assert.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
}

// One submitted event matches the first epoch (VirtualIndex=0, base=0).
// Epoch status advances from CLAIM_COMPUTED to CLAIM_SUBMITTED.
func TestUpdateSubmitted_FirstClaim(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	// VirtualIndex == 0
	epochs := []*model.Epoch{epoch}
	endBlock := uint64(100)
	ev := makeSubmittedEvent(app, epoch, txHashA)

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	// At startBlock-1: count=0 (base), at startBlock: count=1 (transition found),
	// at endBlock: count=1 (no further transitions)
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(0), nil).Once() // prevValue at startBlock-1
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once() // startBlock value in FindTransitions
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once() // endBlock value — startValue==endValue, no binary search
	ic.On("FilterClaimSubmitted", mock.Anything, ([]common.Address)(nil),
		[]common.Address{app.IApplicationAddress}).
		Return([]*iconsensus.IConsensusClaimSubmitted{ev}, nil).Once()

	r.On("UpdateEpochClaimSubmittedTransactionHash", ctx, app.Name, epoch).
		Return(nil).Once()
	r.On("UpdateEpochStatus", ctx, app.Name, epoch).
		Return(nil).Once()
	r.On("UpdateEventLastCheckBlock", ctx, []int64{app.ID}, model.MonitoredEvent_ClaimSubmitted, endBlock).
		Return(nil).Once()

	err := updateClaimSubmitted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.NoError(t, err)
	assert.Equal(t, model.EpochStatus_ClaimSubmitted, epoch.Status)
	assert.Equal(t, txHashA, *epoch.ClaimSubmittedTransactionHash)
}

// One submitted event matches second epoch (VirtualIndex=1, base=1).
func TestUpdateSubmitted_WithAntecessor(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 1)
	epoch.VirtualIndex = 1
	epochs := []*model.Epoch{epoch}
	endBlock := uint64(100)
	ev := makeSubmittedEvent(app, epoch, txHashA)

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	// prevValue = 1 (already 1 claim submitted before our range)
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once() // startBlock-1
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(2), nil).Once() // startBlock in FindTransitions → transition found
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(2), nil).Once() // endBlock — startValue==endValue, no binary search
	ic.On("FilterClaimSubmitted", mock.Anything, ([]common.Address)(nil),
		[]common.Address{app.IApplicationAddress}).
		Return([]*iconsensus.IConsensusClaimSubmitted{ev}, nil).Once()

	r.On("UpdateEpochClaimSubmittedTransactionHash", ctx, app.Name, epoch).
		Return(nil).Once()
	r.On("UpdateEpochStatus", ctx, app.Name, epoch).
		Return(nil).Once()
	r.On("UpdateEventLastCheckBlock", ctx, []int64{app.ID}, model.MonitoredEvent_ClaimSubmitted, endBlock).
		Return(nil).Once()

	err := updateClaimSubmitted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.NoError(t, err)
	assert.Equal(t, model.EpochStatus_ClaimSubmitted, epoch.Status)
}

// Event sequence mismatch: epoch.VirtualIndex != base+i → application marked inoperable.
func TestUpdateSubmitted_VirtualIndexMismatch(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	epoch.VirtualIndex = 5 // mismatched: base=0, expected=0, got=5
	epochs := []*model.Epoch{epoch}
	endBlock := uint64(100)
	ev := makeSubmittedEvent(app, epoch, txHashA)

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(0), nil).Once()
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once()
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once()
	ic.On("FilterClaimSubmitted", mock.Anything, ([]common.Address)(nil),
		[]common.Address{app.IApplicationAddress}).
		Return([]*iconsensus.IConsensusClaimSubmitted{ev}, nil).Once()

	r.On("UpdateApplicationState", ctx, app.ID, model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	err := updateClaimSubmitted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.Error(t, err)
}

// Event field mismatch (wrong OutputsMerkleRoot) → application marked inoperable.
func TestUpdateSubmitted_EventMismatch(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	epochs := []*model.Epoch{epoch}
	endBlock := uint64(100)
	ev := makeSubmittedEvent(app, epoch, txHashA)
	ev.OutputsMerkleRoot = common.HexToHash("0xBAD") // wrong root

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(0), nil).Once()
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once()
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once()
	ic.On("FilterClaimSubmitted", mock.Anything, ([]common.Address)(nil),
		[]common.Address{app.IApplicationAddress}).
		Return([]*iconsensus.IConsensusClaimSubmitted{ev}, nil).Once()

	r.On("UpdateApplicationState", ctx, app.ID, model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	err := updateClaimSubmitted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.Error(t, err)
}

// More events than epochs: only the available epoch is processed; endBlock is
// capped to the surplus event's block minus 1 before UpdateEventLastCheckBlock.
func TestUpdateSubmitted_MoreEventsThanEpochs(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeComputedEpoch(app.ID, 0)
	epochs := []*model.Epoch{epoch} // only 1 epoch
	endBlock := uint64(200)

	ev0 := makeSubmittedEvent(app, epoch, txHashA)
	// ev1 is a second event for which there is no epoch
	epoch1 := makeComputedEpoch(app.ID, 1)
	ev1 := makeSubmittedEvent(app, epoch1, txHashB)
	ev1.Raw.BlockNumber = 150

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	// Two transitions: count goes 0→1 at one block, 1→2 at another.
	// Simplify by returning both events from a single FilterClaimSubmitted call
	// on a single transition block (count jumps by 2).
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(0), nil).Once() // prevValue
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(2), nil).Once() // endBlock value
	ic.On("GetNumberOfSubmittedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(2), nil) // mid-point queries all resolve to 2 as well
	ic.On("FilterClaimSubmitted", mock.Anything, ([]common.Address)(nil),
		[]common.Address{app.IApplicationAddress}).
		Return([]*iconsensus.IConsensusClaimSubmitted{ev0, ev1}, nil).Once()

	r.On("UpdateEpochClaimSubmittedTransactionHash", ctx, app.Name, epoch).
		Return(nil).Once()
	r.On("UpdateEpochStatus", ctx, app.Name, epoch).
		Return(nil).Once()
	// endBlock is capped to ev1.Raw.BlockNumber - 1 = 149
	r.On("UpdateEventLastCheckBlock", ctx, []int64{app.ID}, model.MonitoredEvent_ClaimSubmitted, uint64(149)).
		Return(nil).Once()

	err := updateClaimSubmitted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.NoError(t, err)
	assert.Equal(t, model.EpochStatus_ClaimSubmitted, epoch.Status)
}

// ////////////////////////////////////////////////////////////////////////////
// updateClaimAccepted
// ////////////////////////////////////////////////////////////////////////////

// updateClaimAccepted must use GetNumberOfAcceptedClaims (not GetNumberOfSubmittedClaims).
// When the count does not change, no FilterClaimAccepted is called.
func TestUpdateAccepted_NoEvents(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeSubmittedEpoch(app.ID, 0)
	epochs := []*model.Epoch{epoch}
	endBlock := uint64(100)

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	count := big.NewInt(0)
	// oracle must call GetNumberOfAcceptedClaims — NOT GetNumberOfSubmittedClaims
	// FindTransitions calls oracle at startBlock-1, startBlock, and endBlock (3 total)
	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(count, nil).Times(3)
	// GetNumberOfSubmittedClaims must NOT be called (would indicate the bug is present)
	r.On("UpdateEventLastCheckBlock", ctx, []int64{app.ID}, model.MonitoredEvent_ClaimAccepted, endBlock).
		Return(nil).Once()

	err := updateClaimAccepted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.NoError(t, err)
	ic.AssertNotCalled(t, "GetNumberOfSubmittedClaims")
}

// One accepted event matches the first epoch.
func TestUpdateAccepted_FirstClaim(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeSubmittedEpoch(app.ID, 0)
	epoch.VirtualIndex = 0
	epochs := []*model.Epoch{epoch}
	endBlock := uint64(100)
	ev := makeAcceptedEvent(app, epoch, txHashA)

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(0), nil).Once()
	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once()
	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once()
	ic.On("FilterClaimAccepted", mock.Anything, []common.Address{app.IApplicationAddress}).
		Return([]*iconsensus.IConsensusClaimAccepted{ev}, nil).Once()

	r.On("UpdateEpochClaimAcceptedTransactionHash", ctx, app.Name, epoch).
		Return(nil).Once()
	r.On("UpdateEpochStatus", ctx, app.Name, epoch).
		Return(nil).Once()
	r.On("UpdateEventLastCheckBlock", ctx, []int64{app.ID}, model.MonitoredEvent_ClaimAccepted, endBlock).
		Return(nil).Once()

	err := updateClaimAccepted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.NoError(t, err)
	assert.Equal(t, model.EpochStatus_ClaimAccepted, epoch.Status)
	assert.Equal(t, txHashA, *epoch.ClaimAcceptedTransactionHash)
}

// VirtualIndex mismatch on accepted event → application marked inoperable.
func TestUpdateAccepted_VirtualIndexMismatch(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeSubmittedEpoch(app.ID, 0)
	epoch.VirtualIndex = 7 // base=0, expected=0, got=7
	epochs := []*model.Epoch{epoch}
	endBlock := uint64(100)
	ev := makeAcceptedEvent(app, epoch, txHashA)

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(0), nil).Once()
	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once()
	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once()
	ic.On("FilterClaimAccepted", mock.Anything, []common.Address{app.IApplicationAddress}).
		Return([]*iconsensus.IConsensusClaimAccepted{ev}, nil).Once()

	r.On("UpdateApplicationState", ctx, app.ID, model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	err := updateClaimAccepted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.Error(t, err)
}

// Event field mismatch (wrong OutputsMerkleRoot) on accepted event → inoperable.
func TestUpdateAccepted_EventMismatch(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeSubmittedEpoch(app.ID, 0)
	epoch.VirtualIndex = 0
	epochs := []*model.Epoch{epoch}
	endBlock := uint64(100)
	ev := makeAcceptedEvent(app, epoch, txHashA)
	ev.OutputsMerkleRoot = common.HexToHash("0xBAD")

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(0), nil).Once()
	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once()
	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(1), nil).Once()
	ic.On("FilterClaimAccepted", mock.Anything, []common.Address{app.IApplicationAddress}).
		Return([]*iconsensus.IConsensusClaimAccepted{ev}, nil).Once()

	r.On("UpdateApplicationState", ctx, app.ID, model.ApplicationState_Inoperable, mock.Anything).
		Return(nil).Once()

	err := updateClaimAccepted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.Error(t, err)
}

// More accepted events than epochs → only available epochs processed, endBlock capped.
func TestUpdateAccepted_MoreEventsThanEpochs(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	epoch := makeSubmittedEpoch(app.ID, 0)
	epoch.VirtualIndex = 0
	epochs := []*model.Epoch{epoch}
	endBlock := uint64(200)

	ev0 := makeAcceptedEvent(app, epoch, txHashA)
	epoch1 := makeSubmittedEpoch(app.ID, 1)
	ev1 := makeAcceptedEvent(app, epoch1, txHashB)
	ev1.Raw.BlockNumber = 150

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	ic := &consensusMock{}
	defer ic.AssertExpectations(t)

	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(0), nil).Once()
	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(2), nil).Once()
	ic.On("GetNumberOfAcceptedClaims", mock.Anything, app.IApplicationAddress).
		Return(big.NewInt(2), nil)
	ic.On("FilterClaimAccepted", mock.Anything, []common.Address{app.IApplicationAddress}).
		Return([]*iconsensus.IConsensusClaimAccepted{ev0, ev1}, nil).Once()

	r.On("UpdateEpochClaimAcceptedTransactionHash", ctx, app.Name, epoch).
		Return(nil).Once()
	r.On("UpdateEpochStatus", ctx, app.Name, epoch).
		Return(nil).Once()
	r.On("UpdateEventLastCheckBlock", ctx, []int64{app.ID}, model.MonitoredEvent_ClaimAccepted, uint64(149)).
		Return(nil).Once()

	err := updateClaimAccepted(ctx, logger, app, epochs, r, ic, endBlock)
	assert.NoError(t, err)
	assert.Equal(t, model.EpochStatus_ClaimAccepted, epoch.Status)
}

// ////////////////////////////////////////////////////////////////////////////
// updateApplication
// ////////////////////////////////////////////////////////////////////////////

// When ListEpochs returns no epochs, updateApplication returns nil immediately
// and never calls newConsensus (and thus never calls iBlockchain RPC methods).
func TestUpdateApp_NoEpochs(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	endBlock := uint64(100)

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	b := &blockchainMock{}

	r.On("ListEpochs", ctx, app.Name, epochFilter(), repository.Pagination{}, false).
		Return([]*model.Epoch{}, uint64(0), nil).Once()

	err := updateApplication(ctx, logger, nil, r, b, app, endBlock)
	assert.NoError(t, err)
}

// ////////////////////////////////////////////////////////////////////////////
// update
// ////////////////////////////////////////////////////////////////////////////

// When ListApplications returns no apps, update returns an empty error slice.
func TestUpdate_NoApps(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	b := &blockchainMock{}

	r.On("ListApplications", ctx, appFilter(), repository.Pagination{}, true).
		Return([]*model.Application{}, uint64(0), nil).Once()

	errs := update(ctx, logger, nil, r, b, 100)
	assert.Empty(t, errs)
}

// Dave consensus (PRT) apps are skipped without error.
func TestUpdate_DaveConsensusSkipped(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	daveApp := makeDaveApp()

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	b := &blockchainMock{}

	r.On("ListApplications", ctx, appFilter(), repository.Pagination{}, true).
		Return([]*model.Application{daveApp}, uint64(1), nil).Once()

	errs := update(ctx, logger, nil, r, b, 100)
	assert.Empty(t, errs)
	// ListEpochs must not be called for a PRT app
	r.AssertNotCalled(t, "ListEpochs")
}

// An enabled non-PRT app with no epochs is processed without error.
func TestUpdate_EnabledApp_NoEpochs(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	app := makeApp()
	endBlock := uint64(100)

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	b := &blockchainMock{}

	r.On("ListApplications", ctx, appFilter(), repository.Pagination{}, true).
		Return([]*model.Application{app}, uint64(1), nil).Once()
	r.On("ListEpochs", ctx, app.Name, epochFilter(), repository.Pagination{}, false).
		Return([]*model.Epoch{}, uint64(0), nil).Once()

	errs := update(ctx, logger, nil, r, b, endBlock)
	assert.Empty(t, errs)
}

// ListApplications error is propagated as the single returned error.
func TestUpdate_ListApplicationsError(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()
	expectedErr := errors.New("db failure")

	r := &repositoryMock{}
	defer r.AssertExpectations(t)
	b := &blockchainMock{}

	r.On("ListApplications", ctx, appFilter(), repository.Pagination{}, true).
		Return([]*model.Application(nil), uint64(0), expectedErr).Once()

	errs := update(ctx, logger, nil, r, b, 100)
	assert.Len(t, errs, 1)
	assert.ErrorIs(t, errs[0], expectedErr)
}
