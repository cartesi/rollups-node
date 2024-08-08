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

func (s *RepositorySuite) TestSetEpochClaimAndInsertProofsTransaction() {
	var siblings []Hash
	siblings = append(siblings, common.HexToHash("deadbeef"))

	output := Output{
		Id:                   1,
		Index:                1,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: siblings,
	}

	hash := common.HexToHash("deadbeef")

	expectedEpoch, err := s.database.GetEpoch(s.ctx, 0, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)

	expectedEpoch.ClaimHash = &hash
	expectedEpoch.Status = EpochStatusClaimComputed

	epoch, err := s.database.GetEpoch(s.ctx, 0, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)

	epoch.ClaimHash = &hash
	epoch.Status = EpochStatusClaimComputed

	var outputs []Output
	outputs = append(outputs, output)

	err = s.database.SetEpochClaimAndInsertProofsTransaction(s.ctx, *epoch, outputs)
	s.Require().Nil(err)

	actualEpoch, err := s.database.GetEpoch(s.ctx, epoch.Index, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(expectedEpoch, actualEpoch)

	actualOutput, err := s.database.GetOutput(s.ctx, output.Index, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().Equal(output, *actualOutput)
}

func (s *RepositorySuite) TestSetEpochClaimAndInsertProofsTransactionRollback() {
	var siblings []Hash
	siblings = append(siblings, common.HexToHash("deadbeef"))

	output := Output{
		Id:                   5,
		Index:                4,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: siblings,
	}

	hash := common.HexToHash("deadbeef")

	epoch, err := s.database.GetEpoch(s.ctx, 1, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)

	epoch.ClaimHash = &hash
	epoch.Status = EpochStatusClaimComputed

	expectedEpoch, err := s.database.GetEpoch(s.ctx, epoch.Index, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)

	var outputs []Output
	outputs = append(outputs, output)

	err = s.database.SetEpochClaimAndInsertProofsTransaction(s.ctx, *epoch, outputs)
	s.Require().ErrorContains(err, "unable to set claim")

	actualEpoch, err := s.database.GetEpoch(s.ctx, expectedEpoch.Index, expectedEpoch.AppAddress)
	s.Require().Nil(err)
	s.Require().NotNil(actualEpoch)
	s.Require().Equal(expectedEpoch, actualEpoch)
}
