// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func resetFlagConfig(t *testing.T) {
	t.Helper()
	viper.Reset()
	viper.AutomaticEnv()
	config.SetDefaults()
	t.Cleanup(func() {
		viper.Reset()
		viper.AutomaticEnv()
		config.SetDefaults()
	})
}

func TestFlagHelpersKeepLiveValuesOutOfMetadata(t *testing.T) {
	const marker = "SYNTHETIC_FLAG_SECRET_6420187953"
	t.Setenv(config.DATABASE_CONNECTION, marker)
	t.Setenv(config.BLOCKCHAIN_HTTP_ENDPOINT, "https://example.test/"+marker)
	t.Setenv(config.BLOCKCHAIN_DEFAULT_BLOCK, marker)
	t.Setenv(config.LOG_LEVEL, "error")
	t.Setenv(config.LOG_COLOR, "false")
	t.Setenv(config.BLOCKCHAIN_MAX_BLOCK_RANGE, "918273645")
	for _, output := range []string{"help", "usage", "debug", "markdown"} {
		t.Run(output, func(t *testing.T) {
			resetFlagConfig(t)
			cmd := &cobra.Command{Use: "test-command", Run: func(*cobra.Command, []string) {}}
			var database, endpoint, block, level string
			var color bool
			var maxRange uint64
			AddFlagStrVar(cmd.Flags(), &database, "database", config.DATABASE_CONNECTION, "Database")
			AddFlagStrVar(cmd.Flags(), &endpoint, "endpoint", config.BLOCKCHAIN_HTTP_ENDPOINT, "Endpoint")
			AddFlagStrVarP(cmd.Flags(), &block, "block", "b", config.BLOCKCHAIN_DEFAULT_BLOCK, "Block")
			AddFlagStrVar(cmd.Flags(), &level, "level", config.LOG_LEVEL, "Level")
			AddFlagBoolVar(cmd.Flags(), &color, "color", config.LOG_COLOR, "Color")
			AddFlagUint64Var(cmd.Flags(), &maxRange, "max-range", config.BLOCKCHAIN_MAX_BLOCK_RANGE, "Range")
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			switch output {
			case "help":
				cmd.SetArgs([]string{"--help"})
				require.NoError(t, cmd.Execute())
			case "usage":
				cmd.SetArgs([]string{"--unknown-flag"})
				require.Error(t, cmd.Execute())
			case "debug":
				cmd.DebugFlags()
			case "markdown":
				require.NoError(t, doc.GenMarkdown(cmd, &buf))
			}
			require.Contains(t, buf.String(), "--database")
			require.Contains(t, buf.String(), "--endpoint")
			require.NotContains(t, buf.String(), marker)
			require.NotContains(t, buf.String(), "918273645")
			require.Contains(t, buf.String(), "finalized")
			require.Contains(t, buf.String(), "info")
			require.Empty(t, database)
			require.Empty(t, endpoint)
			require.Equal(t, "finalized", block)
			require.Equal(t, "info", level)
			require.True(t, color)
			require.Zero(t, maxRange)
			// Application resolution still consumes the environment after registration.
			require.Equal(t, marker, viper.GetString(config.DATABASE_CONNECTION))
			require.Equal(t, uint64(918273645), viper.GetUint64(config.BLOCKCHAIN_MAX_BLOCK_RANGE))
		})
	}
}

func TestFlagConfigurationPrecedence(t *testing.T) {
	const envLevel = "error"
	const fileLevel = "debug"
	for _, tc := range []struct {
		name        string
		flag        string
		env         string
		configValue string
		want        slog.Level
	}{
		{"flag wins", "warn", envLevel, fileLevel, slog.LevelWarn},
		{"env wins", "", envLevel, fileLevel, slog.LevelError},
		{"file wins", "", "", fileLevel, slog.LevelDebug},
		{"default", "", "", "", slog.LevelInfo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetFlagConfig(t)
			t.Setenv(config.LOG_LEVEL, tc.env)
			if tc.configValue != "" {
				viper.SetConfigType("toml")
				require.NoError(t, viper.ReadConfig(strings.NewReader(fmt.Sprintf("%s = %q", config.LOG_LEVEL, tc.configValue))))
			}
			cmd := &cobra.Command{Use: "test-command"}
			var backing string
			AddFlagStrVar(cmd.Flags(), &backing, "level", config.LOG_LEVEL, "Level")
			if tc.flag != "" {
				require.NoError(t, cmd.Flags().Set("level", tc.flag))
			}
			got, err := config.GetLogLevel()
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
