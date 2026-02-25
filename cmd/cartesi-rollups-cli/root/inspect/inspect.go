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
	"github.com/spf13/viper"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/inspectclient"
)

var Cmd = &cobra.Command{
	Use:     "inspect  [app-name-or-address] [payload]",
	Short:   "Calls inspect API",
	Args:    cobra.MinimumNArgs(1),
	Example: examples,
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_INSPECT_URL                             Inspect API URL`,
}

const examples = `# Makes a request with "hi":
cartesi-rollups-cli inspect echo-dapp "hi"

# Makes a request with "hi" encoded as hex:
cartesi-rollups-cli inspect echo-dapp 0x6869 --hex

# Reads payload from stdin:
echo -n "hi" | cartesi-rollups-cli inspect echo-dapp`

var isHex bool

func init() {
	Cmd.Flags().BoolVarP(&isHex, "hex", "x", false,
		"Force interpretation of payload as hex.")

	Cmd.Flags().String("inspect-url", "http://localhost:10012/",
		"address used to connect to the inspect API")
	cobra.CheckErr(viper.BindPFlag(config.INSPECT_URL, Cmd.Flags().Lookup("inspect-url")))

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		origHelpFunc(command, strings)
	})
}

func resolvePayload(args []string) ([]byte, error) {
	// If we have exactly one argument (just the app name/address), read from stdin
	if len(args) == 1 {
		stdinBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read from stdin: %w", err)
		}
		if isHex {
			return decodeHex(string(stdinBytes))
		}
		return stdinBytes, nil
	}
	// Otherwise, use the second argument as payload
	if isHex {
		return decodeHex(args[1])
	}
	return []byte(args[1]), nil
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

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	url, err := config.GetInspectUrl()
	cobra.CheckErr(err)

	client, err := inspectclient.NewClient(url)
	cobra.CheckErr(err)

	payload, err := resolvePayload(args)
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
