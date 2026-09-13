// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/cli"
)

const (
	prtFlag      = "--prt"
	managerFlag  = "--sentry-manager"
	sentriesFlag = "--sentries"
	requiresPRT  = "require --prt"
)

func TestApplicationDeploymentSentryFlags(t *testing.T) {
	manager := common.HexToAddress("0x10").Hex()
	sentryOne := common.HexToAddress("0xab").Hex()
	sentryTwo := common.HexToAddress("0xcd").Hex()
	for _, name := range []string{"sentry-manager", "sentries"} {
		require.NotNil(t, applicationCmd.Flags().Lookup(name), "the deployed command must expose %s", name)
	}
	for _, test := range []struct {
		name      string
		flags     []string
		manager   common.Address
		sentries  []common.Address
		wantError string
	}{
		{name: "non PRT defaults"},
		{name: "PRT defaults", flags: []string{prtFlag}},
		{name: "manager only", flags: []string{prtFlag, managerFlag, manager}, manager: common.HexToAddress(manager)},
		{name: "fixed sentries", flags: []string{prtFlag, sentriesFlag, sentryOne},
			sentries: []common.Address{common.HexToAddress(sentryOne)}},
		{name: "explicit zero manager", flags: []string{prtFlag, managerFlag, common.Address{}.Hex(), sentriesFlag, sentryOne},
			sentries: []common.Address{common.HexToAddress(sentryOne)}},
		{name: "ordered list", flags: []string{prtFlag, managerFlag, manager, sentriesFlag, sentryTwo + "," + sentryOne},
			manager:  common.HexToAddress(manager),
			sentries: []common.Address{common.HexToAddress(sentryTwo), common.HexToAddress(sentryOne)}},
		{name: "repeated flag", flags: []string{prtFlag, sentriesFlag, sentryTwo, sentriesFlag, sentryOne},
			sentries: []common.Address{common.HexToAddress(sentryTwo), common.HexToAddress(sentryOne)}},
		{name: "empty list", flags: []string{prtFlag, "--sentries="}},
		{name: "manager without PRT", flags: []string{managerFlag, manager}, wantError: requiresPRT},
		{name: "sentries without PRT", flags: []string{sentriesFlag, sentryOne}, wantError: requiresPRT},
		{name: "explicit empty without PRT", flags: []string{"--sentries="}, wantError: requiresPRT},
		{name: "malformed manager", flags: []string{prtFlag, managerFlag, "0x01"}, wantError: "invalid --sentry-manager"},
		{name: "malformed sentry", flags: []string{prtFlag, sentriesFlag, "not-an-address"}, wantError: "invalid --sentries entry 1"},
		{name: "zero sentry", flags: []string{prtFlag, sentriesFlag, common.Address{}.Hex()}, wantError: "must not be the zero address"},
		{name: "duplicate sentry", flags: []string{prtFlag, sentriesFlag, sentryOne, sentriesFlag, strings.ToLower(sentryOne)},
			wantError: "duplicates address"},
		{name: "empty entry", flags: []string{prtFlag, sentriesFlag, sentryOne + ","}, wantError: "invalid --sentries entry 2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := newSentryDeploymentTestCommand(t, test.flags)
			// Exercise the command's real pre-run hook, not only the parser.
			err := applicationCmd.PreRunE(cmd, nil)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			gotManager, gotSentries, err := parsePRTSentryConfig(cmd)
			require.NoError(t, err)
			require.Equal(t, test.manager, gotManager)
			require.Equal(t, append([]common.Address{}, test.sentries...), gotSentries)
		})
	}
}

func newSentryDeploymentTestCommand(t *testing.T, args []string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().Bool("prt", false, "")
	cmd.Flags().Bool("register", false, "")
	cmd.Flags().String("template-hash", "", "")
	addPRTSentryFlags(cmd)
	cli.AddTransactionFlags(cmd)
	require.NoError(t, cmd.Flags().Set("template-hash", common.HexToHash("0x01").Hex()))
	require.NoError(t, cmd.ParseFlags(args))
	return cmd
}

func TestBuildPRTDeploymentPreservesSentryConfig(t *testing.T) {
	t.Setenv("CARTESI_CONTRACTS_DAVE_APP_FACTORY_ADDRESS", common.HexToAddress("0x100").Hex())
	previousHash := applicationTemplateHashParam
	t.Cleanup(func() { applicationTemplateHashParam = previousHash })
	applicationTemplateHashParam = common.HexToHash("0x01").Hex()
	manager := common.HexToAddress("0x10")
	sentryOne, sentryTwo := common.HexToAddress("0x30"), common.HexToAddress("0x20")
	cmd := newSentryDeploymentTestCommand(t, []string{
		prtFlag, managerFlag, manager.Hex(), sentriesFlag, sentryOne.Hex() + "," + sentryTwo.Hex(),
	})
	request, err := buildPrtApplicationDeployment(cmd, nil)
	require.NoError(t, err)
	require.Equal(t, manager, request.SentryManager)
	require.Equal(t, []common.Address{sentryOne, sentryTwo}, request.Sentries)
}
