// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

// An entry in the toml's top level table representing an environment variable.
type Env struct {
	// Name of the environment variable.
	Name string

	// The default value for the variable.
	// This field is optional.
	Default *string `toml:"default"`

	// The Go type for the environment variable.
	// This field is required.
	GoType string `toml:"go-type"`

	// A brief description of the environment variable.
	// This field is required.
	Description string `toml:"description"`

	// If defined omit the field from the generated Config struct
	Omit bool `toml:"omit"`

	// Configuration also has a "_FILE" variant that should be searched
	File bool `toml:"file"`

	// List of services that use this environment variable.
	// Possible values: "advancer", "claimer", "cli", "evm-reader", "jsonrpc-api", "node", "validator"
	UsedBy []string `toml:"used-by"`
}

// Validates whether the fields of the environment variables were initialized correctly
// and sets defaults for optional fields.
func (e *Env) validate() {
	if e.GoType == "" {
		panic("missing go-type for " + e.Name)
	}
	if e.Description == "" {
		panic("missing description for " + e.Name)
	}
}
