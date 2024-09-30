// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This binary generates part of the node documentation automatically.

//go:generate go run .
package main

import (
	"log"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root"
	"github.com/spf13/cobra/doc"
)

func main() {
	generateCartesiRollupsCliDocs()
}

func generateCartesiRollupsCliDocs() {
	err := doc.GenMarkdownTree(root.Cmd, "docs/cli")
	if err != nil {
		log.Fatalf("failed to gen cartesi-rollups-cli docs: %v", err)
	}
	log.Print("generated docs for cartesi-rollups-cli")
}
