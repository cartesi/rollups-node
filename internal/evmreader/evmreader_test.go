// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	_ "embed"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	. "github.com/cartesi/rollups-node/internal/node/model"
	appcontract "github.com/cartesi/rollups-node/pkg/contracts/application"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
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
)

type EvmReaderSuite struct {
	suite.Suite
	ctx             context.Context
	cancel          context.CancelFunc
	client          *MockEthClient
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
	s.inputBox = newMockInputBox()
	s.repository = newMockRepository()
	s.contractFactory = newEmvReaderContractFactory()
	s.evmReader = NewEvmReader(
		s.client,
		s.inputBox,
		s.repository,
		0,
		DefaultBlockStatusLatest,
		s.contractFactory,
	)

}

// Evm tests
func (s *EvmReaderSuite) TestItWrongIConsensus() {

	consensusContract := &MockIConsensusContract{}

	contractFactory := newEmvReaderContractFactory()

	contractFactory.Unset("NewIConsensus")
	contractFactory.On("NewIConsensus",
		mock.Anything,
	).Return(consensusContract, nil)

	evmReader := NewEvmReader(
		s.client,
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

	// Run
	err := evmReader.Step(s.ctx)
	s.Require().Nil(err)

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

// Mock inputbox.InputBox
type MockInputBox struct {
	mock.Mock
}

func newMockInputBox() *MockInputBox {
	inputSource := &MockInputBox{}

	events := []inputbox.InputBoxInputAdded{inputAddedEvent0}
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
) ([]inputbox.InputBoxInputAdded, error) {
	args := m.Called(opts, appContract, index)
	return args.Get(0).([]inputbox.InputBoxInputAdded), args.Error(1)
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

	repo.On("GetEpochsWithOpenClaims",
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

func (m *MockRepository) GetEpochsWithOpenClaims(
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
) ([]*appcontract.ApplicationOutputExecuted, error) {
	args := m.Called(opts)
	return args.Get(0).([]*appcontract.ApplicationOutputExecuted), args.Error(1)
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
		mock.Anything).Return([]*appcontract.ApplicationOutputExecuted{}, nil)

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
