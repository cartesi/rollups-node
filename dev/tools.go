// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build tools

package main

import (
	_ "github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen"
	// Import the Jet CLI tool for code generation
	_ "github.com/go-jet/jet/v2/cmd/jet"
)
