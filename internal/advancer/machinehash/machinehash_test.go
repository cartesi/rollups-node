// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machinehash

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/advancer/snapshot"
	"github.com/cartesi/rollups-node/internal/node/config"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
)

type ValidateMachineHashSuite struct {
	suite.Suite
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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	machineDir, err := snapshot.CreateDefaultMachineSnapshot()
	s.Require().Nil(err)
	defer os.RemoveAll(machineDir)

	appAddr := common.HexToAddress("0x0000000000000000000000000000000000000000")
	err = ValidateMachineHash(
		ctx,
		machineDir,
		appAddr,
		config.GetBlockchainHttpEndpoint(),
	)
	s.ErrorIs(err, context.DeadlineExceeded)
}

func (s *ValidateMachineHashSuite) TestItFailsWhenHttpEndpointIsInvalid() {
	machineDir, err := snapshot.CreateDefaultMachineSnapshot()
	s.Require().Nil(err)
	defer os.RemoveAll(machineDir)

	appAddr := common.HexToAddress("0x0000000000000000000000000000000000000000")
	err = ValidateMachineHash(
		context.Background(),
		machineDir,
		appAddr,
		"http://invalid-endpoint:8545",
	)
	s.ErrorContains(err, "no such host")
}

func (s *ValidateMachineHashSuite) TestItFailsWhenAppAddressIsInvalid() {
	machineDir, err := snapshot.CreateDefaultMachineSnapshot()
	s.Require().Nil(err)
	defer os.RemoveAll(machineDir)

	ctx := context.Background()
	appAddr := common.HexToAddress("invalid address")
	err = ValidateMachineHash(
		ctx,
		machineDir,
		appAddr,
		config.GetBlockchainHttpEndpoint(),
	)
	s.ErrorContains(err, "no contract code at given address")
}

func (s *ValidateMachineHashSuite) TestItFailsWhenAppAddressIsWrong() {
	machineDir, err := snapshot.CreateDefaultMachineSnapshot()
	s.Require().Nil(err)
	defer os.RemoveAll(machineDir)

	templateHash, err := snapshot.ReadHash(machineDir)
	s.Require().Nil(err)

	ctx := context.Background()
	_, cleanup, err := ethutil.CreateAnvilSnapshotAndDeployApp(ctx, config.GetBlockchainHttpEndpoint(), templateHash)
	s.Require().Nil(err)
	defer cleanup()

	wrongAppAddr := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	err = ValidateMachineHash(
		ctx,
		machineDir,
		wrongAppAddr,
		config.GetBlockchainHttpEndpoint(),
	)
	s.ErrorContains(err, "no contract code at given address")
}

func (s *ValidateMachineHashSuite) TestItSucceedsWhenHashesAreEqual() {
	machineDir, err := snapshot.CreateDefaultMachineSnapshot()
	s.Require().Nil(err)
	defer os.RemoveAll(machineDir)

	templateHash, err := snapshot.ReadHash(machineDir)
	s.Require().Nil(err)

	ctx := context.Background()
	appAddr, cleanup, err := ethutil.CreateAnvilSnapshotAndDeployApp(ctx, config.GetBlockchainHttpEndpoint(), templateHash)
	s.Require().Nil(err)
	defer cleanup()

	err = ValidateMachineHash(
		ctx,
		machineDir,
		appAddr,
		config.GetBlockchainHttpEndpoint(),
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
