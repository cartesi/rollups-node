// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/pkg/ethutil"
)

func addPRTSentryFlags(cmd *cobra.Command) {
	cmd.Flags().String("sentry-manager", common.Address{}.Hex(),
		"PRT only: immutable sentry manager address. Zero disables address rotation, not sentry claims.")
	cmd.Flags().StringSlice("sentries", nil,
		"PRT only: initial sentry addresses in slot order (IDs start at 1). Repeat or comma-separate. Slot count cannot change.")
}

func parsePRTSentryConfig(cmd *cobra.Command) (common.Address, []common.Address, error) {
	manager := common.Address{}
	sentries := []common.Address{}
	if !cmd.Flags().Changed("sentry-manager") && !cmd.Flags().Changed("sentries") {
		return manager, sentries, nil
	}
	prt, err := cmd.Flags().GetBool("prt")
	if err != nil {
		return manager, nil, err
	}
	if !prt {
		return manager, nil, fmt.Errorf("--sentry-manager and --sentries require --prt")
	}
	managerText, err := cmd.Flags().GetString("sentry-manager")
	if err != nil {
		return manager, nil, err
	}
	manager, err = parseHexAddress(managerText)
	if err != nil {
		return manager, nil, fmt.Errorf("invalid --sentry-manager: %w", err)
	}
	addresses, err := cmd.Flags().GetStringSlice("sentries")
	if err != nil {
		return manager, nil, err
	}
	for i, text := range addresses {
		address, err := parseHexAddress(text)
		if err != nil {
			return manager, nil, fmt.Errorf("invalid --sentries entry %d: %w", i+1, err)
		}
		sentries = append(sentries, address)
	}
	if err := ethutil.ValidateSentryAddresses(sentries); err != nil {
		return manager, nil, fmt.Errorf("invalid --sentries: %w", err)
	}
	return manager, sentries, nil
}
