// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package httpclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDiagnosticNamesTypedFailuresWithoutRenderingCauses(t *testing.T) {
	const marker = "PROVIDER_CREDENTIAL"
	expired, cancel := context.WithDeadline(t.Context(), time.Unix(1, 0))
	defer cancel()
	for _, test := range []struct {
		name, category string
		cause          error
		ctx            context.Context
	}{
		{"DNS", "DNS lookup failed", &net.DNSError{Err: marker, Name: marker}, t.Context()},
		{"refused", "connection refused", fmt.Errorf("%s: %w", marker, syscall.ECONNREFUSED), t.Context()},
		{"proxy", "proxy connection failed", &net.OpError{Op: "proxyconnect", Err: syscall.ECONNREFUSED}, t.Context()},
		{"proxy timeout", "proxy connection failed", &net.OpError{Op: "proxyconnect", Err: timeoutError{}}, t.Context()},
		{"reset", "connection reset", fmt.Errorf("%s: %w", marker, syscall.ECONNRESET), t.Context()},
		{"CA", "unknown certificate authority", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, t.Context()},
		{
			"hostname", "certificate hostname mismatch",
			&tls.CertificateVerificationError{Err: x509.HostnameError{Host: marker}}, t.Context(),
		},
		{"certificate", "certificate verification failed", &tls.CertificateVerificationError{Err: errors.New(marker)}, t.Context()},
		{"TLS record", "TLS handshake failed", tls.RecordHeaderError{Msg: marker}, t.Context()},
		{"timeout", "timeout", timeoutError{}, t.Context()},
		{"deadline", "context deadline exceeded", fmt.Errorf("%s: %w", marker, context.DeadlineExceeded), expired},
		{"cancellation", "context canceled", fmt.Errorf("%s: %w", marker, context.Canceled), t.Context()},
		{"unexpected EOF", "unexpected EOF", fmt.Errorf("%s: %w", marker, io.ErrUnexpectedEOF), t.Context()},
		{"EOF", "connection closed", fmt.Errorf("%s: %w", marker, io.EOF), t.Context()},
	} {
		t.Run(test.name, func(t *testing.T) {
			safe := Diagnostic(test.ctx, test.cause, "HTTP request", "https://provider.example")
			require.Contains(t, safe.Error(), test.category)
			require.Contains(t, safe.Error(), "https://provider.example")
			require.NotContains(t, safe.Error(), marker)
			require.ErrorIs(t, safe, test.cause)
		})
	}
}

type observedBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func (b *observedBody) Close() error { b.closed = true; return nil }

func TestErrorResponseReadIsBounded(t *testing.T) {
	const budget = 64 << 10
	original := &observedBody{Reader: strings.NewReader("nonce too low " + strings.Repeat("x", budget*4) + "CREDENTIAL")}
	base, rt, err := New("http://provider.example/CREDENTIAL", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: original}, nil
	}), Options{})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, base, nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.LessOrEqual(t, original.read, budget+1)
	require.True(t, original.closed)
	require.Contains(t, string(body), "HTTP request failed")
	require.NotContains(t, string(body), "CREDENTIAL")
	require.NotContains(t, string(body), "nonce too low", "oversized input must use the generic fallback")
}

func TestDiagnosticReadBoundaryAndSuccessfulPayload(t *testing.T) {
	const budget = 64 << 10
	for _, size := range []int{budget - 1, budget, budget + 1} {
		for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
			t.Run(fmt.Sprintf("size=%d/status=%d", size, status), func(t *testing.T) {
				payload := "nonce too low " + strings.Repeat("x", size-len("nonce too low "))
				base, rt, err := New("http://provider.example/credential", roundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(payload))}, nil
				}), Options{})
				require.NoError(t, err)
				req, err := http.NewRequest(http.MethodGet, base, nil)
				require.NoError(t, err)
				resp, err := rt.RoundTrip(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				switch {
				case status == http.StatusOK:
					require.Equal(t, payload, string(body))
				case size <= budget:
					require.Equal(t, "nonce too low", string(body))
					require.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))
				default:
					require.Equal(t, "HTTP request failed", string(body))
				}
			})
		}
	}
}

func TestUnannouncedResponseTrailerSurvives(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n"+
			"1\r\na\r\n0\r\nX-Unannounced: trailer-value\r\n\r\n")
	}))
	defer server.Close()
	base, client, err := NewClient(server.URL+"/credential", nil, Options{})
	require.NoError(t, err)
	resp, err := client.Get(base)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "a", string(body))
	require.Equal(t, "trailer-value", resp.Trailer.Get("X-Unannounced"))
}

func TestBodylessResponsesRemainBodyless(t *testing.T) {
	for _, test := range []struct {
		method string
		status int
	}{
		{http.MethodHead, http.StatusNotFound},
		{http.MethodHead, http.StatusNoContent},
		{http.MethodGet, http.StatusNoContent},
		{http.MethodGet, http.StatusNotModified},
		{http.MethodGet, http.StatusSwitchingProtocols},
	} {
		t.Run(fmt.Sprintf("%s/%d", test.method, test.status), func(t *testing.T) {
			base, rt, err := New("http://provider.example/credential", roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: http.Header{},
					Body: io.NopCloser(strings.NewReader("CREDENTIAL"))}, nil
			}), Options{})
			require.NoError(t, err)
			req, err := http.NewRequest(test.method, base, nil)
			require.NoError(t, err)
			resp, err := rt.RoundTrip(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Empty(t, body)
			require.Empty(t, resp.Header.Get("Content-Type"))
		})
	}
}

func TestErrorMessagesRequireExplicitPolicyAndCompleteMatch(t *testing.T) {
	for _, message := range []string{"Known service failure", "Another service failure"} {
		t.Run(message, func(t *testing.T) {
			for _, allowed := range []bool{false, true} {
				options := Options{}
				if allowed {
					options.AllowedErrorMessages = []string{message}
				}
				for _, raw := range []string{message, message + "\n", message + " CREDENTIAL", "CREDENTIAL " + message} {
					base, rt, err := New("http://provider.example/CREDENTIAL", roundTripFunc(func(*http.Request) (*http.Response, error) {
						return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(raw))}, nil
					}), options)
					require.NoError(t, err)
					req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base, nil)
					require.NoError(t, err)
					resp, err := rt.RoundTrip(req)
					require.NoError(t, err)
					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					require.NoError(t, resp.Body.Close())
					want := "HTTP request failed"
					if allowed && (raw == message || raw == message+"\n") {
						want = message
					}
					require.Equal(t, want, string(body))
				}
			}
		})
	}
}

func TestErrorMessagePolicyCopiesCallerSlice(t *testing.T) {
	const trustedMessage = "Known service failure"
	allowed := []string{trustedMessage}
	raw := "CREDENTIAL"
	base, rt, err := New("http://provider.example/CREDENTIAL", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(raw))}, nil
	}), Options{AllowedErrorMessages: allowed})
	require.NoError(t, err)
	allowed[0] = raw
	for _, test := range []struct{ input, want string }{
		{raw, "HTTP request failed"},
		{trustedMessage, trustedMessage},
	} {
		raw = test.input
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base, nil)
		require.NoError(t, err)
		resp, err := rt.RoundTrip(req)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, test.want, string(body))
	}
}

func TestProxyAndTransportTimeoutLabels(t *testing.T) {
	t.Run("proxy connection", func(t *testing.T) {
		proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		proxyURL, err := url.Parse(proxy.URL)
		require.NoError(t, err)
		proxy.Close()
		next := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		defer next.CloseIdleConnections()
		base, client, err := NewClient("http://provider.invalid/CREDENTIAL", &http.Client{Transport: next}, Options{})
		require.NoError(t, err)
		_, err = client.Get(base)
		require.Error(t, err)
		require.Contains(t, err.Error(), "proxy connection failed")
		require.Contains(t, err.Error(), "http://provider.invalid")
		require.NotContains(t, err.Error(), "CREDENTIAL")
		require.NotContains(t, err.Error(), proxy.URL)
	})
	t.Run("response header timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer server.Close()
		next := &http.Transport{ResponseHeaderTimeout: 10 * time.Millisecond}
		defer next.CloseIdleConnections()
		base, client, err := NewClient(server.URL+"/CREDENTIAL", &http.Client{Transport: next}, Options{})
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base, nil)
		require.NoError(t, err)
		_, err = client.Do(req)
		require.Error(t, err)
		require.NoError(t, req.Context().Err())
		require.Contains(t, err.Error(), ": timeout")
		require.Contains(t, err.Error(), server.URL)
		require.NotContains(t, err.Error(), "context deadline")
		require.NotContains(t, err.Error(), "CREDENTIAL")
	})
	t.Run("dial timeout", func(t *testing.T) {
		next := &http.Transport{DialContext: (&net.Dialer{Timeout: time.Nanosecond}).DialContext}
		defer next.CloseIdleConnections()
		base, client, err := NewClient("http://127.0.0.1:1/CREDENTIAL", &http.Client{Transport: next}, Options{})
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base, nil)
		require.NoError(t, err)
		_, err = client.Do(req)
		require.Error(t, err)
		require.NoError(t, req.Context().Err())
		require.Contains(t, err.Error(), ": timeout")
		require.Contains(t, err.Error(), "http://127.0.0.1:1")
		require.NotContains(t, err.Error(), "context deadline")
		require.NotContains(t, err.Error(), "CREDENTIAL")
	})
}

func TestDiagnosticBodyReadFailuresKeepSafeStatus(t *testing.T) {
	body := &readFailure{}
	base, rt, err := New("http://provider.example/CREDENTIAL", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Body: body}, nil
	}), Options{})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base, nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "HTTP request failed", string(data))
	require.True(t, body.closed)
}

func TestClosedLegacyCancelKeepsReadFailureAboveRetryStatus(t *testing.T) {
	cause := errors.New("CREDENTIAL body read failure")
	body := &observedBody{Reader: iotest.ErrReader(cause)}
	base, rt, err := New("http://provider.example/CREDENTIAL", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Body: body}, nil
	}), Options{})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base, nil)
	require.NoError(t, err)
	canceled := make(chan struct{})
	close(canceled)
	//nolint:staticcheck,nolintlint // Exercise the deliberate legacy Cancel exemption used for Client.Timeout.
	req.Cancel = canceled
	require.NoError(t, req.Context().Err())
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err, "retry policy must receive the non-2xx status")
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	data, err := io.ReadAll(resp.Body)
	require.Empty(t, data, "partial diagnostic bytes must not be exposed")
	require.ErrorIs(t, err, cause)
	require.NotErrorIs(t, err, context.Canceled, "legacy cancellation must not manufacture a context cause")
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	require.Contains(t, err.Error(), "http://provider.example")
	require.NotContains(t, err.Error(), "CREDENTIAL")
	require.True(t, body.closed)
}
