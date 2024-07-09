// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"testing"
	"time"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const testTimeout = 300 * time.Second

// This suite sets up a container running a postgres database
type RepositorySuite struct {
	suite.Suite
	ctx      context.Context
	cancel   context.CancelFunc
	postgres *postgres.PostgresContainer
	database *database
}

func (s *RepositorySuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	var err error
	s.postgres, err = newPostgresContainer(s.ctx)
	s.Require().Nil(err)

	endpoint, err := s.postgres.ConnectionString(s.ctx, "sslmode=disable")
	s.Require().Nil(err)
	RunMigrations(endpoint)

	s.database, err = Connect(s.ctx, endpoint)
	s.Require().Nil(err)

	s.SetupDatabase()
}

func (s *RepositorySuite) TearDownSuite() {
	err := s.postgres.Terminate(s.ctx)
	s.Nil(err)
	s.cancel()
}

func (s *RepositorySuite) SetupDatabase() {
	config := NodePersistentConfig{
		DefaultBlock:            DefaultBlockStatusFinalized,
		InputBoxDeploymentBlock: 1,
		InputBoxAddress:         common.HexToAddress("deadbeef"),
		ChainId:                 1,
		IConsensusAddress:       common.HexToAddress("deadbeef"),
	}

	err := s.database.InsertNodeConfig(s.ctx, &config)
	s.Require().Nil(err)

	app := Application{
		Id:                 1,
		ContractAddress:    common.HexToAddress("deadbeef"),
		TemplateHash:       common.HexToHash("deadbeef"),
		SnapshotURI:        "this/is/a/test",
		LastProcessedBlock: 1,
		EpochLength:        10,
		Status:             ApplicationStatusRunning,
	}

	err = s.database.InsertApplication(s.ctx, &app)
	s.Require().Nil(err)

	genericHash := common.HexToHash("deadbeef")

	input1 := Input{
		Index:            1,
		CompletionStatus: InputStatusAccepted,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      1,
		MachineHash:      &genericHash,
		OutputsHash:      &genericHash,
		AppAddress:       common.HexToAddress("deadbeef"),
	}

	err = s.database.InsertInput(s.ctx, &input1)
	s.Require().Nil(err)

	input2 := Input{
		Index:            2,
		CompletionStatus: InputStatusNone,
		RawData:          common.Hex2Bytes("deadbeef"),
		BlockNumber:      3,
		MachineHash:      &genericHash,
		OutputsHash:      &genericHash,
		AppAddress:       common.HexToAddress("deadbeef"),
	}

	err = s.database.InsertInput(s.ctx, &input2)
	s.Require().Nil(err)

	var siblings []Hash
	siblings = append(siblings, genericHash)

	output0 := Output{
		Index:                1,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: nil,
	}

	err = s.database.InsertOutput(s.ctx, &output0)
	s.Require().Nil(err)

	output1 := Output{
		Index:                2,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: nil,
	}

	err = s.database.InsertOutput(s.ctx, &output1)
	s.Require().Nil(err)

	output3 := Output{
		Index:                3,
		InputId:              2,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: siblings,
	}

	err = s.database.InsertOutput(s.ctx, &output3)
	s.Require().Nil(err)

	report := Report{
		Index:   1,
		InputId: 1,
		RawData: common.Hex2Bytes("deadbeef"),
	}

	err = s.database.InsertReport(s.ctx, &report)
	s.Require().Nil(err)

	claim := Claim{
		Status:               ClaimStatusPending,
		Index:                1,
		OutputMerkleRootHash: genericHash,
		TransactionHash:      &genericHash,
		AppAddress:           common.HexToAddress("deadbeef"),
	}

	err = s.database.InsertClaim(s.ctx, &claim)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestApplicationExists() {
	app := Application{
		Id:                 1,
		ContractAddress:    common.HexToAddress("deadbeef"),
		TemplateHash:       common.HexToHash("deadbeef"),
		SnapshotURI:        "this/is/a/test",
		LastProcessedBlock: 1,
		EpochLength:        10,
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
		TemplateHash:       common.HexToHash("deadbeef"),
		SnapshotURI:        "this/is/a/test",
		LastProcessedBlock: 0,
		EpochLength:        10,
		Status:             ApplicationStatusRunning,
	}

	err := s.database.InsertApplication(s.ctx, &app)
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

	err := s.database.InsertInput(s.ctx, &input)
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

	err := s.database.InsertInput(s.ctx, &input)
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

func (s *RepositorySuite) TestOutputFailsDuplicateRow() {
	output := Output{
		Index:                1,
		InputId:              1,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: nil,
	}

	err := s.database.InsertOutput(s.ctx, &output)
	s.Require().ErrorContains(err, "duplicate key value")
}

func (s *RepositorySuite) TestOutputFailsInputDoesntExist() {
	output := Output{
		Index:                10,
		InputId:              10,
		RawData:              common.Hex2Bytes("deadbeef"),
		OutputHashesSiblings: nil,
	}

	err := s.database.InsertOutput(s.ctx, &output)
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

func (s *RepositorySuite) TestReportFailsDuplicateRow() {
	report := Report{
		Index:   1,
		InputId: 1,
		RawData: common.Hex2Bytes("deadbeef"),
	}

	err := s.database.InsertReport(s.ctx, &report)
	s.Require().ErrorContains(err, "duplicate key value")
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

func (s *RepositorySuite) TestClaimExists() {
	genericHash := common.HexToHash("deadbeef")

	claim := Claim{
		Id:                   1,
		Status:               ClaimStatusPending,
		Index:                1,
		TransactionHash:      &genericHash,
		OutputMerkleRootHash: common.HexToHash("deadbeef"),
		AppAddress:           common.HexToAddress("deadbeef"),
	}

	response, err := s.database.GetClaim(s.ctx, common.HexToAddress("deadbeef"), 1)
	s.Require().Equal(claim, *response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestClaimDoesntExist() {
	response, err := s.database.GetClaim(s.ctx, common.HexToAddress("deadbeef"), 0)
	s.Require().Nil(response)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TestClaimFailsDuplicateRow() {
	claim := Claim{
		Status:               ClaimStatusPending,
		Index:                1,
		OutputMerkleRootHash: common.HexToHash("deadbeef"),
		AppAddress:           common.HexToAddress("deadbeef"),
	}

	err := s.database.InsertClaim(s.ctx, &claim)
	s.Require().ErrorContains(err, "duplicate key value")
}

func (s *RepositorySuite) TestClaimFailsApplicationDoesntExist() {
	claim := Claim{
		Status:               ClaimStatusPending,
		Index:                2,
		OutputMerkleRootHash: common.HexToHash("deadbeef"),
		AppAddress:           common.HexToAddress("deadbeefaaa"),
	}

	err := s.database.InsertClaim(s.ctx, &claim)
	s.Require().ErrorContains(err, "violates foreign key constraint")
}

func TestRepositorySuite(t *testing.T) {
	suite.Run(t, new(RepositorySuite))
}

// We use the postgres alpine docker image to test the repository.
func newPostgresContainer(ctx context.Context) (*postgres.PostgresContainer, error) {
	dbName := "postgres"
	dbUser := "postgres"
	dbPassword := "password"

	// Start the postgres container
	container, err := postgres.Run(
		ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)

	return container, err
}
