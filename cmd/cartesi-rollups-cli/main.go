// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package contains the cartesi-rollups CLI binary.
package main

import (
	"os"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root"
)

func main() {
	err := root.Cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
