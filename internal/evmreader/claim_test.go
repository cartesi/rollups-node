// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"fmt"
	"math/big"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
)

func (s *EvmReaderSuite) TestNoClaimsAcceptance() {
	wsClient := FakeWSEhtClient{}
	s.evmReader.wsClient = &wsClient
	s.evmReader.inputBoxDeploymentBlock = 0x10

	// Prepare repository
	s.repository.Unset("ListApplications")
	s.repository.On(
		"ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]*Application{{
		IApplicationAddress: common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		EpochLength:         10,
		LastClaimCheckBlock: 0x10,
	}}, nil).Once()
	s.repository.On(
		"ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]*Application{{
		IApplicationAddress: common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		EpochLength:         10,
		LastClaimCheckBlock: 0x11,
	}}, nil).Once()

	s.repository.Unset("UpdateEpochsClaimAccepted")
	s.repository.On("UpdateEpochsClaimAccepted",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Once().Run(func(arguments mock.Arguments) {
		obj := arguments.Get(1)
		claims, ok := obj.([]*Epoch)
		s.Require().True(ok)
		s.Require().Equal(0, len(claims))

		obj = arguments.Get(2)
		lastClaimCheck, ok := obj.(uint64)
		s.Require().True(ok)
		s.Require().Equal(uint64(17), lastClaimCheck)

	}).Return(nil)
	s.repository.On("UpdateEpochsClaimAccepted",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Once().Run(func(arguments mock.Arguments) {
		obj := arguments.Get(1)
		claims, ok := obj.([]*Epoch)
		s.Require().True(ok)
		s.Require().Equal(0, len(claims))

		obj = arguments.Get(2)
		lastClaimCheck, ok := obj.(uint64)
		s.Require().True(ok)
		s.Require().Equal(uint64(18), lastClaimCheck)

	}).Return(nil)

	//No Inputs
	s.inputBox.Unset("RetrieveInputs")
	s.inputBox.On("RetrieveInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{}, nil)

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

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- s.evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	wsClient.fireNewHead(&header0)
	wsClient.fireNewHead(&header1)
	time.Sleep(1 * time.Second)

	s.repository.AssertNumberOfCalls(
		s.T(),
		"UpdateEpochsClaimAccepted",
		0,
	)

}

func (s *EvmReaderSuite) TestReadClaimAcceptance() {

	appAddress := common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E")

	// Contract Factory

	consensusContract := &MockIConsensusContract{}

	contractFactory := newEmvReaderContractFactory()

	contractFactory.Unset("NewIConsensus")
	contractFactory.On("NewIConsensus",
		mock.Anything,
	).Return(consensusContract, nil)

	//New EVM Reader
	wsClient := FakeWSEhtClient{}
	s.evmReader.wsClient = &wsClient
	s.evmReader.contractFactory = contractFactory

	// Prepare Claims Acceptance Events

	claimEvent0 := &iconsensus.IConsensusClaimAcceptance{
		AppContract:              appAddress,
		LastProcessedBlockNumber: big.NewInt(3),
		Claim:                    common.HexToHash("0xdeadbeef"),
	}

	claimEvents := []*iconsensus.IConsensusClaimAcceptance{claimEvent0}
	consensusContract.On("RetrieveClaimAcceptanceEvents",
		mock.Anything,
		mock.Anything,
	).Return(claimEvents, nil).Once()
	consensusContract.On("RetrieveClaimAcceptanceEvents",
		mock.Anything,
		mock.Anything,
	).Return([]*iconsensus.IConsensusClaimAcceptance{}, nil)

	// Epoch Length
	consensusContract.On("GetEpochLength",
		mock.Anything,
	).Return(big.NewInt(1), nil).Once()

	// Prepare repository
	s.repository.Unset("ListApplications")
	s.repository.On(
		"ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]*Application{{
		IApplicationAddress: appAddress,
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		EpochLength:         10,
		LastClaimCheckBlock: 0x10,
	}}, nil).Once()
	s.repository.On(
		"ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]*Application{{
		IApplicationAddress: appAddress,
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		EpochLength:         10,
		LastClaimCheckBlock: 0x11,
	}}, nil).Once()

	claim1Hash := common.HexToHash("0xdeadbeef")
	claim0 := &Epoch{
		Index:      3,
		FirstBlock: 3,
		LastBlock:  3,
		Status:     EpochStatus_ClaimSubmitted,
		ClaimHash:  &claim1Hash,
	}

	s.repository.Unset("GetEpoch")
	s.repository.On("GetEpoch",
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(claim0, nil)

	s.repository.Unset("ListEpochs")
	s.repository.On("ListEpochs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]*Epoch{}, nil)

	s.repository.Unset("UpdateEpochsClaimAccepted")
	s.repository.On("UpdateEpochsClaimAccepted",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Once().Run(func(arguments mock.Arguments) {
		obj := arguments.Get(2)
		claims, ok := obj.([]*Epoch)
		s.Require().True(ok)
		s.Require().Equal(1, len(claims))
		claim0 := claims[0]
		s.Require().Equal(uint64(3), claim0.LastBlock)
		s.Require().Equal(EpochStatus_ClaimAccepted, claim0.Status)

	}).Return(nil)

	//No Inputs
	s.inputBox.Unset("RetrieveInputs")
	s.inputBox.On("RetrieveInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{}, nil)

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
		errChannel <- s.evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	wsClient.fireNewHead(&header0)
	time.Sleep(10 * time.Second)

	s.repository.AssertNumberOfCalls(
		s.T(),
		"UpdateEpochsClaimAccepted",
		1,
	)

}

func (s *EvmReaderSuite) TestCheckClaimFails() {
	s.Run("whenRetrievePreviousEpochsFails", func() {

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		appAddress := common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E")

		// Contract Factory

		consensusContract := &MockIConsensusContract{}

		contractFactory := newEmvReaderContractFactory()

		contractFactory.Unset("NewIConsensus")
		contractFactory.On("NewIConsensus",
			mock.Anything,
		).Return(consensusContract, nil)

		//New EVM Reader
		client := newMockEthClient()
		inputBox := newMockInputBox()
		repository := newMockRepository()
		wsClient := &FakeWSEhtClient{}
		s.evmReader.client = client
		s.evmReader.wsClient = wsClient
		s.evmReader.inputSource = inputBox
		s.evmReader.repository = repository

		// Prepare Claims Acceptance Events

		claimEvent0 := &iconsensus.IConsensusClaimAcceptance{
			AppContract:              appAddress,
			LastProcessedBlockNumber: big.NewInt(3),
			Claim:                    common.HexToHash("0xdeadbeef"),
		}

		claimEvents := []*iconsensus.IConsensusClaimAcceptance{claimEvent0}
		consensusContract.On("RetrieveClaimAcceptanceEvents",
			mock.Anything,
			mock.Anything,
		).Return(claimEvents, nil).Once()
		consensusContract.On("RetrieveClaimAcceptanceEvents",
			mock.Anything,
			mock.Anything,
		).Return([]*iconsensus.IConsensusClaimAcceptance{}, nil)

		// Epoch Length
		consensusContract.On("GetEpochLength",
			mock.Anything,
		).Return(big.NewInt(1), nil).Once()

		// Prepare repository
		repository.Unset("ListApplications")
		repository.On(
			"ListApplications",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Application{{
			IApplicationAddress: appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			EpochLength:         10,
			LastClaimCheckBlock: 0x10,
		}}, nil).Once()
		repository.On(
			"ListApplications",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Application{{
			IApplicationAddress: appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			EpochLength:         10,
			LastClaimCheckBlock: 0x11,
		}}, nil).Once()

		claim1Hash := common.HexToHash("0xdeadbeef")
		claim1 := &Epoch{
			Index:      3,
			FirstBlock: 3,
			LastBlock:  3,
			Status:     EpochStatus_ClaimSubmitted,
			ClaimHash:  &claim1Hash,
		}

		repository.Unset("GetEpoch")
		repository.On("GetEpoch",
			mock.Anything,
			mock.Anything,
			mock.Anything).Return(claim1, nil)

		repository.Unset("ListEpochs")
		repository.On("ListEpochs",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Epoch{}, fmt.Errorf("No previous epochs for you"))

		repository.Unset("UpdateEpochsClaimAccepted")
		repository.On("UpdateEpochsClaimAccepted",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return(nil)

		//No Inputs
		inputBox.Unset("RetrieveInputs")
		inputBox.On("RetrieveInputs",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]iinputbox.IInputBoxInputAdded{}, nil)

		// Prepare Client
		client.Unset("HeaderByNumber")
		client.On(
			"HeaderByNumber",
			mock.Anything,
			mock.Anything,
		).Return(&header0, nil).Once()

		// Start service
		ready := make(chan struct{}, 1)
		errChannel := make(chan error, 1)

		go func() {
			errChannel <- s.evmReader.Run(ctx, ready)
		}()

		select {
		case <-ready:
			break
		case err := <-errChannel:
			s.FailNow("unexpected error signal", err)
		}

		wsClient.fireNewHead(&header0)
		time.Sleep(1 * time.Second)

		repository.AssertNumberOfCalls(
			s.T(),
			"UpdateEpochsClaimAccepted",
			0,
		)

	})

	s.Run("whenGetEpochsFails", func() {

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		appAddress := common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E")

		// Contract Factory

		consensusContract := &MockIConsensusContract{}

		contractFactory := newEmvReaderContractFactory()

		contractFactory.Unset("NewIConsensus")
		contractFactory.On("NewIConsensus",
			mock.Anything,
		).Return(consensusContract, nil)

		//New EVM Reader
		client := newMockEthClient()
		wsClient := FakeWSEhtClient{}
		inputBox := newMockInputBox()
		repository := newMockRepository()
		evmReader := Service{
			client:                  client,
			wsClient:                &wsClient,
			inputSource:             inputBox,
			repository:              repository,
			inputBoxDeploymentBlock: 0x00,
			defaultBlock:            DefaultBlock_Latest,
			contractFactory:         contractFactory,
			hasEnabledApps:          true,
		}
		Create(&CreateInfo{
			MaxStartupTime: 5 * time.Second,
			CreateInfo: service.CreateInfo{
				Name: "evm-reader",
				Impl: &evmReader,
			},
		}, &evmReader)

		// Prepare Claims Acceptance Events

		claimEvent0 := &iconsensus.IConsensusClaimAcceptance{
			AppContract:              appAddress,
			LastProcessedBlockNumber: big.NewInt(3),
			Claim:                    common.HexToHash("0xdeadbeef"),
		}

		claimEvents := []*iconsensus.IConsensusClaimAcceptance{claimEvent0}
		consensusContract.On("RetrieveClaimAcceptanceEvents",
			mock.Anything,
			mock.Anything,
		).Return(claimEvents, nil).Once()
		consensusContract.On("RetrieveClaimAcceptanceEvents",
			mock.Anything,
			mock.Anything,
		).Return([]*iconsensus.IConsensusClaimAcceptance{}, nil)

		// Epoch Length
		consensusContract.On("GetEpochLength",
			mock.Anything,
		).Return(big.NewInt(1), nil).Once()

		// Prepare repository
		repository.Unset("ListApplications")
		repository.On(
			"ListApplications",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Application{{
			IApplicationAddress: appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			EpochLength:         10,
			LastClaimCheckBlock: 0x10,
		}}, nil).Once()
		repository.On(
			"ListApplications",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Application{{
			IApplicationAddress: appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			EpochLength:         10,
			LastClaimCheckBlock: 0x11,
		}}, nil).Once()

		claim0Hash := common.HexToHash("0xdeadbeef")
		claim0 := &Epoch{
			Index:      1,
			FirstBlock: 1,
			LastBlock:  1,
			Status:     EpochStatus_ClaimSubmitted,
			ClaimHash:  &claim0Hash,
		}

		repository.Unset("GetEpoch")
		repository.On("GetEpoch",
			mock.Anything,
			mock.Anything,
			mock.Anything).Return(nil, fmt.Errorf("No epoch for you"))

		repository.Unset("ListEpochs")
		repository.On("ListEpochs",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Epoch{claim0}, nil)

		repository.Unset("UpdateEpochsClaimAccepted")
		repository.On("UpdateEpochsClaimAccepted",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return(nil)

		//No Inputs
		inputBox.Unset("RetrieveInputs")
		inputBox.On("RetrieveInputs",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]iinputbox.IInputBoxInputAdded{}, nil)

		// Prepare Client
		client.Unset("HeaderByNumber")
		client.On(
			"HeaderByNumber",
			mock.Anything,
			mock.Anything,
		).Return(&header0, nil).Once()

		// Start service
		ready := make(chan struct{}, 1)
		errChannel := make(chan error, 1)

		go func() {
			errChannel <- evmReader.Run(ctx, ready)
		}()

		select {
		case <-ready:
			break
		case err := <-errChannel:
			s.FailNow("unexpected error signal", err)
		}

		wsClient.fireNewHead(&header0)
		time.Sleep(1 * time.Second)

		repository.AssertNumberOfCalls(
			s.T(),
			"UpdateEpochsClaimAccepted",
			0,
		)

	})

	s.Run("whenHasPreviousOpenClaims", func() {

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		appAddress := common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E")

		// Contract Factory

		consensusContract := &MockIConsensusContract{}

		contractFactory := newEmvReaderContractFactory()

		contractFactory.Unset("NewIConsensus")
		contractFactory.On("NewIConsensus",
			mock.Anything,
		).Return(consensusContract, nil)

		//New EVM Reader
		client := newMockEthClient()
		wsClient := FakeWSEhtClient{}
		inputBox := newMockInputBox()
		repository := newMockRepository()
		s.evmReader.client = client
		s.evmReader.wsClient = &wsClient
		s.evmReader.inputSource = inputBox
		s.evmReader.repository = repository

		// Prepare Claims Acceptance Events

		claimEvent0 := &iconsensus.IConsensusClaimAcceptance{
			AppContract:              appAddress,
			LastProcessedBlockNumber: big.NewInt(3),
			Claim:                    common.HexToHash("0xdeadbeef"),
		}

		claimEvents := []*iconsensus.IConsensusClaimAcceptance{claimEvent0}
		consensusContract.On("RetrieveClaimAcceptanceEvents",
			mock.Anything,
			mock.Anything,
		).Return(claimEvents, nil).Once()
		consensusContract.On("RetrieveClaimAcceptanceEvents",
			mock.Anything,
			mock.Anything,
		).Return([]*iconsensus.IConsensusClaimAcceptance{}, nil)

		// Epoch Length
		consensusContract.On("GetEpochLength",
			mock.Anything,
		).Return(big.NewInt(1), nil).Once()

		// Prepare repository
		repository.Unset("ListApplications")
		repository.On(
			"ListApplications",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Application{{
			IApplicationAddress: appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			EpochLength:         10,
			LastClaimCheckBlock: 0x10,
		}}, nil).Once()
		repository.On(
			"ListApplications",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Application{{
			IApplicationAddress: appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			EpochLength:         10,
			LastClaimCheckBlock: 0x11,
		}}, nil).Once()

		claim0Hash := common.HexToHash("0xdeadbeef")
		claim0 := &Epoch{
			Index:      1,
			FirstBlock: 1,
			LastBlock:  1,
			Status:     EpochStatus_ClaimSubmitted,
			ClaimHash:  &claim0Hash,
		}

		repository.Unset("ListEpochs")
		repository.On("ListEpochs",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Epoch{claim0}, nil)

		repository.Unset("UpdateEpochsClaimAccepted")
		repository.On("UpdateEpochsClaimAccepted",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return(nil)

		//No Inputs
		inputBox.Unset("RetrieveInputs")
		inputBox.On("RetrieveInputs",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]iinputbox.IInputBoxInputAdded{}, nil)

		// Prepare Client
		client.Unset("HeaderByNumber")
		client.On(
			"HeaderByNumber",
			mock.Anything,
			mock.Anything,
		).Return(&header0, nil).Once()

		// Start service
		ready := make(chan struct{}, 1)
		errChannel := make(chan error, 1)

		go func() {
			errChannel <- s.evmReader.Run(ctx, ready)
		}()

		select {
		case <-ready:
			break
		case err := <-errChannel:
			s.FailNow("unexpected error signal", err)
		}

		wsClient.fireNewHead(&header0)
		time.Sleep(1 * time.Second)

		repository.AssertNumberOfCalls(
			s.T(),
			"UpdateEpochsClaimAccepted",
			0,
		)

	})
}
