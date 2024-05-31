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

	err = s.database.SetupDatabaseState(s.ctx, 1, 1, 1)
	s.Require().Nil(err)
}

func (s *RepositorySuite) TearDownSuite() {
	err := s.postgres.Terminate(s.ctx)
	s.Nil(err)
	s.cancel()
}

func (s *RepositorySuite) TestInputExists() {

	input := Input{
		Index:            0,
		Status:           None,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      1,
		MachineStateHash: common.HexToHash("deadbeef"),
	}

	err := s.database.InsertInput(s.ctx, &input)
	s.Require().Nil(err)

	response := s.database.GetInput(s.ctx, 0)
	s.Require().Equal(&input, response)
}

func (s *RepositorySuite) TestInputFailsDoesntExist() {
	response := s.database.GetInput(s.ctx, 1)
	s.Require().Nil(response)
}

func (s *RepositorySuite) TestInputFailsDuplicateRow() {
	input := Input{
		Index:            0,
		Status:           None,
		Blob:             common.Hex2Bytes("deadbeef"),
		BlockNumber:      1,
		MachineStateHash: common.HexToHash("deadbeef"),
	}

	err := s.database.InsertInput(s.ctx, &input)

	s.Require().ErrorContains(err, "duplicate key value")
}

func (s *RepositorySuite) TestOutputExists() {
	output := Output{
		Index:      0,
		InputIndex: 0,
		Blob:       common.Hex2Bytes("deadbeef"),
	}

	err := s.database.InsertOutput(s.ctx, true, &output)
	s.Require().Nil(err)

	response := s.database.GetOutput(s.ctx, true, 0, 0)
	s.Require().Equal(&output, response)
}

func (s *RepositorySuite) TestOutputFailsDoesntExist() {
	response := s.database.GetOutput(s.ctx, false, 1, 1)
	s.Require().Nil(response)
}

func (s *RepositorySuite) TestOutputFailsDuplicateRow() {
	output := Output{
		Index:      0,
		InputIndex: 0,
		Blob:       common.Hex2Bytes("deadbeef"),
	}

	err := s.database.InsertOutput(s.ctx, true, &output)
	s.Require().ErrorContains(err, "duplicate key value")
}

func (s *RepositorySuite) TestOutputFailsInputDoesntExist() {
	output := Output{
		Index:      0,
		InputIndex: 1,
		Blob:       common.Hex2Bytes("deadbeef"),
	}

	err := s.database.InsertOutput(s.ctx, true, &output)
	s.Require().ErrorContains(err, "violates foreign key constraint")
}

func (s *RepositorySuite) TestClaimExists() {
	inputRange := InputRange{
		First: 1,
		Last:  2,
	}
	claim := Claim{
		Id:         0,
		Epoch:      1,
		InputRange: inputRange,
		EpochHash:  common.HexToHash("deadbeef"),
		AppAddress: common.HexToAddress("deadbeef"),
	}

	err := s.database.InsertClaim(s.ctx, &claim)
	s.Require().Nil(err)

	response := s.database.GetClaim(s.ctx, 0)
	s.Require().Equal(claim, *response)
}

func (s *RepositorySuite) TestClaimFailsDoesntExist() {
	response := s.database.GetClaim(s.ctx, 1)
	s.Require().Nil(response)
}

func (s *RepositorySuite) TestClaimFailsDuplicateRow() {
	inputRange := InputRange{
		First: 1,
		Last:  2,
	}
	claim := Claim{
		Id:         0,
		Epoch:      1,
		InputRange: inputRange,
		EpochHash:  common.HexToHash("deadbeef"),
		AppAddress: common.HexToAddress("deadbeef"),
	}

	err := s.database.InsertClaim(s.ctx, &claim)
	s.Require().ErrorContains(err, "duplicate key value")
}

func (s *RepositorySuite) TestClaimFailsEpochDoesntExist() {
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

	err := s.database.InsertClaim(s.ctx, &claim)
	s.Require().ErrorContains(err, "violates foreign key constraint")
}

func (s *RepositorySuite) TestProofExists() {
	inputRange := InputRange{
		First: 1,
		Last:  2,
	}
	var siblings []Hash
	siblings = append(siblings, common.HexToHash("deadbeef"))

	proof := Proof{
		InputIndex:                       0,
		ClaimId:                          0,
		InputRange:                       inputRange,
		InputIndexWithinEpoch:            0,
		OutputIndexWithinInput:           0,
		OutputHashesRootHash:             common.HexToHash("deadbeef"),
		OutputsEpochRootHash:             common.HexToHash("deadbeef"),
		MachineStateHash:                 common.HexToHash("deadbeef"),
		OutputHashInOutputHashesSiblings: siblings,
		OutputHashesInEpochSiblings:      siblings,
	}

	err := s.database.InsertProof(s.ctx, &proof)
	s.Require().Nil(err)

	response := s.database.GetProof(s.ctx, 0, 0)
	s.Require().Equal(proof, *response)
}

func (s *RepositorySuite) TestProofFailsDoesntExist() {
	response := s.database.GetProof(s.ctx, 1, 0)
	s.Require().Nil(response)
}

func (s *RepositorySuite) TestProofFailsDuplicateRow() {
	inputRange := InputRange{
		First: 1,
		Last:  2,
	}
	var siblings []Hash
	siblings = append(siblings, common.HexToHash("deadbeef"))

	proof := Proof{
		InputIndex:                       0,
		ClaimId:                          0,
		InputRange:                       inputRange,
		InputIndexWithinEpoch:            0,
		OutputIndexWithinInput:           0,
		OutputHashesRootHash:             common.HexToHash("deadbeef"),
		OutputsEpochRootHash:             common.HexToHash("deadbeef"),
		MachineStateHash:                 common.HexToHash("deadbeef"),
		OutputHashInOutputHashesSiblings: siblings,
		OutputHashesInEpochSiblings:      siblings,
	}

	err := s.database.InsertProof(s.ctx, &proof)
	s.Require().ErrorContains(err, "duplicate key value")
}

func (s *RepositorySuite) TestProofFailsClaimDoesntExist() {
	inputRange := InputRange{
		First: 1,
		Last:  2,
	}
	var siblings []Hash
	siblings = append(siblings, common.HexToHash("deadbeef"))

	proof := Proof{
		InputIndex:                       0,
		ClaimId:                          10,
		InputRange:                       inputRange,
		InputIndexWithinEpoch:            0,
		OutputIndexWithinInput:           1,
		OutputHashesRootHash:             common.HexToHash("deadbeef"),
		OutputsEpochRootHash:             common.HexToHash("deadbeef"),
		MachineStateHash:                 common.HexToHash("deadbeef"),
		OutputHashInOutputHashesSiblings: siblings,
		OutputHashesInEpochSiblings:      siblings,
	}

	err := s.database.InsertProof(s.ctx, &proof)
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
	container, err := postgres.RunContainer(
		ctx,
		testcontainers.WithImage("postgres:16-alpine"),
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
