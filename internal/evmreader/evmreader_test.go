// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	suiteTimeout = 120 * time.Second
)

//go:embed testdata/input_added_event_0.json
var inputAddedEvent0JsonBytes []byte

//go:embed testdata/input_added_event_1.json
var inputAddedEvent1JsonBytes []byte

//go:embed testdata/input_added_event_2.json
var inputAddedEvent2JsonBytes []byte

//go:embed testdata/input_added_event_3.json
var inputAddedEvent3JsonBytes []byte

//go:embed testdata/header_0.json
var header0JsonBytes []byte

//go:embed testdata/header_1.json
var header1JsonBytes []byte

//go:embed testdata/header_2.json
var header2JsonBytes []byte

//go:embed testdata/header_3.json
var header3JsonBytes []byte

var (
	header0 = types.Header{}
	header1 = types.Header{}
	header2 = types.Header{}
	header3 = types.Header{}

	inputAddedEvent0 = iinputbox.IInputBoxInputAdded{}
	inputAddedEvent1 = iinputbox.IInputBoxInputAdded{}
	inputAddedEvent2 = iinputbox.IInputBoxInputAdded{}
	inputAddedEvent3 = iinputbox.IInputBoxInputAdded{}

	subscription0 = newMockSubscription()
)

var applications = []*Application{{
	Name:                 "my-app-1",
	IApplicationAddress:  common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
	IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
	IInputBoxAddress:     common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3"),
	DataAvailability:     DataAvailability_InputBox[:],
	IInputBoxBlock:       0x01,
	EpochLength:          10,
	LastInputCheckBlock:  0x00,
	LastOutputCheckBlock: 0x00,
}, {
	Name:                 "my-app-2",
	IApplicationAddress:  common.HexToAddress("0x78c716FDaE477595a820D86D0eFAfe0eE54dF7dB"),
	IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
	IInputBoxAddress:     common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3"),
	DataAvailability:     []byte{0x11, 0x32, 0x45, 0x56},
	IInputBoxBlock:       0x01,
	EpochLength:          10,
	LastInputCheckBlock:  0x00,
	LastOutputCheckBlock: 0x00,
}}

type EvmReaderSuite struct {
	suite.Suite
	ctx                  context.Context
	cancel               context.CancelFunc
	client               *MockEthClient
	wsClient             *MockEthClient
	repository           *MockRepository
	evmReader            *Service
	contractFactory      *MockAdapterFactory
	applicationContract1 *MockApplicationContract
	applicationContract2 *MockApplicationContract
	inputBox             *MockInputBox
}

func TestEvmReaderSuite(t *testing.T) {
	suite.Run(t, new(EvmReaderSuite))
}

func (s *EvmReaderSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), suiteTimeout)
	config.SetDefaults()

	err := json.Unmarshal(header0JsonBytes, &header0)
	s.Require().Nil(err)
	err = json.Unmarshal(header1JsonBytes, &header1)
	s.Require().Nil(err)
	err = json.Unmarshal(header2JsonBytes, &header2)
	s.Require().Nil(err)
	err = json.Unmarshal(header3JsonBytes, &header3)
	s.Require().Nil(err)

	err = json.Unmarshal(inputAddedEvent0JsonBytes, &inputAddedEvent0)
	s.Require().Nil(err)
	err = json.Unmarshal(inputAddedEvent1JsonBytes, &inputAddedEvent1)
	s.Require().Nil(err)
	err = json.Unmarshal(inputAddedEvent2JsonBytes, &inputAddedEvent2)
	s.Require().Nil(err)
	err = json.Unmarshal(inputAddedEvent3JsonBytes, &inputAddedEvent3)
	s.Require().Nil(err)
}

func (s *EvmReaderSuite) TearDownSuite() {
	s.cancel()
}

func (s *EvmReaderSuite) SetupTest() {
	s.client = newMockEthClient().SetupDefaultBehavior()
	s.wsClient = newMockEthClient().SetupDefaultWsBehavior()
	s.repository = newMockRepository().SetupDefaultBehavior()
	s.applicationContract1 = newMockApplicationContract().SetupDefaultBehavior()
	s.applicationContract2 = newMockApplicationContract().SetupDefaultBehavior()
	s.inputBox = newMockInputBox().SetupDefaultBehavior(s.ctx)
	s.contractFactory = newMockAdapterFactory().SetupDefaultBehavior(s.applicationContract1, s.applicationContract2, s.inputBox)

	s.evmReader = &Service{
		client:                              s.client,
		wsClient:                            s.wsClient,
		repository:                          s.repository,
		defaultBlock:                        DefaultBlock_Latest,
		adapterFactory:                      s.contractFactory,
		hasEnabledApps:                      true,
		inputReaderEnabled:                  true,
		blockchainMaxRetries:                0,
		blockchainSubscriptionRetryInterval: time.Second,
	}

	logLevel, err := config.GetLogLevel()
	s.Require().Nil(err)

	serviceArgs := &service.CreateInfo{Name: "evm-reader", Impl: s.evmReader, LogLevel: logLevel}
	err = service.Create(context.Background(), serviceArgs, &s.evmReader.Service)
	s.Require().Nil(err)
}

// Service tests
func (s *EvmReaderSuite) TestItStopsWhenContextIsCanceled() {
	ctx, cancel := context.WithCancel(s.ctx)
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- s.evmReader.Run(ctx, ready)
	}()
	cancel()

	err := <-errChannel
	s.Require().Equal(context.Canceled, err, "stopped for the wrong reason")
}

func (s *EvmReaderSuite) TestItEventuallyBecomesReady() {
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- s.evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
	case err := <-errChannel:
		s.FailNow("unexpected failure", err)
	}
}

func (s *EvmReaderSuite) TestItFailsToSubscribeForNewInputsOnStart() {
	s.wsClient.Unset("ChainID")
	s.wsClient.Unset("SubscribeNewHead")
	emptySubscription := &MockSubscription{}
	s.wsClient.On(
		"SubscribeNewHead",
		mock.Anything,
		mock.Anything,
	).Return(emptySubscription, fmt.Errorf("expected failure"))

	err := s.evmReader.Run(s.ctx, make(chan struct{}, 1))
	s.Require().ErrorContains(err, "expected failure")
	s.wsClient.AssertNumberOfCalls(s.T(), "SubscribeNewHead", 1)
	s.wsClient.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) TestIndexApps() {

	s.Run("Ok", func() {
		apps := []appContracts{
			{application: &Application{LastInputCheckBlock: 23}},
			{application: &Application{LastInputCheckBlock: 22}},
			{application: &Application{LastInputCheckBlock: 21}},
			{application: &Application{LastInputCheckBlock: 23}},
		}

		keyByProcessedBlock := func(a appContracts) uint64 {
			return a.application.LastInputCheckBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(3, len(indexApps))
		apps, ok := indexApps[23]
		s.Require().True(ok)
		s.Require().Equal(2, len(apps))
	})

	s.Run("whenIndexAppsArrayEmpty", func() {
		apps := []appContracts{}

		keyByProcessedBlock := func(a appContracts) uint64 {
			return a.application.LastInputCheckBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(0, len(indexApps))
	})

	s.Run("whenIndexAppsArray", func() {
		apps := []appContracts{}

		keyByProcessedBlock := func(a appContracts) uint64 {
			return a.application.LastInputCheckBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(0, len(indexApps))
	})

	s.Run("whenUsesWrongKey", func() {
		apps := []appContracts{
			{application: &Application{LastInputCheckBlock: 23}},
			{application: &Application{LastInputCheckBlock: 22}},
			{application: &Application{LastInputCheckBlock: 21}},
			{application: &Application{LastInputCheckBlock: 23}},
		}

		keyByProcessedBlock := func(a appContracts) uint64 {
			return a.application.LastInputCheckBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(3, len(indexApps))
		apps, ok := indexApps[0]
		s.Require().False(ok)
		s.Require().Nil(apps)

	})

}

// Mock EthClient
type MockEthClient struct {
	mock.Mock
}

func newMockEthClient() *MockEthClient {
	return &MockEthClient{}
}

func (m *MockEthClient) SetupDefaultBehavior() *MockEthClient {
	return m
}

func (m *MockEthClient) SetupDefaultWsBehavior() *MockEthClient {
	m.On("ChainID", mock.Anything).Return(big.NewInt(1), nil)
	m.On("SubscribeNewHead",
		mock.Anything,
		mock.Anything,
	).Return(subscription0, nil)
	return m
}

func UnsetAll(m *mock.Mock, methodName string) {
	// Assuming no multithreading issues for test purposes
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

func (m *MockEthClient) SubscribeNewHead(
	ctx context.Context,
	ch chan<- *types.Header,
) (ethereum.Subscription, error) {
	args := m.Called(ctx, ch)
	return args.Get(0).(ethereum.Subscription), args.Error(1)
}

func (m *MockEthClient) ChainID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), args.Error(1)
}

// Mock ethereum.Subscription
type MockSubscription struct {
	mock.Mock
}

func newMockSubscription() *MockSubscription {
	sub := &MockSubscription{}

	sub.On("Unsubscribe").Return()
	sub.On("Err").Return(make(<-chan error))

	return sub
}

func (m *MockSubscription) Unsubscribe() {
}

func (m *MockSubscription) Err() <-chan error {
	args := m.Called()
	return args.Get(0).(<-chan error)
}

// FakeClient
type FakeWSEhtClient struct {
	ch chan<- *types.Header
}

func (f *FakeWSEhtClient) SubscribeNewHead(
	_ context.Context,
	ch chan<- *types.Header,
) (ethereum.Subscription, error) {
	f.ch = ch
	return newMockSubscription(), nil
}

func (f *FakeWSEhtClient) HeaderByNumber(
	_ context.Context,
	_ *big.Int,
) (*types.Header, error) {
	return &header0, nil
}

func (f *FakeWSEhtClient) ChainID(_ context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (f *FakeWSEhtClient) fireNewHead(header *types.Header) {
	f.ch <- header
}

// Mock inputbox.InputBox
type MockInputBox struct {
	mock.Mock
}

func (m *MockInputBox) SetupDefaultBehavior(ctx context.Context) *MockInputBox {
	events0 := []iinputbox.IInputBoxInputAdded{inputAddedEvent0}
	retrieveInputsOpts0 := bind.FilterOpts{
		Context: ctx,
		Start:   0x11,
		End:     Pointer(uint64(0x11)),
	}
	m.On("RetrieveInputs",
		&retrieveInputsOpts0,
		mock.Anything,
		mock.Anything,
	).Return(events0, nil).Once()

	events1 := []iinputbox.IInputBoxInputAdded{inputAddedEvent1}
	retrieveInputsOpts1 := bind.FilterOpts{
		Context: ctx,
		Start:   0x12,
		End:     Pointer(uint64(0x12)),
	}
	m.On("RetrieveInputs",
		&retrieveInputsOpts1,
		mock.Anything,
		mock.Anything,
	).Return(events1, nil).Once()

	events2 := []iinputbox.IInputBoxInputAdded{inputAddedEvent2, inputAddedEvent3}
	retrieveInputsOpts2 := bind.FilterOpts{
		Context: ctx,
		Start:   0x13,
		End:     Pointer(uint64(0x13)),
	}
	m.On("RetrieveInputs",
		&retrieveInputsOpts2,
		mock.Anything,
		mock.Anything,
	).Return(events2, nil).Once()

	m.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(0), nil).Once()
	m.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(1), nil).Once()
	m.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(0), nil).Times(4)
	m.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(2), nil).Once()
	m.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(4), nil).Once()
	return m
}

func newMockInputBox() *MockInputBox {
	return &MockInputBox{}
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

func (m *MockInputBox) GetNumberOfInputs(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	args := m.Called(opts, appContract)
	return args.Get(0).(*big.Int), args.Error(1)
}

// Mock InputReaderRepository
type MockRepository struct {
	mock.Mock
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
			ClaimHash:            nil,
			ClaimTransactionHash: nil,
		}, nil).Twice()
	return m
}

func newMockRepository() *MockRepository {
	return &MockRepository{}
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

func (m *MockRepository) SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error {
	args := m.Called(ctx, key, rawJSON)
	return args.Error(0)
}

func (m *MockRepository) LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error) {
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

func (m *MockRepository) GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error) {
	args := m.Called(ctx, nameOrAddress, index)
	obj := args.Get(0)
	if obj == nil {
		return nil, args.Error(1)
	}
	return obj.(*Epoch), args.Error(1)
}

func (m *MockRepository) ListEpochs(ctx context.Context, nameOrAddress string,
	f repository.EpochFilter, p repository.Pagination, descending bool) ([]*Epoch, uint64, error) {
	args := m.Called(ctx, nameOrAddress, f, p, descending)
	return args.Get(0).([]*Epoch), args.Get(1).(uint64), args.Error(2)
}

func (m *MockRepository) GetOutput(ctx context.Context, nameOrAddress string, indexKey uint64) (*Output, error) {
	args := m.Called(ctx, nameOrAddress, indexKey)
	obj := args.Get(0)
	if obj == nil {
		return nil, args.Error(1)
	}
	return obj.(*Output), args.Error(1)
}

func (m *MockRepository) UpdateEpoch(ctx context.Context, nameOrAddress string, e *Epoch) error {
	args := m.Called(ctx, nameOrAddress, e)
	return args.Error(0)
}

func (m *MockRepository) GetLastNonOpenEpoch(ctx context.Context, nameOrAddress string) (*Epoch, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(*Epoch), args.Error(1)
}

func (m *MockRepository) GetNumberOfInputs(ctx context.Context, nameOrAddress string) (uint64, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRepository) GetNumberOfExecutedOutputs(ctx context.Context, nameOrAddress string) (uint64, error) {
	args := m.Called(ctx, nameOrAddress)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRepository) UpdateOutputsExecution(ctx context.Context, nameOrAddress string,
	executedOutputs []*Output, blockNumber uint64) error {
	args := m.Called(ctx, nameOrAddress, executedOutputs, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error {
	args := m.Called(ctx, appID, state, reason)
	return args.Error(0)
}

func (m *MockRepository) UpdateEventLastCheckBlock(ctx context.Context, appIDs []int64,
	event MonitoredEvent, blockNumber uint64) error {
	args := m.Called(ctx, appIDs, event, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) GetEventLastCheckBlock(ctx context.Context, appID int64, event MonitoredEvent) (uint64, error) {
	args := m.Called(ctx, appID, event)
	return args.Get(0).(uint64), args.Error(1)
}

type MockApplicationContract struct {
	mock.Mock
}

func (m *MockApplicationContract) SetupDefaultBehavior() *MockApplicationContract {
	m.On("GetDeploymentBlockNumber",
		mock.Anything,
	).Return(new(big.Int).SetUint64(0x10), nil).Once()
	m.On("GetNumberOfExecutedOutputs",
		mock.Anything,
	).Return(new(big.Int).SetUint64(0), nil).Times(4)
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

func (m *MockApplicationContract) GetDeploymentBlockNumber(opts *bind.CallOpts) (*big.Int, error) {
	args := m.Called(opts)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockApplicationContract) GetNumberOfExecutedOutputs(opts *bind.CallOpts) (*big.Int, error) {
	args := m.Called(opts)
	return args.Get(0).(*big.Int), args.Error(1)
}

func newMockApplicationContract() *MockApplicationContract {
	return &MockApplicationContract{}
}

type MockAdapterFactory struct {
	mock.Mock
}

func (m *MockAdapterFactory) Unset(methodName string) {
	UnsetAll(&m.Mock, methodName)
}

func (m *MockAdapterFactory) CreateAdapters(
	app *Application,
	client EthClientInterface,
) (ApplicationContractAdapter, InputSourceAdapter, DaveConsensusAdapter, error) {
	args := m.Called(app, client)

	// Safely handle nil values to prevent interface conversion panic
	appContract, _ := args.Get(0).(ApplicationContractAdapter)
	inputSource, _ := args.Get(1).(InputSourceAdapter)

	// If we got nil values but no error was returned, return mock implementations
	if appContract == nil && args.Error(2) == nil {
		appContract = &MockApplicationContract{}
	}

	if inputSource == nil && args.Error(2) == nil {
		inputSource = newMockInputBox()
	}

	return appContract, inputSource, nil, args.Error(2)
}

func (m *MockAdapterFactory) SetupDefaultBehavior(
	appContract1 *MockApplicationContract,
	appContract2 *MockApplicationContract,
	inputBox1 *MockInputBox,
) *MockAdapterFactory {

	// Set up a default behavior that always returns valid non-nil interfaces
	m.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(appContract1, inputBox1, nil).Once()
	m.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(appContract2, nil, nil).Once()
	m.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(appContract1, inputBox1, nil).Once()
	m.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(appContract2, nil, nil).Once()
	m.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(appContract1, inputBox1, nil).Once()
	m.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(appContract2, nil, nil).Once()
	return m
}

func (m *MockAdapterFactory) SetupDefaultBehaviorSingleApp(
	appContract *MockApplicationContract,
	inputBox *MockInputBox) *MockAdapterFactory {
	// Set up a default behavior that always returns valid non-nil interfaces
	m.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(appContract, inputBox, nil)
	return m
}

func newMockAdapterFactory() *MockAdapterFactory {
	return &MockAdapterFactory{}
}
