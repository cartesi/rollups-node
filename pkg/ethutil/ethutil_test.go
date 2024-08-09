// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/deps"
	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
	"github.com/cartesi/rollups-node/pkg/testutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

const testTimeout = 300 * time.Second
const inputBoxDeploymentBlockNumber = 0x0F

// This suite sets up a container running a devnet Ethereum node, and connects to it using
// go-ethereum's client.
type EthUtilSuite struct {
	suite.Suite
	ctx      context.Context
	cancel   context.CancelFunc
	deps     *deps.DepsContainers
	client   *ethclient.Client
	endpoint string
	signer   Signer
	book     *addresses.Book
}

func (s *EthUtilSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	var err error
	s.deps, err = newDevNetContainer(context.Background())
	s.Require().Nil(err)

	s.endpoint, err = s.deps.DevnetEndpoint(s.ctx, "ws")
	s.Require().Nil(err)

	s.client, err = ethclient.DialContext(s.ctx, s.endpoint)
	s.Require().Nil(err)

	s.signer, err = NewMnemonicSigner(s.ctx, s.client, FoundryMnemonic, 0)
	s.Require().Nil(err)

	s.book = addresses.GetTestBook()
}

func (s *EthUtilSuite) TearDownTest() {
	err := deps.Terminate(context.Background(), s.deps)
	s.Nil(err)
	s.cancel()
}

func (s *EthUtilSuite) TestAddInput() {

	signer, err := NewMnemonicSigner(s.ctx, s.client, FoundryMnemonic, 0)
	s.Require().Nil(err)

	sender := signer.Account()
	payload := common.Hex2Bytes("deadbeef")

	indexChan := make(chan int)
	errChan := make(chan error)

	waitGroup := sync.WaitGroup{}
	waitGroup.Add(1)

	go func() {
		waitGroup.Done()
		inputIndex, err := AddInput(s.ctx, s.client, s.book, s.signer, payload)
		if err != nil {
			errChan <- err
			return
		}
		indexChan <- inputIndex
	}()

	waitGroup.Wait()
	time.Sleep(1 * time.Second)
	blockNumber, err := MineNewBlock(s.ctx, s.endpoint)
	s.Require().Nil(err)
	s.Require().Equal(uint64(inputBoxDeploymentBlockNumber+1), blockNumber)

	select {
	case err := <-errChan:
		s.logDevnetOutput()
		s.Require().FailNow("Unexpected Error", err)
	case inputIndex := <-indexChan:
		s.Require().Equal(0, inputIndex)

		event, err := GetInputFromInputBox(s.client, s.book, inputIndex)
		s.Require().Nil(err)

		inputsABI, err := inputs.InputsMetaData.GetAbi()
		s.Require().Nil(err)
		advanceInputABI := inputsABI.Methods["EvmAdvance"]
		inputArgs := map[string]interface{}{}
		err = advanceInputABI.Inputs.UnpackIntoMap(inputArgs, event.Input[4:])
		s.Require().Nil(err)

		s.T().Log(inputArgs)
		s.Require().Equal(sender, inputArgs["msgSender"])
		s.Require().Equal(payload, inputArgs["payload"])
	}
}

func (s *EthUtilSuite) TestMineNewBlock() {
	blockNumber, err := MineNewBlock(s.ctx, s.endpoint)
	s.Require().Nil(err)
	s.Require().Equal(uint64(inputBoxDeploymentBlockNumber+1), blockNumber)

}

// Log the output of the given container
func (s *EthUtilSuite) logDevnetOutput() {
	reader, err := s.deps.DevnetLogs(s.ctx)
	s.Require().Nil(err)
	defer reader.Close()

	bytes, err := io.ReadAll(reader)
	s.Require().Nil(err)
	s.T().Log(string(bytes))
}

func TestEthUtilSuite(t *testing.T) {
	suite.Run(t, new(EthUtilSuite))
}

// We use the node devnet docker image to test the client.
// This image starts an anvil node with the Rollups contracts already deployed.
func newDevNetContainer(ctx context.Context) (*deps.DepsContainers, error) {

	container, err := deps.Run(ctx, deps.DepsConfig{
		Devnet: &deps.DevnetConfig{
			DockerImage: deps.DefaultDevnetDockerImage,
			NoMining:    true,
			Port:        testutil.GetCartesiTestDepsPortRange(),
		},
	})
	if err != nil {
		return nil, err
	}
	return container, nil
}
