// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"time"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
)

func (s *RepositorySuite) TestFinishEmptyEpoch() {
	epoch := Epoch{
		StartBlock: 2,
		EndBlock:   3,
	}

	err := s.database.FinishEmptyEpochTransaction(s.ctx, &epoch)
	s.Require().Nil(err)

	response, err := s.database.GetCurrentEpoch(s.ctx)
	s.Require().Nil(err)

	s.Require().Equal(&epoch, response)
}

func (s *RepositorySuite) TestFinishEpochRollback() {
	epoch := Epoch{
		StartBlock: 2,
		EndBlock:   3,
	}

	err := s.database.FinishEmptyEpochTransaction(s.ctx, &epoch)

	s.Require().ErrorContains(err, "unable to finish empty epoch")
}

func (s *RepositorySuite) TestGetMostRecentBlock() {
	var block uint64 = 1

	response, err := s.database.GetMostRecentBlock(s.ctx)
	s.Require().Nil(err)

	s.Require().Equal(block, response)
}

func (s *RepositorySuite) TestGetAllOutputsFromProcessedInputs() {
	input := Input{
		Index:            2,
		Status:           Accepted,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      2,
		MachineStateHash: common.HexToHash("deadbeef"),
	}

	err := s.database.InsertInput(s.ctx, &input)
	s.Require().Nil(err)

	output0 := Output{
		Index:      0,
		InputIndex: 2,
		Blob:       common.Hex2Bytes("deadbeef"),
	}

	err = s.database.InsertOutput(s.ctx, true, &output0)
	s.Require().Nil(err)

	output1 := Output{
		Index:      1,
		InputIndex: 2,
		Blob:       common.Hex2Bytes("deadbeef"),
	}

	err = s.database.InsertOutput(s.ctx, true, &output1)
	s.Require().Nil(err)

	timeout := 5 * time.Second

	response, err := s.database.GetAllOutputsFromProcessedInputs(s.ctx, 2, 3, &timeout)
	s.Require().Nil(err)
	s.Require().Equal(output0, response[0])
	s.Require().Equal(output1, response[1])
}

func (s *RepositorySuite) TestGetAllOutputsFromProcessedInputsTimeout() {
	input := Input{
		Index:            3,
		Status:           None,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      3,
		MachineStateHash: common.HexToHash("deadbeef"),
	}
	err := s.database.InsertInput(s.ctx, &input)
	s.Require().Nil(err)

	output0 := Output{
		Index:      0,
		InputIndex: 3,
		Blob:       common.Hex2Bytes("deadbeef"),
	}
	err = s.database.InsertOutput(s.ctx, true, &output0)
	s.Require().Nil(err)

	output1 := Output{
		Index:      1,
		InputIndex: 3,
		Blob:       common.Hex2Bytes("deadbeef"),
	}
	err = s.database.InsertOutput(s.ctx, true, &output1)
	s.Require().Nil(err)

	timeout := 5 * time.Second

	_, err = s.database.GetAllOutputsFromProcessedInputs(s.ctx, 2, 3, &timeout)

	s.Require().ErrorContains(err, "GetAllOutputsFromProcessedInputs query timed out")

}

func (s *RepositorySuite) TestFinishEpochTransaction() {
	input := Input{
		Index:            4,
		Status:           Accepted,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      4,
		MachineStateHash: common.HexToHash("deadbeef"),
	}
	err := s.database.InsertInput(s.ctx, &input)
	s.Require().Nil(err)

	output1 := Output{
		Index:      0,
		InputIndex: 4,
		Blob:       common.Hex2Bytes("deadbeef"),
	}
	err = s.database.InsertOutput(s.ctx, true, &output1)
	s.Require().Nil(err)

	output2 := Output{
		Index:      1,
		InputIndex: 4,
		Blob:       common.Hex2Bytes("deadbeef"),
	}
	err = s.database.InsertOutput(s.ctx, true, &output2)
	s.Require().Nil(err)

	epoch := Epoch{
		StartBlock: 3,
		EndBlock:   4,
	}

	inputRange := InputRange{
		First: 2,
		Last:  3,
	}

	claim := Claim{
		Id:         1,
		Epoch:      2,
		InputRange: inputRange,
		EpochHash:  common.HexToHash("deadbeef"),
		AppAddress: common.HexToAddress("deadbeef"),
	}

	var siblings []Hash
	siblings = append(siblings, common.HexToHash("deadbeef"))

	proof1 := Proof{
		InputIndex:                       4,
		ClaimId:                          1,
		InputRange:                       inputRange,
		InputIndexWithinEpoch:            0,
		OutputIndexWithinInput:           0,
		OutputHashesRootHash:             common.HexToHash("deadbeef"),
		OutputsEpochRootHash:             common.HexToHash("deadbeef"),
		MachineStateHash:                 common.HexToHash("deadbeef"),
		OutputHashInOutputHashesSiblings: siblings,
		OutputHashesInEpochSiblings:      siblings,
	}

	proof2 := Proof{
		InputIndex:                       4,
		ClaimId:                          1,
		InputRange:                       inputRange,
		InputIndexWithinEpoch:            0,
		OutputIndexWithinInput:           1,
		OutputHashesRootHash:             common.HexToHash("deadbeef"),
		OutputsEpochRootHash:             common.HexToHash("deadbeef"),
		MachineStateHash:                 common.HexToHash("deadbeef"),
		OutputHashInOutputHashesSiblings: siblings,
		OutputHashesInEpochSiblings:      siblings,
	}

	var proofs []Proof
	proofs = append(proofs, proof1)
	proofs = append(proofs, proof2)

	err = s.database.FinishEpochTransaction(s.ctx, epoch, &claim, proofs)
	s.Require().Nil(err)

	response, err := s.database.GetCurrentEpoch(s.ctx)
	s.Require().Nil(err)
	s.Require().Equal(epoch, *response)

	response1 := s.database.GetClaim(s.ctx, 1)
	s.Require().Equal(claim, *response1)

	response2 := s.database.GetProof(s.ctx, 4, 0)
	s.Require().Equal(proof1, *response2)
}

func (s *RepositorySuite) TestFinishEpochTransactionRollback() {
	input := Input{
		Index:            5,
		Status:           Accepted,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      4,
		MachineStateHash: common.HexToHash("deadbeef"),
	}
	err := s.database.InsertInput(s.ctx, &input)
	s.Require().Nil(err)

	output := Output{
		Index:      0,
		InputIndex: 5,
		Blob:       common.Hex2Bytes("deadbeef"),
	}
	err = s.database.InsertOutput(s.ctx, true, &output)
	s.Require().Nil(err)

	epoch := Epoch{
		StartBlock: 4,
		EndBlock:   5,
	}

	inputRange := InputRange{
		First: 3,
		Last:  4,
	}

	claim := Claim{
		Id:         2,
		Epoch:      3,
		InputRange: inputRange,
		EpochHash:  common.HexToHash("deadbeef"),
		AppAddress: common.HexToAddress("deadbeef"),
	}

	var siblings []Hash
	siblings = append(siblings, common.HexToHash("deadbeef"))

	proof1 := Proof{
		InputIndex:                       10,
		ClaimId:                          2,
		InputRange:                       inputRange,
		InputIndexWithinEpoch:            0,
		OutputIndexWithinInput:           0,
		OutputHashesRootHash:             common.HexToHash("deadbeef"),
		OutputsEpochRootHash:             common.HexToHash("deadbeef"),
		MachineStateHash:                 common.HexToHash("deadbeef"),
		OutputHashInOutputHashesSiblings: siblings,
		OutputHashesInEpochSiblings:      siblings,
	}

	var proofs []Proof
	proofs = append(proofs, proof1)

	err = s.database.FinishEpochTransaction(s.ctx, epoch, &claim, proofs)
	s.Require().ErrorContains(err, "unable to finish epoch")

	response, err := s.database.GetCurrentEpoch(s.ctx)
	s.Require().Nil(err)

	var block uint64 = 3
	s.Require().Equal(block, response.StartBlock)
}
