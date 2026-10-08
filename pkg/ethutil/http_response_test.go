// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func responseTestClient(t *testing.T, endpoint string, retry bool, logs *bytes.Buffer) *ethclient.Client {
	t.Helper()
	if !retry {
		return secretTestClient(t, endpoint, false, logs)
	}
	client, err := NewEthClient(t.Context(), endpoint, slog.New(slog.NewTextHandler(logs, nil)), RetryConfig{
		MaxRetries: 2, RetryMinWait: time.Millisecond, RetryMaxWait: time.Millisecond, RequestTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}

func TestTruncatedHTTPErrorBodyPreservesRetryDecision(t *testing.T) {
	const marker = "TRUNCATED_RESPONSE_CREDENTIAL"
	for _, retry := range []bool{false, true} {
		for _, status := range []int{http.StatusBadRequest, http.StatusServiceUnavailable} {
			t.Run(fmt.Sprintf("retry=%v/status=%d", retry, status), func(t *testing.T) {
				var attempts atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempts.Add(1)
					require.Equal(t, "/"+marker+"?key="+marker, r.RequestURI)
					user, password, ok := r.BasicAuth()
					require.True(t, ok)
					require.Equal(t, "operator", user)
					require.Equal(t, marker, password)
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					defer conn.Close()
					body := "nonce too low " + r.RequestURI + " " + r.Header.Get("Authorization")
					_, _ = fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\nContent-Length: %d\r\n\r\n%s",
						status, http.StatusText(status), len(body)+10, body)
				}))
				defer server.Close()
				endpoint, err := url.Parse(server.URL + "/" + marker + "?key=" + marker)
				require.NoError(t, err)
				endpoint.User = url.UserPassword("operator", marker)
				var logs bytes.Buffer
				client := responseTestClient(t, endpoint.String(), retry, &logs)
				_, err = client.ChainID(t.Context())
				require.Error(t, err)
				require.NotContains(t, err.Error(), marker)
				require.NotContains(t, logs.String(), marker)
				require.False(t, IsNonceTooLowError(err), "partial diagnostic bodies must be discarded")
				wantAttempts := int32(1)
				if retry && status == http.StatusServiceUnavailable {
					wantAttempts = 3
					require.Contains(t, err.Error(), server.URL)
				} else {
					var httpError rpc.HTTPError
					require.ErrorAs(t, err, &httpError)
					require.Equal(t, status, httpError.StatusCode)
					require.Equal(t, "HTTP request failed", string(httpError.Body))
				}
				require.Equal(t, wantAttempts, attempts.Load())
			})
		}
	}
}

func TestHTTPErrorBodyReadPreservesCanceledContext(t *testing.T) {
	for _, retry := range []bool{false, true} {
		for _, deadline := range []bool{false, true} {
			t.Run(fmt.Sprintf("retry=%v/deadline=%v", retry, deadline), func(t *testing.T) {
				ready := make(chan struct{}, 1)
				var attempts atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempts.Add(1)
					w.Header().Set("Content-Length", "1000")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, "nonce too low CREDENTIAL")
					w.(http.Flusher).Flush()
					ready <- struct{}{}
					<-r.Context().Done()
				}))
				defer server.Close()
				var logs bytes.Buffer
				client := responseTestClient(t, server.URL+"/CREDENTIAL", retry, &logs)
				ctx, cancel := context.WithCancel(t.Context())
				want := context.Canceled
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(t.Context(), time.Second)
					want = context.DeadlineExceeded
				}
				defer cancel()
				result := make(chan error, 1)
				go func() {
					_, err := client.ChainID(ctx)
					result <- err
				}()
				select {
				case <-ready:
				case <-time.After(5 * time.Second):
					t.Fatal("response headers were not received")
				}
				if !deadline {
					cancel()
				}
				err := <-result
				require.ErrorIs(t, err, want)
				require.NotContains(t, err.Error(), "CREDENTIAL")
				require.NotContains(t, logs.String(), "CREDENTIAL")
				require.Equal(t, int32(1), attempts.Load())
			})
		}
	}
}

func TestClientTimeoutOnErrorBodyPreservesRetryDecisionAndCause(t *testing.T) {
	const (
		marker         = "CLIENT_TIMEOUT_CREDENTIAL"
		maxRetries     = 2
		requestTimeout = 100 * time.Millisecond
	)
	for _, status := range []int{http.StatusBadRequest, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				require.Equal(t, "/"+marker+"?key="+marker, r.RequestURI)
				user, password, ok := r.BasicAuth()
				require.True(t, ok)
				require.Equal(t, "operator", user)
				require.Equal(t, marker, password)
				w.Header().Set("Content-Length", "1000000")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "nonce too low "+marker)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			endpoint, err := url.Parse(server.URL + "/" + marker + "?key=" + marker)
			require.NoError(t, err)
			endpoint.User = url.UserPassword("operator", marker)
			var logs bytes.Buffer
			client, err := NewEthClient(t.Context(), endpoint.String(), slog.New(slog.NewTextHandler(&logs, nil)), RetryConfig{
				MaxRetries: maxRetries, RetryMinWait: time.Millisecond, RetryMaxWait: time.Millisecond, RequestTimeout: requestTimeout,
			})
			require.NoError(t, err)
			defer client.Close()
			_, err = client.ChainID(t.Context())
			require.Error(t, err)
			require.Contains(t, err.Error(), server.URL)
			require.NotContains(t, err.Error(), marker)
			require.NotContains(t, logs.String(), marker)
			require.False(t, IsNonceTooLowError(err), "partial diagnostic text must be discarded")
			if status == http.StatusBadRequest {
				require.Equal(t, int32(1), attempts.Load())
				require.ErrorIs(t, err, context.DeadlineExceeded)
				var timeout net.Error
				require.ErrorAs(t, err, &timeout)
				require.True(t, timeout.Timeout())
				require.Contains(t, err.Error(), "read RPC response")
				require.Contains(t, err.Error(), ": timeout")
				require.NotContains(t, err.Error(), "context deadline exceeded", "the caller context is still active")
			} else {
				require.Equal(t, int32(maxRetries+1), attempts.Load())
				require.NotErrorIs(t, err, context.DeadlineExceeded, "exhausted 5xx status retries discard the body cause on baseline")
			}
		})
	}
}

func TestEthClientDoesNotRetainOtherServiceErrorMessages(t *testing.T) {
	const message = "Machine not ready"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, message, http.StatusServiceUnavailable)
	}))
	defer server.Close()
	var logs bytes.Buffer
	client := responseTestClient(t, server.URL+"/CREDENTIAL", false, &logs)
	_, err := client.ChainID(t.Context())
	var httpError rpc.HTTPError
	require.ErrorAs(t, err, &httpError)
	require.Equal(t, http.StatusServiceUnavailable, httpError.StatusCode)
	require.Equal(t, "HTTP request failed", string(httpError.Body))
}

func TestEthClientEmptyResponseHasDecodeLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	for _, retry := range []bool{false, true} {
		var logs bytes.Buffer
		client := responseTestClient(t, server.URL+"/CREDENTIAL", retry, &logs)
		_, err := client.ChainID(t.Context())
		require.ErrorIs(t, err, io.EOF)
		require.Contains(t, err.Error(), "empty response")
		require.Contains(t, err.Error(), server.URL)
		require.NotContains(t, err.Error(), "connection closed")
		require.NotContains(t, err.Error(), "CREDENTIAL")
	}
}

func TestEthClientZoneScopedEndpointConstructs(t *testing.T) {
	for _, retry := range []bool{false, true} {
		var logs bytes.Buffer
		_ = responseTestClient(t, "http://user:password@[::1%25lo0]:8545/CREDENTIAL?key=CREDENTIAL", retry, &logs)
	}
}

func TestEthClientBatchAcceptsFirstResponseFrame(t *testing.T) {
	for _, retry := range []bool{false, true} {
		for _, trailing := range []string{` {"another":true}`, ` PROVIDER_TRAILING_DATA`} {
			t.Run(fmt.Sprintf("retry=%v/%s", retry, trailing), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var requests []struct{ ID json.RawMessage }
					require.NoError(t, json.NewDecoder(r.Body).Decode(&requests))
					require.Len(t, requests, 1)
					_, _ = fmt.Fprintf(w, `[{"jsonrpc":"2.0","id":%s,"result":"0x1"}]%s`, requests[0].ID, trailing)
				}))
				defer server.Close()
				var logs bytes.Buffer
				client := responseTestClient(t, server.URL+"/CREDENTIAL", retry, &logs)
				var result string
				batch := []rpc.BatchElem{{Method: "eth_chainId", Result: &result}}
				require.NoError(t, client.Client().BatchCallContext(t.Context(), batch))
				require.NoError(t, batch[0].Error)
				require.Equal(t, "0x1", result)
			})
		}
	}
}
