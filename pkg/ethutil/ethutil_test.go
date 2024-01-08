// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const testTimeout = 300 * time.Second

// This suite sets up a container running a devnet Ethereum node, and connects to it using
// go-ethereum's client.
type EthUtilSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	devNet testcontainers.Container
	client *ethclient.Client
	signer Signer
	book   *addresses.Book
}

func (s *EthUtilSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	var err error
	s.devNet, err = newDevNetContainer(s.ctx)
	s.Require().Nil(err)

	endpoint, err := s.devNet.Endpoint(s.ctx, "ws")
	s.Require().Nil(err)

	s.client, err = ethclient.DialContext(s.ctx, endpoint)
	s.Require().Nil(err)

	s.signer, err = NewMnemonicSigner(s.ctx, s.client, FoundryMnemonic, 0)
	s.Require().Nil(err)

	s.book = addresses.GetTestBook()
}

func (s *EthUtilSuite) TearDownTest() {
	err := s.devNet.Terminate(s.ctx)
	s.Nil(err)
	s.cancel()
}

func (s *EthUtilSuite) TestAddInput() {
	sender := common.HexToAddress("f39fd6e51aad88f6f4ce6ab8827279cfffb92266")
	payload := common.Hex2Bytes("deadbeef")

	inputIndex, err := AddInput(s.ctx, s.client, s.book, s.signer, payload)
	if !s.Nil(err) {
		s.logDevnetOutput()
		s.T().FailNow()
	}

	s.Require().Equal(0, inputIndex)

	event, err := GetInputFromInputBox(s.client, s.book, inputIndex)
	s.Require().Nil(err)
	s.Require().Equal(sender, event.Sender)
	s.Require().Equal(payload, event.Input)
}

// Log the output of the given container
func (s *EthUtilSuite) logDevnetOutput() {
	reader, err := s.devNet.Logs(s.ctx)
	s.Require().Nil(err)
	defer reader.Close()

	bytes, err := io.ReadAll(reader)
	s.Require().Nil(err)
	s.T().Log(string(bytes))
}

func TestEthUtilSuite(t *testing.T) {
	suite.Run(t, new(EthUtilSuite))
}

// We use the sunodo devnet docker image to test the client.
// This image starts an anvil node with the Rollups contracts already deployed.
func newDevNetContainer(ctx context.Context) (testcontainers.Container, error) {
	req := testcontainers.ContainerRequest{
		Image: "cartesi/rollups-devnet:devel",
		Env: map[string]string{
			"ANVIL_IP_ADDR": "0.0.0.0",
		},
		ExposedPorts: []string{"8545/tcp"},
		WaitingFor:   wait.ForLog("Listening on 0.0.0.0:8545"),
	}
	genericReq := testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	}
	return testcontainers.GenericContainer(ctx, genericReq)
}
