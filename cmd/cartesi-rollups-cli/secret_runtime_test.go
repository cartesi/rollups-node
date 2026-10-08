// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/test/tooling/command"
)

const secretTestAddress = "0x0000000000000000000000000000000000000001"
const secretContractCommand = "contract"
const secretSummaryCommand = "summary"

func TestSecretFileMistakeCLI(t *testing.T) {
	const marker = "SECRET_MISTAKEN_FOR_FILE_PATH"
	stdout, stderr, err := command.Run(t,
		[]string{config.BLOCKCHAIN_HTTP_ENDPOINT_FILE + "=" + marker},
		secretContractCommand, secretSummaryCommand, secretTestAddress)
	require.Error(t, err)
	require.NotContains(t, stdout+stderr, marker)
	require.Contains(t, stdout+stderr, config.BLOCKCHAIN_HTTP_ENDPOINT_FILE)
	require.Contains(t, stdout+stderr, "no such file")
}
