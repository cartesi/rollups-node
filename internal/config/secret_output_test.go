// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func resetSecretConfig(t *testing.T) {
	t.Helper()
	viper.Reset()
	viper.AutomaticEnv()
	SetDefaults()
	t.Cleanup(func() {
		viper.Reset()
		viper.AutomaticEnv()
		SetDefaults()
	})
}

func TestSensitiveFileErrorsDoNotExposeInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		get  func() error
	}{
		{AUTH_MNEMONIC, func() error { _, err := GetAuthMnemonic(); return err }},
		{AUTH_PRIVATE_KEY, func() error { _, err := GetAuthPrivateKey(); return err }},
		{PRT_AUTH_MNEMONIC, func() error { _, err := GetPrtAuthMnemonic(); return err }},
		{PRT_AUTH_PRIVATE_KEY, func() error { _, err := GetPrtAuthPrivateKey(); return err }},
		{DATABASE_CONNECTION, func() error { _, err := GetDatabaseConnection(); return err }},
		{BLOCKCHAIN_HTTP_ENDPOINT, func() error { _, err := GetBlockchainHttpEndpoint(); return err }},
		{BLOCKCHAIN_HTTP_AUTHORIZATION, func() error { _, err := GetBlockchainHttpAuthorization(); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetSecretConfig(t)
			const marker = "SYNTHETIC_SECRET_INSTEAD_OF_FILENAME_76289041"
			t.Setenv(tc.name, "")
			t.Setenv(tc.name+"_FILE", filepath.Join(t.TempDir(), marker))
			err := tc.get()
			require.Error(t, err)
			require.ErrorIs(t, err, fs.ErrNotExist)
			require.Contains(t, err.Error(), tc.name+"_FILE")
			require.NotContains(t, fmt.Sprintf("%+v", err), marker)
			var pathErr *fs.PathError
			require.False(t, errors.As(err, &pathErr), "a reachable PathError retains the mistaken secret")
		})
	}
}

func TestRedactedFormattingDoesNotExposeValue(t *testing.T) {
	const marker = "SYNTHETIC_REDACTED_VALUE_5098724316"
	secret := RedactedString{Value: marker}
	for _, value := range []any{secret, &secret} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			output := fmt.Sprintf(format, value)
			require.NotContains(t, output, marker)
			require.Contains(t, output, "[REDACTED]")
		}
		data, err := json.Marshal(struct{ Secret any }{Secret: value})
		require.NoError(t, err)
		require.NotContains(t, string(data), marker)
		require.Contains(t, string(data), "[REDACTED]")
		for _, handler := range []func(*bytes.Buffer) slog.Handler{
			func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
			func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
		} {
			var output bytes.Buffer
			slog.New(handler(&output)).Info("config", "secret", value)
			require.NotContains(t, output.String(), marker)
			require.Contains(t, output.String(), "[REDACTED]")
		}
	}
}

func TestRedactedUintErrorsDoNotExposeInput(t *testing.T) {
	for _, tc := range []struct {
		input string
		cause error
	}{
		{"SYNTHETIC_INVALID_NUMBER_6302907431", strconv.ErrSyntax},
		{"18446744073709551616", strconv.ErrRange},
	} {
		value, err := ToRedactedUint32FromString(tc.input)
		require.Error(t, err)
		require.ErrorIs(t, err, tc.cause)
		require.NotContains(t, err.Error(), tc.input)
		var numberErr *strconv.NumError
		require.False(t, errors.As(err, &numberErr))
		require.Zero(t, value.Value)
	}
	value, err := ToRedactedUint32FromString("4294967295")
	require.NoError(t, err)
	require.Equal(t, uint32(4294967295), value.Value)
}

func TestSensitiveFilesStillLoad(t *testing.T) {
	resetSecretConfig(t)
	const marker = "SYNTHETIC_FILE_SECRET_2431098547"
	path := filepath.Join(t.TempDir(), "credential")
	require.NoError(t, os.WriteFile(path, []byte("  "+marker+"\n"), 0600))
	t.Setenv(AUTH_MNEMONIC, "")
	t.Setenv(AUTH_MNEMONIC_FILE, path)
	value, err := GetAuthMnemonic()
	require.NoError(t, err)
	require.Equal(t, marker, value.Value)
	t.Setenv(AUTH_MNEMONIC, "SYNTHETIC_DIRECT_VALUE")
	value, err = GetAuthMnemonic()
	require.NoError(t, err)
	require.Equal(t, "SYNTHETIC_DIRECT_VALUE", value.Value)
}

func TestSensitiveFileErrorsKeepSafeFilesystemCause(t *testing.T) {
	resetSecretConfig(t)
	const marker = "SYNTHETIC_SECRET_FILE_PATH_1743852690"
	directory := filepath.Join(t.TempDir(), marker)
	require.NoError(t, os.Mkdir(directory, 0700))
	t.Setenv(AUTH_PRIVATE_KEY, "")
	t.Setenv(AUTH_PRIVATE_KEY_FILE, directory)
	_, err := GetAuthPrivateKey()
	require.ErrorIs(t, err, syscall.EISDIR)
	require.NotContains(t, err.Error(), marker)

	path := filepath.Join(t.TempDir(), marker)
	require.NoError(t, os.WriteFile(path, []byte("secret"), 0000))
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	if _, probeErr := os.ReadFile(path); probeErr == nil {
		t.Skip("current user can bypass filesystem permissions")
	}
	t.Setenv(AUTH_PRIVATE_KEY_FILE, path)
	_, err = GetAuthPrivateKey()
	require.ErrorIs(t, err, fs.ErrPermission)
	require.NotContains(t, err.Error(), marker)
}

func TestRedactedNumericFormattingAndDeserialization(t *testing.T) {
	secret := RedactedUint{Value: 1234567890}
	for _, value := range []any{secret, &secret} {
		for _, format := range []string{"%d", "%x", "%#v"} {
			require.Equal(t, "[REDACTED]", fmt.Sprintf(format, value))
		}
	}
	// JSON input and direct value access used by signer configuration remain valid.
	var value RedactedString
	require.NoError(t, json.Unmarshal([]byte(`{"Value":"SYNTHETIC_SIGNER_VALUE"}`), &value))
	require.Equal(t, "SYNTHETIC_SIGNER_VALUE", value.Value)
}
