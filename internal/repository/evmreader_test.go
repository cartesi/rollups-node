// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
)

func (s *RepositorySuite) TestInsertInputsAndUpdateLastProcessedBlock() {
	input0 := Input{
		Id:               6,
		Index:            5,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      5,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	input1 := Input{
		Id:               7,
		Index:            6,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      6,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	var inputs []Input
	inputs = append(inputs, input0)
	inputs = append(inputs, input1)

	err := s.database.InsertInputsAndUpdateLastProcessedBlock(
		s.ctx,
		inputs,
		6,
		common.HexToAddress("deadbeef"),
	)
	s.Require().Nil(err)

	response, err := s.database.GetInput(s.ctx, 5, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(&input0, response)

	var mostRecentCheck uint64 = 6
	response2, err := s.database.GetLastProcessedBlock(s.ctx, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(mostRecentCheck, response2)
}

func (s *RepositorySuite) TestInsertInputsAndUpdateMostRecentFinalizedBlockEmptyInputs() {
	var inputs []Input

	err := s.database.InsertInputsAndUpdateLastProcessedBlock(
		s.ctx,
		inputs,
		7,
		common.HexToAddress("deadbeef"),
	)
	s.Require().Nil(err)

	var block uint64 = 7
	response, err := s.database.GetLastProcessedBlock(s.ctx, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(block, response)
}

func (s *RepositorySuite) TestInsertInputsAndUpdateLastProcessedBlockInputAlreadyExists() {
	input := Input{
		Id:               5,
		Index:            5,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      5,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	var inputs []Input
	inputs = append(inputs, input)

	err := s.database.InsertInputsAndUpdateLastProcessedBlock(
		s.ctx,
		inputs,
		8,
		common.HexToAddress("deadbeef"),
	)
	s.Require().ErrorContains(err, "duplicate key value violates unique constraint")
}

func (s *RepositorySuite) TestInsertInputsAndUpdateLastProcessedBlockDuplicateInput() {
	input0 := Input{
		Id:               7,
		Index:            7,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      7,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	input1 := Input{
		Id:               7,
		Index:            7,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      7,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	var inputs []Input
	inputs = append(inputs, input0)
	inputs = append(inputs, input1)

	err := s.database.InsertInputsAndUpdateLastProcessedBlock(
		s.ctx,
		inputs,
		8,
		common.HexToAddress("deadbeef"),
	)
	s.Require().ErrorContains(err, "duplicate key value violates unique constraint")
}

func (s *RepositorySuite) TestGetAllRunningApplications() {
	app := Application{
		Id:                 1,
		ContractAddress:    common.HexToAddress("deadbeef"),
		TemplateHash:       common.HexToHash("deadbeef"),
		LastProcessedBlock: 1,
		EpochLength:        10,
		Status:             ApplicationStatusRunning,
	}

	response, err := s.database.GetAllRunningApplications(s.ctx)
	s.Require().Nil(err)
	s.Require().Equal(app, response[0])
}
