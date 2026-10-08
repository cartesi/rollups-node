// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const endpointSecret = "credential-0123456789abcdef0123456789abcdef" //nolint:gosec // Synthetic credential for leak regression tests.

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestClientConfinesEndpointAndPreservesRequests(t *testing.T) {
	var ids []uint64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/provider%2Frpc//?key="+endpointSecret+"&key=second+", r.RequestURI)
		username, password, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "operator", username)
		require.Equal(t, endpointSecret, password)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		var request rpcRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "cartesi_getApplication", request.Method)
		ids = append(ids, request.ID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"value":%q}}`, request.ID, endpointSecret)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL + "/provider%2Frpc//?key=" + endpointSecret + "&key=second+")
	require.NoError(t, err)
	endpoint.User = url.UserPassword("operator", endpointSecret)
	c := NewClient(endpoint.String())
	require.Equal(t, endpoint.String(), c.URL, "the public configuration API remains compatible")
	for range 2 {
		var result map[string]string
		require.NoError(t, c.Call(t.Context(), "cartesi_getApplication", map[string]string{"application": "echo"}, &result))
		require.Equal(t, endpointSecret, result["value"], "successful results must remain intact")
	}
	require.Equal(t, []uint64{1, 2}, ids)
}

func TestClientSupportsInjectedHTTPClientAndEndpointReplacement(t *testing.T) {
	c := NewClient("http://example.invalid/initial?key=" + endpointSecret)
	c.URL = "http://replacement.invalid/replacement?key=" + endpointSecret
	var seen bool
	original := &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
		seen = true
		require.Equal(t, c.URL, req.URL.String())
		require.Equal(t, "replacement.invalid", req.Host)
		return &http.Response{StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":1}`))}, nil
	})}
	c.HTTPClient = original
	var result int
	require.NoError(t, c.Call(t.Context(), "test", nil, &result))
	require.Equal(t, 1, result)
	require.True(t, seen)
	require.Same(t, original, c.HTTPClient)
	require.Nil(t, original.CheckRedirect, "protecting requests must not mutate the injected client")
}

func TestClientCanRemoveSameHostCredentialsThroughURL(t *testing.T) {
	c := NewClient("http://operator:" + endpointSecret + "@example.invalid/private?key=" + endpointSecret)
	c.URL = "http://example.invalid"
	c.HTTPClient = &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "http://example.invalid", req.URL.String())
		require.Empty(t, req.Header.Get("Authorization"))
		return &http.Response{StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":1}`))}, nil
	})}
	require.NoError(t, c.Call(t.Context(), "test", nil, nil))
}

func TestClientFailureOutputDoesNotDependOnCredentials(t *testing.T) {
	outputs := make([]string, 0, 2)
	for _, secret := range []string{"secret-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "secret-BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"} {
		c := NewClient("http://operator:" + secret + "@example.invalid/" + secret + "?key=" + secret)
		c.HTTPClient = &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("provider failed for %s", req.URL)
		})}
		err := c.Call(t.Context(), "test", nil, nil)
		require.Error(t, err)
		require.NotContains(t, err.Error(), secret)
		require.Contains(t, err.Error(), "http://example.invalid")
		outputs = append(outputs, withoutClientRequestID(t, err.Error()))
	}
	require.Equal(t, outputs[0], outputs[1])
}

func TestClientFailuresHavePublicEndpointAndNoCredentials(t *testing.T) {
	for _, status := range []int{http.StatusAccepted, http.StatusBadRequest, http.StatusInternalServerError, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "http://example.invalid/"+endpointSecret+"/%bad")
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, endpointSecret)
			}))
			defer server.Close()
			c := NewClient(server.URL + "/" + endpointSecret + "?key=" + endpointSecret)
			err := c.Call(t.Context(), "test", nil, nil)
			require.Error(t, err)
			require.NotContains(t, err.Error(), endpointSecret)
			require.Contains(t, err.Error(), server.URL)
		})
	}
	t.Run("invalid endpoint", func(t *testing.T) {
		c := NewClient("http://operator:" + endpointSecret + "@example.invalid:" + endpointSecret)
		err := c.Call(t.Context(), "test", nil, nil)
		require.Error(t, err)
		require.NotContains(t, err.Error(), endpointSecret)
	})
	t.Run("malformed result", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":1234567890123456789012345678901234567890}`)
		}))
		defer server.Close()
		c := NewClient(server.URL + "/?key=1234567890123456789012345678901234567890")
		var result int64
		err := c.Call(t.Context(), "test", nil, &result)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "1234567890123456789012345678901234567890")
		require.Contains(t, err.Error(), server.URL)
		var typeError *json.UnmarshalTypeError
		require.ErrorAs(t, err, &typeError)
	})
}

func TestClientProtocolUpgradeDoesNotExposeStream(t *testing.T) {
	observed := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- r.RequestURI
		user, password, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "operator", user)
		require.Equal(t, endpointSecret, password)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n%s %s",
			r.RequestURI, r.Header.Get("Authorization"))
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL + "/" + endpointSecret + "?key=" + endpointSecret)
	require.NoError(t, err)
	endpoint.User = url.UserPassword("operator", endpointSecret)
	err = NewClient(endpoint.String()).Call(t.Context(), "test", nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "101 Switching Protocols")
	require.Contains(t, err.Error(), server.URL)
	require.NotContains(t, err.Error(), endpointSecret)
	require.NotContains(t, err.Error(), "Basic ")
	require.Equal(t, "/"+endpointSecret+"?key="+endpointSecret, <-observed)
}

func TestClientCancellationAndRPCErrorRemainClassifiable(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		c := NewClient("http://127.0.0.1:1/" + endpointSecret)
		err := c.Call(ctx, "test", nil, nil)
		require.ErrorIs(t, err, context.Canceled)
		require.NotContains(t, err.Error(), endpointSecret)
	})
	t.Run("RPC error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32005,"message":"block range","data":"0x1234"}}`)
		}))
		defer server.Close()
		c := NewClient(server.URL + "/?key=3")
		err := c.Call(t.Context(), "test", nil, nil)
		var rpcErr *rpcError
		require.True(t, errors.As(err, &rpcErr))
		require.Equal(t, -32005, rpcErr.Code)
		require.Equal(t, "block range", rpcErr.Message)
		require.Equal(t, "0x1234", rpcErr.Data)
	})
}

func TestClientConcurrentRequestsHaveDistinctIDs(t *testing.T) {
	const requests = 16
	var mu sync.Mutex
	ids := make(map[uint64]bool)
	requestIDs := make(map[string]bool)
	c := NewClient("http://example.invalid/rpc?key=" + endpointSecret)
	c.HTTPClient = &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
		var request rpcRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			return nil, err
		}
		requestID := req.Header.Get("X-Request-ID")
		if _, err := uuid.Parse(requestID); err != nil {
			return nil, fmt.Errorf("invalid request ID: %w", err)
		}
		mu.Lock()
		ids[request.ID] = true
		requestIDs[requestID] = true
		mu.Unlock()
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(
			fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":1}`, request.ID)))}, nil
	})}
	var wg sync.WaitGroup
	failures := make(chan error, requests)
	for range requests {
		wg.Go(func() { failures <- c.Call(t.Context(), "test", nil, nil) })
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	require.Len(t, ids, requests)
	require.Len(t, requestIDs, requests)
}
