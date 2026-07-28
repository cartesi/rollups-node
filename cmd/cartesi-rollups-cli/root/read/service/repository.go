// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

type RepositoryReadService struct {
	Repository repository.Repository
	InputAbi   *abi.ABI
	OutputAbi  *abi.ABI
}

func (s *RepositoryReadService) GetApplication(ctx context.Context, params api.GetApplicationParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}

	data, err := repo.GetApplication(ctx, application)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNotFound
	}

	response := map[string]any{
		"data": data,
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) GetEpoch(ctx context.Context, params api.GetEpochParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, fmt.Errorf("invalid epoch index: %w", err)
	}

	data, err := repo.GetEpoch(ctx, application, epochIndex)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNotFound
	}

	response := map[string]any{
		"data": data,
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) ListEpochs(ctx context.Context, params api.ListEpochsParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	filter := repository.EpochFilter{}
	pagination := repository.Pagination{}
	// Add status filter if provided
	if params.Status != nil {
		var statusVal model.EpochStatus
		if err := statusVal.Scan(*params.Status); err != nil {
			return nil, fmt.Errorf("invalid status: %w", err)
		}
		filter.Status = []model.EpochStatus{statusVal}
	}
	pagination.Limit = params.Limit
	pagination.Offset = params.Offset

	data, total, err := repo.ListEpochs(ctx, application, filter, pagination, params.Descending)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		app, err := repo.GetApplication(ctx, application)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, ErrApplicationNotFound
		}
		data = make([]*model.Epoch, 0)
	}

	response := map[string]any{
		"data": data,
		"pagination": map[string]uint64{
			"total_count": total,
			"limit":       pagination.Limit,
			"offset":      pagination.Offset,
		},
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) GetInput(ctx context.Context, params api.GetInputParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	inputIndex, err := config.ToIndexFromString(params.InputIndex)
	if err != nil {
		return nil, fmt.Errorf("invalid input index: %w", err)
	}

	data, err := repo.GetInput(ctx, application, inputIndex)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNotFound
	}
	dataVal, err := api.DecodeInput(data, s.InputAbi)
	if err != nil {
		dataVal = &api.DecodedInput{Input: data}
	}

	response := map[string]any{
		"data": dataVal,
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) ListInputs(ctx context.Context, params api.ListInputsParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	filter := repository.InputFilter{}
	pagination := repository.Pagination{}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		epochIndexVal, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
		filter.EpochIndex = &epochIndexVal
	}
	// Add sender filter if provided
	if params.Sender != nil {
		senderVal, err := config.ToAddressFromString(*params.Sender)
		if err != nil {
			return nil, fmt.Errorf("invalid sender: %w", err)
		}
		filter.Sender = &senderVal
	}
	if params.TransactionHash != nil {
		transactionHashVal, err := config.ToHashFromString(*params.TransactionHash)
		if err != nil {
			return nil, fmt.Errorf("invalid transaction hash: %w", err)
		}
		filter.TransactionHash = &transactionHashVal
	}
	pagination.Limit = params.Limit
	pagination.Offset = params.Offset

	data, total, err := repo.ListInputs(ctx, application, filter, pagination, params.Descending)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		app, err := repo.GetApplication(ctx, application)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, ErrApplicationNotFound
		}
		data = make([]*model.Input, 0)
	}
	dataVal := make([]*api.DecodedInput, 0, len(data))
	for _, item := range data {
		decoded, err := api.DecodeInput(item, s.InputAbi)
		if err != nil {
			decoded = &api.DecodedInput{Input: item}
		}
		dataVal = append(dataVal, decoded)
	}

	response := map[string]any{
		"data": dataVal,
		"pagination": map[string]uint64{
			"total_count": total,
			"limit":       pagination.Limit,
			"offset":      pagination.Offset,
		},
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) GetOutput(ctx context.Context, params api.GetOutputParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	outputIndex, err := config.ToIndexFromString(params.OutputIndex)
	if err != nil {
		return nil, fmt.Errorf("invalid output index: %w", err)
	}

	data, err := repo.GetOutput(ctx, application, outputIndex)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNotFound
	}
	dataVal, err := api.DecodeOutput(data, s.OutputAbi)
	if err != nil {
		dataVal = &api.DecodedOutput{
			Output:      data,
			DecodedData: &api.DecodedData{Type: "error", RawData: err.Error()},
		}
	}

	response := map[string]any{
		"data": dataVal,
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) ListOutputs(ctx context.Context, params api.ListOutputsParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	filter := repository.OutputFilter{}
	pagination := repository.Pagination{}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		epochIndexVal, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
		filter.EpochIndex = &epochIndexVal
	}
	// Add input index filter if provided
	if params.InputIndex != nil {
		inputIndexVal, err := config.ToIndexFromString(*params.InputIndex)
		if err != nil {
			return nil, fmt.Errorf("invalid input index: %w", err)
		}
		filter.InputIndex = &inputIndexVal
	}
	// Add output type filter if provided
	if params.OutputType != nil {
		outputTypeVal := make([][]byte, len(*params.OutputType))
		for i, selector := range *params.OutputType {
			parsed, err := api.ParseOutputType(selector)
			if err != nil {
				return nil, fmt.Errorf("invalid output type #%d: %w", i+1, err)
			}
			outputTypeVal[i] = parsed
		}
		filter.OutputType = &outputTypeVal
	}
	// Add voucher address filter if provided
	if params.VoucherAddress != nil {
		voucherAddressVal, err := config.ToAddressFromString(*params.VoucherAddress)
		if err != nil {
			return nil, fmt.Errorf("invalid voucher address: %w", err)
		}
		filter.VoucherAddress = &voucherAddressVal
	}
	pagination.Limit = params.Limit
	pagination.Offset = params.Offset

	data, total, err := repo.ListOutputs(ctx, application, filter, pagination, params.Descending)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		app, err := repo.GetApplication(ctx, application)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, ErrApplicationNotFound
		}
		data = make([]*model.Output, 0)
	}
	dataVal := make([]*api.DecodedOutput, 0, len(data))
	for _, item := range data {
		decoded, err := api.DecodeOutput(item, s.OutputAbi)
		if err != nil {
			decoded = &api.DecodedOutput{
				Output:      item,
				DecodedData: &api.DecodedData{Type: "error", RawData: err.Error()},
			}
		}
		dataVal = append(dataVal, decoded)
	}

	response := map[string]any{
		"data": dataVal,
		"pagination": map[string]uint64{
			"total_count": total,
			"limit":       pagination.Limit,
			"offset":      pagination.Offset,
		},
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) GetReport(ctx context.Context, params api.GetReportParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	reportIndex, err := config.ToIndexFromString(params.ReportIndex)
	if err != nil {
		return nil, fmt.Errorf("invalid report index: %w", err)
	}

	data, err := repo.GetReport(ctx, application, reportIndex)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNotFound
	}

	response := map[string]any{
		"data": data,
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) ListReports(ctx context.Context, params api.ListReportsParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	filter := repository.ReportFilter{}
	pagination := repository.Pagination{}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		epochIndexVal, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
		filter.EpochIndex = &epochIndexVal
	}
	// Add input index filter if provided
	if params.InputIndex != nil {
		inputIndexVal, err := config.ToIndexFromString(*params.InputIndex)
		if err != nil {
			return nil, fmt.Errorf("invalid input index: %w", err)
		}
		filter.InputIndex = &inputIndexVal
	}
	pagination.Limit = params.Limit
	pagination.Offset = params.Offset

	data, total, err := repo.ListReports(ctx, application, filter, pagination, params.Descending)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		app, err := repo.GetApplication(ctx, application)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, ErrApplicationNotFound
		}
		data = make([]*model.Report, 0)
	}

	response := map[string]any{
		"data": data,
		"pagination": map[string]uint64{
			"total_count": total,
			"limit":       pagination.Limit,
			"offset":      pagination.Offset,
		},
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) GetWithdrawal(ctx context.Context, params api.GetWithdrawalParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	accountIndex, err := config.ToIndexFromString(params.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("invalid account index: %w", err)
	}

	data, err := repo.GetWithdrawal(ctx, application, accountIndex)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNotFound
	}

	response := map[string]any{
		"data": data,
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) ListWithdrawals(ctx context.Context, params api.ListWithdrawalsParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	filter := repository.WithdrawalFilter{}
	pagination := repository.Pagination{}
	if params.AccountIndex != nil {
		accountIndexVal, err := config.ToIndexFromString(*params.AccountIndex)
		if err != nil {
			return nil, fmt.Errorf("invalid account index: %w", err)
		}
		filter.AccountIndex = &accountIndexVal
	}
	pagination.Limit = params.Limit
	pagination.Offset = params.Offset

	data, total, err := repo.ListWithdrawals(ctx, application, filter, pagination, params.Descending)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		app, err := repo.GetApplication(ctx, application)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, ErrApplicationNotFound
		}
		data = make([]*model.Withdrawal, 0)
	}

	response := map[string]any{
		"data": data,
		"pagination": map[string]uint64{
			"total_count": total,
			"limit":       pagination.Limit,
			"offset":      pagination.Offset,
		},
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) GetTournament(ctx context.Context, params api.GetTournamentParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	if _, err := config.ToAddressFromString(params.Address); err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	data, err := repo.GetTournament(ctx, application, params.Address)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNotFound
	}

	response := map[string]any{
		"data": data,
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) ListTournaments(ctx context.Context, params api.ListTournamentsParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	filter := repository.TournamentFilter{}
	pagination := repository.Pagination{}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		epochIndexVal, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
		filter.EpochIndex = &epochIndexVal
	}
	// Add level filter if provided
	if params.Level != nil {
		levelVal, err := config.ToIndexFromString(*params.Level)
		if err != nil {
			return nil, fmt.Errorf("invalid level: %w", err)
		}
		filter.Level = &levelVal
	}
	// Add parent tournament address filter if provided
	if params.ParentTournamentAddress != nil {
		parentTournamentAddressVal, err := config.ToAddressFromString(*params.ParentTournamentAddress)
		if err != nil {
			return nil, fmt.Errorf("invalid parent tournament address: %w", err)
		}
		filter.ParentTournamentAddress = &parentTournamentAddressVal
	}
	// Add parent match ID hash filter if provided
	if params.ParentMatchIDHash != nil {
		parentMatchIDHashVal, err := config.ToHashFromString(*params.ParentMatchIDHash)
		if err != nil {
			return nil, fmt.Errorf("invalid parent match ID hash: %w", err)
		}
		filter.ParentMatchIDHash = &parentMatchIDHashVal
	}
	pagination.Limit = params.Limit
	pagination.Offset = params.Offset

	data, total, err := repo.ListTournaments(ctx, application, filter, pagination, params.Descending)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		app, err := repo.GetApplication(ctx, application)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, ErrApplicationNotFound
		}
		data = make([]*model.Tournament, 0)
	}

	response := map[string]any{
		"data": data,
		"pagination": map[string]uint64{
			"total_count": total,
			"limit":       pagination.Limit,
			"offset":      pagination.Offset,
		},
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) GetCommitment(ctx context.Context, params api.GetCommitmentParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, fmt.Errorf("invalid epoch index: %w", err)
	}
	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, fmt.Errorf("invalid tournament address: %w", err)
	}
	if _, err := config.ToHashFromString(params.Commitment); err != nil {
		return nil, fmt.Errorf("invalid commitment: %w", err)
	}

	data, err := repo.GetCommitment(ctx, application, epochIndex, params.TournamentAddress, params.Commitment)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNotFound
	}

	response := map[string]any{
		"data": data,
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) ListCommitments(ctx context.Context, params api.ListCommitmentsParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	filter := repository.CommitmentFilter{}
	pagination := repository.Pagination{}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		epochIndexVal, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
		filter.EpochIndex = &epochIndexVal
	}
	// Add tournament address filter if provided
	if params.TournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.TournamentAddress); err != nil {
			return nil, fmt.Errorf("invalid tournament address: %w", err)
		}
		filter.TournamentAddress = params.TournamentAddress
	}
	pagination.Limit = params.Limit
	pagination.Offset = params.Offset

	data, total, err := repo.ListCommitments(ctx, application, filter, pagination, params.Descending)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		app, err := repo.GetApplication(ctx, application)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, ErrApplicationNotFound
		}
		data = make([]*model.Commitment, 0)
	}

	response := map[string]any{
		"data": data,
		"pagination": map[string]uint64{
			"total_count": total,
			"limit":       pagination.Limit,
			"offset":      pagination.Offset,
		},
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) GetMatch(ctx context.Context, params api.GetMatchParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, fmt.Errorf("invalid epoch index: %w", err)
	}
	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, fmt.Errorf("invalid tournament address: %w", err)
	}
	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, fmt.Errorf("invalid ID hash: %w", err)
	}

	data, err := repo.GetMatch(ctx, application, epochIndex, params.TournamentAddress, params.IDHash)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNotFound
	}

	response := map[string]any{
		"data": data,
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) ListMatches(ctx context.Context, params api.ListMatchesParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	filter := repository.MatchFilter{}
	pagination := repository.Pagination{}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		epochIndexVal, err := config.ToIndexFromString(*params.EpochIndex)
		if err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
		filter.EpochIndex = &epochIndexVal
	}
	// Add tournament address filter if provided
	if params.TournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.TournamentAddress); err != nil {
			return nil, fmt.Errorf("invalid tournament address: %w", err)
		}
		filter.TournamentAddress = params.TournamentAddress
	}
	pagination.Limit = params.Limit
	pagination.Offset = params.Offset

	data, total, err := repo.ListMatches(ctx, application, filter, pagination, params.Descending)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		app, err := repo.GetApplication(ctx, application)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, ErrApplicationNotFound
		}
		data = make([]*model.Match, 0)
	}

	response := map[string]any{
		"data": data,
		"pagination": map[string]uint64{
			"total_count": total,
			"limit":       pagination.Limit,
			"offset":      pagination.Offset,
		},
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) GetMatchAdvanced(ctx context.Context, params api.GetMatchAdvancedParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, fmt.Errorf("invalid epoch index: %w", err)
	}
	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, fmt.Errorf("invalid tournament address: %w", err)
	}
	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, fmt.Errorf("invalid ID hash: %w", err)
	}
	if _, err := config.ToHashFromString(params.Parent); err != nil {
		return nil, fmt.Errorf("invalid parent: %w", err)
	}

	data, err := repo.GetMatchAdvanced(ctx, application, epochIndex, params.TournamentAddress, params.IDHash, params.Parent)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, ErrNotFound
	}

	response := map[string]any{
		"data": data,
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) ListMatchAdvances(ctx context.Context, params api.ListMatchAdvancesParams) (json.RawMessage, error) {
	repo := s.Repository
	application, err := config.ToApplicationNameOrAddressFromString(params.Application)
	if err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	epochIndex, err := config.ToIndexFromString(params.EpochIndex)
	if err != nil {
		return nil, fmt.Errorf("invalid epoch index: %w", err)
	}
	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, fmt.Errorf("invalid tournament address: %w", err)
	}
	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, fmt.Errorf("invalid ID hash: %w", err)
	}
	pagination := repository.Pagination{}
	pagination.Limit = params.Limit
	pagination.Offset = params.Offset

	data, total, err := repo.ListMatchAdvances(ctx, application, epochIndex, params.TournamentAddress, params.IDHash, pagination, params.Descending)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		app, err := repo.GetApplication(ctx, application)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, ErrApplicationNotFound
		}
		data = make([]*model.MatchAdvanced, 0)
	}

	response := map[string]any{
		"data": data,
		"pagination": map[string]uint64{
			"total_count": total,
			"limit":       pagination.Limit,
			"offset":      pagination.Offset,
		},
	}

	result, err := json.Marshal(response)

	return json.RawMessage(result), err
}

func (s *RepositoryReadService) Close() {
	s.Repository.Close()
}

func NewRepositoryReadService(ctx context.Context, dns string) (ReadService, error) {
	inputAbi, err := inputs.InputsMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	outputAbi, err := outputs.OutputsMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	repo, err := factory.NewRepositoryFromConnectionString(ctx, dns)
	if err != nil {
		return nil, err
	}

	service := &RepositoryReadService{
		Repository: repo,
		InputAbi:   inputAbi,
		OutputAbi:  outputAbi,
	}
	return service, nil
}
