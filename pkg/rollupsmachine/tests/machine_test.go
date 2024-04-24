// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package tests

import (
	"os"
	"testing"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/pkg/model"
	rm "github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/tests/echo/protocol"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ErrHalted.
// ErrFailed.
// ErrYieldedSoftly.

// TestRollupsMachine runs all the tests for the rollupsmachine package.
func TestRollupsMachine(t *testing.T) {
	initT(t)
	suite.Run(t, new(RollupsMachineSuite))
}

type RollupsMachineSuite struct{ suite.Suite }

func (s *RollupsMachineSuite) TestLoad()    { suite.Run(s.T(), new(LoadSuite)) }
func (s *RollupsMachineSuite) TestFork()    { suite.Run(s.T(), new(ForkSuite)) }
func (s *RollupsMachineSuite) TestAdvance() { suite.Run(s.T(), new(AdvanceSuite)) }
func (s *RollupsMachineSuite) TestInspect() { suite.Run(s.T(), new(InspectSuite)) }
func (s *RollupsMachineSuite) TestCycles()  { suite.Run(s.T(), new(CyclesSuite)) }

// ------------------------------------------------------------------------------------------------

type LoadSuite struct{ suite.Suite }

func (suite *LoadSuite) TestOk() {
	require := require.New(suite.T())

	address, err := rm.StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	require.Nil(err)

	machine, err := rm.Load("rollup-accept", address, &emulator.MachineRuntimeConfig{})
	require.Nil(err)
	require.NotNil(machine)

	err = rm.StopServer(address)
	require.Nil(err)
}

// There is no server running at the given address.
func (suite *LoadSuite) TestInvalidAddress() {
	require := require.New(suite.T())
	address := "invalid-address"
	machine, err := rm.Load("rollup-accept", address, &emulator.MachineRuntimeConfig{})
	require.NotNil(err)
	require.ErrorIs(err, rm.ErrRemoteLoadMachine)
	require.Nil(machine)
}

// There is not a machine stored at the given path.
func (suite *LoadSuite) TestInvalidStoredMachine() {
	require := require.New(suite.T())

	address, err := rm.StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	require.Nil(err)

	machine, err := rm.Load("invalid-path", address, &emulator.MachineRuntimeConfig{})
	require.NotNil(err)
	require.ErrorIs(err, rm.ErrRemoteLoadMachine)
	require.Nil(machine)
}

// The machine is not ready to receive requests, because the last yield was an automatic yield.
func (suite *LoadSuite) TestNotAtManualYield() {
	require := require.New(suite.T())

	address, err := rm.StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	require.Nil(err)

	machine, err := rm.Load("rollup-notice", address, &emulator.MachineRuntimeConfig{})
	require.NotNil(err)
	require.ErrorIs(err, rm.ErrNotReadyForRequests)
	require.ErrorIs(err, rm.ErrNotAtManualYield)
	require.Nil(machine)
}

// The machine is not ready to receive requests, because the last input was rejected.
func (suite *LoadSuite) TestLastInputWasRejected() {
	require := require.New(suite.T())

	address, err := rm.StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	require.Nil(err)

	machine, err := rm.Load("rollup-reject", address, &emulator.MachineRuntimeConfig{})
	require.NotNil(err)
	require.ErrorIs(err, rm.ErrNotReadyForRequests)
	require.ErrorIs(err, rm.ErrLastInputWasRejected)
	require.Nil(machine)
}

// TODO
// // The machine is not ready to receive requests, because the last input yielded an exception.
// func (suite *LoadSuite) TestLastInputYieldedAnException() {
// 	slog.Warn("Hey, I'm in TestRollupException!")
//
// 	address, err := rollupsmachine.StartServer(suite.serverVerbosity, 0, os.Stdout, os.Stderr)
// 	require.Nil(err)
//
// 	machine, err := rollupsmachine.Load(address, "rollup-exception", suite.runtimeConfig)
// 	require.NotNil(err)
// 	require.ErrorIs(err, rollupsmachine.ErrNotReadyForRequests)
// 	require.ErrorIs(err, rollupsmachine.ErrLastInputYieldedAnException)
// 	require.Nil(machine)
// }

// ------------------------------------------------------------------------------------------------

type ForkSuite struct{ EchoSuite }

// [ACTION 1] When we fork a machine
// [ACTION 2] and then destroy it,
// [EXPECTED] the function calls should not return with any errors.
func (suite *ForkSuite) TestOk() {
	require := require.New(suite.T())

	// Forks the machine.
	forkedMachine, err := suite.machine.Fork()
	require.Nil(err)
	require.NotNil(forkedMachine)

	// Destroys the forked machine.
	err = forkedMachine.Destroy()
	require.Nil(err)
}

// [ACTION 1] When we fork a machine
// [ACTION 2] and send an advance-state request to the forked machine,
// [EXPECTED] the father machine should remain unchanged.
func (suite *ForkSuite) TestWithAdvance() {
	t := suite.T()
	require := require.New(t)

	// Forks the machine.
	forkedMachine, err := suite.machine.Fork()
	require.Nil(err)
	require.NotNil(forkedMachine)
	defer func() {
		err = forkedMachine.Destroy()
		require.Nil(err)
	}()

	quote := `"He loved Big Brother." -- 1984, George Orwell`

	{ // Sends an input to the forked machine.
		inputData := protocol.InputData{Quote: quote, Notices: 1}
		outputs, _ := echoAdvance(t, suite.machine, inputData)

		notice := expectNotice(t, outputs[0])

		noticeData := protocol.FromBytes[protocol.NoticeData](notice.Data)
		require.Equal(1, noticeData.Counter) // the father machine received 1 advance-state request
		require.Equal(inputData.Quote, noticeData.Quote)
		require.Equal(0, noticeData.Index)
	}

	{ // Checks the father machine state.
		queryData := protocol.QueryData{Quote: quote, Reports: 1}
		reports := echoInspect(t, forkedMachine, queryData)

		report := protocol.FromBytes[protocol.Report](reports[0])
		require.Equal(0, report.Counter) // the forked machine received 0 advance-state requests
		require.Equal(queryData.Quote, report.Quote)
		require.Equal(0, report.Index)
	}
}

// TODO: test fork corrupted machine (server shut down).

// ------------------------------------------------------------------------------------------------

type AdvanceSuite struct{ EchoSuite }

func (suite *AdvanceSuite) TestAccept() {
	inputData := protocol.InputData{Reject: false, Exception: false}
	input := newInput(suite.T(), appContract, sender, inputData)
	outputs, reports, _, err := suite.machine.Advance(input)

	require := suite.Require()
	require.Nil(err)
	require.Empty(outputs)
	require.Empty(reports)
}

func (suite *AdvanceSuite) TestReject() {
	inputData := protocol.InputData{Reject: true, Exception: false}
	input := newInput(suite.T(), appContract, sender, inputData)
	outputs, reports, _, err := suite.machine.Advance(input)

	require := suite.Require()
	require.ErrorIs(err, rm.ErrLastInputWasRejected)
	require.Empty(outputs)
	require.Empty(reports)
}

func (suite *AdvanceSuite) TestException() {
	inputData := protocol.InputData{Reject: false, Exception: true}
	input := newInput(suite.T(), appContract, sender, inputData)
	outputs, reports, _, err := suite.machine.Advance(input)

	require := suite.Require()
	require.ErrorIs(err, rm.ErrLastInputYieldedAnException)
	require.Empty(outputs)
	require.Empty(reports)
}

func (suite *AdvanceSuite) TestNoResponse() {
	quote := `"He who controls the spice controls the universe." -- Dune 1984`
	inputData := protocol.InputData{Quote: quote}
	_, _ = echoAdvance(suite.T(), suite.machine, inputData)
}

func (suite *AdvanceSuite) TestSingleResponse() {
	quote := `"Time is an illusion. Lunchtime doubly so." -- THGTTG, Douglas Adams`

	suite.Run("Vouchers=1", func() {
		t := suite.T()
		inputData := protocol.InputData{Quote: quote, Vouchers: 1}
		outputs, _ := echoAdvance(t, suite.machine, inputData)
		voucher := expectVoucher(t, outputs[0])

		voucherData := protocol.FromBytes[protocol.VoucherData](voucher.Data)
		require := suite.Require()
		require.Equal(1, voucherData.Counter)
		require.Equal(inputData.Quote, voucherData.Quote)
		require.Equal(0, voucherData.Index)
	})

	suite.Run("Notices=1", func() {
		t := suite.T()
		inputData := protocol.InputData{Quote: quote, Notices: 1}
		outputs, _ := echoAdvance(t, suite.machine, inputData)
		notice := expectNotice(t, outputs[0])

		noticeData := protocol.FromBytes[protocol.NoticeData](notice.Data)
		require := suite.Require()
		require.Equal(2, noticeData.Counter)
		require.Equal(inputData.Quote, noticeData.Quote)
		require.Equal(0, noticeData.Index)
	})

	suite.Run("Reports=1", func() {
		inputData := protocol.InputData{Quote: quote, Reports: 1}
		_, reports := echoAdvance(suite.T(), suite.machine, inputData)

		reportData := protocol.FromBytes[protocol.Report](reports[0])
		require := suite.Require()
		require.Equal(3, reportData.Counter)
		require.Equal(inputData.Quote, reportData.Quote)
		require.Equal(0, reportData.Index)
	})
}

func (suite *AdvanceSuite) TestMultipleReponses() {
	require := suite.Require()

	inputData := protocol.InputData{
		Quote: `"Any fool can tell a crisis when it arrives.
                 The real service to the state is to detect it in embryo."
                                              -- Foundation, Isaac Asimov`,
		Vouchers: 3,
		Notices:  4,
		Reports:  5,
	}
	outputs, reports := echoAdvance(suite.T(), suite.machine, inputData)

	{ // outputs
		numberOfVouchers := 0
		numberOfNotices := 0
		for _, output := range outputs {
			voucher, notice, err := rm.DecodeOutput(output)
			require.Nil(err)
			if voucher != nil {
				require.Nil(notice)
				require.Equal(sender, voucher.Address)
				require.Equal(protocol.VoucherValue.Int64(), voucher.Value.Int64())

				voucherData := protocol.FromBytes[protocol.VoucherData](voucher.Data)
				require.Equal(1, voucherData.Counter)
				require.Equal(inputData.Quote, voucherData.Quote)
				require.Equal(numberOfVouchers, voucherData.Index)
				numberOfVouchers++
			} else if notice != nil {
				require.Nil(voucher)
				noticeData := protocol.FromBytes[protocol.NoticeData](notice.Data)
				require.Equal(1, noticeData.Counter)
				require.Equal(inputData.Quote, noticeData.Quote)
				require.Equal(numberOfNotices, noticeData.Index)
				numberOfNotices++
			} else {
				panic("not a voucher and not a notice")
			}
		}
		require.Equal(inputData.Vouchers, numberOfVouchers)
		require.Equal(inputData.Notices, numberOfNotices)
	}

	{ // reports
		for i, report := range reports {
			reportData := protocol.FromBytes[protocol.Report](report)
			require.Equal(1, reportData.Counter)
			require.Equal(inputData.Quote, reportData.Quote)
			require.Equal(i, reportData.Index)
		}
	}
}

// ------------------------------------------------------------------------------------------------

type CyclesSuite struct{ EchoSuite }

// When we send a request to the machine with machine.Max set too low,
// the function call should return with the ErrMaxCycles error.
func (suite *CyclesSuite) TestMaxCyclesError() {
	quote := `"I must not fear. Fear is the mind-killer." -- Dune, Frank Herbert`

	suite.machine.Inc = 1000
	suite.machine.Max = 1

	t := suite.T()
	queryData := protocol.InputData{Quote: quote}
	query := newQuery(t, queryData)
	reports, err := suite.machine.Inspect(query)
	require.Equal(t, rm.ErrMaxCycles, err)
	require.Empty(t, reports)
}

// When we send a request to the machine with machine.Max set too low,
// but the request gets fully processed within one run of machine.Inc cycles,
// then the function call should not return with the ErrMaxCycles error.
//
// If the machine needs two runs to process the input (for example, in case of an automatic yield),
// then the function call should return with the ErrMaxCycles error.
func (suite *CyclesSuite) TestSmallMaxBigInc() {
	quote := `"Arrakis teaches the attitude of the knife
               - chopping off what's incomplete and saying:
               'Now, it's complete because it's ended here.'"
                                      -- Dune, Frank Herbert`

	suite.machine.Max = 1
	suite.machine.Inc = rm.DefaultMax

	suite.Run("Notices=0", func() {
		t := suite.T()
		inputData := protocol.InputData{Quote: quote}
		_, _ = echoAdvance(t, suite.machine, inputData)
	})

	suite.Run("Notices=1", func() {
		t := suite.T()
		inputData := protocol.InputData{Quote: quote, Notices: 1}
		input := newInput(t, appContract, sender, inputData)
		outputs, reports, _, err := suite.machine.Advance(input)
		require.Equal(t, rm.ErrMaxCycles, err)
		require.Len(t, outputs, 1)
		require.Empty(t, reports)
	})
}

func (suite *CyclesSuite) TestInc() {
	quote := `"Isn't it enough to see that a garden is beautiful
               without having to believe that there are fairies at the bottom of it, too?"
                                                                 -- THGTTG, Douglas Adams`
	queryData := protocol.InputData{Quote: quote}

	suite.Run("Inc=DefaultInc", func() {
		suite.machine.Inc = rm.DefaultInc
		query := newQuery(suite.T(), queryData)
		reports, err := suite.machine.Inspect(query)
		require := suite.Require()
		require.Nil(err)
		require.Empty(reports)
	})

	suite.Run("Inc=100", func() {
		suite.machine.Inc = 100
		query := newQuery(suite.T(), queryData)
		reports, err := suite.machine.Inspect(query)
		require := suite.Require()
		require.Nil(err)
		require.Empty(reports)
	})

	suite.Run("Inc=9", func() {
		suite.T().Skip() // NOTE: takes too long
		suite.machine.Inc = 9
		query := newQuery(suite.T(), queryData)
		reports, err := suite.machine.Inspect(query)
		require := suite.Require()
		require.Nil(err)
		require.Empty(reports)
	})
}

// ------------------------------------------------------------------------------------------------

type InspectSuite struct{ EchoSuite }

func (suite *InspectSuite) TestAccept() {
	queryData := protocol.QueryData{Reject: false, Exception: false}
	query := newQuery(suite.T(), queryData)
	reports, err := suite.machine.Inspect(query)

	require := suite.Require()
	require.Nil(err)
	require.Empty(reports)
}

func (suite *InspectSuite) TestReject() {
	queryData := protocol.QueryData{Reject: true, Exception: false}
	query := newQuery(suite.T(), queryData)
	reports, err := suite.machine.Inspect(query)

	require := suite.Require()
	require.ErrorIs(err, rm.ErrLastInputWasRejected)
	require.Empty(reports)
}

func (suite *InspectSuite) TestException() {
	queryData := protocol.QueryData{Reject: false, Exception: true}
	query := newQuery(suite.T(), queryData)
	reports, err := suite.machine.Inspect(query)

	require := suite.Require()
	require.ErrorIs(err, rm.ErrLastInputYieldedAnException)
	require.Empty(reports)
}
func (suite *InspectSuite) TestNoResponse() {
	quote := `"He who controls the spice controls the universe." -- Dune 1984`
	queryData := protocol.QueryData{Quote: quote}
	reports := echoInspect(suite.T(), suite.machine, queryData)
	require.Empty(suite.T(), reports)
}

func (suite *InspectSuite) TestSingleResponse() {
	quote := `"Time is an illusion. Lunchtime doubly so." -- THGTTG, Douglas Adams`
	queryData := protocol.QueryData{Quote: quote, Reports: 1}
	reports := echoInspect(suite.T(), suite.machine, queryData)

	reportData := protocol.FromBytes[protocol.Report](reports[0])
	require := suite.Require()
	require.Zero(reportData.Counter)
	require.Equal(queryData.Quote, reportData.Quote)
	require.Equal(0, reportData.Index)
}

func (suite *InspectSuite) TestMultipleReponses() {
	require := suite.Require()
	quote := `"Any fool can tell a crisis when it arrives.
               The real service to the state is to detect it in embryo."
                                            -- Foundation, Isaac Asimov`
	queryData := protocol.QueryData{Quote: quote, Reports: 7}
	reports := echoInspect(suite.T(), suite.machine, queryData)
	for i, report := range reports {
		reportData := protocol.FromBytes[protocol.Report](report)
		require.Zero(reportData.Counter)
		require.Equal(queryData.Quote, reportData.Quote)
		require.Equal(i, reportData.Index)
	}
}

// ------------------------------------------------------------------------------------------------

// EchoSuite is a superclass for other test suites
// that use the "echo" app; it does not contain tests.
type EchoSuite struct {
	suite.Suite
	machine *rm.RollupsMachine
}

func (suite *EchoSuite) SetupTest() {
	require := require.New(suite.T())

	// Starts the server.
	address, err := rm.StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
	require.Nil(err)

	// Loads the "echo" application.
	path := "echo/snapshot"
	suite.machine, err = rm.Load(path, address, &emulator.MachineRuntimeConfig{})
	require.Nil(err)
	require.NotNil(suite.machine)
}

func (suite *EchoSuite) TearDownTest() {
	// Destroys the machine and shuts down the server.
	err := suite.machine.Destroy()
	require.Nil(suite.T(), err)
}

var appContract = [20]byte{}
var sender = [20]byte{}

// advance sends an advance-state request to an echo machine
// and asserts that it produced the correct amount of outputs and reports.
func echoAdvance(t *testing.T, machine *rm.RollupsMachine, data protocol.InputData) (
	[]rm.Output,
	[]rm.Report,
) {
	input := newInput(t, appContract, sender, data)
	outputs, reports, outputsHash, err := machine.Advance(input)
	require.Nil(t, err)
	require.Len(t, outputs, data.Vouchers+data.Notices)
	require.Len(t, reports, data.Reports)
	require.Len(t, outputsHash, model.HashSize)
	return outputs, reports
}

// echoInspect sends an inspect-state request to an echo machine
// and asserts that it produced the correct amount of reports.
func echoInspect(t *testing.T, machine *rm.RollupsMachine, data protocol.QueryData) []rm.Report {
	query := newQuery(t, data)
	reports, err := machine.Inspect(query)
	require.Nil(t, err)
	require.Len(t, reports, data.Reports)
	return reports
}

// expectVoucher decodes the output and asserts that it is a voucher.
func expectVoucher(t *testing.T, output rm.Output) *rm.Voucher {
	voucher, notice, err := rm.DecodeOutput(output)
	require.Nil(t, err)
	require.NotNil(t, voucher)
	require.Nil(t, notice)
	require.Equal(t, sender, voucher.Address)
	require.Equal(t, protocol.VoucherValue.Int64(), voucher.Value.Int64())
	return voucher
}

// expectNotice decodes the output and asserts that it is a notice.
func expectNotice(t *testing.T, output rm.Output) *rm.Notice {
	voucher, notice, err := rm.DecodeOutput(output)
	require.Nil(t, err)
	require.Nil(t, voucher)
	require.NotNil(t, notice)
	return notice
}
