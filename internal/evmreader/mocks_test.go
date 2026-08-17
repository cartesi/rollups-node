// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"math/big"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// blockRange returns a mock.MatchedBy matcher for CallOpts where BlockNumber is in [lo, hi).
func blockRange(lo, hi uint64) interface{} {
	return mock.MatchedBy(func(opts *bind.CallOpts) bool {
		b := opts.BlockNumber.Uint64()
		return b >= lo && b < hi
	})
}

// blockFrom returns a mock.MatchedBy matcher for CallOpts where BlockNumber >= lo.
func blockFrom(lo uint64) interface{} {
	return mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() >= lo
	})
}

// UnsetAll removes all expectations for the given method from a mock.
func UnsetAll(m *mock.Mock, methodName string) {
	var index int
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			continue
		}
		m.ExpectedCalls[index] = call
		index++
	}
	m.ExpectedCalls = m.ExpectedCalls[:index]
}

func copyApplications(apps []*Application) []*Application {
	copies := make([]*Application, len(apps))
	for i, app := range apps {
		if app == nil {
			continue
		}
		copyApp := *app
		copies[i] = &copyApp
	}
	return copies
}

// ---------------------------------------------------------------------------
// MockEthClient
// ---------------------------------------------------------------------------

type MockEthClient struct {
	mock.Mock
}

func newMockEthClient() *MockEthClient {
	return &MockEthClient{}
}

func (m *MockEthClient) SetupDefaultBehavior() *MockEthClient {
	return m
}

func (m *MockEthClient) EnqueueNewHead(blknum int64) *mock.Call {
	return m.On("HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&types.Header{Number: big.NewInt(blknum)}, nil)
}

func (m *MockEthClient) Unset(methodName string) {
	UnsetAll(&m.Mock, methodName)
}

func (m *MockEthClient) HeaderByNumber(
	ctx context.Context,
	number *big.Int,
) (*types.Header, error) {
	args := m.Called(ctx, number)
	return args.Get(0).(*types.Header), args.Error(1)
}

func (m *MockEthClient) ChainID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

// ---------------------------------------------------------------------------
// MockInputBox
// ---------------------------------------------------------------------------

type MockInputBox struct {
	mock.Mock
}

func newMockInputBox() *MockInputBox {
	return &MockInputBox{}
}

func (m *MockInputBox) SetupDefaultBehavior() *MockInputBox {
	// RetrieveInputs: matched by Start block, not call order.
	m.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x11 }),
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{inputAddedEvent0}, nil)

	m.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x12 }),
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{inputAddedEvent1}, nil)

	m.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x13 }),
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{inputAddedEvent2, inputAddedEvent3}, nil)

	// GetNumberOfInputs: block-based matching models the on-chain state.
	// Each range returns the input count at that point in the blockchain.
	m.On("GetNumberOfInputs", blockRange(0, 0x11), mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	m.On("GetNumberOfInputs", blockRange(0x11, 0x12), mock.Anything).
		Return(new(big.Int).SetUint64(1), nil)
	m.On("GetNumberOfInputs", blockRange(0x12, 0x13), mock.Anything).
		Return(new(big.Int).SetUint64(2), nil)
	m.On("GetNumberOfInputs", blockFrom(0x13), mock.Anything).
		Return(new(big.Int).SetUint64(4), nil)
	return m
}

func (m *MockInputBox) Unset(methodName string) {
	UnsetAll(&m.Mock, methodName)
}

func (m *MockInputBox) RetrieveInputs(
	opts *bind.FilterOpts,
	appContract []common.Address,
	index []*big.Int,
) ([]iinputbox.IInputBoxInputAdded, error) {
	args := m.Called(opts, appContract, index)
	return args.Get(0).([]iinputbox.IInputBoxInputAdded), args.Error(1)
}

func (m *MockInputBox) GetNumberOfInputs(
	opts *bind.CallOpts, appContract common.Address,
) (*big.Int, error) {
	args := m.Called(opts, appContract)
	return args.Get(0).(*big.Int), args.Error(1)
}

// ---------------------------------------------------------------------------
// MockRepository
// ---------------------------------------------------------------------------

type MockRepository struct {
	mock.Mock
}

func newMockRepository() *MockRepository {
	m := &MockRepository{}
	m.On("GetNumberOfPendingExecutableOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(1), nil).Maybe()
	return m
}

func (m *MockRepository) SetupDefaultBehavior() *MockRepository {

	apps := copyApplications(applications)
	m.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(2), nil).Once()

	apps = copyApplications(applications)
	apps[0].LastInputCheckBlock = 0x11
	apps[0].LastOutputCheckBlock = 0x11
	apps[1].LastOutputCheckBlock = 0x11
	m.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(2), nil).Once()

	apps = copyApplications(applications)
	apps[0].LastInputCheckBlock = 0x12
	apps[0].LastOutputCheckBlock = 0x12
	apps[1].LastOutputCheckBlock = 0x12
	m.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(2), nil).Once()

	m.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Return(nil).Times(1)
	m.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(8)
	m.On("UpdateApplicationLastForecloseCheckBlock",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nil).Maybe()

	m.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Once().Return(uint64(0), nil)
	m.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Once().Return(uint64(1), nil)
	m.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Once().Return(uint64(2), nil)

	m.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(0), nil).Times(6)
	m.On("CreateEpochsAndInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(nil)

	m.On("GetEpoch",
		mock.Anything,
		mock.Anything,
		uint64(0)).Return(nil, nil).Once()
	m.On("GetEpoch",
		mock.Anything,
		mock.Anything,
		uint64(1)).Return(
		&Epoch{
			Index:                1,
			FirstBlock:           11,
			LastBlock:            20,
			Status:               EpochStatus_Open,
			TxBufferDataBlock:    nil,
			ClaimTransactionHash: nil,
		}, nil).Twice()

	// Catch-all: returns empty list for extra headers.
	m.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{}, uint64(0), nil)

	return m
}

func (m *MockRepository) Unset(methodName string) {
	UnsetAll(&m.Mock, methodName)
}

func (m *MockRepository) ListApplications(
	ctx context.Context,
	f repository.ApplicationFilter,
	pagination repository.Pagination,
	descending bool,
) ([]*Application, uint64, error) {
	args := m.Called(ctx, f, pagination, descending)
	return args.Get(0).([]*Application), args.Get(1).(uint64), args.Error(2)
}

func (m *MockRepository) SaveNodeConfigRaw(
	ctx context.Context, key string, rawJSON []byte,
) error {
	args := m.Called(ctx, key, rawJSON)
	return args.Error(0)
}

func (m *MockRepository) LoadNodeConfigRaw(
	ctx context.Context, key string,
) (rawJSON []byte, createdAt, updatedAt time.Time, err error) {
	args := m.Called(ctx, key)
	return args.Get(0).([]byte), args.Get(1).(time.Time), args.Get(2).(time.Time), args.Error(3)
}

func (m *MockRepository) CreateEpochsAndInputs(
	ctx context.Context, nameOrAddress string,
	epochInputMap map[*Epoch][]*Input, blockNumber uint64,
) error {
	args := m.Called(ctx, nameOrAddress, epochInputMap, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) GetEpoch(
	ctx context.Context, nameOrAddress string, index uint64,
) (*Epoch, error) {
	args := m.Called(ctx, nameOrAddress, index)
	obj := args.Get(0)
	if obj == nil {
		return nil, args.Error(1)
	}
	return obj.(*Epoch), args.Error(1)
}

func (m *MockRepository) ListEpochs(
	ctx context.Context, nameOrAddress string,
	f repository.EpochFilter, p repository.Pagination, descending bool,
) ([]*Epoch, uint64, error) {
	args := m.Called(ctx, nameOrAddress, f, p, descending)
	return args.Get(0).([]*Epoch), args.Get(1).(uint64), args.Error(2)
}

func (m *MockRepository) GetOutput(
	ctx context.Context, nameOrAddress string, indexKey uint64,
) (*Output, error) {
	args := m.Called(ctx, nameOrAddress, indexKey)
	obj := args.Get(0)
	if obj == nil {
		return nil, args.Error(1)
	}
	return obj.(*Output), args.Error(1)
}

func (m *MockRepository) UpdateEpochClaimTransactionHash(
	ctx context.Context, nameOrAddress string, e *Epoch,
) error {
	args := m.Called(ctx, nameOrAddress, e)
	return args.Error(0)
}

func (m *MockRepository) GetLastNonOpenEpoch(
	ctx context.Context, nameOrAddress string,
) (*Epoch, error) {
	args := m.Called(ctx, nameOrAddress)
	obj := args.Get(0)
	if obj == nil {
		return nil, args.Error(1)
	}
	return obj.(*Epoch), args.Error(1)
}

func (m *MockRepository) GetNumberOfInputs(
	ctx context.Context, nameOrAddress string,
) (uint64, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRepository) GetNumberOfExecutedOutputs(
	ctx context.Context, nameOrAddress string,
) (uint64, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRepository) GetNumberOfPendingExecutableOutputs(
	ctx context.Context, nameOrAddress string,
) (uint64, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRepository) UpdateOutputsExecution(
	ctx context.Context, nameOrAddress string,
	executedOutputs []*Output, blockNumber uint64,
) error {
	args := m.Called(ctx, nameOrAddress, executedOutputs, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) UpdateApplicationStatus(
	ctx context.Context, appID int64, state ApplicationStatus, reason *string,
) error {
	args := m.Called(ctx, appID, state, reason)
	return args.Error(0)
}

func (m *MockRepository) UpdateApplicationForeclosure(
	ctx context.Context, appID int64, block uint64, txHash common.Hash, blockNumber uint64,
) error {
	args := m.Called(ctx, appID, block, txHash, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) UpdateApplicationLastForecloseCheckBlock(
	ctx context.Context, appID int64, blockNumber uint64,
) error {
	args := m.Called(ctx, appID, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) UpdateAccountsDriveProved(
	ctx context.Context, appID int64, block uint64, txHash common.Hash, root common.Hash, blockNumber uint64,
) error {
	args := m.Called(ctx, appID, block, txHash, root, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) UpdateApplicationLastAccountsDriveProvedCheckBlock(
	ctx context.Context, appID int64, blockNumber uint64,
) error {
	args := m.Called(ctx, appID, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) StoreWithdrawalEvents(
	ctx context.Context, appID int64, withdrawals []*Withdrawal, blockNumber uint64,
) error {
	args := m.Called(ctx, appID, withdrawals, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) GetNumberOfWithdrawals(ctx context.Context, appID int64) (uint64, error) {
	args := m.Called(ctx, appID)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRepository) InsertWithdrawal(ctx context.Context, w *Withdrawal) error {
	args := m.Called(ctx, w)
	return args.Error(0)
}

func (m *MockRepository) UpdateEventLastCheckBlock(
	ctx context.Context, appIDs []int64,
	event MonitoredEvent, blockNumber uint64,
) error {
	args := m.Called(ctx, appIDs, event, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) GetEventLastCheckBlock(
	ctx context.Context, appID int64, event MonitoredEvent,
) (uint64, error) {
	args := m.Called(ctx, appID, event)
	return args.Get(0).(uint64), args.Error(1)
}

// ---------------------------------------------------------------------------
// MockApplicationContract
// ---------------------------------------------------------------------------

type MockApplicationContract struct {
	mock.Mock
}

func newMockApplicationContract() *MockApplicationContract {
	m := &MockApplicationContract{}
	// Foreclosure detection runs on every evmreader tick. Default to a
	// not-foreclosed app so tests that don't care about this path don't
	// need to wire it up. .Maybe() lets AssertExpectations pass even
	// when these calls didn't happen (test never reached the foreclosure
	// branch). Tests that exercise foreclosure call Unset("IsForeclosed")
	// + re-mock with the desired behavior.
	m.On("IsForeclosed", mock.Anything).Return(false, nil).Maybe()
	m.On("RetrieveForeclosureEvents", mock.Anything).
		Return([]*iapplication.IApplicationForeclosure{}, nil).Maybe()
	return m
}

func (m *MockApplicationContract) SetupDefaultBehavior() *MockApplicationContract {
	m.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(0x10), nil)
	m.On("GetNumberOfExecutedOutputs", mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	return m
}

func (m *MockApplicationContract) Unset(methodName string) {
	UnsetAll(&m.Mock, methodName)
}

func (m *MockApplicationContract) RetrieveOutputExecutionEvents(
	opts *bind.FilterOpts,
) ([]*iapplication.IApplicationOutputExecuted, error) {
	args := m.Called(opts)
	return args.Get(0).([]*iapplication.IApplicationOutputExecuted), args.Error(1)
}

func (m *MockApplicationContract) RetrieveForeclosureEvents(
	opts *bind.FilterOpts,
) ([]*iapplication.IApplicationForeclosure, error) {
	args := m.Called(opts)
	return args.Get(0).([]*iapplication.IApplicationForeclosure), args.Error(1)
}

func (m *MockApplicationContract) GetDeploymentBlockNumber(
	opts *bind.CallOpts,
) (*big.Int, error) {
	args := m.Called(opts)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockApplicationContract) GetNumberOfExecutedOutputs(
	opts *bind.CallOpts,
) (*big.Int, error) {
	args := m.Called(opts)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockApplicationContract) GetAccountsDriveMerkleRoot(opts *bind.CallOpts) (bool, common.Hash, error) {
	args := m.Called(opts)
	return args.Bool(0), args.Get(1).(common.Hash), args.Error(2)
}

func (m *MockApplicationContract) IsForeclosed(opts *bind.CallOpts) (bool, error) {
	args := m.Called(opts)
	return args.Bool(0), args.Error(1)
}

func (m *MockApplicationContract) GetNumberOfWithdrawals(opts *bind.CallOpts) (*big.Int, error) {
	args := m.Called(opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockApplicationContract) RetrieveWithdrawalEvents(
	opts *bind.FilterOpts,
) ([]*iapplication.IApplicationWithdrawal, error) {
	args := m.Called(opts)
	return args.Get(0).([]*iapplication.IApplicationWithdrawal), args.Error(1)
}

func (m *MockApplicationContract) RetrieveAccountsDriveProvedEvents(
	opts *bind.FilterOpts,
) ([]*iapplication.IApplicationAccountsDriveMerkleRootProved, error) {
	args := m.Called(opts)
	return args.Get(0).([]*iapplication.IApplicationAccountsDriveMerkleRootProved), args.Error(1)
}

// ---------------------------------------------------------------------------
// MockDaveConsensus
// ---------------------------------------------------------------------------

type MockDaveConsensus struct {
	mock.Mock
}

func newMockDaveConsensus() *MockDaveConsensus {
	return &MockDaveConsensus{}
}

func (m *MockDaveConsensus) Unset(methodName string) {
	UnsetAll(&m.Mock, methodName)
}

func (m *MockDaveConsensus) GetInputBox(
	opts *bind.CallOpts,
) (common.Address, error) {
	args := m.Called(opts)
	return args.Get(0).(common.Address), args.Error(1)
}

func (m *MockDaveConsensus) GetCurrentSealedEpoch(
	opts *bind.CallOpts,
) (struct {
	EpochNumber          *big.Int
	InputIndexLowerBound *big.Int
	InputIndexUpperBound *big.Int
	Tournament           common.Address
}, error) {
	args := m.Called(opts)
	return args.Get(0).(struct {
		EpochNumber          *big.Int
		InputIndexLowerBound *big.Int
		InputIndexUpperBound *big.Int
		Tournament           common.Address
	}), args.Error(1)
}

func (m *MockDaveConsensus) GetApplicationContract(
	opts *bind.CallOpts,
) (common.Address, error) {
	args := m.Called(opts)
	return args.Get(0).(common.Address), args.Error(1)
}

func (m *MockDaveConsensus) GetTournamentFactory(
	opts *bind.CallOpts,
) (common.Address, error) {
	args := m.Called(opts)
	return args.Get(0).(common.Address), args.Error(1)
}

func (m *MockDaveConsensus) GetDeploymentBlockNumber(
	opts *bind.CallOpts,
) (*big.Int, error) {
	args := m.Called(opts)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockDaveConsensus) RetrieveSealedEpochs(
	opts *bind.FilterOpts,
) ([]*idaveconsensus.IDaveConsensusEpochSealed, error) {
	args := m.Called(opts)
	return args.Get(0).([]*idaveconsensus.IDaveConsensusEpochSealed), args.Error(1)
}

// ---------------------------------------------------------------------------
// MockAdapterFactory
// ---------------------------------------------------------------------------

type MockAdapterFactory struct {
	mock.Mock
}

func newMockAdapterFactory() *MockAdapterFactory {
	return &MockAdapterFactory{}
}

func (m *MockAdapterFactory) Unset(methodName string) {
	UnsetAll(&m.Mock, methodName)
}

func (m *MockAdapterFactory) CreateAdapters(
	app *Application,
) (ApplicationContractAdapter, InputSourceAdapter, DaveConsensusAdapter, error) {
	args := m.Called(app)

	// Safely handle nil values to prevent interface conversion panic
	appContract, _ := args.Get(0).(ApplicationContractAdapter)
	inputSource, _ := args.Get(1).(InputSourceAdapter)
	daveConsensus, _ := args.Get(2).(DaveConsensusAdapter)

	// If we got nil values but no error was returned, return mock implementations
	if appContract == nil && args.Error(3) == nil {
		appContract = &MockApplicationContract{}
	}

	if inputSource == nil && args.Error(3) == nil {
		inputSource = newMockInputBox()
	}

	return appContract, inputSource, daveConsensus, args.Error(3)
}

func (m *MockAdapterFactory) SetupDefaultBehavior(
	appContract1 *MockApplicationContract,
	appContract2 *MockApplicationContract,
	inputBox1 *MockInputBox,
) *MockAdapterFactory {

	// Match by application address so adapters are returned correctly regardless of call count
	// (adapter caching means CreateAdapters is only called once per app address)
	m.On("CreateAdapters",
		mock.MatchedBy(func(app *Application) bool {
			return app.IApplicationAddress == applications[0].IApplicationAddress
		}),
	).Return(appContract1, inputBox1, nil, nil)
	m.On("CreateAdapters",
		mock.MatchedBy(func(app *Application) bool {
			return app.IApplicationAddress == applications[1].IApplicationAddress
		}),
	).Return(appContract2, nil, nil, nil)
	return m
}

func (m *MockAdapterFactory) SetupDefaultBehaviorSingleApp(
	appContract *MockApplicationContract,
	inputBox *MockInputBox,
) *MockAdapterFactory {
	m.On("CreateAdapters",
		mock.Anything,
	).Return(appContract, inputBox, nil, nil)
	return m
}
