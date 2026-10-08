// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"testing"

	"github.com/cartesi/rollups-node/test/tooling/command"
)

func TestSecretCommandProcess(t *testing.T) {
	command.Child(t, main, newRootCommand())
}

func TestSecretCommandHelp(t *testing.T) {
	command.CheckHelp(t, "machine-tool")
}
