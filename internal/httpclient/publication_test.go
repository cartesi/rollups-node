// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package httpclient

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicationTransportPreservesOnlyParsedMessageCategories(t *testing.T) {
	for _, test := range []struct{ method, body, want string }{
		{publicationMethod, `{"error":{"code":-32000,"message":"already known SECRET"}}`, "already known"},
		{publicationMethod, `{"error":{"code":-32000,"message":"unknown","data":"nonce too low SECRET"}}`, rpcFailedMessage},
		{publicationMethod, `{"error":{"code":3,"message":"already known SECRET"}}`, rpcFailedMessage},
		{"eth_call", `{"error":{"code":-32000,"message":"already known SECRET"}}`, rpcFailedMessage},
		{publicationMethod, `<html>nonce too low SECRET</html>`, httpFailedMessage},
	} {
		t.Run(test.method+test.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, err := io.WriteString(w, test.body)
				require.NoError(t, err)
			}))
			defer server.Close()
			safe, client, err := NewClient(server.URL, nil, Options{PublicationReplies: true})
			require.NoError(t, err)
			requestBody, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": test.method})
			require.NoError(t, err)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, safe, bytes.NewReader(requestBody))
			require.NoError(t, err)
			response, err := client.Do(req)
			require.NoError(t, err)
			data, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Contains(t, string(data), test.want)
			require.NotContains(t, string(data), "SECRET")
			if test.want == rpcFailedMessage {
				require.NotContains(t, string(data), nonceLowMessage)
				require.NotContains(t, string(data), "already known")
			}
		})
	}
}
