// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/jsonrpc/client"
)

type JsonrpcReadService struct {
	Client *client.Client
}

func (s *JsonrpcReadService) GetApplication(ctx context.Context, params jsonrpc.GetApplicationParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_getApplication", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) GetEpoch(ctx context.Context, params jsonrpc.GetEpochParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	if _, err := config.ToIndexFromString(params.EpochIndex); err != nil {
		return nil, fmt.Errorf("invalid epoch index: %w", err)
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_getEpoch", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) ListEpochs(ctx context.Context, params jsonrpc.ListEpochsParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	// Add status filter if provided
	if params.Status != nil {
		var statusVal model.EpochStatus
		if err := statusVal.Scan(*params.Status); err != nil {
			return nil, fmt.Errorf("invalid status: %w", err)
		}
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_listEpochs", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) GetInput(ctx context.Context, params jsonrpc.GetInputParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	if _, err := config.ToIndexFromString(params.InputIndex); err != nil {
		return nil, fmt.Errorf("invalid input index: %w", err)
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_getInput", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) ListInputs(ctx context.Context, params jsonrpc.ListInputsParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		if _, err := config.ToIndexFromString(*params.EpochIndex); err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
	}
	// Add sender filter if provided
	if params.Sender != nil {
		if _, err := config.ToAddressFromString(*params.Sender); err != nil {
			return nil, fmt.Errorf("invalid sender: %w", err)
		}
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_listInputs", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) GetOutput(ctx context.Context, params jsonrpc.GetOutputParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	if _, err := config.ToIndexFromString(params.OutputIndex); err != nil {
		return nil, fmt.Errorf("invalid output index: %w", err)
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_getOutput", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) ListOutputs(ctx context.Context, params jsonrpc.ListOutputsParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		if _, err := config.ToIndexFromString(*params.EpochIndex); err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
	}
	// Add input index filter if provided
	if params.InputIndex != nil {
		if _, err := config.ToIndexFromString(*params.InputIndex); err != nil {
			return nil, fmt.Errorf("invalid input index: %w", err)
		}
	}
	// Add output type filter if provided
	if params.OutputType != nil {
		if _, err := jsonrpc.ParseOutputType(*params.OutputType); err != nil {
			return nil, fmt.Errorf("invalid output type: %w", err)
		}
	}
	// Add voucher address filter if provided
	if params.VoucherAddress != nil {
		if _, err := config.ToAddressFromString(*params.VoucherAddress); err != nil {
			return nil, fmt.Errorf("invalid voucher address: %w", err)
		}
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_listOutputs", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) GetReport(ctx context.Context, params jsonrpc.GetReportParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	if _, err := config.ToIndexFromString(params.ReportIndex); err != nil {
		return nil, fmt.Errorf("invalid report index: %w", err)
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_getReport", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) ListReports(ctx context.Context, params jsonrpc.ListReportsParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		if _, err := config.ToIndexFromString(*params.EpochIndex); err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
	}
	// Add input index filter if provided
	if params.InputIndex != nil {
		if _, err := config.ToIndexFromString(*params.InputIndex); err != nil {
			return nil, fmt.Errorf("invalid input index: %w", err)
		}
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_listReports", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) GetTournament(ctx context.Context, params jsonrpc.GetTournamentParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	if _, err := config.ToAddressFromString(params.Address); err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_getTournament", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) ListTournaments(ctx context.Context, params jsonrpc.ListTournamentsParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		if _, err := config.ToIndexFromString(*params.EpochIndex); err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
	}
	// Add level filter if provided
	if params.Level != nil {
		if _, err := config.ToIndexFromString(*params.Level); err != nil {
			return nil, fmt.Errorf("invalid level: %w", err)
		}
	}
	// Add parent tournament address filter if provided
	if params.ParentTournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.ParentTournamentAddress); err != nil {
			return nil, fmt.Errorf("invalid parent tournament address: %w", err)
		}
	}
	// Add parent match ID hash filter if provided
	if params.ParentMatchIDHash != nil {
		if _, err := config.ToHashFromString(*params.ParentMatchIDHash); err != nil {
			return nil, fmt.Errorf("invalid parent match ID hash: %w", err)
		}
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_listTournaments", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) GetCommitment(ctx context.Context, params jsonrpc.GetCommitmentParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	if _, err := config.ToIndexFromString(params.EpochIndex); err != nil {
		return nil, fmt.Errorf("invalid epoch index: %w", err)
	}
	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, fmt.Errorf("invalid tournament address: %w", err)
	}
	if _, err := config.ToHashFromString(params.Commitment); err != nil {
		return nil, fmt.Errorf("invalid commitment: %w", err)
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_getCommitment", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) ListCommitments(ctx context.Context, params jsonrpc.ListCommitmentsParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		if _, err := config.ToIndexFromString(*params.EpochIndex); err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
	}
	// Add tournament address filter if provided
	if params.TournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.TournamentAddress); err != nil {
			return nil, fmt.Errorf("invalid tournament address: %w", err)
		}
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_listCommitments", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) GetMatch(ctx context.Context, params jsonrpc.GetMatchParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	if _, err := config.ToIndexFromString(params.EpochIndex); err != nil {
		return nil, fmt.Errorf("invalid epoch index: %w", err)
	}
	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, fmt.Errorf("invalid tournament address: %w", err)
	}
	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, fmt.Errorf("invalid ID hash: %w", err)
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_getMatch", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) ListMatches(ctx context.Context, params jsonrpc.ListMatchesParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	// Add epoch index filter if provided
	if params.EpochIndex != nil {
		if _, err := config.ToIndexFromString(*params.EpochIndex); err != nil {
			return nil, fmt.Errorf("invalid epoch index: %w", err)
		}
	}
	// Add tournament address filter if provided
	if params.TournamentAddress != nil {
		if _, err := config.ToAddressFromString(*params.TournamentAddress); err != nil {
			return nil, fmt.Errorf("invalid tournament address: %w", err)
		}
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_listMatches", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) GetMatchAdvanced(ctx context.Context, params jsonrpc.GetMatchAdvancedParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	if _, err := config.ToIndexFromString(params.EpochIndex); err != nil {
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

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_getMatchAdvanced", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) ListMatchAdvances(ctx context.Context, params jsonrpc.ListMatchAdvancesParams) (json.RawMessage, error) {
	if _, err := config.ToApplicationNameOrAddressFromString(params.Application); err != nil {
		return nil, fmt.Errorf("invalid application: %w", err)
	}
	if _, err := config.ToIndexFromString(params.EpochIndex); err != nil {
		return nil, fmt.Errorf("invalid epoch index: %w", err)
	}
	if _, err := config.ToAddressFromString(params.TournamentAddress); err != nil {
		return nil, fmt.Errorf("invalid tournament address: %w", err)
	}
	if _, err := config.ToHashFromString(params.IDHash); err != nil {
		return nil, fmt.Errorf("invalid ID hash: %w", err)
	}

	var resp json.RawMessage
	err := s.Client.Call(ctx, "cartesi_listMatchAdvances", params, &resp)
	return resp, err
}

func (s *JsonrpcReadService) Close() {
}

func NewJsonrpcReadService(_ context.Context, serviceURL string) (ReadService, error) {
	if _, err := url.ParseRequestURI(serviceURL); err != nil {
		return nil, err
	}
	service := &JsonrpcReadService{
		Client: client.NewClient(serviceURL),
	}
	return service, nil
}
