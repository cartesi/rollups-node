// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"encoding/json"
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

const invalidMatchAdvanceLogIndex = "invalid log index"

func validMatchAdvanceParams() api.GetMatchAdvanceParams {
	return api.GetMatchAdvanceParams{Application: "echo-dapp", EpochIndex: "0x10",
		TournamentAddress: common.HexToAddress("0xabc").Hex(), IDHash: common.HexToHash("0xdef").Hex(),
		TxHash: common.HexToHash("0x123").Hex(), LogIndex: "0x0"}
}

func TestMatchAdvanceBackendsRejectInvalidIdentityBeforeIO(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*api.GetMatchAdvanceParams)
		message string
	}{
		{"application", func(p *api.GetMatchAdvanceParams) { p.Application = "" }, "invalid application"},
		{"epoch", func(p *api.GetMatchAdvanceParams) { p.EpochIndex = "" }, "invalid epoch index"},
		{"tournament", func(p *api.GetMatchAdvanceParams) { p.TournamentAddress = "" }, "invalid tournament address"},
		{"match", func(p *api.GetMatchAdvanceParams) { p.IDHash = "" }, "invalid ID hash"},
		{"transaction absent", func(p *api.GetMatchAdvanceParams) { p.TxHash = "" }, "invalid transaction hash"},
		{"transaction short", func(p *api.GetMatchAdvanceParams) { p.TxHash = "0x123" }, "invalid transaction hash"},
		{"log index absent", func(p *api.GetMatchAdvanceParams) { p.LogIndex = "" }, invalidMatchAdvanceLogIndex},
		{"log index decimal", func(p *api.GetMatchAdvanceParams) { p.LogIndex = "12" }, invalidMatchAdvanceLogIndex},
		{"log index negative", func(p *api.GetMatchAdvanceParams) { p.LogIndex = "-1" }, invalidMatchAdvanceLogIndex},
		{"log index overflow", func(p *api.GetMatchAdvanceParams) { p.LogIndex = "0x10000000000000000" }, invalidMatchAdvanceLogIndex},
	} {
		for _, backend := range []struct {
			name    string
			service ReadService
		}{
			{"repository", &RepositoryReadService{}}, {"jsonrpc", &JsonrpcReadService{}},
		} {
			t.Run(backend.name+"/"+test.name, func(t *testing.T) {
				params := validMatchAdvanceParams()
				test.change(&params)
				// Both dependencies are nil: reaching I/O before validation fails this test.
				_, err := backend.service.GetMatchAdvanced(t.Context(), params)
				require.ErrorContains(t, err, test.message)
			})
		}
	}
}

type matchAdvanceRepository struct {
	repository.Repository
	get  func(context.Context, string, uint64, string, string, common.Hash, uint64) (*model.MatchAdvanced, error)
	list func(context.Context, string, uint64, string, string, repository.Pagination, bool) ([]*model.MatchAdvanced, uint64, error)
}

func (r *matchAdvanceRepository) GetMatchAdvanced(ctx context.Context, app string, epoch uint64, tournament, id string,
	tx common.Hash, logIndex uint64) (*model.MatchAdvanced, error) {
	return r.get(ctx, app, epoch, tournament, id, tx, logIndex)
}

func (r *matchAdvanceRepository) ListMatchAdvances(ctx context.Context, app string, epoch uint64, tournament, id string,
	pagination repository.Pagination, descending bool) ([]*model.MatchAdvanced, uint64, error) {
	return r.list(ctx, app, epoch, tournament, id, pagination, descending)
}

func TestRepositoryMatchAdvanceUsesScopedEventIdentity(t *testing.T) {
	params := validMatchAdvanceParams()
	called := false
	repo := &matchAdvanceRepository{get: func(ctx context.Context, app string, epoch uint64, tournament, id string,
		tx common.Hash, logIndex uint64) (*model.MatchAdvanced, error) {
		called = true
		require.Equal(t, t.Context(), ctx)
		require.Equal(t, params.Application, app)
		require.Equal(t, uint64(16), epoch)
		require.Equal(t, params.TournamentAddress, tournament)
		require.Equal(t, params.IDHash, id)
		require.Equal(t, common.HexToHash(params.TxHash), tx)
		require.Zero(t, logIndex, "block log index zero is valid")
		return &model.MatchAdvanced{TxHash: tx, LogIndex: logIndex}, nil
	}}
	s := &RepositoryReadService{Repository: repo}
	result, err := s.GetMatchAdvanced(t.Context(), params)
	require.NoError(t, err)
	require.True(t, called)
	var response api.SingleResponse[*model.MatchAdvanced]
	require.NoError(t, json.Unmarshal(result, &response))
	require.Equal(t, common.HexToHash(params.TxHash), response.Data.TxHash)
	require.Zero(t, response.Data.LogIndex)
}

func TestRepositoryMatchAdvanceListKeepsMatchScopeAndPagination(t *testing.T) {
	identity := validMatchAdvanceParams()
	params := api.ListMatchAdvancesParams{Application: identity.Application, EpochIndex: identity.EpochIndex,
		TournamentAddress: identity.TournamentAddress, IDHash: identity.IDHash, Limit: 3, Offset: 2, Descending: true}
	called := false
	repo := &matchAdvanceRepository{list: func(_ context.Context, app string, epoch uint64, tournament, id string,
		pagination repository.Pagination, descending bool) ([]*model.MatchAdvanced, uint64, error) {
		called = true
		require.Equal(t, params.Application, app)
		require.Equal(t, uint64(16), epoch)
		require.Equal(t, params.TournamentAddress, tournament)
		require.Equal(t, params.IDHash, id)
		require.Equal(t, repository.Pagination{Limit: 3, Offset: 2}, pagination)
		require.True(t, descending)
		return []*model.MatchAdvanced{{LogIndex: 4}}, 7, nil
	}}
	result, err := (&RepositoryReadService{Repository: repo}).ListMatchAdvances(t.Context(), params)
	require.NoError(t, err)
	require.True(t, called)
	var response api.ListResponse[*model.MatchAdvanced]
	require.NoError(t, json.Unmarshal(result, &response))
	require.Equal(t, uint64(7), response.Pagination.TotalCount)
	require.Equal(t, uint64(3), response.Pagination.Limit)
	require.Equal(t, uint64(2), response.Pagination.Offset)
}

type matchAdvanceTransport func(*http.Request) (*http.Response, error)

func (transport matchAdvanceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestJSONRPCMatchAdvanceSendsScopedParams(t *testing.T) {
	identity := validMatchAdvanceParams()
	for _, get := range []bool{false, true} {
		name := "list"
		method := "cartesi_listMatchAdvances"
		if get {
			name, method = "get", "cartesi_getMatchAdvance"
		}
		t.Run(name, func(t *testing.T) {
			called := false
			transport := matchAdvanceTransport(func(request *http.Request) (*http.Response, error) {
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
				require.Equal(t, identity.IDHash, wire.Params["id_hash"])
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
				_, err = s.GetMatchAdvanced(t.Context(), identity)
			} else {
				_, err = s.ListMatchAdvances(t.Context(), api.ListMatchAdvancesParams{Application: identity.Application,
					EpochIndex: identity.EpochIndex, TournamentAddress: identity.TournamentAddress, IDHash: identity.IDHash,
					Limit: 3, Offset: 2, Descending: true})
			}
			require.NoError(t, err)
			require.True(t, called)
		})
	}
}
