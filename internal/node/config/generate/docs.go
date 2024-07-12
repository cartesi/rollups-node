// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	_ "embed"
	"os"
	"text/template"
)

// generateDocsFile generates a Markdown file with the documentation of the config variables.
func generateDocsFile(path string, env []Env) {
	// Open output file
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Load template
	funcMap := template.FuncMap{
		"backtick": func(s string) string {
			return "`" + s + "`"
		},
		"quote": func(s string) string {
			return `"` + s + `"`
		},
	}
	tmpl := template.Must(template.New("docs").Funcs(funcMap).Parse(docsTemplate))

	// Execute template
	err = tmpl.Execute(file, env)
	if err != nil {
		panic(err)
	}
}

//go:embed docs.md.tpl
var docsTemplate string
