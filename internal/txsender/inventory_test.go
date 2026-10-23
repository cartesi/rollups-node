// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Forbidigo checks normalized binding and client symbols. This companion
// check covers RPC method literals, which forbidigo does not inspect.
func TestServicesCannotPublishRawTransactions(t *testing.T) {
	for _, directory := range []string{"../claimer", "../prt"} {
		files, err := filepath.Glob(filepath.Join(directory, "*.go"))
		require.NoError(t, err)
		for _, name := range files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
			require.NoError(t, err)
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				require.NoError(t, err)
				require.NotEqual(t, "eth_sendRawTransaction", value, "%s bypasses the owned sender", name)
				return true
			})
		}
	}
}
