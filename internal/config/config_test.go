// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"log/slog"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestResolveServiceLogLevel(t *testing.T) {
	t.Run("returns global level when no override is set", func(t *testing.T) {
		viper.Reset()
		level := ResolveServiceLogLevel(ServiceAdvancer, slog.LevelInfo)
		require.Equal(t, slog.LevelInfo, level)
	})

	t.Run("returns per-service level when override is set", func(t *testing.T) {
		viper.Reset()
		viper.Set(LOG_LEVEL_ADVANCER, "debug")
		level := ResolveServiceLogLevel(ServiceAdvancer, slog.LevelInfo)
		require.Equal(t, slog.LevelDebug, level)
	})

	t.Run("returns global level when override has invalid value", func(t *testing.T) {
		viper.Reset()
		viper.Set(LOG_LEVEL_VALIDATOR, "invalid")
		level := ResolveServiceLogLevel(ServiceValidator, slog.LevelWarn)
		require.Equal(t, slog.LevelWarn, level)
	})

	t.Run("returns global level for unknown service name", func(t *testing.T) {
		viper.Reset()
		level := ResolveServiceLogLevel("unknown-service", slog.LevelError)
		require.Equal(t, slog.LevelError, level)
	})

	t.Run("each service resolves independently", func(t *testing.T) {
		viper.Reset()
		viper.Set(LOG_LEVEL_ADVANCER, "debug")
		viper.Set(LOG_LEVEL_CLAIMER, "error")

		require.Equal(t, slog.LevelDebug, ResolveServiceLogLevel(ServiceAdvancer, slog.LevelInfo))
		require.Equal(t, slog.LevelError, ResolveServiceLogLevel(ServiceClaimer, slog.LevelInfo))
		require.Equal(t, slog.LevelInfo, ResolveServiceLogLevel(ServiceValidator, slog.LevelInfo))
		require.Equal(t, slog.LevelInfo, ResolveServiceLogLevel(ServiceEvmReader, slog.LevelInfo))
		require.Equal(t, slog.LevelInfo, ResolveServiceLogLevel(ServiceJsonrpc, slog.LevelInfo))
		require.Equal(t, slog.LevelInfo, ResolveServiceLogLevel(ServicePrt, slog.LevelInfo))
	})

	t.Run("all service constants are mapped", func(t *testing.T) {
		viper.Reset()
		services := []string{
			ServiceAdvancer, ServiceClaimer, ServiceEvmReader,
			ServiceJsonrpc, ServicePrt, ServiceValidator,
		}
		for _, name := range services {
			_, ok := serviceLogLevelGetters[name]
			require.True(t, ok, "service %q should have a log level getter", name)
		}
	})
}
