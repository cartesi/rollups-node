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
// - a formatted get.go file, with  get functions for some environment variables;
//
// - a formatted nodeconfig.go file with the NodeConfig struct and two init functions:
// one that initializes it by reading the values from environment variables and another that
// initializes it with default values;
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

	//Validate envs
	for i := range envs {
		envs[i].validate()
	}

	writeDoc(envs)
	writeCode(envs)
}

func writeCode(envs []Env) {
	var privateGetFunctions strings.Builder
	addGetHeader(&privateGetFunctions)
	// //Add get functions
	for _, env := range envs {
		if !*env.Export {
			addLine(&privateGetFunctions, env.toFunction())
		}
	}
	writeToFile("get.go", formatCode(privateGetFunctions.String()))

	var code strings.Builder
	addCodeHeader(&code)
	//Add NodeConfig Struct
	addLine(&code, "type NodeConfig struct{")
	for _, env := range envs {
		if *env.Export {
			addLine(&code, env.toStructMember())
		}
	}
	addLine(&code, "cartesiAuth *Auth")
	addLine(&code, "cartesiAuthError error")
	addLine(&code, "}")

	//Add Getters and Setters
	addLine(&code, "")
	for _, env := range envs {
		if *env.Export {
			name := toTitleCase(env.Name)
			varName := toVarName(env.Name)
			//Getter
			addLine(&code, "func (nodeConfig* NodeConfig) "+name+"() "+env.GoType+" {")
			addLine(&code, "if nodeConfig."+varName+" == nil {")
			addLine(&code, `fail("Missing required `+env.Name+` env var")`)
			addLine(&code, "}")
			addLine(&code, "return *nodeConfig."+varName)
			addLine(&code, "}")
			addLine(&code, "")
			//Setter
			addLine(&code, "func (nodeConfig* NodeConfig) Set"+name+"(v *"+env.GoType+") {")
			addLine(&code, "nodeConfig."+varName+" = v")
			addLine(&code, "}")
			addLine(&code, "")
			addLine(&code, "")
		}
	}

	addLine(&code, "func (nodeConfig* NodeConfig) CartesiAuth() Auth {")
	addLine(&code, "if nodeConfig.cartesiAuth == nil {panic(nodeConfig.cartesiAuthError)}")
	addLine(&code, "return *nodeConfig.cartesiAuth")
	addLine(&code, "}")
	addLine(&code, "")

	addLine(&code, "func (nodeConfig* NodeConfig) SetCartesiAuth(v *Auth) {")
	addLine(&code, "nodeConfig.cartesiAuth = v")
	addLine(&code, "}")
	addLine(&code, "")
	addLine(&code, "")

	//Add init function from System Environment
	addLine(&code, "")
	addLine(&code, "func NewNodeConfigFromEnv() (NodeConfig){")
	addLine(&code, "nodeConfig := NodeConfig{")
	for _, env := range envs {
		if *env.Export {
			name := toVarName(env.Name)
			addLine(&code, name+": "+env.toEnvGetCall()+",")
		}
	}
	addLine(&code, "}")
	addLine(&code, "nodeConfig.cartesiAuth, nodeConfig.cartesiAuthError = getAuth()")
	addLine(&code, "return nodeConfig")
	addLine(&code, "}")

	//Add init function from Default Values
	addLine(&code, "")
	addLine(&code, "func NewNodeConfig() (NodeConfig){")
	addLine(&code, "nodeConfig := NodeConfig{}")
	for _, env := range envs {
		if *env.Export && env.Default != nil && *env.Default != "" {
			name := toVarName(env.Name)
			varName := toVarName(name)
			addLine(&code, varName+", err := "+toToFuncName(env.GoType)+`("`+*env.Default+`")`)
			addLine(&code, "if err != nil {")
			addLine(&code, "panic(err)")
			addLine(&code, "}")
			addLine(&code, "nodeConfig."+name+" = &"+varName)
			addLine(&code, "")
		}
	}
	addLine(&code, `var auth Auth = AuthMnemonic{`)
	addLine(&code, `Mnemonic: "test test test test test test test test test test test junk",`)
	addLine(&code, `AccountIndex: 0,`)
	addLine(&code, "}")
	addLine(&code, "nodeConfig.cartesiAuth = &auth")
	addLine(&code, "nodeConfig.cartesiAuthError = nil")
	addLine(&code, "return nodeConfig")
	addLine(&code, "}")

	writeToFile("nodeconfig.go", formatCode(code.String()))
}

func writeDoc(envs []Env) {
	var doc strings.Builder

	addDocHeader(&doc)
	addLine(&doc, "<!-- markdownlint-disable MD012 -->")
	for _, env := range envs {
		addLine(&doc, env.toDoc())
	}
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
	name := toTitleCase(e.Name)
	typ := e.GoType
	get := "get"
	vars := "v"

	var defaultValue string
	hasDefault := e.Default != nil
	if hasDefault {
		defaultValue = *e.Default
	}

	to := toToFuncName(e.GoType)

	args := fmt.Sprintf(`"%s", "%s", %t, %t, %s`, e.Name, defaultValue, hasDefault, *e.Redact, to)

	name = "get" + name

	typ = fmt.Sprintf("*%s", typ)
	get += "Optional"

	body := fmt.Sprintf("%s := %s(%s)\n", vars, get, args)
	body += "return " + vars
	return fmt.Sprintf("func %s() %s { %s }\n", name, typ, body)
}

func (e Env) toEnvGetCall() string {

	var defaultValue string
	hasDefault := e.Default != nil
	if hasDefault {
		defaultValue = *e.Default
	}

	to := toToFuncName(e.GoType)

	args := fmt.Sprintf(`"%s", "%s", %t, %t, %s`, e.Name, defaultValue, hasDefault, *e.Redact, to)

	get := "getOptional"

	return fmt.Sprintf("%s(%s)", get, args)
}

// Generates the Config Struct member for the envrionemnt variable.
func (e Env) toStructMember() string {
	name := toVarName(e.Name)
	return name + " *" + e.GoType
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
func toTitleCase(env string) string {
	caser := cases.Title(language.English)
	words := strings.Split(env, "_")
	for i, word := range words {
		words[i] = caser.String(word)
	}
	return strings.Join(words, "")
}

func toVarName(name string) string {
	name = toTitleCase(name)
	name_ := []rune(name)
	name_[0] = unicode.ToLower(name_[0])
	return string(name_)
}

func toToFuncName(goType string) string {
	to_ := []rune(goType)
	to_[0] = unicode.ToUpper(to_[0])
	return "to" + string(to_)
}
