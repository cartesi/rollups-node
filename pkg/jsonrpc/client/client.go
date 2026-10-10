// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package client provides a minimal JSON-RPC 2.0 transport for the rollups
// node API. Callers invoke methods through the generic Call and decode the
// result themselves; the response envelope and parameter shapes are defined
// by the server's OpenRPC specification (served via rpc.discover).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/cartesi/rollups-node/internal/httpclient"
	"github.com/google/uuid"
)

// Client is a JSON-RPC 2.0 client over HTTP.
type Client struct {
	// URL configures the HTTP(S) endpoint and may contain credentials. Transport
	// diagnostics use its public scheme and host. Valid HTTP-200 provider errors
	// retain their messages and may reflect credentials supplied to that provider.
	URL string
	// HTTPClient is the underlying HTTP client.
	HTTPClient *http.Client
	// idCounter is used to generate unique request IDs.
	idCounter uint64
}

// NewClient creates an HTTP(S) JSON-RPC client. Requests preserve the configured
// path, query and authentication, and do not follow redirects. Call validates the
// endpoint and emits safe transport diagnostics. Unexpected non-200 statuses are
// errors; non-2xx messages are filtered and other 2xx bodies are omitted.
func NewClient(endpoint string) *Client {
	return &Client{
		URL:        endpoint,
		HTTPClient: http.DefaultClient,
	}
}

func (c *Client) nextID() uint64 {
	return atomic.AddUint64(&c.idCounter, 1)
}

// rpcRequest and rpcResponse define the JSON‑RPC request and response formats.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      uint64 `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      uint64          `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("RPC Error %d: %s", e.Code, e.Message)
}

// Call sends a JSON‑RPC request with the given method and parameters, and
// decodes the first response value into result (if non-nil). Trailing values are
// ignored. Valid HTTP-200 RPC error messages are returned without redaction.
// Requests carry a locally generated X-Request-ID. Failures include that same
// identifier after request construction and preserve the original error cause.
// If the request reached the node, its logs contain that identifier.
func (c *Client) Call(ctx context.Context, method string, params any, result any) (err error) {
	endpoint, httpClient, err := httpclient.NewClient(c.URL, c.HTTPClient, httpclient.Options{})
	if err != nil {
		return fmt.Errorf("configure JSON-RPC client: %w", err)
	}
	reqObj := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      c.nextID(),
	}
	reqBody, err := json.Marshal(reqObj)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return httpclient.Diagnostic(ctx, err, "create JSON-RPC request", endpoint)
	}
	requestID := uuid.NewString()
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w (request_id=%s)", err, requestID)
		}
	}()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", requestID)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The shared transport preserves successful payloads. Unexpected 2xx
		// bodies therefore remain untrusted diagnostic input and are omitted.
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return fmt.Errorf("HTTP error from %s: %s", endpoint, resp.Status)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return httpclient.Diagnostic(ctx, err, "read JSON-RPC error response", endpoint)
		}
		return fmt.Errorf("HTTP error from %s: %s, body: %s", endpoint, resp.Status, string(body))
	}

	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return httpclient.Diagnostic(ctx, err, "decode JSON-RPC response", endpoint)
	}
	if rpcResp.Error != nil {
		return rpcResp.Error
	}
	if result != nil {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return httpclient.Diagnostic(ctx, err, "decode JSON-RPC result", endpoint)
		}
	}
	return nil
}
