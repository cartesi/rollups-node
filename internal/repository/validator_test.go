// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"time"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
)

func (s *RepositorySuite) TestGetMostRecentBlock() {
	var block uint64 = 1

	response, err := s.database.GetLastProcessedBlock(s.ctx, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)

	s.Require().Equal(block, response)
}

func (s *RepositorySuite) TestGetAllOutputsFromProcessedInputs() {
	output0 := Output{
		Id:                   1,
		Index:                1,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: nil,
	}

	output1 := Output{
		Id:                   2,
		Index:                2,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: nil,
	}

	timeout := 5 * time.Second

	response, err := s.database.GetAllOutputsFromProcessedInputs(s.ctx, 1, 2, &timeout)
	s.Require().Nil(err)
	s.Require().Equal(output0, response[0])
	s.Require().Equal(output1, response[1])
}

func (s *RepositorySuite) TestGetAllOutputsFromProcessedInputsTimeout() {
	timeout := 5 * time.Second

	_, err := s.database.GetAllOutputsFromProcessedInputs(s.ctx, 2, 3, &timeout)
	s.Require().ErrorContains(err, "timeout")
}

func (s *RepositorySuite) TestFinishEpochTransaction() {
	var siblings []Hash
	siblings = append(siblings, common.HexToHash("deadbeef"))

	output := Output{
		Id:                   1,
		Index:                1,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: siblings,
	}

	claim := Claim{
		Id:                   4,
		Index:                2,
		Status:               ClaimStatusPending,
		OutputMerkleRootHash: common.HexToHash("deadbeef"),
		AppAddress:           common.HexToAddress("deadbeef"),
	}

	var outputs []Output
	outputs = append(outputs, output)

	err := s.database.FinishEpoch(s.ctx, &claim, outputs)
	s.Require().Nil(err)

	response0, err := s.database.GetClaim(s.ctx, common.HexToAddress("deadbeef"), 2)
	s.Require().Nil(err)
	s.Require().Equal(claim, *response0)

	response1, err := s.database.GetOutput(s.ctx, 1, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(output, *response1)
}

func (s *RepositorySuite) TestFinishEpochTransactionRollback() {
	var siblings []Hash
	siblings = append(siblings, common.HexToHash("deadbeef"))

	output := Output{
		Id:                   2,
		Index:                2,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: siblings,
	}

	claim := Claim{
		Index:                2,
		Status:               ClaimStatusPending,
		OutputMerkleRootHash: common.HexToHash("deadbeef"),
		AppAddress:           common.HexToAddress("deadbeef"),
	}

	var outputs []Output
	outputs = append(outputs, output)

	err := s.database.FinishEpoch(s.ctx, &claim, outputs)
	s.Require().ErrorContains(err, "unable to finish epoch")
}
