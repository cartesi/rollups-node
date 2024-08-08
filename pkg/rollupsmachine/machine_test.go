// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package rollupsmachine

import (
	"errors"
	"log"
	"log/slog"
	"os"
	"testing"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
	"github.com/cartesi/rollups-node/test/snapshot"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	defaultInc = Cycle(10000000)
	defaultMax = Cycle(1000000000)
)

func init() {
	log.SetFlags(log.Ltime)
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

const (
	cycles          = uint64(1_000_000_000)
	serverVerbosity = cartesimachine.ServerVerbosityInfo
)

// ------------------------------------------------------------------------------------------------

// TestRollupsMachine runs all the tests for the rollupsmachine package.
func TestRollupsMachine(t *testing.T) {
	suite.Run(t, new(RollupsMachineSuite))
}

type RollupsMachineSuite struct{ suite.Suite }

func (s *RollupsMachineSuite) TestNew()     { suite.Run(s.T(), new(NewSuite)) }
func (s *RollupsMachineSuite) TestFork()    { suite.Run(s.T(), new(ForkSuite)) }
func (s *RollupsMachineSuite) TestAdvance() { suite.Run(s.T(), new(AdvanceSuite)) }
func (s *RollupsMachineSuite) TestInspect() { suite.Run(s.T(), new(InspectSuite)) }

// ------------------------------------------------------------------------------------------------

type NewSuite struct {
	suite.Suite
	address string

	acceptSnapshot *snapshot.Snapshot
	rejectSnapshot *snapshot.Snapshot
}

func (s *NewSuite) SetupSuite() {
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
}

func (s *NewSuite) TearDownSuite() {
	s.acceptSnapshot.Close()
	s.rejectSnapshot.Close()
}

func (s *NewSuite) SetupTest() {
	address, err := cartesimachine.StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	s.Require().Nil(err)
	s.address = address
}

func (s *NewSuite) TearDownTest() {
	err := cartesimachine.StopServer(s.address)
	s.Require().Nil(err)
}

func (s *NewSuite) TestOkAccept() {
	require := s.Require()

	config := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cartesimachine.Load(s.acceptSnapshot.Path(), s.address, config)
	require.NotNil(cartesiMachine)
	require.Nil(err)

	rollupsMachine, err := New(cartesiMachine, defaultInc, defaultMax)
	require.NotNil(rollupsMachine)
	require.Nil(err)
}

func (s *NewSuite) TestOkReject() {
	require := s.Require()

	config := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cartesimachine.Load(s.rejectSnapshot.Path(), s.address, config)
	require.NotNil(cartesiMachine)
	require.Nil(err)

	rollupsMachine, err := New(cartesiMachine, defaultInc, defaultMax)
	require.NotNil(rollupsMachine)
	require.Nil(err)
}

func (s *NewSuite) TestInvalidAddress() {
	require := s.Require()

	config := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cartesimachine.Load(s.acceptSnapshot.Path(), "invalid address", config)
	require.Nil(cartesiMachine)
	require.NotNil(err)

	require.ErrorContains(err, "could not load the machine")
	require.ErrorIs(err, cartesimachine.ErrCartesiMachine)
}

func (s *NewSuite) TestInvalidPath() {
	require := s.Require()

	config := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cartesimachine.Load("invalid path", s.address, config)
	require.Nil(cartesiMachine)
	require.NotNil(err)

	require.ErrorIs(err, cartesimachine.ErrCartesiMachine)
	require.ErrorContains(err, "could not load the machine")
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
	address, err := cartesimachine.StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	require.Nil(err)

	// Loads the machine.
	cartesiMachine, err := cartesimachine.Load(
		snapshot.Path(),
		address,
		&emulator.MachineRuntimeConfig{})
	require.NotNil(cartesiMachine)
	require.Nil(err)

	machine, err := New(cartesiMachine, defaultInc, defaultMax)
	require.NotNil(machine)
	require.Nil(err)
	defer func() { require.Nil(machine.Close()) }()

	// Forks the machine.
	forkMachine, err := machine.Fork()
	require.Nil(err)
	require.NotNil(forkMachine)
	require.NotEqual(address, forkMachine.inner.Address())
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
	address, err := cartesimachine.StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	s.Require().Nil(err)
	s.address = address
}

func (s *AdvanceSuite) TestEchoLoop() {
	require := s.Require()

	// Loads the machine.
	config := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cartesimachine.Load(s.snapshotEcho.Path(), s.address, config)
	require.NotNil(cartesiMachine)
	require.Nil(err)

	machine, err := New(cartesiMachine, defaultInc, defaultMax)
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
	config := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cartesimachine.Load(snapshot.Path(), s.address, config)
	require.NotNil(cartesiMachine)
	require.Nil(err)

	machine, err := New(cartesiMachine, defaultInc, defaultMax)
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
	config := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cartesimachine.Load(snapshot.Path(), s.address, config)
	require.NotNil(cartesiMachine)
	require.Nil(err)

	machine, err := New(cartesiMachine, defaultInc, defaultMax)
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
	address, err := cartesimachine.StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	s.Require().Nil(err)
	s.address = address
}

func (s *InspectSuite) TestEchoLoop() {
	require := s.Require()

	// Loads the machine.
	config := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cartesimachine.Load(s.snapshotEcho.Path(), s.address, config)
	require.NotNil(cartesiMachine)
	require.Nil(err)

	machine, err := New(cartesiMachine, defaultInc, defaultMax)
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

// ------------------------------------------------------------------------------------------------
// Unit tests
// ------------------------------------------------------------------------------------------------

func TestRollupsMachineUnit(t *testing.T) {
	suite.Run(t, new(UnitSuite))
}

type UnitSuite struct{ suite.Suite }

func (_ *UnitSuite) newMachines() (*CartesiMachineMock, *RollupsMachine) {
	mock := new(CartesiMachineMock)
	machine := &RollupsMachine{inner: mock, inc: defaultInc, max: defaultMax}
	return mock, machine
}

func (s *UnitSuite) TestNew() {
	newCartesiMachine := func() *CartesiMachineMock {
		mock := new(CartesiMachineMock)
		mock.IsAtManualYieldReturn = true
		mock.IsAtManualYieldError = nil
		mock.ReadYieldReasonReturn = []emulator.HtifYieldReason{emulator.ManualYieldReasonAccepted}
		mock.ReadYieldReasonError = []error{nil}
		return mock
	}

	s.Run("Ok", func() {
		s.Run("Accepted", func() {
			require := s.Require()
			mock := newCartesiMachine()

			machine, err := New(mock, defaultInc, defaultMax)
			require.Nil(err)
			require.NotNil(machine)
		})

		s.Run("Rejected", func() {
			require := s.Require()
			mock := newCartesiMachine()
			mock.ReadYieldReasonReturn = []emulator.HtifYieldReason{
				emulator.ManualYieldReasonRejected,
			}

			machine, err := New(mock, defaultInc, defaultMax)
			require.Nil(err)
			require.NotNil(machine)
		})
	})

	s.Run("CartesiMachineState", func() {
		s.Run("NotAtManualYield", func() {
			require := s.Require()
			mock := newCartesiMachine()
			mock.IsAtManualYieldReturn = false

			machine, err := New(mock, defaultInc, defaultMax)
			require.Equal(ErrNotAtManualYield, err)
			require.Nil(machine)
		})

		s.Run("Exception", func() {
			require := s.Require()
			mock := newCartesiMachine()
			mock.ReadYieldReasonReturn = []emulator.HtifYieldReason{
				emulator.ManualYieldReasonException,
			}

			machine, err := New(mock, defaultInc, defaultMax)
			require.Equal(ErrException, err)
			require.Nil(machine)
		})

		s.Run("Panic", func() {
			require := s.Require()
			require.PanicsWithValue(ErrUnreachable, func() {
				mock := newCartesiMachine()
				mock.ReadYieldReasonReturn = []emulator.HtifYieldReason{10}
				_, _ = New(mock, defaultInc, defaultMax)
			})
		})
	})

	s.Run("CartesiMachineError", func() {
		s.Run("IsAtManualYield", func() {
			require := s.Require()
			errIsAtManualYield := errors.New("IsAtManualYield error")
			mock := newCartesiMachine()
			mock.IsAtManualYieldError = errIsAtManualYield

			machine, err := New(mock, defaultInc, defaultMax)
			require.Equal(errIsAtManualYield, err)
			require.Nil(machine)
		})

		s.Run("ReadYieldReason", func() {
			require := s.Require()
			errReadYieldReason := errors.New("ReadYieldReason error")
			mock := newCartesiMachine()
			mock.ReadYieldReasonError = []error{errReadYieldReason}

			machine, err := New(mock, defaultInc, defaultMax)
			require.Equal(errReadYieldReason, err)
			require.Nil(machine)
		})
	})
}

func (s *UnitSuite) TestFork() {
	s.Run("Ok", func() {
		require := s.Require()
		forkedMock := new(CartesiMachineMock)
		mock, machine := s.newMachines()
		mock.ForkReturn = forkedMock
		mock.ForkError = nil

		fork, err := machine.Fork()
		require.Nil(err)
		require.NotNil(fork)
		require.Equal(forkedMock, fork.inner)
		require.Equal(machine.inc, fork.inc)
		require.Equal(machine.max, fork.max)
	})

	s.Run("CartesiMachineError", func() {
		require := s.Require()
		errFork := errors.New("Fork error")
		mock, machine := s.newMachines()
		mock.ForkReturn = new(CartesiMachineMock)
		mock.ForkError = errFork

		fork, err := machine.Fork()
		require.Equal(errFork, err)
		require.Nil(fork)
	})
}

func (s *UnitSuite) TestHash() {
	machineHash := [32]byte{}
	machineHash[0] = 1
	machineHash[31] = 1

	s.Run("Ok", func() {
		require := s.Require()
		mock, machine := s.newMachines()
		mock.ReadHashReturn = machineHash
		mock.ReadHashError = nil

		hash, err := machine.Hash()
		require.Nil(err)
		require.Equal(machineHash, hash)
		require.Equal(uint8(1), hash[0])
		require.Equal(uint8(1), hash[31])
		for i := 1; i < 31; i++ {
			require.Equal(uint8(0), hash[i])
		}
	})

	s.Run("CartesiMachineError", func() {
		require := s.Require()
		errReadHash := errors.New("ReadHash error")
		mock, machine := s.newMachines()
		mock.ReadHashReturn = machineHash
		mock.ReadHashError = errReadHash

		hash, err := machine.Hash()
		require.Equal(errReadHash, err)
		require.Equal(machineHash, hash)
	})
}

func (s *UnitSuite) TestAdvance() {
	s.T().Skip("TODO")
}

func (s *UnitSuite) TestInspect() {
	s.T().Skip("TODO")
}

func (s *UnitSuite) TestClose() {
	s.Run("Ok", func() {
		require := s.Require()
		mock, machine := s.newMachines()
		mock.CloseError = nil

		err := machine.Close()
		require.Nil(err)
		require.Nil(machine.inner)
	})

	s.Run("Reentry", func() {
		require := s.Require()
		mock, machine := s.newMachines()
		mock.CloseError = nil

		err := machine.Close()
		require.Nil(err)

		require.NotPanics(func() {
			err := machine.Close()
			require.Nil(err)
		})
	})

	s.Run("CartesiMachineError", func() {
		require := s.Require()
		errClose := errors.New("Close error")
		mock, machine := s.newMachines()
		mock.CloseError = errClose

		err := machine.Close()
		require.Equal(errClose, err)
	})
}

func (s *UnitSuite) TestLastRequestWasAccepted() {}

func (s *UnitSuite) TestProcess() {}

func (s *UnitSuite) TestRun() {
	newMachines := func() (*CartesiMachineMock, *RollupsMachine) {
		mock, machine := s.newMachines()
		mock.RunReturn = []emulator.BreakReason{0, emulator.BreakReasonYieldedManually}
		mock.RunError = []error{nil, nil}
		mock.ReadCycleError = []error{nil, nil}
		return mock, machine
	}
	var newMachinesONRN func() (*CartesiMachineMock, *RollupsMachine)

	s.Run("Step", func() {
		s.Run("Once", func() {
			require := s.Require()
			mock, machine := newMachines()

			mock.Cycle = 0
			machine.inc = 2
			machine.max = 10

			outputs, reports, err := machine.run()
			require.Nil(err)
			require.Empty(outputs)
			require.Empty(reports)
			require.Equal(uint(1), mock.Steps-1)
		})

		s.Run("Multiple", func() {
			require := s.Require()
			mock, machine := newMachines()
			mock.RunReturn = []emulator.BreakReason{0,
				emulator.BreakReasonReachedTargetMcycle,
				emulator.BreakReasonReachedTargetMcycle,
				emulator.BreakReasonYieldedManually,
			}
			mock.RunError = []error{nil, nil, nil, nil}
			mock.ReadCycleError = []error{nil, nil, nil, nil}

			mock.Cycle = 10
			machine.inc = 3
			machine.max = 8

			outputs, reports, err := machine.run()
			require.Nil(err)
			require.Empty(outputs)
			require.Empty(reports)
			require.Equal(uint(3), mock.Steps-1)
		})
	})

	s.Run("Responses", func() {
		s.Run("Outputs=1/Reports=0", func() {
			require := s.Require()
			mock, machine := newMachines()

			mock.RunReturn = []emulator.BreakReason{0,
				emulator.BreakReasonYieldedAutomatically,
				emulator.BreakReasonYieldedManually,
			}
			mock.RunError = []error{nil, nil, nil}
			mock.ReadCycleError = []error{nil, nil, nil}

			mock.ReadYieldReasonReturn = []emulator.HtifYieldReason{
				emulator.AutomaticYieldReasonOutput,
			}
			mock.ReadYieldReasonError = []error{nil}
			mock.ReadMemoryReturn = [][]byte{[]byte("an output")}
			mock.ReadMemoryError = []error{nil}

			mock.Cycle = 0
			machine.inc = 2
			machine.max = 10

			outputs, reports, err := machine.run()
			require.Nil(err)
			require.Len(outputs, 1)
			require.Empty(reports)

			require.Equal([]byte("an output"), outputs[0])

			require.Equal(uint(2), mock.Steps-1)
			require.Equal(uint(1), mock.Responses)
		})

		s.Run("Outputs=0/Reports=1", func() {
			require := s.Require()
			mock, machine := newMachines()

			mock.RunReturn = []emulator.BreakReason{0,
				emulator.BreakReasonYieldedAutomatically,
				emulator.BreakReasonYieldedManually,
			}
			mock.RunError = []error{nil, nil, nil}
			mock.ReadCycleError = []error{nil, nil, nil}

			mock.ReadYieldReasonReturn = []emulator.HtifYieldReason{
				emulator.AutomaticYieldReasonReport,
			}
			mock.ReadYieldReasonError = []error{nil}
			mock.ReadMemoryReturn = [][]byte{[]byte("a report")}
			mock.ReadMemoryError = []error{nil}

			mock.Cycle = 0
			machine.inc = 2
			machine.max = 10

			outputs, reports, err := machine.run()
			require.Nil(err)
			require.Empty(outputs)
			require.Len(reports, 1)

			require.Equal([]byte("a report"), reports[0])

			require.Equal(uint(2), mock.Steps-1)
			require.Equal(uint(1), mock.Responses)
		})

		newMachinesONRN = func() (*CartesiMachineMock, *RollupsMachine) {
			mock, machine := newMachines()

			mock.RunReturn = []emulator.BreakReason{0,
				emulator.BreakReasonYieldedAutomatically,
				emulator.BreakReasonYieldedAutomatically,
				emulator.BreakReasonYieldedAutomatically,
				emulator.BreakReasonYieldedAutomatically,
				emulator.BreakReasonYieldedAutomatically,
				emulator.BreakReasonYieldedManually,
			}
			mock.RunError = []error{nil, nil, nil, nil, nil, nil, nil}
			mock.ReadCycleError = []error{nil, nil, nil, nil, nil, nil, nil}

			mock.ReadYieldReasonReturn = []emulator.HtifYieldReason{
				emulator.AutomaticYieldReasonOutput,
				emulator.AutomaticYieldReasonReport,
				emulator.AutomaticYieldReasonOutput,
				emulator.AutomaticYieldReasonReport,
				emulator.AutomaticYieldReasonReport,
			}
			mock.ReadYieldReasonError = []error{nil, nil, nil, nil, nil}
			mock.ReadMemoryReturn = [][]byte{
				[]byte("output 1"),
				[]byte("report 1"),
				[]byte("output 2"),
				[]byte("report 2"),
				[]byte("report 3"),
			}
			mock.ReadMemoryError = []error{nil, nil, nil, nil, nil}

			mock.Cycle = 0
			machine.inc = 2
			machine.max = 20

			return mock, machine
		}

		s.Run("Outputs=N/Reports=N", func() {
			require := s.Require()
			mock, machine := newMachinesONRN()

			outputs, reports, err := machine.run()
			require.Nil(err)
			require.Len(outputs, 2)
			require.Len(reports, 3)

			require.Equal([]byte("output 1"), outputs[0])
			require.Equal([]byte("output 2"), outputs[1])
			require.Equal([]byte("report 1"), reports[0])
			require.Equal([]byte("report 2"), reports[1])
			require.Equal([]byte("report 3"), reports[2])

			require.Equal(uint(6), mock.Steps-1)
			require.Equal(uint(5), mock.Responses)
		})
	})

	s.Run("CycleLimitExceeded", func() {
		require := s.Require()
		mock, machine := newMachinesONRN()
		machine.max = 5

		outputs, reports, err := machine.run()
		require.Equal(ErrCycleLimitExceeded, err)
		require.Len(outputs, 2)
		require.Len(reports, 1)

		require.Equal([]byte("output 1"), outputs[0])
		require.Equal([]byte("output 2"), outputs[1])
		require.Equal([]byte("report 1"), reports[0])

		require.Equal(uint(3), mock.Steps-1)
		require.Equal(uint(3), mock.Responses)
	})
}

func (s *UnitSuite) TestStep() {
	newMachines := func() (*CartesiMachineMock, *RollupsMachine) {
		mock, machine := s.newMachines()
		machine.inc = 0
		machine.max = 0
		mock.RunReturn = []emulator.BreakReason{emulator.BreakReasonReachedTargetMcycle}
		mock.RunError = []error{nil}
		mock.ReadCycleError = []error{nil}
		return mock, machine
	}

	s.Run("Cycles", func() {
		s.Run("Current < Limit", func() {
			s.Run("Inc == 0", func() {
				require := s.Require()
				mock, machine := newMachines()

				currentCycle := uint64(5)
				limitCycle := uint64(6)
				machine.inc = 0
				mock.Cycle = currentCycle

				yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
				require.Nil(err)
				require.Nil(yt)
				require.Equal(currentCycle, newCurrentCycle)
			})

			s.Run("Inc < Leftover", func() {
				require := s.Require()
				mock, machine := newMachines()

				currentCycle := uint64(10)
				limitCycle := uint64(14)
				machine.inc = 2
				mock.Cycle = currentCycle

				yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
				require.Nil(err)
				require.Nil(yt)
				require.Equal(currentCycle+machine.inc, newCurrentCycle)
			})

			s.Run("Inc == Leftover", func() {
				require := s.Require()
				mock, machine := newMachines()

				currentCycle := uint64(0)
				limitCycle := uint64(3)
				machine.inc = 3
				mock.Cycle = currentCycle

				yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
				require.Nil(err)
				require.Nil(yt)
				require.Equal(limitCycle, newCurrentCycle)
				require.Equal(newCurrentCycle, currentCycle+machine.inc)
			})

			s.Run("Inc > Leftover", func() {
				require := s.Require()
				mock, machine := newMachines()

				currentCycle := uint64(1)
				limitCycle := uint64(4)
				machine.inc = 5
				mock.Cycle = currentCycle

				yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
				require.Nil(err)
				require.Nil(yt)
				require.Equal(limitCycle, newCurrentCycle)
				require.Less(newCurrentCycle, currentCycle+machine.inc)
			})
		})

		s.Run("Current == Limit", func() {
			s.Run("Inc != 0", func() {
				require := s.Require()
				mock, machine := newMachines()

				currentCycle := uint64(6)
				limitCycle := currentCycle
				machine.inc = 2
				mock.Cycle = currentCycle

				yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
				require.Equal(ErrCycleLimitExceeded, err)
				require.Nil(yt)
				require.Zero(newCurrentCycle)
			})

			s.Run("Inc == 0", func() {
				require := s.Require()
				mock, machine := newMachines()

				currentCycle := uint64(6)
				limitCycle := currentCycle
				machine.inc = 0
				mock.Cycle = currentCycle

				yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
				require.Nil(err)
				require.Nil(yt)
				require.Equal(currentCycle, newCurrentCycle)
			})
		})

		s.Run("Current > Limit", func() {
			s.Run("Inc != 0", func() {
				require := s.Require()
				mock, machine := newMachines()

				currentCycle := uint64(9)
				limitCycle := uint64(4)
				machine.inc = 1
				mock.Cycle = currentCycle

				yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
				require.Equal(ErrCycleLimitExceeded, err)
				require.Nil(yt)
				require.Zero(newCurrentCycle)
			})

			s.Run("Inc == 0", func() {
				require := s.Require()
				mock, machine := newMachines()

				currentCycle := uint64(9)
				limitCycle := uint64(4)
				machine.inc = 0
				mock.Cycle = currentCycle

				yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
				require.Nil(err)
				require.Nil(yt)
				require.Equal(currentCycle, newCurrentCycle)
			})
		})
	})

	s.Run("CartesiMachineError", func() {
		s.Run("Run", func() {
			require := s.Require()
			errRun := errors.New("Run error")
			mock, machine := newMachines()
			mock.RunError[0] = errRun

			currentCycle := uint64(5)
			limitCycle := uint64(12)
			machine.inc = 5
			mock.Cycle = currentCycle

			yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
			require.Equal(errRun, err)
			require.Nil(yt)
			require.Zero(newCurrentCycle)
		})

		s.Run("ReadCycle", func() {
			require := s.Require()
			errReadCycle := errors.New("ReadCycle error")
			mock, machine := newMachines()
			mock.ReadCycleError[0] = errReadCycle

			currentCycle := uint64(100)
			limitCycle := uint64(1000)
			machine.inc = 100
			mock.Cycle = currentCycle

			yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
			require.Equal(errReadCycle, err)
			require.Nil(yt)
			require.Zero(newCurrentCycle)
		})
	})

	s.Run("Panic", func() {
		s.Run("BreakReasonFailed", func() {
			require := s.Require()
			mock, machine := newMachines()
			mock.RunReturn[0] = emulator.BreakReasonFailed

			currentCycle := uint64(10)
			limitCycle := uint64(100)
			machine.inc = 10
			mock.Cycle = currentCycle

			require.PanicsWithValue(ErrUnreachable, func() {
				_, _, _ = machine.step(currentCycle, limitCycle)
			})
		})

		s.Run("BreakReasonInvalid", func() {
			require := s.Require()
			mock, machine := newMachines()
			mock.RunReturn[0] = 10 // invalid break reason

			currentCycle := uint64(5)
			limitCycle := uint64(50)
			machine.inc = 6
			mock.Cycle = currentCycle

			require.PanicsWithValue(ErrUnreachable, func() {
				_, _, _ = machine.step(currentCycle, limitCycle)
			})
		})
	})

	s.Run("ManualYield", func() {
		require := s.Require()
		mock, machine := newMachines()
		mock.RunReturn[0] = emulator.BreakReasonYieldedManually

		currentCycle := uint64(3)
		limitCycle := uint64(17)
		machine.inc = 9
		mock.Cycle = currentCycle

		yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
		require.Nil(err)
		require.NotNil(yt)
		require.Equal(manualYield, *yt)
		require.Equal(currentCycle+machine.inc, newCurrentCycle)
	})

	s.Run("AutomaticYield", func() {
		require := s.Require()
		mock, machine := newMachines()
		mock.RunReturn[0] = emulator.BreakReasonYieldedAutomatically

		currentCycle := uint64(8)
		limitCycle := uint64(17)
		machine.inc = 9
		mock.Cycle = currentCycle

		yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
		require.Nil(err)
		require.NotNil(yt)
		require.Equal(automaticYield, *yt)
		require.Equal(limitCycle, newCurrentCycle)
	})

	s.Run("Halted", func() {
		require := s.Require()
		mock, machine := newMachines()
		mock.RunReturn[0] = emulator.BreakReasonHalted

		currentCycle := uint64(4)
		limitCycle := uint64(6)
		machine.inc = 1
		mock.Cycle = currentCycle

		yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
		require.Equal(ErrHalted, err)
		require.Nil(yt)
		require.Zero(newCurrentCycle)
	})

	s.Run("SoftYield", func() {
		require := s.Require()
		mock, machine := newMachines()
		mock.RunReturn[0] = emulator.BreakReasonYieldedSoftly

		currentCycle := uint64(3)
		limitCycle := uint64(8)
		machine.inc = 4
		mock.Cycle = currentCycle

		yt, newCurrentCycle, err := machine.step(currentCycle, limitCycle)
		require.Equal(ErrSoftYield, err)
		require.Nil(yt)
		require.Zero(newCurrentCycle)
	})
}

// ------------------------------------------------------------------------------------------------
// Mock
// ------------------------------------------------------------------------------------------------

type CartesiMachineMock struct {
	ForkReturn cartesimachine.CartesiMachine
	ForkError  error

	ContinueError error

	CloseError error

	IsAtManualYieldReturn bool
	IsAtManualYieldError  error

	ReadHashReturn [32]byte
	ReadHashError  error

	WriteRequestError error

	AddressReturn string

	Responses             uint
	ReadYieldReasonReturn []emulator.HtifYieldReason
	ReadYieldReasonError  []error
	ReadMemoryReturn      [][]byte
	ReadMemoryError       []error

	Steps          uint
	Cycle          uint64
	RunReturn      []emulator.BreakReason
	RunError       []error
	ReadCycleError []error
}

func (machine *CartesiMachineMock) Fork() (cartesimachine.CartesiMachine, error) {
	return machine.ForkReturn, machine.ForkError
}

func (machine *CartesiMachineMock) Continue() error {
	return machine.ContinueError
}

func (machine *CartesiMachineMock) Close() error {
	return machine.CloseError
}

func (machine *CartesiMachineMock) IsAtManualYield() (bool, error) {
	return machine.IsAtManualYieldReturn, machine.IsAtManualYieldError
}

func (machine *CartesiMachineMock) ReadHash() ([32]byte, error) {
	return machine.ReadHashReturn, machine.ReadHashError
}

func (machine *CartesiMachineMock) WriteRequest(data []byte, _ cartesimachine.RequestType) error {
	return machine.WriteRequestError
}

func (machine *CartesiMachineMock) PayloadLengthLimit() uint {
	return 100000
}

func (machine *CartesiMachineMock) Address() string {
	return machine.AddressReturn
}

// ------------------------------------------------------------------------------------------------

func (machine *CartesiMachineMock) ReadYieldReason() (emulator.HtifYieldReason, error) {
	yieldReason := machine.ReadYieldReasonReturn[machine.Responses]
	err := machine.ReadYieldReasonError[machine.Responses]
	return yieldReason, err
}

func (machine *CartesiMachineMock) ReadMemory() ([]byte, error) {
	bytes := machine.ReadMemoryReturn[machine.Responses]
	err := machine.ReadMemoryError[machine.Responses]
	machine.Responses++
	return bytes, err
}

// ------------------------------------------------------------------------------------------------

func (machine *CartesiMachineMock) Run(cycle uint64) (emulator.BreakReason, error) {
	machine.Cycle += cycle - machine.Cycle
	return machine.RunReturn[machine.Steps], machine.RunError[machine.Steps]
}

func (machine *CartesiMachineMock) ReadCycle() (uint64, error) {
	if err := machine.ReadCycleError[machine.Steps]; err != nil {
		return machine.Cycle, err
	}
	cycle, err := machine.Cycle, machine.ReadCycleError[machine.Steps]
	machine.Steps++
	return cycle, err
}
