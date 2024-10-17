// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machinehash

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/advancer/snapshot"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
)

const testTimeout = 300 * time.Second

type ValidateMachineHashSuite struct {
	suite.Suite
	ctx          context.Context
	cancel       context.CancelFunc
	endpoint     string
	book         *addresses.Book
	appAddr      common.Address
	machineDir   string
	templateHash string
	cleanup      func()
}

func (s *ValidateMachineHashSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	s.endpoint = config.GetBlockchainHttpEndpoint()

	var err error
	s.book, err = addresses.GetBookFromFile("../../../deployment.json") // FIXME
	s.Require().Nil(err)

	s.machineDir, err = snapshot.CreateDefaultMachineSnapshot()
	s.Require().Nil(err)

	s.templateHash, err = snapshot.ReadHash(s.machineDir)
	s.Require().Nil(err)

	s.appAddr, s.cleanup, err = ethutil.CreateAnvilSnapshotAndDeployApp(s.ctx, s.endpoint, s.book.SelfHostedApplicationFactory, s.templateHash)
	s.Require().Nil(err)
}

func (s *ValidateMachineHashSuite) TearDownTest() {
	os.RemoveAll(s.machineDir)
	if s.cleanup != nil {
		s.cleanup()
	}
	s.cancel()
}

func TestValidateMachineHash(t *testing.T) {
	suite.Run(t, new(ValidateMachineHashSuite))
}

func (s *ValidateMachineHashSuite) TestItFailsWhenSnapshotHasNoHash() {
	machineDir, err := os.MkdirTemp("", "")
	s.Require().Nil(err)
	defer os.RemoveAll(machineDir)

	appAddr := common.HexToAddress("0x0000000000000000000000000000000000000000")
	err = ValidateMachineHash(context.Background(), machineDir, appAddr, "")
	s.ErrorContains(err, "no such file or directory")
}

func (s *ValidateMachineHashSuite) TestItFailsWhenHashHasWrongSize() {
	machineDir, err := mockMachineDir("deadbeef")
	s.Require().Nil(err)
	defer os.RemoveAll(machineDir)

	appAddr := common.HexToAddress("0x0000000000000000000000000000000000000000")
	err = ValidateMachineHash(context.Background(), machineDir, appAddr, "")
	s.ErrorContains(err, "wrong size")
}

func (s *ValidateMachineHashSuite) TestItFailsWhenContextIsCanceled() {
	ctx, cancel := context.WithTimeout(context.Background(), 1)
	defer cancel()

	err := ValidateMachineHash(
		ctx,
		s.machineDir,
		s.appAddr,
		s.endpoint,
	)
	s.ErrorIs(err, context.DeadlineExceeded)
}

func (s *ValidateMachineHashSuite) TestItFailsWhenHttpEndpointIsInvalid() {
	err := ValidateMachineHash(
		s.ctx,
		s.machineDir,
		s.appAddr,
		"http://invalid-endpoint:8545",
	)
	s.ErrorContains(err, "no such host")
}

func (s *ValidateMachineHashSuite) TestItFailsWhenAppAddressIsInvalid() {
	appAddr := common.HexToAddress("invalid address")
	err := ValidateMachineHash(
		s.ctx,
		s.machineDir,
		appAddr,
		s.endpoint,
	)
	s.ErrorContains(err, "no contract code at given address")
}

func (s *ValidateMachineHashSuite) TestItFailsWhenAppAddressIsWrong() {
	wrongAppAddr := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	err := ValidateMachineHash(
		s.ctx,
		s.machineDir,
		wrongAppAddr,
		s.endpoint,
	)
	s.ErrorContains(err, "no contract code at given address")
}

func (s *ValidateMachineHashSuite) TestItSucceedsWhenHashesAreEqual() {
	err := ValidateMachineHash(
		s.ctx,
		s.machineDir,
		s.appAddr,
		s.endpoint,
	)
	s.Nil(err)
}

// ------------------------------------------------------------------------------------------------
// Auxiliary functions
// ------------------------------------------------------------------------------------------------

// Mocks the Cartesi Machine directory by creating a temporary directory with
// a single file named "hash" with the contents of `hash`, a hexadecimal string
func mockMachineDir(hash string) (string, error) {
	temp, err := os.MkdirTemp("", "")
	if err != nil {
		return "", err
	}
	hashFile := temp + "/hash"
	err = os.WriteFile(hashFile, common.FromHex(hash), os.ModePerm)
	if err != nil {
		return "", err
	}
	return temp, nil
}
