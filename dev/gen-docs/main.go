// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This binary generates part of the node documentation automatically.

//go:generate go run .
package main

import (
	"log"
	"os"

	advancer_root "github.com/cartesi/rollups-node/cmd/cartesi-rollups-advancer/root"
	cli_root "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root"
	evmreader_root "github.com/cartesi/rollups-node/cmd/cartesi-rollups-evm-reader/root"
	validator_root "github.com/cartesi/rollups-node/cmd/cartesi-rollups-validator/root"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	generateDocs("cli", cli_root.Cmd)
	generateDocs("evm-reader", evmreader_root.Cmd)
	generateDocs("advancer", advancer_root.Cmd)
	generateDocs("validator", validator_root.Cmd)
}

func generateDocs(suffix string, cmd *cobra.Command) {
	dir := "docs/" + suffix
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("failed to create directory %s: %v", dir, err)
	}
	err := doc.GenMarkdownTree(cmd, dir)
	if err != nil {
		log.Fatalf("failed to gen %s docs: %v", suffix, err)
	}
	log.Printf("generated docs for %s", suffix)
}
