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

	. "github.com/cartesi/rollups-node/internal/node/model"
	appcontract "github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
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
	inputBox        *MockInputBox
	repository      *MockRepository
	evmReader       *EvmReader
	contractFactory *MockEvmReaderContractFactory
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

func (s *EvmReaderSuite) SetupTest() {

	s.client = newMockEthClient()
	s.wsClient = s.client
	s.inputBox = newMockInputBox()
	s.repository = newMockRepository()
	s.contractFactory = newEmvReaderContractFactory()
	inputReader := NewEvmReader(
		s.client,
		s.wsClient,
		s.inputBox,
		s.repository,
		0,
		DefaultBlockStatusLatest,
		s.contractFactory,
	)
	s.evmReader = &inputReader
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

func (s *EvmReaderSuite) TestItWrongIConsensus() {

	consensusContract := &MockIConsensusContract{}

	contractFactory := newEmvReaderContractFactory()

	contractFactory.Unset("NewIConsensus")
	contractFactory.On("NewIConsensus",
		mock.Anything,
	).Return(consensusContract, nil)

	wsClient := FakeWSEhtClient{}

	evmReader := NewEvmReader(
		s.client,
		&wsClient,
		s.inputBox,
		s.repository,
		0x10,
		DefaultBlockStatusLatest,
		contractFactory,
	)

	// Prepare consensus
	claimEvent0 := &iconsensus.IConsensusClaimAcceptance{
		AppContract:              common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		LastProcessedBlockNumber: big.NewInt(3),
		Claim:                    common.HexToHash("0xdeadbeef"),
	}

	claimEvents := []*iconsensus.IConsensusClaimAcceptance{claimEvent0}
	consensusContract.On("RetrieveClaimAcceptanceEvents",
		mock.Anything,
		mock.Anything,
	).Return(claimEvents, nil).Once()

	// Prepare repository
	s.repository.Unset("GetAllRunningApplications")
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:  common.HexToAddress("0xFFFFFFFF"),
		LastProcessedBlock: 0x00,
	}}, nil).Once()

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil).Once()

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	wsClient.fireNewHead(&header0)
	time.Sleep(time.Second)

	// Should not advance input processing
	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 0)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"StoreEpochAndInputsTransaction",
		0,
	)

	// Should not advance claim acceptance processing
	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveClaimAcceptanceEvents", 0)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"UpdateEpochs",
		0,
	)
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

// Mock InputReaderRepository
type MockRepository struct {
	mock.Mock
}

func newMockRepository() *MockRepository {
	repo := &MockRepository{}

	repo.On("StoreEpochAndInputsTransaction",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(make(map[uint64]uint64), make(map[uint64][]uint64), nil)

	repo.On("GetEpoch",
		mock.Anything,
		uint64(0),
		mock.Anything).Return(
		&Epoch{
			Id:              1,
			Index:           0,
			FirstBlock:      0,
			LastBlock:       9,
			Status:          EpochStatusOpen,
			AppAddress:      common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
			ClaimHash:       nil,
			TransactionHash: nil,
		}, nil)
	repo.On("GetEpoch",
		mock.Anything,
		uint64(1),
		mock.Anything).Return(
		&Epoch{
			Id:              2,
			Index:           1,
			FirstBlock:      10,
			LastBlock:       19,
			Status:          EpochStatusOpen,
			AppAddress:      common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
			ClaimHash:       nil,
			TransactionHash: nil,
		}, nil)
	repo.On("GetEpoch",
		mock.Anything,
		uint64(2),
		mock.Anything).Return(
		&Epoch{
			Id:              3,
			Index:           2,
			FirstBlock:      20,
			LastBlock:       29,
			Status:          EpochStatusOpen,
			AppAddress:      common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
			ClaimHash:       nil,
			TransactionHash: nil,
		}, nil)

	repo.On("InsertEpoch",
		mock.Anything,
		mock.Anything).Return(1, nil)

	repo.On("GetPreviousEpochsWithOpenClaims",
		mock.Anything,
		mock.Anything,
	).Return([]Epoch{}, nil)

	repo.On("UpdateEpochs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nil)

	repo.On("UpdateOutputExecutionTransaction",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(nil)

	outputHash := common.HexToHash("0xAABBCCDDEE")
	repo.On("GetOutput",
		mock.Anything,
		0,
		common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E")).Return(
		&Output{
			Id:                   1,
			Index:                0,
			RawData:              common.Hex2Bytes("0xdeadbeef"),
			Hash:                 &outputHash,
			InputId:              1,
			OutputHashesSiblings: nil,
			TransactionHash:      nil,
		},
	)

	return repo

}

func (s *EvmReaderSuite) TestIndexApps() {

	s.Run("Ok", func() {
		apps := []application{
			{Application: Application{LastProcessedBlock: 23}},
			{Application: Application{LastProcessedBlock: 22}},
			{Application: Application{LastProcessedBlock: 21}},
			{Application: Application{LastProcessedBlock: 23}},
		}

		keyByProcessedBlock := func(a application) uint64 {
			return a.LastProcessedBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(3, len(indexApps))
		apps, ok := indexApps[23]
		s.Require().True(ok)
		s.Require().Equal(2, len(apps))
	})

	s.Run("whenIndexAppsArrayEmpty", func() {
		apps := []application{}

		keyByProcessedBlock := func(a application) uint64 {
			return a.LastProcessedBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(0, len(indexApps))
	})

	s.Run("whenIndexAppsArray", func() {
		apps := []application{}

		keyByProcessedBlock := func(a application) uint64 {
			return a.LastProcessedBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(0, len(indexApps))
	})

	s.Run("whenIndexByEmptyKey", func() {
		apps := []application{
			{Application: Application{LastProcessedBlock: 23}},
			{Application: Application{LastProcessedBlock: 22}},
			{Application: Application{LastProcessedBlock: 21}},
			{Application: Application{LastProcessedBlock: 23}},
		}

		keyByIConsensus := func(a application) ConsensusContract {
			return a.consensusContract
		}

		indexApps := indexApps(keyByIConsensus, apps)

		s.Require().Equal(1, len(indexApps))
		apps, ok := indexApps[nil]
		s.Require().True(ok)
		s.Require().Equal(4, len(apps))
	})

	s.Run("whenUsesWrongKey", func() {
		apps := []application{
			{Application: Application{LastProcessedBlock: 23}},
			{Application: Application{LastProcessedBlock: 22}},
			{Application: Application{LastProcessedBlock: 21}},
			{Application: Application{LastProcessedBlock: 23}},
		}

		keyByProcessedBlock := func(a application) uint64 {
			return a.LastProcessedBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(3, len(indexApps))
		apps, ok := indexApps[0]
		s.Require().False(ok)
		s.Require().Nil(apps)

	})

}

func (m *MockRepository) Unset(methodName string) {
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			call.Unset()
		}
	}
}

func (m *MockRepository) StoreEpochAndInputsTransaction(
	ctx context.Context,
	epochInputMap map[*Epoch][]Input,
	blockNumber uint64,
	appAddress common.Address,
) (epochIndexIdMap map[uint64]uint64, epochIndexInputIdsMap map[uint64][]uint64, err error) {
	args := m.Called(ctx, epochInputMap, blockNumber, appAddress)
	return args.Get(0).(map[uint64]uint64), args.Get(1).(map[uint64][]uint64), args.Error(2)
}

func (m *MockRepository) GetAllRunningApplications(
	ctx context.Context,
) ([]Application, error) {
	args := m.Called(ctx)
	return args.Get(0).([]Application), args.Error(1)
}

func (m *MockRepository) GetNodeConfig(
	ctx context.Context,
) (*NodePersistentConfig, error) {
	args := m.Called(ctx)
	return args.Get(0).(*NodePersistentConfig), args.Error(1)
}

func (m *MockRepository) GetEpoch(
	ctx context.Context,
	index uint64,
	appAddress common.Address,
) (*Epoch, error) {
	args := m.Called(ctx, index, appAddress)
	obj := args.Get(0)
	if obj == nil {
		return nil, args.Error(1)
	}
	return obj.(*Epoch), args.Error(1)
}

func (m *MockRepository) InsertEpoch(
	ctx context.Context,
	epoch *Epoch,
) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRepository) GetPreviousEpochsWithOpenClaims(
	ctx context.Context,
	app Address,
	lastBlock uint64,
) ([]*Epoch, error) {
	args := m.Called(ctx, app, lastBlock)
	obj := args.Get(0)
	if obj == nil {
		return nil, args.Error(1)
	}
	return obj.([]*Epoch), args.Error(1)

}
func (m *MockRepository) UpdateEpochs(ctx context.Context,
	app Address,
	epochs []*Epoch,
	mostRecentBlockNumber uint64,
) error {
	args := m.Called(ctx, epochs, mostRecentBlockNumber)
	return args.Error(0)
}

func (m *MockRepository) GetOutput(
	ctx context.Context, indexKey uint64, appAddressKey Address,
) (*Output, error) {
	args := m.Called(ctx, indexKey, appAddressKey)
	obj := args.Get(0)
	if obj == nil {
		return nil, args.Error(1)
	}
	return obj.(*Output), args.Error(1)
}

func (m *MockRepository) UpdateOutputExecutionTransaction(
	ctx context.Context, app Address, executedOutputs []*Output, blockNumber uint64,
) error {
	args := m.Called(ctx, app, executedOutputs, blockNumber)
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

func (m *MockApplicationContract) GetConsensus(
	opts *bind.CallOpts,
) (common.Address, error) {
	args := m.Called(opts)
	return args.Get(0).(common.Address), args.Error(1)
}

func (m *MockApplicationContract) RetrieveOutputExecutionEvents(
	opts *bind.FilterOpts,
) ([]*appcontract.IApplicationOutputExecuted, error) {
	args := m.Called(opts)
	return args.Get(0).([]*appcontract.IApplicationOutputExecuted), args.Error(1)
}

type MockIConsensusContract struct {
	mock.Mock
}

func (m *MockIConsensusContract) GetEpochLength(
	opts *bind.CallOpts,
) (*big.Int, error) {
	args := m.Called(opts)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *MockIConsensusContract) RetrieveClaimAcceptanceEvents(
	opts *bind.FilterOpts, appAddresses []common.Address,
) ([]*iconsensus.IConsensusClaimAcceptance, error) {
	args := m.Called(opts, appAddresses)
	return args.Get(0).([]*iconsensus.IConsensusClaimAcceptance), args.Error(1)
}

type MockEvmReaderContractFactory struct {
	mock.Mock
}

func (m *MockEvmReaderContractFactory) Unset(methodName string) {
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			call.Unset()
		}
	}
}

func (m *MockEvmReaderContractFactory) NewApplication(
	Address,
) (ApplicationContract, error) {
	args := m.Called(context.Background())
	return args.Get(0).(ApplicationContract), args.Error(1)
}

func (m *MockEvmReaderContractFactory) NewIConsensus(
	Address,
) (ConsensusContract, error) {
	args := m.Called(context.Background())
	return args.Get(0).(ConsensusContract), args.Error(1)
}

func newEmvReaderContractFactory() *MockEvmReaderContractFactory {

	applicationContract := &MockApplicationContract{}

	applicationContract.On("GetConsensus",
		mock.Anything,
	).Return(common.HexToAddress("0xdeadbeef"), nil)

	applicationContract.On("RetrieveOutputExecutionEvents",
		mock.Anything).Return([]*appcontract.IApplicationOutputExecuted{}, nil)

	consensusContract := &MockIConsensusContract{}

	consensusContract.On("GetEpochLength",
		mock.Anything).Return(big.NewInt(10), nil)

	consensusContract.On("RetrieveClaimAcceptanceEvents",
		mock.Anything,
		mock.Anything,
	).Return([]*iconsensus.IConsensusClaimAcceptance{}, nil)

	factory := &MockEvmReaderContractFactory{}

	factory.On("NewApplication",
		mock.Anything,
	).Return(applicationContract, nil)

	factory.On("NewIConsensus",
		mock.Anything).Return(consensusContract, nil)

	return factory
}
