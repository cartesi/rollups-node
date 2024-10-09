// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/machine"
	"github.com/cartesi/rollups-node/internal/node/config"
	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

const testTimeout = 300 * time.Second

// This suite sets up a container running a devnet Ethereum node, and connects to it using
// go-ethereum's client.
type EthUtilSuite struct {
	suite.Suite
	ctx        context.Context
	cancel     context.CancelFunc
	client     *ethclient.Client
	endpoint   string
	signer     Signer
	book       *addresses.Book
	appAddr    common.Address
	machineDir string
	cleanup    func()
}

func (s *EthUtilSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	s.endpoint = config.GetBlockchainHttpEndpoint()

	var err error
	s.client, err = ethclient.DialContext(s.ctx, s.endpoint)
	s.Require().Nil(err)

	s.signer, err = NewMnemonicSigner(s.ctx, s.client, FoundryMnemonic, 0)
	s.Require().Nil(err)

	s.book, err = addresses.GetBookFromFile("../../deployment.json") // FIXME
	s.Require().Nil(err)

	s.machineDir, err = machine.CreateDefaultMachineSnapshot()
	s.Require().Nil(err)

	templateHash, err := machine.ReadHash(s.machineDir)
	s.Require().Nil(err)

	s.appAddr, s.cleanup, err = CreateAnvilSnapshotAndDeployApp(s.ctx, templateHash)
	s.Require().Nil(err)
}

func (s *EthUtilSuite) TearDownTest() {
	os.RemoveAll(s.machineDir)
	s.cleanup()
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
		inputIndex, err := AddInput(s.ctx, s.client, s.book, s.appAddr, s.signer, payload)
		if err != nil {
			errChan <- err
			return
		}
		indexChan <- inputIndex
	}()

	waitGroup.Wait()
	time.Sleep(1 * time.Second)
	_, err = MineNewBlock(s.ctx, s.endpoint)
	s.Require().Nil(err)

	select {
	case err := <-errChan:
		s.Require().FailNow("Unexpected Error", err)
	case inputIndex := <-indexChan:
		s.Require().Equal(0, inputIndex)

		event, err := GetInputFromInputBox(s.client, s.book, s.appAddr, inputIndex)
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
	prevBlockNumber, err := s.client.BlockNumber(s.ctx)
	s.Require().Nil(err)
	blockNumber, err := MineNewBlock(s.ctx, s.endpoint)
	s.Require().Nil(err)
	s.Require().Equal(prevBlockNumber+1, blockNumber)

}

func TestEthUtilSuite(t *testing.T) {
	suite.Run(t, new(EthUtilSuite))
}
