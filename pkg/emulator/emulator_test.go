// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Test Cartesi Machine C API wrapper

package emulator

import (
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetDefaultConfig(t *testing.T) {
	cfg, err := GetDefaultConfig()
	assert.Nil(t, err)
	assert.NotNil(t, cfg)
}

func TestNewDefaultMachineConfig(t *testing.T) {
	cfg := NewDefaultMachineConfig()
	assert.NotNil(t, cfg)
}

func TestNewLocalMachine(t *testing.T) {
	cfg := makeMachineConfig()
	runtimeConfig := &MachineRuntimeConfig{}
	machine, err := NewMachine(cfg, runtimeConfig)
	defer machine.Free()
	assert.Nil(t, err)
	assert.NotNil(t, machine)
	sharedMachineTests(t, machine)
}

func TestReadWriteMemoryOnLocalMachine(t *testing.T) {
	cfg := makeMachineConfig()
	runtimeConfig := &MachineRuntimeConfig{}
	machine, err := NewMachine(cfg, runtimeConfig)
	assert.Nil(t, err)
	defer machine.Free()
	sharedTestReadWriteMemory(t, machine)
}

func TestReadWriteMemoryOnRemoteMachine(t *testing.T) {
	// launch remote server
	cmd := launchRemoteServer(t)
	defer cmd.Process.Kill()
	// connect to remote server
	mgr, err := NewRemoteMachineManager(makeRemoteAddress())
	assert.Nil(t, err)
	assert.NotNil(t, mgr)
	defer func() {
		mgr.Shutdown()
		defer mgr.Free()
	}()
	// create machine
	cfg := makeMachineConfig()
	runtimeConfig := &MachineRuntimeConfig{}
	machine, err := mgr.NewMachine(cfg, runtimeConfig)
	assert.Nil(t, err)
	assert.NotNil(t, machine)
	defer func() {
		machine.Destroy()
		machine.Free()
	}()
	sharedTestReadWriteMemory(t, machine)
}

func sharedTestReadWriteMemory(t *testing.T, machine *Machine) {
	// read existing data
	existingData, err := machine.ReadMemory(rxBufferStartAddress, 1024)
	assert.Nil(t, err)
	assert.NotNil(t, existingData)
	assert.Equal(t, 1024, len(existingData))
	// Prepare new data
	var newData [1024]byte
	for i := 0; i < 1024; i++ {
		newData[i] = byte(i % 256)
	}
	assert.NotEqual(t, newData[:], existingData)
	// write new data
	err = machine.WriteMemory(rxBufferStartAddress, newData[:])
	assert.Nil(t, err)
	// read back the data
	readBackData, err := machine.ReadMemory(rxBufferStartAddress, 1024)
	assert.Nil(t, err)
	assert.NotNil(t, readBackData)
	assert.Equal(t, newData[:], readBackData)

	// create a byte array full of 0xda of the size of rxBufferLength
	var newRxBufferData [rxBufferLength]byte
	for i := 0; i < rxBufferLength; i++ {
		newRxBufferData[i] = 0xda
	}
	// create a temporary file and write the new data to it
	tempFile, err := os.CreateTemp("", "rxBuffer")
	assert.Nil(t, err)
	defer os.Remove(tempFile.Name())
	_, err = tempFile.Write(newRxBufferData[:])
	assert.Nil(t, err)
	tempFile.Close()
	// initialize a MemoryRangeConfig pointing to this file
	newRxBuffer := &MemoryRangeConfig{
		Start:         rxBufferStartAddress,
		Length:        rxBufferLength,
		Shared:        false,
		ImageFilename: tempFile.Name(),
	}
	// replace the rxBuffer with the new data
	err = machine.ReplaceMemoryRange(newRxBuffer)
	assert.Nil(t, err)
	// read back memory and check if it was replaced
	readBackRxData, err := machine.ReadMemory(rxBufferStartAddress, rxBufferLength)
	assert.Nil(t, err)
	assert.NotNil(t, readBackRxData)
	assert.Equal(t, newRxBufferData[:], readBackRxData)

}

func TestRunLocalMachineHappyPath(t *testing.T) {
	cfg := makeMachineConfig()
	runtimeConfig := &MachineRuntimeConfig{}
	machine, err := NewMachine(cfg, runtimeConfig)
	defer machine.Free()
	assert.Nil(t, err)
	assert.NotNil(t, machine)
	var iflagsH bool
	var mcycle uint64
	var hashBefore *MerkleTreeHash
	var hashAfter *MerkleTreeHash
	var breakReason BreakReason
	// assert initial state
	iflagsH, err = machine.ReadIFlagsH()
	assert.Nil(t, err)
	assert.False(t, iflagsH)
	mcycle, err = machine.ReadCSR(ProcCsrMcycle)
	assert.Nil(t, err)
	assert.Equal(t, uint64(0), mcycle)
	hashBefore, err = machine.GetRootHash()
	assert.Nil(t, err)
	// Advance one cycle
	breakReason, err = machine.Run(1)
	assert.Nil(t, err)
	assert.Equal(t, BreakReasonReachedTargetMcycle, breakReason)
	iflagsH, err = machine.ReadIFlagsH()
	assert.Nil(t, err)
	assert.False(t, iflagsH) // still not halted
	mcycle, err = machine.ReadCSR(ProcCsrMcycle)
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), mcycle) // advanced one cycle
	hashAfter, err = machine.GetRootHash()
	assert.Nil(t, err)
	assert.NotEqual(t, hashBefore.String(), hashAfter.String()) // hash changed
	storedHash := hashAfter
	// Store machine
	tempDir, err := os.MkdirTemp("", "testmachines")
	assert.Nil(t, err)
	defer os.RemoveAll(tempDir)
	storePath := tempDir + "/machine"
	err = machine.Store(storePath)
	assert.Nil(t, err)
	// run until halt
	hashBefore = hashAfter
	breakReason, err = machine.Run(math.MaxUint64)
	assert.Nil(t, err)
	assert.Equal(t, BreakReasonHalted, breakReason)
	iflagsH, err = machine.ReadIFlagsH()
	assert.Nil(t, err)
	assert.True(t, iflagsH) //  halted
	mcycle, err = machine.ReadCSR(ProcCsrMcycle)
	assert.Nil(t, err)
	assert.Greater(t, mcycle, uint64(1))
	hashAfter, err = machine.GetRootHash()
	assert.Nil(t, err)
	assert.NotEqual(t, hashBefore.String(), hashAfter.String()) // hash changed	 again

	// load a second machine from empDir, err := ioutil.TempDir("", "example")
	machine2, err := LoadMachine(storePath, runtimeConfig)
	defer machine2.Free()
	assert.Nil(t, err)
	assert.NotNil(t, machine2)
	var hashMachine2 *MerkleTreeHash
	hashMachine2, err = machine2.GetRootHash()
	assert.Nil(t, err)
	assert.Equal(t, storedHash.String(), hashMachine2.String()) // hash is the same
}

func TestNewRemoteMachine(t *testing.T) {
	// launch remote server
	cmd := launchRemoteServer(t)
	defer cmd.Process.Kill()
	// connect to remote server
	mgr, err := NewRemoteMachineManager(makeRemoteAddress())
	assert.Nil(t, err)
	assert.NotNil(t, mgr)
	defer func() {
		mgr.Shutdown()
		defer mgr.Free()
	}()
	// create machine
	cfg := makeMachineConfig()
	runtimeConfig := &MachineRuntimeConfig{}
	machine, err := mgr.NewMachine(cfg, runtimeConfig)
	assert.Nil(t, err)
	assert.NotNil(t, machine)
	defer func() {
		machine.Destroy()
		machine.Free()
	}()
	sharedMachineTests(t, machine)
}

func TestRemoteMachineHappyPath(t *testing.T) {
	var err error
	remoteAddress := makeRemoteAddress()
	// launch remote server
	cmd := launchRemoteServer(t)
	defer cmd.Process.Kill()
	// Connect to remote server
	var mgr *RemoteMachineManager
	mgr, err = NewRemoteMachineManager(makeRemoteAddress())
	assert.Nil(t, err)
	assert.NotNil(t, mgr)
	defer func() {
		mgr.Shutdown()
		defer mgr.Free()
	}()
	// get default config from remote server
	var remoteDefCfg *MachineConfig
	remoteDefCfg, err = mgr.GetDefaultConfig()
	assert.Nil(t, err)
	assert.NotNil(t, remoteDefCfg)

	// create a remote machine
	var machine *Machine
	cfg := makeMachineConfig()
	runtimeConfig := &MachineRuntimeConfig{}
	machine, err = mgr.NewMachine(cfg, runtimeConfig)
	assert.Nil(t, err)
	assert.NotNil(t, machine)
	defer func() {
		machine.Destroy() // destroy machine from server
		machine.Free()
	}()
	// assert initial machine state
	var iflagsH bool
	var mcycle uint64
	var hashBefore *MerkleTreeHash
	var finalHash *MerkleTreeHash
	var breakReason BreakReason
	iflagsH, err = machine.ReadIFlagsH()
	assert.Nil(t, err)
	assert.False(t, iflagsH)
	mcycle, err = machine.ReadCSR(ProcCsrMcycle)
	assert.Nil(t, err)
	assert.Equal(t, uint64(0), mcycle)
	hashBefore, err = machine.GetRootHash()
	assert.Nil(t, err)
	// Advance one cycle
	breakReason, err = machine.Run(1)
	assert.Nil(t, err)
	assert.Equal(t, BreakReasonReachedTargetMcycle, breakReason)
	iflagsH, err = machine.ReadIFlagsH()
	assert.Nil(t, err)
	assert.False(t, iflagsH) // still not halted
	mcycle, err = machine.ReadCSR(ProcCsrMcycle)
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), mcycle) // advanced one cycle
	finalHash, err = machine.GetRootHash()
	assert.Nil(t, err)
	assert.NotEqual(t, hashBefore.String(), finalHash.String()) // hash changed
	storedHash := finalHash
	// Store machine
	tempDir, err := os.MkdirTemp("", "testmachines")
	assert.Nil(t, err)
	defer os.RemoveAll(tempDir)
	storePath := tempDir + "/machine"
	err = machine.Store(storePath)
	assert.Nil(t, err)
	// run until halt
	hashBefore = finalHash
	breakReason, err = machine.Run(math.MaxUint64)
	assert.Nil(t, err)
	assert.Equal(t, BreakReasonHalted, breakReason)
	iflagsH, err = machine.ReadIFlagsH()
	assert.Nil(t, err)
	assert.True(t, iflagsH) //  halted
	mcycle, err = machine.ReadCSR(ProcCsrMcycle)
	assert.Nil(t, err)
	assert.Greater(t, mcycle, uint64(1))
	finalHash, err = machine.GetRootHash()
	assert.Nil(t, err)
	assert.NotEqual(t, hashBefore.String(), finalHash.String()) // hash changed	 again
	// fork the remote server
	var secondRemoteAddress *string
	secondRemoteAddress, err = mgr.Fork()
	assert.Nil(t, err)
	assert.NotEqual(t, remoteAddress, secondRemoteAddress)
	assert.NotEqual(t, "", secondRemoteAddress)
	assert.NotEqual(t, remoteAddress, secondRemoteAddress)
	// connect to the forked server
	var mgr2 *RemoteMachineManager
	mgr2, err = NewRemoteMachineManager(*secondRemoteAddress)
	assert.Nil(t, err)
	assert.NotNil(t, mgr2)
	// Get forked machine
	var forkedMachine *Machine
	forkedMachine, err = mgr2.GetMachine()
	assert.Nil(t, err)
	assert.NotNil(t, forkedMachine)
	defer func() {
		forkedMachine.Destroy()
		forkedMachine.Free()
	}()
	var forkedHash *MerkleTreeHash
	forkedHash, err = forkedMachine.GetRootHash()
	assert.Nil(t, err)
	assert.Equal(t, finalHash.String(), forkedHash.String())
	// destroy existing machine on mgr2
	err = forkedMachine.Destroy()
	assert.Nil(t, err)
	// Load stored machine on mgr2
	var loadedMachine *Machine
	loadedMachine, err = mgr2.LoadMachine(storePath, runtimeConfig)
	assert.Nil(t, err)
	assert.NotNil(t, loadedMachine)
	defer func() {
		loadedMachine.Destroy()
		loadedMachine.Free()
	}()
	var loadedHash *MerkleTreeHash
	loadedHash, err = loadedMachine.GetRootHash()
	assert.Nil(t, err)
	assert.Equal(t, storedHash.String(), loadedHash.String())
}

func TestSnapshot(t *testing.T) {
	// launch remote server
	cmd := launchRemoteServer(t)
	defer cmd.Process.Kill()
	// Connect to remote server
	var mgr *RemoteMachineManager
	var err error
	mgr, err = NewRemoteMachineManager(makeRemoteAddress())
	assert.Nil(t, err)
	assert.NotNil(t, mgr)
	defer func() {
		mgr.Shutdown()
		defer mgr.Free()
	}()
	// create a machine
	var machine *Machine
	cfg := makeMachineConfig()
	runtimeConfig := &MachineRuntimeConfig{}
	machine, err = mgr.NewMachine(cfg, runtimeConfig)
	assert.Nil(t, err)
	assert.NotNil(t, machine)
	defer func() {
		machine.Destroy()
		machine.Free()
	}()
	// get current hash
	var hashBefore *MerkleTreeHash
	hashBefore, err = machine.GetRootHash()
	assert.Nil(t, err)
	// take a snapshot
	err = machine.Snapshot()
	assert.Nil(t, err)
	// advance one cycle
	var breakReason BreakReason
	breakReason, err = machine.Run(1)
	assert.Nil(t, err)
	assert.Equal(t, BreakReasonReachedTargetMcycle, breakReason)
	// get new hash
	var hashAfter *MerkleTreeHash
	hashAfter, err = machine.GetRootHash()
	assert.Nil(t, err)
	assert.NotEqual(t, hashBefore.String(), hashAfter.String())
	// rollback
	err = machine.Rollback()
	assert.Nil(t, err)
	// get hash after rollback
	var hashAfterRollback *MerkleTreeHash
	hashAfterRollback, err = machine.GetRootHash()
	assert.Nil(t, err)
	assert.Equal(t, hashBefore.String(), hashAfterRollback.String())

}

func sharedMachineTests(t *testing.T, m *Machine) {
	cfg, err := m.GetInitialConfig()
	assert.Nil(t, err)
	assert.NotNil(t, cfg)

	// Toggle flags that control dapp execution
	// read and toggle IFlagsY
	var iflagsY bool
	iflagsY, err = m.ReadIFlagsY()
	assert.Nil(t, err)
	assert.False(t, iflagsY)
	err = m.SetIFlagsY()
	assert.Nil(t, err)
	iflagsY, err = m.ReadIFlagsY()
	assert.Nil(t, err)
	assert.True(t, iflagsY)
	// read and toggle IFlagsH
	var iflagsH bool
	iflagsH, err = m.ReadIFlagsH()
	assert.Nil(t, err)
	assert.False(t, iflagsH)
	err = m.SetIFlagsH()
	assert.Nil(t, err)
	iflagsH, err = m.ReadIFlagsH()
	assert.Nil(t, err)
	assert.True(t, iflagsH)
}

const rxBufferStartAddress = 0x60000000
const rxBufferLength = 1 << 21

func makeMachineConfig() *MachineConfig {
	images_path := strings.TrimRight(os.Getenv("CARTESI_IMAGES_PATH"), "/") + "/"
	cfg := NewDefaultMachineConfig()
	cfg.Processor.Mimpid = math.MaxUint64
	cfg.Processor.Marchid = math.MaxUint64
	cfg.Processor.Mvendorid = math.MaxUint64
	cfg.Ram.ImageFilename = images_path + "linux.bin"
	cfg.Ram.Length = 64 << 20
	cfg.FlashDrive = []MemoryRangeConfig{
		{
			Start:         0x80000000000000,
			Length:        0xffffffffffffffff,
			Shared:        false,
			ImageFilename: images_path + "rootfs.ext2",
		},
	}
	cfg.Dtb.Bootargs = "quiet earlycon=sbi console=hvc0 rootfstype=ext2 root=/dev/pmem0 rw init=/usr/sbin/cartesi-init"
	cfg.Dtb.Init = `echo "Opa!"
			busybox mkdir -p /run/drive-label && echo "root" > /run/drive-label/pmem0\
			USER=dapp
		`

	cfg.Cmio.HsaValue = true
	cfg.Cmio.RxBuffer = MemoryRangeConfig{
		Start:  rxBufferStartAddress,
		Length: rxBufferLength,
	}
	cfg.Cmio.TxBuffer = MemoryRangeConfig{
		Start:  0x60200000,
		Length: 1 << 21,
	}

	return cfg
}

func makeRemoteAddress() string {
	remoteServerPort := os.Getenv("JSONRPC_REMOTE_CARTESI_MACHINE_PORT") // example: 3333
	return "localhost:" + remoteServerPort
}

func launchRemoteServer(t *testing.T) *exec.Cmd {
	remoteMachineServerPath := os.Getenv("JSONRPC_REMOTE_CARTESI_MACHINE_PATH")
	// launch remote server
	cmd := exec.Command(remoteMachineServerPath, "--server-address="+makeRemoteAddress())
	err := cmd.Start()
	assert.Nil(t, err)
	time.Sleep(2 * time.Second)
	return cmd
}
