// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"log/slog"
	"net/url"
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

func TestSafeURL(t *testing.T) {
	t.Run("String redacts full URL", func(t *testing.T) {
		u, _ := url.Parse("https://eth-mainnet.alchemyapi.io/v2/my-secret-key")
		safe := NewSafeURL(u)
		require.Equal(t, "https://eth-mainnet.alchemyapi.io", safe.String())
	})

	t.Run("Raw returns full URL", func(t *testing.T) {
		raw := "https://eth-mainnet.alchemyapi.io/v2/my-secret-key"
		u, _ := url.Parse(raw)
		safe := NewSafeURL(u)
		require.Equal(t, raw, safe.Raw())
	})

	t.Run("String redacts userinfo passwords", func(t *testing.T) {
		u, _ := url.Parse("postgres://user:secret@localhost:5432/mydb")
		safe := NewSafeURL(u)
		require.Equal(t, "postgres://localhost:5432", safe.String())
	})

	t.Run("zero value is safe", func(t *testing.T) {
		var safe SafeURL
		require.Equal(t, "[REDACTED]", safe.String())
		require.Equal(t, "", safe.Raw())
		require.Equal(t, "", safe.Scheme())
		require.Equal(t, "", safe.Host())
	})

	t.Run("nil url is safe", func(t *testing.T) {
		safe := NewSafeURL(nil)
		require.Equal(t, "[REDACTED]", safe.String())
		require.Equal(t, "", safe.Raw())
	})

	t.Run("LogValue returns redacted string", func(t *testing.T) {
		u, _ := url.Parse("https://infura.io/v3/my-api-key")
		safe := NewSafeURL(u)
		logVal := safe.LogValue()
		require.Equal(t, slog.KindString, logVal.Kind())
		require.Equal(t, "https://infura.io", logVal.String())
	})

	t.Run("Scheme and Host delegate correctly", func(t *testing.T) {
		u, _ := url.Parse("wss://example.com:8546/ws")
		safe := NewSafeURL(u)
		require.Equal(t, "wss", safe.Scheme())
		require.Equal(t, "example.com:8546", safe.Host())
	})

	t.Run("ToURLFromString returns SafeURL", func(t *testing.T) {
		safe, err := ToURLFromString("https://host.example/path?key=secret")
		require.NoError(t, err)
		require.Equal(t, "https://host.example", safe.String())
		require.Equal(t, "https://host.example/path?key=secret", safe.Raw())
	})

	t.Run("ToURLFromString error returns zero SafeURL", func(t *testing.T) {
		safe, err := ToURLFromString("://invalid")
		require.Error(t, err)
		require.Equal(t, "[REDACTED]", safe.String())
	})
}
