// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

// These tests run without endtoendtests. The fake CLI checks the actual
// subprocess arguments and environment without starting a node or using L1.
func TestIntegrationCLI(t *testing.T) {
	t.Run("signers", func(t *testing.T) {
		installTestCLI(t, `printf '%s\n' "$CARTESI_AUTH_KIND" "$CARTESI_AUTH_MNEMONIC" \
"$CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX" "$CARTESI_PRT_AUTH_MNEMONIC_ACCOUNT_INDEX" \
"$CARTESI_DATABASE_CONNECTION" "$@"`)
		const parentDatabase = "parent-database"
		t.Setenv("CARTESI_AUTH_KIND", "private_key")
		t.Setenv("CARTESI_AUTH_MNEMONIC", "parent mnemonic must not be used")
		t.Setenv("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX", "0")
		t.Setenv("CARTESI_PRT_AUTH_MNEMONIC_ACCOUNT_INDEX", "6")
		t.Setenv("CARTESI_DATABASE_CONNECTION", parentDatabase)
		for _, tc := range []struct {
			name, index, database string
			overrides             []string
		}{
			{name: "default payer", index: "5", database: parentDatabase},
			{name: "unrelated override", index: "5", database: "unavailable", overrides: []string{
				"CARTESI_DATABASE_CONNECTION=unavailable",
			}},
			{name: "guardian", index: "1", database: parentDatabase, overrides: []string{
				"CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=1",
			}},
			{name: "depositor", index: "8", database: parentDatabase, overrides: []string{
				"CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=8",
			}},
			{name: "refund payer", index: "9", database: parentDatabase, overrides: []string{
				"CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=9",
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				before := os.Environ()
				var out string
				var err error
				if tc.overrides == nil {
					out, err = runCLI(t.Context(), "send", "app", "payload with spaces")
				} else {
					out, err = runCLIWithEnv(t.Context(), tc.overrides, "send", "app", "payload with spaces")
				}
				require.NoError(t, err)
				require.Equal(t, []string{"mnemonic", ethutil.FoundryMnemonic, tc.index, "6", tc.database,
					"send", "app", "payload with spaces"}, strings.Split(strings.TrimSpace(out), "\n"))
				require.Equal(t, before, os.Environ(), "CLI execution must not change the node environment")
			})
		}
	})

	t.Run("deployment", func(t *testing.T) {
		installTestCLI(t, `printf '%s\n' "$CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX" "$@" > "$CARTESI_TEST_CLI_CAPTURE"
printf '%s\n' "$CARTESI_TEST_CLI_RESULT"`)
		capture := filepath.Join(t.TempDir(), "arguments")
		t.Setenv("CARTESI_TEST_CLI_CAPTURE", capture)
		t.Setenv("CARTESI_TEST_CLI_RESULT", `{"iapplication_address":"application","iconsensus_address":"consensus"}`)
		const claimerAddress = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
		const ownerOverride = "0x0000000000000000000000000000000000001234"
		for _, tc := range []struct {
			name, owner string
			args        []string
		}{
			{name: "authority", owner: claimerAddress},
			{name: "prt", owner: claimerAddress, args: []string{prtFlag}},
			{name: "external consensus", owner: claimerAddress, args: []string{"--consensus", "external"}},
			{name: "explicit owner", owner: ownerOverride, args: []string{"--authority-owner", ownerOverride}},
			{name: "owner shorthand", owner: ownerOverride, args: []string{"-O", ownerOverride}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				for _, withConsensus := range []bool{false, true} {
					var address string
					var err error
					if withConsensus {
						var consensus string
						address, consensus, err = deployApplicationWithConsensus(t.Context(), "app", "template", tc.args...)
						require.NoError(t, err)
						require.Equal(t, "consensus", consensus)
					} else {
						address, err = deployApplication(t.Context(), "app", "template", tc.args...)
						require.NoError(t, err)
					}
					require.Equal(t, "application", address)
					captured, err := os.ReadFile(capture)
					require.NoError(t, err)
					args := strings.Split(strings.TrimSpace(string(captured)), "\n")
					require.Equal(t, []string{"5", deployCommand, "application"}, args[:3])
					flags := pflag.NewFlagSet("application", pflag.ContinueOnError)
					owner := flags.StringP("authority-owner", "O", "", "")
					flags.Bool("json", false, "")
					flags.Bool("prt", false, "")
					flags.String("consensus", "", "")
					require.NoError(t, flags.Parse(args[3:]))
					require.Equal(t, []string{"app", "template"}, flags.Args())
					require.Equal(t, tc.owner, *owner)
				}
			})
		}
		for _, result := range []string{"{", `{}`, `{"iapplication_address":"application"}`} {
			t.Setenv("CARTESI_TEST_CLI_RESULT", result)
			_, _, err := deployApplicationWithConsensus(t.Context(), "app", "template")
			require.Error(t, err, "incomplete deployment output must fail")
		}
	})
}

func installTestCLI(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	// The fixture contains no secrets and must be executable by the subprocess.
	err := os.WriteFile(filepath.Join(dir, cliBinary), []byte("#!/bin/sh\nset -eu\n"+script+"\n"), 0o755) //nolint:gosec
	require.NoError(t, err)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
