// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
)

const testTimeout = 300 * time.Second

// This suite sets up a container running a postgres database
type RepositorySuite struct {
	suite.Suite
	ctx      context.Context
	cancel   context.CancelFunc
	database *Database
}

func (s *RepositorySuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	var err error
	endpoint, err := db.GetPostgresTestEndpoint()
	s.Require().Nil(err)

	err = db.SetupTestPostgres(endpoint)
	s.Require().Nil(err)

	s.database, err = Connect(s.ctx, endpoint)
	s.Require().Nil(err)

	s.SetupDatabase()
}

func (s *RepositorySuite) TearDownSuite() {
	s.cancel()
}

func (s *RepositorySuite) SetupDatabase() {
	config := NodePersistentConfig{
		DefaultBlock:            DefaultBlockStatusFinalized,
		InputBoxDeploymentBlock: 1,
		InputBoxAddress:         common.HexToAddress("deadbeef"),
		ChainId:                 1,
	}

	err := s.database.InsertNodeConfig(s.ctx, &config)
	s.Require().Nil(err)

	app := Application{
		ContractAddress:    common.HexToAddress("deadbeef"),
		IConsensusAddress:  common.HexToAddress("ffffff"),
		TemplateHash:       common.HexToHash("deadbeef"),
		TemplateUri:        "path/to/template/uri/0",
		LastProcessedBlock: 1,
		Status:             ApplicationStatusRunning,
	}

	app2 := Application{
		ContractAddress:    common.HexToAddress("feadbeef"),
		IConsensusAddress:  common.HexToAddress("ffffff"),
		TemplateHash:       common.HexToHash("deadbeef"),
		TemplateUri:        "path/to/template/uri/0",
		LastProcessedBlock: 1,
		Status:             ApplicationStatusNotRunning,
	}

	_, err = s.database.InsertApplication(s.ctx, &app)
	s.Require().Nil(err)

	_, err = s.database.InsertApplication(s.ctx, &app2)
	s.Require().Nil(err)

	genericHash := common.HexToHash("deadbeef")

	epoch1 := Epoch{
		Id:              1,
		Index:           0,
		FirstBlock:      0,
		LastBlock:       99,
		AppAddress:      app.ContractAddress,
		ClaimHash:       nil,
		TransactionHash: nil,
		Status:          EpochStatusOpen,
	}

	_, err = s.database.InsertEpoch(s.ctx, &epoch1)
	s.Require().Nil(err)

	epoch2 := Epoch{
		Id:              2,
		Index:           1,
		FirstBlock:      100,
		LastBlock:       199,
		AppAddress:      app.ContractAddress,
		ClaimHash:       nil,
		TransactionHash: nil,
		Status:          EpochStatusOpen,
	}

	_, err = s.database.InsertEpoch(s.ctx, &epoch2)
	s.Require().Nil(err)

	epoch3 := Epoch{
		Id:              3,
		Index:           2,
		FirstBlock:      200,
		LastBlock:       299,
		AppAddress:      app.ContractAddress,
		ClaimHash:       nil,
		TransactionHash: nil,
		Status:          EpochStatusClaimSubmitted,
	}

	_, err = s.database.InsertEpoch(s.ctx, &epoch3)
	s.Require().Nil(err)

	input1 := Input{
		Index:            1,
		CompletionStatus: InputStatusAccepted,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      1,
		MachineHash:      &genericHash,
		OutputsHash:      &genericHash,
		AppAddress:       app.ContractAddress,
		EpochId:          1,
	}

	input1.Id, err = s.database.InsertInput(s.ctx, &input1)
	s.Require().Nil(err)

	input2 := Input{
		Index:            2,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      3,
		MachineHash:      &genericHash,
		OutputsHash:      &genericHash,
		AppAddress:       app.ContractAddress,
		EpochId:          1,
	}

	input2.Id, err = s.database.InsertInput(s.ctx, &input2)
	s.Require().Nil(err)

	input3 := Input{
		Index:            3,
		CompletionStatus: InputStatusAccepted,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      (math.MaxUint64 / 2) + 1,
		MachineHash:      &genericHash,
		OutputsHash:      &genericHash,
		AppAddress:       app.ContractAddress,
		EpochId:          2,
	}

	input3.Id, err = s.database.InsertInput(s.ctx, &input3)
	s.Require().Nil(err)

	var siblings []Hash
	siblings = append(siblings, genericHash)

	output0 := Output{
		Index:                1,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: siblings,
	}

	output0.Id, err = s.database.InsertOutput(s.ctx, &output0)
	s.Require().Nil(err)

	output1 := Output{
		Index:                2,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: siblings,
	}

	output1.Id, err = s.database.InsertOutput(s.ctx, &output1)
	s.Require().Nil(err)

	output2 := Output{
		Index:                3,
		InputId:              2,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: siblings,
	}

	output2.Id, err = s.database.InsertOutput(s.ctx, &output2)
	s.Require().Nil(err)

	output3 := Output{
		Index:                4,
		InputId:              3,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: siblings,
	}

	output3.Id, err = s.database.InsertOutput(s.ctx, &output3)
	s.Require().Nil(err)

	report := Report{
		Index:   1,
		InputId: 1,
		RawData: common.Hex2Bytes("deadbeef"),
	}

	err = s.database.InsertReport(s.ctx, &report)
	s.Require().Nil(err)

	snapshot := Snapshot{
		InputId:    1,
		AppAddress: app.ContractAddress,
		URI:        "/some/path",
	}

	id, err := s.database.InsertSnapshot(s.ctx, &snapshot)
	s.Require().Nil(err)
	s.Require().Equal(uint64(1), id)
}

func (s *RepositorySuite) TestApplicationExists() {
	app := Application{
		Id:                 1,
		ContractAddress:    common.HexToAddress("deadbeef"),
		IConsensusAddress:  common.HexToAddress("ffffff"),
		TemplateHash:       common.HexToHash("deadbeef"),
		TemplateUri:        "path/to/template/uri/0",
		LastProcessedBlock: 1,
		Status:             ApplicationStatusRunning,
	}

	response, err := s.database.GetApplication(s.ctx, common.HexToAddress("deadbeef"))
	s.Require().Equal(&app, response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestApplicationDoesntExist() {
	response, err := s.database.GetApplication(s.ctx, common.HexToAddress("deadbeefaaa"))
	s.Require().Nil(response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestApplicationFailsDuplicateRow() {
	app := Application{
		Id:                 1,
		ContractAddress:    common.HexToAddress("deadbeef"),
		IConsensusAddress:  common.HexToAddress("ffffff"),
		TemplateHash:       common.HexToHash("deadbeef"),
		TemplateUri:        "path/to/template/uri/0",
		LastProcessedBlock: 1,
		Status:             ApplicationStatusRunning,
	}

	_, err := s.database.InsertApplication(s.ctx, &app)
	s.Require().ErrorContains(err, "duplicate key value")
}

func (s *RepositorySuite) TestInputExists() {
	genericHash := common.HexToHash("deadbeef")

	input := Input{
		Id:               1,
		Index:            1,
		CompletionStatus: InputStatusAccepted,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      1,
		MachineHash:      &genericHash,
		OutputsHash:      &genericHash,
		AppAddress:       common.HexToAddress("deadbeef"),
		EpochId:          1,
	}

	response, err := s.database.GetInput(s.ctx, 1, common.HexToAddress("deadbeef"))
	s.Require().Equal(&input, response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestInputDoesntExist() {
	response, err := s.database.GetInput(s.ctx, 10, common.HexToAddress("deadbeef"))
	s.Require().Nil(response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestInputFailsDuplicateRow() {
	input := Input{
		Index:            1,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      1,
		AppAddress:       common.HexToAddress("deadbeef"),
	}

	_, err := s.database.InsertInput(s.ctx, &input)
	s.Require().ErrorContains(err, "duplicate key value")
}

func (s *RepositorySuite) TestInputFailsApplicationDoesntExist() {
	input := Input{
		Index:            3,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      3,
		AppAddress:       common.HexToAddress("deadbeefaaa"),
	}

	_, err := s.database.InsertInput(s.ctx, &input)
	s.Require().ErrorContains(err, "violates foreign key constraint")
}

func (s *RepositorySuite) TestOutputExists() {
	var siblings []Hash
	siblings = append(siblings, common.HexToHash("deadbeef"))

	output := Output{
		Id:                   1,
		Index:                1,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: siblings,
	}

	response, err := s.database.GetOutput(s.ctx, 1, common.HexToAddress("deadbeef"))
	s.Require().Equal(&output, response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestOutputDoesntExist() {
	response, err := s.database.GetOutput(s.ctx, 10, common.HexToAddress("deadbeef"))
	s.Require().Nil(response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestOutputFailsInputDoesntExist() {
	output := Output{
		Index:                10,
		InputId:              10,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: nil,
	}

	_, err := s.database.InsertOutput(s.ctx, &output)
	s.Require().ErrorContains(err, "violates foreign key constraint")
}

func (s *RepositorySuite) TestReportExists() {
	report := Report{
		Id:      1,
		Index:   1,
		InputId: 1,
		RawData: common.Hex2Bytes("deadbeef"),
	}

	response, err := s.database.GetReport(s.ctx, 1, common.HexToAddress("deadbeef"))
	s.Require().Equal(&report, response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestReportDoesntExist() {
	response, err := s.database.GetReport(s.ctx, 10, common.HexToAddress("deadbeef"))
	s.Require().Nil(response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestReportFailsInputDoesntExist() {
	report := Report{
		Index:   2,
		InputId: 10,
		RawData: common.Hex2Bytes("deadbeef"),
	}

	err := s.database.InsertReport(s.ctx, &report)
	s.Require().ErrorContains(err, "violates foreign key constraint")
}

func (s *RepositorySuite) TestEpochExists() {

	epoch := Epoch{
		Id:              1,
		Status:          EpochStatusOpen,
		Index:           0,
		FirstBlock:      0,
		LastBlock:       99,
		TransactionHash: nil,
		ClaimHash:       nil,
		AppAddress:      common.HexToAddress("deadbeef"),
	}

	response, err := s.database.GetEpoch(s.ctx, 0, common.HexToAddress("deadbeef"))
	s.Require().Equal(epoch, *response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestEpochDoesntExist() {
	response, err := s.database.GetEpoch(s.ctx, 3, common.HexToAddress("deadbeef"))
	s.Require().Nil(response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestEpochFailsDuplicateRow() {
	epoch := Epoch{
		Status:          EpochStatusOpen,
		Index:           0,
		FirstBlock:      0,
		LastBlock:       math.MaxUint64,
		TransactionHash: nil,
		ClaimHash:       nil,
		AppAddress:      common.HexToAddress("deadbeef"),
	}

	_, err := s.database.InsertEpoch(s.ctx, &epoch)
	s.Require().ErrorContains(err, "duplicate key value")
}

func (s *RepositorySuite) TestEpochFailsApplicationDoesntExist() {
	hash := common.HexToHash("deadbeef")
	epoch := Epoch{
		Status:     EpochStatusOpen,
		Index:      2,
		FirstBlock: 0,
		LastBlock:  math.MaxUint64,
		ClaimHash:  &hash,
		AppAddress: common.HexToAddress("deadbeefaaa"),
	}

	_, err := s.database.InsertEpoch(s.ctx, &epoch)
	s.Require().ErrorContains(err, "violates foreign key constraint")
}

func (s *RepositorySuite) TestGetSnapshot() {

	expectedSnapshot := Snapshot{
		Id:         1,
		InputId:    1,
		AppAddress: common.HexToAddress("deadbeef"),
		URI:        "/some/path",
	}

	actualSnapshot, err := s.database.GetSnapshot(s.ctx, 1, common.HexToAddress("deadbeef"))
	s.Require().Nil(err)
	s.Require().NotNil(actualSnapshot)
	s.Require().Equal(&expectedSnapshot, actualSnapshot)
}

func (s *RepositorySuite) TestInsertSnapshotFailsSameInputId() {

	snapshot := Snapshot{
		InputId:    1,
		AppAddress: common.HexToAddress("feadbeef"),
		URI:        "/some/path",
	}

	_, err := s.database.InsertSnapshot(s.ctx, &snapshot)
	s.Require().ErrorContains(err, "violates unique constraint")

}

func TestRepositorySuite(t *testing.T) {
	suite.Run(t, new(RepositorySuite))
}
