// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPSettingsRequireSchemeAndHostname(t *testing.T) {
	getters := []struct {
		key string
		get func() (SafeURL, error)
	}{
		{BLOCKCHAIN_HTTP_ENDPOINT, GetBlockchainHttpEndpoint},
		{JSONRPC_API_URL, GetJsonrpcApiUrl},
		{INSPECT_URL, GetInspectUrl},
	}
	const marker = "SYNTHETIC_HTTP_ENDPOINT_SECRET_1426709538"
	for _, getter := range getters {
		t.Run(getter.key, func(t *testing.T) {
			for _, raw := range []string{
				"wss://provider.example/" + marker,
				"ws://provider.example/" + marker,
				"file:///tmp/" + marker,
				"/tmp/" + marker,
				"http:///" + marker,
				"https://:8545/" + marker,
				"http://user:" + marker + "@/",
				"https://provider.example/%" + marker,
			} {
				resetSecretConfig(t)
				t.Setenv(getter.key, raw)
				value, err := getter.get()
				require.Error(t, err, "expected an HTTP endpoint rejection")
				require.Contains(t, err.Error(), getter.key)
				require.NotContains(t, fmt.Sprintf("%+v", err), marker)
				require.Empty(t, value.Raw())
			}
			for _, scheme := range []string{"http", "https"} {
				resetSecretConfig(t)
				raw := scheme + "://user:" + marker + "@provider.example:8545/a%2Fb?key=" + marker + "&key=second"
				t.Setenv(getter.key, raw)
				value, err := getter.get()
				require.NoError(t, err)
				require.Equal(t, raw, value.Raw())
				require.Equal(t, scheme+"://provider.example:8545", value.String())
			}
		})
	}
}

func TestHTTPPolicyDoesNotChangeGenericURLs(t *testing.T) {
	for _, raw := range []string{
		"postgres://user:secret@db.example:5432/rollups?sslmode=require",
		"postgresql://user:secret@db.example:5432/rollups?sslmode=require",
		"wss://provider.example/ws",
	} {
		value, err := ToURLFromString(raw)
		require.NoError(t, err)
		require.Equal(t, raw, value.Raw())
	}
}

func TestDatabaseURLValidationDoesNotEchoQueryKeys(t *testing.T) {
	const marker = "SYNTHETIC_DATABASE_QUERY_SECRET_7962140835"
	_, err := ToURLFromString("postgres://user:password@db.example/rollups?" + marker + "=first&" + marker + "=second")
	require.Error(t, err)
	require.Contains(t, err.Error(), "repeated")
	require.NotContains(t, err.Error(), marker)
}
