// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
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

const docsTemplate string = `<!--
File generated by internal/config/generate.
DO NOT EDIT.
-->

<!-- markdownlint-disable line_length -->
# Node Configuration

The node is configurable through environment variables.
(There is no other way to configure it.)

This file documents the configuration options.

<!-- markdownlint-disable MD012 -->
{{- range .}}

## {{backtick .Name}}

{{.Description}}

* **Type:** {{backtick .GoType}}
{{- if .Default}}
* **Default:** {{.Default | quote | backtick}}
{{- end}}
{{- end}}
`
