// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testEchoPrtCleanup(t *testing.T) {
	for _, expired := range []bool{false, true} {
		name := "cancelled"
		if expired {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			installTestCLI(t, `printf '%s\n' "$@" > "$CARTESI_TEST_CLI_CAPTURE"`)
			capture := filepath.Join(t.TempDir(), "arguments")
			t.Setenv("CARTESI_TEST_CLI_CAPTURE", capture)
			logPath := filepath.Join(t.TempDir(), "node.log")
			require.NoError(t, os.WriteFile(logPath, nil, 0o600))
			t.Setenv("CARTESI_TEST_NODE_LOG_FILE", logPath)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if expired {
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				defer cancel()
			}
			require.Error(t, ctx.Err())
			s := &EchoPrtSuite{ctx: ctx, cancel: cancel, appName: "finished-case"}
			s.SetT(t)
			s.StartLogCapture()
			s.TearDownTest()
			arguments, err := os.ReadFile(capture)
			require.NoError(t, err, "cleanup must execute despite the test's cancelled context")
			require.Equal(t, "app\nstatus\n"+s.appName+"\ndisabled\n--yes\n", string(arguments))
		})
	}
}
