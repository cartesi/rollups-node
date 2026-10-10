// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/cartesi/rollups-node/internal/httpclient"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/require"
)

func secretTestClient(t *testing.T, endpoint string, retry bool, logs *bytes.Buffer, options ...rpc.ClientOption) *ethclient.Client {
	t.Helper()
	var client *ethclient.Client
	var err error
	if retry {
		logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		client, err = NewEthClient(t.Context(), endpoint, logger, RetryConfig{RequestTimeout: time.Second}, options...)
	} else {
		client, err = DialEthClient(t.Context(), endpoint)
	}
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}

func TestEthClientAcceptsFirstResponseFrame(t *testing.T) {
	for _, retry := range []bool{false, true} {
		for _, trailing := range []string{` {"another":true}`, ` PROVIDER_TRAILING_DATA`} {
			t.Run(fmt.Sprintf("retry=%v/%s", retry, trailing), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"0x1"}`+trailing)
				}))
				defer server.Close()
				var logs bytes.Buffer
				client := secretTestClient(t, server.URL, retry, &logs)
				chainID, err := client.ChainID(t.Context())
				require.NoError(t, err)
				require.Equal(t, big.NewInt(1), chainID)
			})
		}
	}
}

func TestEthClientStillRejectsMalformedOuterFieldTypes(t *testing.T) {
	const marker = "123456789012345678901234567890"
	for _, field := range []string{"jsonrpc", "method"} {
		for _, retry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/retry=%v", field, retry), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = fmt.Fprintf(w, `{"%s":%s,"id":1,"result":"0x1"}`, field, marker)
				}))
				defer server.Close()
				var logs bytes.Buffer
				client := secretTestClient(t, server.URL+"/"+marker, retry, &logs)
				_, err := client.ChainID(t.Context())
				require.Error(t, err)
				require.Contains(t, err.Error(), "decode RPC response")
				require.Contains(t, err.Error(), server.URL)
				require.NotContains(t, err.Error(), marker)
			})
		}
	}
}

func TestEthClientNonSuccessNonceClassification(t *testing.T) {
	const marker = "NONCE_PROVIDER_CREDENTIAL"
	for _, retry := range []bool{false, true} {
		for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError} {
			for _, body := range []string{
				`{"error":{"code":-32000,"message":"[Nonce Too Low] ` + marker + `"}}`,
				"broadcast rejected: NONCE TOO LOW " + marker,
			} {
				t.Run(fmt.Sprintf("retry=%v/status=%d/%s", retry, status, body[:1]), func(t *testing.T) {
					var requests atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						requests.Add(1)
						w.WriteHeader(status)
						_, _ = io.WriteString(w, body)
					}))
					defer server.Close()
					var logs bytes.Buffer
					client := secretTestClient(t, server.URL+"/"+marker, retry, &logs)
					_, err := client.ChainID(t.Context())
					require.Error(t, err)
					require.Equal(t, !retry || status == http.StatusBadRequest, IsNonceTooLowError(err))
					require.Equal(t, int32(1), requests.Load())
					require.NotContains(t, err.Error(), marker)
				})
			}
		}
	}
}

func TestNewEthClientActuallyRetriesWithProtectedRequests(t *testing.T) {
	const marker = "RETRY_PROVIDER_CREDENTIAL"
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/"+marker+"?key="+marker, r.RequestURI)
		user, password, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "operator", user)
		require.Equal(t, marker, password)
		if attempts.Add(1) == 1 {
			http.Error(w, r.RequestURI+" "+r.Header.Get("Authorization"), http.StatusServiceUnavailable)
			return
		}
		var request struct{ ID json.RawMessage }
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x1"}`, request.ID)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL + "/" + marker + "?key=" + marker)
	require.NoError(t, err)
	endpoint.User = url.UserPassword("operator", marker)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client, err := NewEthClient(t.Context(), endpoint.String(), logger, RetryConfig{
		MaxRetries: 2, RetryMinWait: time.Millisecond, RetryMaxWait: time.Millisecond, RequestTimeout: time.Second,
	})
	require.NoError(t, err)
	defer client.Close()
	chainID, err := client.ChainID(t.Context())
	require.NoError(t, err)
	require.Equal(t, big.NewInt(1), chainID)
	require.Equal(t, int32(2), attempts.Load())
	require.Contains(t, logs.String(), "retrying request")
	require.Contains(t, logs.String(), server.URL)
	require.NotContains(t, logs.String(), marker)
}

func TestNewEthClientDoesNotRetryInvalidHeadersOrCertificates(t *testing.T) {
	const marker = "NONRETRYABLE_CREDENTIAL"
	for _, certificate := range []bool{false, true} {
		t.Run(fmt.Sprint("certificate=", certificate), func(t *testing.T) {
			var requests atomic.Int32
			handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) })
			server := httptest.NewServer(handler)
			if certificate {
				server.Close()
				server = httptest.NewTLSServer(handler)
			}
			defer server.Close()
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			var options []rpc.ClientOption
			if !certificate {
				options = append(options, rpc.WithHeader("X-"+marker, "invalid\nvalue"))
			}
			client, err := NewEthClient(t.Context(), server.URL+"/"+marker, logger, RetryConfig{
				MaxRetries: 2, RetryMinWait: time.Millisecond, RetryMaxWait: time.Millisecond, RequestTimeout: time.Second,
			}, options...)
			require.NoError(t, err)
			defer client.Close()
			_, err = client.ChainID(t.Context())
			require.Error(t, err)
			require.Contains(t, err.Error(), server.URL)
			require.NotContains(t, err.Error(), marker)
			require.NotContains(t, logs.String(), marker)
			require.Equal(t, 1, strings.Count(logs.String(), `"msg":"request failed"`))
			require.Zero(t, requests.Load())
		})
	}
}

func TestEthClientSecretsOnRealFailures(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprint("retry=", retry), func(t *testing.T) {
			var previous string
			for _, marker := range []string{"secret_Ab09_one", "secret_Zx17_two"} {
				var logs bytes.Buffer
				endpoint := "http://user_" + marker + ":pass_" + marker + "@127.0.0.1:1/" + marker + "?key=" + marker
				client := secretTestClient(t, endpoint, retry, &logs)
				_, err := client.ChainID(t.Context())
				require.Error(t, err)
				require.Contains(t, err.Error(), "http://127.0.0.1:1")
				require.NotContains(t, err.Error(), marker)
				require.NotContains(t, logs.String(), marker)
				if retry {
					require.Contains(t, logs.String(), "request failed")
					require.Contains(t, logs.String(), "http://127.0.0.1:1")
				}
				if previous != "" {
					require.Equal(t, previous, err.Error(), "different credentials changed diagnostics")
				}
				previous = err.Error()
			}
		})
	}
}

func TestEthClientProtocolAndEnvelopeDiagnostics(t *testing.T) {
	const marker = "SECRET_Ab09_reflection"
	const payload = `{"jsonrpc":"2.0","id":1,"result":"0x1"}`
	const trailerFormat = "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n" + "%x\r\n%s\r\n0\r\n%s\r\n\r\n"
	for _, retry := range []bool{false, true} {
		for _, tc := range []struct{ name, reply string }{
			{"status", "HTTP/1.1 " + marker + " Nope\r\nContent-Length: 0\r\n\r\n"},
			{"header", "HTTP/1.1 200 OK\r\n" + marker + "\r\nContent-Length: 0\r\n\r\n"},
			{"trailer", fmt.Sprintf(trailerFormat, len(payload), payload, marker)},
		} {
			t.Run(fmt.Sprintf("%s/retry=%v", tc.name, retry), func(t *testing.T) {
				var received atomic.Bool
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					received.Store(strings.Contains(r.RequestURI, marker))
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						return
					}
					defer conn.Close()
					_, _ = io.WriteString(conn, tc.reply)
				}))
				defer server.Close()
				var logs bytes.Buffer
				client := secretTestClient(t, server.URL+"/"+marker+"?key="+marker, retry, &logs)
				_, err := client.ChainID(t.Context())
				require.Error(t, err)
				require.True(t, received.Load())
				require.Contains(t, err.Error(), server.URL)
				require.NotContains(t, err.Error(), marker)
				require.NotContains(t, logs.String(), marker)
			})
		}
		// An invalid JSON character is reflected by go-ethereum's decoder.
		// Even a one-character credential must stay out of its diagnostic.
		const syntaxMarker = "Z"
		t.Run(fmt.Sprint("envelope syntax/retry=", retry), func(t *testing.T) {
			var received atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Store(strings.Contains(r.RequestURI, syntaxMarker))
				_, _ = io.WriteString(w, syntaxMarker)
			}))
			defer server.Close()
			var logs bytes.Buffer
			client := secretTestClient(t, server.URL+"/"+syntaxMarker, retry, &logs)
			_, err := client.ChainID(t.Context())
			require.Error(t, err)
			require.True(t, received.Load())
			require.NotContains(t, err.Error(), syntaxMarker)
			require.Contains(t, err.Error(), "decode RPC response")
			require.Contains(t, err.Error(), server.URL)
		})
	}
}

func TestValidLargeRPCDataRetainsDependencyClassification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32005,"message":"range too large","data":1e999}}`)
	}))
	defer server.Close()
	// go-ethereum stores error as RawMessage and ignores decodeError's conversion
	// error. Preserve that behavior rather than introducing a stricter data type.
	baseline, err := ethclient.DialContext(t.Context(), server.URL)
	require.NoError(t, err)
	defer baseline.Close()
	_, baselineErr := baseline.ChainID(t.Context())
	var baselineData rpc.DataError
	require.ErrorAs(t, baselineErr, &baselineData)
	for _, retry := range []bool{false, true} {
		var logs bytes.Buffer
		client := secretTestClient(t, server.URL+"/SECRET", retry, &logs)
		_, err := client.ChainID(t.Context())
		require.True(t, queryBlockRangeTooLarge(err))
		var rpcError rpc.Error
		var data rpc.DataError
		require.ErrorAs(t, err, &rpcError)
		require.ErrorAs(t, err, &data)
		require.Equal(t, -32005, rpcError.ErrorCode())
		require.Equal(t, baselineData.ErrorData(), data.ErrorData())
		require.Equal(t, baselineErr.Error(), err.Error())
	}
}

func TestEthClientNonSuccessKeepsBlockRangeClassification(t *testing.T) {
	for _, code := range []int{-32047, -32600, -32005, 400} {
		for _, retry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/retry=%v", code, retry), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(400)
					_, _ = fmt.Fprintf(w,
						`{"jsonrpc":"2.0","id":1,"error":{"code":%d,"message":"SECRET password=3","data":"SECRET"}}`, code)
				}))
				defer server.Close()
				var logs bytes.Buffer
				client := secretTestClient(t, server.URL+"/SECRET?password=3", retry, &logs)
				_, err := client.ChainID(t.Context())
				require.Error(t, err)
				// This is deliberately the real direct-type classifier used by Filter.
				require.True(t, queryBlockRangeTooLarge(err))
				var httpError rpc.HTTPError
				require.ErrorAs(t, err, &httpError)
				require.NotContains(t, err.Error(), "SECRET")
				require.NotContains(t, string(httpError.Body), "password=3")
			})
		}
	}
}

func TestConfinedClientActuallyChunksHTTPErrorRanges(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprint(retry), func(t *testing.T) {
			var ranges []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
					Params []struct {
						FromBlock string `json:"fromBlock"`
						ToBlock   string `json:"toBlock"`
					} `json:"params"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.Equal(t, "eth_getLogs", request.Method)
				require.Len(t, request.Params, 1)
				from, err := strconv.ParseUint(strings.TrimPrefix(request.Params[0].FromBlock, "0x"), 16, 64)
				require.NoError(t, err)
				to, err := strconv.ParseUint(strings.TrimPrefix(request.Params[0].ToBlock, "0x"), 16, 64)
				require.NoError(t, err)
				ranges = append(ranges, fmt.Sprintf("%d-%d", from, to))
				if to-from > 1 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32005,"message":"SECRET password=3"}}`, request.ID)
					return
				}
				_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":[]}`, request.ID)
			}))
			defer server.Close()
			var logs bytes.Buffer
			client := secretTestClient(t, server.URL+"/SECRET?password=3", retry, &logs)
			filter := Filter{
				MinChunkSize: big.NewInt(1), MaxChunkSize: big.NewInt(4), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			query := ethereum.FilterQuery{FromBlock: big.NewInt(0), ToBlock: big.NewInt(3)}
			results, err := filter.ChunkedFilterLogs(t.Context(), client, query)
			require.NoError(t, err)
			for _, err := range results {
				require.NoError(t, err)
			}
			require.Equal(t, []string{"0-3", "0-2", "0-1", "2-3"}, ranges)
		})
	}
}

func TestEthClientRejectsRedirectsAndKeepsRevertTypes(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprint(retry), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if strings.Contains(r.URL.Path, "/redirect") {
					w.Header().Set("Location", "http://%SECRET")
					w.WriteHeader(http.StatusTemporaryRedirect)
					return
				}
				_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":3,"message":"execution reverted","data":"0xdeadbeef"}}`)
			}))
			defer server.Close()
			var logs bytes.Buffer
			client := secretTestClient(t, server.URL+"/redirect/SECRET", retry, &logs)
			_, err := client.ChainID(t.Context())
			require.Error(t, err)
			var status rpc.HTTPError
			require.ErrorAs(t, err, &status)
			require.Equal(t, http.StatusTemporaryRedirect, status.StatusCode)
			require.Equal(t, int32(1), requests.Load())
			require.NotContains(t, err.Error(), "SECRET")
			client = secretTestClient(t, server.URL+"/revert/SECRET", retry, &logs)
			_, err = client.ChainID(t.Context())
			var rpcError rpc.Error
			var dataError rpc.DataError
			require.ErrorAs(t, err, &rpcError)
			require.ErrorAs(t, err, &dataError)
			require.Equal(t, 3, rpcError.ErrorCode())
			require.Equal(t, "0xdeadbeef", dataError.ErrorData())
			data, ok := ethclient.RevertErrorData(err)
			require.True(t, ok)
			require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, data)
		})
	}
}

func TestEthClientWireAndOwnedHTTPClient(t *testing.T) {
	for _, auth := range []string{"", "Bearer explicit"} {
		var received atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			received.Store(true)
			require.Equal(t, "/base%2Fkey//./rpc?key=a%2Fb&key=second+", r.RequestURI)
			if auth != "" {
				require.Equal(t, auth, r.Header.Get("Authorization"))
			} else {
				user, password, ok := r.BasicAuth()
				require.True(t, ok)
				require.Equal(t, "user", user)
				require.Equal(t, "3", password)
			}
			var payload struct {
				ID json.RawMessage `json:"id"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x1","error":null}`, payload.ID)
		}))
		endpoint, _ := url.Parse(server.URL + "/base%2Fkey//./rpc?key=a%2Fb&key=second+")
		endpoint.User = url.UserPassword("user", "3")
		var logs bytes.Buffer
		var override atomic.Bool
		injected := &http.Client{Transport: secretRoundTripFunc(func(*http.Request) (*http.Response, error) {
			override.Store(true)
			return nil, errors.New("override must not run")
		})}
		options := []rpc.ClientOption{rpc.WithHTTPClient(injected)}
		if auth != "" {
			options = append(options, rpc.WithHeader("Authorization", auth))
		}
		client := secretTestClient(t, endpoint.String(), true, &logs, options...)
		chain, err := client.ChainID(t.Context())
		require.NoError(t, err)
		require.Equal(t, big.NewInt(1), chain)
		require.True(t, received.Load())
		require.False(t, override.Load())
		server.Close()
	}
}

type secretRoundTripFunc func(*http.Request) (*http.Response, error)

func (f secretRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProtectedRetryPolicyMatchesOriginalPolicy(t *testing.T) {
	for _, cause := range []error{
		errors.New("net/http: invalid header field value for SECRET"),
		&tls.CertificateVerificationError{Err: errors.New("SECRET certificate")},
		errors.New("ordinary SECRET network failure"),
		context.Canceled, context.DeadlineExceeded,
	} {
		original := &url.Error{Op: http.MethodPost, URL: "http://host/SECRET", Err: cause}
		protected := &url.Error{
			Op: http.MethodPost, URL: "http://host",
			Err: httpclient.Diagnostic(t.Context(), cause, "HTTP request", "http://host"),
		}
		want, wantErr := retryablehttp.DefaultRetryPolicy(context.Background(), nil, original)
		got, gotErr := protectedRetryPolicy(context.Background(), nil, protected)
		require.Equal(t, want, got)
		require.Equal(t, wantErr, gotErr)
		require.NotContains(t, protected.Error(), "SECRET")
	}
	for _, cause := range []error{context.Canceled, errors.Join(context.Canceled, errors.New("SECRET failure"))} {
		var logs bytes.Buffer
		original := &http.Client{Transport: secretRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, cause })}
		_, inner, err := httpclient.NewClient("http://host/SECRET", original, httpclient.Options{})
		require.NoError(t, err)
		rclient := retryablehttp.NewClient()
		rclient.RetryMax = 0
		rclient.HTTPClient = inner
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		rclient.Logger = &retryLeveledLogger{logger: logger}
		rclient.CheckRetry = protectedRetryPolicy
		_, err = rclient.StandardClient().Get("http://host")
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, errutil.IsOnlyCancellation(cause), errutil.IsOnlyCancellation(err))
		require.NotContains(t, err.Error(), "SECRET")
		require.NotContains(t, logs.String(), "SECRET")
	}
}

func TestEthConstructorsRejectNonHTTP(t *testing.T) {
	for _, endpoint := range []string{"SECRET.sock", "wss://host/SECRET", "https:///SECRET", "http://host/%SECRET"} {
		_, err := DialEthClient(t.Context(), endpoint)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "SECRET")
		_, err = NewEthClient(t.Context(), endpoint, slog.Default(), RetryConfig{})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "SECRET")
	}
}
