// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:generate go run . -mode=code

// This script will read the Config.toml file and create:
// - a formatted get.go file, with get functions for each environment variable;
// - a config.md file with documentation for the environment variables.
//
// Usage:
//
//	go run . -mode=code  # generate only the Go code (generated.go)
//	go run . -mode=docs  # generate only the documentation (docs/config.md)
//	go run . -mode=all   # generate both (default)
//
// Each table entry in the toml file translates into an environment variable.
// In Go, this becomes a map[string](map[string]Env), with the keys of the outer map being topic
// names, and the keys of the inner map being variable names.
package main

import (
	"flag"
	"log"
)

func main() {
	mode := flag.String("mode", "all", "generation mode: code, docs, or all")
	flag.Parse()

	data := readTOML("Config.toml")
	config := decodeTOML(data)
	envs := sortConfig(config)
	for _, env := range envs {
		env.validate()
	}

	switch *mode {
	case "code":
		generateCodeFile("../generated.go", envs)
	case "docs":
		generateDocsFile("../../../docs/config.md", envs)
	case "all":
		generateCodeFile("../generated.go", envs)
		generateDocsFile("../../../docs/config.md", envs)
	default:
		log.Fatalf("invalid mode: %s (expected code, docs, or all)", *mode)
	}
}
