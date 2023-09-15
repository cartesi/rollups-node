// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package loads the node config from enviroment variables
package config

import (
	"fmt"
	"os"
)

// Prefix to all enviroment variables
const Prefix string = "CARTESI_"

// Definition of config variables.
// For each variable declared below, there should be a entry in meta.go.

var PrintConfig bool
var LogLevel string

// Prints the documentation for each variable in Stdout
func PrintDocumentation() {
	genDocumentation(os.Stdout)
}

// Loads the config from the environment.
// If PrintConfig is true, prints the config to Stdout.
// This function calls os.Exit if there is an error.
func Load() {
	err := loadFromEnv(os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
