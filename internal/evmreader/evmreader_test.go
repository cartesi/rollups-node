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

var (
	header0 = types.Header{}
	header1 = types.Header{}
	header2 = types.Header{}

	block0 = types.Block{}

	inputAddedEvent0 = iinputbox.IInputBoxInputAdded{}
	inputAddedEvent1 = iinputbox.IInputBoxInputAdded{}
	inputAddedEvent2 = iinputbox.IInputBoxInputAdded{}
	inputAddedEvent3 = iinputbox.IInputBoxInputAdded{}

	subscription0 = newMockSubscription()
)

type EvmReaderSuite struct {
	suite.Suite
	ctx             context.Context
	cancel          context.CancelFunc
	client          *MockEthClient
	wsClient        *MockEthClient
	repository      *MockRepository
	evmReader       *Service
	contractFactory *MockAdapterFactory
}

func TestEvmReaderSuite(t *testing.T) {
	suite.Run(t, new(EvmReaderSuite))
}

func (s *EvmReaderSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), suiteTimeout)

	err := json.Unmarshal(header0JsonBytes, &header0)
	s.Require().Nil(err)
	err = json.Unmarshal(header1JsonBytes, &header1)
	s.Require().Nil(err)
	err = json.Unmarshal(header2JsonBytes, &header2)
	s.Require().Nil(err)

	block0 = *types.NewBlockWithHeader(&header0)

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

func (me *EvmReaderSuite) SetupTest() {
	me.client = newMockEthClient()
	me.client.On("ChainID", mock.Anything).Return(big.NewInt(1), nil)
	me.wsClient = me.client
	me.repository = newMockRepository()
	me.contractFactory = newMockAdapterFactory()

	me.evmReader = &Service{
		client:             me.client,
		wsClient:           me.wsClient,
		repository:         me.repository,
		defaultBlock:       DefaultBlock_Latest,
		adapterFactory:     me.contractFactory,
		hasEnabledApps:     true,
		inputReaderEnabled: true,
	}
	serviceArgs := &service.CreateInfo{Name: "evm-reader", Impl: me.evmReader}
	err := service.Create(context.Background(), serviceArgs, &me.evmReader.Service)
	me.Require().Nil(err)
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
	s.client.Unset("SubscribeNewHead")
	emptySubscription := &MockSubscription{}
	s.client.On(
		"SubscribeNewHead",
		mock.Anything,
		mock.Anything,
	).Return(emptySubscription, fmt.Errorf("expected failure"))

	s.Require().ErrorContains(
		s.evmReader.Run(s.ctx, make(chan struct{}, 1)),
		"expected failure")
	s.client.AssertNumberOfCalls(s.T(), "SubscribeNewHead", 1)
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
	client := &MockEthClient{}

	client.On("HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil)

	client.On("SubscribeNewHead",
		mock.Anything,
		mock.Anything,
	).Return(subscription0, nil)

	return client
}

func (m *MockEthClient) Unset(methodName string) {
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			call.Unset()
		}
	}
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
	ctx context.Context,
	ch chan<- *types.Header,
) (ethereum.Subscription, error) {
	f.ch = ch
	return newMockSubscription(), nil
}

func (f *FakeWSEhtClient) HeaderByNumber(
	ctx context.Context,
	number *big.Int,
) (*types.Header, error) {
	return &header0, nil
}

func (f *FakeWSEhtClient) ChainID(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (f *FakeWSEhtClient) fireNewHead(header *types.Header) {
	f.ch <- header
}

// Mock inputbox.InputBox
type MockInputBox struct {
	mock.Mock
}

func newMockInputBox() *MockInputBox {
	inputSource := &MockInputBox{}

	events := []iinputbox.IInputBoxInputAdded{inputAddedEvent0}
	inputSource.On("RetrieveInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(events, nil)

	return inputSource
}

func (m *MockInputBox) Unset(methodName string) {
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			call.Unset()
		}
	}
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

func newMockRepository() *MockRepository {
	repo := &MockRepository{}

	repo.On("CreateEpochsAndInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(nil)

	repo.On("GetEpoch",
		mock.Anything,
		mock.Anything,
		uint64(0)).Return(
		&Epoch{
			Index:                0,
			FirstBlock:           0,
			LastBlock:            9,
			Status:               EpochStatus_Open,
			ClaimHash:            nil,
			ClaimTransactionHash: nil,
		}, nil)
	repo.On("GetEpoch",
		mock.Anything,
		mock.Anything,
		uint64(1)).Return(
		&Epoch{
			Index:                1,
			FirstBlock:           10,
			LastBlock:            19,
			Status:               EpochStatus_Open,
			ClaimHash:            nil,
			ClaimTransactionHash: nil,
		}, nil)
	repo.On("GetEpoch",
		mock.Anything,
		mock.Anything,
		uint64(2)).Return(
		&Epoch{
			Index:                2,
			FirstBlock:           20,
			LastBlock:            29,
			Status:               EpochStatus_Open,
			ClaimHash:            nil,
			ClaimTransactionHash: nil,
		}, nil)

	repo.On("ListEpochs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Epoch{}, uint64(0), nil)

	repo.On("UpdateOutputsExecution",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(nil)

	outputHash := common.HexToHash("0xAABBCCDDEE")
	repo.On("GetOutput",
		mock.Anything,
		common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E").String(),
		0).Return(
		&Output{
			Index:                    0,
			RawData:                  common.Hex2Bytes("0xdeadbeef"),
			Hash:                     &outputHash,
			InputIndex:               1,
			OutputHashesSiblings:     nil,
			ExecutionTransactionHash: nil,
		},
	)

	return repo

}

func (m *MockRepository) Unset(methodName string) {
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			call.Unset()
		}
	}
}

func (m *MockRepository) CreateEpochAndInputs(
	ctx context.Context,
	nameOrAddress string,
	epochInputMap map[*Epoch][]*Input,
	blockNumber uint64,
) (err error) {
	args := m.Called(ctx, nameOrAddress, epochInputMap, blockNumber)
	return args.Error(0)
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

type MockApplicationContract struct {
	mock.Mock
}

func (m *MockApplicationContract) Unset(methodName string) {
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			call.Unset()
		}
	}
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

type MockAdapterFactory struct {
	mock.Mock
}

func (m *MockAdapterFactory) Unset(methodName string) {
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			call.Unset()
		}
	}
}

func (m *MockAdapterFactory) CreateAdapters(
	app *Application,
	client EthClientInterface,
) (ApplicationContractAdapter, InputSourceAdapter, error) {
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

	return appContract, inputSource, args.Error(2)
}

func newMockAdapterFactory() *MockAdapterFactory {
	applicationContract := &MockApplicationContract{}
	applicationContract.On("RetrieveOutputExecutionEvents",
		mock.Anything,
	).Return([]*iapplication.IApplicationOutputExecuted{}, nil)

	inputBox := newMockInputBox()
	inputBox.On("RetrieveInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{}, nil)

	factory := &MockAdapterFactory{}
	// Set up a default behavior that always returns valid non-nil interfaces
	factory.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(applicationContract, inputBox, nil)

	return factory
}
