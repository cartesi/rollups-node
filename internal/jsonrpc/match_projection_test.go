// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

type matchProjectionRepository struct {
	repository.Repository
	match *Match
}

func (r *matchProjectionRepository) GetMatch(context.Context, string, uint64, string, string) (*Match, error) {
	return r.match, nil
}

func TestMatchDeletionTransactionProjection(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		name := "not deleted"
		if deleted {
			name = "deleted with zero hash"
		}
		t.Run(name, func(t *testing.T) {
			match := &Match{TournamentAddress: common.HexToAddress("0x1"), IDHash: common.HexToHash("0x2"),
				Winner: WinnerCommitment_NONE, DeletionReason: MatchDeletionReason_NOT_DELETED}
			if deleted {
				match.DeletionReason = MatchDeletionReason_TIMEOUT
				match.DeletionBlockNumber = 1
				match.DeletionTxHash = new(common.Hash)
			}
			params, err := json.Marshal(api.GetMatchParams{Application: "app", EpochIndex: "0x0",
				TournamentAddress: match.TournamentAddress.Hex(), IDHash: match.IDHash.Hex()})
			require.NoError(t, err)
			s := &Service{repository: &matchProjectionRepository{match: match}}
			response, err := handleGetMatch(s, httptest.NewRequest(http.MethodPost, "/", nil), RPCRequest{Params: params})
			require.NoError(t, err)
			data, err := json.Marshal(response)
			require.NoError(t, err)
			var decoded struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			require.NoError(t, json.Unmarshal(data, &decoded))
			if deleted {
				require.JSONEq(t, `"`+(common.Hash{}).Hex()+`"`, string(decoded.Data["deletion_tx_hash"]))
			} else {
				require.Equal(t, "null", string(decoded.Data["deletion_tx_hash"]))
			}
		})
	}
}

func TestDiscoverySchemaNullableMatchDeletionTransaction(t *testing.T) {
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	require.NoError(t, err)
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(data, &spec))
	require.JSONEq(t, `{"oneOf":[{"$ref":"#/components/schemas/Hash"},{"type":"null"}]}`,
		string(spec.Components.Schemas["Match"].Properties["deletion_tx_hash"]))
}
