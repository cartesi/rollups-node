// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestValidate(t *testing.T) {
	for _, raw := range []string{"http://localhost:8545", "https://u:p@example.com/path?q=key", "http://[::1]:8545/rpc"} {
		u, err := Validate(raw)
		require.NoError(t, err)
		require.NotEmpty(t, u.Hostname())
	}
	for _, raw := range []string{
		"", "SECRET.sock", "wss://host/SECRET", "ws://host/SECRET", "ipc://host/SECRET",
		"https:///SECRET", "http://:8545/SECRET", "http:SECRET", "http://host/%SECRET", "http://host:SECRET",
	} {
		_, err := Validate(raw)
		require.Error(t, err, raw)
		require.NotContains(t, err.Error(), "SECRET")
	}
}

func TestWireURLAndAuthentication(t *testing.T) {
	const marker = "secret_Aa23%2Ffragment"
	type observation struct{ URI, Host, Auth, Body string }
	var got []observation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = append(got, observation{r.RequestURI, r.Host, r.Header.Get("Authorization"), string(body)})
		_, _ = io.WriteString(w, "successful payload "+marker)
	}))
	defer server.Close()
	u, err := url.Parse(server.URL + "/one%2Ftwo//./path?key=a%2Fb&key=second+&empty=#ignored")
	require.NoError(t, err)
	u.User = url.UserPassword("user:name", "3")
	for _, auth := range []string{"", "Bearer explicit"} {
		for _, protect := range []bool{false, true} {
			base, client := u.String(), server.Client()
			if protect {
				base, client, err = NewClient(base, client, Options{})
				require.NoError(t, err)
			}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base, strings.NewReader("rpc payload"))
			require.NoError(t, err)
			if auth != "" {
				req.Header.Set("Authorization", auth)
			}
			resp, err := client.Do(req)
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, "successful payload "+marker, string(body))
		}
		require.Equal(t, got[len(got)-2], got[len(got)-1], "wire behavior differs from net/http")
	}
}

func TestZoneScopedEndpointPreservesWireURL(t *testing.T) {
	const endpoint = "http://operator:credential@[fe80::1%25lo0]:8545/base%2Froute?key=credential"
	var received bool
	base, client, err := NewClient(endpoint, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		received = true
		require.Equal(t, endpoint, req.URL.String())
		require.Equal(t, "/base%2Froute?key=credential", req.URL.RequestURI())
		user, password, ok := req.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "operator", user)
		require.Equal(t, "credential", password)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}, Options{})
	require.NoError(t, err)
	require.Equal(t, "http://[fe80::1%25lo0]:8545", base)
	resp, err := client.Get(base)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.True(t, received)
}

func TestOperationCompositionAndImmutableCopies(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	const endpoint = "https://user:pass@host.example/base%2Fkey//?k=a%2Fb&k=second+" //nolint:gosec // Synthetic Basic auth credentials.
	base, rt, err := New(endpoint, roundTripFunc(func(wire *http.Request) (*http.Response, error) {
		require.Equal(t, ctx, wire.Context())
		require.Equal(t, "/base%2Fkey//inspect/app%2Fname?k=a%2Fb&k=second+&q=last+", wire.URL.RequestURI())
		require.Equal(t, "custom.host", wire.Host)
		user, password, ok := wire.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "user", user)
		require.Equal(t, "pass", password)
		wire.Header.Set("X-Original", "changed")
		return &http.Response{
			StatusCode: 200, Status: "200 provider text", Header: http.Header{"Location": {"%malformed"}},
			Body: io.NopCloser(strings.NewReader("ok")), Request: wire,
		}, nil
	}), Options{AppendOperation: true})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/inspect/app%2Fname?q=last+", strings.NewReader("data"))
	require.NoError(t, err)
	req.Header.Set("X-Original", "original")
	req.Host = "custom.host"
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Same(t, req, resp.Request)
	require.Equal(t, "200 OK", resp.Status)
	require.Empty(t, resp.Header.Get("Location"))
	require.Nil(t, req.URL.User)
	require.Empty(t, req.Header.Get("Authorization"))
	require.Equal(t, "original", req.Header.Get("X-Original"))
	require.Equal(t, "/inspect/app%2Fname?q=last+", req.URL.RequestURI())
}

func TestOperationRetainsEncodedTrailingSlash(t *testing.T) {
	base, rt, err := New("http://host/provider%2F?k=1", roundTripFunc(func(wire *http.Request) (*http.Response, error) {
		require.Equal(t, "/provider%2F/inspect/app?k=1", wire.URL.RequestURI())
		require.Equal(t, "/provider//inspect/app", wire.URL.Path)
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}), Options{AppendOperation: true})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, base+"/inspect/app", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestCookieJarUsesActualEndpointPath(t *testing.T) {
	for _, appendOperation := range []bool{false, true} {
		t.Run(fmt.Sprint(appendOperation), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Contains(t, r.URL.Path, "/base/")
				cookie, err := r.Cookie("scoped")
				require.NoError(t, err)
				require.Equal(t, "present", cookie.Value)
				if requests.Add(1) == 2 {
					cookie, err := r.Cookie("default-path")
					require.NoError(t, err)
					require.Equal(t, "stored", cookie.Value)
				}
				http.SetCookie(w, &http.Cookie{
					Name: "default-path", Value: "stored", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
				})
				_, _ = io.WriteString(w, "ok")
			}))
			defer server.Close()
			jar, err := cookiejar.New(nil)
			require.NoError(t, err)
			endpoint, err := url.Parse(server.URL + "/base/rpc")
			require.NoError(t, err)
			jar.SetCookies(endpoint, []*http.Cookie{{
				Name: "scoped", Value: "present", Path: "/base", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
			}})
			original := server.Client()
			original.Jar = jar
			base, client, err := NewClient(endpoint.String(), original, Options{AppendOperation: appendOperation})
			require.NoError(t, err)
			if appendOperation {
				base += "/inspect/app"
			}
			for range 2 {
				resp, err := client.Get(base)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
			}
			require.Same(t, jar, original.Jar)
		})
	}
}

func TestProtocolErrorsNeverReflectProviderText(t *testing.T) {
	const marker = "SECRET_aB09_PathQuery"
	for _, tc := range []struct {
		name, reply string
		read        bool
	}{
		{"status", "HTTP/1.1 " + marker + " reason\r\nContent-Length: 0\r\n\r\n", false},
		{"version", marker + " 200 OK\r\nContent-Length: 0\r\n\r\n", false},
		{"header", "HTTP/1.1 200 OK\r\n" + marker + "\r\nContent-Length: 0\r\n\r\n", false},
		{"header value", "HTTP/1.1 200 OK\r\nX-Echo: " + marker + "\x00\r\nContent-Length: 0\r\n\r\n", false},
		{"trailer", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n2\r\n{}\r\n0\r\n" + marker + "\r\n\r\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			base, client, err := NewClient(server.URL+"/"+marker+"?key="+marker, server.Client(), Options{})
			require.NoError(t, err)
			resp, err := client.Get(base)
			if tc.read {
				require.NoError(t, err)
				defer resp.Body.Close()
				_, err = io.ReadAll(resp.Body)
			}
			require.Error(t, err)
			require.True(t, received.Load(), "configured wire operation was not sent")
			require.Contains(t, err.Error(), server.URL)
			require.NotContains(t, err.Error(), marker)
			require.NotContains(t, fmt.Sprintf("%#v", err), marker)
		})
	}
}

func TestNonSuccessDiagnosticsPreserveOnlyNumericRPCCode(t *testing.T) {
	for _, code := range []int{-32047, -32600, -32005, 400} {
		const marker = "echo_Aa17_secret"
		base, rt, err := New("http://user:3@host/"+marker, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := fmt.Sprintf(
				`{"jsonrpc":"2.0","id":"%s","error":{"code":%d,"message":"%s password=3","data":"%s"},"unknown":"%s"}`,
				marker, code, marker, marker, marker,
			)
			return &http.Response{
				StatusCode: 400, Status: "400 " + marker, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: req,
			}, nil
		}), Options{})
		require.NoError(t, err)
		req, _ := http.NewRequest(http.MethodPost, base, nil)
		resp, err := rt.RoundTrip(req)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, "400 Bad Request", resp.Status)
		require.Equal(t, int64(len(body)), resp.ContentLength)
		require.NotContains(t, string(body), marker)
		require.NotContains(t, string(body), "password=3")
		var parsed struct {
			Error struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(body, &parsed))
		require.Equal(t, code, parsed.Error.Code)
	}
	for _, body := range []string{"plain secret", `{"error":{"code":"secret","message":"secret"}}`, `{"error":null}`, `[]`} {
		require.NotContains(t, string(diagnosticResponse([]byte(body), nil)), "secret")
	}
}

func TestRedirectsAreNotFollowedOrParsed(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, location := range []string{"/elsewhere", "http://%SECRET"} {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.Header().Set("Location", location)
				w.WriteHeader(status)
			}))
			base, client, err := NewClient(server.URL+"/SECRET", server.Client(), Options{})
			require.NoError(t, err)
			resp, err := client.Get(base)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, status, resp.StatusCode)
			require.Empty(t, resp.Header.Get("Location"))
			require.Equal(t, int32(1), hits.Load())
			server.Close()
		}
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "SECRET timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestDiagnosticClassificationAndFormatting(t *testing.T) {
	for _, original := range []error{
		context.Canceled, context.DeadlineExceeded, timeoutError{},
		errors.Join(context.Canceled, errors.New("SECRET failure")), io.ErrUnexpectedEOF,
	} {
		safe := Diagnostic(t.Context(), original, "HTTP response read", "https://host")
		require.ErrorIs(t, safe, original)
		require.Equal(t, errutil.IsOnlyCancellation(original), errutil.IsOnlyCancellation(safe))
		for _, format := range []string{"%v", "%+v", "%#v"} {
			require.NotContains(t, fmt.Sprintf(format, safe), "SECRET")
			require.Contains(t, fmt.Sprintf(format, safe), "https://host")
		}
	}
	safe := Diagnostic(t.Context(), timeoutError{}, "HTTP request", "https://host")
	require.True(t, (&url.Error{Op: "Post", URL: "https://host", Err: safe}).Timeout())
	require.Nil(t, Diagnostic(t.Context(), nil, "HTTP request", "https://host"))
}

type readFailure struct{ closed bool }

func (r *readFailure) Read(p []byte) (int, error) {
	return copy(p, "unchanged SECRET bytes"), errors.New("SECRET trailer")
}
func (r *readFailure) Close() error { r.closed = true; return nil }

func TestReadErrorsDoNotChangeSuccessfulBytes(t *testing.T) {
	body := &readFailure{}
	safe := &diagnosticBody{ReadCloser: body, safeBase: "https://host"}
	buffer := make([]byte, 100)
	n, err := safe.Read(buffer)
	require.Equal(t, "unchanged SECRET bytes", string(buffer[:n]))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "SECRET")
	require.NoError(t, safe.Close())
	require.True(t, body.closed)
}

type closingTransport struct{ closed atomic.Bool }

func (*closingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unused")
}
func (t *closingTransport) CloseIdleConnections() { t.closed.Store(true) }

func TestClientSettingsCleanupAndConcurrency(t *testing.T) {
	next := &closingTransport{}
	original := &http.Client{Transport: next, Timeout: time.Second}
	_, protected, err := NewClient("http://host/SECRET", original, Options{})
	require.NoError(t, err)
	require.Equal(t, original.Timeout, protected.Timeout)
	require.Same(t, next, original.Transport)
	protected.CloseIdleConnections()
	require.True(t, next.closed.Load())
	base, rt, err := New("http://u:p@host/key?x=1", roundTripFunc(func(wire *http.Request) (*http.Response, error) {
		require.Equal(t, "/key?x=1", wire.URL.RequestURI())
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}), Options{})
	require.NoError(t, err)
	req, _ := http.NewRequest(http.MethodPost, base, nil)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { resp, err := rt.RoundTrip(req); require.NoError(t, err); _ = resp.Body.Close() })
	}
	wg.Wait()
	require.Nil(t, req.URL.User)
	require.Empty(t, req.Header.Get("Authorization"))
}
