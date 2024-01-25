// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cartesi/rollups-node/pkg/inspectclient"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "inspect",
	Short:   "Calls inspect API",
	Example: examples,
	Run:     run,
}

const examples = `# Makes a request with "hi" encoded as hex:
cartesi-rollups-cli inspect --payload 0x$(printf "hi" | xxd -p)`

var (
	hexPayload      string
	inspectEndpoint string
)

func init() {
	Cmd.Flags().StringVar(&hexPayload, "payload", "",
		"input payload hex-encoded starting with 0x")

	cobra.CheckErr(Cmd.MarkFlagRequired("payload"))

	Cmd.Flags().StringVar(&inspectEndpoint, "inspect-endpoint", "http://localhost:10000/",
		"address used to connect to the inspect api")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	client, err := inspectclient.NewClient(inspectEndpoint)
	cobra.CheckErr(err)

	payload, err := hexutil.Decode(hexPayload)
	cobra.CheckErr(err)

	requestBody := bytes.NewReader(payload)

	response, err := client.InspectPostWithBody(ctx, "application/octet-stream", requestBody)
	cobra.CheckErr(err)

	respBytes, err := io.ReadAll(response.Body)
	cobra.CheckErr(err)

	var prettyJSON bytes.Buffer
	cobra.CheckErr(json.Indent(&prettyJSON, []byte(respBytes), "", "    "))

	fmt.Print(prettyJSON.String())
}
