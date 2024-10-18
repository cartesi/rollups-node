// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/cartesi/rollups-node/pkg/inspectclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "inspect",
	Short:   "Calls inspect API",
	Example: examples,
	Run:     run,
}

const examples = `# Makes a request with "hi":
cartesi-rollups-cli inspect --payload "hi"`

var (
	applicationAddress string
	payload            string
	inspectEndpoint    string
)

func init() {
	Cmd.Flags().StringVarP(
		&applicationAddress,
		"address",
		"a",
		"",
		"Application contract address",
	)
	cobra.CheckErr(Cmd.MarkFlagRequired("address"))

	Cmd.Flags().StringVar(&payload, "payload", "",
		"input payload")
	cobra.CheckErr(Cmd.MarkFlagRequired("payload"))

	Cmd.Flags().StringVar(&inspectEndpoint, "inspect-endpoint", "http://localhost:10000/",
		"address used to connect to the inspect api")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	client, err := inspectclient.NewClient(inspectEndpoint)
	cobra.CheckErr(err)

	encodedPayload := url.QueryEscape(payload)
	requestBody := bytes.NewReader([]byte(encodedPayload))

	response, err := client.InspectPostWithBody(ctx, applicationAddress, "application/octet-stream", requestBody)
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
