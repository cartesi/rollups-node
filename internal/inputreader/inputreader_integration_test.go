// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inputreader

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/cartesi/rollups-node/internal/deps"
	"github.com/cartesi/rollups-node/internal/machine"
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

type InputReaderIntegrationTestSuite struct {
	suite.Suite
	containers             *deps.DepsContainers
	ctx                    context.Context
	cancel                 context.CancelFunc
	serviceErr             chan error
	blockchainHttpEndpoint string
}

func (s *InputReaderIntegrationTestSuite) SetupTest() {

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)

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
			NoMining:                true,
			BlockToWaitForOnStartup: deps.DefaultDevnetBlockToWaitForOnStartup,
		},
	}

	depsContainers, err := deps.Run(ctx, depsConfig)
	s.Require().Nil(err)

	// Capture endpoints
	postgresEndpoint, err := depsContainers.PostgresEndpoint(ctx, "postgres")
	s.Require().Nil(err)

	postgresUrl, err := url.Parse(postgresEndpoint)
	s.Require().Nil(err)

	postgresUrl.User = url.UserPassword(deps.DefaultPostgresUser, deps.DefaultPostgresPassword)
	postgresUrl = postgresUrl.JoinPath(deps.DefaultPostgresDatabase)

	devnetHttpEndpoint, err := depsContainers.DevnetEndpoint(ctx, "http")
	s.Require().Nil(err)

	s.blockchainHttpEndpoint = devnetHttpEndpoint

	// Fix the Blockchain timestamp. Must be "in the future"
	err = ethutil.SetNextDevnetBlockTimestamp(ctx, devnetHttpEndpoint, blockTimestampInSeconds)
	s.Require().Nil(err)

	// Setup the database
	// run database migrations
	repository.RunMigrations(fmt.Sprintf("%v?sslmode=disable", postgresUrl))

	book := addresses.GetTestBook()

	inputReaderService := NewInputReaderService(devnetHttpEndpoint, postgresEndpoint, book.InputBox, uint64(0x10), book.Application)

	ready := make(chan struct{}, 1)
	serviceErr := make(chan error, 1)

	// Configure Suite for tear down
	s.containers = depsContainers
	s.ctx = ctx
	s.cancel = cancel
	s.serviceErr = serviceErr

	//Start Service
	go func() {
		err := inputReaderService.Start(ctx, ready)
		if err != nil {
			serviceErr <- err
		}
	}()

	select {
	case err := <-serviceErr:
		s.Require().Nil(err)
	case <-ready:
		break
	}

}

func (s *InputReaderIntegrationTestSuite) TearDownTest() {

	// Stop Node services
	s.cancel()

	// Terminate deps
	ctx := context.Background()
	err := deps.Terminate(ctx, s.containers)
	s.Require().Nil(err)

}

func (s *InputReaderIntegrationTestSuite) TestAddInput() {

	receipt, err := ethutil.AddInputUsingFoundryMnemonic(s.ctx, s.blockchainHttpEndpoint, payload)
	s.Require().Nil(err)
	s.Require().NotNil(receipt)

	_, err = ethutil.MineNewBlock(s.ctx, s.blockchainHttpEndpoint)
	s.Require().Nil(err)

	index, err := ethutil.GetInputIndex(s.ctx, client, book, receipt)
	s.Require().Equal(0, index)

	// Check input was correctly added to the blockchain
	s.Require().Equal(0, receipt)

}
