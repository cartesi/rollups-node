// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/version"
)

//go:embed jsonrpc-discover.json
var discoverSpec embed.FS

const (
	// Maximum allowed body size (1 MB).
	MAX_BODY_SIZE = 1 << 20 //nolint: revive
	// Maximum cumulative response size (10 MB).
	MAX_RESPONSE_SIZE = 10 << 20 //nolint: revive
	// Maximum amount of request in a batch (100)
	MAX_BATCH_SIZE = 100 //nolint: revive
	// Maximum amount of items to list (10,000).
	LIST_ITEM_LIMIT = 10000 //nolint: revive
	// Default amount of item on a list (50)
	LIST_ITEM_DEFAULT = 50 //nolint: revive
)

const (
	// JSON-RPC  Standard Error Codes (https://json-rpc.dev/docs/reference/error-codes)
	JSONRPC_PARSE_ERROR      int = -32700 //nolint: revive
	JSONRPC_INVALID_REQUEST  int = -32600 //nolint: revive
	JSONRPC_METHOD_NOT_FOUND int = -32601 //nolint: revive
	JSONRPC_INVALID_PARAMS   int = -32602 //nolint: revive
	JSONRPC_INTERNAL_ERROR   int = -32603 //nolint: revive
	JSONRPC_INVALID_BATCH    int = -32040 //nolint: revive
	JSONRPC_TIMEOUT_ERROR    int = -32070 //nolint: revive

	// Resource not found: the requested resource does not exist in the method's
	// scope. For application-scoped methods, this means the application exists
	// but the requested entity does not; unknown applications use
	// JSONRPC_APPLICATION_NOT_FOUND. For forward-looking keys, this can be the
	// "not created yet" signal and may be safe to poll depending on the method.
	JSONRPC_RESOURCE_NOT_FOUND int = -31001 //nolint: revive
	// Application not found: the application identifier itself is unknown to
	// this node. A configuration error that will not resolve by retrying.
	JSONRPC_APPLICATION_NOT_FOUND int = -31002 //nolint: revive
	// Response size limit exceeded:  cumulative buffered-response budget was
	// not enough for all responses in the batch.
	JSONRPC_RESPONSE_SIZE_LIMIT_EXCEEDED int = -31003 //nolint: revive
)

type rpcHandler = func(*Service, *http.Request, RPCRequest) (any, error)
type dispatchTable = map[string]rpcHandler

var jsonrpcHandlers = dispatchTable{
	"rpc.discover":                      handleDiscover,
	"cartesi_listApplications":          handleListApplications,
	"cartesi_getApplication":            handleGetApplication,
	"cartesi_listEpochs":                handleListEpochs,
	"cartesi_getEpoch":                  handleGetEpoch,
	"cartesi_getLastAcceptedEpochIndex": handleGetLastAcceptedEpochIndex,
	"cartesi_listInputs":                handleListInputs,
	"cartesi_getInput":                  handleGetInput,
	"cartesi_getProcessedInputCount":    handleGetProcessedInputCount,
	"cartesi_listOutputs":               handleListOutputs,
	"cartesi_getOutput":                 handleGetOutput,
	"cartesi_listReports":               handleListReports,
	"cartesi_getReport":                 handleGetReport,
	"cartesi_listWithdrawals":           handleListWithdrawals,
	"cartesi_getWithdrawal":             handleGetWithdrawal,
	"cartesi_listTournaments":           handleListTournaments,
	"cartesi_getTournament":             handleGetTournament,
	"cartesi_listCommitments":           handleListCommitments,
	"cartesi_getCommitment":             handleGetCommitment,
	"cartesi_listMatches":               handleListMatches,
	"cartesi_getMatch":                  handleGetMatch,
	"cartesi_listMatchAdvances":         handleListMatchAdvances,
	"cartesi_getMatchAdvanced":          handleGetMatchAdvanced,
	"cartesi_getChainId":                handleGetChainID,
	"cartesi_getNodeVersion":            handleGetNodeVersion,
}

// -----------------------------------------------------------------------------
// Dispatching JSON‑RPC methods
// -----------------------------------------------------------------------------

func (s *Service) handleWriteResponse(err error) bool {
	if err == nil {
		return true
	}
	s.Logger.Warn("failed writing response", "error", err)
	return false
}

func (s *Service) writeByte(w http.ResponseWriter, c byte) bool {
	_, err := w.Write([]byte{c})
	return s.handleWriteResponse(err)
}

// writeRPCError sends a generic error response for internal errors.
func (s *Service) writeRPCError(w http.ResponseWriter, id any, code int, message string) bool {
	err := writeRPCError(w, id, code, message, nil)
	return s.handleWriteResponse(err)
}

func (s *Service) handleRequest(w io.Writer, r *http.Request, req RPCRequest) error {
	switch req.ID.(type) {
	case nil, string, float64:
	default:
		return writeRPCError(w, nil, JSONRPC_INVALID_REQUEST, "invalid request", nil)
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		return writeRPCError(w, req.ID, JSONRPC_INVALID_REQUEST, "invalid request", nil)
	}
	fn, ok := jsonrpcHandlers[req.Method]
	if !ok {
		s.Logger.Debug("RPC method not found", "method", req.Method)
		return writeRPCError(w, req.ID, JSONRPC_METHOD_NOT_FOUND, "Method not found", nil)
	}

	result, err := fn(s, r, req)
	if err == nil {
		return writeRPCResult(w, req.ID, result)
	}

	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		// RPC errors describe expected client-facing failures. Do not log them at
		// error level; unexpected failures are logged below before being hidden.
		return writeRPCError(w, req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
	}

	s.Logger.Error("RPC method failed", "method", req.Method, "error", err)
	return writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
}

func (s *Service) dispatchOneRequest(w http.ResponseWriter, r *http.Request, req RPCRequest, budgetResp *budgetWriter) bool {
	buffer := budgetResp.NewLimitedWriter()
	if buffer == nil {
		return s.writeRPCError(w, req.ID, JSONRPC_RESPONSE_SIZE_LIMIT_EXCEEDED, "Response size limit exceeded")
	}
	err := s.handleRequest(buffer, r, req)
	switch {
	case err == nil:
		return s.handleWriteResponse(buffer.Flush())
	case errors.Is(err, io.ErrShortBuffer):
		return s.writeRPCError(w, req.ID, JSONRPC_RESPONSE_SIZE_LIMIT_EXCEEDED, "Response size limit exceeded")
	default:
		s.Logger.Error("RPC method response encode failed", "method", req.Method, "error", err)
		return s.writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
}

func (s *Service) handleRPC(w http.ResponseWriter, r *http.Request) {
	// Limit request body size and ensure it is closed.
	r.Body = http.MaxBytesReader(w, r.Body, MAX_BODY_SIZE)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		http.Error(w, "Empty request body", http.StatusBadRequest)
		return
	}

	budgetResp := newBudgetWriter(w, MAX_RESPONSE_SIZE)

	switch body[0] {
	case '{':
		var req RPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			s.writeRPCError(w, nil, JSONRPC_PARSE_ERROR, "invalid request")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		s.Logger.Info("Dispatching RPC request", "method", req.Method)
		s.dispatchOneRequest(w, r, req, budgetResp)

	case '[':
		w.Header().Set("Content-Type", "application/json")
		// Keep each batch element raw so malformed requests fail independently and
		// the list-item limit can be checked before dispatching any request.
		var reqSeq []json.RawMessage
		if err := json.Unmarshal(body, &reqSeq); err != nil {
			s.writeRPCError(w, nil, JSONRPC_PARSE_ERROR, "invalid request batch")
			return
		}
		if len(reqSeq) == 0 || len(reqSeq) > MAX_BATCH_SIZE {
			s.writeRPCError(w, nil, JSONRPC_INVALID_BATCH, fmt.Sprintf("invalid request batch size (expected [1..%v])", MAX_BATCH_SIZE))
			return
		}

		s.Logger.Info("Received RPC request batch", "items", len(reqSeq))
		if !s.writeByte(w, '[') {
			return
		}

		for i, rawReq := range reqSeq {

			if i > 0 && !s.writeByte(w, ',') {
				return
			}

			var responded bool
			var req RPCRequest

			switch r.Context().Err() {
			case context.Canceled:
				return
			case context.DeadlineExceeded:
				s.Logger.Warn("RPC method dispatch timeout")
				if err := json.Unmarshal(rawReq, &req); err != nil {
					responded = s.writeRPCError(w, nil, JSONRPC_INVALID_REQUEST, "invalid request")
				} else {
					responded = s.writeRPCError(w, req.ID, JSONRPC_TIMEOUT_ERROR, "Request timed out")
				}
			default:
				if err := json.Unmarshal(rawReq, &req); err != nil {
					responded = s.writeRPCError(w, nil, JSONRPC_INVALID_REQUEST, "invalid request")
				} else {
					s.Logger.Debug("Dispatching RPC request", "method", req.Method)
					responded = s.dispatchOneRequest(w, r, req, budgetResp)
				}
			}

			if !responded {
				return
			}
		}
		s.writeByte(w, ']')

	default:
		w.Header().Set("Content-Type", "application/json")
		if json.Valid(body) {
			s.writeRPCError(w, nil, JSONRPC_INVALID_REQUEST, "invalid request")
		} else {
			s.writeRPCError(w, nil, JSONRPC_PARSE_ERROR, "Parse error")
		}

	}
}

// -----------------------------------------------------------------------------
// Individual Method Handlers
// -----------------------------------------------------------------------------

// Discovery: return the embedded specification.
func handleDiscover(s *Service, _ *http.Request, _ RPCRequest) (any, error) {
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	if err != nil {
		s.Logger.Error("Unable to read jsonrpc-discover content", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	var spec any
	if err := json.Unmarshal(data, &spec); err != nil {
		s.Logger.Error("Unable to unmarshal discovery spec JSON", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	return spec, nil
}

func handleListApplications(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListApplicationsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}
	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}
	// Cap limit to 10,000.
	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	apps, total, err := s.repository.ListApplications(r.Context(), repository.ApplicationFilter{}, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve applications from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if apps == nil {
		apps = []*model.Application{}
	}

	return api.ListResponse[*model.Application]{
		Data: apps,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}, nil
}

func handleGetApplication(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetApplicationParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	app, err := s.repository.GetApplication(r.Context(), params.Application)
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if app == nil {
		return nil, newRPCError(JSONRPC_APPLICATION_NOT_FOUND, "Application not found", nil)
	}

	return api.SingleResponse[*model.Application]{Data: app}, nil
}

func handleListEpochs(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListEpochsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	var epochFilter repository.EpochFilter
	if params.Status != nil {
		var status model.EpochStatus
		if err := status.Scan(*params.Status); err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch status: %v", err), nil)
		}
		epochFilter.Status = []model.EpochStatus{status}
	}

	epochs, total, err := s.repository.ListEpochs(r.Context(), params.Application, epochFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve epochs from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}

	if len(epochs) == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}
	if epochs == nil {
		epochs = []*model.Epoch{}
	}

	return api.ListResponse[*model.Epoch]{
		Data: epochs,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}, nil
}

func handleGetEpoch(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetEpochParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	index, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
	}

	epoch, err := s.repository.GetEpoch(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve epoch from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if epoch == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Epoch not found", nil)
	}

	return api.SingleResponse[*model.Epoch]{Data: epoch}, nil
}

func handleGetLastAcceptedEpochIndex(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetLastAcceptedEpochIndexParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	index, err := s.repository.GetLastAcceptedEpochIndex(r.Context(), params.Application)
	if errors.Is(err, repository.ErrNotFound) {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Epoch not found", nil)
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve epoch from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}

	return api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", index)}, nil
}

func handleListInputs(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListInputsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	// Create input filter based on params
	inputFilter := repository.InputFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
		}
		inputFilter.EpochIndex = &epochIndex
	}

	// Add sender filter if provided
	if params.Sender != nil {
		sender, err := config.ToAddressFromString(*params.Sender)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input sender address: %v", err), nil)
		}
		inputFilter.Sender = &sender
	}
	if params.TransactionHash != nil {
		transactionHash, err := config.ToHashFromString(*params.TransactionHash)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid transaction hash: %v", err), nil)
		}
		inputFilter.TransactionHash = &transactionHash
	}

	inputs, total, err := s.repository.ListInputs(r.Context(), params.Application, inputFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve inputs from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if len(inputs) == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}

	resultInputs := make([]*api.DecodedInput, 0, len(inputs))
	for _, in := range inputs {
		decoded, err := api.DecodeInput(in, s.inputABI)
		if err != nil {
			s.Logger.Error("Unable to decode Input", "app", params.Application, "index", in.Index, "err", err)
		}
		resultInputs = append(resultInputs, decoded)
	}

	return api.ListResponse[*api.DecodedInput]{
		Data: resultInputs,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}, nil
}

func handleGetInput(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetInputParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	index, err := config.ToIndexFromString(params.InputIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input index: %v", err), nil)
	}

	input, err := s.repository.GetInput(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve input from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if input == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Input not found", nil)
	}

	decoded, err := api.DecodeInput(input, s.inputABI)
	if err != nil {
		s.Logger.Error("Unable to decode Input", "app", params.Application, "index", input.Index, "err", err)
	}

	return api.SingleResponse[*api.DecodedInput]{Data: decoded}, nil
}

func handleGetProcessedInputCount(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetApplicationParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	processedInputs, err := s.repository.GetProcessedInputCount(r.Context(), params.Application)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, newRPCError(JSONRPC_APPLICATION_NOT_FOUND, "Application not found", nil)
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}

	return api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", processedInputs)}, nil
}

func handleListOutputs(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListOutputsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	// Create output filter based on params
	outputFilter := repository.OutputFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
		}
		outputFilter.EpochIndex = &epochIndex
	}

	if params.InputIndex != nil {
		inputIndex, err := config.ToIndexFromString(*params.InputIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input index: %v", err), nil)
		}
		outputFilter.InputIndex = &inputIndex
	}

	// Add output type filter if provided
	if params.OutputType != nil {
		outputType, err := api.ParseOutputType(*params.OutputType)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid output type: %v", err), nil)
		}
		outputFilter.OutputType = &outputType
	}

	// Add sender filter if provided
	if params.VoucherAddress != nil {
		voucherAddress, err := config.ToAddressFromString(*params.VoucherAddress)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid voucher address: %v", err), nil)
		}
		outputFilter.VoucherAddress = &voucherAddress
	}

	outputs, total, err := s.repository.ListOutputs(r.Context(), params.Application, outputFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve outputs from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}

	resultOutputs := make([]*api.DecodedOutput, 0, len(outputs))
	for _, out := range outputs {
		decoded, err := api.DecodeOutput(out, s.outputABI)
		if err != nil {
			s.Logger.Error("Unable to decode Output", "app", params.Application, "index", out.Index, "err", err)
		}
		resultOutputs = append(resultOutputs, decoded)
	}

	if len(resultOutputs) == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}

	return api.ListResponse[*api.DecodedOutput]{
		Data: resultOutputs,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}, nil
}

func handleGetOutput(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetOutputParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	index, err := config.ToIndexFromString(params.OutputIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid output index: %v", err), nil)
	}

	output, err := s.repository.GetOutput(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve output from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if output == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Output not found", nil)
	}

	decoded, err := api.DecodeOutput(output, s.outputABI)
	if err != nil {
		s.Logger.Error("Unable to decode Output", "app", params.Application, "index", output.Index, "err", err)
	}

	return api.SingleResponse[*api.DecodedOutput]{Data: decoded}, nil
}

func handleListReports(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListReportsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	// Create report filter based on params
	reportFilter := repository.ReportFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
		}
		reportFilter.EpochIndex = &epochIndex
	}

	if params.InputIndex != nil {
		inputIndex, err := config.ToIndexFromString(*params.InputIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input index: %v", err), nil)
		}
		reportFilter.InputIndex = &inputIndex
	}

	reports, total, err := s.repository.ListReports(r.Context(), params.Application, reportFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve reports from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}

	if len(reports) == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}
	if reports == nil {
		reports = []*model.Report{}
	}

	return api.ListResponse[*model.Report]{
		Data: reports,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}, nil
}

func handleGetReport(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetReportParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	index, err := config.ToIndexFromString(params.ReportIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid report index: %v", err), nil)
	}

	report, err := s.repository.GetReport(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve report from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if report == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Report not found", nil)
	}

	return api.SingleResponse[*model.Report]{Data: report}, nil
}

func handleListWithdrawals(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListWithdrawalsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}
	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	withdrawalFilter := repository.WithdrawalFilter{}
	if params.AccountIndex != nil {
		accountIndex, err := config.ToIndexFromString(*params.AccountIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid account index: %v", err), nil)
		}
		withdrawalFilter.AccountIndex = &accountIndex
	}

	withdrawals, total, err := s.repository.ListWithdrawals(
		r.Context(), params.Application, withdrawalFilter,
		repository.Pagination{Limit: params.Limit, Offset: params.Offset},
		params.Descending,
	)
	if err != nil {
		s.Logger.Error("Unable to retrieve withdrawals from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}

	if len(withdrawals) == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}
	if withdrawals == nil {
		withdrawals = []*model.Withdrawal{}
	}

	return api.ListResponse[*model.Withdrawal]{
		Data: withdrawals,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}, nil
}

func handleGetWithdrawal(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetWithdrawalParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	accountIndex, err := config.ToIndexFromString(params.AccountIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid account index: %v", err), nil)
	}

	withdrawal, err := s.repository.GetWithdrawal(r.Context(), params.Application, accountIndex)
	if err != nil {
		s.Logger.Error("Unable to retrieve withdrawal from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if withdrawal == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Withdrawal not found", nil)
	}

	return api.SingleResponse[*model.Withdrawal]{Data: withdrawal}, nil
}

func handleListTournaments(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListTournamentsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	// Create tournament filter based on params
	tournamentFilter := repository.TournamentFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
		}
		tournamentFilter.EpochIndex = &epochIndex
	}

	if params.Level != nil {
		level, err := config.ToIndexFromString(*params.Level)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid level: %v", err), nil)
		}
		tournamentFilter.Level = &level
	}

	if params.ParentTournamentAddress != nil {
		parentAddress, err := config.ToAddressFromString(*params.ParentTournamentAddress)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid parent tournament address: %v", err), nil)
		}
		tournamentFilter.ParentTournamentAddress = &parentAddress
	}

	if params.ParentMatchIDHash != nil {
		parentMatchIDHash, err := config.ToHashFromString(*params.ParentMatchIDHash)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid parent match ID hash: %v", err), nil)
		}
		tournamentFilter.ParentMatchIDHash = &parentMatchIDHash
	}

	tournaments, total, err := s.repository.ListTournaments(r.Context(), params.Application, tournamentFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve tournaments from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if len(tournaments) == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}
	if tournaments == nil {
		tournaments = []*model.Tournament{}
	}

	return api.ListResponse[*model.Tournament]{
		Data: tournaments,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}, nil
}

func handleGetTournament(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetTournamentParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	// Validate tournament address
	if _, err := config.ToAddressFromString(params.Address); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
	}

	tournament, err := s.repository.GetTournament(r.Context(), params.Application, params.Address)
	if err != nil {
		s.Logger.Error("Unable to retrieve tournament from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if tournament == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Tournament not found", nil)
	}

	return api.SingleResponse[*model.Tournament]{Data: tournament}, nil
}

func handleListCommitments(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListCommitmentsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	// Create commitment filter based on params
	commitmentFilter := repository.CommitmentFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
		}
		commitmentFilter.EpochIndex = &epochIndex
	}

	if params.TournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.TournamentAddress); err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
		}
		commitmentFilter.TournamentAddress = params.TournamentAddress
	}

	commitments, total, err := s.repository.ListCommitments(r.Context(), params.Application, commitmentFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve commitments from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if len(commitments) == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}
	if commitments == nil {
		commitments = []*model.Commitment{}
	}

	return api.ListResponse[*model.Commitment]{
		Data: commitments,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}, nil
}

func handleGetCommitment(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetCommitmentParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
	}

	if len(params.Commitment) == 0 {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid commitment hex: Empty string", nil)
	}
	if _, err := config.ToHashFromString(params.Commitment); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid commitment hex: %v", err), nil)
	}

	commitment, err := s.repository.GetCommitment(r.Context(), params.Application, epochIndex, params.TournamentAddress, params.Commitment)
	if err != nil {
		s.Logger.Error("Unable to retrieve commitment from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if commitment == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Commitment not found", nil)
	}

	return api.SingleResponse[*model.Commitment]{Data: commitment}, nil
}

func handleListMatches(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListMatchesParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	// Create match filter based on params
	matchFilter := repository.MatchFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
		}
		matchFilter.EpochIndex = &epochIndex
	}

	if params.TournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.TournamentAddress); err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
		}
		matchFilter.TournamentAddress = params.TournamentAddress
	}

	matches, total, err := s.repository.ListMatches(r.Context(), params.Application, matchFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve matches from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if len(matches) == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}
	if matches == nil {
		matches = []*model.Match{}
	}

	return api.ListResponse[*model.Match]{
		Data: matches,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}, nil
}

func handleGetMatch(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetMatchParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
	}

	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid ID hash: %v", err), nil)
	}

	match, err := s.repository.GetMatch(r.Context(), params.Application, epochIndex, params.TournamentAddress, params.IDHash)
	if err != nil {
		s.Logger.Error("Unable to retrieve match from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if match == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Match not found", nil)
	}

	return api.SingleResponse[*model.Match]{Data: match}, nil
}

func handleListMatchAdvances(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListMatchAdvancesParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	// Create match advance filter based on params
	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
	}

	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid ID hash: %v", err), nil)
	}

	pagination := repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}
	matchAdvances, total, err := s.repository.ListMatchAdvances(r.Context(), params.Application, epochIndex,
		params.TournamentAddress, params.IDHash, pagination, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve match advances from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if len(matchAdvances) == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}
	if matchAdvances == nil {
		matchAdvances = []*model.MatchAdvanced{}
	}

	return api.ListResponse[*model.MatchAdvanced]{
		Data: matchAdvances,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}, nil
}

func handleGetMatchAdvanced(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetMatchAdvancedParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
	}

	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
	}

	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid ID hash: %v", err), nil)
	}

	if _, err := config.ToHashFromString(params.Parent); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid parent hash: %v", err), nil)
	}

	matchAdvanced, err := s.repository.GetMatchAdvanced(r.Context(), params.Application, epochIndex,
		params.TournamentAddress, params.IDHash, params.Parent[2:]) // TODO: use parsed value
	if err != nil {
		s.Logger.Error("Unable to retrieve match advanced from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}
	if matchAdvanced == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Match advanced not found", nil)
	}

	return api.SingleResponse[*model.MatchAdvanced]{Data: matchAdvanced}, nil
}

func handleGetChainID(s *Service, r *http.Request, _ RPCRequest) (any, error) {
	config, err := repository.LoadNodeConfig[evmreader.PersistentConfig](r.Context(), s.repository, evmreader.EvmReaderConfigKey)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "EVM Reader config not found", nil)
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve evmreader config from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	}

	return api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", config.Value.ChainID)}, nil
}

func handleGetNodeVersion(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
	return api.SingleResponse[string]{Data: version.BuildVersion}, nil
}

func (s *Service) applicationAbsentOrError(
	r *http.Request,
	validatedNameOrAddress string,
) error {
	app, err := s.repository.GetApplication(r.Context(), validatedNameOrAddress)
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		return newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
	} else if app == nil {
		return newRPCError(JSONRPC_APPLICATION_NOT_FOUND, "Application not found", nil)
	}
	return nil
}
