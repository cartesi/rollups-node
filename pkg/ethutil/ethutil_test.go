// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
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
	client   *ethclient.Client
	endpoint string
	signer   Signer
	book     *addresses.Book
	appAddr  common.Address
}

func (s *EthUtilSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	var err error
	s.client, err = ethclient.DialContext(s.ctx, s.endpoint)
	s.Require().Nil(err)

	s.signer, err = NewMnemonicSigner(s.ctx, s.client, FoundryMnemonic, 0)
	s.Require().Nil(err)

	s.book, err = addresses.GetBookFromFile("deployment.json") // FIXME
	s.Require().Nil(err)

	s.appAddr = common.HexToAddress("0x0000000000000000000000000000000000000000") // FIXME
}

func (s *EthUtilSuite) TearDownTest() {
	// TODO revert anvil snapshot
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
	blockNumber, err := MineNewBlock(s.ctx, s.endpoint)
	s.Require().Nil(err)
	s.Require().Equal(uint64(inputBoxDeploymentBlockNumber+1), blockNumber)

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
	blockNumber, err := MineNewBlock(s.ctx, s.endpoint)
	s.Require().Nil(err)
	s.Require().Equal(uint64(inputBoxDeploymentBlockNumber+1), blockNumber)

}

func TestEthUtilSuite(t *testing.T) {
	suite.Run(t, new(EthUtilSuite))
}
