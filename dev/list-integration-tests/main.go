// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// list-integration-tests reads go list -json output and lists top-level tests
// without compiling the integration package or loading its native dependencies.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

func main() {
	if err := listTests(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func listTests(input io.Reader, output io.Writer) error {
	decoder := json.NewDecoder(input)
	for {
		var pkg struct {
			Dir          string
			TestGoFiles  []string
			XTestGoFiles []string
		}
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return fmt.Errorf("read Go package metadata: %w", err)
		}
		for _, name := range append(pkg.TestGoFiles, pkg.XTestGoFiles...) {
			path := filepath.Join(pkg.Dir, name)
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return fmt.Errorf("parse integration test: %w", err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || fn.Name.Name == "TestMain" {
					continue
				}
				suffix, test := strings.CutPrefix(fn.Name.Name, "Test")
				next, _ := utf8.DecodeRuneInString(suffix)
				if test && !unicode.IsLower(next) {
					if _, err := fmt.Fprintln(output, fn.Name.Name); err != nil {
						return fmt.Errorf("write integration test name: %w", err)
					}
				}
			}
		}
	}
}
