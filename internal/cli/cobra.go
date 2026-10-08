// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/cartesi/rollups-node/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func AddFlagBoolVar(flags *pflag.FlagSet, varRef *bool, flagName string, cfgName string, flagDesc string) {
	var value bool
	if declared := declaredFlagDefault(cfgName); declared != "" {
		var err error
		value, err = strconv.ParseBool(declared)
		cobra.CheckErr(err)
	}
	flags.BoolVar(varRef, flagName, value, flagDesc)
	cobra.CheckErr(viper.BindPFlag(cfgName, flags.Lookup(flagName)))
}

func AddFlagUint64Var(flags *pflag.FlagSet, varRef *uint64, flagName string, cfgName string, flagDesc string) {
	var value uint64
	if declared := declaredFlagDefault(cfgName); declared != "" {
		var err error
		value, err = strconv.ParseUint(declared, 10, 64)
		cobra.CheckErr(err)
	}
	flags.Uint64Var(varRef, flagName, value, flagDesc)
	cobra.CheckErr(viper.BindPFlag(cfgName, flags.Lookup(flagName)))
}

func AddFlagStrVar(flags *pflag.FlagSet, varRef *string, flagName string, cfgName string, flagDesc string) {
	flags.StringVar(varRef, flagName, declaredFlagDefault(cfgName), flagDesc)
	cobra.CheckErr(viper.BindPFlag(cfgName, flags.Lookup(flagName)))
}

func AddFlagStrVarP(flags *pflag.FlagSet, varRef *string, flagName string, flagShort string, cfgName string, flagDesc string) {
	flags.StringVarP(varRef, flagName, flagShort, declaredFlagDefault(cfgName), flagDesc)
	cobra.CheckErr(viper.BindPFlag(cfgName, flags.Lookup(flagName)))
}

func declaredFlagDefault(cfgName string) string {
	value, known := config.DeclaredDefault(cfgName)
	if !known {
		panic("unknown configuration key for flag: " + cfgName)
	}
	return value
}

// LogErr logs an error without exiting, so callers can finish deferred cleanup.
func LogErr(logger *slog.Logger, err error, args ...any) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}

	msg := "Error"
	if len(args) > 0 {
		arg0 := args[0]
		args = args[1:]
		switch value := arg0.(type) {
		case string:
			msg = value
		case []byte:
			msg = string(value)
		case fmt.Stringer:
			msg = value.String()
		default:
			msg = fmt.Sprintf("%v", value)
		}
	}

	args = append([]any{"error", err}, args...)
	logger.Error(msg, args...)
}
