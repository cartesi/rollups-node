// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package command exercises real executable initialization and fatal output in tests.
package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

const (
	childProcess        = "CARTESI_TEST_SECRET_COMMAND_PROCESS"
	markdownCommand     = "markdown"
	expectedEnvironment = "CARTESI_TEST_EXPECTED_ENVIRONMENT"
	markdownDirectory   = "CARTESI_TEST_MARKDOWN_DIRECTORY"
)

// Child runs main in a selected subprocess, or generates its real command tree's Markdown.
func Child(t *testing.T, mainFunc func(), root *cobra.Command) {
	t.Helper()
	if os.Getenv(childProcess) != "1" {
		t.Skip("only runs in a command subprocess")
	}
	var expected []string
	require.NoError(t, json.Unmarshal([]byte(os.Getenv(expectedEnvironment)), &expected))
	for _, entry := range expected {
		key, value, _ := strings.Cut(entry, "=")
		actual, exists := os.LookupEnv(key)
		require.True(t, exists && actual == value, "environment input %s did not reach the child", key)
		if strings.HasPrefix(key, "CARTESI_") {
			require.True(t, viper.GetString(key) == value, "configuration input %s did not reach Viper", key)
		}
	}
	for i, arg := range os.Args {
		if arg != "--" {
			continue
		}
		args := os.Args[i+1:]
		if len(args) == 1 && args[0] == markdownCommand {
			// The parent owns cleanup: os.Exit does not run testing cleanups.
			dir := os.Getenv(markdownDirectory)
			require.NotEmpty(t, dir)
			require.NoError(t, doc.GenMarkdownTree(root, dir))
			//nolint:gosec // The parent supplies its own private test directory.
			require.NoError(t, filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				//nolint:gosec // Only freshly generated Markdown in a private test directory.
				body, err := os.ReadFile(path)
				if err == nil {
					_, err = fmt.Fprint(os.Stdout, string(body))
				}
				return err
			}))
		} else {
			os.Args = append([]string{"cartesi-secret-test"}, args...)
			mainFunc()
		}
		os.Exit(0)
	}
	t.Fatal("missing subprocess argument separator")
}

// Run sets configuration before package initialization and captures both output streams.
func Run(t *testing.T, env []string, args ...string) (string, string, error) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	const timeout = 30 * time.Second
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	processArgs := append([]string{"-test.run=^TestSecretCommandProcess$", "--"}, args...)
	process := exec.CommandContext(ctx, executable, processArgs...)
	// A child may exit while a grandchild still holds its output pipes open.
	// Bound output draining as well as the direct child's lifetime.
	process.WaitDelay = time.Second
	// Keep only platform execution settings. All application configuration and
	// credentials come from explicit test inputs, not the developer's shell.
	for _, key := range []string{"PATH", "TMPDIR", "TMP", "TEMP", "SYSTEMROOT", "WINDIR", "LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH"} {
		if value, exists := os.LookupEnv(key); exists {
			process.Env = append(process.Env, key+"="+value)
		}
	}
	expected, err := json.Marshal(env)
	require.NoError(t, err)
	process.Env = append(process.Env, childProcess+"=1", expectedEnvironment+"="+string(expected), "GOCOVERDIR="+t.TempDir())
	if len(args) == 1 && args[0] == markdownCommand {
		process.Env = append(process.Env, markdownDirectory+"="+t.TempDir())
	}
	process.Env = append(process.Env, env...)
	var stdout, stderr bytes.Buffer
	process.Stdout, process.Stderr = &stdout, &stderr
	err = process.Run()
	require.NoError(t, ctx.Err(), "command exceeded subprocess timeout")
	return stdout.String(), stderr.String(), err
}

// CheckHelp covers root help, fatal usage, every command's Markdown, and secret-file controls.
func CheckHelp(t *testing.T, artifact string) {
	t.Helper()
	const marker = "ENV_SECRET_MARKER"
	file := filepath.Join(t.TempDir(), marker+"_FILE")
	require.NoError(t, os.WriteFile(file, []byte(marker+"_CONTENTS"), 0o600)) //nolint:mnd // Private secret fixture.
	env := []string{
		"CARTESI_DATABASE_CONNECTION=postgres://user:" + marker + "@localhost:5432/rollupsdb",
		"CARTESI_BLOCKCHAIN_HTTP_ENDPOINT=https://" + marker + ":password@provider.invalid/" + marker + "?key=" + marker,
		"CARTESI_AUTH_KIND=private_key",
		"CARTESI_AUTH_PRIVATE_KEY=" + marker,
		"CARTESI_AUTH_MNEMONIC=" + marker,
		"CARTESI_AUTH_MNEMONIC_FILE=" + file,
		"CARTESI_PRT_AUTH_PRIVATE_KEY=" + marker,
		"CARTESI_PRT_AUTH_MNEMONIC=" + marker,
		"CARTESI_BLOCKCHAIN_HTTP_AUTHORIZATION=Authorization:Bearer " + marker,
		"CARTESI_AUTH_AWS_KMS_KEY_ID=" + marker,
		"CARTESI_PRT_AUTH_AWS_KMS_KEY_ID=" + marker,
		"AWS_ACCESS_KEY_ID=" + marker,
		"AWS_SECRET_ACCESS_KEY=" + marker,
		"AWS_SESSION_TOKEN=" + marker,
		"PGPASSWORD=" + marker,
		"CARTESI_LOG_COLOR=false",
	}
	type helpCase struct {
		name string
		args []string
		fail bool
	}
	cases := []helpCase{
		{name: "help", args: []string{"--help"}},
		{name: "usage", args: []string{"--unknown-secret-test-flag"}, fail: true},
		{name: markdownCommand, args: []string{markdownCommand}},
	}
	if artifact != "cli" && artifact != "machine-tool" {
		cases = append(cases, helpCase{name: "config error usage", args: []string{"--log-level=invalid-test-level"}, fail: true})
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, err := Run(t, env, test.args...)
			if test.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err, stderr)
			}
			output := stdout + stderr
			require.NotContains(t, output, marker)
			switch test.name {
			case markdownCommand:
				require.Contains(t, output, "cartesi-rollups-"+artifact)
			case "usage":
				require.Contains(t, output, "unknown flag:")
			default:
				require.Contains(t, output, "Usage:")
			}
		})
	}
}
