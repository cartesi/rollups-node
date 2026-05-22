// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package client provides a JSON‑RPC client that can be used as a library
// by other services or commands.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/cartesi/rollups-node/internal/model"
)

// JsonRpcClient defines the interface for our client so it can be mocked.
type JsonRpcClient interface {
	Discover(ctx context.Context) (any, error)
	ListApplications(ctx context.Context, limit, offset int64) ([]*model.Application, error)
	GetApplication(ctx context.Context, application string) (*model.Application, error)
	GetApplicationAddress(ctx context.Context, name string) (string, error)
	ListEpochs(ctx context.Context, application string, status *string, limit, offset int64) ([]*model.Epoch, error)
	GetEpoch(ctx context.Context, application string, index uint64) (*model.Epoch, error)
	GetLastAcceptedEpochIndex(ctx context.Context, application string) (uint64, error)
	ListInputs(ctx context.Context, application string, epochIndex *string, sender *string, decode bool, limit, offset int64) ([]interface{}, error)
	GetInput(ctx context.Context, application string, inputIndex string, decode bool) (any, error)
	GetProcessedInputCount(ctx context.Context, application string) (int64, error)
	ListOutputs(ctx context.Context, application string, epochIndex, inputIndex, rawDataPrefix, outputType, voucherAddress *string, decode bool, limit, offset int64) ([]interface{}, error)
	GetOutput(ctx context.Context, application string, outputIndex string, decode bool) (any, error)
	ListReports(ctx context.Context, application string, epochIndex, inputIndex *string, limit, offset int64) ([]*model.Report, error)
	GetReport(ctx context.Context, application string, reportIndex string) (*model.Report, error)
}

// Client is the concrete implementation of JsonRpcClient.
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

// -----------------------------------------------------------------------------
// Wrapper Types for Responses
// -----------------------------------------------------------------------------

type ApplicationListResult struct {
	Applications []*model.Application `json:"applications"`
}

type ApplicationGetResult struct {
	Application *model.Application `json:"application"`
}

type GetApplicationAddressResult struct {
	Address string `json:"address"`
}

type EpochListResult struct {
	Epochs []*model.Epoch `json:"epochs"`
}

type EpochGetResult struct {
	Epoch *model.Epoch `json:"epoch"`
}

type InputListResult struct {
	Inputs []any `json:"inputs"`
}

type InputGetResult struct {
	Input any `json:"input"`
}

type OutputListResult struct {
	Outputs []any `json:"outputs"`
}

type OutputGetResult struct {
	Output any `json:"output"`
}

type ReportListResult struct {
	Reports []*model.Report `json:"reports"`
}

type ReportGetResult struct {
	Report *model.Report `json:"report"`
}

// -----------------------------------------------------------------------------
// Wrapper Methods
// -----------------------------------------------------------------------------

// Discover calls the "rpc.discover" method and returns the service specification.
func (c *Client) Discover(ctx context.Context) (any, error) {
	var spec any
	if err := c.Call(ctx, "rpc.discover", []any{}, &spec); err != nil {
		return nil, err
	}
	return spec, nil
}

// ListApplications calls "cartesi_ListApplications".
func (c *Client) ListApplications(ctx context.Context, limit, offset int64) ([]*model.Application, error) {
	// Cap limit at 10,000.
	if limit > 10000 {
		limit = 10000
	}
	params := struct {
		Limit  int64 `json:"limit"`
		Offset int64 `json:"offset"`
	}{Limit: limit, Offset: offset}
	var result ApplicationListResult
	if err := c.Call(ctx, "cartesi_listApplications", params, &result); err != nil {
		return nil, err
	}
	return result.Applications, nil
}

// GetApplication calls "cartesi_getApplication".
func (c *Client) GetApplication(ctx context.Context, application string) (*model.Application, error) {
	params := struct {
		Application string `json:"application"`
	}{Application: application}
	var result ApplicationGetResult
	if err := c.Call(ctx, "cartesi_getApplication", params, &result); err != nil {
		return nil, err
	}
	return result.Application, nil
}

// GetApplicationAddress calls "cartesi_getApplicationAddress".
func (c *Client) GetApplicationAddress(ctx context.Context, name string) (string, error) {
	params := struct {
		Name string `json:"name"`
	}{Name: name}
	var result GetApplicationAddressResult
	if err := c.Call(ctx, "cartesi_getApplicationAddress", params, &result); err != nil {
		return "", err
	}
	return result.Address, nil
}

// ListEpochs calls "cartesi_ListEpochs".
func (c *Client) ListEpochs(ctx context.Context, application string, status *string, limit, offset int64) ([]*model.Epoch, error) {
	if limit > 10000 {
		limit = 10000
	}
	params := struct {
		Application string  `json:"application"`
		Status      *string `json:"status,omitempty"`
		Limit       int64   `json:"limit"`
		Offset      int64   `json:"offset"`
	}{
		Application: application,
		Status:      status,
		Limit:       limit,
		Offset:      offset,
	}
	var result EpochListResult
	if err := c.Call(ctx, "cartesi_listEpochs", params, &result); err != nil {
		return nil, err
	}
	return result.Epochs, nil
}

// GetEpoch calls "cartesi_getEpoch".
func (c *Client) GetEpoch(ctx context.Context, application string, index uint64) (*model.Epoch, error) {
	params := struct {
		Application string `json:"application"`
		Index       string `json:"index"`
	}{
		Application: application,
		Index:       fmt.Sprintf("%d", index),
	}
	var result EpochGetResult
	if err := c.Call(ctx, "cartesi_getEpoch", params, &result); err != nil {
		return nil, err
	}
	return result.Epoch, nil
}

// GetLastAcceptedEpochIndex calls "cartesi_getLastAcceptedEpochIndex".
func (c *Client) GetLastAcceptedEpochIndex(ctx context.Context, application string) (uint64, error) {
	params := struct {
		Application string `json:"application"`
	}{Application: application}
	var result struct {
		LastAcceptedEpochIndex uint64 `json:"data"`
	}
	if err := c.Call(ctx, "cartesi_getLastAcceptedEpochIndex", params, &result); err != nil {
		return 0, err
	}
	return result.LastAcceptedEpochIndex, nil
}

// ListInputs calls "cartesi_ListInputs".
func (c *Client) ListInputs(ctx context.Context, application string, epochIndex *string, sender *string, decode bool, limit, offset int64) ([]interface{}, error) {
	if limit > 10000 {
		limit = 10000
	}
	params := struct {
		Application string  `json:"application"`
		EpochIndex  *string `json:"epoch_index,omitempty"`
		Sender      *string `json:"sender,omitempty"`
		Decode      bool    `json:"decode,omitempty"`
		Limit       int64   `json:"limit"`
		Offset      int64   `json:"offset"`
	}{
		Application: application,
		EpochIndex:  epochIndex,
		Sender:      sender,
		Decode:      decode,
		Limit:       limit,
		Offset:      offset,
	}
	var result InputListResult
	if err := c.Call(ctx, "cartesi_listInputs", params, &result); err != nil {
		return nil, err
	}
	return result.Inputs, nil
}

// GetInput calls "cartesi_getInput".
func (c *Client) GetInput(ctx context.Context, application string, inputIndex string, decode bool) (any, error) {
	params := struct {
		Application string `json:"application"`
		InputIndex  string `json:"input_index"`
		Decode      bool   `json:"decode,omitempty"`
	}{
		Application: application,
		InputIndex:  inputIndex,
		Decode:      decode,
	}
	var result InputGetResult
	if err := c.Call(ctx, "cartesi_getInput", params, &result); err != nil {
		return nil, err
	}
	return result.Input, nil
}

// GetProcessedInputCount calls "cartesi_getProcessedInputCount".
func (c *Client) GetProcessedInputCount(ctx context.Context, application string) (int64, error) {
	params := struct {
		Application string `json:"application"`
	}{
		Application: application,
	}
	var result struct {
		ProcessedInputs int64 `json:"processed_inputs"`
	}
	if err := c.Call(ctx, "cartesi_getProcessedInputCount", params, &result); err != nil {
		return 0, err
	}
	return result.ProcessedInputs, nil
}

// ListOutputs calls "cartesi_ListOutputs".
func (c *Client) ListOutputs(ctx context.Context, application string, epochIndex, inputIndex, rawDataPrefix, outputType, voucherAddress *string, decode bool, limit, offset int64) ([]interface{}, error) {
	if limit > 10000 {
		limit = 10000
	}
	params := struct {
		Application    string  `json:"application"`
		EpochIndex     *string `json:"epoch_index,omitempty"`
		InputIndex     *string `json:"input_index,omitempty"`
		RawDataPrefix  *string `json:"raw_data_prefix,omitempty"`
		OutputType     *string `json:"output_type,omitempty"`
		VoucherAddress *string `json:"voucher_address,omitempty"`
		Decode         bool    `json:"decode,omitempty"`
		Limit          int64   `json:"limit"`
		Offset         int64   `json:"offset"`
	}{
		Application:    application,
		EpochIndex:     epochIndex,
		InputIndex:     inputIndex,
		RawDataPrefix:  rawDataPrefix,
		OutputType:     outputType,
		VoucherAddress: voucherAddress,
		Decode:         decode,
		Limit:          limit,
		Offset:         offset,
	}
	var result OutputListResult
	if err := c.Call(ctx, "cartesi_listOutputs", params, &result); err != nil {
		return nil, err
	}
	return result.Outputs, nil
}

// GetOutput calls "cartesi_getOutput".
func (c *Client) GetOutput(ctx context.Context, application string, outputIndex string, decode bool) (any, error) {
	params := struct {
		Application string `json:"application"`
		OutputIndex string `json:"output_index"`
		Decode      bool   `json:"decode,omitempty"`
	}{
		Application: application,
		OutputIndex: outputIndex,
		Decode:      decode,
	}
	var result OutputGetResult
	if err := c.Call(ctx, "cartesi_getOutput", params, &result); err != nil {
		return nil, err
	}
	return result.Output, nil
}

// ListReports calls "cartesi_ListReports".
func (c *Client) ListReports(ctx context.Context, application string, epochIndex, inputIndex *string, limit, offset int64) ([]*model.Report, error) {
	if limit > 10000 {
		limit = 10000
	}
	params := struct {
		Application string  `json:"application"`
		EpochIndex  *string `json:"epoch_index,omitempty"`
		InputIndex  *string `json:"input_index,omitempty"`
		Limit       int64   `json:"limit"`
		Offset      int64   `json:"offset"`
	}{
		Application: application,
		EpochIndex:  epochIndex,
		InputIndex:  inputIndex,
		Limit:       limit,
		Offset:      offset,
	}
	var result ReportListResult
	if err := c.Call(ctx, "cartesi_listReports", params, &result); err != nil {
		return nil, err
	}
	return result.Reports, nil
}

// GetReport calls "cartesi_getReport".
func (c *Client) GetReport(ctx context.Context, application string, reportIndex string) (*model.Report, error) {
	params := struct {
		Application string `json:"application"`
		ReportIndex string `json:"report_index"`
	}{
		Application: application,
		ReportIndex: reportIndex,
	}
	var result ReportGetResult
	if err := c.Call(ctx, "cartesi_getReport", params, &result); err != nil {
		return nil, err
	}
	return result.Report, nil
}
