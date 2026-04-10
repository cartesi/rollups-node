// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func AddFlagBoolVar(flags *pflag.FlagSet, varRef *bool, flagName string, cfgName string, flagDesc string) {
	flags.BoolVar(varRef, flagName, viper.GetBool(cfgName), flagDesc)
	cobra.CheckErr(viper.BindPFlag(cfgName, flags.Lookup(flagName)))
}

func AddFlagUint64Var(flags *pflag.FlagSet, varRef *uint64, flagName string, cfgName string, flagDesc string) {
	flags.Uint64Var(varRef, flagName, viper.GetUint64(cfgName), flagDesc)
	cobra.CheckErr(viper.BindPFlag(cfgName, flags.Lookup(flagName)))
}

func AddFlagStrVar(flags *pflag.FlagSet, varRef *string, flagName string, cfgName string, flagDesc string) {
	flags.StringVar(varRef, flagName, viper.GetString(cfgName), flagDesc)
	cobra.CheckErr(viper.BindPFlag(cfgName, flags.Lookup(flagName)))
}

func AddFlagStrVarP(flags *pflag.FlagSet, varRef *string, flagName string, flagShort string, cfgName string, flagDesc string) {
	flags.StringVarP(varRef, flagName, flagShort, viper.GetString(cfgName), flagDesc)
	cobra.CheckErr(viper.BindPFlag(cfgName, flags.Lookup(flagName)))
}
