// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspectclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const inspectSecret = "inspect-0123456789abcdef0123456789abcdef"

type inspectTransport func(*http.Request) (*http.Response, error)

func (f inspectTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestSafeClientPreservesInspectBasePathQueryAndAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/provider%2Froute//inspect/echo?key="+inspectSecret+"&key=second+", req.RequestURI)
		user, password, ok := req.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "operator", user)
		require.Equal(t, inspectSecret, password)
		require.Equal(t, "application/octet-stream", req.Header.Get("Content-Type"))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, "input", string(body))
		_, _ = fmt.Fprint(w, inspectSecret)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL + "/provider%2Froute//?key=" + inspectSecret + "&key=second+")
	require.NoError(t, err)
	endpoint.User = url.UserPassword("operator", inspectSecret)
	c, err := NewSafeClient(endpoint.String())
	require.NoError(t, err)
	require.Equal(t, server.URL, c.Endpoint())
	resp, err := c.InspectPostWithBody(t.Context(), "echo", "application/octet-stream", strings.NewReader("input"))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, inspectSecret, string(body), "successful inspect results must not be scrubbed")
	require.Equal(t, server.URL+"/inspect/echo", resp.Request.URL.String())
}

func TestSafeClientPreservesInjectedClientAndRequestEditors(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(t.Context(), contextKey{}, "request")
	original := &http.Client{Transport: inspectTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "request", req.Context().Value(contextKey{}))
		require.Equal(t, "/base/inspect/echo?key="+inspectSecret, req.URL.RequestURI())
		require.Equal(t, "Bearer explicit", req.Header.Get("Authorization"))
		require.Equal(t, "edited", req.Header.Get("X-Request"))
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	c, err := NewSafeClient("http://user:"+inspectSecret+"@example.invalid/base?key="+inspectSecret,
		WithHTTPClient(original), WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			require.NotContains(t, req.URL.String(), inspectSecret)
			req.Header.Set("Authorization", "Bearer explicit")
			req.Header.Set("X-Request", "edited")
			return nil
		}))
	require.NoError(t, err)
	resp, err := c.InspectPostWithBody(ctx, "echo", "application/octet-stream", strings.NewReader("input"))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.NotSame(t, original, c.client.Client)
	require.Nil(t, original.CheckRedirect)
}

func TestSafeClientFailuresKeepPublicEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://example.invalid/"+inspectSecret+"/%bad")
		w.WriteHeader(http.StatusTemporaryRedirect)
		_, _ = fmt.Fprint(w, inspectSecret)
	}))
	defer server.Close()
	c, err := NewSafeClient(server.URL + "/" + inspectSecret + "?key=" + inspectSecret)
	require.NoError(t, err)
	resp, err := c.InspectPostWithBody(t.Context(), "echo", "application/octet-stream", strings.NewReader("input"))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	require.Empty(t, resp.Header.Get("Location"))
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), inspectSecret)
	require.Contains(t, resp.Request.URL.String(), server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = c.InspectPostWithBody(ctx, "echo", "application/octet-stream", strings.NewReader("input"))
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, err.Error(), inspectSecret)
	require.Contains(t, err.Error(), server.URL)
}

type unsupportedDoer struct{}

func (unsupportedDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("must not be called")
}

func TestSafeClientRejectsUnsafeConfigurationAndSanitizesEditorErrors(t *testing.T) {
	_, err := NewSafeClient("http://example.invalid/"+inspectSecret, WithHTTPClient(unsupportedDoer{}))
	require.Error(t, err)
	require.NotContains(t, err.Error(), inspectSecret)
	require.Contains(t, err.Error(), "http://example.invalid")
	_, err = NewSafeClient("http://user:" + inspectSecret + "@example.invalid:" + inspectSecret)
	require.Error(t, err)
	require.NotContains(t, err.Error(), inspectSecret)
	cause := errors.New(inspectSecret)
	c, err := NewSafeClient("http://example.invalid/"+inspectSecret,
		WithRequestEditorFn(func(context.Context, *http.Request) error { return cause }))
	require.NoError(t, err)
	_, err = c.InspectPostWithBody(t.Context(), "echo", "application/octet-stream", strings.NewReader("input"))
	require.ErrorIs(t, err, cause)
	require.NotContains(t, err.Error(), inspectSecret)
	require.Contains(t, err.Error(), "http://example.invalid")
	c, err = NewSafeClient("http://example.invalid/" + inspectSecret)
	require.NoError(t, err)
	_, err = c.InspectPostWithBody(t.Context(), "echo", "application/octet-stream", strings.NewReader("input"),
		func(context.Context, *http.Request) error { return cause })
	require.ErrorIs(t, err, cause)
	require.NotContains(t, err.Error(), inspectSecret)
	require.Contains(t, err.Error(), "http://example.invalid")
}

func TestSafeClientSupportsBaseURLOptionWithoutLosingQuery(t *testing.T) {
	original := &http.Client{Transport: inspectTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "http://replacement.invalid/base/inspect/echo?key="+inspectSecret, req.URL.String())
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	c, err := NewSafeClient("http://example.invalid/ignored", WithHTTPClient(original),
		WithBaseURL("http://replacement.invalid/base?key="+inspectSecret))
	require.NoError(t, err)
	require.Equal(t, "http://replacement.invalid", c.Endpoint())
	resp, err := c.InspectPostWithBody(t.Context(), "echo", "application/octet-stream", strings.NewReader("input"))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestSafeClientZoneEndpointPreservesCredentialsAndOperation(t *testing.T) {
	var calls int
	original := &http.Client{Transport: inspectTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "[::1%lo0]:1", req.URL.Host)
		require.Equal(t, "/provider%2Froute/inspect/echo?key="+inspectSecret, req.URL.RequestURI())
		user, password, ok := req.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "operator", user)
		require.Equal(t, inspectSecret, password)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	c, err := NewSafeClient("http://operator:"+inspectSecret+"@[::1%25lo0]:1/provider%2Froute?key="+inspectSecret,
		WithHTTPClient(original))
	require.NoError(t, err)
	require.Equal(t, "http://[::1%25lo0]:1", c.Endpoint())
	response, err := c.InspectPostWithBody(t.Context(), "echo", "application/octet-stream", strings.NewReader("input"))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, 1, calls)
}

func TestSafeClientConstructorOptionErrorCannotExposeRawEndpoint(t *testing.T) {
	endpoint := "http://operator:" + inspectSecret + "@example.invalid/private?key=" + inspectSecret
	cause := errors.New(endpoint)
	_, err := NewSafeClient(endpoint, func(c *Client) error {
		require.Equal(t, endpoint, c.Server)
		return cause
	})
	require.ErrorIs(t, err, cause)
	require.Contains(t, err.Error(), "configure inspect client")
	require.Contains(t, err.Error(), "http://example.invalid")
	require.NotContains(t, err.Error(), inspectSecret)
	require.NotContains(t, err.Error(), "operator")
}

func TestSafeClientPreservesEscapedBasePath(t *testing.T) {
	for _, path := range []string{"/base", "/base/", "/base//", "/provider%2F", "/provider%252F", "/provider%20token"} {
		t.Run(path, func(t *testing.T) {
			expected := strings.TrimSuffix(path, "/") + "/inspect/echo?key=" + inspectSecret
			underlying := &http.Client{Transport: inspectTransport(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, expected, req.URL.RequestURI())
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})}
			c, err := NewSafeClient("http://example.invalid"+path+"?key="+inspectSecret, WithHTTPClient(underlying))
			require.NoError(t, err)
			resp, err := c.InspectPostWithBody(t.Context(), "echo", "application/octet-stream", strings.NewReader("input"))
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
		})
	}
}

func TestSafeClientFailureDiagnosticUsesCallerDeadline(t *testing.T) {
	for _, editorFailure := range []bool{false, true} {
		t.Run(fmt.Sprint("editor=", editorFailure), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			var calls int
			transport := &http.Client{Transport: inspectTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				<-req.Context().Done()
				return nil, fmt.Errorf("%s: %w", inspectSecret, req.Context().Err())
			})}
			c, err := NewSafeClient("http://operator:"+inspectSecret+"@example.invalid/base?key="+inspectSecret,
				WithHTTPClient(transport))
			require.NoError(t, err)
			var editors []RequestEditorFn
			if editorFailure {
				editors = append(editors, func(ctx context.Context, _ *http.Request) error {
					<-ctx.Done()
					return fmt.Errorf("%s: %w", inspectSecret, ctx.Err())
				})
			}
			_, err = c.InspectPostWithBody(ctx, "echo", "application/octet-stream", strings.NewReader("input"), editors...)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Contains(t, err.Error(), "inspect request failed for http://example.invalid: context deadline exceeded")
			require.NotContains(t, err.Error(), inspectSecret)
			if editorFailure {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
		})
	}
}
