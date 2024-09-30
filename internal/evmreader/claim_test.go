// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"fmt"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/inputbox"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
)

func (s *EvmReaderSuite) TestNoClaimsAcceptance() {

	//New EVM Reader
	evmReader := NewEvmReader(
		s.client,
		s.inputBox,
		s.repository,
		0x10,
		DefaultBlockStatusLatest,
		s.contractFactory,
	)

	// Prepare repository
	s.repository.Unset("GetAllRunningApplications")
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:     common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		LastClaimCheckBlock: 0x10,
	}}, nil).Once()
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:     common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		LastClaimCheckBlock: 0x11,
	}}, nil).Once()

	s.repository.Unset("UpdateEpochs")
	s.repository.On("UpdateEpochs",
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
	s.repository.On("UpdateEpochs",
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
	).Return([]inputbox.InputBoxInputAdded{}, nil)

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

	// Run 2 steps
	err := evmReader.Step(s.ctx)
	s.Require().Nil(err)
	err = evmReader.Step(s.ctx)
	s.Require().Nil(err)

	s.repository.AssertNumberOfCalls(
		s.T(),
		"UpdateEpochs",
		2,
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

	evmReader := NewEvmReader(
		s.client,
		s.inputBox,
		s.repository,
		0x00,
		DefaultBlockStatusLatest,
		contractFactory,
	)

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
	s.repository.Unset("GetAllRunningApplications")
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:     appAddress,
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		LastClaimCheckBlock: 0x10,
	}}, nil).Once()
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:     appAddress,
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		LastClaimCheckBlock: 0x11,
	}}, nil).Once()

	claim1Hash := common.HexToHash("0xdeadbeef")
	claim0 := &Epoch{
		Index:      3,
		FirstBlock: 3,
		LastBlock:  3,
		AppAddress: appAddress,
		Status:     EpochStatusClaimSubmitted,
		ClaimHash:  &claim1Hash,
	}

	s.repository.Unset("GetEpoch")
	s.repository.On("GetEpoch",
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(claim0, nil)

	s.repository.Unset("GetEpochsWithOpenClaims")
	s.repository.On("GetEpochsWithOpenClaims",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]*Epoch{}, nil)

	s.repository.Unset("UpdateEpochs")
	s.repository.On("UpdateEpochs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Once().Run(func(arguments mock.Arguments) {
		obj := arguments.Get(1)
		claims, ok := obj.([]*Epoch)
		s.Require().True(ok)
		s.Require().Equal(1, len(claims))
		claim0 := claims[0]
		s.Require().Equal(uint64(3), claim0.LastBlock)
		s.Require().Equal(EpochStatusClaimAccepted, claim0.Status)

	}).Return(nil)

	//No Inputs
	s.inputBox.Unset("RetrieveInputs")
	s.inputBox.On("RetrieveInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]inputbox.InputBoxInputAdded{}, nil)

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil).Once()

	//Run 1 step
	err := evmReader.Step(s.ctx)
	s.Require().Nil(err)

	s.repository.AssertNumberOfCalls(
		s.T(),
		"UpdateEpochs",
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
		evmReader := NewEvmReader(
			client,
			inputBox,
			repository,
			0x00,
			DefaultBlockStatusLatest,
			contractFactory,
		)

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
		repository.Unset("GetAllRunningApplications")
		repository.On(
			"GetAllRunningApplications",
			mock.Anything,
		).Return([]Application{{
			ContractAddress:     appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			LastClaimCheckBlock: 0x10,
		}}, nil).Once()
		repository.On(
			"GetAllRunningApplications",
			mock.Anything,
		).Return([]Application{{
			ContractAddress:     appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			LastClaimCheckBlock: 0x11,
		}}, nil).Once()

		claim1Hash := common.HexToHash("0xdeadbeef")
		claim1 := &Epoch{
			Index:      3,
			FirstBlock: 3,
			LastBlock:  3,
			AppAddress: appAddress,
			Status:     EpochStatusClaimSubmitted,
			ClaimHash:  &claim1Hash,
		}

		repository.Unset("GetEpoch")
		repository.On("GetEpoch",
			mock.Anything,
			mock.Anything,
			mock.Anything).Return(claim1, nil)

		repository.Unset("GetEpochsWithOpenClaims")
		repository.On("GetEpochsWithOpenClaims",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Epoch{}, fmt.Errorf("No previous epochs for you"))

		repository.Unset("UpdateEpochs")
		repository.On("UpdateEpochs",
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
		).Return([]inputbox.InputBoxInputAdded{}, nil)

		// Prepare Client
		client.Unset("HeaderByNumber")
		client.On(
			"HeaderByNumber",
			mock.Anything,
			mock.Anything,
		).Return(&header0, nil).Once()

		err := evmReader.Step(ctx)
		s.Require().Nil(err)

		repository.AssertNumberOfCalls(
			s.T(),
			"UpdateEpochs",
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
		inputBox := newMockInputBox()
		repository := newMockRepository()
		evmReader := NewEvmReader(
			client,
			inputBox,
			repository,
			0x00,
			DefaultBlockStatusLatest,
			contractFactory,
		)

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
		repository.Unset("GetAllRunningApplications")
		repository.On(
			"GetAllRunningApplications",
			mock.Anything,
		).Return([]Application{{
			ContractAddress:     appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			LastClaimCheckBlock: 0x10,
		}}, nil).Once()
		repository.On(
			"GetAllRunningApplications",
			mock.Anything,
		).Return([]Application{{
			ContractAddress:     appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			LastClaimCheckBlock: 0x11,
		}}, nil).Once()

		claim0Hash := common.HexToHash("0xdeadbeef")
		claim0 := &Epoch{
			Index:      1,
			FirstBlock: 1,
			LastBlock:  1,
			AppAddress: appAddress,
			Status:     EpochStatusClaimSubmitted,
			ClaimHash:  &claim0Hash,
		}

		repository.Unset("GetEpoch")
		repository.On("GetEpoch",
			mock.Anything,
			mock.Anything,
			mock.Anything).Return(nil, fmt.Errorf("No epoch for you"))

		repository.Unset("GetEpochsWithOpenClaims")
		repository.On("GetEpochsWithOpenClaims",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Epoch{claim0}, nil)

		repository.Unset("UpdateEpochs")
		repository.On("UpdateEpochs",
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
		).Return([]inputbox.InputBoxInputAdded{}, nil)

		// Prepare Client
		client.Unset("HeaderByNumber")
		client.On(
			"HeaderByNumber",
			mock.Anything,
			mock.Anything,
		).Return(&header0, nil).Once()

		err := evmReader.Step(ctx)
		s.Require().Nil(err)

		repository.AssertNumberOfCalls(
			s.T(),
			"UpdateEpochs",
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
		inputBox := newMockInputBox()
		repository := newMockRepository()
		evmReader := NewEvmReader(
			client,
			inputBox,
			repository,
			0x00,
			DefaultBlockStatusLatest,
			contractFactory,
		)

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
		repository.Unset("GetAllRunningApplications")
		repository.On(
			"GetAllRunningApplications",
			mock.Anything,
		).Return([]Application{{
			ContractAddress:     appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			LastClaimCheckBlock: 0x10,
		}}, nil).Once()
		repository.On(
			"GetAllRunningApplications",
			mock.Anything,
		).Return([]Application{{
			ContractAddress:     appAddress,
			IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
			LastClaimCheckBlock: 0x11,
		}}, nil).Once()

		claim0Hash := common.HexToHash("0xdeadbeef")
		claim0 := &Epoch{
			Index:      1,
			FirstBlock: 1,
			LastBlock:  1,
			AppAddress: appAddress,
			Status:     EpochStatusClaimSubmitted,
			ClaimHash:  &claim0Hash,
		}

		repository.Unset("GetEpochsWithOpenClaims")
		repository.On("GetEpochsWithOpenClaims",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]*Epoch{claim0}, nil)

		repository.Unset("UpdateEpochs")
		repository.On("UpdateEpochs",
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
		).Return([]inputbox.InputBoxInputAdded{}, nil)

		// Prepare Client
		client.Unset("HeaderByNumber")
		client.On(
			"HeaderByNumber",
			mock.Anything,
			mock.Anything,
		).Return(&header0, nil).Once()

		err := evmReader.Step(ctx)
		s.Require().Nil(err)

		repository.AssertNumberOfCalls(
			s.T(),
			"UpdateEpochs",
			0,
		)

	})
}
