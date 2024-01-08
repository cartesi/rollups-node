// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build ignore

package main

import (
	_ "cmd/gen-docs"
	_ "internal/config/config.go"
	_ "pkg/contracts/main.go"
	_ "pkg/readerclient/main.go"
)
