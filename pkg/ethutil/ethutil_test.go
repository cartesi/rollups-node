// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/deps"
	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/testutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

const testTimeout = 300 * time.Second

// This suite sets up a container running a devnet Ethereum node, and connects to it using
// go-ethereum's client.
type EthUtilSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	deps   *deps.DepsContainers
	client *ethclient.Client
	signer Signer
	book   *addresses.Book
}

func (s *EthUtilSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	var err error
	s.deps, err = newDevNetContainer(context.Background())
	s.Require().Nil(err)

	endpoint, err := s.deps.DevnetEndpoint(s.ctx, "ws")
	s.Require().Nil(err)

	s.client, err = ethclient.DialContext(s.ctx, endpoint)
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
			DockerImage:             deps.DefaultDevnetDockerImage,
			BlockTime:               deps.DefaultBlockTime,
			BlockToWaitForOnStartup: deps.DefaultBlockToWaitForOnStartup,
			Port:                    testutil.GetCartesiTestDepsPortRange(),
		},
	})
	if err != nil {
		return nil, err
	}
	return container, nil
}
