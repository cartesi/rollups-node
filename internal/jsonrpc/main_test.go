// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cartesi/rollups-node/test/tooling/db"
)

func TestMain(m *testing.M) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		os.Exit(m.Run())
	}
	release, err := db.LockTestPostgres(context.Background(), endpoint)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	release()
	os.Exit(code)
}
