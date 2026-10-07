// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package withdrawalhelpers

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	flashDriveKind    = "flash_drive"
	nvramKind         = "nvram"
	accountsDriveSize = uint64(1) << 22
	fixtureStartIndex = uint64(768)
	otherStartIndex   = uint64(1024)
	fixtureMode       = os.FileMode(0600)
	executableMode    = os.FileMode(0700)
	commandTimeout    = 30 * time.Second
	rangeError        = "expected one accounts range at the configured address with length at least 4Mi"
)

type memoryRange struct {
	Start  uint64 `json:"start"`
	Length uint64 `json:"length"`
}

func TestAccountsDriveStartIndex_MemoryRanges(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	requireCommands(t, "jq", "bash")
	accounts := memoryRange{Start: fixtureStartIndex * accountsDriveSize, Length: accountsDriveSize}
	larger := memoryRange{Start: accounts.Start, Length: accountsDriveSize * 2}
	other := memoryRange{Start: otherStartIndex * accountsDriveSize, Length: accountsDriveSize}
	misaligned := memoryRange{Start: accounts.Start + 4096, Length: accountsDriveSize}

	tests := []struct {
		name     string
		ranges   map[string][]memoryRange
		override string
		wantErr  string
	}{
		{
			name: "wrong_override",
			ranges: map[string][]memoryRange{
				nvramKind: {accounts},
			},
			override: "769",
			wantErr:  rangeError,
		},
		{
			name:     "valid flash override",
			ranges:   map[string][]memoryRange{flashDriveKind: {accounts}},
			override: "768",
		},
		{
			name:     "valid nvram override",
			ranges:   map[string][]memoryRange{nvramKind: {accounts}},
			override: "768",
		},
		{
			name:     "valid flash override with larger range",
			ranges:   map[string][]memoryRange{flashDriveKind: {larger}},
			override: "768",
		},
		{
			name:     "valid nvram override with larger range",
			ranges:   map[string][]memoryRange{nvramKind: {larger}},
			override: "768",
		},
		{
			name: "override selects nvram among equal size candidates",
			ranges: map[string][]memoryRange{
				flashDriveKind: {other},
				nvramKind:      {accounts},
			},
			override: "768",
		},
		{
			name:   "infer unique flash range",
			ranges: map[string][]memoryRange{flashDriveKind: {accounts}},
		},
		{
			name:   "infer unique nvram range",
			ranges: map[string][]memoryRange{nvramKind: {accounts}},
		},
		{
			name: "ambiguous ranges require override",
			ranges: map[string][]memoryRange{
				flashDriveKind: {other},
				nvramKind:      {accounts},
			},
			wantErr: "expected one accounts range; set ACCOUNTS_DRIVE_START_INDEX",
		},
		{
			name:    "misaligned nvram cannot be inferred",
			ranges:  map[string][]memoryRange{nvramKind: {misaligned}},
			wantErr: rangeError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			cfg, err := json.Marshal(map[string]any{"config": tt.ranges})
			require.NoError(t, err)
			configPath := filepath.Join(workDir, "config.json")
			writeFixture(t, configPath, string(cfg), fixtureMode)

			out, err := runCommand(t, workDir, nil,
				"bash", filepath.Join(root, "scripts/accounts-drive-start-index"), configPath, tt.override)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, out, tt.wantErr)
				return
			}
			require.NoError(t, err, "%s", out)
			require.Equal(t, strconv.FormatUint(fixtureStartIndex, 10), strings.TrimSpace(out))
		})
	}
}

func TestDeploymentRecipe_AccountsRangeValidation(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	requireCommands(t, "make", "jq", "bash", "openssl")

	tests := []struct {
		name      string
		override  string
		wantError bool
	}{
		{name: "valid override reaches deployment", override: "768"},
		{name: "invalid override stops before deployment", override: "769", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			copyRepositoryFile(t, root, workDir, "Makefile")
			copyRepositoryFile(t, root, workDir, "scripts/accounts-drive-start-index")
			writeFixture(t, filepath.Join(workDir, "applications/erc20-withdrawal-dapp/config.json"),
				`{"config":{"nvram":[{"start":3221225472,"length":4194304}]}}`, fixtureMode)
			cliLog := filepath.Join(workDir, "cli-calls")
			writeFixture(t, filepath.Join(workDir, "cartesi-rollups-cli"), `#!/bin/sh
printf 'CALL\n' >> "$WITHDRAWAL_HELPERS_CLI_LOG"
printf '%s\n' "$@" >> "$WITHDRAWAL_HELPERS_CLI_LOG"
`, executableMode)
			// -o uses the supplied config without building a machine fixture.
			out, err := runCommand(t, workDir, map[string]string{
				"ACCOUNTS_DRIVE_START_INDEX": tt.override,
				"APP":                        "test-withdrawal",
				"WITHDRAWAL_HELPERS_CLI_LOG": cliLog,
			}, "make", "-f", "Makefile", "-o", "applications/erc20-withdrawal-dapp", "deploy-erc20-withdrawal-dapp")
			if tt.wantError {
				require.Error(t, err, "invalid drive must stop deployment: %s", out)
				require.Contains(t, out, rangeError)
				requireNotInvoked(t, cliLog)
				return
			}
			require.NoError(t, err, "%s", out)
			calls, err := os.ReadFile(cliLog)
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(string(calls),
				"CALL\ndeploy\napplication\ntest-withdrawal\napplications/erc20-withdrawal-dapp\n"))
			require.Equal(t, 3, strings.Count(string(calls), "CALL\n"), "deploy, snapshot policy, and enable must run")
			var withdrawalConfig struct {
				AccountsDriveStartIndex uint64 `json:"accounts_drive_start_index"`
				Log2MaxNumOfAccounts    uint8  `json:"log2_max_num_of_accounts"`
				Log2LeavesPerAccount    uint8  `json:"log2_leaves_per_account"`
			}
			require.NoError(t, json.Unmarshal([]byte(flagValue(t, string(calls), "--withdrawal-config")), &withdrawalConfig))
			require.Equal(t, fixtureStartIndex, withdrawalConfig.AccountsDriveStartIndex)
			require.Equal(t, uint8(17), withdrawalConfig.Log2MaxNumOfAccounts)
			require.Zero(t, withdrawalConfig.Log2LeavesPerAccount)
		})
	}
}

func TestProveCommand_RegisteredWithdrawalConfig(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	requireCommands(t, "bash", "jq")
	const validApps = `[{"name":"test-withdrawal","iapplication_address":"0x0000000000000000000000000000000000000001",
		"withdrawal_config":{"accounts_drive_start_index":"0x345","log2_max_num_of_accounts":"0xf","log2_leaves_per_account":"0x2"}}]`
	tests := []struct {
		name      string
		apps      string
		wantError bool
	}{
		{name: "nondefault geometry and address from registered config", apps: validApps},
		{
			name:      "missing withdrawal config",
			apps:      `[{"name":"test-withdrawal","iapplication_address":"0x0000000000000000000000000000000000000001"}]`,
			wantError: true,
		},
		{
			name: "missing geometry field",
			apps: `[{"name":"test-withdrawal","iapplication_address":"0x0000000000000000000000000000000000000001",
				"withdrawal_config":{"accounts_drive_start_index":"0x345","log2_max_num_of_accounts":"0xf"}}]`,
			wantError: true,
		},
		{name: "malformed registered config JSON", apps: `[{"name":`, wantError: true},
		{name: "application not registered", apps: `[]`, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workDir := t.TempDir()
			appsPath := filepath.Join(workDir, "applications.json")
			writeFixture(t, appsPath, tt.apps, fixtureMode)
			cli := filepath.Join(workDir, "cli")
			writeFixture(t, cli, `#!/bin/sh
[ "$#" -eq 2 ] && [ "$1" = app ] && [ "$2" = list ] || exit 1
cat "$WITHDRAWAL_HELPERS_APPS"
`, executableMode)
			tool := filepath.Join(workDir, "machine-tool")
			toolLog := filepath.Join(workDir, "tool-args")
			writeFixture(t, tool, `#!/bin/sh
printf '%s\n' "$@" > "$WITHDRAWAL_HELPERS_TOOL_LOG"
printf '{}\n'
`, executableMode)
			stubBin := filepath.Join(workDir, "bin")
			// wallet() checks that cast exists even when WALLET is supplied. Any actual invocation must fail locally.
			writeFixture(t, filepath.Join(stubBin, "cast"), "#!/bin/sh\nexit 1\n", executableMode)
			artifacts := filepath.Join(workDir, "artifacts")
			require.NoError(t, os.MkdirAll(filepath.Join(artifacts, "snapshot-epoch-0"), executableMode))
			dapp := filepath.Join(workDir, "template")
			// This deliberately disagrees with the registered address. Proving must not rediscover it from the template.
			writeFixture(t, filepath.Join(dapp, "config.json"),
				`{"config":{"flash_drive":[{"start":3221225472,"length":4194304}]}}`, fixtureMode)

			out, err := runCommand(t, workDir, map[string]string{
				"APP":                         "test-withdrawal",
				"CLI":                         cli,
				"MACHINE_TOOL":                tool,
				"PATH":                        stubBin + string(os.PathListSeparator) + os.Getenv("PATH"),
				"ARTIFACTS":                   artifacts,
				"DAPP":                        dapp,
				"WALLET":                      "0x0000000000000000000000000000000000000002",
				"WITHDRAWAL_HELPERS_APPS":     appsPath,
				"WITHDRAWAL_HELPERS_TOOL_LOG": toolLog,
			}, "bash", filepath.Join(root, "scripts/withdrawal-lifecycle"), "prove", "0")
			if tt.wantError {
				require.Error(t, err, "invalid config must stop proving: %s", out)
				requireNotInvoked(t, toolLog)
				return
			}
			require.NoError(t, err, "%s", out)
			args, err := os.ReadFile(toolLog)
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(string(args), "prove\naccounts-drive\n"))
			for flag, want := range map[string]uint64{
				"--accounts-drive-start-index": 0x345,
				"--log2-max-num-of-accounts":   15,
				"--log2-leaves-per-account":    2,
			} {
				got, err := strconv.ParseUint(flagValue(t, string(args), flag), 0, 64)
				require.NoError(t, err, "%s", flag)
				require.Equal(t, want, got, "%s must use the registered config", flag)
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	return root
}

func requireCommands(t *testing.T, commands ...string) {
	t.Helper()
	for _, command := range commands {
		_, err := exec.LookPath(command)
		require.NoError(t, err, "%s is required for withdrawal helper regression tests", command)
	}
}

func copyRepositoryFile(t *testing.T, root, destination, relativePath string) {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, relativePath))
	require.NoError(t, err)
	writeFixture(t, filepath.Join(destination, relativePath), string(contents), fixtureMode)
}

func writeFixture(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), executableMode))
	//nolint:gosec // Fixture paths come from t.TempDir and fixed file names.
	require.NoError(t, os.WriteFile(path, []byte(contents), mode))
}

func runCommand(t *testing.T, directory string, environment map[string]string, executable string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = directory
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := environment[key]; replaced || key == "MAKEFLAGS" || key == "MFLAGS" || key == "MAKEOVERRIDES" {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	for key, value := range environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func requireNotInvoked(t *testing.T, invocationLog string) {
	t.Helper()
	_, err := os.Stat(invocationLog)
	require.ErrorIs(t, err, os.ErrNotExist, "invalid input must be rejected before invoking the stub")
}

func flagValue(t *testing.T, arguments, flag string) string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(arguments), "\n")
	for i, line := range lines {
		if line == flag {
			require.Less(t, i+1, len(lines), "missing value for %s", flag)
			return lines[i+1]
		}
	}
	require.FailNow(t, "missing command flag", "%s absent from %s", flag, arguments)
	return ""
}
