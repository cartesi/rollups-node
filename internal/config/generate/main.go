// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// This script will read the Config.toml file and create:
//
// - a formatted get.go file, with get functions for each environment variable;
//
// - a config.md file with documentation for the environment variables.
//
// Each table entry in the toml file translates into an environment variable.
// In Go, this becomes a map[string](map[string]Env),
// with the keys of the outter map being topic names,
// and the keys of the inner map being variable names.
func main() {
	data := readTOML("generate/Config.toml")
	config := decodeTOML(data)
	envs := sortConfig(config)

	var code strings.Builder
	var doc strings.Builder

	addCodeHeader(&code)
	addDocHeader(&doc)

	addLine(&doc, "<!-- markdownlint-disable MD012 -->")
	for _, env := range envs {
		env.validate()
		addLine(&code, env.toFunction())
		addLine(&doc, env.toDoc())
	}

	writeToFile("get.go", formatCode(code.String()))
	writeToFile("../../docs/config.md", []byte(doc.String()))
}

// ------------------------------------------------------------------------------------------------
// Env
// ------------------------------------------------------------------------------------------------

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
	// If true, the generated get function will be exported by the config module.
	// This field is optional.
	// By default, this field is true.
	Export *bool `toml:"export"`
	// If true, the generated get function will not log into the console.
	// This field is optional.
	// By default, this field is false.
	Redact *bool `toml:"redact"`
	// A brief description of the environment variable.
	// This field is required.
	Description string `toml:"description"`
}

// Validates whether the fields of the environment variables were initialized correctly
// and sets defaults for optional fields.
func (e *Env) validate() {
	if e.GoType == "" {
		panic("missing go-type for " + e.Name)
	}
	if e.Export == nil {
		export := true
		e.Export = &export
	}
	if e.Redact == nil {
		redact := false
		e.Redact = &redact
	}
	if e.Description == "" {
		panic("missing description for " + e.Name)
	}
}

// Generates the get function for the environment variable.
func (e Env) toFunction() string {
	name := toFunctionName(e.Name)
	typ := e.GoType
	get := "get"
	vars := "v"

	var defaultValue string
	hasDefault := e.Default != nil
	if hasDefault {
		defaultValue = *e.Default
	}

	to_ := []rune(e.GoType)
	to_[0] = unicode.ToUpper(to_[0])
	to := "to" + string(to_)

	args := fmt.Sprintf(`"%s", "%s", %t, %t, %s`, e.Name, defaultValue, hasDefault, *e.Redact, to)

	if *e.Export {
		name = "Get" + name
	} else {
		name = "get" + name
		typ = fmt.Sprintf("(%s, bool)", typ)
		get += "Optional"
		vars += ", ok"
	}

	body := fmt.Sprintf("%s := %s(%s)\n", vars, get, args)
	body += "return " + vars
	return fmt.Sprintf("func %s() %s { %s }\n", name, typ, body)
}

// Generates the documentation entry for the environment variable.
func (e Env) toDoc() string {
	s := fmt.Sprintf("## `%s`\n\n%s\n\n", e.Name, e.Description)
	s = fmt.Sprintf("%s* **Type:** `%s`\n", s, e.GoType)
	if e.Default != nil {
		s = fmt.Sprintf("%s* **Default:** `\"%s\"`\n", s, *e.Default)
	}
	return s
}

// Splits the string by "_" and joins each substring with the first letter in upper case.
func toFunctionName(env string) string {
	caser := cases.Title(language.English)
	words := strings.Split(env, "_")
	for i, word := range words {
		words[i] = caser.String(word)
	}
	return strings.Join(words, "")
}
