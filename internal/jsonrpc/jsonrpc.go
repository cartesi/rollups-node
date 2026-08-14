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
	"math"
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
	// Maximum response size for a single request or cumulative response size for
	// all entries in a batch (10 MB).
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
	// Response size limit exceeded: the buffered-response budget was not enough
	// for a single response or all responses in a batch.
	JSONRPC_RESPONSE_SIZE_LIMIT_EXCEEDED int = -31003 //nolint: revive
)

type rpcHandler = func(*Service, *http.Request, RPCRequest) (any, error)
type dispatchTable = map[string]rpcHandler

var jsonrpcHandlers = dispatchTable{
	"rpc.discover":                            handleDiscover,
	"cartesi_listApplications":                handleListApplications,
	"cartesi_getApplication":                  handleGetApplication,
	"cartesi_listEpochs":                      handleListEpochs,
	"cartesi_getEpoch":                        handleGetEpoch,
	"cartesi_getEpochByVirtualIndex":          handleGetEpochByVirtualIndex,
	"cartesi_getLastAcceptedEpochIndex":       handleGetLastAcceptedEpochIndex,
	"cartesi_listInputs":                      handleListInputs,
	"cartesi_getInput":                        handleGetInput,
	"cartesi_getProcessedInputCount":          handleGetProcessedInputCount,
	"cartesi_getExecutedOutputCount":          handleGetExecutedOutputCount,
	"cartesi_getPendingExecutableOutputCount": handleGetPendingExecutableOutputCount,
	"cartesi_listOutputs":                     handleListOutputs,
	"cartesi_getOutput":                       handleGetOutput,
	"cartesi_listReports":                     handleListReports,
	"cartesi_getReport":                       handleGetReport,
	"cartesi_listWithdrawals":                 handleListWithdrawals,
	"cartesi_getWithdrawal":                   handleGetWithdrawal,
	"cartesi_listTournaments":                 handleListTournaments,
	"cartesi_getTournament":                   handleGetTournament,
	"cartesi_listCommitments":                 handleListCommitments,
	"cartesi_getCommitment":                   handleGetCommitment,
	"cartesi_listMatches":                     handleListMatches,
	"cartesi_getMatch":                        handleGetMatch,
	"cartesi_listMatchAdvances":               handleListMatchAdvances,
	"cartesi_getMatchAdvance":                 handleGetMatchAdvance,
	"cartesi_getNodeInfo":                     handleGetNodeInfo,
	"cartesi_getChainId":                      handleGetChainID,
	"cartesi_getNodeVersion":                  handleGetNodeVersion,
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
	err := writeRPCError(w, id, code, message)
	return s.handleWriteResponse(err)
}

func (s *Service) handleRequest(w io.Writer, r *http.Request, req RPCRequest) error {
	switch req.ID.(type) {
	case nil, string, float64:
	default:
		return writeRPCError(w, nil, JSONRPC_INVALID_REQUEST, "invalid request")
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		return writeRPCError(w, req.ID, JSONRPC_INVALID_REQUEST, "invalid request")
	}
	fn, ok := jsonrpcHandlers[req.Method]
	if !ok {
		s.Logger.Debug("RPC method not found", "method", req.Method)
		return writeRPCError(w, req.ID, JSONRPC_METHOD_NOT_FOUND, "Method not found")
	}

	result, err := fn(s, r, req)
	if err == nil {
		return writeRPCResult(w, req.ID, result)
	}

	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		// RPC errors describe expected client-facing failures. Do not log them at
		// error level; unexpected failures are logged below before being hidden.
		return writeRPCError(w, req.ID, rpcErr.Code, rpcErr.Message)
	}

	s.Logger.Error("RPC method failed", "method", req.Method, "error", err)
	return writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error")
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
	return s.discoverSpec, nil
}

func handleListApplications(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListApplicationsParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
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
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
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
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	app, err := s.repository.GetApplication(r.Context(), params.Application)
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if app == nil {
		return nil, newRPCError(JSONRPC_APPLICATION_NOT_FOUND, "Application not found")
	}

	return api.SingleResponse[*model.Application]{Data: app}, nil
}

func handleListEpochs(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListEpochsParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
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
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	var epochFilter repository.EpochFilter
	indexRange, err := parseIndexRange(params.From, params.To)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, err.Error())
	}
	epochFilter.IndexRange = indexRange
	if params.Status != nil {
		if len(*params.Status) == 0 {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid epoch status: expected at least one status")
		}
		statuses := make([]model.EpochStatus, 0, len(*params.Status))
		for _, value := range *params.Status {
			var status model.EpochStatus
			if err := status.Scan(value); err != nil {
				return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch status: %v", err))
			}
			statuses = append(statuses, status)
		}
		epochFilter.Status = statuses
	}

	epochs, total, err := s.repository.ListEpochs(r.Context(), params.Application, epochFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve epochs from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
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
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	index, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
	}

	epoch, err := s.repository.GetEpoch(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve epoch from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if epoch == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Epoch not found")
	}

	return api.SingleResponse[*model.Epoch]{Data: epoch}, nil
}

func handleGetEpochByVirtualIndex(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetEpochByVirtualIndexParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	index, err := config.ToIndexFromString(params.VirtualIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid virtual index: %v", err))
	}

	epoch, err := s.repository.GetEpochByVirtualIndex(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve epoch from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if epoch == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Epoch not found")
	}

	return api.SingleResponse[*model.Epoch]{Data: epoch}, nil
}

func handleGetLastAcceptedEpochIndex(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetLastAcceptedEpochIndexParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	index, err := s.repository.GetLastAcceptedEpochIndex(r.Context(), params.Application)
	if errors.Is(err, repository.ErrNotFound) {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Epoch not found")
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve epoch from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}

	return api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", index)}, nil
}

func handleListInputs(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListInputsParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
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
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	// Create input filter based on params
	inputFilter := repository.InputFilter{}
	indexRange, err := parseIndexRange(params.From, params.To)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, err.Error())
	}
	inputFilter.IndexRange = indexRange
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
		}
		inputFilter.EpochIndex = &epochIndex
	}

	// Add sender filter if provided
	if params.Sender != nil {
		sender, err := config.ToAddressFromString(*params.Sender)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input sender address: %v", err))
		}
		inputFilter.Sender = &sender
	}
	if params.TransactionHash != nil {
		transactionHash, err := config.ToHashFromString(*params.TransactionHash)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid transaction hash: %v", err))
		}
		inputFilter.TransactionHash = &transactionHash
	}

	inputs, total, err := s.repository.ListInputs(r.Context(), params.Application, inputFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve inputs from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
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
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	index, err := config.ToIndexFromString(params.InputIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input index: %v", err))
	}

	input, err := s.repository.GetInput(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve input from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if input == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Input not found")
	}

	decoded, err := api.DecodeInput(input, s.inputABI)
	if err != nil {
		s.Logger.Error("Unable to decode Input", "app", params.Application, "index", input.Index, "err", err)
	}

	return api.SingleResponse[*api.DecodedInput]{Data: decoded}, nil
}

func handleGetProcessedInputCount(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetApplicationParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	processedInputs, err := s.repository.GetProcessedInputCount(r.Context(), params.Application)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, newRPCError(JSONRPC_APPLICATION_NOT_FOUND, "Application not found")
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}

	return api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", processedInputs)}, nil
}

func handleGetExecutedOutputCount(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetApplicationParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	count, err := s.repository.GetNumberOfExecutedOutputs(r.Context(), params.Application)
	if err != nil {
		s.Logger.Error("Unable to retrieve executed output count from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if count == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}

	return api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", count)}, nil
}

func handleGetPendingExecutableOutputCount(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetApplicationParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	count, err := s.repository.GetNumberOfPendingExecutableOutputs(r.Context(), params.Application)
	if err != nil {
		s.Logger.Error("Unable to retrieve pending executable output count from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if count == 0 {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
	}

	return api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", count)}, nil
}

func handleListOutputs(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListOutputsParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
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
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	// Create output filter based on params
	outputFilter := repository.OutputFilter{}
	indexRange, err := parseIndexRange(params.From, params.To)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, err.Error())
	}
	outputFilter.IndexRange = indexRange
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
		}
		outputFilter.EpochIndex = &epochIndex
	}

	if params.InputIndex != nil {
		inputIndex, err := config.ToIndexFromString(*params.InputIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input index: %v", err))
		}
		outputFilter.InputIndex = &inputIndex
	}

	// Add output type filter if provided
	if params.OutputType != nil {
		if len(*params.OutputType) == 0 {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid output type: expected at least one selector")
		}
		outputTypes := make([][]byte, 0, len(*params.OutputType))
		for _, selector := range *params.OutputType {
			outputType, err := api.ParseOutputType(selector)
			if err != nil {
				return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid output type: %v", err))
			}
			outputTypes = append(outputTypes, outputType)
		}
		outputFilter.OutputType = &outputTypes
	}
	outputFilter.Executed = params.Executed

	// Add sender filter if provided
	if params.VoucherAddress != nil {
		voucherAddress, err := config.ToAddressFromString(*params.VoucherAddress)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid voucher address: %v", err))
		}
		outputFilter.VoucherAddress = &voucherAddress
	}

	outputs, total, err := s.repository.ListOutputs(r.Context(), params.Application, outputFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve outputs from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
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
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	index, err := config.ToIndexFromString(params.OutputIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid output index: %v", err))
	}

	output, err := s.repository.GetOutput(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve output from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if output == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Output not found")
	}

	decoded, err := api.DecodeOutput(output, s.outputABI)
	if err != nil {
		s.Logger.Error("Unable to decode Output", "app", params.Application, "index", output.Index, "err", err)
	}

	return api.SingleResponse[*api.DecodedOutput]{Data: decoded}, nil
}

func handleListReports(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListReportsParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
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
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	// Create report filter based on params
	reportFilter := repository.ReportFilter{}
	indexRange, err := parseIndexRange(params.From, params.To)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, err.Error())
	}
	reportFilter.IndexRange = indexRange
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
		}
		reportFilter.EpochIndex = &epochIndex
	}

	if params.InputIndex != nil {
		inputIndex, err := config.ToIndexFromString(*params.InputIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input index: %v", err))
		}
		reportFilter.InputIndex = &inputIndex
	}

	reports, total, err := s.repository.ListReports(r.Context(), params.Application, reportFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve reports from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
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
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	index, err := config.ToIndexFromString(params.ReportIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid report index: %v", err))
	}

	report, err := s.repository.GetReport(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve report from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if report == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Report not found")
	}

	return api.SingleResponse[*model.Report]{Data: report}, nil
}

func handleListWithdrawals(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListWithdrawalsParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}
	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	withdrawalFilter := repository.WithdrawalFilter{}
	if params.AccountIndex != nil {
		accountIndex, err := config.ToIndexFromString(*params.AccountIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid account index: %v", err))
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
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
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
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	accountIndex, err := config.ToIndexFromString(params.AccountIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid account index: %v", err))
	}

	withdrawal, err := s.repository.GetWithdrawal(r.Context(), params.Application, accountIndex)
	if err != nil {
		s.Logger.Error("Unable to retrieve withdrawal from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if withdrawal == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Withdrawal not found")
	}

	return api.SingleResponse[*model.Withdrawal]{Data: withdrawal}, nil
}

func handleListTournaments(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListTournamentsParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
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
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	// Create tournament filter based on params
	tournamentFilter := repository.TournamentFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
		}
		tournamentFilter.EpochIndex = &epochIndex
	}

	if params.Level != nil {
		level, err := config.ToIndexFromString(*params.Level)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid level: %v", err))
		}
		tournamentFilter.Level = &level
	}

	if params.ParentTournamentAddress != nil {
		parentAddress, err := config.ToAddressFromString(*params.ParentTournamentAddress)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid parent tournament address: %v", err))
		}
		tournamentFilter.ParentTournamentAddress = &parentAddress
	}

	if params.ParentMatchIDHash != nil {
		parentMatchIDHash, err := config.ToHashFromString(*params.ParentMatchIDHash)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid parent match ID hash: %v", err))
		}
		tournamentFilter.ParentMatchIDHash = &parentMatchIDHash
	}

	tournaments, total, err := s.repository.ListTournaments(r.Context(), params.Application, tournamentFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve tournaments from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
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
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	// Validate tournament address
	if _, err := config.ToAddressFromString(params.Address); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err))
	}

	tournament, err := s.repository.GetTournament(r.Context(), params.Application, params.Address)
	if err != nil {
		s.Logger.Error("Unable to retrieve tournament from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if tournament == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Tournament not found")
	}

	return api.SingleResponse[*model.Tournament]{Data: tournament}, nil
}

func handleListCommitments(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListCommitmentsParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
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
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	// Create commitment filter based on params
	commitmentFilter := repository.CommitmentFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
		}
		commitmentFilter.EpochIndex = &epochIndex
	}

	if params.TournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.TournamentAddress); err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err))
		}
		commitmentFilter.TournamentAddress = params.TournamentAddress
	}

	commitments, total, err := s.repository.ListCommitments(r.Context(), params.Application, commitmentFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve commitments from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
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
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err))
	}

	if len(params.Commitment) == 0 {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid commitment hex: Empty string")
	}
	if _, err := config.ToHashFromString(params.Commitment); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid commitment hex: %v", err))
	}

	commitment, err := s.repository.GetCommitment(r.Context(), params.Application, epochIndex, params.TournamentAddress, params.Commitment)
	if err != nil {
		s.Logger.Error("Unable to retrieve commitment from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if commitment == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Commitment not found")
	}

	return api.SingleResponse[*model.Commitment]{Data: commitment}, nil
}

func handleListMatches(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListMatchesParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
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
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	// Create match filter based on params
	matchFilter := repository.MatchFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
		}
		matchFilter.EpochIndex = &epochIndex
	}

	if params.TournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.TournamentAddress); err != nil {
			return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err))
		}
		matchFilter.TournamentAddress = params.TournamentAddress
	}

	matches, total, err := s.repository.ListMatches(r.Context(), params.Application, matchFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve matches from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
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
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err))
	}

	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid ID hash: %v", err))
	}

	match, err := s.repository.GetMatch(r.Context(), params.Application, epochIndex, params.TournamentAddress, params.IDHash)
	if err != nil {
		s.Logger.Error("Unable to retrieve match from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if match == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Match not found")
	}

	return api.SingleResponse[*model.Match]{Data: match}, nil
}

func handleListMatchAdvances(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.ListMatchAdvancesParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
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
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	// Create match advance filter based on params
	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err))
	}

	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid ID hash: %v", err))
	}

	pagination := repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}
	matchAdvances, total, err := s.repository.ListMatchAdvances(r.Context(), params.Application, epochIndex,
		params.TournamentAddress, params.IDHash, pagination, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve match advances from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
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

func handleGetMatchAdvance(s *Service, r *http.Request, req RPCRequest) (any, error) {
	var params api.GetMatchAdvanceParams
	if err := api.UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, "Invalid parameters")
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err))
	}

	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err))
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err))
	}

	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid ID hash: %v", err))
	}

	parent, err := config.ToHashFromString(params.Parent)
	if err != nil {
		return nil, newRPCError(JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid parent hash: %v", err))
	}

	matchAdvanced, err := s.repository.GetMatchAdvanced(r.Context(), params.Application, epochIndex,
		params.TournamentAddress, params.IDHash, parent.Hex()[2:])
	if err != nil {
		s.Logger.Error("Unable to retrieve match advanced from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}
	if matchAdvanced == nil {
		if err := s.applicationAbsentOrError(r, params.Application); err != nil {
			return nil, err
		}
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "Match advanced not found")
	}

	return api.SingleResponse[*model.MatchAdvanced]{Data: matchAdvanced}, nil
}

func handleGetNodeInfo(s *Service, r *http.Request, _ RPCRequest) (any, error) {
	cfg, err := repository.LoadNodeConfig[evmreader.PersistentConfig](r.Context(), s.repository, evmreader.EvmReaderConfigKey)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "EVM Reader config not found")
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve evmreader config from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}

	return api.SingleResponse[api.NodeInfo]{Data: api.NodeInfo{
		ChainID:      fmt.Sprintf("0x%x", cfg.Value.ChainID),
		Version:      version.BuildVersion,
		DefaultBlock: string(cfg.Value.DefaultBlock), // FINALIZED | SAFE | LATEST | PENDING
	}}, nil
}

func handleGetChainID(s *Service, r *http.Request, _ RPCRequest) (any, error) {
	config, err := repository.LoadNodeConfig[evmreader.PersistentConfig](r.Context(), s.repository, evmreader.EvmReaderConfigKey)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, newRPCError(JSONRPC_RESOURCE_NOT_FOUND, "EVM Reader config not found")
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve evmreader config from repository", "err", err)
		return nil, newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	}

	return api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", config.Value.ChainID)}, nil
}

func handleGetNodeVersion(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
	return api.SingleResponse[string]{Data: version.BuildVersion}, nil
}

func parseIndexRange(from, to *string) (*repository.Range, error) {
	if from == nil && to == nil {
		return nil, nil
	}

	indexRange := repository.Range{End: math.MaxUint64}
	if from != nil {
		value, err := config.ToIndexFromString(*from)
		if err != nil {
			return nil, fmt.Errorf("invalid from index: %w", err)
		}
		indexRange.Start = value
	}
	if to != nil {
		value, err := config.ToIndexFromString(*to)
		if err != nil {
			return nil, fmt.Errorf("invalid to index: %w", err)
		}
		indexRange.End = value
	}
	if indexRange.Start > indexRange.End {
		return nil, fmt.Errorf("invalid index range: from must be less than or equal to to")
	}
	return &indexRange, nil
}

func (s *Service) applicationAbsentOrError(
	r *http.Request,
	validatedNameOrAddress string,
) error {
	app, err := s.repository.GetApplication(r.Context(), validatedNameOrAddress)
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		return newRPCError(JSONRPC_INTERNAL_ERROR, "Internal server error")
	} else if app == nil {
		return newRPCError(JSONRPC_APPLICATION_NOT_FOUND, "Application not found")
	}
	return nil
}
