// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func setAuthEnv(t *testing.T, direct, file string) {
	t.Helper()
	resetSecretConfig(t)
	t.Setenv(BLOCKCHAIN_HTTP_AUTHORIZATION, direct)
	t.Setenv(BLOCKCHAIN_HTTP_AUTHORIZATION_FILE, file)
}

func writeAuthFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "authorization")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	require.NoError(t, os.Chmod(path, 0o600))
	return path
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
	path := writeAuthFile(t, "Authorization: Bearer file-token\n")
	setAuthEnv(t, "", path)
	opt, err := HTTPAuthorizationOption()
	require.NoError(t, err)
	require.Equal(t, "Bearer file-token", authHeader(t, opt).Get("Authorization"))
}

func TestHTTPAuthorizationOptionDirectValueTakesPrecedence(t *testing.T) {
	path := writeAuthFile(t, "Authorization: Bearer file-token\n")
	setAuthEnv(t, "Authorization: Bearer direct-token", path)
	opt, err := HTTPAuthorizationOption()
	require.NoError(t, err)
	require.Equal(t, "Bearer direct-token", authHeader(t, opt).Get("Authorization"))
}

func TestHTTPAuthorizationOptionMissingFile(t *testing.T) {
	const marker = "SYNTHETIC_AUTH_FILE_PATH_2841937"
	setAuthEnv(t, "", filepath.Join(t.TempDir(), marker))
	opt, err := HTTPAuthorizationOption()
	require.Nil(t, opt)
	require.Error(t, err)
	require.NotContains(t, err.Error(), marker)
}

func TestHTTPAuthorizationOptionRejectsMalformedValues(t *testing.T) {
	const marker = "SYNTHETIC_AUTH_SECRET_6284193"
	tests := []struct {
		name  string
		value string
	}{
		{name: "missing colon", value: "Bearer " + marker},
		{name: "empty field name", value: ": " + marker},
		{name: "empty field value", value: "Authorization:"},
		{name: "space in field name", value: "Bad Key: " + marker},
		{name: "invalid field name character", value: "Authoriz@tion: " + marker},
		{name: "CRLF in field value", value: "Authorization: " + marker + "\r\nX-Injected: yes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setAuthEnv(t, tt.value, "")
			opt, err := HTTPAuthorizationOption()
			require.Nil(t, opt)
			require.ErrorContains(t, err, "malformed BlockchainHttpAuthorization")
			require.NotContains(t, err.Error(), marker)
		})
	}
}

func TestHTTPAuthorizationOptionRejectsMalformedFile(t *testing.T) {
	const marker = "SYNTHETIC_AUTH_FILE_SECRET_7318402"
	path := writeAuthFile(t, "Bad Key: "+marker+"\n")
	setAuthEnv(t, "", path)
	opt, err := HTTPAuthorizationOption()
	require.Nil(t, opt)
	require.ErrorContains(t, err, "malformed BlockchainHttpAuthorization")
	require.NotContains(t, err.Error(), marker)
}

func TestParseHTTPAuthorizationRejectsControlCharacters(t *testing.T) {
	const marker = "SYNTHETIC_AUTH_CONTROL_SECRET_9182734"
	_, _, err := parseHTTPAuthorization("Authorization: " + marker + "\x00")
	require.ErrorContains(t, err, "malformed BlockchainHttpAuthorization")
	require.NotContains(t, err.Error(), marker)
}
