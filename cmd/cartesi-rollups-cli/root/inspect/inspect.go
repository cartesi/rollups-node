// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/pkg/inspectclient"
)

var Cmd = &cobra.Command{
	Use:     "inspect",
	Short:   "Calls inspect API",
	Example: examples,
	Run:     run,
}

const examples = `# Makes a request with "hi":
cartesi-rollups-cli inspect -n echo-dapp --payload "hi"

# Makes a request with "hi" encoded as hex:
cartesi-rollups-cli inspect -n echo-dapp --payload 0x6869 --hex

# Reads payload from stdin:
echo -n "hi" | cartesi-rollups-cli inspect -n echo-dapp`

var (
	name            string
	address         string
	cmdPayload      string
	isHex           bool
	inspectEndpoint string
)

func init() {
	Cmd.Flags().StringVarP(
		&name,
		"name",
		"n",
		"",
		"Application name",
	)

	Cmd.Flags().StringVarP(
		&address,
		"address",
		"a",
		"",
		"Application contract address",
	)

	Cmd.Flags().StringVar(&cmdPayload, "payload", "",
		"input payload")

	Cmd.Flags().BoolVarP(&isHex, "hex", "x", false,
		"Force interpretation of --payload as hex.")

	Cmd.Flags().StringVar(&inspectEndpoint, "inspect-endpoint", "http://localhost:10012/",
		"address used to connect to the inspect api")

	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if name == "" && address == "" {
			return fmt.Errorf("either 'name' or 'address' must be specified")
		}
		if name != "" && address != "" {
			return fmt.Errorf("only one of 'name' or 'address' can be specified")
		}
		return nil
	}
}

func resolvePayload(cmd *cobra.Command) ([]byte, error) {
	if !cmd.Flags().Changed("payload") {
		stdinBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read from stdin: %w", err)
		}
		return stdinBytes, nil
	}

	if isHex {
		return decodeHex(cmdPayload)
	}

	return []byte(cmdPayload), nil
}

func decodeHex(s string) ([]byte, error) {
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		s = "0x" + s
	}

	b, err := hexutil.Decode(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hex payload %q: %w", s, err)
	}
	return b, nil
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	var nameOrAddress string
	if cmd.Flags().Changed("name") {
		nameOrAddress = name
	} else if cmd.Flags().Changed("address") {
		nameOrAddress = address
	}

	client, err := inspectclient.NewClient(inspectEndpoint)
	cobra.CheckErr(err)

	payload, err := resolvePayload(cmd)
	cobra.CheckErr(err)
	requestBody := bytes.NewReader(payload)

	response, err := client.InspectPostWithBody(ctx, nameOrAddress, "application/octet-stream", requestBody)
	cobra.CheckErr(err)
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(response.Body)
		cobra.CheckErr(fmt.Errorf("HTTP request failed with status %d: %s", response.StatusCode, string(bodyBytes)))
	}

	respBytes, err := io.ReadAll(response.Body)
	cobra.CheckErr(err)

	var prettyJSON bytes.Buffer
	cobra.CheckErr(json.Indent(&prettyJSON, []byte(respBytes), "", "    "))

	fmt.Print(prettyJSON.String())
}
