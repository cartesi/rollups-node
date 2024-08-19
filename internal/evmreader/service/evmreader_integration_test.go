// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/deps"
	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/testutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
)

const (
	payload                 = "0xdeadbeef"
	blockTimestampInSeconds = 7000000000
	testTimeout             = 600 * time.Second
)

type EvmReaderIntegrationTestSuite struct {
	suite.Suite
	containers             *deps.DepsContainers
	ctx                    context.Context
	cancel                 context.CancelFunc
	serviceErr             chan error
	blockchainHttpEndpoint string
	db                     *repository.Database
	applicationAddress     common.Address
}

func TestInputReaderIntegrationSuite(t *testing.T) {
	suite.Run(t, new(EvmReaderIntegrationTestSuite))
}

func (s *EvmReaderIntegrationTestSuite) SetupTest() {

	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	// Run deps
	var depsConfig = deps.DepsConfig{
		Postgres: &deps.PostgresConfig{
			DockerImage: deps.DefaultPostgresDockerImage,
			Port:        testutil.GetCartesiTestDepsPortRange(),
			Password:    deps.DefaultPostgresPassword,
		},
		Devnet: &deps.DevnetConfig{
			DockerImage: deps.DefaultDevnetDockerImage,
			Port:        testutil.GetCartesiTestDepsPortRange(),
			NoMining:    true,
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

	// Capture ChainID
	chainId, err := ethutil.GetChainId(s.ctx, devnetHttpEndpoint)
	s.Require().Nil(err)

	// Fix the Blockchain timestamp. Must be "in the future"
	err = ethutil.SetNextDevnetBlockTimestamp(s.ctx, devnetHttpEndpoint, blockTimestampInSeconds)
	s.Require().Nil(err)

	// run database migrations
	schemaManager, err := repository.NewSchemaManager(postgresUrl.String())
	s.Require().Nil(err)
	err = schemaManager.Upgrade()
	s.Require().Nil(err)

	// Setup the database
	s.db, err = repository.Connect(s.ctx, fmt.Sprintf("%v?sslmode=disable", postgresUrl))
	s.Require().Nil(err)

	// Setup Evm Reader Service
	book := addresses.GetTestBook()

	nodePersistentConfig := &model.NodePersistentConfig{
		DefaultBlock:            model.DefaultBlockStatusLatest,
		InputBoxAddress:         book.InputBox,
		ChainId:                 chainId.Uint64(),
		InputBoxDeploymentBlock: 15,
	}

	err = s.db.InsertNodeConfig(s.ctx, nodePersistentConfig)
	s.Require().Nil(err)

	inputReaderService := NewEvmReaderService(
		devnetHttpEndpoint,
		devnetWsEndpoint,
		s.db,
		1,
		1,
	)

	ready := make(chan struct{}, 1)
	serviceErr := make(chan error, 1)

	s.serviceErr = serviceErr

	// Add Applications
	s.applicationAddress = book.Application
	err = s.db.InsertApplication(s.ctx, &model.Application{
		ContractAddress:    s.applicationAddress,
		Status:             model.ApplicationStatusRunning,
		LastProcessedBlock: 0,
	})
	s.Require().Nil(err)

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

func (s *EvmReaderIntegrationTestSuite) TearDownTest() {

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

func (s *EvmReaderIntegrationTestSuite) TestAddInput() {

	//s.T().Skip("Skipping this test because epoch algorithm is not implemented in the EVM Reader")

	// Send Input
	indexChan := make(chan int)
	go func() {
		index, err := ethutil.AddInputUsingFoundryMnemonic(s.ctx, s.blockchainHttpEndpoint, payload)
		s.Require().Nil(err)
		s.Require().Equal(0, index)
		indexChan <- index

	}()

	// Wait a little to input transaction to be sent
	time.Sleep(5 * time.Second)
	// Mine one block
	blockNumber, err := ethutil.MineNewBlock(s.ctx, s.blockchainHttpEndpoint)
	s.Require().Nil(err)
	s.Require().Equal(uint64(0x10), blockNumber)

	index := <-indexChan
	// Wait a little so EvmReader can process the Input sent ( make it polling? )
	time.Sleep(5 * time.Second)
	input, err := s.db.GetInput(s.ctx, uint64(index), s.applicationAddress)
	s.Require().Nil(err)
	s.Require().NotNil(input)

	epoch, err := s.db.GetEpoch(s.ctx, 1, input.AppAddress)
	s.Require().Nil(err)
	s.Require().NotNil(epoch)
	s.Require().Equal(model.EpochStatusOpen, epoch.Status)

	// Mine four more blocks
	for i := 0; i < 4; i++ {
		blockNumber, err = ethutil.MineNewBlock(s.ctx, s.blockchainHttpEndpoint)
		s.Require().Nil(err)
		s.Require().Equal(uint64(0x11+i), blockNumber)
	}

	// Wait a little so EvmReader can process ( make it polling? )
	time.Sleep(5 * time.Second)
	epoch, err = s.db.GetEpoch(s.ctx, 1, input.AppAddress)
	s.Require().Nil(err)
	s.Require().NotNil(epoch)
	s.Require().Equal(model.EpochStatusClosed, epoch.Status)

}
