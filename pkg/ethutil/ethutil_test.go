// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
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
	txOptsFactory        TransactOptsFactory
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

	txOpts, err := bind.NewKeyedTransactorWithChainID(privateKey, chainId)
	s.Require().Nil(err)
	s.txOptsFactory = NewStaticTransactOptsFactory(txOpts)

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

	sender := s.txOptsFactory.From()
	payload := common.Hex2Bytes("deadbeef")

	indexChan := make(chan uint64)
	errChan := make(chan error)

	waitGroup := sync.WaitGroup{}
	waitGroup.Add(1)

	go func() {
		waitGroup.Done()
		inputIndex, _, _, err := AddInput(s.ctx, s.client, s.txOptsFactory, s.inputBoxAddr, s.appAddr, payload)
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

func TestAddInputAsyncUsesContextForBindingTransaction(t *testing.T) {
	timeout := 20 * time.Millisecond

	srv := rpc.NewServer()
	backend := &addInputAsyncContextBackend{estimateGasTimeout: 10 * timeout}
	require.NoError(t, srv.RegisterName("eth", backend))
	defer srv.Stop()

	client := ethclient.NewClient(rpc.DialInProc(srv))
	defer client.Close()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	txOpts, err := bind.NewKeyedTransactorWithChainID(privateKey, big.NewInt(1))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err = AddInputAsync(
		ctx,
		client,
		NewStaticTransactOptsFactory(txOpts),
		common.HexToAddress("0x1000000000000000000000000000000000000001"),
		common.HexToAddress("0x2000000000000000000000000000000000000002"),
		[]byte("payload"),
	)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.True(t, backend.estimateGasCalled, "expected AddInputAsync to reach the binding gas-estimation boundary")
}

type addInputAsyncContextBackend struct {
	estimateGasCalled  bool
	estimateGasTimeout time.Duration
}

func (b *addInputAsyncContextBackend) GetTransactionCount(
	context.Context,
	common.Address,
	string,
) (hexutil.Uint64, error) {
	return 0, nil
}

func (b *addInputAsyncContextBackend) GasPrice(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(1)), nil
}

func (b *addInputAsyncContextBackend) GetCode(
	context.Context,
	common.Address,
	string,
) (hexutil.Bytes, error) {
	return hexutil.Bytes{0x01}, nil
}

func (b *addInputAsyncContextBackend) EstimateGas(
	ctx context.Context,
	_ map[string]interface{},
) (hexutil.Uint64, error) {
	b.estimateGasCalled = true
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(b.estimateGasTimeout):
		return 0, errors.New("eth_estimateGas was not called with the AddInputAsync context")
	}
}
