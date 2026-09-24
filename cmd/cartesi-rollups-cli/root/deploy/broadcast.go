// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/cli"
)

func writeDeploymentBroadcast(cmd *cobra.Command, tx *types.Transaction, predictedAddress common.Address) error {
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	result := struct {
		cli.TransactionResult
		PredictedAddress common.Address `json:"predicted_address"`
	}{TransactionResult: cli.NewTransactionResult(tx, nil), PredictedAddress: predictedAddress}
	if asJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		err = encoder.Encode(result)
	} else {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Transaction broadcast: %s\nPredicted address (deployment not confirmed): %s\n",
			result.TransactionHash, predictedAddress.Hex())
	}
	if err != nil {
		return fmt.Errorf("write deployment broadcast result: %w", err)
	}
	return nil
}
