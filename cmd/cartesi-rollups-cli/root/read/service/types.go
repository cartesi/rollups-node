// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"encoding/json/jsontext"
	"errors"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrApplicationNotFound = errors.New("application not found")
)

type ReadService interface {
	GetApplication(ctx context.Context, params api.GetApplicationParams) (jsontext.Value, error)
	GetEpoch(ctx context.Context, params api.GetEpochParams) (jsontext.Value, error)
	ListEpochs(ctx context.Context, params api.ListEpochsParams) (jsontext.Value, error)
	GetInput(ctx context.Context, params api.GetInputParams) (jsontext.Value, error)
	ListInputs(ctx context.Context, params api.ListInputsParams) (jsontext.Value, error)
	GetOutput(ctx context.Context, params api.GetOutputParams) (jsontext.Value, error)
	ListOutputs(ctx context.Context, params api.ListOutputsParams) (jsontext.Value, error)
	GetReport(ctx context.Context, params api.GetReportParams) (jsontext.Value, error)
	ListReports(ctx context.Context, params api.ListReportsParams) (jsontext.Value, error)
	GetWithdrawal(ctx context.Context, params api.GetWithdrawalParams) (jsontext.Value, error)
	ListWithdrawals(ctx context.Context, params api.ListWithdrawalsParams) (jsontext.Value, error)
	GetTournament(ctx context.Context, params api.GetTournamentParams) (jsontext.Value, error)
	ListTournaments(ctx context.Context, params api.ListTournamentsParams) (jsontext.Value, error)
	GetCommitment(ctx context.Context, params api.GetCommitmentParams) (jsontext.Value, error)
	ListCommitments(ctx context.Context, params api.ListCommitmentsParams) (jsontext.Value, error)
	GetMatch(ctx context.Context, params api.GetMatchParams) (jsontext.Value, error)
	ListMatches(ctx context.Context, params api.ListMatchesParams) (jsontext.Value, error)
	GetMatchAdvanced(ctx context.Context, params api.GetMatchAdvanceParams) (jsontext.Value, error)
	ListMatchAdvances(ctx context.Context, params api.ListMatchAdvancesParams) (jsontext.Value, error)
	GetStandingChange(ctx context.Context, params api.GetStandingChangeParams) (jsontext.Value, error)
	ListStandingChanges(ctx context.Context, params api.ListStandingChangesParams) (jsontext.Value, error)
	Close()
}

func CreateReadService(ctx context.Context, useJsonrpc bool) (ReadService, error) {
	if useJsonrpc {
		url, err := config.GetJsonrpcApiUrl()
		if err != nil {
			return nil, err
		}
		return NewJsonrpcReadService(ctx, url.Raw()) //nolint:forbidigo // Connect through the protected JSON-RPC transport.
	}
	dsn, err := config.GetDatabaseConnection()
	if err != nil {
		return nil, err
	}
	return NewRepositoryReadService(ctx, dsn)
}
