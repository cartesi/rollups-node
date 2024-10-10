// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
)

func (s *RepositorySuite) TestInsertInputsAndUpdateLastProcessedBlock() {

	epoch0 := Epoch{
		Index:      0,
		FirstBlock: 0,
		LastBlock:  9,
		AppAddress: common.HexToAddress("deadbeef"),
		Status:     EpochStatusOpen,
	}

	input0 := Input{
		Index:            5,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      5,
		AppAddress:       common.HexToAddress("deadbeef"),
	}

	input1 := Input{
		Index:            6,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      6,
		AppAddress:       common.HexToAddress("deadbeef"),
	}

	epochInputMap := make(map[*Epoch][]Input)

	epochInputMap[&epoch0] = []Input{input0, input1}

	epochIdMap, epochInputIdMap, err := s.database.StoreEpochAndInputsTransaction(
		s.ctx,
		epochInputMap,
		6,
		common.HexToAddress("deadbeef"),
	)
	s.Require().Nil(err)
	s.Require().Len(epochIdMap, 1)
	s.Require().Len(epochInputIdMap[0], 2)

	input0.Id = epochInputIdMap[0][0]
	input0.EpochId = epochIdMap[0]
	input1.Id = epochInputIdMap[0][1]
	input1.EpochId = epochIdMap[0]

	response, err := s.database.GetInput(s.ctx, 5, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(&input0, response)

	var mostRecentCheck uint64 = 6
	response2, err := s.database.GetLastProcessedBlock(s.ctx, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(mostRecentCheck, response2)
}

func (s *RepositorySuite) TestInsertInputsAndUpdateMostRecentFinalizedBlockEmptyInputs() {
	_, _, err := s.database.StoreEpochAndInputsTransaction(
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

	epoch, err := s.database.GetEpoch(s.ctx, 0, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)

	input := Input{
		Index:            5,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      5,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	epochInputMap := make(map[*Epoch][]Input)
	epochInputMap[epoch] = []Input{input}
	_, _, err = s.database.StoreEpochAndInputsTransaction(
		s.ctx,
		epochInputMap,
		8,
		common.HexToAddress("deadbeef"),
	)
	s.Require().ErrorContains(err, "duplicate key value violates unique constraint")
}

func (s *RepositorySuite) TestInsertInputsAndUpdateLastProcessedBlockDuplicateInput() {

	epoch, err := s.database.GetEpoch(s.ctx, 0, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)

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
	epochInputMap := make(map[*Epoch][]Input)
	epochInputMap[epoch] = []Input{input0, input1}
	_, _, err = s.database.StoreEpochAndInputsTransaction(
		s.ctx,
		epochInputMap,
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
		IConsensusAddress:  common.HexToAddress("ffffff"),
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

func (s *RepositorySuite) TestGetPreviousEpochsWithOpenClaims() {
	response, err := s.database.GetPreviousEpochsWithOpenClaims(
		s.ctx, common.HexToAddress("deadbeef"), 300)

	s.Require().Nil(err)
	s.Require().NotNil(response)
	s.Require().Equal(1, len(response))

	epoch, err := s.database.GetEpoch(s.ctx, 2, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)

	s.Require().Equal(epoch, response[0])
}

func (s *RepositorySuite) TestUpdateEpochs() {

	claim, err := s.database.GetEpoch(s.ctx, 2, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().NotNil(claim)

	s.Require().Equal(EpochStatusClaimSubmitted, claim.Status)
	claim.Status = EpochStatusClaimAccepted

	claims := []*Epoch{claim}

	err = s.database.UpdateEpochs(
		s.ctx,
		common.HexToAddress("deadbeef"),
		claims,
		499,
	)
	s.Require().Nil(err)

	claim, err = s.database.GetEpoch(s.ctx, 2, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().NotNil(claim)
	s.Require().Equal(EpochStatusClaimAccepted, claim.Status)

	application, err := s.database.GetApplication(s.ctx, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(uint64(499), application.LastClaimCheckBlock)

}

func (s *RepositorySuite) TestUpdateOutputExecutionTransaction() {
	output, err := s.database.GetOutput(s.ctx, 1, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().NotNil(output)

	var executedOutputs []*Output
	hash := common.HexToHash("0xAABBCCDD")
	output.TransactionHash = &hash

	executedOutputs = append(executedOutputs, output)

	err = s.database.UpdateOutputExecutionTransaction(
		s.ctx, common.HexToAddress("deadbeef"), executedOutputs, 854758)
	s.Require().Nil(err)

	actualOutput, err := s.database.GetOutput(s.ctx, 1, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(output, actualOutput)

	application, err := s.database.GetApplication(s.ctx, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(uint64(854758), application.LastOutputCheckBlock)

}
