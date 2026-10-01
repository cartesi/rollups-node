// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDropNullError(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string // empty when the body must come back unchanged
	}{
		{
			name: "single response",
			body: `{"jsonrpc":"2.0","id":1,"result":"0x10","error":null}`,
			want: `{"jsonrpc":"2.0","id":1,"result":"0x10"}`,
		},
		{
			name: "batch response",
			body: `[{"jsonrpc":"2.0","id":1,"result":"0x1","error":null},` +
				`{"jsonrpc":"2.0","id":2,"error":{"code":3,"message":"execution reverted"}}]`,
			want: `[{"jsonrpc":"2.0","id":1,"result":"0x1"},` +
				`{"jsonrpc":"2.0","id":2,"error":{"code":3,"message":"execution reverted"}}]`,
		},
		{
			name: "error object",
			body: `{"jsonrpc":"2.0","id":1,"error":{"code":3,"message":"execution reverted"}}`,
		},
		{
			name: "no error member",
			body: `{"jsonrpc":"2.0","id":1,"result":"0x10"}`,
		},
		{
			name: "error string in the result",
			body: `{"jsonrpc":"2.0","id":1,"result":["error"]}`,
		},
		{
			name: "not JSON",
			body: `"error" page`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := dropNullError([]byte(tc.body))
			if tc.want == "" {
				require.False(t, changed)
				require.Equal(t, tc.body, string(got))
				return
			}
			require.True(t, changed)
			require.JSONEq(t, tc.want, string(got))
		})
	}
}

// newNullErrorServer answers every JSON-RPC call with the result 0x10 and a null
// error member.
func newNullErrorServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x10","error":null}`, request.ID)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestNewEthClientAcceptsNullError(t *testing.T) {
	server := newNullErrorServer(t)
	logger := slog.New(slog.DiscardHandler)
	client, err := NewEthClient(t.Context(), server.URL, logger, RetryConfig{RequestTimeout: time.Second})
	require.NoError(t, err)
	defer client.Close()
	number, err := client.BlockNumber(t.Context())
	require.NoError(t, err)
	require.Equal(t, uint64(16), number)
}

func TestDialEthClientAcceptsNullError(t *testing.T) {
	server := newNullErrorServer(t)
	client, err := DialEthClient(t.Context(), server.URL)
	require.NoError(t, err)
	defer client.Close()
	number, err := client.BlockNumber(t.Context())
	require.NoError(t, err)
	require.Equal(t, uint64(16), number)
}
