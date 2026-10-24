// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListTestsUsesGoSelectedFiles(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"native_test.go": `package integration
import (
    "testing"
    _ "github.com/cartesi/rollups-node/internal/advancer"
)
type Suite struct{}
func TestNative(t *testing.T) {}
func Test(t *testing.T) {}
func TestÉpoch(t *testing.T) {}
func TestMain(m *testing.M) {}
func Testhelper() {}
func BenchmarkNative(b *testing.B) {}
func (s *Suite) TestMethod() {}
`,
		"external_test.go": `package integration_test
import "testing"
func TestExternal(t *testing.T) {}
`,
		"inactive_test.go": "not valid Go: this file was excluded by go list",
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	metadata, err := json.Marshal(map[string]any{
		"Dir": dir, "TestGoFiles": []string{"native_test.go"}, "XTestGoFiles": []string{"external_test.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := listTests(bytes.NewReader(metadata), &output); err != nil {
		t.Fatal(err)
	}
	want := "TestNative\nTest\nTestÉpoch\nTestExternal\n"
	if output.String() != want {
		t.Fatalf("got %q, want %q", output.String(), want)
	}
}

func TestListTestsRejectsReadErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "invalid_test.go"), []byte("invalid Go"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct{ name, input string }{
		{"metadata", "not JSON"},
		{"missing file", `{"Dir":"` + dir + `","TestGoFiles":["missing_test.go"]}`},
		{"invalid syntax", `{"Dir":"` + dir + `","TestGoFiles":["invalid_test.go"]}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if err := listTests(strings.NewReader(scenario.input), &bytes.Buffer{}); err == nil {
				t.Fatal("expected discovery error")
			}
		})
	}
}
