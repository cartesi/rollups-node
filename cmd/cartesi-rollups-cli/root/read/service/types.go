// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrApplicationNotFound = errors.New("application not found")
)

type ReadService interface {
	GetApplication(ctx context.Context, params api.GetApplicationParams) (json.RawMessage, error)
	GetEpoch(ctx context.Context, params api.GetEpochParams) (json.RawMessage, error)
	ListEpochs(ctx context.Context, params api.ListEpochsParams) (json.RawMessage, error)
	GetInput(ctx context.Context, params api.GetInputParams) (json.RawMessage, error)
	ListInputs(ctx context.Context, params api.ListInputsParams) (json.RawMessage, error)
	GetOutput(ctx context.Context, params api.GetOutputParams) (json.RawMessage, error)
	ListOutputs(ctx context.Context, params api.ListOutputsParams) (json.RawMessage, error)
	GetReport(ctx context.Context, params api.GetReportParams) (json.RawMessage, error)
	ListReports(ctx context.Context, params api.ListReportsParams) (json.RawMessage, error)
	GetTournament(ctx context.Context, params api.GetTournamentParams) (json.RawMessage, error)
	ListTournaments(ctx context.Context, params api.ListTournamentsParams) (json.RawMessage, error)
	GetCommitment(ctx context.Context, params api.GetCommitmentParams) (json.RawMessage, error)
	ListCommitments(ctx context.Context, params api.ListCommitmentsParams) (json.RawMessage, error)
	GetMatch(ctx context.Context, params api.GetMatchParams) (json.RawMessage, error)
	ListMatches(ctx context.Context, params api.ListMatchesParams) (json.RawMessage, error)
	GetMatchAdvanced(ctx context.Context, params api.GetMatchAdvancedParams) (json.RawMessage, error)
	ListMatchAdvances(ctx context.Context, params api.ListMatchAdvancesParams) (json.RawMessage, error)
	Close()
}

func CreateReadService(ctx context.Context, useJsonrpc bool) (ReadService, error) {
	if useJsonrpc {
		url, err := config.GetJsonrpcApiUrl()
		if err != nil {
			return nil, err
		}
		return NewJsonrpcReadService(ctx, url)
	} else {
		dsn, err := config.GetDatabaseConnection()
		if err != nil {
			return nil, err
		}
		return NewRepositoryReadService(ctx, dsn.String())
	}
}
