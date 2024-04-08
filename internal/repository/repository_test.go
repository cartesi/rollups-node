// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"testing"
	"time"

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
}

func (s *RepositorySuite) TearDownSuite() {
	err := s.postgres.Terminate(s.ctx)
	s.Nil(err)
	s.cancel()
}

func (s *RepositorySuite) TestInputExists() {

	input := Input{
		Index:  0,
		Status: "UNPROCESSED",
		Blob:   common.Hex2Bytes("deadbeef"),
	}

	err := s.database.InsertInput(s.ctx, &input)
	s.Require().Nil(err)

	response, err := s.database.GetInput(s.ctx, 0)
	s.Require().Nil(err)

	s.Require().Equal(&input, response)
}

func (s *RepositorySuite) TestInputFailsDoesntExist() {
	response, err := s.database.GetInput(s.ctx, 1)
	s.Require().Nil(response)

	s.Require().ErrorContains(err, "no rows in result set")
}

func (s *RepositorySuite) TestInputFailsDuplicateRow() {
	input := Input{
		Index:  0,
		Status: "UNPROCESSED",
		Blob:   common.Hex2Bytes("deadbeef"),
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

	response, err := s.database.GetOutput(s.ctx, true, 0, 0)
	s.Require().Nil(err)

	s.Require().Equal(&output, response)
}

func (s *RepositorySuite) TestOutputFailsDoesntExist() {
	response, err := s.database.GetOutput(s.ctx, false, 1, 1)
	s.Require().Nil(response)

	s.Require().ErrorContains(err, "no rows in result set")

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

func TestRepositorySuite(t *testing.T) {
	suite.Run(t, new(RepositorySuite))
}

// We use the postgres alpine docker image to test the repository.
func newPostgresContainer(ctx context.Context) (*postgres.PostgresContainer, error) {
	dbName := "postgres"
	dbUser := "postgres"
	dbPassword := "password"

	// 1. Start the postgres container and run any migrations on it
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
