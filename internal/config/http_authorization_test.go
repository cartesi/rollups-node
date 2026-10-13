// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// setAuthEnv configures the blockchain http authorization the same way the
// generated getters read it: direct value first, then the _FILE variant.
func setAuthEnv(t *testing.T, direct, file string) {
	t.Helper()
	viper.Reset()
	viper.AutomaticEnv() // Reset drops the package init's AutomaticEnv registration
	t.Setenv(BLOCKCHAIN_HTTP_AUTHORIZATION, direct)
	t.Setenv(BLOCKCHAIN_HTTP_AUTHORIZATION_FILE, file)
}

// authHeader performs one JSON-RPC call through an rpc.Client built with opt
// and returns the headers the server observed on the request.
func authHeader(t *testing.T, opt rpc.ClientOption) http.Header {
	t.Helper()
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":null}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	client, err := rpc.DialOptions(ctx, srv.URL, opt)
	require.NoError(t, err)
	defer client.Close()

	var result any
	require.NoError(t, client.CallContext(ctx, &result, "eth_blockNumber"))
	return got
}

func TestHTTPAuthorizationOptionUnset(t *testing.T) {
	setAuthEnv(t, "", "")
	opt, err := HTTPAuthorizationOption()
	require.NoError(t, err)
	require.Nil(t, opt)
}

func TestHTTPAuthorizationOptionDirectValue(t *testing.T) {
	setAuthEnv(t, "Authorization: Bearer direct-token", "")
	opt, err := HTTPAuthorizationOption()
	require.NoError(t, err)
	require.Equal(t, "Bearer direct-token", authHeader(t, opt).Get("Authorization"))
}

func TestHTTPAuthorizationOptionFromFile(t *testing.T) {
	path := writeSecretFile(t, 0o400, "Authorization: Bearer file-token\n")
	setAuthEnv(t, "", path)
	opt, err := HTTPAuthorizationOption()
	require.NoError(t, err)
	require.Equal(t, "Bearer file-token", authHeader(t, opt).Get("Authorization"))
}

func TestHTTPAuthorizationOptionDirectValueTakesPrecedence(t *testing.T) {
	path := writeSecretFile(t, 0o400, "Authorization: Bearer file-token\n")
	setAuthEnv(t, "Authorization: Bearer direct-token", path)
	opt, err := HTTPAuthorizationOption()
	require.NoError(t, err)
	require.Equal(t, "Bearer direct-token", authHeader(t, opt).Get("Authorization"))
}

func TestHTTPAuthorizationOptionMissingFile(t *testing.T) {
	setAuthEnv(t, "", "/nonexistent/cartesi-http-authorization")
	_, err := HTTPAuthorizationOption()
	require.Error(t, err)
}

func TestHTTPAuthorizationOptionRejectsInsecurePermissions(t *testing.T) {
	skipIfNoPermissionPolicy(t)
	path := writeSecretFile(t, 0o644, "Authorization: Bearer token\n")
	setAuthEnv(t, "", path)
	_, err := HTTPAuthorizationOption()
	require.ErrorContains(t, err, "does not conform with uid/gid/mode rules")
}

func TestHTTPAuthorizationOptionMalformed(t *testing.T) {
	setAuthEnv(t, "no-colon-here", "")
	_, err := HTTPAuthorizationOption()
	require.ErrorContains(t, err, "malformed BlockchainHttpAuthorization")
}
