// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inputs

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "inputs",
	Short:   "Reads inputs ordered by index",
	Example: examples,
	Run:     run,
}

const examples = `# Read inputs from GraphQL:
cartesi-rollups-cli read inputs -n echo-dapp`

var (
	index       uint64
	decodeInput bool
)

func init() {
	Cmd.Flags().Uint64Var(&index, "index", 0, "index of the input")
	Cmd.Flags().BoolVarP(&decodeInput, "decode", "d", false,
		"Prints the decoded input RawData")
}

type EvmAdvance struct {
	ChainId     string `json:"chainId"`
	AppContract string `json:"appContract"`
	MsgSender   string `json:"msgSender"`
	BlockNumber string `json:"blockNumber"`
	BlockTime   string `json:"blockTimestamp"`
	PrevRandao  string `json:"prevRandao"`
	Index       string `json:"index"`
	Payload     string `json:"payload"`
}

func decodeInputData(input *model.Input, parsedAbi *abi.ABI) (EvmAdvance, error) {
	decoded := make(map[string]interface{})
	err := parsedAbi.Methods["EvmAdvance"].Inputs.UnpackIntoMap(decoded, input.RawData[4:])
	if err != nil {
		return EvmAdvance{}, err
	}

	params := EvmAdvance{
		ChainId:     decoded["chainId"].(*big.Int).String(),
		AppContract: decoded["appContract"].(common.Address).Hex(),
		MsgSender:   decoded["msgSender"].(common.Address).Hex(),
		BlockNumber: decoded["blockNumber"].(*big.Int).String(),
		BlockTime:   decoded["blockTimestamp"].(*big.Int).String(),
		PrevRandao:  decoded["prevRandao"].(*big.Int).String(),
		Index:       decoded["index"].(*big.Int).String(),
		Payload:     "0x" + hex.EncodeToString(decoded["payload"].([]byte)),
	}

	return params, nil
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
	if cmd.Flags().Changed("index") {
		input, err := cmdcommon.Repository.GetInput(ctx, nameOrAddress, index)
		cobra.CheckErr(err)

		if decodeInput {
			parsedAbi, err := inputs.InputsMetaData.GetAbi()
			cobra.CheckErr(err)

			params, err := decodeInputData(input, parsedAbi)
			cobra.CheckErr(err)

			result, err = json.MarshalIndent(params, "", "  ")
			cobra.CheckErr(err)
		} else {
			result, err = json.MarshalIndent(input, "", "    ")
			cobra.CheckErr(err)
		}

	} else {
		inputList, err := cmdcommon.Repository.ListInputs(ctx, nameOrAddress, repository.InputFilter{}, repository.Pagination{})
		cobra.CheckErr(err)

		if decodeInput {
			parsedAbi, err := inputs.InputsMetaData.GetAbi()
			cobra.CheckErr(err)

			var decodedInputs []EvmAdvance
			for _, input := range inputList {
				params, err := decodeInputData(input, parsedAbi)
				cobra.CheckErr(err)
				decodedInputs = append(decodedInputs, params)
			}

			result, err = json.MarshalIndent(decodedInputs, "", "  ")
			cobra.CheckErr(err)
		} else {
			result, err = json.MarshalIndent(inputList, "", "    ")
			cobra.CheckErr(err)
		}
	}

	fmt.Println(string(result))
}
