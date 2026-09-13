// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestBuildSelfhostedDeploymentRejectsApplicationOwner(t *testing.T) {
	t.Setenv("CARTESI_CONTRACTS_SELF_HOSTED_APPLICATION_FACTORY_ADDRESS", "0x1000000000000000000000000000000000000001")
	for _, ownerProvided := range []bool{false, true} {
		name := "omitted"
		if ownerProvided {
			name = "explicit"
		}
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("application-owner", "", "")
			if ownerProvided {
				require.NoError(t, cmd.Flags().Set("application-owner", "0x2000000000000000000000000000000000000002"))
			}
			_, err := buildSelfhostedApplicationDeployment(t.Context(), cmd, nil, nil, &bind.TransactOpts{})
			if ownerProvided {
				require.ErrorContains(t, err, "ownership is renounced")
			} else {
				// Without an owner flag, validation reaches the next required argument.
				require.ErrorContains(t, err, "template-hash")
			}
		})
	}
}

func TestApplicationCommandDoesNotExposeDataAvailabilityFlag(t *testing.T) {
	require.Nil(t, applicationCmd.Flags().Lookup("data-availability"))
}

func TestApplicationDeploymentValidatesBeforeRunning(t *testing.T) {
	for _, test := range []struct {
		name         string
		register     bool
		noWait       bool
		templateHash bool
		args         []string
		wantError    string
	}{
		{name: "registration requires receipt", register: true, noWait: true, wantError: "--no-wait requires --register=false"},
		{name: "registration requires name", register: true, templateHash: true, wantError: "missing application name"},
		{name: "template required", wantError: "missing template"},
		{name: "template hash only", templateHash: true},
		{name: "no wait with template hash", noWait: true, templateHash: true},
		{name: "registered template hash", register: true, templateHash: true, args: []string{"example"}},
		{name: "positional template", register: true, args: []string{"example", "applications/example"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Bool("register", test.register, "")
			cmd.Flags().String("template-hash", "", "")
			cli.AddTransactionFlags(cmd)
			if test.noWait {
				require.NoError(t, cmd.Flags().Set("no-wait", "true"))
			}
			if test.templateHash {
				require.NoError(t, cmd.Flags().Set("template-hash", "0x01"))
			}
			// Invoke the actual command hook. The test must fail if the argument
			// validation is removed from the command's pre-run path.
			err := applicationCmd.PreRunE(cmd, test.args)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
