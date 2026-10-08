// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/cartesi/rollups-node/test/tooling/command"
	"github.com/cartesi/rollups-node/test/tooling/db"
)

const secretTestAddress = "0x0000000000000000000000000000000000000001"
const secretContractCommand = "contract"
const secretSummaryCommand = "summary"
const secretApplicationName = "echo-dapp"
const secretInspectCommand = "inspect"
const secretReadCommand = "read"
const jsonRPCFlag = "--jsonrpc"
const readInputsOperation = "inputs"

var secretRequestIDSuffix = regexp.MustCompile(` \(request_id=([^()\r\n]+)\)\n?$`)

func withoutSecretRequestID(t *testing.T, output string) string {
	t.Helper()
	match := secretRequestIDSuffix.FindStringSubmatch(output)
	require.Len(t, match, 2, "request failure must print its local ID")
	id, err := uuid.Parse(match[1])
	require.NoError(t, err)
	require.Equal(t, id.String(), match[1])
	require.Equal(t, uuid.Version(4), id.Version())
	return strings.TrimSuffix(output, match[0])
}

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
		{name: secretReadCommand, args: []string{secretReadCommand, readInputsOperation, secretApplicationName, jsonRPCFlag}},
		{name: secretInspectCommand, args: []string{secretInspectCommand, secretApplicationName, "hi"}},
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
				if test.name == secretReadCommand || test.name == secretInspectCommand {
					output = withoutSecretRequestID(t, output)
				}
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
		require.Equal(t, "/"+marker+"?key="+marker, r.RequestURI)
		user, password, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, marker, user)
		require.Equal(t, "password", password)
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

func TestSecretReadFragmentDoesNotExposeUserinfo(t *testing.T) {
	const marker = "FRAGMENT_USERINFO_SECRET"
	base := closedSecretEndpoint(t)
	endpoint, err := url.Parse(base + "#fragment")
	require.NoError(t, err)
	endpoint.User = url.UserPassword(marker, marker)
	stdout, stderr, err := command.Run(t, []string{config.JSONRPC_API_URL + "=" + endpoint.String()},
		secretReadCommand, readInputsOperation, secretApplicationName, jsonRPCFlag)
	require.Error(t, err)
	require.NotContains(t, stdout+stderr, marker)
	require.Contains(t, stdout+stderr, base)
	require.Contains(t, stdout+stderr, "connection refused")
}

func TestSecretCLIReadAndInspectPreserveWireCredentials(t *testing.T) {
	const marker = "CLI_WIRE_CREDENTIAL"
	for _, test := range []struct {
		name, key, operation, status string
		args                         []string
	}{
		{secretReadCommand, config.JSONRPC_API_URL, "", ": 500 Internal Server Error, body:",
			[]string{secretReadCommand, readInputsOperation, secretApplicationName, jsonRPCFlag}},
		{secretInspectCommand, config.INSPECT_URL, "/inspect/echo-dapp", "with status 500:",
			[]string{secretInspectCommand, secretApplicationName, "hi"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			requestIDs := make(chan string, 1)
			server := httptest.NewServer(service.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				requestID := service.RequestIDFromContext(r.Context())
				require.Equal(t, r.Header.Get("X-Request-ID"), requestID)
				_, err := uuid.Parse(requestID)
				require.NoError(t, err)
				requestIDs <- requestID
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/base%2F"+marker+test.operation+"?key="+marker+"&key=second+", r.RequestURI)
				user, password, ok := r.BasicAuth()
				require.True(t, ok)
				require.Equal(t, "operator", user)
				require.Equal(t, marker, password)
				if test.name == secretReadCommand {
					var request struct{ Method string }
					require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
					require.Equal(t, "cartesi_listInputs", request.Method)
				}
				w.Header().Set("X-Request-ID", marker)
				http.Error(w, r.RequestURI+" "+r.Header.Get("Authorization")+" (request_id="+marker+")", http.StatusInternalServerError)
			})))
			defer server.Close()
			endpoint, err := url.Parse(server.URL + "/base%2F" + marker + "?key=" + marker + "&key=second+")
			require.NoError(t, err)
			endpoint.User = url.UserPassword("operator", marker)
			stdout, stderr, err := command.Run(t, []string{test.key + "=" + endpoint.String()}, test.args...)
			require.Error(t, err)
			require.Equal(t, int32(1), requests.Load())
			require.NotContains(t, stdout+stderr, marker)
			require.Contains(t, stdout+stderr, test.status)
			select {
			case id := <-requestIDs:
				require.Contains(t, stdout+stderr, " (request_id="+id+")")
			default:
				t.Fatal("the node did not receive the request ID")
			}
		})
	}
}

func TestSecretCLIRequestIDsDoNotChangeSuccessOutput(t *testing.T) {
	for _, test := range []struct {
		name, key, response, output string
		args                        []string
	}{
		{secretReadCommand, config.JSONRPC_API_URL, `{"jsonrpc":"2.0","id":1,"result":{"data":[]}}`, "{\n    \"data\": []\n}\n",
			[]string{secretReadCommand, readInputsOperation, secretApplicationName, jsonRPCFlag}},
		{secretInspectCommand, config.INSPECT_URL,
			`{"status":"Accepted","reports":[]}`, "{\n    \"status\": \"Accepted\",\n    \"reports\": []\n}",
			[]string{secretInspectCommand, secretApplicationName, "hi"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			observed := make(chan string, 1)
			server := httptest.NewServer(service.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id := service.RequestIDFromContext(r.Context())
				_, err := uuid.Parse(id)
				require.NoError(t, err)
				observed <- id
				_, _ = io.WriteString(w, test.response)
			})))
			defer server.Close()
			stdout, stderr, err := command.Run(t, []string{test.key + "=" + server.URL}, test.args...)
			require.NoError(t, err, stderr)
			require.Equal(t, test.output, stdout)
			select {
			case id := <-observed:
				require.NotContains(t, stdout+stderr, id)
			default:
				t.Fatal("the node did not receive the request ID")
			}
			require.NotContains(t, stdout+stderr, "request_id")
		})
	}
}

func TestSecretCLIDecodeFailuresKeepLocalRequestID(t *testing.T) {
	const marker = "CLI_DECODE_CREDENTIAL"
	for _, test := range []struct {
		name, key string
		args      []string
	}{
		{secretReadCommand, config.JSONRPC_API_URL, []string{secretReadCommand, readInputsOperation, secretApplicationName, jsonRPCFlag}},
		{secretInspectCommand, config.INSPECT_URL, []string{secretInspectCommand, secretApplicationName, "hi"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			observed := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				observed <- r.Header.Get("X-Request-ID")
				w.Header().Set("X-Request-ID", marker)
				_, _ = io.WriteString(w, marker)
			}))
			defer server.Close()
			stdout, stderr, err := command.Run(t, []string{test.key + "=" + configuredSecretEndpoint(t, server.URL, marker)}, test.args...)
			require.Error(t, err)
			output := stdout + stderr
			require.Contains(t, output, "decode")
			require.Contains(t, output, server.URL)
			select {
			case id := <-observed:
				require.Contains(t, output, " (request_id="+id+")")
			default:
				t.Fatal("the node did not receive the request ID")
			}
			require.NotContains(t, output, marker)
			withoutSecretRequestID(t, output)
		})
	}
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
