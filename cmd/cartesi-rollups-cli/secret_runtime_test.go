// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/test/tooling/command"
	"github.com/cartesi/rollups-node/test/tooling/db"
)

const secretTestAddress = "0x0000000000000000000000000000000000000001"
const secretContractCommand = "contract"
const secretSummaryCommand = "summary"

func closedSecretEndpoint(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	endpoint := "http://" + listener.Addr().String()
	require.NoError(t, listener.Close())
	return endpoint
}

func configuredSecretEndpoint(t *testing.T, base, marker string) string {
	t.Helper()
	endpoint, err := url.Parse(base + "/" + marker + "?key=" + marker)
	require.NoError(t, err)
	endpoint.User = url.UserPassword(marker, "password")
	return endpoint.String()
}

func TestSecretRuntimeFailures(t *testing.T) {
	base := closedSecretEndpoint(t)
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "contract text", args: []string{secretContractCommand, secretSummaryCommand, secretTestAddress}},
		{name: "contract json", args: []string{secretContractCommand, secretSummaryCommand, secretTestAddress, "--json"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var firstOutput string
			for i, marker := range []string{"SECRET_MARKER_A", "SECRET_MARKER_B"} {
				endpoint := configuredSecretEndpoint(t, base, marker)
				stdout, stderr, err := command.Run(t, []string{
					config.BLOCKCHAIN_HTTP_ENDPOINT + "=" + endpoint,
					config.JSONRPC_API_URL + "=" + endpoint,
					config.INSPECT_URL + "=" + endpoint,
					config.LOG_COLOR + "=false",
				}, test.args...)
				require.Error(t, err)
				output := stdout + stderr
				require.NotContains(t, output, marker)
				require.Contains(t, output, base)
				require.Contains(t, output, "request failed")
				if i == 0 {
					firstOutput = output
				} else {
					require.Equal(t, firstOutput, output, "secret values must not affect failure output")
				}
			}
		})
	}
}

func TestSecretContractSummaryPartialFailure(t *testing.T) {
	const marker = "PARTIAL_SUMMARY_ENDPOINT_SECRET"
	var failures atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid test request", http.StatusBadRequest)
			return
		}
		if request.Method == "eth_chainId" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0x1"})
			return
		}
		failures.Add(1)
		http.Error(w, r.RequestURI+" "+r.Header.Get("Authorization"), http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	endpoint := configuredSecretEndpoint(t, server.URL, marker)
	for _, asJSON := range []bool{false, true} {
		args := []string{secretContractCommand, secretSummaryCommand, secretTestAddress, "--block=0"}
		if asJSON {
			args = append(args, "--json")
		}
		stdout, stderr, err := command.Run(t, []string{config.BLOCKCHAIN_HTTP_ENDPOINT + "=" + endpoint}, args...)
		require.NoError(t, err, stderr)
		require.NotContains(t, stdout+stderr, marker)
		require.Contains(t, stdout, "500 Internal Server Error")
		if asJSON {
			var result struct {
				AppError       string `json:"app_error"`
				ConsensusError string `json:"consensus_error"`
				InputBoxError  string `json:"inputbox_error"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &result))
			require.NotEmpty(t, result.AppError)
			require.NotEmpty(t, result.ConsensusError)
			require.NotEmpty(t, result.InputBoxError)
		}
	}
	require.Positive(t, failures.Load(), "partial query failures must actually occur")
}

func TestSecretFileMistakeCLI(t *testing.T) {
	const marker = "SECRET_MISTAKEN_FOR_FILE_PATH"
	stdout, stderr, err := command.Run(t,
		[]string{config.BLOCKCHAIN_HTTP_ENDPOINT_FILE + "=" + marker},
		secretContractCommand, secretSummaryCommand, secretTestAddress)
	require.Error(t, err)
	require.NotContains(t, stdout+stderr, marker)
	require.Contains(t, stdout+stderr, config.BLOCKCHAIN_HTTP_ENDPOINT_FILE)
	require.Contains(t, stdout+stderr, "no such file")
}

func TestSecretAppRegisterFatalOutput(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skip("registration integration requires CARTESI_TEST_DATABASE_CONNECTION")
	}
	release, err := db.LockTestPostgres(t.Context(), endpoint)
	require.NoError(t, err)
	t.Cleanup(release)
	require.NoError(t, db.SetupTestPostgres(endpoint))
	base := closedSecretEndpoint(t)
	const marker = "REGISTER_ENDPOINT_SECRET"
	stdout, stderr, err := command.Run(t, []string{
		config.DATABASE_CONNECTION + "=" + endpoint,
		config.BLOCKCHAIN_HTTP_ENDPOINT + "=" + configuredSecretEndpoint(t, base, marker),
	}, "app", "register", "--name=secret-test", "--address="+secretTestAddress, "--template-path="+t.TempDir())
	require.Error(t, err)
	require.NotContains(t, stdout+stderr, marker)
	require.Contains(t, stderr, "Failed to get template hash from application:")
	require.Contains(t, stderr, base)
}
