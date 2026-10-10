// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/test/tooling/command"
	"github.com/stretchr/testify/require"
)

func TestReplayDatabaseErrorsDoNotExposeSettings(t *testing.T) {
	for _, flag := range []bool{false, true} {
		name := "environment"
		if flag {
			name = "flag"
		}
		t.Run(name, func(t *testing.T) {
			const marker = "REPLAY_DATABASE_SECRET"
			dsn := "postgres://" + marker + ":" + marker + "@127.0.0.1:1/" + marker + "?connect_timeout=" + marker
			args := []string{"replay", "--template=unused", "--application=echo", "--store=unused", "--to-epoch=0"}
			var env []string
			if flag {
				args = append(args, "--database-connection="+dsn)
			} else {
				env = []string{config.DATABASE_CONNECTION + "=" + dsn}
			}
			stdout, stderr, err := command.Run(t, env, args...)
			require.Error(t, err)
			require.Contains(t, stdout+stderr, "failed to parse Postgres connection string")
			require.NotContains(t, stdout+stderr, marker)
		})
	}
}
