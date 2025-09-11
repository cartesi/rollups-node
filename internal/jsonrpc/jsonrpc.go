// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/version"
)

//go:embed jsonrpc-discover.json
var discoverSpec embed.FS

const (
	// Maximum allowed body size (1 MB).
	MAX_BODY_SIZE = 1 << 20
	// Maximum amount of items to list (10,000).
	LIST_ITEM_LIMIT = 10000
)

const (
	JSONRPC_RESOURCE_NOT_FOUND int = -32001
	JSONRPC_PARSE_ERROR        int = -32700
	JSONRPC_INVALID_REQUEST    int = -32600
	JSONRPC_METHOD_NOT_FOUND   int = -32601
	JSONRPC_INVALID_PARAMS     int = -32602
	JSONRPC_INTERNAL_ERROR     int = -32603
)

type rpcHandler = func(*Service, http.ResponseWriter, *http.Request, RPCRequest)
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
	"cartesi_getChainId":                handleGetChainId,
	"cartesi_getNodeVersion":            handleGetNodeVersion,
}

// -----------------------------------------------------------------------------
// Dispatching JSON‑RPC methods
// -----------------------------------------------------------------------------

func (s *Service) handleRPC(w http.ResponseWriter, r *http.Request) {
	// Limit request body size and ensure it is closed.
	r.Body = http.MaxBytesReader(w, r.Body, MAX_BODY_SIZE)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	var req RPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	s.Logger.Info(fmt.Sprintf("Received RPC request: %s", req.Method))
	if fn, ok := jsonrpcHandlers[req.Method]; ok {
		fn(s, w, r, req)
	} else {
		s.Logger.Info(fmt.Sprintf("RPC method not found: %s", req.Method))
		writeRPCError(w, req.ID, JSONRPC_METHOD_NOT_FOUND, "Method not found", nil)
	}
}

// -----------------------------------------------------------------------------
// Individual Method Handlers
// -----------------------------------------------------------------------------

// Discovery: return the embedded specification.
func handleDiscover(s *Service, w http.ResponseWriter, _ *http.Request, req RPCRequest) {
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	if err != nil {
		s.Logger.Error("Unable to read jsonrpc-discover content", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	var spec any
	if err := json.Unmarshal(data, &spec); err != nil {
		s.Logger.Error("Unable to unmarshal discovery spec JSON", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	writeRPCResult(w, req.ID, spec)
}

func handleListApplications(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params ListApplicationsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}
	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = 50
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
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if apps == nil {
		apps = []*model.Application{}
	}

	// Create result with proper pagination format per spec
	result := struct {
		Data       []*model.Application `json:"data"`
		Pagination struct {
			TotalCount uint64 `json:"total_count"`
			Limit      uint64 `json:"limit"`
			Offset     uint64 `json:"offset"`
		} `json:"pagination"`
	}{
		Data: apps,
		Pagination: struct {
			TotalCount uint64 `json:"total_count"`
			Limit      uint64 `json:"limit"`
			Offset     uint64 `json:"offset"`
		}{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}

	writeRPCResult(w, req.ID, result)
}

func handleGetApplication(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params GetApplicationParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	app, err := s.repository.GetApplication(r.Context(), params.Application)
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if app == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Application not found", nil)
		return
	}

	// Return in the format specified in the OpenRPC spec
	result := struct {
		Data *model.Application `json:"data"`
	}{
		Data: app,
	}

	writeRPCResult(w, req.ID, result)
}

func handleListEpochs(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params ListEpochsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = 50
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	var epochFilter repository.EpochFilter
	if params.Status != nil {
		var status model.EpochStatus
		if err := status.Scan(*params.Status); err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch status: %v", err), nil)
			return
		}
		epochFilter.Status = &status
	}

	epochs, total, err := s.repository.ListEpochs(r.Context(), params.Application, epochFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve epochs from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}

	if len(epochs) == 0 && !s.applicationExists(w, r, req, params.Application) {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Application not found", nil)
		return
	}
	if epochs == nil {
		epochs = []*model.Epoch{}
	}

	// Format response according to spec
	result := struct {
		Data       []*model.Epoch `json:"data"`
		Pagination struct {
			TotalCount uint64 `json:"total_count"`
			Limit      uint64 `json:"limit"`
			Offset     uint64 `json:"offset"`
		} `json:"pagination"`
	}{
		Data: epochs,
		Pagination: struct {
			TotalCount uint64 `json:"total_count"`
			Limit      uint64 `json:"limit"`
			Offset     uint64 `json:"offset"`
		}{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}

	writeRPCResult(w, req.ID, result)
}

func (s *Service) applicationExists(
	w http.ResponseWriter,
	r *http.Request,
	req RPCRequest,
	validatedNameOrAddress string,
) bool {
	app, err := s.repository.GetApplication(r.Context(), validatedNameOrAddress)
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return false
	}
	return app != nil
}

func parseIndex(indexString string, field string) (uint64, error) {
	if len(indexString) < 3 || (!strings.HasPrefix(indexString, "0x") && !strings.HasPrefix(indexString, "0X")) {
		return 0, fmt.Errorf("invalid %s: expected hex encoded value", field)
	}
	str := indexString[2:]
	index, err := strconv.ParseUint(str, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %v", field, "error parsing")
	}
	return index, nil
}

func handleGetEpoch(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params GetEpochParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	index, err := parseIndex(params.EpochIndex, "epoch_index")
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, err.Error(), nil)
		return
	}

	epoch, err := s.repository.GetEpoch(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve epoch from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if epoch == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Epoch not found", nil)
		return
	}

	// Format response according to spec
	result := struct {
		Data *model.Epoch `json:"data"`
	}{
		Data: epoch,
	}

	writeRPCResult(w, req.ID, result)
}

func handleGetLastAcceptedEpochIndex(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params GetLastAcceptedEpochIndexParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	index, err := s.repository.GetLastAcceptedEpochIndex(r.Context(), params.Application)
	if errors.Is(err, repository.ErrNotFound) {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Epoch not found", nil)
		return
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve epoch from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}

	// Format response according to spec
	result := struct {
		Data string `json:"data"`
	}{
		Data: fmt.Sprintf("0x%x", index),
	}

	writeRPCResult(w, req.ID, result)
}

func handleListInputs(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params ListInputsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = 50
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	// Create input filter based on params
	inputFilter := repository.InputFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := parseIndex(*params.EpochIndex, "epoch_index")
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, err.Error(), nil)
			return
		}
		inputFilter.EpochIndex = &epochIndex
	}

	// Add sender filter if provided
	if params.Sender != nil {
		sender, err := config.ToAddressFromString(*params.Sender)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input sender address: %v", err), nil)
			return
		}
		inputFilter.Sender = &sender
	}

	inputs, total, err := s.repository.ListInputs(r.Context(), params.Application, inputFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve inputs from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if len(inputs) == 0 && !s.applicationExists(w, r, req, params.Application) {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Application not found", nil)
		return
	}

	var resultInputs []*DecodedInput
	for _, in := range inputs {
		decoded, err := DecodeInput(in, s.inputABI)
		if err != nil {
			s.Logger.Error("Unable to decode Input", "app", params.Application, "index", in.Index, "err", err)
		}
		resultInputs = append(resultInputs, decoded)
	}
	if resultInputs == nil {
		resultInputs = []*DecodedInput{}
	}

	// Format response according to spec
	result := struct {
		Data       []*DecodedInput `json:"data"`
		Pagination struct {
			TotalCount uint64 `json:"total_count"`
			Limit      uint64 `json:"limit"`
			Offset     uint64 `json:"offset"`
		} `json:"pagination"`
	}{
		Data: resultInputs,
		Pagination: struct {
			TotalCount uint64 `json:"total_count"`
			Limit      uint64 `json:"limit"`
			Offset     uint64 `json:"offset"`
		}{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}

	writeRPCResult(w, req.ID, result)
}

func handleGetInput(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params GetInputParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	index, err := parseIndex(params.InputIndex, "input_index")
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, err.Error(), nil)
		return
	}

	input, err := s.repository.GetInput(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve input from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if input == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Input not found", nil)
		return
	}

	decoded, err := DecodeInput(input, s.inputABI)
	if err != nil {
		s.Logger.Error("Unable to decode Input", "app", params.Application, "index", input.Index, "err", err)
	}

	// Format response according to spec
	response := struct {
		Data *DecodedInput `json:"data"`
	}{
		Data: decoded,
	}

	writeRPCResult(w, req.ID, response)
}

func handleGetProcessedInputCount(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params GetApplicationParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	processedInputs, err := s.repository.GetProcessedInputs(r.Context(), params.Application)
	if errors.Is(err, repository.ErrNotFound) {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Application not found", nil)
		return
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}

	// Return processed input count as specified in the spec
	result := struct {
		ProcessedInputs string `json:"data"`
	}{
		ProcessedInputs: fmt.Sprintf("0x%x", processedInputs),
	}

	writeRPCResult(w, req.ID, result)
}

func ParseOutputType(s string) ([]byte, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if len(s) != 8 { // nolint: mnd
		return []byte{}, fmt.Errorf("invalid output type: expected exactly 4 bytes")
	}
	// Decode the hex string into bytes.
	b, err := hex.DecodeString(s)
	if err != nil {
		return []byte{}, err
	}
	return b, nil
}

func handleListOutputs(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params ListOutputsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = 50
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	// Create output filter based on params
	outputFilter := repository.OutputFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := parseIndex(*params.EpochIndex, "epoch_index")
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, err.Error(), nil)
			return
		}
		outputFilter.EpochIndex = &epochIndex
	}

	if params.InputIndex != nil {
		inputIndex, err := parseIndex(*params.InputIndex, "input_index")
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, err.Error(), nil)
			return
		}
		outputFilter.InputIndex = &inputIndex
	}

	// Add output type filter if provided
	if params.OutputType != nil {
		outputType, err := ParseOutputType(*params.OutputType)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid output type: %v", err), nil)
			return
		}
		outputFilter.OutputType = &outputType
	}

	// Add sender filter if provided
	if params.VoucherAddress != nil {
		voucherAddress, err := config.ToAddressFromString(*params.VoucherAddress)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid voucher address: %v", err), nil)
			return
		}
		outputFilter.VoucherAddress = &voucherAddress
	}

	outputs, total, err := s.repository.ListOutputs(r.Context(), params.Application, outputFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve outputs from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}

	var resultOutputs []*DecodedOutput
	for _, out := range outputs {
		decoded, err := DecodeOutput(out, s.outputABI)
		if err != nil {
			s.Logger.Error("Unable to decode Output", "app", params.Application, "index", out.Index, "err", err)
		}
		resultOutputs = append(resultOutputs, decoded)
	}

	if len(resultOutputs) == 0 && !s.applicationExists(w, r, req, params.Application) {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Application not found", nil)
		return
	}
	if resultOutputs == nil {
		resultOutputs = []*DecodedOutput{}
	}

	// Format response according to spec
	result := struct {
		Data       []*DecodedOutput `json:"data"`
		Pagination struct {
			TotalCount uint64 `json:"total_count"`
			Limit      uint64 `json:"limit"`
			Offset     uint64 `json:"offset"`
		} `json:"pagination"`
	}{
		Data: resultOutputs,
		Pagination: struct {
			TotalCount uint64 `json:"total_count"`
			Limit      uint64 `json:"limit"`
			Offset     uint64 `json:"offset"`
		}{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}

	writeRPCResult(w, req.ID, result)
}

func handleGetOutput(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params GetOutputParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	index, err := parseIndex(params.OutputIndex, "output_index")
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, err.Error(), nil)
		return
	}

	output, err := s.repository.GetOutput(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve output from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if output == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Output not found", nil)
		return
	}

	decoded, err := DecodeOutput(output, s.outputABI)
	if err != nil {
		s.Logger.Error("Unable to decode Output", "app", params.Application, "index", output.Index, "err", err)
	}

	// Format response according to spec
	response := struct {
		Data *DecodedOutput `json:"data"`
	}{
		Data: decoded,
	}

	writeRPCResult(w, req.ID, response)
}

func handleListReports(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params ListReportsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Use default values if not provided
	if params.Limit <= 0 {
		params.Limit = 50
	}

	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	// Create report filter based on params
	reportFilter := repository.ReportFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := parseIndex(*params.EpochIndex, "epoch_index")
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, err.Error(), nil)
			return
		}
		reportFilter.EpochIndex = &epochIndex
	}

	if params.InputIndex != nil {
		inputIndex, err := parseIndex(*params.InputIndex, "input_index")
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, err.Error(), nil)
			return
		}
		reportFilter.InputIndex = &inputIndex
	}

	reports, total, err := s.repository.ListReports(r.Context(), params.Application, reportFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve reports from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}

	if len(reports) == 0 && !s.applicationExists(w, r, req, params.Application) {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Application not found", nil)
		return
	}
	if reports == nil {
		reports = []*model.Report{}
	}

	// Format response according to spec
	result := struct {
		Data       []*model.Report `json:"data"`
		Pagination struct {
			TotalCount uint64 `json:"total_count"`
			Limit      uint64 `json:"limit"`
			Offset     uint64 `json:"offset"`
		} `json:"pagination"`
	}{
		Data: reports,
		Pagination: struct {
			TotalCount uint64 `json:"total_count"`
			Limit      uint64 `json:"limit"`
			Offset     uint64 `json:"offset"`
		}{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	}

	writeRPCResult(w, req.ID, result)
}

func handleGetReport(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params GetReportParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	// Validate application parameter
	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	index, err := parseIndex(params.ReportIndex, "report_index")
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, err.Error(), nil)
		return
	}

	report, err := s.repository.GetReport(r.Context(), params.Application, index)
	if err != nil {
		s.Logger.Error("Unable to retrieve report from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if report == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Report not found", nil)
		return
	}

	// Format response according to spec
	response := struct {
		Data *model.Report `json:"data"`
	}{
		Data: report,
	}

	writeRPCResult(w, req.ID, response)
}

func handleGetChainId(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {

	config, err := repository.LoadNodeConfig[evmreader.PersistentConfig](r.Context(), s.repository, evmreader.EvmReaderConfigKey)
	if errors.Is(err, repository.ErrNotFound) {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "EVM Reader config not found", nil)
		return
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve evmreader config from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}

	result := struct {
		Data string `json:"data"`
	}{
		Data: fmt.Sprintf("0x%x", config.Value.ChainID),
	}

	writeRPCResult(w, req.ID, result)
}

func handleGetNodeVersion(s *Service, w http.ResponseWriter, _ *http.Request, req RPCRequest) {
	result := struct {
		Data string `json:"data"`
	}{
		Data: version.BuildVersion,
	}

	writeRPCResult(w, req.ID, result)
}
