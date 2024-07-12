// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"bytes"
	_ "embed"
	"go/format"
	"os"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// generateCodeFile generates a Go file with the getters for the config variables.
func generateCodeFile(path string, env []Env) {
	// Load template
	funcMap := template.FuncMap{
		"toFunctionName": func(env string) string {
			caser := cases.Title(language.English)
			words := strings.Split(env, "_")
			for i, word := range words {
				words[i] = caser.String(word)
			}
			return strings.Join(words[1:], "")
		},
		"toGoFunc": func(goType string) string {
			return "to" + strings.ToUpper(goType[:1]) + goType[1:]
		},
	}
	tmpl := template.Must(template.New("code").Funcs(funcMap).Parse(codeTemplate))

	// Execute template
	var buff bytes.Buffer
	err := tmpl.Execute(&buff, env)
	if err != nil {
		panic(err)
	}

	// Format code
	code, err := format.Source(buff.Bytes())
	if err != nil {
		panic(err)
	}

	// Write file
	var perm os.FileMode = 0644
	err = os.WriteFile(path, code, perm)
	if err != nil {
		panic(err)
	}
}

//go:embed code.go.tpl
var codeTemplate string
