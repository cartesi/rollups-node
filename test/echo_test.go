// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests
// +build endtoendtests

package endtoendtests

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/cartesi/rollups-node/internal/deps"
	"github.com/cartesi/rollups-node/internal/machine"
	"github.com/cartesi/rollups-node/internal/node"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/readerclient"
	"github.com/stretchr/testify/suite"
)

const (
	payload                    = "0xdeadbeef"
	maxReadInputAttempts       = 10
	defaulTxDatabaseFile       = "default_tx_database"
	blockTimestampinSeconds    = 7000000000
	testTimeout                = 300 * time.Second
	devNetAdvanceTimeInSeconds = 120
)

type EchoInputTestSuite struct {
	suite.Suite
	containers    *deps.DepsContainers
	ctx           context.Context
	cancel        context.CancelFunc
	tempDir       string
	supervisorErr chan error
}

//go:embed data/echo_input/expected_input_with_proofs.json
var expectedInputJsonBytes []byte

func (s *EchoInputTestSuite) SetupTest() {

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)

	// Clear default_tx_database
	cwd, err := os.Getwd()
	s.Require().Nil(err)

	_ = os.Remove(filepath.Join(cwd, defaulTxDatabaseFile))

	// Create machine snapshot
	tempDir, err := os.MkdirTemp("", "machine-snapshot")
	s.Require().Nil(err)
	machine.Save("cartesi/rollups-node-snapshot:devel", tempDir, "test-echo-app")

	// Run deps

	var depsConfig = deps.NewDefaultDepsConfig()
	// This ensures input will be added at the first block, making this test
	// really reproducible
	depsConfig.Devnet.BlockTime = "2"
	depsContainers, err := deps.Run(ctx, *depsConfig)
	s.Require().Nil(err)

	// Fix the Blochain timestamp. Must be "in the future"
	err = ethutil.SetNextDevnetBlockTimestamp(ctx, LocalBlockchainHttpEndpoint, blockTimestampinSeconds)
	s.Require().Nil(err)

	// Run Node Service
	nodeConfig := NewLocalNodeConfig()

	nodeConfig.SnapshotDir = tempDir

	//ctx, cancel := context.WithTimeout(ctx, testTimeout)
	supervisor, err := node.Setup(ctx, nodeConfig)
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

	// Remove machine snpshot
	os.RemoveAll(s.tempDir)

	// Terminate deps
	ctx := context.Background()
	err := deps.Terminate(ctx, s.containers)
	s.Require().Nil(err)

	// Clear default_tx_database
	cwd, err := os.Getwd()
	s.Require().Nil(err)

	_ = os.Remove(filepath.Join(cwd, defaulTxDatabaseFile))

}

func (s *EchoInputTestSuite) TestSendInput() {

	inputIndex, err := ethutil.AddInputUsingFoundryMneumonic(s.ctx, LocalBlockchainHttpEndpoint, payload)
	s.Require().Nil(err)

	// Check input was correctly added to the blockchain
	s.Require().Equal(0, inputIndex)

	s.Require().Nil(ethutil.AdvanceDevnetTime(s.ctx, LocalBlockchainHttpEndpoint, devNetAdvanceTimeInSeconds))

	// Get Input with vouchers and proofs
	graphQlClient := graphql.NewClient("http://localhost:10000/graphql", nil)
	getInputChan := make(chan *readerclient.Input, 1)
	getInputErr := make(chan struct{}, 1)

	go func() {
		var resp *readerclient.Input
		attempts := 0
		for ; attempts < maxReadInputAttempts; attempts++ {
			time.Sleep(2 * time.Second)
			resp, err = readerclient.GetInput(s.ctx, graphQlClient, inputIndex)
			if err == nil && resp.Status == "ACCEPTED" &&
				resp.Vouchers != nil && resp.Vouchers[0].Proof != nil &&
				resp.Notices != nil && resp.Notices[0].Proof != nil {
				break
			}
		}
		if attempts == maxReadInputAttempts {
			getInputErr <- struct{}{}
			return
		}
		getInputChan <- resp
	}()

	select {
	case input := <-getInputChan:

		//Check Input
		var expectedInput readerclient.Input
		err = json.Unmarshal(expectedInputJsonBytes, &expectedInput)
		s.Require().Nil(err)
		s.Require().EqualValues(&expectedInput, input)

		break
	case err = <-s.supervisorErr:
		s.Require().Nil(err)
		break
	case <-getInputErr:
		s.T().FailNow()
	}

}

func TestEchoInput(t *testing.T) {

	suite.Run(t, new(EchoInputTestSuite))
}
