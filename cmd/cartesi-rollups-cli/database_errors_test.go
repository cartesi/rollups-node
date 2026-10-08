// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/test/tooling/command"
	"github.com/stretchr/testify/require"
)

func TestDatabaseRuntimeErrorsDoNotExposeSettings(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{"application list", []string{"app", "list"}},
		{"database read", []string{secretReadCommand, readInputsOperation, secretApplicationName}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var firstOutput string
			for i, marker := range []string{"DATABASE_SECRET_A", "DATABASE_SECRET_B"} {
				dsn := "postgres://" + marker + ":" + marker + "@127.0.0.1:1/" + marker + "?connect_timeout=" + marker
				stdout, stderr, err := command.Run(t, []string{config.DATABASE_CONNECTION + "=" + dsn}, test.args...)
				require.Error(t, err)
				output := stdout + stderr
				require.Contains(t, output, "failed to parse Postgres connection string")
				require.NotContains(t, output, marker)
				if i == 0 {
					firstOutput = output
				} else {
					require.Equal(t, firstOutput, output, "connection settings must not affect failure output")
				}
			}
		})
	}
}
