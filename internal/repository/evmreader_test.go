// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
)

func (s *RepositorySuite) TestInsertInputsAndUpdateLastProcessedBlock() {
	input0 := Input{
		Index:            5,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      5,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	input1 := Input{
		Index:            6,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      6,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	ids, err := s.database.InsertInputsAndUpdateLastProcessedBlock(
		s.ctx,
		[]Input{input0, input1},
		6,
		common.HexToAddress("deadbeef"),
	)
	s.Require().Nil(err)
	s.Require().Len(ids, 2)

	input0.Id = ids[0]
	input1.Id = ids[1]

	response, err := s.database.GetInput(s.ctx, 5, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(&input0, response)

	var mostRecentCheck uint64 = 6
	response2, err := s.database.GetLastProcessedBlock(s.ctx, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(mostRecentCheck, response2)
}

func (s *RepositorySuite) TestInsertInputsAndUpdateMostRecentFinalizedBlockEmptyInputs() {
	_, err := s.database.InsertInputsAndUpdateLastProcessedBlock(
		s.ctx,
		nil,
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
		Index:            5,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      5,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}
	_, err := s.database.InsertInputsAndUpdateLastProcessedBlock(
		s.ctx,
		[]Input{input},
		8,
		common.HexToAddress("deadbeef"),
	)
	s.Require().ErrorContains(err, "duplicate key value violates unique constraint")
}

func (s *RepositorySuite) TestInsertInputsAndUpdateLastProcessedBlockDuplicateInput() {
	input0 := Input{
		Index:            7,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      7,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	input1 := Input{
		Index:            7,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      7,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	_, err := s.database.InsertInputsAndUpdateLastProcessedBlock(
		s.ctx,
		[]Input{input0, input1},
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
		Status:             ApplicationStatusRunning,
	}

	response, err := s.database.GetAllRunningApplications(s.ctx)
	s.Require().Nil(err)
	s.Require().Equal(app, response[0])
}

func (s *RepositorySuite) TestGetMostRecentBlock() {
	var block uint64 = 1

	response, err := s.database.GetLastProcessedBlock(s.ctx, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)

	s.Require().Equal(block, response)
}
