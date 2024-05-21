// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package test

import (
	"fmt"
	"log"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/test/snapshot"

	"github.com/stretchr/testify/suite"
)

// Basic smoke tests to check if the binding is implemented correctly.
func TestLibcmt(t *testing.T) {
	suite.Run(t, new(LibcmtSuite))
}

type LibcmtSuite struct{ suite.Suite }

func (suite *LibcmtSuite) SetupSuite() {
	log.SetFlags(log.Ltime)
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func (suite *LibcmtSuite) test(code string, breakReason emulator.BreakReason) {
	header := `
        package main

        import "github.com/cartesi/rollups-node/test/libcmt"
    `
	code = header + code
	require := suite.Require()
	snapshot, err := snapshot.FromGoCode(1_000_000_000, code)
	require.Nil(err)
	defer func() { require.Nil(snapshot.Close()) }()
	require.Equal(breakReason, snapshot.BreakReason,
		"expected %s -- actual %s", breakReason, snapshot.BreakReason)
}

func (suite *LibcmtSuite) TestNewRollup() {
	code := `
        func main() {
            _, err := libcmt.NewRollup()
            if err != nil { for{} }
        }
    `
	suite.test(code, emulator.BreakReasonHalted)
}

func (suite *LibcmtSuite) TestClose() {
	code := `
        import "errors"
        func main() {
            rollup, err := libcmt.NewRollup()
            if err != nil { for{} }
            err = rollup.Close()
            if err != nil { for{} }
            err = rollup.Close()
            if !errors.Is(err, libcmt.ErrClosed) { for{} }
        }
    `
	suite.test(code, emulator.BreakReasonHalted)
}

func (suite *LibcmtSuite) TestFinish() {
	code := `
        func main() {
            rollup, err := libcmt.NewRollup()
            if err != nil { for{} }
            _, _ = rollup.Finish(%s)
            for {}
        }
    `
	suite.Run("True", func() {
		code := fmt.Sprintf(code, "true")
		suite.test(code, emulator.BreakReasonYieldedManually)
	})

	suite.Run("False", func() {
		code := fmt.Sprintf(code, "false")
		suite.test(code, emulator.BreakReasonYieldedManually)
	})
}

func (suite *LibcmtSuite) TestEmitVoucher() {
	code := `
        func main() {
            rollup, err := libcmt.NewRollup()
            if err != nil { for{} }
            _, _ = rollup.EmitVoucher(libcmt.Address{}, []byte{}, []byte("Whiplash"))
            for {}
        }
    `
	suite.test(code, emulator.BreakReasonYieldedAutomatically)
}

func (suite *LibcmtSuite) TestEmitNotice() {
	code := `
        func main() {
            rollup, err := libcmt.NewRollup()
            if err != nil { for{} }
            _, _ = rollup.EmitNotice([]byte("Past Lives"))
            for {}
        }
    `
	suite.test(code, emulator.BreakReasonYieldedAutomatically)
}

func (suite *LibcmtSuite) TestEmitReport() {
	code := `
        func main() {
            rollup, err := libcmt.NewRollup()
            if err != nil { for{} }
            _ = rollup.EmitReport([]byte("Challengers"))
            for {}
        }
    `
	suite.test(code, emulator.BreakReasonYieldedAutomatically)
}

func (suite *LibcmtSuite) TestEmitException() {
	code := `
        func main() {
            rollup, err := libcmt.NewRollup()
            if err != nil { for{} }
            _ = rollup.EmitException([]byte("Monster"))
            for {}
        }
    `
	suite.test(code, emulator.BreakReasonYieldedManually)
}
