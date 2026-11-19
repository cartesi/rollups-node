// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/jsonutil"
)

type countedJSONItem struct{ calls *int }

func (v countedJSONItem) MarshalJSON() ([]byte, error) {
	*v.calls++
	return json.Marshal(strings.Repeat("x", 1024), jsonutil.Options)
}

func TestListResponseStopsEncodingAtBudget(t *testing.T) {
	calls := 0
	items := make([]countedJSONItem, 10000)
	for i := range items {
		items[i].calls = &calls
	}
	var output bytes.Buffer
	budget := newBudgetWriter(&output, 4096)
	limited := budget.NewLimitedWriter()
	err := writeRPCResult(limited, jsontext.Value("1"), api.ListResponse[countedJSONItem]{Data: items})
	require.ErrorIs(t, err, io.ErrShortBuffer)
	// JSON v2 can buffer a small chunk ahead of the limited writer.
	require.Positive(t, calls)
	require.Less(t, calls, 100, "must stop marshaling items when the writer rejects the response")
	require.NoError(t, limited.Flush())
	require.Empty(t, output.String(), "partial JSON must never reach the client")
}

func TestStreamedListResponseMatchesStandardEncoding(t *testing.T) {
	for _, data := range [][]string{nil, {}, {"<hello>", "a\nb", "雪"}} {
		for _, id := range []jsontext.Value{nil, jsontext.Value("null"), jsontext.Value(`"<id>"`), jsontext.Value("42")} {
			result := api.ListResponse[string]{Data: data, Pagination: api.Pagination{TotalCount: 3, Limit: 10}}
			var want, got bytes.Buffer
			require.NoError(t, json.MarshalEncode(jsontext.NewEncoder(&want), RPCResponse{JSONRPC: "2.0", Result: result, ID: id}, jsonutil.Options))
			require.NoError(t, writeRPCResult(&got, id, result))
			require.Equal(t, want.String(), got.String())
		}
	}
}
