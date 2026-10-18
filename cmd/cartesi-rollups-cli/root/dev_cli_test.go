// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/stretchr/testify/require"
)

func TestDevelopmentCLIPayerAndAuthorityOwner(t *testing.T) {
	const (
		deployCommand      = "deploy"
		templateName       = "template with spaces"
		payloadValue       = "payload with spaces"
		applicationCommand = "application"
		applicationName    = "app"
		ownerFlag          = "--authority-owner"
		authorityCommand   = "authority"
		sendCommand        = "send"
	)
	const claimerAddress = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	const explicitOwner = "0x0000000000000000000000000000000000001234"
	fixture := filepath.Join(t.TempDir(), "cli")
	script := "#!/bin/bash\nset -euo pipefail\nprintf '%s\\n' \"$CARTESI_AUTH_KIND\" \"$CARTESI_AUTH_MNEMONIC\" " +
		"\"$CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX\" \"$@\"\n"
	require.NoError(t, os.WriteFile(fixture, []byte(script), 0o755)) //nolint:gosec // Executable test fixture.
	t.Setenv("CARTESI_DEV_CLI_BINARY", fixture)
	t.Setenv("CARTESI_AUTH_KIND", "private_key")
	t.Setenv("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX", "0")
	t.Setenv("CARTESI_DEV_CLI_ACCOUNT_INDEX", "5")
	t.Setenv("CARTESI_DEV_AUTHORITY_OWNER", claimerAddress)
	wrapper := filepath.Join("..", "..", "..", "scripts", "dev-cli")
	for _, tc := range []struct {
		name       string
		args, want []string
	}{
		{applicationCommand, []string{deployCommand, applicationCommand, applicationName, templateName},
			[]string{deployCommand, applicationCommand, applicationName, templateName, ownerFlag, claimerAddress}},
		{authorityCommand, []string{deployCommand, authorityCommand}, []string{deployCommand, authorityCommand, ownerFlag, claimerAddress}},
		{"explicit owner", []string{deployCommand, applicationCommand, applicationName, ownerFlag, explicitOwner},
			[]string{deployCommand, applicationCommand, applicationName, ownerFlag, explicitOwner}},
		{"owner equals", []string{deployCommand, authorityCommand, "--authority-owner=" + explicitOwner},
			[]string{deployCommand, authorityCommand, "--authority-owner=" + explicitOwner}},
		{"short owner", []string{deployCommand, authorityCommand, "-O", explicitOwner},
			[]string{deployCommand, authorityCommand, "-O", explicitOwner}},
		{sendCommand, []string{sendCommand, applicationName, payloadValue},
			[]string{sendCommand, applicationName, payloadValue}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := os.Environ()
			args := append([]string{wrapper}, tc.args...)
			output, err := exec.CommandContext(t.Context(), "bash", args...).CombinedOutput()
			require.NoError(t, err, "%s", output)
			want := append([]string{"mnemonic", ethutil.FoundryMnemonic, "5"}, tc.want...)
			require.Equal(t, want, strings.Split(strings.TrimSuffix(string(output), "\n"), "\n"))
			require.Equal(t, before, os.Environ())
		})
	}
}
