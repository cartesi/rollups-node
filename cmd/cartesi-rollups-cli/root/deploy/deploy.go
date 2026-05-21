// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var (
	epochLengthParam          uint64
	claimStagingPeriodParam   uint64
	withdrawalConfigParam     string
	withdrawalConfigFileParam string
	saltParam                 string
	asJSONParam               bool
	verboseParam              bool
)

var Cmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy Contracts and Applications",
	Run:   run,
}

func init() {
	Cmd.PersistentFlags().Uint64VarP(&epochLengthParam, "epoch-length", "", 10, // nolint: mnd
		"Epoch length")
	Cmd.PersistentFlags().MarkHidden("epoch-length")
	Cmd.PersistentFlags().Uint64Var(&claimStagingPeriodParam, "claim-staging-period", 0,
		"Number of blocks between a claim being submitted and accepted (Authority/Quorum only)")
	Cmd.PersistentFlags().StringVar(&withdrawalConfigParam, "withdrawal-config", "",
		"Inline JSON object describing the WithdrawalConfig "+
			"(see docs/withdrawal-config-guide.md). Omit to deploy without foreclosure.")
	Cmd.PersistentFlags().StringVar(&withdrawalConfigFileParam, "withdrawal-config-file", "",
		"Path to a JSON file describing the WithdrawalConfig. Mutually exclusive with --withdrawal-config.")
	Cmd.PersistentFlags().StringVar(&saltParam, "salt", "0000000000000000000000000000000000000000000000000000000000000000",
		"Salt value for contract deployment")
	Cmd.PersistentFlags().MarkHidden("salt")
	Cmd.PersistentFlags().BoolVarP(&asJSONParam, "json", "", false,
		"Print results as JSON")
	Cmd.PersistentFlags().MarkHidden("json")
	Cmd.PersistentFlags().BoolVarP(&verboseParam, "verbose", "", false,
		"Print extra information")
	Cmd.PersistentFlags().MarkHidden("verbose")

	Cmd.AddCommand(applicationCmd)
	Cmd.AddCommand(authorityCmd)
	Cmd.AddCommand(quorumCmd)
}

func run(cmd *cobra.Command, args []string) {
	// If no subcommand is provided, show help
	err := cmd.Help()
	cobra.CheckErr(err)
}

// parse common.Address with error checking
func parseHexAddress(address string) (common.Address, error) {
	if !common.IsHexAddress(address) {
		return common.Address{}, fmt.Errorf("failed to parse hex address")
	}
	return common.HexToAddress(address), nil
}
