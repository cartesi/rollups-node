// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/pkg/contracts/inputbox"
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

	inputAddedEvent0 = inputbox.InputBoxInputAdded{}
	inputAddedEvent1 = inputbox.InputBoxInputAdded{}
	inputAddedEvent2 = inputbox.InputBoxInputAdded{}
	inputAddedEvent3 = inputbox.InputBoxInputAdded{}

	subscription0 = newMockSubscription()
)

type EvmReaderSuite struct {
	suite.Suite
	ctx        context.Context
	cancel     context.CancelFunc
	client     *MockEthClient
	wsClient   *MockEthClient
	inputBox   *MockInputBox
	repository *MockRepository
	evmReader  *EvmReader
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
	s.inputBox = newMockInputBox(s)
	s.repository = newMockRepository()
	inputReader := NewEvmReader(
		s.client,
		s.wsClient,
		s.inputBox,
		s.repository,
		NodePersistentConfig{
			DefaultBlock:            model.DefaultBlockStatusLatest,
			InputBoxDeploymentBlock: 0,
		},
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

func (s *EvmReaderSuite) TestItReadsInputsFromNewBlocks() {

	waitGroup := sync.WaitGroup{}
	wsClient := FakeWSEhtClient{}
	wsClient.NewHeaders = []*Header{&header0, &header1}
	wsClient.WaitGroup = &waitGroup
	inputReader := NewEvmReader(
		s.client,
		&wsClient,
		s.inputBox,
		s.repository,
		model.NodePersistentConfig{
			InputBoxDeploymentBlock: 0x10,
			DefaultBlock:            model.DefaultBlockStatusLatest,
		},
	)

	// Prepare repository
	s.repository.Unset("GetAllRunningApplications")
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		LastProcessedBlock: 0x00,
	}}, nil).Once()
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		LastProcessedBlock: 0x11,
	}}, nil).Once()

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil).Once()
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header1, nil).Once()
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header2, nil).Once()

	// Prepare sequence of inputs
	s.inputBox.Unset("RetrieveInputs")
	events_0 := []InputBoxInputAdded{inputAddedEvent0}
	currentMostRecentFinalizedBlockNumber_0 := uint64(0x11)
	retrieveInputsOpts_0 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x10,
		End:     &currentMostRecentFinalizedBlockNumber_0,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_0,
		mock.Anything,
		mock.Anything,
	).Return(events_0, nil)

	events_1 := []InputBoxInputAdded{inputAddedEvent1}
	currentMostRecentFinalizedBlockNumber_1 := uint64(0x12)
	retrieveInputsOpts_1 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x12,
		End:     &currentMostRecentFinalizedBlockNumber_1,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_1,
		mock.Anything,
		mock.Anything,
	).Return(events_1, nil)

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	waitGroup.Add(1)
	go func() {
		errChannel <- inputReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	waitGroup.Wait()

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 2)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"InsertInputsAndUpdateLastProcessedBlock",
		2,
	)
}

func (s *EvmReaderSuite) TestItUpdatesLastProcessedBlockWhenThereIsNoInputs() {

	waitGroup := sync.WaitGroup{}
	wsClient := FakeWSEhtClient{}
	wsClient.NewHeaders = []*Header{&header0, &header1}
	wsClient.WaitGroup = &waitGroup
	inputReader := NewEvmReader(
		s.client,
		&wsClient,
		s.inputBox,
		s.repository,
		model.NodePersistentConfig{
			InputBoxDeploymentBlock: 0x10,
			DefaultBlock:            model.DefaultBlockStatusLatest,
		},
	)

	// Prepare repository
	s.repository.Unset("GetAllRunningApplications")
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		LastProcessedBlock: 0x00,
	}}, nil).Once()
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		LastProcessedBlock: 0x11,
	}}, nil).Once()

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil).Once()
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header1, nil).Once()
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header2, nil).Once()

	// Prepare sequence of inputs
	s.inputBox.Unset("RetrieveInputs")
	events_0 := []InputBoxInputAdded{}
	currentMostRecentFinalizedBlockNumber_0 := uint64(0x11)
	retrieveInputsOpts_0 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x10,
		End:     &currentMostRecentFinalizedBlockNumber_0,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_0,
		mock.Anything,
		mock.Anything,
	).Return(events_0, nil)

	events_1 := []InputBoxInputAdded{}
	currentMostRecentFinalizedBlockNumber_1 := uint64(0x12)
	retrieveInputsOpts_1 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x12,
		End:     &currentMostRecentFinalizedBlockNumber_1,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_1,
		mock.Anything,
		mock.Anything,
	).Return(events_1, nil)

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	waitGroup.Add(1)
	go func() {
		errChannel <- inputReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	waitGroup.Wait()

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 2)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"InsertInputsAndUpdateLastProcessedBlock",
		2,
	)
}

func (s *EvmReaderSuite) TestItReadsMultipleInputsFromSingleNewBlock() {

	waitGroup := sync.WaitGroup{}
	wsClient := FakeWSEhtClient{}
	wsClient.NewHeaders = []*Header{&header2}
	wsClient.WaitGroup = &waitGroup
	inputReader := NewEvmReader(
		s.client,
		&wsClient,
		s.inputBox,
		s.repository,
		model.NodePersistentConfig{
			InputBoxDeploymentBlock: 0x10,
			DefaultBlock:            model.DefaultBlockStatusLatest,
		},
	)

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header2, nil).Once()

	// Prepare sequence of inputs
	s.inputBox.Unset("RetrieveInputs")
	events_2 := []inputbox.InputBoxInputAdded{inputAddedEvent2, inputAddedEvent3}
	currentMostRecentFinalizedBlockNumber_2 := uint64(0x13)
	retrieveInputsOpts_2 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x13,
		End:     &currentMostRecentFinalizedBlockNumber_2,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_2,
		mock.Anything,
		mock.Anything,
	).Return(events_2, nil)

	// Prepare Repo
	s.repository.Unset("GetAllRunningApplications")
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		LastProcessedBlock: 0x12,
	}}, nil).Once()
	s.repository.Unset("InsertInputsAndUpdateLastProcessedBlock")
	s.repository.On(
		"InsertInputsAndUpdateLastProcessedBlock",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Once().Run(func(arguments mock.Arguments) {
		var inputs []model.Input
		obj := arguments.Get(1)
		inputs, ok := obj.([]model.Input)
		s.Require().True(ok)
		s.Assert().Equal(2, len(inputs))
	}).Return(nil)

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	waitGroup.Add(1)
	go func() {
		errChannel <- inputReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	waitGroup.Wait()

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 1)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"InsertInputsAndUpdateLastProcessedBlock",
		1,
	)
}

func (s *EvmReaderSuite) TestItStartsWhenLasProcessedBlockIsTheMostRecentBlock() {

	waitGroup := sync.WaitGroup{}
	wsClient := FakeWSEhtClient{}
	wsClient.NewHeaders = []*Header{&header2}
	wsClient.WaitGroup = &waitGroup
	inputReader := NewEvmReader(
		s.client,
		&wsClient,
		s.inputBox,
		s.repository,
		model.NodePersistentConfig{
			InputBoxDeploymentBlock: 0x10,
			DefaultBlock:            model.DefaultBlockStatusLatest,
		},
	)

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil).Once()

	// Prepare Repo
	s.repository.Unset("GetAllRunningApplications")
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		LastProcessedBlock: 0x11,
	}}, nil).Once()

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	waitGroup.Add(1)
	go func() {
		errChannel <- inputReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	waitGroup.Wait()

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 0)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"InsertInputsAndUpdateLastProcessedBlock",
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
) (*Header, error) {
	args := m.Called(ctx, number)
	return args.Get(0).(*Header), args.Error(1)
}

func (m *MockEthClient) SubscribeNewHead(
	ctx context.Context,
	ch chan<- *Header,
) (Subscription, error) {
	args := m.Called(ctx, ch)
	return args.Get(0).(Subscription), args.Error(1)
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
	NewHeaders []*Header
	WaitGroup  *sync.WaitGroup
}

func (f *FakeWSEhtClient) SubscribeNewHead(
	ctx Context,
	ch chan<- *Header,
) (Subscription, error) {
	go func() {

		for _, header := range f.NewHeaders {
			ch <- header
		}
		//Give some time to headers to be processed
		time.Sleep(1 * time.Second)
		f.WaitGroup.Done()
	}()
	return newMockSubscription(), nil
}

// Mock inputbox.InputBox
type MockInputBox struct {
	mock.Mock
}

func newMockInputBox(s *EvmReaderSuite) *MockInputBox {
	inputSource := &MockInputBox{}

	events := []InputBoxInputAdded{inputAddedEvent0}
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
	opts *FilterOpts,
	appContract []Address,
	index []*big.Int,
) ([]InputBoxInputAdded, error) {
	args := m.Called(opts, appContract, index)
	return args.Get(0).([]InputBoxInputAdded), args.Error(1)
}

// Mock InputReaderRepository
type MockRepository struct {
	mock.Mock
}

func newMockRepository() *MockRepository {
	repo := &MockRepository{}

	repo.On("GetMostRecentlyFinalizedBlock",
		mock.Anything,
		mock.Anything).Return(uint64(0), nil)
	repo.On("InsertInputsAndUpdateLastProcessedBlock",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(nil)

	return repo
}

func (m *MockRepository) Unset(methodName string) {
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			call.Unset()
		}
	}
}

func (m *MockRepository) InsertInputsAndUpdateLastProcessedBlock(
	ctx Context,
	inputs []Input,
	blockNumber uint64,
	appAddress Address,
) error {
	args := m.Called(ctx, inputs, blockNumber)
	return args.Error(0)
}

func (m *MockRepository) GetAllRunningApplications(
	ctx Context,
) ([]Application, error) {
	args := m.Called(ctx)
	return args.Get(0).([]Application), args.Error(1)
}

func (m *MockRepository) GetNodeConfig(
	ctx Context,
) (*NodePersistentConfig, error) {
	args := m.Called(ctx)
	return args.Get(0).(*NodePersistentConfig), args.Error(1)
}
