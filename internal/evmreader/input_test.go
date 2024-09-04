// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/pkg/contracts/inputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
)

func (s *EvmReaderSuite) TestItReadsInputsFromNewBlocks() {

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
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:  common.HexToAddress("0xdeadbeef"),
		LastProcessedBlock: 0x00,
	}}, nil).Once()
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:  common.HexToAddress("0xdeadbeef"),
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
	events_0 := []inputbox.InputBoxInputAdded{inputAddedEvent0}
	mostRecentBlockNumber_0 := uint64(0x11)
	retrieveInputsOpts_0 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x10,
		End:     &mostRecentBlockNumber_0,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_0,
		mock.Anything,
		mock.Anything,
	).Return(events_0, nil)

	events_1 := []inputbox.InputBoxInputAdded{inputAddedEvent1}
	mostRecentBlockNumber_1 := uint64(0x12)
	retrieveInputsOpts_1 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x12,
		End:     &mostRecentBlockNumber_1,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_1,
		mock.Anything,
		mock.Anything,
	).Return(events_1, nil)

	// Run 2 steps
	err := evmReader.Step(s.ctx)
	s.Require().Nil(err)
	err = evmReader.Step(s.ctx)
	s.Require().Nil(err)

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 2)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"StoreEpochAndInputsTransaction",
		2,
	)
}

func (s *EvmReaderSuite) TestItUpdatesLastProcessedBlockWhenThereIsNoInputs() {

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
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:  common.HexToAddress("0xdeadbeef"),
		LastProcessedBlock: 0x00,
	}}, nil).Once()
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:    common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:  common.HexToAddress("0xdeadbeef"),
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
	events_0 := []inputbox.InputBoxInputAdded{}
	mostRecentBlockNumber_0 := uint64(0x11)
	retrieveInputsOpts_0 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x10,
		End:     &mostRecentBlockNumber_0,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_0,
		mock.Anything,
		mock.Anything,
	).Return(events_0, nil)

	events_1 := []inputbox.InputBoxInputAdded{}
	mostRecentBlockNumber_1 := uint64(0x12)
	retrieveInputsOpts_1 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x12,
		End:     &mostRecentBlockNumber_1,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_1,
		mock.Anything,
		mock.Anything,
	).Return(events_1, nil)

	// Run 2 steps
	err := evmReader.Step(s.ctx)
	s.Require().Nil(err)
	err = evmReader.Step(s.ctx)
	s.Require().Nil(err)

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 2)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"StoreEpochAndInputsTransaction",
		2,
	)
}

func (s *EvmReaderSuite) TestItReadsMultipleInputsFromSingleNewBlock() {

	evmReader := NewEvmReader(
		s.client,
		s.inputBox,
		s.repository,
		0x10,
		DefaultBlockStatusLatest,
		s.contractFactory,
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
	mostRecentBlockNumber_2 := uint64(0x13)
	retrieveInputsOpts_2 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x13,
		End:     &mostRecentBlockNumber_2,
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
		IConsensusAddress:  common.HexToAddress("0xdeadbeef"),
		LastProcessedBlock: 0x12,
	}}, nil).Once()
	s.repository.Unset("StoreEpochAndInputsTransaction")
	s.repository.On(
		"StoreEpochAndInputsTransaction",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Once().Run(func(arguments mock.Arguments) {
		var epochInputMap map[*Epoch][]Input
		obj := arguments.Get(1)
		epochInputMap, ok := obj.(map[*Epoch][]Input)
		s.Require().True(ok)
		s.Require().Equal(1, len(epochInputMap))
		for _, inputs := range epochInputMap {
			s.Require().Equal(2, len(inputs))
			break
		}

	}).Return(make(map[uint64]uint64), make(map[uint64][]uint64), nil)

	// Run 1 step
	err := evmReader.Step(s.ctx)
	s.Require().Nil(err)

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 1)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"StoreEpochAndInputsTransaction",
		1,
	)
}

func (s *EvmReaderSuite) TestItStartsWhenLasProcessedBlockIsTheMostRecentBlock() {

	evmReader := NewEvmReader(
		s.client,
		s.inputBox,
		s.repository,
		0x10,
		DefaultBlockStatusLatest,
		s.contractFactory,
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
		IConsensusAddress:  common.HexToAddress("0xdeadbeef"),
		LastProcessedBlock: 0x11,
	}}, nil).Once()

	// Run 1 setp
	err := evmReader.Step(s.ctx)
	s.Require().Nil(err)

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 0)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"StoreEpochAndInputsTransaction",
		0,
	)
}
