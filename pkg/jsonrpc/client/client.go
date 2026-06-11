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
)

// Client is a JSON-RPC 2.0 client over HTTP.
type Client struct {
	// URL is the endpoint of the JSON‑RPC service.
	URL string
	// HTTPClient is the underlying HTTP client.
	HTTPClient *http.Client
	// idCounter is used to generate unique request IDs.
	idCounter uint64
}

// NewClient creates a new JSON‑RPC client.
func NewClient(url string) *Client {
	return &Client{
		URL:        url,
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
// decodes the response into result (if non‑nil).
func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP error: %s, body: %s", resp.Status, string(body))
	}

	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	if rpcResp.Error != nil {
		return rpcResp.Error
	}
	if result != nil {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}
	return nil
}
