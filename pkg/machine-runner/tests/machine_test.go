// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/cartesi/rollups-node/pkg/machine-runner/machine"
	"github.com/cartesi/rollups-node/pkg/machine-runner/machine/binding"

	"github.com/stretchr/testify/suite"
)

func init() {
	CreateSimpleSnapshot("rollup-accept", "rollup accept")
	CreateSimpleSnapshot("rollup-reject", "rollup reject")
	CreateSimpleSnapshot("rollup-notice", payload("Hari Seldon")+" | rollup notice")
	CreateSimpleSnapshot("rollup-exception", payload("Paul Atreides")+" | rollup exception")

	CreateGollupSnapshot("advance-inspect", 4)
}

// ------------------------------------------------------------------------------------------------

type LoadSuite struct {
	suite.Suite
	address string
	config  binding.RuntimeConfig
}

func (suite *LoadSuite) SetupTest() {
	var err error
	suite.address, err = machine.StartServer(machine.ServerLogLevelInfo, 0, os.Stdout, os.Stderr)
	suite.Nil(err)
	suite.config = binding.RuntimeConfig{}
}

func (suite *LoadSuite) TearDownTest() {
	err := machine.StopServer(suite.address)
	suite.Nil(err)
}

func (suite *LoadSuite) TestLoad() {
	m, err := machine.Load(suite.address, "rollup-accept/snapshot", suite.config)
	suite.Nil(err)
	suite.NotNil(m)
}

func (suite *LoadSuite) TestInvalidAddress() {
	// NOTE: This test does not require an initialized server; the setup is incidental.
	m, err := machine.Load("invalid address", "rollup-accept/snapshot", suite.config)
	suite.NotNil(err)
	suite.Nil(m)
	suite.Equal(binding.ErrorRuntime, err.(binding.Error).Code)
}

func (suite *LoadSuite) TestNonExistingSnapshot() {
	m, err := machine.Load(suite.address, "non-existing/snapshot", suite.config)
	suite.NotNil(err)
	suite.Nil(m)
	suite.Equal(binding.ErrorRuntime, err.(binding.Error).Code)
}

func (suite *LoadSuite) TestNotPrimedNotAtManualYield() {
	m, err := machine.Load(suite.address, "rollup-notice/snapshot", suite.config)
	suite.NotNil(err)
	suite.ErrorIs(err, machine.ErrNotAtManualYield)
	suite.Nil(m)
}

func (suite *LoadSuite) TestNotPrimedInputRejected() {
	m, err := machine.Load(suite.address, "rollup-reject/snapshot", suite.config)
	suite.NotNil(err)
	suite.ErrorIs(err, machine.ErrLastInputWasRejected)
	suite.Nil(m)
}

func (suite *LoadSuite) TestNotPrimedInputException() {
	m, err := machine.Load(suite.address, "rollup-exception/snapshot", suite.config)
	suite.NotNil(err)
	suite.ErrorIs(err, machine.ErrLastInputYieldedAnException)
	suite.Nil(m)
}

func TestLoad(t *testing.T) {
	suite.Run(t, new(LoadSuite))
}

// ------------------------------------------------------------------------------------------------

/*
type ForkSuite struct {
	suite.Suite
	machine *Machine
}

func (suite *ForkSuite) SetupTest() {
	address, err := StartServer(binary, ServerLogLevelInfo, 0, os.Stdout, os.Stderr)
	suite.Nil(err)

	suite.machine, err = Load(address, snapshot, binding.RuntimeConfig{})
	suite.Nil(err)
	suite.NotNil(suite.machine)
}

func (suite *ForkSuite) TearDownTest() {
	err := suite.machine.Destroy()
	suite.Nil(err)
}

	machineA := load(t)
	defer machineA.Destroy()

	advance(t, machineA, "first advance on machine A", 0, 1)
	inspect(t, machineA, "", 1, 1)
	inspect(t, machineA, "", 1, 1)

	machineB, err := machineA.Fork()
	require.Nil(t, err)
	require.NotNil(t, machineB)
	defer machineB.Destroy()

	advance(t, machineA, "second advance on machine A", 1, 2)
	inspect(t, machineA, "", 2, 1)

	advance(t, machineB, "first advance on machine B (but counter came from A)", 1, 2)
	inspect(t, machineB, "", 2, 1)

	advance(t, machineA, "third advance on machine A", 2, 3)
	inspect(t, machineA, "", 3, 1)
*/

// ------------------------------------------------------------------------------------------------

type AdvanceInspectSuite struct {
	suite.Suite
	machine *machine.Machine
}

func (suite *AdvanceInspectSuite) SetupTest() {
	address, err := machine.StartServer(machine.ServerLogLevelInfo, 0, os.Stdout, os.Stderr)
	suite.Nil(err)

	machine, err := machine.Load(address, "advance-inspect/snapshot", binding.RuntimeConfig{})
	suite.Nil(err)
	suite.NotNil(machine)

	suite.machine = machine
}

func (suite *AdvanceInspectSuite) TearDownTest() {
	err := suite.machine.Destroy()
	suite.Nil(err)
	suite.machine = nil
}

func (suite *AdvanceInspectSuite) TestSingleOutput() {
	fmt.Println("Start")
	defer fmt.Println("End")

	// s := `Any fool can tell a crisis when it arrives.
	//       The real service to the state is to detect it in embryo.
	//                                    -- Isaac Asimov, Foundation`
	s := "nugget"
	input := machine.Input{Data: []byte(s)}
	request, err := input.Encode()
	suite.Nil(err)

	fmt.Println("Input", input, s)

	outputs, err := suite.machine.Advance(request)
	suite.Nil(err)
	suite.Len(outputs, 1)

	fmt.Println("Output:", string(outputs[0]))
}

// Advance/Inspect ok (single output).
// Advance/Inspect ok (multiple outputs).
// Advance/Inspect with small cycle Max (ErrReachedMaxCycles).
// Advance/Inspect with small cycle Increment.
// Advance/Inspect input rejected.
// Advance/Inspect input exception.

func TestAdvanceInspect(t *testing.T) {
	suite.Run(t, new(AdvanceInspectSuite))
}

// ------------------------------------------------------------------------------------------------

/*

- Fork ok.
- Fork corrupted machine (server shut down).

- Advance receiving reports.
- Inspect receiving outputs.

- ErrHalted.
- ErrFailed.
- ErrYieldedSoftly.

*/

/*
func TestAdvance(t *testing.T) {
	machine := load(t)
	defer machine.Destroy()

	// single response
	advance(t, machine, "advance", 0, 1)

	// multiple responses
	advance(t, machine, "multiple", 1, 6)
}

func TestInspect(t *testing.T) {
	machine := load(t)
	defer machine.Destroy()

	// single response
	inspect(t, machine, "inspect", 0, 1)

	// multiple responses
	inspect(t, machine, "multiple", 0, 10)
}

func TestFork(t *testing.T) {
	machineA := load(t)
	defer machineA.Destroy()

	advance(t, machineA, "first advance on machine A", 0, 1)
	inspect(t, machineA, "", 1, 1)
	inspect(t, machineA, "", 1, 1)

	machineB, err := machineA.Fork()
	require.Nil(t, err)
	require.NotNil(t, machineB)
	defer machineB.Destroy()

	advance(t, machineA, "second advance on machine A", 1, 2)
	inspect(t, machineA, "", 2, 1)

	advance(t, machineB, "first advance on machine B (but counter came from A)", 1, 2)
	inspect(t, machineB, "", 2, 1)

	advance(t, machineA, "third advance on machine A", 2, 3)
	inspect(t, machineA, "", 3, 1)
}

// ------------------------------------------------------------------------------------------------

func startServer(t *testing.T, port uint32) (address string) {
	address, err := StartServer(binary, ServerLogLevelInfo, port, os.Stdout, os.Stderr)
	require.Nil(t, err)
	return address
}

func load(t *testing.T, address string) *Machine {
	config := binding.RuntimeConfig{}
	machine, err := Load(address, snapshot, config)
	require.Nil(t, err)
	require.NotNil(t, machine)
	return machine
}

func advance(t *testing.T, machine *Machine, data string, counter1, counter2 int) {
	require := require.New(t)

	input := Input{Data: []byte(data)}
	request, err := input.Encode()
	require.Nil(err)

	outputs, err := machine.Advance(request)
	require.Nil(err)
	require.Len(outputs, counter2-counter1)

	for _, output := range outputs {
		voucher, notice, err := Decode(output)
		require.Nil(err)
		require.Nil(voucher)

		require.Equal(fmt.Sprintf("%s (%d)", data, counter1), string(notice.Data))

		counter1++
	}
}

func inspect(t *testing.T, machine *Machine, data string, counter, length int) {
	require := require.New(t)

	query := Query{Data: []byte(data)}
	request, err := query.Encode()
	require.Nil(err)

	reports, err := machine.Inspect(request)
	require.Nil(err)
	require.Len(reports, length)

	for _, report := range reports {
		require.Equal(fmt.Sprintf("%d", counter), string(report))
	}
}
*/

// ------------------------------------------------------------------------------------------------

func payload(s string) string {
	return fmt.Sprintf("echo '{ \"payload\": \"%s\" }'", s)
}
