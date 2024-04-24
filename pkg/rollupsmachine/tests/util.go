// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package tests

import (
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/tests/echo/protocol"
	"github.com/stretchr/testify/require"
)

const serverVerbosity = rollupsmachine.ServerVerbosityInfo

func init() {
	log.SetFlags(log.Ltime)
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func initT(t *testing.T) {
	var entrypoint string
	cycles := uint64(1_000_000_000)

	slog.Debug("CREATING TEST MACHINES =============================")

	entrypoint = rollup("accept")
	simpleSnapshot(t, "rollup-accept", entrypoint, cycles, emulator.BreakReasonYieldedManually)

	entrypoint = rollup("reject")
	simpleSnapshot(t, "rollup-reject", entrypoint, cycles, emulator.BreakReasonYieldedManually)

	entrypoint = rollup("exception", "Paul Atreides")
	simpleSnapshot(t, "rollup-exception", entrypoint, cycles, emulator.BreakReasonYieldedManually)

	entrypoint = rollup("notice", "Hari Seldon")
	simpleSnapshot(t, "rollup-notice", entrypoint, cycles, emulator.BreakReasonYieldedAutomatically)

	crossCompiledSnapshot(t, "echo", cycles, emulator.BreakReasonYieldedManually)

	slog.Debug("FINISHED CREATING TEST MACHINES ====================")
}

func payload(s string) string {
	return fmt.Sprintf("echo '{ \"payload\": \"0x%s\" }'", hex.EncodeToString([]byte(s)))
}

func rollup(s ...string) (cmd string) {
	switch s[0] {
	case "accept", "reject":
		cmd = "rollup " + s[0]
	case "notice", "exception":
		cmd = payload(s[1]) + " | " + "rollup " + s[0]
	default:
		panic("invalid rollup action")
	}
	slog.Debug("stored machine", "command", cmd)
	return
}

func newInput(t *testing.T, appContract, sender [20]byte, data protocol.Data) []byte {
	bytes, err := data.ToBytes()
	require.Nil(t, err)
	input, err := rollupsmachine.Input{
		AppContract: appContract,
		Sender:      sender,
		Data:        bytes,
	}.Encode()
	require.Nil(t, err)
	return input
}

func newQuery(t *testing.T, data protocol.Data) []byte {
	bytes, err := data.ToBytes()
	require.Nil(t, err)
	query, err := rollupsmachine.Query{Data: bytes}.Encode()
	require.Nil(t, err)
	return query
}
