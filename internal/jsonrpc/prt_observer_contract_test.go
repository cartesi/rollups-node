// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const (
	observerGetBondEventMethod      = "cartesi_getBondEvent"
	observerListBondEventsMethod    = "cartesi_listBondEvents"
	observerGetMatchAdvanceMethod   = "cartesi_getMatchAdvance"
	observerListMatchAdvancesMethod = "cartesi_listMatchAdvances"
	observerTestApplication         = "app"
	observerUint64Overflow          = "0x10000000000000000"
	observerApplicationKey          = "application"
	observerEpochIndexKey           = "epoch_index"
	observerTournamentAddressKey    = "tournament_address"
	observerMatchIDKey              = "id_hash"
	observerLogIndexKey             = "log_index"
	observerTxHashKey               = "tx_hash"
)

type observerAPIRepository struct {
	repository.Repository
	app        *Application
	bond       *BondEvent
	advance    *MatchAdvanced
	tournament *Tournament
	commitment *Commitment
	match      *Match
	err        error

	application       string
	txHash            common.Hash
	logIndex          uint64
	bondFilter        repository.BondEventFilter
	epochIndex        uint64
	tournamentAddress string
	idHash            string
	pagination        repository.Pagination
	descending        bool
}

func (r *observerAPIRepository) GetApplication(context.Context, string) (*Application, error) {
	return r.app, r.err
}

func (r *observerAPIRepository) GetBondEvent(_ context.Context, app string, tx common.Hash, index uint64) (*BondEvent, error) {
	r.application, r.txHash, r.logIndex = app, tx, index
	return r.bond, r.err
}

func (r *observerAPIRepository) GetMatchAdvanced(_ context.Context, app string, epoch uint64, tournament, id string,
	tx common.Hash, index uint64,
) (*MatchAdvanced, error) {
	r.application, r.txHash, r.logIndex = app, tx, index
	r.epochIndex, r.tournamentAddress, r.idHash = epoch, tournament, id
	return r.advance, r.err
}

func (r *observerAPIRepository) ListBondEvents(_ context.Context, app string, f repository.BondEventFilter,
	p repository.Pagination, descending bool,
) ([]*BondEvent, uint64, error) {
	r.application, r.bondFilter, r.pagination, r.descending = app, f, p, descending
	if r.bond == nil || r.err != nil {
		return nil, 0, r.err
	}
	return []*BondEvent{r.bond}, 1, nil
}

func (r *observerAPIRepository) ListMatchAdvances(_ context.Context, app string, epoch uint64, tournament, id string,
	p repository.Pagination, descending bool,
) ([]*MatchAdvanced, uint64, error) {
	r.application, r.pagination, r.descending = app, p, descending
	r.epochIndex, r.tournamentAddress, r.idHash = epoch, tournament, id
	if r.advance == nil || r.err != nil {
		return nil, 0, r.err
	}
	return []*MatchAdvanced{r.advance}, 1, nil
}

func (r *observerAPIRepository) GetTournament(context.Context, string, string) (*Tournament, error) {
	return r.tournament, r.err
}

func (r *observerAPIRepository) GetCommitment(context.Context, string, uint64, string, string) (*Commitment, error) {
	return r.commitment, r.err
}

func (r *observerAPIRepository) GetMatch(context.Context, string, uint64, string, string) (*Match, error) {
	return r.match, r.err
}

func observerAPIRequest(t *testing.T, repo *observerAPIRepository, method string, params any) (json.RawMessage, error) {
	t.Helper()
	s := &Service{repository: repo}
	s.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	encoded, err := json.Marshal(params)
	require.NoError(t, err)
	result, err := jsonrpcHandlers[method](s, httptest.NewRequest(http.MethodPost, "/", nil), RPCRequest{Params: encoded})
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	return data, nil
}

func TestTournamentEventAPIIdentityAndErrors(t *testing.T) {
	tx := common.HexToHash("0xabc")
	tournament, id := common.HexToAddress("0x12"), common.HexToHash("0x34")
	for _, method := range []string{observerGetBondEventMethod, observerGetMatchAdvanceMethod} {
		t.Run(method, func(t *testing.T) {
			repo := &observerAPIRepository{app: &Application{},
				bond:    &BondEvent{TxHash: tx, Type: BondEventPartialRefund, Refund: &PartialBondRefund{}},
				advance: &MatchAdvanced{TxHash: tx}}
			for _, index := range []string{"0x0", "0xffffffffffffffff"} {
				params := map[string]string{
					observerApplicationKey: observerTestApplication, observerTxHashKey: tx.Hex(), observerLogIndexKey: index,
				}
				if method == observerGetMatchAdvanceMethod {
					params[observerEpochIndexKey], params[observerTournamentAddressKey], params[observerMatchIDKey] =
						"0x7", tournament.Hex(), id.Hex()
				}
				_, err := observerAPIRequest(t, repo, method, params)
				require.NoError(t, err)
				require.Equal(t, observerTestApplication, repo.application)
				require.Equal(t, tx, repo.txHash)
				if method == observerGetMatchAdvanceMethod {
					require.Equal(t, uint64(7), repo.epochIndex)
					require.Equal(t, tournament.Hex(), repo.tournamentAddress)
					require.Equal(t, id.Hex(), repo.idHash)
				}
				if index == "0x0" {
					require.Zero(t, repo.logIndex)
				} else {
					require.Equal(t, ^uint64(0), repo.logIndex)
				}
			}
			for name, params := range map[string]map[string]string{
				"missing hash":  {observerApplicationKey: observerTestApplication, observerLogIndexKey: "0x0"},
				"short hash":    {observerApplicationKey: observerTestApplication, observerTxHashKey: "0x01", observerLogIndexKey: "0x0"},
				"missing index": {observerApplicationKey: observerTestApplication, observerTxHashKey: tx.Hex()},
				"decimal index": {observerApplicationKey: observerTestApplication, observerTxHashKey: tx.Hex(), observerLogIndexKey: "1"},
				"overflow index": {
					observerApplicationKey: observerTestApplication, observerTxHashKey: tx.Hex(),
					observerLogIndexKey: observerUint64Overflow,
				},
				"legacy payload key": {observerApplicationKey: observerTestApplication, observerEpochIndexKey: "0x0", "parent": tx.Hex()},
			} {
				t.Run(name, func(t *testing.T) {
					_, err := observerAPIRequest(t, &observerAPIRepository{}, method, params)
					var rpcErr *RPCError
					require.ErrorAs(t, err, &rpcErr)
					require.Equal(t, JSONRPC_INVALID_PARAMS, rpcErr.Code)
				})
			}
			params := map[string]string{
				observerApplicationKey: observerTestApplication, observerTxHashKey: tx.Hex(), observerLogIndexKey: "0x0",
			}
			if method == observerGetMatchAdvanceMethod {
				params[observerEpochIndexKey], params[observerTournamentAddressKey], params[observerMatchIDKey] =
					"0x7", tournament.Hex(), id.Hex()
			}
			for _, test := range []struct {
				name string
				repo *observerAPIRepository
				code int
			}{
				{"unknown application", &observerAPIRepository{}, JSONRPC_APPLICATION_NOT_FOUND},
				{"unknown event", &observerAPIRepository{app: &Application{}}, JSONRPC_RESOURCE_NOT_FOUND},
				{"repository failure", &observerAPIRepository{err: errors.New("read failed")}, JSONRPC_INTERNAL_ERROR},
			} {
				t.Run(test.name, func(t *testing.T) {
					_, err := observerAPIRequest(t, test.repo, method, params)
					var rpcErr *RPCError
					require.ErrorAs(t, err, &rpcErr)
					require.Equal(t, test.code, rpcErr.Code)
				})
			}
		})
	}
}

func TestTournamentEventAPIListFilters(t *testing.T) {
	tournament, id := common.HexToAddress("0x12"), common.HexToHash("0x34")
	for _, method := range []string{observerListBondEventsMethod, observerListMatchAdvancesMethod} {
		t.Run(method, func(t *testing.T) {
			repo := &observerAPIRepository{app: &Application{}}
			params := map[string]any{observerApplicationKey: observerTestApplication}
			if method == observerListMatchAdvancesMethod {
				params[observerEpochIndexKey], params[observerTournamentAddressKey], params[observerMatchIDKey] =
					"0x7", tournament.Hex(), id.Hex()
			}
			response, err := observerAPIRequest(t, repo, method, params)
			require.NoError(t, err)
			require.JSONEq(t, `{"data":[],"pagination":{"total_count":0,"limit":50,"offset":0}}`, string(response))
			require.Equal(t, repository.BondEventFilter{}, repo.bondFilter)
			params[observerEpochIndexKey], params[observerTournamentAddressKey] = "0x7", tournament.Hex()
			params["limit"], params["offset"], params["descending"] = 20000, 3, true
			_, err = observerAPIRequest(t, repo, method, params)
			require.NoError(t, err)
			require.Equal(t, repository.Pagination{Limit: LIST_ITEM_LIMIT, Offset: 3}, repo.pagination)
			require.True(t, repo.descending)
			if method == observerListBondEventsMethod {
				require.Equal(t, repository.BondEventFilter{EpochIndex: new(uint64(7)), TournamentAddress: &tournament}, repo.bondFilter)
			} else {
				require.Equal(t, uint64(7), repo.epochIndex)
				require.Equal(t, tournament.Hex(), repo.tournamentAddress)
				require.Equal(t, id.Hex(), repo.idHash)
			}
		})
	}
}

func TestMatchAdvanceAPIRequiresMatchScope(t *testing.T) {
	for _, method := range []string{observerListMatchAdvancesMethod, observerGetMatchAdvanceMethod} {
		for field, invalidValues := range map[string][]any{
			observerApplicationKey:       {nil, json.RawMessage("null"), "", "invalid/name"},
			observerEpochIndexKey:        {nil, json.RawMessage("null"), "", "1", observerUint64Overflow},
			observerTournamentAddressKey: {nil, json.RawMessage("null"), "", "not-an-address"},
			observerMatchIDKey:           {nil, json.RawMessage("null"), "", "0x01"},
		} {
			for _, value := range invalidValues {
				t.Run(fmt.Sprintf("%s/%s/%v", method, field, value), func(t *testing.T) {
					params := map[string]any{observerApplicationKey: observerTestApplication, observerEpochIndexKey: "0x0",
						observerTournamentAddressKey: (common.Address{}).Hex(), observerMatchIDKey: (common.Hash{}).Hex(),
						observerTxHashKey: (common.Hash{}).Hex(), observerLogIndexKey: "0x0"}
					if value == nil {
						delete(params, field)
					} else {
						params[field] = value
					}
					repo := &observerAPIRepository{}
					_, err := observerAPIRequest(t, repo, method, params)
					var rpcErr *RPCError
					require.ErrorAs(t, err, &rpcErr)
					require.Equal(t, JSONRPC_INVALID_PARAMS, rpcErr.Code)
					require.Empty(t, repo.application, "invalid scope must not reach the repository")
				})
			}
		}
	}
}

func TestBondEventAPIKeepsExactOutcomes(t *testing.T) {
	maximum, err := Uint256FromBig(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)))
	require.NoError(t, err)
	for _, success := range []bool{false, true} {
		repo := &observerAPIRepository{bond: &BondEvent{Type: BondEventPartialRefund, BlockNumber: 12, LogIndex: 0,
			Refund: &PartialBondRefund{Recipient: common.HexToAddress("0x1"), Value: maximum, Success: success}}}
		response, err := observerAPIRequest(t, repo, observerGetBondEventMethod, api.GetBondEventParams{
			Application: observerTestApplication, TxHash: repo.bond.TxHash.Hex(), LogIndex: "0x0",
		})
		require.NoError(t, err)
		var data struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response, &data))
		require.Equal(t, "null", string(data.Data["recovery"]))
		require.JSONEq(t, fmt.Sprintf(`{"recipient":%q,"value":%q,"success":%t}`,
			repo.bond.Refund.Recipient.Hex(), "0x"+strings.Repeat("f", 64), success), string(data.Data["refund"]))
		require.Equal(t, `"0x0"`, string(data.Data[observerLogIndexKey]))
	}
	repo := &observerAPIRepository{bond: &BondEvent{Type: BondEventRecovered,
		Recovery: &BondRecovered{Payment: Uint256{}, Burned: maximum}}}
	response, err := observerAPIRequest(t, repo, observerGetBondEventMethod, api.GetBondEventParams{
		Application: observerTestApplication, TxHash: repo.bond.TxHash.Hex(), LogIndex: "0x0",
	})
	require.NoError(t, err)
	var result struct {
		Data BondEvent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response, &result))
	require.Nil(t, result.Data.Refund)
	require.Equal(t, repo.bond.Recovery, result.Data.Recovery)
}

func TestTournamentAPISeparatesCurrentStateFromFacts(t *testing.T) {
	address := common.HexToAddress("0x12")
	candidate := common.HexToHash("0x34")
	repo := &observerAPIRepository{tournament: &Tournament{Address: address, Level: 1,
		Snapshot: TournamentSnapshot{AsOfBlock: 100, Standing: TournamentStandingInnerEliminableWinnerExpired,
			Candidate: &candidate, FinishedAtBlock: 80,
			InnerResult:  &TournamentInnerResult{Disposition: InnerTournamentEliminable},
			BondRecovery: TournamentBondRecovery{Disposition: BondDispositionRecovered}}}}
	response, err := observerAPIRequest(t, repo, "cartesi_getTournament",
		api.GetTournamentParams{Application: observerTestApplication, Address: address.Hex()})
	require.NoError(t, err)
	var decoded struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response, &decoded))
	for _, field := range []string{"winner_commitment", "final_state_hash", "finished_at_block"} {
		require.NotContains(t, decoded.Data, field, "current result must not have a duplicate top-level field")
	}
	var snapshot map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(decoded.Data["snapshot"], &snapshot))
	require.Equal(t, `"0x64"`, string(snapshot["as_of_block"]))
	require.Equal(t, `"`+candidate.Hex()+`"`, string(snapshot["candidate"]))
	require.Equal(t, "null", string(snapshot["winner_commitment"]))
	require.Equal(t, "null", string(snapshot["final_state_hash"]))
	require.Equal(t, "null", string(snapshot["parent_commitment"]))
	require.Equal(t, `"0x0"`, string(snapshot["winner_expires_at"]))
}

func TestMatchAPIPreservesPhasePayloads(t *testing.T) {
	for _, phase := range []MatchPhase{MatchPhaseUninitialized, MatchPhaseBisecting, MatchPhaseReadyToSeal, MatchPhaseSealed} {
		t.Run(string(phase), func(t *testing.T) {
			match := &Match{Snapshot: MatchSnapshot{AsOfBlock: 200, Phase: phase, TimeoutOutcome: MatchTimeoutNone}}
			switch phase {
			case MatchPhaseBisecting, MatchPhaseReadyToSeal:
				match.Snapshot.Bisection = &MatchBisectionSnapshot{Responder: CommitmentSideTwo}
				if phase == MatchPhaseBisecting {
					match.Snapshot.Bisection.CurrentHeight = new(uint64(4))
				}
			case MatchPhaseSealed:
				match.Snapshot.Sealed = &MatchSealedSnapshot{}
			case MatchPhaseUninitialized:
				match.DeletionReason = MatchDeletionReason_TIMEOUT
				match.DeletionBlockNumber = 190
				match.DeletionTxHash, match.DeletionLogIndex = new(common.Hash), new(uint64)
			}
			response, err := observerAPIRequest(t, &observerAPIRepository{match: match}, "cartesi_getMatch", api.GetMatchParams{
				Application: observerTestApplication, EpochIndex: "0x0",
				TournamentAddress: match.TournamentAddress.Hex(), IDHash: match.IDHash.Hex(),
			})
			require.NoError(t, err)
			var result struct {
				Data struct {
					Snapshot MatchSnapshot `json:"snapshot"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response, &result))
			require.Equal(t, match.Snapshot, result.Data.Snapshot)
		})
	}
}

func TestCommitmentAPIKeepsOriginalSubmitterAfterRecovery(t *testing.T) {
	commitment := &Commitment{SubmitterAddress: common.HexToAddress("0x12"), FinalStateHash: common.HexToHash("0x34"),
		Snapshot: CommitmentSnapshot{AsOfBlock: 100, ClockAllowance: 50}}
	response, err := observerAPIRequest(t, &observerAPIRepository{commitment: commitment}, "cartesi_getCommitment",
		api.GetCommitmentParams{Application: observerTestApplication, EpochIndex: "0x0",
			TournamentAddress: commitment.TournamentAddress.Hex(), Commitment: commitment.Commitment.Hex()})
	require.NoError(t, err)
	var decoded struct {
		Data Commitment `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response, &decoded))
	require.Equal(t, commitment.SubmitterAddress, decoded.Data.SubmitterAddress)
	require.Equal(t, commitment.FinalStateHash, decoded.Data.FinalStateHash)
	require.Equal(t, common.Address{}, decoded.Data.Snapshot.Claimer)
	require.Equal(t, commitment.Snapshot, decoded.Data.Snapshot)
}

func TestMatchAdvanceAPIKeepsFullWidthPosition(t *testing.T) {
	position, err := Uint256FromBig(new(big.Int).Lsh(big.NewInt(1), 200))
	require.NoError(t, err)
	advance := &MatchAdvanced{EpochIndex: 7, TournamentAddress: common.HexToAddress("0x12"),
		IDHash: common.HexToHash("0x34"), OtherParent: common.HexToHash("0x56"), LeftNode: common.HexToHash("0x78"),
		BlockNumber: 100, TxHash: common.HexToHash("0x90"), SegmentStartPosition: position, EliminableAt: 300, LogIndex: 4}
	expected, err := json.Marshal(advance)
	require.NoError(t, err)
	for _, method := range []string{observerGetMatchAdvanceMethod, observerListMatchAdvancesMethod} {
		t.Run(method, func(t *testing.T) {
			params := map[string]any{observerApplicationKey: observerTestApplication, observerEpochIndexKey: "0x7",
				observerTournamentAddressKey: advance.TournamentAddress.Hex(), observerMatchIDKey: advance.IDHash.Hex()}
			if method == observerGetMatchAdvanceMethod {
				params[observerTxHashKey], params[observerLogIndexKey] = advance.TxHash.Hex(), "0x4"
			}
			response, err := observerAPIRequest(t, &observerAPIRepository{advance: advance}, method, params)
			require.NoError(t, err)
			var result struct {
				Data json.RawMessage `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response, &result))
			if method == observerListMatchAdvancesMethod {
				var records []json.RawMessage
				require.NoError(t, json.Unmarshal(result.Data, &records))
				require.Len(t, records, 1)
				result.Data = records[0]
			}
			require.JSONEq(t, string(expected), string(result.Data), "list and get must return the complete same record")
			var decoded struct {
				Position     Uint256     `json:"segment_start_position"`
				EliminableAt string      `json:"eliminable_at"`
				LogIndex     string      `json:"log_index"`
				TxHash       common.Hash `json:"tx_hash"`
			}
			require.NoError(t, json.Unmarshal(result.Data, &decoded))
			require.Equal(t, position, decoded.Position)
			require.Equal(t, "0x12c", decoded.EliminableAt)
			require.Equal(t, "0x4", decoded.LogIndex)
			require.Equal(t, advance.TxHash, decoded.TxHash)
		})
	}
}

func TestDiscoverySchemaPassiveObserver(t *testing.T) {
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	require.NoError(t, err)
	var spec struct {
		Methods []struct {
			Name   string `json:"name"`
			Params []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"params"`
		} `json:"methods"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(data, &spec))
	expected := map[string][]string{
		observerGetBondEventMethod: {observerApplicationKey, observerTxHashKey, observerLogIndexKey},
		observerListBondEventsMethod: {
			observerApplicationKey, observerEpochIndexKey, observerTournamentAddressKey, "limit", "offset", "descending",
		},
		observerGetMatchAdvanceMethod: {
			observerApplicationKey, observerEpochIndexKey, observerTournamentAddressKey, observerMatchIDKey,
			observerTxHashKey, observerLogIndexKey,
		},
		observerListMatchAdvancesMethod: {
			observerApplicationKey, observerEpochIndexKey, observerTournamentAddressKey, observerMatchIDKey,
			"limit", "offset", "descending",
		},
	}
	for _, method := range spec.Methods {
		if want, ok := expected[method.Name]; ok {
			names := make([]string, 0, len(method.Params))
			requiredCount := len(want)
			switch method.Name {
			case observerListMatchAdvancesMethod:
				requiredCount = 4
			case observerListBondEventsMethod:
				requiredCount = 1
			}
			for index, param := range method.Params {
				names = append(names, param.Name)
				require.Equal(t, index < requiredCount, param.Required)
			}
			require.Equal(t, want, names)
			delete(expected, method.Name)
		}
	}
	require.Empty(t, expected, "all observer methods must be documented")
	var wide struct {
		Pattern string `json:"pattern"`
	}
	require.NoError(t, json.Unmarshal(spec.Components.Schemas["Uint256"], &wide))
	pattern, err := regexp.Compile(wide.Pattern)
	require.NoError(t, err)
	for _, valid := range []string{"0x0", "0x1", observerUint64Overflow, "0x" + strings.Repeat("f", 64)} {
		require.True(t, pattern.MatchString(valid), valid)
	}
	for _, invalid := range []string{"0x", "0x00", "-0x1", "0x" + strings.Repeat("f", 65), "1", "0x1.5"} {
		require.False(t, pattern.MatchString(invalid), invalid)
	}
	for _, name := range []string{"Tournament", "Commitment", "Match"} {
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(spec.Components.Schemas[name], &schema))
		require.Contains(t, schema.Properties, "snapshot")
		if name == "Tournament" {
			require.NotContains(t, schema.Properties, "winner_commitment")
			require.NotContains(t, schema.Properties, "finished_at_block")
		}
	}
}
