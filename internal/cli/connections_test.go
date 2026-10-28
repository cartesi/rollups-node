// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestDialBlockchainUsesConfiguredAuthorizationAndRetries(t *testing.T) {
	viper.Reset()
	config.SetDefaults()
	t.Cleanup(func() { viper.Reset(); config.SetDefaults() })
	viper.Set(config.BLOCKCHAIN_HTTP_AUTHORIZATION, "Authorization:Bearer private-provider-key")
	viper.Set(config.BLOCKCHAIN_HTTP_MAX_RETRIES, 1)
	viper.Set(config.BLOCKCHAIN_HTTP_RETRY_MIN_WAIT, "0")
	viper.Set(config.BLOCKCHAIN_HTTP_RETRY_MAX_WAIT, "0")
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer private-provider-key" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}
		if attempt == 1 {
			http.Error(w, "try again", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x7a69"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	endpoint, err := config.ToHTTPURLFromString(server.URL)
	require.NoError(t, err)
	client, err := DialBlockchain(t.Context(), endpoint)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	chainID, err := client.ChainID(t.Context())
	require.NoError(t, err)
	require.Equal(t, uint64(31337), chainID.Uint64())
	require.Equal(t, int64(2), calls.Load())
}
