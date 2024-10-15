// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:generate go run .

// This script will read the Config.toml file and create:
// - a formatted get.go file, with get functions for each environment variable;
// - a config.md file with documentation for the environment variables.
//
// Each table entry in the toml file translates into an environment variable.
// In Go, this becomes a map[string](map[string]Env), with the keys of the outer map being topic
// names, and the keys of the inner map being variable names.
package main

func main() {
	data := readTOML("Config.toml")
	config := decodeTOML(data)
	envs := sortConfig(config)
	for _, env := range envs {
		env.validate()
	}
	generateDocsFile("../../../docs/config.md", envs)
	generateCodeFile("../generated.go", envs)
}
