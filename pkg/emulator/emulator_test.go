// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

import (
	"math"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	imagesPath = "/usr/share/cartesi-machine/images/"
	address    = "127.0.0.1:8081"
)

func init() {
	if value, hasValue := os.LookupEnv("CARTESI_TEST_MACHINE_IMAGES_PATH"); hasValue {
		imagesPath = value
	}
}

// ------------------------------------------------------------------------------------------------

func TestGetDefaultConfig(t *testing.T) {
	config, err := GetDefaultMachineConfig()
	require.Nil(t, err)
	require.NotNil(t, config)
}

func TestNewDefaultMachineConfig(t *testing.T) {
	config := NewDefaultMachineConfig()
	require.NotNil(t, config)
}

// ------------------------------------------------------------------------------------------------

// MachineSuite is run inside TestNew of both LocalMachineSuite and RemoteMachineSuite.
type MachineSuite struct {
	suite.Suite
	machine *Machine
}

func (suite *MachineSuite) TestGetInitialConfig() {
	require := suite.Require()
	config, err := suite.machine.GetInitialConfig()
	require.Nil(err)
	require.NotNil(config)
}

func (suite *MachineSuite) TestToggleIFlagsY() {
	require := suite.Require()

	iflagsY, err := suite.machine.ReadIFlagsY()
	require.Nil(err)
	require.False(iflagsY)

	err = suite.machine.SetIFlagsY()
	require.Nil(err)

	iflagsY, err = suite.machine.ReadIFlagsY()
	require.Nil(err)
	require.True(iflagsY)
}

func (suite *MachineSuite) TestToggleIFlagsH() {
	require := suite.Require()

	iflagsH, err := suite.machine.ReadIFlagsH()
	require.Nil(err)
	require.False(iflagsH)

	err = suite.machine.SetIFlagsH()
	require.Nil(err)

	iflagsH, err = suite.machine.ReadIFlagsH()
	require.Nil(err)
	require.True(iflagsH)
}

func (suite *MachineSuite) TestReadWrite() {
	require := suite.Require()
	const size = 1024

	// Reads the existing data.
	existingData, err := suite.machine.ReadMemory(rxBufferStartAddress, size)
	require.Nil(err)
	require.NotNil(existingData)
	require.Len(existingData, size)

	// Writes the new data.
	var newData [size]byte
	for i := 0; i < size; i++ {
		newData[i] = byte(i % 256)
	}
	require.NotEqual(newData[:], existingData)
	err = suite.machine.WriteMemory(rxBufferStartAddress, newData[:])
	require.Nil(err)

	// Reads the now newData.
	readData, err := suite.machine.ReadMemory(rxBufferStartAddress, size)
	require.Nil(err)
	require.NotNil(readData)
	require.Equal(newData[:], readData)

	// Creates an array of the size of rxBufferLength filled with 0xda.
	var newRxBufferData [rxBufferLength]byte
	for i := 0; i < rxBufferLength; i++ {
		newRxBufferData[i] = 0xda
	}

	// Create a temporary file and writes newRxBufferData to it.
	temp, err := os.CreateTemp("", "rxBuffer")
	require.Nil(err)
	defer os.Remove(temp.Name())
	_, err = temp.Write(newRxBufferData[:])
	require.Nil(err)
	temp.Close()

	// Initializes a MemoryRangeConfig pointing to this file.
	newRxBuffer := &MemoryRangeConfig{
		Start:         rxBufferStartAddress,
		Length:        rxBufferLength,
		Shared:        false,
		ImageFilename: temp.Name(),
	}

	// Replaces rxBuffer with newRxBuffer.
	err = suite.machine.ReplaceMemoryRange(newRxBuffer)
	require.Nil(err)

	// Reads the memory and checks if it was indeed replaced.
	rxData, err := suite.machine.ReadMemory(rxBufferStartAddress, rxBufferLength)
	require.Nil(err)
	require.NotNil(rxData)
	require.Equal(newRxBufferData[:], rxData)
}

// ------------------------------------------------------------------------------------------------

func TestLocalMachine(t *testing.T) {
	suite.Run(t, new(LocalMachineSuite))
}

type LocalMachineSuite struct{ suite.Suite }

func (s *LocalMachineSuite) TestNew() {
	require := s.Require()

	config := defaultConfig()
	runtime := &MachineRuntimeConfig{}

	machine, err := NewMachine(config, runtime)
	require.Nil(err)
	require.NotNil(machine)
	defer machine.Delete()

	suite.Run(s.T(), &MachineSuite{machine: machine})
}

func (s *LocalMachineSuite) TestHappyPath() {
	require := s.Require()

	runtime := &MachineRuntimeConfig{}

	machine, err := NewMachine(defaultConfig(), runtime)
	require.Nil(err)
	require.NotNil(machine)
	defer machine.Delete()

	hashBefore := assertInitialState(require, machine)
	hashAfterOneCycle := advanceOneCycle(require, machine)

	require.NotEqual(hashBefore.String(), hashAfterOneCycle.String())

	// Stores the machine.
	temp, err := os.MkdirTemp("", "testmachines")
	require.Nil(err)
	defer os.RemoveAll(temp)
	storePath := temp + "/machine"
	err = machine.Store(storePath)
	require.Nil(err)

	hashAfterHalt := runUntilHalt(require, machine)
	require.NotEqual(hashAfterHalt.String(), hashBefore.String())
	require.NotEqual(hashAfterHalt.String(), hashAfterOneCycle.String())

	{ // Loads the stored machine.
		storedMachine, err := LoadMachine(storePath, runtime)
		require.Nil(err)
		require.NotNil(storedMachine)
		defer storedMachine.Delete()

		storedHash, err := storedMachine.GetRootHash()
		require.Nil(err)
		require.Equal(hashAfterOneCycle.String(), storedHash.String())
	}
}

// ------------------------------------------------------------------------------------------------

func TestRemoteMachine(t *testing.T) {
	suite.Run(t, new(RemoteMachineSuite))
}

type RemoteMachineSuite struct{ suite.Suite }

func (s *RemoteMachineSuite) TestNew() {
	require := s.Require()

	// Launchs the remote server.
	cmd, serverAddress := launchRemoteServer(s.T())
	defer func() { require.Nil(cmd.Process.Kill()) }()

	// Connects to the remote server.
	remote, err := NewRemoteMachineManager(serverAddress)
	require.Nil(err)
	require.NotNil(remote)
	defer func() {
		defer remote.Delete()
		err := remote.Shutdown()
		require.Nil(err)
	}()

	s.Run("GetDefaultMachineConfig", func() {
		require := s.Require()
		var config *MachineConfig
		config, err = remote.GetDefaultMachineConfig()
		require.Nil(err)
		require.NotNil(config)
	})

	// Creates the machine.
	machine, err := remote.NewMachine(defaultConfig(), &MachineRuntimeConfig{})
	require.Nil(err)
	require.NotNil(machine)
	defer func() {
		defer machine.Delete()
		err := machine.Destroy()
		require.Nil(err)
	}()

	suite.Run(s.T(), &MachineSuite{machine: machine})
}

func (s *RemoteMachineSuite) TestHappyPath() {
	require := s.Require()

	// Launchs the remote server.
	cmd, serverAddress := launchRemoteServer(s.T())
	defer func() { require.Nil(cmd.Process.Kill()) }()

	// Connects to the remote server.
	remote, err := NewRemoteMachineManager(serverAddress)
	require.Nil(err)
	require.NotNil(remote)
	defer func() {
		defer remote.Delete()
		err := remote.Shutdown()
		require.Nil(err)
	}()

	// Creates the machine.
	runtime := &MachineRuntimeConfig{}
	machine, err := remote.NewMachine(defaultConfig(), runtime)
	require.Nil(err)
	require.NotNil(machine)
	defer func() {
		defer machine.Delete()
		err := machine.Destroy()
		require.Nil(err)
	}()

	hashBefore := assertInitialState(require, machine)
	hashAfterOneCycle := advanceOneCycle(require, machine)

	require.NotEqual(hashBefore.String(), hashAfterOneCycle.String())

	// Stores the machine.
	temp, err := os.MkdirTemp("", "testmachines")
	require.Nil(err)
	defer os.RemoveAll(temp)
	storePath := temp + "/machine"
	err = machine.Store(storePath)
	require.Nil(err)

	hashAfterHalt := runUntilHalt(require, machine)
	require.NotEqual(hashAfterHalt.String(), hashBefore.String())
	require.NotEqual(hashAfterHalt.String(), hashAfterOneCycle.String())

	// Forks the remote server.
	otherAddress, err := remote.Fork()
	require.Nil(err)
	require.NotEmpty(otherAddress)
	require.NotEqual(otherAddress, serverAddress)

	// Connects to the forked server.
	otherRemote, err := NewRemoteMachineManager(otherAddress)
	require.Nil(err)
	require.NotNil(otherRemote)
	defer func() {
		defer otherRemote.Delete()
		err := otherRemote.Shutdown()
		require.Nil(err)
	}()

	// Gets the forked machine.
	forkedMachine, err := otherRemote.GetMachine()
	require.Nil(err)
	require.NotNil(forkedMachine)
	defer func() {
		defer forkedMachine.Delete()
		err := forkedMachine.Destroy()
		require.Nil(err)
	}()

	forkedHash, err := forkedMachine.GetRootHash()
	require.Nil(err)
	require.Equal(hashAfterHalt.String(), forkedHash.String())

	// Destroys the machine on otherRemote.
	err = forkedMachine.Destroy()
	require.Nil(err)

	// Loads the stored machine on otherRemote.
	otherMachine, err := otherRemote.LoadMachine(storePath, runtime)
	require.Nil(err)
	require.NotNil(otherMachine)
	defer func() {
		defer otherMachine.Delete()
		err := otherMachine.Destroy()
		require.Nil(err)
	}()

	storedHash, err := otherMachine.GetRootHash()
	require.Nil(err)
	require.Equal(hashAfterOneCycle.String(), storedHash.String())
}

func (s *RemoteMachineSuite) TestSnapshot() {
	require := s.Require()

	// Launches the remote server.
	cmd, serverAddress := launchRemoteServer(s.T())
	defer func() { require.Nil(cmd.Process.Kill()) }()

	// Connects to the remote server.
	remote, err := NewRemoteMachineManager(serverAddress)
	require.Nil(err)
	require.NotNil(remote)
	defer func() {
		defer remote.Delete()
		err := remote.Shutdown()
		require.Nil(err)
	}()

	// Creates the machine.
	runtime := &MachineRuntimeConfig{}
	machine, err := remote.NewMachine(defaultConfig(), runtime)
	require.Nil(err)
	require.NotNil(machine)
	defer func() {
		defer machine.Delete()
		err := machine.Destroy()
		require.Nil(err)
	}()

	// Gets the current hash.
	hashBefore, err := machine.GetRootHash()
	require.Nil(err)

	// Takes a snapshot.
	err = machine.Snapshot()
	require.Nil(err)

	// Advances one cycle.
	reason, err := machine.Run(1)
	require.Nil(err)
	require.Equal(BreakReasonReachedTargetMcycle, reason)

	// Gets the new hash.
	hashAfter, err := machine.GetRootHash()
	require.Nil(err)
	require.NotEqual(hashBefore.String(), hashAfter.String())

	// Rolls back.
	err = machine.Rollback()
	require.Nil(err)

	// Compares the first hash with the hash after the rollback.
	hashAfterRollback, err := machine.GetRootHash()
	require.Nil(err)
	require.Equal(hashBefore.String(), hashAfterRollback.String())
}

// ------------------------------------------------------------------------------------------------

func assertInitialState(require *require.Assertions, machine *Machine) MerkleTreeHash {
	iflagsH, err := machine.ReadIFlagsH()
	require.Nil(err)
	require.False(iflagsH)

	cycle, err := machine.ReadCSR(ProcCsrMcycle)
	require.Nil(err)
	require.Equal(uint64(0), cycle)

	hash, err := machine.GetRootHash()
	require.Nil(err)

	return hash
}

func advanceOneCycle(require *require.Assertions, machine *Machine) MerkleTreeHash {
	reason, err := machine.Run(1)
	require.Nil(err)
	require.Equal(BreakReasonReachedTargetMcycle, reason)

	iflagsH, err := machine.ReadIFlagsH()
	require.Nil(err)
	require.False(iflagsH)

	cycle, err := machine.ReadCSR(ProcCsrMcycle)
	require.Nil(err)
	require.Equal(uint64(1), cycle)

	hash, err := machine.GetRootHash()
	require.Nil(err)

	return hash
}

func runUntilHalt(require *require.Assertions, machine *Machine) MerkleTreeHash {
	reason, err := machine.Run(math.MaxUint64)
	require.Nil(err)
	require.Equal(BreakReasonHalted, reason)

	iflagsH, err := machine.ReadIFlagsH()
	require.Nil(err)
	require.True(iflagsH)

	cycle, err := machine.ReadCSR(ProcCsrMcycle)
	require.Nil(err)
	require.Greater(cycle, uint64(1))

	hash, err := machine.GetRootHash()
	require.Nil(err)

	return hash
}

const rxBufferStartAddress = 0x60000000
const rxBufferLength = 1 << 21

func defaultConfig() *MachineConfig {
	config := NewDefaultMachineConfig()
	config.Processor.Mimpid = math.MaxUint64
	config.Processor.Marchid = math.MaxUint64
	config.Processor.Mvendorid = math.MaxUint64
	config.Ram.ImageFilename = imagesPath + "linux.bin"
	config.Ram.Length = 64 << 20
	config.FlashDrive = []MemoryRangeConfig{{
		Start:         0x80000000000000,
		Length:        0xffffffffffffffff,
		Shared:        false,
		ImageFilename: imagesPath + "rootfs.ext2",
	}}
	config.Dtb.Bootargs = "quiet " +
		"earlycon=sbi " +
		"console=hvc0 " +
		"rootfstype=ext2 " +
		"root=/dev/pmem0 " +
		"rw " +
		"init=/usr/sbin/cartesi-init"
	config.Dtb.Init = `
        echo "Initializing the cartesi-machine!"
        busybox mkdir -p /run/drive-label && echo "root" > /run/drive-label/pmem0\
        USER=dapp
    `
	return config
}

func launchRemoteServer(t *testing.T) (*exec.Cmd, string) {
	cmd := exec.Command("jsonrpc-remote-cartesi-machine", "--server-address="+address)
	err := cmd.Start()
	require.Nil(t, err)
	time.Sleep(2 * time.Second)
	return cmd, address
}
