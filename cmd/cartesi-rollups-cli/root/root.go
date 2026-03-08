// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/contract"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/db"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/deploy"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/execute"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/inspect"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/send"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/validate"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/version"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Cmd = &cobra.Command{
	Use:     "cartesi-rollups-cli",
	Short:   "Command line interface for the Cartesi Rollups Node",
	Version: version.BuildVersion,
}

var (
	verbose            bool
	databaseConnection string
	blockchainEndpoint string
	inputBoxAddress    string
)

func init() {
	config.SetDefaults()

	// Common persistent flags that will be available to all commands
	Cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	cobra.CheckErr(Cmd.PersistentFlags().MarkHidden("verbose"))

	// Database connection flag
	Cmd.PersistentFlags().StringVar(&databaseConnection, "database-connection", "",
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database')")
	cobra.CheckErr(viper.BindPFlag(config.DATABASE_CONNECTION, Cmd.PersistentFlags().Lookup("database-connection")))
	cobra.CheckErr(Cmd.PersistentFlags().MarkHidden("database-connection"))

	// Blockchain endpoint flag
	Cmd.PersistentFlags().StringVar(&blockchainEndpoint, "blockchain-http-endpoint", "",
		"Blockchain HTTP endpoint")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_HTTP_ENDPOINT, Cmd.PersistentFlags().Lookup("blockchain-http-endpoint")))
	cobra.CheckErr(Cmd.PersistentFlags().MarkHidden("blockchain-http-endpoint"))

	// Input box address flag
	Cmd.PersistentFlags().StringVar(&inputBoxAddress, "inputbox", "",
		"Input Box contract address")
	cobra.CheckErr(viper.BindPFlag(config.CONTRACTS_INPUT_BOX_ADDRESS, Cmd.PersistentFlags().Lookup("inputbox")))
	cobra.CheckErr(Cmd.PersistentFlags().MarkHidden("inputbox"))

	Cmd.AddCommand(send.Cmd)
	Cmd.AddCommand(read.Cmd)
	Cmd.AddCommand(inspect.Cmd)
	Cmd.AddCommand(validate.Cmd)
	Cmd.AddCommand(execute.Cmd)
	Cmd.AddCommand(app.Cmd)
	Cmd.AddCommand(db.Cmd)
	Cmd.AddCommand(deploy.Cmd)
	Cmd.AddCommand(contract.Cmd)
	Cmd.DisableAutoGenTag = true
}
