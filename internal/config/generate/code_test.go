// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/stretchr/testify/require"
)

const testDescription = "test"

func TestDeclaredDefaultsMatchConfiguration(t *testing.T) {
	for _, env := range sortConfig(decodeTOML(readTOML("Config.toml"))) {
		env.validate()
		got, known := config.DeclaredDefault(env.Name)
		require.True(t, known, env.Name)
		want := ""
		if env.Default != nil {
			want = *env.Default
		}
		require.Equal(t, want, got, env.Name)
	}
	_, known := config.DeclaredDefault("CARTESI_NOT_A_SETTING")
	require.False(t, known)
}

func TestGenerateCodeQuotesDefaults(t *testing.T) {
	value := "a quoted \"value\" and a newline\nwith \\slashes"
	env := Env{Name: "CARTESI_TEST_STRING", GoType: "string", Default: &value, Description: testDescription, UsedBy: []string{"cli"}}
	path := filepath.Join(t.TempDir(), "generated.go")
	generateCodeFile(path, []Env{env})
	code, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(code), fmt.Sprintf("return %q, true", value))
	require.Contains(t, string(code), fmt.Sprintf("viper.SetDefault(TEST_STRING, %q)", value))
}

func TestDeclaredDefaultsValidateFlagConversions(t *testing.T) {
	for _, tc := range []struct {
		kind  string
		value string
	}{
		{"bool", "invalid"},
		{"uint64", "-1"},
		{"uint64", "18446744073709551616"},
	} {
		env := Env{Name: "CARTESI_TEST", GoType: tc.kind, Default: &tc.value, Description: testDescription}
		require.Panics(t, env.validate)
	}
	env := Env{Name: "CARTESI_TEST", GoType: "string", HTTP: true, Description: testDescription}
	require.Panics(t, env.validate)
}
