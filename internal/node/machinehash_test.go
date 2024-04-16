// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/deps"
	"github.com/cartesi/rollups-node/internal/machine"
	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/testutil"
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

	err = validateMachineHash(context.Background(), machineDir, "", "")
	s.ErrorContains(err, "no such file or directory")
}

func (s *ValidateMachineHashSuite) TestItFailsWhenHashHasWrongSize() {
	machineDir, err := mockMachineDir("deadbeef")
	s.Require().Nil(err)
	defer os.RemoveAll(machineDir)

	err = validateMachineHash(context.Background(), machineDir, "", "")
	s.ErrorContains(err, "wrong size")
}

func (s *ValidateMachineHashSuite) TestItFailsWhenContextIsCanceled() {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	machineDir, err := createMachineSnapshot()
	s.Require().Nil(err)
	defer os.RemoveAll(machineDir)

	dependencies, err := startDevnet()
	s.Require().Nil(err)
	defer func() {
		err = deps.Terminate(context.Background(), dependencies)
		s.Nil(err)
	}()

	blockchainHttpEndpoint, err := dependencies.DevnetEndpoint(context.Background(), "http")
	s.Require().Nil(err)

	err = validateMachineHash(
		ctx,
		machineDir,
		addresses.GetTestBook().CartesiDApp.String(),
		blockchainHttpEndpoint,
	)
	s.NotNil(err)
	s.ErrorIs(err, context.DeadlineExceeded)
}

func (s *ValidateMachineHashSuite) TestItSucceedsWhenHashesAreEqual() {
	ctx := context.Background()

	machineDir, err := createMachineSnapshot()
	s.Require().Nil(err)
	defer os.RemoveAll(machineDir)

	dependencies, err := startDevnet()
	s.Require().Nil(err)
	defer func() {
		err = deps.Terminate(context.Background(), dependencies)
		s.Nil(err)
	}()

	blockchainHttpEndpoint, err := dependencies.DevnetEndpoint(context.Background(), "http")
	s.Require().Nil(err)

	err = validateMachineHash(
		ctx,
		machineDir,
		addresses.GetTestBook().CartesiDApp.String(),
		blockchainHttpEndpoint,
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

// Generates a new Cartesi Machine snapshot in a temporary directory and returns
// its path
func createMachineSnapshot() (string, error) {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", err
	}
	if err = machine.Save(
		"cartesi/rollups-node-snapshot:devel",
		tmpDir,
		"snapshotTemp",
	); err != nil {
		return "", err
	}
	return tmpDir, nil
}

// Starts a devnet in a Docker container with the default parameters
func startDevnet() (*deps.DepsContainers, error) {
	container, err := deps.Run(context.Background(), deps.DepsConfig{
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
