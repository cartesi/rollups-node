// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// oapi-codegen emits v1 JSON calls. Keep its checked-in client on the same
// native v2 API and wire conventions as the rest of the node after regeneration.
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"regexp"
	"strings"
)

func migrate(src []byte) ([]byte, error) {
	s := string(src)
	if !strings.Contains(s, `"encoding/json"`) {
		return nil, fmt.Errorf("expected oapi-codegen JSON import; review generator output")
	}
	s = strings.ReplaceAll(s, `"encoding/json"`, `"encoding/json/v2"`)
	s = strings.Replace(s, `"github.com/oapi-codegen/runtime"`, "\"github.com/oapi-codegen/runtime\"\n\n\"github.com/cartesi/rollups-node/internal/jsonutil\"", 1)
	// The inspect client has only response decoding; reject new JSON operations
	// rather than silently generating calls with incompatible v1 signatures.
	call := regexp.MustCompile(`json\.Unmarshal\((\w+), &(\w+)\)`)
	if len(call.FindAllString(s, -1)) != 1 {
		return nil, fmt.Errorf("expected one inspect response decode; review generator output")
	}
	s = call.ReplaceAllString(s, `json.Unmarshal($1, &$2, jsonutil.Options)`)
	optional := regexp.MustCompile("(\\*\\w+\\s+`json:\"[^\"]*),omitempty(\"`)")
	s = optional.ReplaceAllString(s, "${1},omitzero${2}")
	return format.Source([]byte(s))
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: jsonv2gen generated.go")
	}
	path := os.Args[1]
	src, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	dst, err := migrate(src)
	if err != nil {
		panic(err)
	}
	if bytes.Equal(src, dst) {
		return
	}
	if err := os.WriteFile(path, dst, 0644); err != nil {
		panic(err)
	}
}
