// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var clientRequestIDSuffix = regexp.MustCompile(` \(request_id=([^()]+)\)$`)

func withoutClientRequestID(t *testing.T, message string) string {
	t.Helper()
	match := clientRequestIDSuffix.FindStringSubmatch(message)
	require.Len(t, match, 2, "failure must include the client-generated request ID")
	id, err := uuid.Parse(match[1])
	require.NoError(t, err)
	require.Equal(t, id.String(), match[1])
	return strings.TrimSuffix(message, match[0])
}

func TestClientFailureRequestIDMatchesNodeAndRejectsResponseID(t *testing.T) {
	for _, hostile := range []bool{false, true} {
		t.Run(fmt.Sprint("hostile=", hostile), func(t *testing.T) {
			observed := make(chan string, 1)
			server := httptest.NewServer(service.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id := service.RequestIDFromContext(r.Context())
				require.Equal(t, r.Header.Get("X-Request-ID"), id)
				_, err := uuid.Parse(id)
				require.NoError(t, err)
				observed <- id
				if hostile {
					w.Header().Set("X-Request-ID", endpointSecret)
					http.Error(w, "service at capacity (request_id="+endpointSecret+")", http.StatusServiceUnavailable)
					return
				}
				service.WriteInternalError(r.Context(), w, slog.New(slog.NewTextHandler(io.Discard, nil)), errors.New(endpointSecret))
			})))
			defer server.Close()
			err := NewClient(server.URL+"/rpc?key="+endpointSecret).Call(t.Context(), "test", nil, nil)
			require.Error(t, err)
			select {
			case id := <-observed:
				require.Contains(t, err.Error(), " (request_id="+id+")")
			default:
				t.Fatal("the node did not receive the request ID")
			}
			require.NotContains(t, err.Error(), endpointSecret)
			require.Contains(t, err.Error(), server.URL)
			withoutClientRequestID(t, err.Error())
		})
	}
}

func TestClientZoneEndpointReachesTransportWithFullCredentials(t *testing.T) {
	c := NewClient("http://operator:" + endpointSecret + "@[::1%25lo0]:1/provider%2Frpc?key=" + endpointSecret)
	var calls int
	c.HTTPClient = &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "[::1%lo0]:1", req.URL.Host)
		require.Equal(t, "/provider%2Frpc?key="+endpointSecret, req.URL.RequestURI())
		user, password, ok := req.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "operator", user)
		require.Equal(t, endpointSecret, password)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":1}`))}, nil
	})}
	var result int
	require.NoError(t, c.Call(t.Context(), "test", nil, &result))
	require.Equal(t, 1, calls)
	require.Equal(t, 1, result)
}

func TestClientEnvelopeDecodeErrorsHaveNoRawEndpoint(t *testing.T) {
	for _, body := range []string{
		`{"jsonrpc":1234567890123456789012345678901234567890,"id":1,"result":0}`,
		`{"jsonrpc":"2.0","id":1234567890123456789012345678901234567890,"result":0}`,
		"",
	} {
		t.Run(body, func(t *testing.T) {
			c := NewClient("http://operator:" + endpointSecret + "@example.invalid/private?key=" + endpointSecret)
			c.HTTPClient = &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			err := c.Call(t.Context(), "test", nil, nil)
			require.Error(t, err)
			require.Contains(t, err.Error(), "decode JSON-RPC response")
			require.Contains(t, err.Error(), "http://example.invalid")
			require.NotContains(t, err.Error(), endpointSecret)
			require.NotContains(t, err.Error(), "1234567890123456789012345678901234567890")
			if body == "" {
				require.Contains(t, err.Error(), "empty response")
				require.ErrorIs(t, err, io.EOF)
			} else {
				var cause *json.UnmarshalTypeError
				require.ErrorAs(t, err, &cause)
			}
		})
	}
}

func TestClientLocalFailuresHaveNoRequestIDOrDispatch(t *testing.T) {
	for _, test := range []struct {
		name, endpoint string
		params         any
		context        func() context.Context
	}{
		{name: "configuration", endpoint: "http://operator:" + endpointSecret + "@example.invalid:%bad"},
		{name: "marshal", params: make(chan int)},
		{name: "request construction", context: func() context.Context { return nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			endpoint := test.endpoint
			if endpoint == "" {
				endpoint = "http://operator:" + endpointSecret + "@example.invalid/private?key=" + endpointSecret
			}
			c := NewClient(endpoint)
			var calls int
			c.HTTPClient = &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("unexpected request")
			})}
			ctx := t.Context()
			if test.context != nil {
				ctx = test.context()
			}
			err := c.Call(ctx, "test", test.params, nil)
			require.Error(t, err)
			require.Zero(t, calls)
			require.NotContains(t, err.Error(), "request_id")
			require.NotContains(t, err.Error(), endpointSecret)
			if test.name == "marshal" {
				var cause *json.UnsupportedTypeError
				require.ErrorAs(t, err, &cause)
			}
		})
	}
}

type deadlineResponseBody struct{ ctx context.Context }

func (b deadlineResponseBody) Read([]byte) (int, error) {
	<-b.ctx.Done()
	return 0, fmt.Errorf("%s: %w", endpointSecret, b.ctx.Err())
}

func (deadlineResponseBody) Close() error { return nil }

type deadlineResult struct{ ctx context.Context }

func (r deadlineResult) UnmarshalJSON([]byte) error {
	<-r.ctx.Done()
	return fmt.Errorf("%s: %w", endpointSecret, r.ctx.Err())
}

func TestClientDecodeDiagnosticsUseCallerDeadline(t *testing.T) {
	for _, decodeResult := range []bool{false, true} {
		t.Run(fmt.Sprint("result=", decodeResult), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			c := NewClient("http://operator:" + endpointSecret + "@example.invalid/private?key=" + endpointSecret)
			var calls int
			c.HTTPClient = &http.Client{Transport: testTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				var body io.ReadCloser = deadlineResponseBody{ctx: req.Context()}
				if decodeResult {
					body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":1}`))
				}
				return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
			})}
			var result any
			if decodeResult {
				result = &deadlineResult{ctx: ctx}
			}
			err := c.Call(ctx, "test", nil, result)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Equal(t, 1, calls)
			operation := "decode JSON-RPC response"
			if decodeResult {
				operation = "decode JSON-RPC result"
			}
			require.Contains(t, err.Error(), operation+" failed for http://example.invalid: context deadline exceeded")
			require.NotContains(t, err.Error(), endpointSecret)
			withoutClientRequestID(t, err.Error())
		})
	}
}

func TestClientRPCFailuresDoNotRetainInspectMessages(t *testing.T) {
	c := NewClient("http://example.invalid/rpc?key=" + endpointSecret)
	c.HTTPClient = &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable,
			Body: io.NopCloser(strings.NewReader("Machine not ready\n"))}, nil
	})}
	err := c.Call(t.Context(), "test", nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "503 Service Unavailable")
	require.Contains(t, err.Error(), "HTTP request failed")
	require.NotContains(t, err.Error(), "Machine not ready")
}
