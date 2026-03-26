// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"crypto/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

const testTimeout = 300 * time.Second

// This suite sets up a container running a devnet Ethereum node
// and connects to it using go-ethereum's client.
type EthUtilSuite struct {
	suite.Suite
	ctx                  context.Context
	cancel               context.CancelFunc
	client               *ethclient.Client
	endpoint             config.URL
	txOpts               *bind.TransactOpts
	inputBoxAddr         common.Address
	selfHostedAppFactory common.Address
	appAddr              common.Address
	machineDir           string
	cleanup              func()
}

func (s *EthUtilSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	var err error
	s.endpoint, err = config.GetBlockchainHttpEndpoint()
	s.Require().Nil(err)

	s.client, err = ethclient.DialContext(s.ctx, s.endpoint.Raw())
	s.Require().Nil(err)

	chainId, err := s.client.ChainID(s.ctx)
	s.Require().Nil(err)

	privateKey, err := MnemonicToPrivateKey(FoundryMnemonic, 0)
	s.Require().Nil(err)

	s.txOpts, err = bind.NewKeyedTransactorWithChainID(privateKey, chainId)
	s.Require().Nil(err)

	s.selfHostedAppFactory, err = config.GetContractsSelfHostedApplicationFactoryAddress()
	s.Require().Nil(err)

	var templateHash common.Hash
	_, err = rand.Read(templateHash[:])
	s.Require().Nil(err)

	s.inputBoxAddr, err = config.GetContractsInputBoxAddress()
	s.Require().Nil(err)

	_, _, encodedDA, err := DefaultDA(s.client, s.inputBoxAddr)
	salt := "0000000000000000000000000000000000000000000000000000000000000000"
	s.appAddr, s.cleanup, err = CreateAnvilSnapshotAndDeployApp(s.ctx, s.client, s.selfHostedAppFactory, templateHash, encodedDA, salt)
	s.Require().Nil(err)
}

func (s *EthUtilSuite) TearDownTest() {
	os.RemoveAll(s.machineDir)
	if s.cleanup != nil {
		s.cleanup()
	}
	s.cancel()
}

func (s *EthUtilSuite) TestAddInput() {

	sender := s.txOpts.From
	payload := common.Hex2Bytes("deadbeef")

	indexChan := make(chan uint64)
	errChan := make(chan error)

	waitGroup := sync.WaitGroup{}
	waitGroup.Add(1)

	go func() {
		waitGroup.Done()
		inputIndex, _, _, err := AddInput(s.ctx, s.client, s.txOpts, s.inputBoxAddr, s.appAddr, payload)
		if err != nil {
			errChan <- err
			return
		}
		indexChan <- inputIndex
	}()

	waitGroup.Wait()
	time.Sleep(1 * time.Second)
	_, err := MineNewBlock(s.ctx, s.client)
	s.Require().Nil(err)

	select {
	case err := <-errChan:
		s.Require().FailNow("Unexpected Error", err)
	case inputIndex := <-indexChan:
		s.Require().Equal(uint64(0), inputIndex)

		event, err := GetInputFromInputBox(s.client, s.inputBoxAddr, s.appAddr, inputIndex)
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
	blockNumber, err := MineNewBlock(s.ctx, s.client)
	s.Require().Nil(err)
	s.Require().Equal(prevBlockNumber+1, blockNumber)

}

func TestEthUtilSuite(t *testing.T) {
	suite.Run(t, new(EthUtilSuite))
}
