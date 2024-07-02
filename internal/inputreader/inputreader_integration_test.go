// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inputreader

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/deps"
	"github.com/cartesi/rollups-node/internal/machine"
	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/testutil"
	"github.com/stretchr/testify/suite"
)

const (
	payload                 = "0xdeadbeef"
	blockTimestampInSeconds = 7000000000
	testTimeout             = 300 * time.Second
)

type IntegrationTestRepository interface {
	InputReaderRepository
	GetInput(
		ctx context.Context,
		index uint64,
	) (*model.Input, error)
	SetupDatabaseState(
		ctx context.Context,
		deploymentBlock uint64,
		epochDuration uint64,
		currentEpoch uint64,
	) error
	Close()
}

type InputReaderIntegrationTestSuite struct {
	suite.Suite
	containers             *deps.DepsContainers
	ctx                    context.Context
	cancel                 context.CancelFunc
	serviceErr             chan error
	blockchainHttpEndpoint string
	db                     IntegrationTestRepository
}

func TestInputReaderIntegrationSuite(t *testing.T) {
	suite.Run(t, new(InputReaderIntegrationTestSuite))
}

func (s *InputReaderIntegrationTestSuite) SetupTest() {

	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	// Create tempdir
	tempDir, err := os.MkdirTemp("", "echo-test")
	s.Require().Nil(err)
	snapshotDir := path.Join(tempDir, "machine-snapshot")

	machine.Save("cartesi/rollups-node-snapshot:devel", snapshotDir, "test-echo-app")

	// Run deps
	var depsConfig = deps.DepsConfig{
		Postgres: &deps.PostgresConfig{
			DockerImage: deps.DefaultPostgresDockerImage,
			Port:        testutil.GetCartesiTestDepsPortRange(),
			Password:    deps.DefaultPostgresPassword,
		},
		Devnet: &deps.DevnetConfig{
			DockerImage:             deps.DefaultDevnetDockerImage,
			Port:                    testutil.GetCartesiTestDepsPortRange(),
			NoMining:                false,
			BlockToWaitForOnStartup: deps.DefaultDevnetBlockToWaitForOnStartup,
			BlockTime:               deps.DefaultDevnetBlockTime,
		},
	}

	depsContainers, err := deps.Run(s.ctx, depsConfig)
	s.Require().Nil(err)
	s.containers = depsContainers

	// Capture endpoints
	postgresEndpoint, err := depsContainers.PostgresEndpoint(s.ctx, "postgres")
	s.Require().Nil(err)

	postgresUrl, err := url.Parse(postgresEndpoint)
	s.Require().Nil(err)

	postgresUrl.User = url.UserPassword(deps.DefaultPostgresUser, deps.DefaultPostgresPassword)
	postgresUrl = postgresUrl.JoinPath(deps.DefaultPostgresDatabase)

	devnetHttpEndpoint, err := depsContainers.DevnetEndpoint(s.ctx, "http")
	s.Require().Nil(err)

	devnetWsEndpoint, err := depsContainers.DevnetEndpoint(s.ctx, "ws")
	s.Require().Nil(err)

	s.blockchainHttpEndpoint = devnetHttpEndpoint

	// Fix the Blockchain timestamp. Must be "in the future"
	err = ethutil.SetNextDevnetBlockTimestamp(s.ctx, devnetHttpEndpoint, blockTimestampInSeconds)
	s.Require().Nil(err)

	// run database migrations
	repository.RunMigrations(fmt.Sprintf("%v?sslmode=disable", postgresUrl))

	// Setup the database
	s.db, err = repository.Connect(s.ctx, fmt.Sprintf("%v?sslmode=disable", postgresUrl))
	s.Require().Nil(err)
	err = s.db.SetupDatabaseState(s.ctx, 0x11, 1, 0x11)
	s.Require().Nil(err)

	// Setup Input Reader Service
	book := addresses.GetTestBook()

	inputReaderService := NewInputReaderService(devnetHttpEndpoint, devnetWsEndpoint, postgresEndpoint, book.InputBox, uint64(0x10), book.Application)

	ready := make(chan struct{}, 1)
	serviceErr := make(chan error, 1)

	s.serviceErr = serviceErr

	//Start Service
	go func() {
		err := inputReaderService.Start(s.ctx, ready)
		if err != nil {
			serviceErr <- err
		}
	}()

	select {
	case err := <-serviceErr:
		s.FailNow("Unexpected error", err)
	case <-ready:
		break
	}

}

func (s *InputReaderIntegrationTestSuite) TearDownTest() {

	// Stop Input Reader
	if s.cancel != nil {
		s.cancel()
	}

	if s.db != nil {
		s.db.Close()
	}

	// Terminate deps
	if s.containers != nil {
		ctx := context.Background()
		err := deps.Terminate(ctx, s.containers)
		s.Require().Nil(err)
	}

}

func (s *InputReaderIntegrationTestSuite) TestAddInput() {

	// Send Input
	indexChan := make(chan int)
	go func() {
		index, err := ethutil.AddInputUsingFoundryMnemonic(s.ctx, s.blockchainHttpEndpoint, payload)
		s.Require().Nil(err)
		s.Require().Equal(0, index)
		indexChan <- index

	}()

	// mine more than 64 blocks. Anvil default epoch os 64 blocks wide
	// for i := 0; i < 70; i++ {
	// 	_, err := ethutil.MineNewBlock(s.ctx, s.blockchainHttpEndpoint)
	// 	s.Require().Nil(err)
	// }

	select {
	case index := <-indexChan:
		time.Sleep(66 * time.Second)
		input, err := s.db.GetInput(s.ctx, uint64(index))
		s.Require().Nil(err)
		s.Require().NotNil(input)
	}

}
