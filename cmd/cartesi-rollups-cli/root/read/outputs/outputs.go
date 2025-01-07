// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package outputs

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"
)

var Cmd = &cobra.Command{
	Use:     "outputs",
	Short:   "Reads outputs. If an input index is specified, reads all outputs from that input",
	Example: examples,
	Run:     run,
}

const examples = `# Read all notices:
cartesi-rollups-cli read outputs -n echo-dapp`

var (
	outputIndex  uint64
	inputIndex   uint64
	decodeOutput bool
)

func init() {
	Cmd.Flags().Uint64Var(&inputIndex, "input-index", 0,
		"filter by input index")

	Cmd.Flags().Uint64Var(&outputIndex, "output-index", 0,
		"filter by output index")

	Cmd.Flags().BoolVarP(&decodeOutput, "decode", "d", false,
		"Prints the decoded Output RawData")
}

type Notice struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type Voucher struct {
	Type        string `json:"type"`
	Destination string `json:"destination"`
	Value       string `json:"value"`
	Payload     string `json:"payload"`
}

type DelegateCallVoucher struct {
	Type        string `json:"type"`
	Destination string `json:"destination"`
	Payload     string `json:"payload"`
}

func decodeOutputData(output *model.Output, parsedAbi *abi.ABI) (interface{}, error) {
	if len(output.RawData) < 4 {
		return nil, fmt.Errorf("raw data too short")
	}

	method, err := parsedAbi.MethodById(output.RawData[:4])
	if err != nil {
		return nil, err
	}

	decoded := make(map[string]interface{})
	if err := method.Inputs.UnpackIntoMap(decoded, output.RawData[4:]); err != nil {
		return nil, fmt.Errorf("failed to unpack %s: %w", method.Name, err)
	}

	switch method.Name {
	case "Notice":
		payload, ok := decoded["payload"].([]byte)
		if !ok {
			return nil, fmt.Errorf("unable to decode Notice payload")
		}
		return Notice{
			Type:    "Notice",
			Payload: "0x" + hex.EncodeToString(payload),
		}, nil

	case "Voucher":
		dest, ok1 := decoded["destination"].(common.Address)
		value, ok2 := decoded["value"].(*big.Int)
		payload, ok3 := decoded["payload"].([]byte)
		if !ok1 || !ok2 || !ok3 {
			return nil, fmt.Errorf("unable to decode Voucher parameters")
		}
		return Voucher{
			Type:        "Voucher",
			Destination: dest.Hex(),
			Value:       value.String(),
			Payload:     "0x" + hex.EncodeToString(payload),
		}, nil

	case "DelegateCallVoucher":
		dest, ok1 := decoded["destination"].(common.Address)
		payload, ok2 := decoded["payload"].([]byte)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("unable to decode DelegateCallVoucher parameters")
		}
		return DelegateCallVoucher{
			Type:        "DelegateCallVoucher",
			Destination: dest.Hex(),
			Payload:     "0x" + hex.EncodeToString(payload),
		}, nil

	default:
		return map[string]string{
			"type":    method.Name,
			"rawData": "0x" + hex.EncodeToString(output.RawData),
		}, nil
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommon.Repository == nil {
		panic("Repository was not initialized")
	}

	var nameOrAddress string
	pFlags := cmd.Flags()
	if pFlags.Changed("name") {
		nameOrAddress = pFlags.Lookup("name").Value.String()
	} else if pFlags.Changed("address") {
		nameOrAddress = pFlags.Lookup("address").Value.String()
	}

	var result []byte
	if cmd.Flags().Changed("output-index") {
		if cmd.Flags().Changed("input-index") {
			fmt.Fprintf(os.Stderr, "Error: Only one of 'output-index' or 'input-index' can be used at a time.\n")
			os.Exit(1)
		}
		output, err := cmdcommon.Repository.GetOutput(ctx, nameOrAddress, outputIndex)
		cobra.CheckErr(err)
		if decodeOutput {
			parsedAbi, err := outputs.OutputsMetaData.GetAbi()
			cobra.CheckErr(err)

			decoded, err := decodeOutputData(output, parsedAbi)
			cobra.CheckErr(err)

			result, err = json.MarshalIndent(decoded, "", "    ")
			cobra.CheckErr(err)
		} else {
			result, err = json.MarshalIndent(output, "", "    ")
			cobra.CheckErr(err)
		}
	} else {
		var outputList []*model.Output
		var err error
		if cmd.Flags().Changed("input-index") {
			f := repository.OutputFilter{InputIndex: &inputIndex}
			p := repository.Pagination{}
			outputList, err = cmdcommon.Repository.ListOutputs(ctx, nameOrAddress, f, p)
			cobra.CheckErr(err)
		} else {
			f := repository.OutputFilter{}
			p := repository.Pagination{}
			outputList, err = cmdcommon.Repository.ListOutputs(ctx, nameOrAddress, f, p)
			cobra.CheckErr(err)
		}
		if decodeOutput {
			parsedAbi, err := outputs.OutputsMetaData.GetAbi()
			cobra.CheckErr(err)

			var decodedOutputs []interface{}
			for _, output := range outputList {
				decoded, err := decodeOutputData(output, parsedAbi)
				if err != nil {
					// If decoding fails, include the error with the raw data.
					decoded = map[string]string{
						"type":    "unknown",
						"rawData": "0x" + hex.EncodeToString(output.RawData),
						"error":   err.Error(),
					}
				}
				decodedOutputs = append(decodedOutputs, decoded)
			}

			// Marshal the decoded outputs as indented JSON.
			result, err = json.MarshalIndent(decodedOutputs, "", "  ")
			cobra.CheckErr(err)

		} else {
			result, err = json.MarshalIndent(outputList, "", "    ")
			cobra.CheckErr(err)
		}
	}

	fmt.Println(string(result))
}
