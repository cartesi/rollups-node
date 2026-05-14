// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/common"
)

// withdrawalConfigJSON is the on-the-wire schema. All five fields are
// required when the user supplies any of them — partial configs are always a
// misconfiguration (see docs/withdrawal-config-guide.md §6). Pointers let us
// distinguish "missing" from "zero".
type withdrawalConfigJSON struct {
	Guardian                *string `json:"guardian"`
	Log2LeavesPerAccount    *uint8  `json:"log2_leaves_per_account"`
	Log2MaxNumOfAccounts    *uint8  `json:"log2_max_num_of_accounts"`
	AccountsDriveStartIndex *uint64 `json:"accounts_drive_start_index"`
	WithdrawalOutputBuilder *string `json:"withdrawal_output_builder"`
}

// parseWithdrawalConfig reads the JSON object from one of the two sources
// (inline string or file path). Exactly one source must be non-empty; the
// caller is responsible for enforcing mutual exclusion via
// MarkFlagsMutuallyExclusive. When both are empty, returns the zero
// (no-foreclosure) config.
func parseWithdrawalConfig(inline, filePath string) (iapplicationfactory.WithdrawalConfig, error) {
	var raw []byte
	var src string
	switch {
	case inline != "" && filePath != "":
		return iapplicationfactory.WithdrawalConfig{}, fmt.Errorf(
			"withdrawal config: --withdrawal-config and --withdrawal-config-file are mutually exclusive")
	case inline != "":
		raw = []byte(inline)
		src = "--withdrawal-config"
	case filePath != "":
		b, err := os.ReadFile(filePath)
		if err != nil {
			return iapplicationfactory.WithdrawalConfig{}, fmt.Errorf(
				"withdrawal config: failed to read %s: %w", filePath, err)
		}
		raw = b
		src = filePath
	default:
		return iapplicationfactory.WithdrawalConfig{}, nil
	}

	var aux withdrawalConfigJSON
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux); err != nil {
		return iapplicationfactory.WithdrawalConfig{}, fmt.Errorf(
			"withdrawal config (%s): invalid JSON: %w", src, err)
	}

	missing := aux.missingKeys()
	if len(missing) > 0 {
		return iapplicationfactory.WithdrawalConfig{}, fmt.Errorf(
			"withdrawal config (%s): missing required keys: %s",
			src, strings.Join(missing, ", "))
	}

	guardian, err := parseHexAddress(*aux.Guardian)
	if err != nil {
		return iapplicationfactory.WithdrawalConfig{}, fmt.Errorf(
			"withdrawal config (%s): invalid guardian address %q: %w",
			src, *aux.Guardian, err)
	}
	builder, err := parseHexAddress(*aux.WithdrawalOutputBuilder)
	if err != nil {
		return iapplicationfactory.WithdrawalConfig{}, fmt.Errorf(
			"withdrawal config (%s): invalid withdrawal_output_builder address %q: %w",
			src, *aux.WithdrawalOutputBuilder, err)
	}

	wc := iapplicationfactory.WithdrawalConfig{
		Guardian:                guardian,
		Log2LeavesPerAccount:    *aux.Log2LeavesPerAccount,
		Log2MaxNumOfAccounts:    *aux.Log2MaxNumOfAccounts,
		AccountsDriveStartIndex: *aux.AccountsDriveStartIndex,
		WithdrawalOutputBuilder: builder,
	}

	if err := ethutil.ValidateWithdrawalConfig(wc); err != nil {
		return iapplicationfactory.WithdrawalConfig{}, fmt.Errorf(
			"withdrawal config (%s): %w", src, err)
	}

	if guardian == (common.Address{}) {
		return iapplicationfactory.WithdrawalConfig{}, fmt.Errorf(
			"withdrawal config (%s): guardian address must not be the zero address "+
				"(omit the flag entirely to deploy without foreclosure)", src)
	}
	if builder == (common.Address{}) {
		return iapplicationfactory.WithdrawalConfig{}, fmt.Errorf(
			"withdrawal config (%s): withdrawal_output_builder address must not be the zero address",
			src)
	}

	return wc, nil
}

func (w *withdrawalConfigJSON) missingKeys() []string {
	var missing []string
	if w.Guardian == nil {
		missing = append(missing, "guardian")
	}
	if w.Log2LeavesPerAccount == nil {
		missing = append(missing, "log2_leaves_per_account")
	}
	if w.Log2MaxNumOfAccounts == nil {
		missing = append(missing, "log2_max_num_of_accounts")
	}
	if w.AccountsDriveStartIndex == nil {
		missing = append(missing, "accounts_drive_start_index")
	}
	if w.WithdrawalOutputBuilder == nil {
		missing = append(missing, "withdrawal_output_builder")
	}
	return missing
}
