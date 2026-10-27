// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/jsonrpc/client"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const (
	invalidStandingChangeLogIndex  = "invalid log index"
	invalidStandingTransactionHash = "invalid transaction hash"
)

func validStandingChangeParams() api.GetStandingChangeParams {
	return api.GetStandingChangeParams{Application: "echo-dapp", EpochIndex: "0x10",
		TournamentAddress: common.HexToAddress("0xabc").Hex(),
		TxHash:            common.HexToHash("0x123").Hex(), LogIndex: "0x0"}
}

func TestStandingChangeBackendsRejectInvalidIdentityBeforeIO(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*api.GetStandingChangeParams)
		message string
	}{
		{"application", func(p *api.GetStandingChangeParams) { p.Application = "" }, "invalid application"},
		{"epoch", func(p *api.GetStandingChangeParams) { p.EpochIndex = "" }, "invalid epoch index"},
		{"tournament", func(p *api.GetStandingChangeParams) { p.TournamentAddress = "" }, "invalid tournament address"},
		{"transaction absent", func(p *api.GetStandingChangeParams) { p.TxHash = "" }, invalidStandingTransactionHash},
		{"transaction short", func(p *api.GetStandingChangeParams) { p.TxHash = "0x123" }, invalidStandingTransactionHash},
		{"log index absent", func(p *api.GetStandingChangeParams) { p.LogIndex = "" }, invalidStandingChangeLogIndex},
		{"log index decimal", func(p *api.GetStandingChangeParams) { p.LogIndex = "12" }, invalidStandingChangeLogIndex},
		{"log index negative", func(p *api.GetStandingChangeParams) { p.LogIndex = "-1" }, invalidStandingChangeLogIndex},
		{"log index overflow", func(p *api.GetStandingChangeParams) { p.LogIndex = "0x10000000000000000" }, invalidStandingChangeLogIndex},
	} {
		for _, backend := range []struct {
			name    string
			service ReadService
		}{
			{"repository", &RepositoryReadService{}}, {"jsonrpc", &JsonrpcReadService{}},
		} {
			t.Run(backend.name+"/"+test.name, func(t *testing.T) {
				params := validStandingChangeParams()
				test.change(&params)
				// Both dependencies are nil: reaching I/O before validation fails this test.
				_, err := backend.service.GetStandingChange(t.Context(), params)
				require.ErrorContains(t, err, test.message)
			})
		}
	}
}

type standingChangeRepository struct {
	repository.Repository
	application func(context.Context, string) (*model.Application, error)
	get         func(context.Context, string, uint64, string, common.Hash, uint64) (*model.StandingChanged, error)
	list        func(context.Context, string, uint64, string, repository.Pagination, bool) ([]*model.StandingChanged, uint64, error)
}

func (r *standingChangeRepository) GetApplication(ctx context.Context, name string) (*model.Application, error) {
	return r.application(ctx, name)
}

func (r *standingChangeRepository) GetStandingChange(ctx context.Context, app string, epoch uint64, tournament string,
	tx common.Hash, logIndex uint64) (*model.StandingChanged, error) {
	return r.get(ctx, app, epoch, tournament, tx, logIndex)
}

func (r *standingChangeRepository) ListStandingChanges(ctx context.Context, app string, epoch uint64, tournament string,
	pagination repository.Pagination, descending bool) ([]*model.StandingChanged, uint64, error) {
	return r.list(ctx, app, epoch, tournament, pagination, descending)
}

func TestRepositoryStandingChangeUsesScopedEventIdentity(t *testing.T) {
	params := validStandingChangeParams()
	called := false
	repo := &standingChangeRepository{get: func(ctx context.Context, app string, epoch uint64, tournament string,
		tx common.Hash, logIndex uint64) (*model.StandingChanged, error) {
		called = true
		require.Equal(t, t.Context(), ctx)
		require.Equal(t, params.Application, app)
		require.Equal(t, uint64(16), epoch)
		require.Equal(t, params.TournamentAddress, tournament)
		require.Equal(t, common.HexToHash(params.TxHash), tx)
		require.Zero(t, logIndex, "block log index zero is valid")
		return &model.StandingChanged{TxHash: tx, LogIndex: logIndex}, nil
	}}
	s := &RepositoryReadService{Repository: repo}
	result, err := s.GetStandingChange(t.Context(), params)
	require.NoError(t, err)
	require.True(t, called)
	var response api.SingleResponse[*model.StandingChanged]
	require.NoError(t, json.Unmarshal(result, &response))
	require.Equal(t, common.HexToHash(params.TxHash), response.Data.TxHash)
	require.Zero(t, response.Data.LogIndex)
}

func TestRepositoryStandingChangeListKeepsTournamentScopeAndPagination(t *testing.T) {
	identity := validStandingChangeParams()
	params := api.ListStandingChangesParams{Application: identity.Application, EpochIndex: identity.EpochIndex,
		TournamentAddress: identity.TournamentAddress, Limit: 3, Offset: 2, Descending: true}
	called := false
	repo := &standingChangeRepository{list: func(_ context.Context, app string, epoch uint64, tournament string,
		pagination repository.Pagination, descending bool) ([]*model.StandingChanged, uint64, error) {
		called = true
		require.Equal(t, params.Application, app)
		require.Equal(t, uint64(16), epoch)
		require.Equal(t, params.TournamentAddress, tournament)
		require.Equal(t, repository.Pagination{Limit: 3, Offset: 2}, pagination)
		require.True(t, descending)
		return []*model.StandingChanged{{LogIndex: 4}}, 7, nil
	}}
	result, err := (&RepositoryReadService{Repository: repo}).ListStandingChanges(t.Context(), params)
	require.NoError(t, err)
	require.True(t, called)
	var response api.ListResponse[*model.StandingChanged]
	require.NoError(t, json.Unmarshal(result, &response))
	require.Equal(t, uint64(7), response.Pagination.TotalCount)
	require.Equal(t, uint64(3), response.Pagination.Limit)
	require.Equal(t, uint64(2), response.Pagination.Offset)
}

type standingChangeTransport func(*http.Request) (*http.Response, error)

func (transport standingChangeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestJSONRPCStandingChangeSendsScopedParams(t *testing.T) {
	identity := validStandingChangeParams()
	for _, get := range []bool{false, true} {
		name := "list"
		method := "cartesi_listStandingChanges"
		if get {
			name, method = "get", "cartesi_getStandingChange"
		}
		t.Run(name, func(t *testing.T) {
			called := false
			transport := standingChangeTransport(func(request *http.Request) (*http.Response, error) {
				called = true
				var wire struct {
					Method string
					Params map[string]any
				}
				require.NoError(t, json.NewDecoder(request.Body).Decode(&wire))
				require.NoError(t, request.Body.Close())
				require.Equal(t, method, wire.Method)
				require.Equal(t, identity.Application, wire.Params["application"])
				require.Equal(t, identity.EpochIndex, wire.Params["epoch_index"])
				require.Equal(t, identity.TournamentAddress, wire.Params["tournament_address"])
				require.NotContains(t, wire.Params, "id_hash")
				require.NotContains(t, wire.Params, "parent")
				if get {
					require.Equal(t, identity.TxHash, wire.Params["tx_hash"])
					require.Equal(t, identity.LogIndex, wire.Params["log_index"])
				} else {
					require.NotContains(t, wire.Params, "tx_hash")
					require.NotContains(t, wire.Params, "log_index")
					require.Equal(t, float64(3), wire.Params["limit"])
					require.Equal(t, float64(2), wire.Params["offset"])
					require.Equal(t, true, wire.Params["descending"])
				}
				return &http.Response{StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{"data":[]}}`))}, nil
			})
			s := &JsonrpcReadService{Client: &client.Client{URL: "http://example.invalid/rpc",
				HTTPClient: &http.Client{Transport: transport}}}
			var err error
			if get {
				_, err = s.GetStandingChange(t.Context(), identity)
			} else {
				_, err = s.ListStandingChanges(t.Context(), api.ListStandingChangesParams{Application: identity.Application,
					EpochIndex: identity.EpochIndex, TournamentAddress: identity.TournamentAddress,
					Limit: 3, Offset: 2, Descending: true})
			}
			require.NoError(t, err)
			require.True(t, called)
		})
	}
}

func TestStandingChangeRepositoryErrorsAndEmptyHistory(t *testing.T) {
	cause := errors.New("standing history unavailable")
	for _, scenario := range []struct {
		name   string
		getErr error
		app    *model.Application
		appErr error
		want   error
	}{
		{name: "dependency error", getErr: cause, want: cause},
		{name: "missing application", want: ErrApplicationNotFound},
		{name: "missing event", app: &model.Application{ID: 1}, want: ErrNotFound},
		{name: "lookup error", appErr: cause, want: cause},
		{name: "cancellation", getErr: context.Canceled, want: context.Canceled},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			repo := &standingChangeRepository{
				get: func(context.Context, string, uint64, string, common.Hash, uint64) (*model.StandingChanged, error) {
					return nil, scenario.getErr
				},
				application: func(context.Context, string) (*model.Application, error) {
					require.NoError(t, scenario.getErr, "failed event reads must not query the application")
					return scenario.app, scenario.appErr
				},
			}
			_, err := (&RepositoryReadService{Repository: repo}).GetStandingChange(t.Context(), validStandingChangeParams())
			require.ErrorIs(t, err, scenario.want)
		})
	}
	t.Run("empty page preserves total and an array", func(t *testing.T) {
		repo := &standingChangeRepository{
			list: func(context.Context, string, uint64, string, repository.Pagination, bool) ([]*model.StandingChanged, uint64, error) {
				return nil, 7, nil
			},
			application: func(context.Context, string) (*model.Application, error) { return &model.Application{ID: 1}, nil },
		}
		identity := validStandingChangeParams()
		result, err := (&RepositoryReadService{Repository: repo}).ListStandingChanges(t.Context(), api.ListStandingChangesParams{
			Application: identity.Application, EpochIndex: identity.EpochIndex, TournamentAddress: identity.TournamentAddress,
			Limit: 3, Offset: 9,
		})
		require.NoError(t, err)
		require.JSONEq(t, `{"data":[],"pagination":{"total_count":7,"limit":3,"offset":9}}`, string(result))
	})
}
