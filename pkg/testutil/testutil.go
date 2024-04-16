// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package contains tests utilities.

package testutil

import "os"

func GetCartesiTestDepsPortRange() string {
	return os.Getenv("CARTESI_TEST_DEPS_PORT_RANGE")
}
