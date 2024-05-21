// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package rollupsmachine

import (
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
	"os"
	"testing"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/test/snapshot"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func init() {
	log.SetFlags(log.Ltime)
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

const (
	cycles          = uint64(1_000_000_000)
	serverVerbosity = ServerVerbosityInfo
)

func payload(s string) string {
	return fmt.Sprintf("echo '{ \"payload\": \"0x%s\" }'", hex.EncodeToString([]byte(s)))
}

// ------------------------------------------------------------------------------------------------

// TestRollupsMachine runs all the tests for the rollupsmachine package.
func TestRollupsMachine(t *testing.T) {
	suite.Run(t, new(RollupsMachineSuite))
}

type RollupsMachineSuite struct{ suite.Suite }

func (s *RollupsMachineSuite) TestLoad()    { suite.Run(s.T(), new(LoadSuite)) }
func (s *RollupsMachineSuite) TestFork()    { suite.Run(s.T(), new(ForkSuite)) }
func (s *RollupsMachineSuite) TestAdvance() { suite.Run(s.T(), new(AdvanceSuite)) }
func (s *RollupsMachineSuite) TestInspect() { suite.Run(s.T(), new(InspectSuite)) }
func (s *RollupsMachineSuite) TestCycles()  { suite.Run(s.T(), new(CyclesSuite)) }

// ------------------------------------------------------------------------------------------------

// Missing:
// - "could not create the remote machine manager"
// - "could not read iflagsY"
// - "could not read the yield reason"
// - machine.Close()
type LoadSuite struct {
	suite.Suite
	address string

	acceptSnapshot    *snapshot.Snapshot
	rejectSnapshot    *snapshot.Snapshot
	exceptionSnapshot *snapshot.Snapshot
	noticeSnapshot    *snapshot.Snapshot
}

func (s *LoadSuite) SetupSuite() {
	var (
		require = s.Require()
		script  string
		err     error
	)

	script = "rollup accept"
	s.acceptSnapshot, err = snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedManually, s.acceptSnapshot.BreakReason)

	script = "rollup reject"
	s.rejectSnapshot, err = snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedManually, s.rejectSnapshot.BreakReason)

	script = payload("Paul Atreides") + " | rollup exception"
	s.exceptionSnapshot, err = snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedManually, s.exceptionSnapshot.BreakReason)

	script = payload("Hari Seldon") + " | rollup notice"
	s.noticeSnapshot, err = snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedAutomatically, s.noticeSnapshot.BreakReason)
}

func (s *LoadSuite) TearDownSuite() {
	s.acceptSnapshot.Close()
	s.rejectSnapshot.Close()
	s.exceptionSnapshot.Close()
	s.noticeSnapshot.Close()
}

func (s *LoadSuite) SetupTest() {
	address, err := StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	s.Require().Nil(err)
	s.address = address
}

func (s *LoadSuite) TearDownTest() {
	err := StopServer(s.address)
	s.Require().Nil(err)
}

func (s *LoadSuite) TestOkAccept() {
	require := s.Require()
	config := &emulator.MachineRuntimeConfig{}
	machine, err := Load(s.acceptSnapshot.Path(), s.address, config)
	require.Nil(err)
	require.NotNil(machine)
}

func (s *LoadSuite) TestOkReject() {
	require := s.Require()
	config := &emulator.MachineRuntimeConfig{}
	machine, err := Load(s.rejectSnapshot.Path(), s.address, config)
	require.Nil(err)
	require.NotNil(machine)
}

func (s *LoadSuite) TestInvalidAddress() {
	require := s.Require()
	config := &emulator.MachineRuntimeConfig{}
	machine, err := Load(s.acceptSnapshot.Path(), "invalid-address", config)
	require.ErrorContains(err, "could not load the machine")
	require.ErrorIs(err, ErrCartesiMachine)
	require.Nil(machine)
}

func (s *LoadSuite) TestInvalidPath() {
	require := s.Require()
	config := &emulator.MachineRuntimeConfig{}
	machine, err := Load("invalid-path", s.address, config)
	require.ErrorContains(err, "could not load the machine")
	require.ErrorIs(err, ErrCartesiMachine)
	require.Nil(machine)
}

func (s *LoadSuite) TestNotAtManualYield() {
	require := s.Require()
	config := &emulator.MachineRuntimeConfig{}
	machine, err := Load(s.noticeSnapshot.Path(), s.address, config)
	require.NotNil(err)
	require.ErrorIs(err, ErrNotAtManualYield)
	require.Nil(machine)
}

func (s *LoadSuite) TestException() {
	require := s.Require()
	config := &emulator.MachineRuntimeConfig{}
	machine, err := Load(s.exceptionSnapshot.Path(), s.address, config)
	require.NotNil(err)
	require.ErrorIs(err, ErrException)
	require.Nil(machine)
}

// ------------------------------------------------------------------------------------------------

type ForkSuite struct{ suite.Suite }

func (s *ForkSuite) TestOk() {
	require := s.Require()

	// Creates the snapshot.
	script := "while true; do rollup accept; done"
	snapshot, err := snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedManually, snapshot.BreakReason)
	defer func() { require.Nil(snapshot.Close()) }()

	// Starts the server.
	address, err := StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	require.Nil(err)

	// Loads the machine.
	machine, err := Load(snapshot.Path(), address, &emulator.MachineRuntimeConfig{})
	require.Nil(err)
	require.NotNil(machine)
	defer func() { require.Nil(machine.Close()) }()

	// Forks the machine.
	forkMachine, forkAddress, err := machine.Fork()
	require.Nil(err)
	require.NotNil(forkMachine)
	require.NotEqual(address, forkAddress)
	require.Nil(forkMachine.Close())
}

// ------------------------------------------------------------------------------------------------

type AdvanceSuite struct {
	suite.Suite
	snapshotEcho   *snapshot.Snapshot
	snapshotReject *snapshot.Snapshot
	address        string
}

func (s *AdvanceSuite) SetupSuite() {
	var (
		require = s.Require()
		script  string
		err     error
	)

	script = "ioctl-echo-loop --vouchers=1 --notices=3 --reports=5 --verbose=1"
	s.snapshotEcho, err = snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedManually, s.snapshotEcho.BreakReason)

	script = "while true; do rollup reject; done"
	s.snapshotReject, err = snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedManually, s.snapshotReject.BreakReason)
}

func (s *AdvanceSuite) TearDownSuite() {
	s.snapshotEcho.Close()
	s.snapshotReject.Close()
}

func (s *AdvanceSuite) SetupTest() {
	address, err := StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	s.Require().Nil(err)
	s.address = address
}

func (s *AdvanceSuite) TestEchoLoop() {
	require := s.Require()

	// Loads the machine.
	machine, err := Load(s.snapshotEcho.Path(), s.address, &emulator.MachineRuntimeConfig{})
	require.Nil(err)
	require.NotNil(machine)
	defer func() { require.Nil(machine.Close()) }()

	// Encodes the input.
	input := Input{Data: []byte("Ender Wiggin")}
	encodedInput, err := input.Encode()
	require.Nil(err)

	// Sends the advance-state request.
	accepted, outputs, reports, outputsHash, err := machine.Advance(encodedInput)
	require.Nil(err)
	require.True(accepted)
	require.Len(outputs, 4)
	require.Len(reports, 5)
	require.NotEmpty(outputsHash)

	// Checks the responses.
	require.Equal(input.Data, expectVoucher(s.T(), outputs[0]).Data)
	for i := 1; i < 4; i++ {
		require.Equal(input.Data, expectNotice(s.T(), outputs[i]).Data)
	}
	for _, report := range reports {
		require.Equal(input.Data, report)
	}
}

func (s *AdvanceSuite) TestAcceptRejectException() {
	require := s.Require()

	// Creates the snapshot.
	script := `rollup accept
               rollup accept
               rollup reject
               echo '{"payload": "0x53616e64776f726d" }' | rollup exception`
	snapshot, err := snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedManually, snapshot.BreakReason)
	defer func() { require.Nil(snapshot.Close()) }()

	// Loads the machine.
	machine, err := Load(snapshot.Path(), s.address, &emulator.MachineRuntimeConfig{})
	require.Nil(err)
	require.NotNil(machine)
	defer func() { require.Nil(machine.Close()) }()

	// Encodes the input.
	input := Input{Data: []byte("Shai-Hulud")}
	encodedInput, err := input.Encode()
	require.Nil(err)

	{ // Accept.
		accepted, outputs, reports, outputsHash, err := machine.Advance(encodedInput)
		require.Nil(err)
		require.True(accepted)
		require.Empty(outputs)
		require.Empty(reports)
		require.NotEmpty(outputsHash)
	}

	{ // Reject.
		accepted, outputs, reports, outputsHash, err := machine.Advance(encodedInput)
		require.Nil(err)
		require.False(accepted)
		require.Nil(outputs)
		require.Empty(reports)
		require.Empty(outputsHash)
	}

	{ // Exception
		_, _, _, _, err := machine.Advance(encodedInput)
		require.Equal(ErrException, err)
	}
}

func (s *AdvanceSuite) TestHalted() {
	require := s.Require()

	// Creates the snapshot.
	script := `rollup accept; echo "Done"`
	snapshot, err := snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedManually, snapshot.BreakReason)
	defer func() { require.Nil(snapshot.Close()) }()

	// Loads the machine.
	machine, err := Load(snapshot.Path(), s.address, &emulator.MachineRuntimeConfig{})
	require.Nil(err)
	require.NotNil(machine)
	defer func() { require.Nil(machine.Close()) }()

	// Encodes the input.
	input := Input{Data: []byte("Fremen")}
	encodedInput, err := input.Encode()
	require.Nil(err)

	_, _, _, _, err = machine.Advance(encodedInput)
	require.Equal(ErrHalted, err)
}

// ------------------------------------------------------------------------------------------------

type CyclesSuite struct {
	suite.Suite
	snapshot *snapshot.Snapshot
	address  string
	machine  *RollupsMachine
	input    []byte
}

func (s *CyclesSuite) SetupSuite() {
	require := s.Require()
	script := "ioctl-echo-loop --vouchers=1 --notices=1 --reports=1 --verbose=1"
	snapshot, err := snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedManually, snapshot.BreakReason)
	s.snapshot = snapshot

	quote := `"I must not fear. Fear is the mind-killer." -- Dune, Frank Herbert`
	input, err := Input{Data: []byte(quote)}.Encode()
	require.Nil(err)
	s.input = input
}

func (s *CyclesSuite) TearDownSuite() {
	s.snapshot.Close()
}

func (s *CyclesSuite) SetupSubTest() {
	require := s.Require()
	var err error

	s.address, err = StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	require.Nil(err)

	s.machine, err = Load(s.snapshot.Path(), s.address, &emulator.MachineRuntimeConfig{})
	require.Nil(err)
	require.NotNil(s.machine)
}

func (s *CyclesSuite) TearDownSubTest() {
	err := s.machine.Close()
	s.Require().Nil(err)
}

// When we send a request to the machine with machine.Max set too low,
// the function call should return the ErrCycleLimitExceeded error.
func (s *CyclesSuite) TestCycleLimitExceeded() {
	// Exits before calling machine.Run.
	s.Run("Max=0", func() {
		require := s.Require()
		s.machine.Max = 0
		_, _, _, _, err := s.machine.Advance(s.input)
		require.Equal(ErrCycleLimitExceeded, err)
	})

	// Runs for exactly one cycle.
	s.Run("Max=1", func() {
		require := s.Require()
		s.machine.Max = 1
		_, _, _, _, err := s.machine.Advance(s.input)
		require.Equal(ErrCycleLimitExceeded, err)
	})

	// Calls machine.Run many times.
	s.Run("Max=100000", func() {
		require := s.Require()
		s.machine.Max = 100000
		s.machine.Inc = 10000
		_, _, _, _, err := s.machine.Advance(s.input)
		require.Equal(ErrCycleLimitExceeded, err)
	})
}

// ------------------------------------------------------------------------------------------------

type InspectSuite struct {
	suite.Suite
	snapshotEcho *snapshot.Snapshot
	address      string
}

func (s *InspectSuite) SetupSuite() {
	var (
		require = s.Require()
		script  string
		err     error
	)

	script = "ioctl-echo-loop --vouchers=3 --notices=5 --reports=7 --verbose=1"
	s.snapshotEcho, err = snapshot.FromScript(script, cycles)
	require.Nil(err)
	require.Equal(emulator.BreakReasonYieldedManually, s.snapshotEcho.BreakReason)
}

func (s *InspectSuite) TearDownSuite() {
	s.snapshotEcho.Close()
}

func (s *InspectSuite) SetupTest() {
	address, err := StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	s.Require().Nil(err)
	s.address = address
}

func (s *InspectSuite) TestEchoLoop() {
	require := s.Require()

	// Loads the machine.
	machine, err := Load(s.snapshotEcho.Path(), s.address, &emulator.MachineRuntimeConfig{})
	require.Nil(err)
	require.NotNil(machine)
	defer func() { require.Nil(machine.Close()) }()

	query := []byte("Bene Gesserit")

	// Sends the inspect-state request.
	accepted, reports, err := machine.Inspect(query)
	require.Nil(err)
	require.True(accepted)
	require.Len(reports, 7)

	// Checks the responses.
	for _, report := range reports {
		require.Equal(query, report)
	}
}

// ------------------------------------------------------------------------------------------------

// expectVoucher decodes the output and asserts that it is a voucher.
func expectVoucher(t *testing.T, output Output) *Voucher {
	voucher, notice, err := DecodeOutput(output)
	require.Nil(t, err)
	require.NotNil(t, voucher)
	require.Nil(t, notice)
	return voucher
}

// expectNotice decodes the output and asserts that it is a notice.
func expectNotice(t *testing.T, output Output) *Notice {
	voucher, notice, err := DecodeOutput(output)
	require.Nil(t, err)
	require.Nil(t, voucher)
	require.NotNil(t, notice)
	return notice
}
