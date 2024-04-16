// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests
// +build endtoendtests

package endtoendtests

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/cartesi/rollups-node/internal/deps"
	"github.com/cartesi/rollups-node/internal/machine"
	"github.com/cartesi/rollups-node/internal/node"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/readerclient"
	"github.com/cartesi/rollups-node/pkg/testutil"
	"github.com/stretchr/testify/suite"
)

const (
	payload                        = "0xdeadbeef"
	maxReadInputAttempts           = 10
	blockTimestampinSeconds        = 7000000000
	testTimeout                    = 300 * time.Second
	devNetAdvanceTimeInSeconds     = 120
	devNetMiningBlockTimeInSeconds = "2"
	graphqlEndpoint                = "http://localhost:10000/graphql"
)

type EchoInputTestSuite struct {
	suite.Suite
	containers             *deps.DepsContainers
	ctx                    context.Context
	cancel                 context.CancelFunc
	tempDir                string
	supervisorErr          chan error
	blockchainHttpEndpoint string
}

//go:embed data/echo_input/expected_input_with_proofs.json
var expectedInputJsonBytes []byte

func (s *EchoInputTestSuite) SetupTest() {

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)

	// Create Tempdir
	tempDir, err := os.MkdirTemp("", "echo-test")
	s.Require().Nil(err)
	snapshotDir := path.Join(tempDir, "machine-snapshot")

	machine.Save("cartesi/rollups-node-snapshot:devel", snapshotDir, "test-echo-app")

	// Run deps
	var depsConfig = deps.DepsConfig{
		&deps.PostgresConfig{
			DockerImage: deps.DefaultPostgresDockerImage,
			Port:        testutil.GetCartesiTestDepsPortRange(),
			Password:    deps.DefaultPostgresPassword,
		},
		&deps.DevnetConfig{
			DockerImage:             deps.DefaultDevnetDockerImage,
			Port:                    testutil.GetCartesiTestDepsPortRange(),
			BlockTime:               devNetMiningBlockTimeInSeconds,
			BlockToWaitForOnStartup: deps.DefaultBlockToWaitForOnStartup,
		},
	}

	depsContainers, err := deps.Run(ctx, depsConfig)
	s.Require().Nil(err)

	// Capture endpoints
	postgressEndpoint, err := depsContainers.PostgresEndpoint(ctx, "postgres")
	s.Require().Nil(err)

	postgresUrl, err := url.Parse(postgressEndpoint)
	s.Require().Nil(err)

	postgresUrl.User = url.UserPassword(deps.DefaultPostgresUser, deps.DefaultPostgresPassword)
	postgresUrl = postgresUrl.JoinPath(deps.DefaultPostgresDatabase)

	devnetHttpEndpoint, err := depsContainers.DevnetEndpoint(ctx, "http")
	s.Require().Nil(err)
	devnetWsEndpoint, err := depsContainers.DevnetEndpoint(ctx, "ws")
	s.Require().Nil(err)

	s.blockchainHttpEndpoint = devnetHttpEndpoint

	// Fix the Blochain timestamp. Must be "in the future"
	err = ethutil.SetNextDevnetBlockTimestamp(ctx, devnetHttpEndpoint, blockTimestampinSeconds)
	s.Require().Nil(err)

	// Run Node Service
	nodeConfig := NewLocalNodeConfig(postgresUrl.String(), devnetHttpEndpoint, devnetWsEndpoint, snapshotDir)

	supervisor, err := node.Setup(ctx, nodeConfig, tempDir)
	s.Require().Nil(err)

	ready := make(chan struct{}, 1)
	supervisorErr := make(chan error, 1)
	go func() {
		err := supervisor.Start(ctx, ready)
		if err != nil {
			supervisorErr <- err
		}
	}()

	select {
	case err := <-supervisorErr:
		s.Require().Nil(err)
	case <-ready:
		break
	}

	// Configure Suite for tear down
	s.containers = depsContainers
	s.tempDir = tempDir
	s.ctx = ctx
	s.cancel = cancel
	s.supervisorErr = supervisorErr

}

func (s *EchoInputTestSuite) TearDownTest() {

	// Stop Node services
	s.cancel()

	// Remove temp files
	os.RemoveAll(s.tempDir)

	// Terminate deps
	ctx := context.Background()
	err := deps.Terminate(ctx, s.containers)
	s.Require().Nil(err)

}

func (s *EchoInputTestSuite) TestSendInput() {

	inputIndex, err := ethutil.AddInputUsingFoundryMnemonic(s.ctx, s.blockchainHttpEndpoint, payload)
	s.Require().Nil(err)

	// Check input was correctly added to the blockchain
	s.Require().Equal(0, inputIndex)

	s.Require().Nil(ethutil.AdvanceDevnetTime(s.ctx, s.blockchainHttpEndpoint, devNetAdvanceTimeInSeconds))

	// Get Input with vouchers and proofs
	graphQlClient := graphql.NewClient(graphqlEndpoint, nil)
	var input *readerclient.Input
	attempts := 0
	for ; attempts < maxReadInputAttempts; attempts++ {
		time.Sleep(2 * time.Second)
		input, err = readerclient.GetInput(s.ctx, graphQlClient, inputIndex)
		if err == nil && input.Status == "ACCEPTED" &&
			input.Vouchers != nil && input.Vouchers[0].Proof != nil &&
			input.Notices != nil && input.Notices[0].Proof != nil {
			break
		}
	}
	if attempts == maxReadInputAttempts {
		select {
		case err = <-s.supervisorErr:
			s.Require().Nil(err)
			break
		case <-time.After(1 * time.Second):
			s.Require().FailNow("Reached max read attempts")
		}
	}

	//Check Input
	var expectedInput readerclient.Input
	err = json.Unmarshal(expectedInputJsonBytes, &expectedInput)
	s.Require().Nil(err)
	s.Require().EqualValues(&expectedInput, input)

}

func TestEchoInput(t *testing.T) {

	suite.Run(t, new(EchoInputTestSuite))
}
