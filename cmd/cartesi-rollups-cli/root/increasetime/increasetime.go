// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package increasetime

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "increase-time",
	Short:   "Increases evm time of the current machine",
	Example: examples,
	Run:     run,
}

const examples = `# Increases evm time by one day (86400 seconds):
cartesi-rollups-cli increase-time`

const defaultTime = 86400

var (
	time          int
	anvilEndpoint string
)

func init() {
	Cmd.Flags().IntVar(&time, "time", defaultTime,
		"The amount of time to increase in the evm, in seconds")

	Cmd.Flags().StringVar(&anvilEndpoint, "anvil-endpoint", "http://localhost:8545",
		"anvil address used to send to the request")
}

func run(cmd *cobra.Command, args []string) {
	client := &http.Client{}
	var data = strings.NewReader(`{
		"id":1337,
		"jsonrpc":"2.0",
		"method":"evm_increaseTime",
		"params":[` + strconv.Itoa(time) + `]
	}`)

	req, err := http.NewRequest("POST", anvilEndpoint, data)
	cobra.CheckErr(err)

	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	cobra.CheckErr(err)

	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	cobra.CheckErr(err)

	fmt.Printf("%s\n", bodyText)
}
