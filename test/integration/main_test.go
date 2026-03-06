// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// TestMain enforces that integration tests run sequentially.
// These tests share a single Anvil blockchain instance and PRT tests call
// anvilMine() which globally advances the block number, affecting Authority
// epoch boundaries. Running tests in parallel would cause subtle timing races.
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Fprintln(os.Stderr, "skipping integration tests in short mode")
		os.Exit(0)
	}
	// Warn if -test.parallel is set above 1. The test binary flag is already
	// parsed by flag.Parse(), so we can inspect it directly via GOMAXPROCS or
	// by checking the flag value. Go's testing package uses -test.parallel to
	// control the maximum number of tests running in parallel; the default is
	// GOMAXPROCS which may be >1.
	p := flag.Lookup("test.parallel")
	if p != nil && p.Value.String() != "1" {
		fmt.Fprintln(os.Stderr,
			"WARNING: integration tests must not run in parallel "+
				"(-test.parallel should be 1). Forcing -test.parallel=1.")
		if err := p.Value.Set("1"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to set -test.parallel=1: %v\n", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
