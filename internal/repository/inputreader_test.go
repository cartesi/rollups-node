// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
)

func (s *RepositorySuite) TestInsertInputsAndUpdateMostRecentFinalizedBlock() {
	input1 := Input{
		Index:            6,
		CompletionStatus: InputStatusAccepted,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      5,
		MachineStateHash: common.HexToHash("deadbeef"),
	}

	input2 := Input{
		Index:            7,
		CompletionStatus: InputStatusAccepted,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      5,
		MachineStateHash: common.HexToHash("deadbeef"),
	}

	input3 := Input{
		Index:            8,
		CompletionStatus: InputStatusAccepted,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      5,
		MachineStateHash: common.HexToHash("deadbeef"),
	}

	var inputs []*Input
	inputs = append(inputs, &input1)
	inputs = append(inputs, &input2)
	inputs = append(inputs, &input3)

	err := s.database.InsertInputsAndUpdateMostRecentlyFinalizedBlock(s.ctx, inputs, 5)
	s.Require().Nil(err)

	response, err := s.database.GetInput(s.ctx, 6)
	s.Require().Nil(err)
	s.Require().Equal(&input1, response)

	var mostRecentCheck uint64 = 5
	response2, err := s.database.GetMostRecentlyFinalizedBlock(s.ctx)
	s.Require().Nil(err)
	s.Require().Equal(mostRecentCheck, response2)
}

func (s *RepositorySuite) TestInsertInputsAndUpdateMostRecentFinalizedBlockEmptyInputs() {
	var inputs []*Input

	err := s.database.InsertInputsAndUpdateMostRecentlyFinalizedBlock(s.ctx, inputs, 6)
	s.Require().Nil(err)

	var block uint64 = 6
	response, err := s.database.GetMostRecentlyFinalizedBlock(s.ctx)
	s.Require().Nil(err)
	s.Require().Equal(block, response)
}

func (s *RepositorySuite) TestInsertInputsAndUpdateMostRecentFinalizedBlockInputAlreadyExists() {
	input1 := Input{
		Index:            8,
		CompletionStatus: InputStatusAccepted,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      5,
		MachineStateHash: common.HexToHash("deadbeef"),
	}

	var inputs []*Input
	inputs = append(inputs, &input1)

	err := s.database.InsertInputsAndUpdateMostRecentlyFinalizedBlock(s.ctx, inputs, 6)
	s.Require().ErrorContains(err, "duplicate key value violates unique constraint")
}

func (s *RepositorySuite) TestInsertInputsAndUpdateMostRecentFinalizedBlockDuplicateInput() {
	input1 := Input{
		Index:            9,
		CompletionStatus: InputStatusAccepted,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      6,
		MachineStateHash: common.HexToHash("deadbeef"),
	}

	input2 := Input{
		Index:            9,
		CompletionStatus: InputStatusAccepted,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      6,
		MachineStateHash: common.HexToHash("deadbeef"),
	}

	var inputs []*Input
	inputs = append(inputs, &input1)
	inputs = append(inputs, &input2)

	err := s.database.InsertInputsAndUpdateMostRecentlyFinalizedBlock(s.ctx, inputs, 6)
	s.Require().ErrorContains(err, "duplicate key value violates unique constraint")
}
