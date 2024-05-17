package inputreader

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/pkg/contracts/inputbox"
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

//go:embed test_data/input_added_event_0.json
var inputAddedEvent0JsonBytes []byte

//go:embed test_data/input_added_event_1.json
var inputAddedEvent1JsonBytes []byte

//go:embed test_data/header_0.json
var header0JsonBytes []byte

var (
	header0 = types.Header{}

	block0 = types.Block{}

	inputAddedEvent0 = inputbox.InputBoxInputAdded{}
	inputAddedEvent1 = inputbox.InputBoxInputAdded{}

	subscription0 = newMockSubscription()
)

type InputReaderSuite struct {
	suite.Suite
	ctx         context.Context
	cancel      context.CancelFunc
	client      *MockEthClient
	inputBox    *MockInputBox
	repository  *MockRepository
	inputReader *InputReader
}

func TestInputReaderSuite(t *testing.T) {
	suite.Run(t, new(InputReaderSuite))
}

func (s *InputReaderSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), suiteTimeout)

	err := json.Unmarshal(header0JsonBytes, &header0)

	s.Require().Nil(err)

	block0 = *types.NewBlockWithHeader(&header0)

	err = json.Unmarshal(inputAddedEvent0JsonBytes, &inputAddedEvent0)
	s.Require().Nil(err)
	err = json.Unmarshal(inputAddedEvent1JsonBytes, &inputAddedEvent1)
	s.Require().Nil(err)
}

func (s *InputReaderSuite) TearDownSuite() {
	s.cancel()
}

func (s *InputReaderSuite) SetupTest() {

	s.client = newMockEthClient()
	s.inputBox = newMockInputBox(s)
	s.repository = newMockRepository()
	inputReader := NewInputReader(
		s.client,
		s.inputBox,
		s.repository,
		common.MaxAddress,
		0,
		common.MaxAddress,
	)
	s.inputReader = &inputReader
}

// Service tests
func (s *InputReaderSuite) TestItStopsWhenContextIsCanceled() {
	ctx, cancel := context.WithCancel(s.ctx)
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- s.inputReader.Start(ctx, ready)
	}()
	cancel()

	err := <-errChannel
	s.Require().Equal(context.Canceled, err, "stopped for the wrong reason")
}

func (s *InputReaderSuite) TestItEventuallyBecomesReady() {
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- s.inputReader.Start(s.ctx, ready)
	}()

	select {
	case <-ready:
		s.repository.AssertNumberOfCalls(s.T(), "UpdateMostRecentFinalizedBlockNumber", 1)
	case err := <-errChannel:
		s.FailNow("unexpected failure", err)
	}
}

// Initialization tests
func (s *InputReaderSuite) TestItFailsToFetchMostRecentFinalizedHeaderOnStart() {
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, fmt.Errorf("expected failure"))

	go func() {
		errChannel <- s.inputReader.Start(s.ctx, ready)
	}()

	select {
	case <-ready:
		s.FailNow("unexpected ready signal")
	case err := <-errChannel:
		s.Require().ErrorContains(err, "expected failure")
	}
}

func (s *InputReaderSuite) TestItFailsToUpdateMostRecentFinalizedBlockOnStart() {
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	s.repository.Unset("UpdateMostRecentFinalizedBlockNumber")
	s.repository.On(
		"UpdateMostRecentFinalizedBlockNumber",
		mock.Anything,
		mock.Anything,
	).Return(fmt.Errorf("expected failure"))

	go func() {
		errChannel <- s.inputReader.Start(s.ctx, ready)
	}()

	select {
	case <-ready:
		s.FailNow("unexpected ready signal")
	case err := <-errChannel:
		s.Require().EqualError(err, "expected failure")
	}
}

func (s *InputReaderSuite) TestItFailsToReadPastInputsOnStart() {
	s.inputBox.Unset("RetrieveInputs")
	noEvents := []*inputbox.InputBoxInputAdded{}
	s.inputBox.On(
		"RetrieveInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(noEvents, fmt.Errorf("expected failure"))

	s.Require().ErrorContains(
		s.inputReader.Start(s.ctx, make(chan struct{}, 1)),
		"expected failure")
	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 1)
}

func (s *InputReaderSuite) TestItFailsToSubscribeForNewInputsOnStart() {
	s.client.Unset("SubscribeNewHead")
	emptySubscription := &MockSubscription{}
	s.client.On(
		"SubscribeNewHead",
		mock.Anything,
		mock.Anything,
	).Return(emptySubscription, fmt.Errorf("expected failure"))

	s.Require().ErrorContains(
		s.inputReader.Start(s.ctx, make(chan struct{}, 1)),
		"expected failure")
	s.client.AssertNumberOfCalls(s.T(), "SubscribeNewHead", 1)
}

func (s *InputReaderSuite) TestItReadsAllPastInputs() {
	// Set finalized block
	inputReader := NewInputReader(
		s.client,
		s.inputBox,
		s.repository,
		common.MaxAddress,
		0x10,
		common.MaxAddress,
	)

	// Prepare sequence of past inputs
	s.inputBox.Unset("RetrieveInputs")
	events := []*inputbox.InputBoxInputAdded{&inputAddedEvent0}
	currentMostRecentFinalizedBlockNumber := uint64(0x11)
	retrieveInputsOpts := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x10,
		End:     &currentMostRecentFinalizedBlockNumber,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts,
		mock.Anything,
		mock.Anything,
	).Return(events, nil)

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- inputReader.Start(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 1)
	s.repository.AssertNumberOfCalls(s.T(), "InsertInput", 1)

}

func (s *InputReaderSuite) TestItReadsInputsFromNewBlocks() {
	// Set finalized block
	// Prepare a block with a single input to be read by subscription
	// Start service
	// Check if the input was captured correctly
	s.FailNow("not implemented")
}

func (s *InputReaderSuite) TestItReadsMultipleInputsFromSingleNewBlock() {
	// Set finalized block
	// Prepare a block with multiple inputs to be read by subscription
	// Start service
	// Check if all inputs were captured correctly
	s.FailNow("not implemented")
}

func (s *InputReaderSuite) TestItReadsInputsNotCapturedBySubscription() {
	// Set scenario comprising both TestItReadsAllPastInputs and TestItReadsInputsFromNewBlocks
	// Set second finalized block before setting blocks to be captured by subscription
	// Start service
	// Check if inputs from first finalized block to the second one are captured correctly
	s.FailNow("not implemented")
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

func (m MockSubscription) Unsubscribe() {
}

func (m MockSubscription) Err() <-chan error {
	args := m.Called()
	return args.Get(0).(<-chan error)
}

// Mock inputbox.InputBox
type MockInputBox struct {
	mock.Mock
}

func newMockInputBox(s *InputReaderSuite) *MockInputBox {
	inputSource := &MockInputBox{}

	events := []*inputbox.InputBoxInputAdded{&inputAddedEvent0}
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
) ([]*inputbox.InputBoxInputAdded, error) {
	args := m.Called(opts, appContract, index)
	return args.Get(0).([]*inputbox.InputBoxInputAdded), args.Error(1)
}

// Mock InputReaderRepository
type MockRepository struct {
	mock.Mock
}

func newMockRepository() *MockRepository {
	repo := &MockRepository{}

	repo.On("InsertInput",
		mock.Anything,
		mock.Anything).Return(nil)

	repo.On("GetMostRecentFinalizedBlockNumber",
		mock.Anything,
		mock.Anything).Return(uint64(0), nil)
	repo.On("UpdateMostRecentFinalizedBlockNumber",
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

func (m *MockRepository) InsertInput(
	ctx context.Context,
	input model.Input,
) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func (m *MockRepository) GetMostRecentFinalizedBlockNumber(
	ctx context.Context,
) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRepository) UpdateMostRecentFinalizedBlockNumber(
	ctx context.Context,
	number uint64,
) error {
	args := m.Called(ctx, number)
	return args.Error(0)
}
