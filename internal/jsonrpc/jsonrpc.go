// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
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
	// Maximum amount of items to list (10,000).
	LIST_ITEM_LIMIT = 10000 //nolint: revive
	// Default amount of item on a list (50)
	LIST_ITEM_DEFAULT = 50 //nolint: revive
)

const (
	JSONRPC_RESOURCE_NOT_FOUND int = -32001 //nolint: revive
	JSONRPC_PARSE_ERROR        int = -32700 //nolint: revive
	JSONRPC_INVALID_REQUEST    int = -32600 //nolint: revive
	JSONRPC_METHOD_NOT_FOUND   int = -32601 //nolint: revive
	JSONRPC_INVALID_PARAMS     int = -32602 //nolint: revive
	JSONRPC_INTERNAL_ERROR     int = -32603 //nolint: revive
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
	var params api.ListApplicationsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
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
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if apps == nil {
		apps = []*model.Application{}
	}

	writeRPCResult(w, req.ID, api.ListResponse[*model.Application]{
		Data: apps,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	})
}

func handleGetApplication(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetApplicationParams
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

	writeRPCResult(w, req.ID, api.SingleResponse[*model.Application]{Data: app})
}

func handleListEpochs(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.ListEpochsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
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
		epochFilter.Status = []model.EpochStatus{status}
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

	if len(epochs) == 0 && s.applicationAbsentOrError(w, r, req, params.Application) {
		return
	}
	if epochs == nil {
		epochs = []*model.Epoch{}
	}

	writeRPCResult(w, req.ID, api.ListResponse[*model.Epoch]{
		Data: epochs,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	})
}

func handleGetEpoch(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetEpochParams
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

	index, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
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

	writeRPCResult(w, req.ID, api.SingleResponse[*model.Epoch]{Data: epoch})
}

func handleGetLastAcceptedEpochIndex(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetLastAcceptedEpochIndexParams
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

	writeRPCResult(w, req.ID, api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", index)})
}

func handleListInputs(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.ListInputsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
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
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	// Create input filter based on params
	inputFilter := repository.InputFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
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
	if len(inputs) == 0 && s.applicationAbsentOrError(w, r, req, params.Application) {
		return
	}

	resultInputs := make([]*api.DecodedInput, 0, len(inputs))
	for _, in := range inputs {
		decoded, err := api.DecodeInput(in, s.inputABI)
		if err != nil {
			s.Logger.Error("Unable to decode Input", "app", params.Application, "index", in.Index, "err", err)
		}
		resultInputs = append(resultInputs, decoded)
	}

	writeRPCResult(w, req.ID, api.ListResponse[*api.DecodedInput]{
		Data: resultInputs,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	})
}

func handleGetInput(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetInputParams
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

	index, err := config.ToIndexFromString(params.InputIndex)
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input index: %v", err), nil)
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

	decoded, err := api.DecodeInput(input, s.inputABI)
	if err != nil {
		s.Logger.Error("Unable to decode Input", "app", params.Application, "index", input.Index, "err", err)
	}

	writeRPCResult(w, req.ID, api.SingleResponse[*api.DecodedInput]{Data: decoded})
}

func handleGetProcessedInputCount(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetApplicationParams
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

	processedInputs, err := s.repository.GetProcessedInputCount(r.Context(), params.Application)
	if errors.Is(err, repository.ErrNotFound) {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Application not found", nil)
		return
	}
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}

	writeRPCResult(w, req.ID, api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", processedInputs)})
}

func handleListOutputs(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.ListOutputsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
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
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	// Create output filter based on params
	outputFilter := repository.OutputFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
			return
		}
		outputFilter.EpochIndex = &epochIndex
	}

	if params.InputIndex != nil {
		inputIndex, err := config.ToIndexFromString(*params.InputIndex)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input index: %v", err), nil)
			return
		}
		outputFilter.InputIndex = &inputIndex
	}

	// Add output type filter if provided
	if params.OutputType != nil {
		outputType, err := api.ParseOutputType(*params.OutputType)
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

	resultOutputs := make([]*api.DecodedOutput, 0, len(outputs))
	for _, out := range outputs {
		decoded, err := api.DecodeOutput(out, s.outputABI)
		if err != nil {
			s.Logger.Error("Unable to decode Output", "app", params.Application, "index", out.Index, "err", err)
		}
		resultOutputs = append(resultOutputs, decoded)
	}

	if len(resultOutputs) == 0 && s.applicationAbsentOrError(w, r, req, params.Application) {
		return
	}

	writeRPCResult(w, req.ID, api.ListResponse[*api.DecodedOutput]{
		Data: resultOutputs,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	})
}

func handleGetOutput(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetOutputParams
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

	index, err := config.ToIndexFromString(params.OutputIndex)
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid output index: %v", err), nil)
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

	decoded, err := api.DecodeOutput(output, s.outputABI)
	if err != nil {
		s.Logger.Error("Unable to decode Output", "app", params.Application, "index", output.Index, "err", err)
	}

	writeRPCResult(w, req.ID, api.SingleResponse[*api.DecodedOutput]{Data: decoded})
}

func handleListReports(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.ListReportsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
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
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	// Create report filter based on params
	reportFilter := repository.ReportFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
			return
		}
		reportFilter.EpochIndex = &epochIndex
	}

	if params.InputIndex != nil {
		inputIndex, err := config.ToIndexFromString(*params.InputIndex)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid input index: %v", err), nil)
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

	if len(reports) == 0 && s.applicationAbsentOrError(w, r, req, params.Application) {
		return
	}
	if reports == nil {
		reports = []*model.Report{}
	}

	writeRPCResult(w, req.ID, api.ListResponse[*model.Report]{
		Data: reports,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	})
}

func handleGetReport(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetReportParams
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

	index, err := config.ToIndexFromString(params.ReportIndex)
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid report index: %v", err), nil)
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

	writeRPCResult(w, req.ID, api.SingleResponse[*model.Report]{Data: report})
}

func handleListWithdrawals(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.ListWithdrawalsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	if params.Limit <= 0 {
		params.Limit = LIST_ITEM_DEFAULT
	}
	if params.Limit > LIST_ITEM_LIMIT {
		params.Limit = LIST_ITEM_LIMIT
	}

	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	withdrawalFilter := repository.WithdrawalFilter{}
	if params.AccountIndex != nil {
		accountIndex, err := config.ToIndexFromString(*params.AccountIndex)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid account index: %v", err), nil)
			return
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
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}

	if len(withdrawals) == 0 && s.applicationAbsentOrError(w, r, req, params.Application) {
		return
	}
	if withdrawals == nil {
		withdrawals = []*model.Withdrawal{}
	}

	writeRPCResult(w, req.ID, api.ListResponse[*model.Withdrawal]{
		Data: withdrawals,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	})
}

func handleGetWithdrawal(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetWithdrawalParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
	}

	if err := validateNameOrAddress(params.Application); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	accountIndex, err := config.ToIndexFromString(params.AccountIndex)
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid account index: %v", err), nil)
		return
	}

	withdrawal, err := s.repository.GetWithdrawal(r.Context(), params.Application, accountIndex)
	if err != nil {
		s.Logger.Error("Unable to retrieve withdrawal from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if withdrawal == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Withdrawal not found", nil)
		return
	}

	writeRPCResult(w, req.ID, api.SingleResponse[*model.Withdrawal]{Data: withdrawal})
}

func handleListTournaments(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.ListTournamentsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
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
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	// Create tournament filter based on params
	tournamentFilter := repository.TournamentFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
			return
		}
		tournamentFilter.EpochIndex = &epochIndex
	}

	if params.Level != nil {
		level, err := config.ToIndexFromString(*params.Level)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid level: %v", err), nil)
			return
		}
		tournamentFilter.Level = &level
	}

	if params.ParentTournamentAddress != nil {
		parentAddress, err := config.ToAddressFromString(*params.ParentTournamentAddress)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid parent tournament address: %v", err), nil)
		}
		tournamentFilter.ParentTournamentAddress = &parentAddress
	}

	if params.ParentMatchIDHash != nil {
		parentMatchIDHash, err := config.ToHashFromString(*params.ParentMatchIDHash)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid parent match ID hash: %v", err), nil)
		}
		tournamentFilter.ParentMatchIDHash = &parentMatchIDHash
	}

	tournaments, total, err := s.repository.ListTournaments(r.Context(), params.Application, tournamentFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve tournaments from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if len(tournaments) == 0 && s.applicationAbsentOrError(w, r, req, params.Application) {
		return
	}
	if tournaments == nil {
		tournaments = []*model.Tournament{}
	}

	writeRPCResult(w, req.ID, api.ListResponse[*model.Tournament]{
		Data: tournaments,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	})
}

func handleGetTournament(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetTournamentParams
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

	// Validate tournament address
	if _, err := config.ToAddressFromString(params.Address); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
		return
	}

	tournament, err := s.repository.GetTournament(r.Context(), params.Application, params.Address)
	if err != nil {
		s.Logger.Error("Unable to retrieve tournament from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if tournament == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Tournament not found", nil)
		return
	}

	writeRPCResult(w, req.ID, api.SingleResponse[*model.Tournament]{Data: tournament})
}

func handleListCommitments(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.ListCommitmentsParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
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
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	// Create commitment filter based on params
	commitmentFilter := repository.CommitmentFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
			return
		}
		commitmentFilter.EpochIndex = &epochIndex
	}

	if params.TournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.TournamentAddress); err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
			return
		}
		commitmentFilter.TournamentAddress = params.TournamentAddress
	}

	commitments, total, err := s.repository.ListCommitments(r.Context(), params.Application, commitmentFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve commitments from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if len(commitments) == 0 && s.applicationAbsentOrError(w, r, req, params.Application) {
		return
	}
	if commitments == nil {
		commitments = []*model.Commitment{}
	}

	writeRPCResult(w, req.ID, api.ListResponse[*model.Commitment]{
		Data: commitments,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	})
}

func handleGetCommitment(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetCommitmentParams
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

	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
		return
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
		return
	}

	if len(params.Commitment) == 0 {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid commitment hex: Empty string", nil)
		return
	}
	if _, err := config.ToHashFromString(params.Commitment); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid commitment hex: %v", err), nil)
		return
	}

	commitment, err := s.repository.GetCommitment(r.Context(), params.Application, epochIndex, params.TournamentAddress, params.Commitment)
	if err != nil {
		s.Logger.Error("Unable to retrieve commitment from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if commitment == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Commitment not found", nil)
		return
	}

	writeRPCResult(w, req.ID, api.SingleResponse[*model.Commitment]{Data: commitment})
}

func handleListMatches(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.ListMatchesParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
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
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	// Create match filter based on params
	matchFilter := repository.MatchFilter{}
	if params.EpochIndex != nil {
		epochIndex, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
			return
		}
		matchFilter.EpochIndex = &epochIndex
	}

	if params.TournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.TournamentAddress); err != nil {
			writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
			return
		}
		matchFilter.TournamentAddress = params.TournamentAddress
	}

	matches, total, err := s.repository.ListMatches(r.Context(), params.Application, matchFilter, repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve matches from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if len(matches) == 0 && s.applicationAbsentOrError(w, r, req, params.Application) {
		return
	}
	if matches == nil {
		matches = []*model.Match{}
	}

	writeRPCResult(w, req.ID, api.ListResponse[*model.Match]{
		Data: matches,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	})
}

func handleGetMatch(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetMatchParams
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

	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
		return
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
		return
	}

	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid ID hash: %v", err), nil)
		return
	}

	match, err := s.repository.GetMatch(r.Context(), params.Application, epochIndex, params.TournamentAddress, params.IDHash)
	if err != nil {
		s.Logger.Error("Unable to retrieve match from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if match == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Match not found", nil)
		return
	}

	writeRPCResult(w, req.ID, api.SingleResponse[*model.Match]{Data: match})
}

func handleListMatchAdvances(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.ListMatchAdvancesParams
	if err := UnmarshalParams(req.Params, &params); err != nil {
		s.Logger.Debug("Invalid parameters", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, "Invalid parameters", nil)
		return
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
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid application identifier: %v", err), nil)
		return
	}

	// Create match advance filter based on params
	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
		return
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
		return
	}

	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid ID hash: %v", err), nil)
		return
	}

	pagination := repository.Pagination{
		Limit:  params.Limit,
		Offset: params.Offset,
	}
	matchAdvances, total, err := s.repository.ListMatchAdvances(r.Context(), params.Application, epochIndex,
		params.TournamentAddress, params.IDHash, pagination, params.Descending)
	if err != nil {
		s.Logger.Error("Unable to retrieve match advances from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if len(matchAdvances) == 0 && s.applicationAbsentOrError(w, r, req, params.Application) {
		return
	}
	if matchAdvances == nil {
		matchAdvances = []*model.MatchAdvanced{}
	}

	writeRPCResult(w, req.ID, api.ListResponse[*model.MatchAdvanced]{
		Data: matchAdvances,
		Pagination: api.Pagination{
			TotalCount: total,
			Limit:      params.Limit,
			Offset:     params.Offset,
		},
	})
}

func handleGetMatchAdvanced(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
	var params api.GetMatchAdvancedParams
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

	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid epoch index: %v", err), nil)
		return
	}

	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid tournament address: %v", err), nil)
		return
	}

	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid ID hash: %v", err), nil)
		return
	}

	if _, err := config.ToHashFromString(params.Parent); err != nil {
		writeRPCError(w, req.ID, JSONRPC_INVALID_PARAMS, fmt.Sprintf("Invalid parent hash: %v", err), nil)
		return
	}

	matchAdvanced, err := s.repository.GetMatchAdvanced(r.Context(), params.Application, epochIndex,
		params.TournamentAddress, params.IDHash, params.Parent[2:]) // TODO: use parsed value
	if err != nil {
		s.Logger.Error("Unable to retrieve match advanced from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return
	}
	if matchAdvanced == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Match advanced not found", nil)
		return
	}

	writeRPCResult(w, req.ID, api.SingleResponse[*model.MatchAdvanced]{Data: matchAdvanced})
}

func handleGetChainID(s *Service, w http.ResponseWriter, r *http.Request, req RPCRequest) {
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

	writeRPCResult(w, req.ID, api.SingleResponse[string]{Data: fmt.Sprintf("0x%x", config.Value.ChainID)})
}

func handleGetNodeVersion(_ *Service, w http.ResponseWriter, _ *http.Request, req RPCRequest) {
	writeRPCResult(w, req.ID, api.SingleResponse[string]{Data: version.BuildVersion})
}

func (s *Service) applicationAbsentOrError(
	w http.ResponseWriter,
	r *http.Request,
	req RPCRequest,
	validatedNameOrAddress string,
) bool {
	app, err := s.repository.GetApplication(r.Context(), validatedNameOrAddress)
	if err != nil {
		s.Logger.Error("Unable to retrieve application from repository", "err", err)
		writeRPCError(w, req.ID, JSONRPC_INTERNAL_ERROR, "Internal server error", nil)
		return true
	} else if app == nil {
		writeRPCError(w, req.ID, JSONRPC_RESOURCE_NOT_FOUND, "Application not found", nil)
		return true
	}
	return false
}
