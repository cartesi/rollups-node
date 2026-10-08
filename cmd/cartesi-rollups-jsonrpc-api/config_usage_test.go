// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/test/tooling/command"
	"github.com/stretchr/testify/require"
)

func TestMalformedSensitiveConfigUsageOmitsValue(t *testing.T) {
	const marker = "MALFORMED_DATABASE_SECRET"
	stdout, stderr, err := command.Run(t, []string{
		config.DATABASE_CONNECTION + "=postgres://user:" + marker + "@localhost/%zz",
		config.LOG_COLOR + "=false",
	})
	require.Error(t, err)
	require.NotContains(t, stdout+stderr, marker)
	require.Contains(t, stdout+stderr, config.DATABASE_CONNECTION)
	require.Contains(t, stdout+stderr, "invalid")
	require.Contains(t, stdout+stderr, "Usage:")
}
