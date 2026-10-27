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
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const (
	observerListStandingChangesMethod = "cartesi_listStandingChanges"
	observerGetStandingChangeMethod   = "cartesi_getStandingChange"
	observerGetBondEventMethod        = "cartesi_getBondEvent"
	observerListBondEventsMethod      = "cartesi_listBondEvents"
	observerGetMatchAdvanceMethod     = "cartesi_getMatchAdvance"
	observerListMatchAdvancesMethod   = "cartesi_listMatchAdvances"
	observerTestApplication           = "app"
	observerUint64Overflow            = "0x10000000000000000"
	observerApplicationKey            = "application"
	observerEpochIndexKey             = "epoch_index"
	observerTournamentAddressKey      = "tournament_address"
	observerMatchIDKey                = "id_hash"
	observerLogIndexKey               = "log_index"
	observerTxHashKey                 = "tx_hash"
	obsUintMax                        = "0xffffffffffffffff"
	obsTournament                     = "Tournament"
	obsDescKey                        = "descending"
	obsNullJSON                       = "null"
	obsLimit                          = "limit"
	obsOffset                         = "offset"
)

type observerAPIRepository struct {
	repository.Repository
	standing       *StandingChanged
	standings      []*StandingChanged
	total          uint64
	standingCalls  int
	appLookups     int
	appErr         error
	seenContext    context.Context
	respectContext bool
	app            *Application
	bond           *BondEvent
	advance        *MatchAdvanced
	tournament     *Tournament
	commitment     *Commitment
	match          *Match
	err            error

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
	r.appLookups++
	if r.appErr != nil {
		return nil, r.appErr
	}
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

func (r *observerAPIRepository) ListStandingChanges(ctx context.Context, app string, epoch uint64, tournament string,
	p repository.Pagination, descending bool,
) ([]*StandingChanged, uint64, error) {
	r.standingCalls++
	r.application, r.epochIndex, r.tournamentAddress = app, epoch, tournament
	r.pagination, r.descending, r.seenContext = p, descending, ctx
	if r.respectContext && ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}
	return r.standings, r.total, r.err
}

func (r *observerAPIRepository) GetStandingChange(ctx context.Context, app string, epoch uint64, tournament string,
	tx common.Hash, index uint64,
) (*StandingChanged, error) {
	r.standingCalls++
	r.application, r.epochIndex, r.tournamentAddress = app, epoch, tournament
	r.txHash, r.logIndex, r.seenContext = tx, index, ctx
	if r.respectContext && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return r.standing, r.err
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
		BlockNumber: 100, TxHash: common.HexToHash("0x90"), SegmentStartPosition: position, EliminableAt: 300, LogIndex: 4,
		CurrentHeight: 1, ResponderDeadline: 211}
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
				Position          Uint256     `json:"segment_start_position"`
				Height            string      `json:"current_height"`
				ResponderDeadline string      `json:"responder_deadline"`
				EliminableAt      string      `json:"eliminable_at"`
				LogIndex          string      `json:"log_index"`
				TxHash            common.Hash `json:"tx_hash"`
			}
			require.NoError(t, json.Unmarshal(result.Data, &decoded))
			require.Equal(t, position, decoded.Position)
			require.Equal(t, "0x1", decoded.Height)
			require.Equal(t, "0xd3", decoded.ResponderDeadline)
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

func TestStandingChangesScopeAndPagination(t *testing.T) {
	tournament, tx := common.HexToAddress("0x12"), common.HexToHash("0x34")
	for _, test := range []struct {
		epoch, logIndex                        string
		epochValue, logValue, limit, wantLimit uint64
		positional                             bool
	}{
		{epoch: "0x0", logIndex: obsUintMax, logValue: math.MaxUint64, wantLimit: 50},
		{epoch: obsUintMax, logIndex: "0x0", epochValue: math.MaxUint64, limit: 10001, wantLimit: 10000, positional: true},
		{epoch: "0x10", logIndex: "0x1", epochValue: 16, logValue: 1, limit: 7, wantLimit: 7},
	} {
		t.Run(test.epoch, func(t *testing.T) {
			params := any(map[string]any{observerApplicationKey: observerTestApplication, observerEpochIndexKey: test.epoch,
				observerTournamentAddressKey: tournament.Hex(), obsLimit: test.limit, obsOffset: uint64(math.MaxInt64), obsDescKey: true})
			if test.positional {
				params = []any{observerTestApplication, test.epoch, tournament.Hex(), test.limit, uint64(math.MaxInt64), true}
			}
			repo := &observerAPIRepository{standings: []*StandingChanged{{}}, total: 29}
			response, err := observerAPIRequest(t, repo, observerListStandingChangesMethod, params)
			require.NoError(t, err)
			require.Equal(t, observerTestApplication, repo.application)
			require.Equal(t, tournament.Hex(), repo.tournamentAddress)
			require.Equal(t, test.epochValue, repo.epochIndex)
			require.Equal(t, repository.Pagination{Limit: test.wantLimit, Offset: math.MaxInt64}, repo.pagination)
			require.True(t, repo.descending)
			require.Zero(t, repo.appLookups, "nonempty results need no application lookup")
			var decoded api.ListResponse[*StandingChanged]
			require.NoError(t, json.Unmarshal(response, &decoded))
			require.Equal(t, api.Pagination{TotalCount: 29, Limit: test.wantLimit, Offset: math.MaxInt64}, decoded.Pagination)

			params = map[string]string{observerApplicationKey: observerTestApplication, observerEpochIndexKey: test.epoch,
				observerTournamentAddressKey: tournament.Hex(), observerTxHashKey: tx.Hex(), observerLogIndexKey: test.logIndex}
			if test.positional {
				params = []any{observerTestApplication, test.epoch, tournament.Hex(), tx.Hex(), test.logIndex}
			}
			repo = &observerAPIRepository{standing: &StandingChanged{}}
			_, err = observerAPIRequest(t, repo, observerGetStandingChangeMethod, params)
			require.NoError(t, err)
			require.Equal(t, observerTestApplication, repo.application)
			require.Equal(t, tournament.Hex(), repo.tournamentAddress)
			require.Equal(t, tx, repo.txHash)
			require.Equal(t, test.epochValue, repo.epochIndex)
			require.Equal(t, test.logValue, repo.logIndex)
			require.Zero(t, repo.appLookups)
		})
	}
}

func TestStandingChangesRejectInvalidParamsBeforeRepository(t *testing.T) {
	for _, method := range []string{observerListStandingChangesMethod, observerGetStandingChangeMethod} {
		valid := map[string]any{observerApplicationKey: observerTestApplication, observerEpochIndexKey: "0x0",
			observerTournamentAddressKey: common.HexToAddress("0x12").Hex()}
		if method == observerGetStandingChangeMethod {
			valid[observerTxHashKey], valid[observerLogIndexKey] = common.HexToHash("0x34").Hex(), "0x0"
		}
		invalid := map[string][]any{observerApplicationKey: {"", "!"}, observerEpochIndexKey: {"", "0", observerUint64Overflow},
			observerTournamentAddressKey: {"", "0xzz"}}
		if method == observerGetStandingChangeMethod {
			invalid[observerTxHashKey] = []any{"", "0x12", "0x" + strings.Repeat("f", 66)}
			invalid[observerLogIndexKey] = []any{"", "0", observerUint64Overflow}
		} else {
			invalid[obsOffset] = []any{uint64(math.MaxInt64) + 1}
			invalid[obsLimit] = []any{-1, "50"}
		}
		for key, values := range invalid {
			for _, value := range values {
				params := make(map[string]any, len(valid))
				for k, v := range valid {
					params[k] = v
				}
				params[key] = value
				repo := &observerAPIRepository{}
				_, err := observerAPIRequest(t, repo, method, params)
				var rpcErr *RPCError
				require.ErrorAs(t, err, &rpcErr, "%s %s %v", method, key, value)
				require.Equal(t, JSONRPC_INVALID_PARAMS, rpcErr.Code)
				require.Zero(t, repo.standingCalls)
				require.Zero(t, repo.appLookups)
			}
		}
		for _, params := range []any{nil, []any{}, []any{observerTestApplication}, []any{observerTestApplication, "0x0"}, "invalid"} {
			repo := &observerAPIRepository{}
			_, err := observerAPIRequest(t, repo, method, params)
			var rpcErr *RPCError
			require.ErrorAs(t, err, &rpcErr)
			require.Equal(t, JSONRPC_INVALID_PARAMS, rpcErr.Code)
			require.Zero(t, repo.standingCalls)
		}
	}
}

func TestStandingChangesMissingPagesAndErrors(t *testing.T) {
	for _, method := range []string{observerListStandingChangesMethod, observerGetStandingChangeMethod} {
		params := map[string]any{observerApplicationKey: observerTestApplication, observerEpochIndexKey: "0x0",
			observerTournamentAddressKey: common.HexToAddress("0x12").Hex()}
		if method == observerGetStandingChangeMethod {
			params[observerTxHashKey], params[observerLogIndexKey] = common.HexToHash("0x34").Hex(), "0x0"
		}
		for _, test := range []struct {
			name        string
			app         *Application
			err, appErr error
			code        int
		}{
			{name: "missing application", code: JSONRPC_APPLICATION_NOT_FOUND},
			{name: "existing application", app: &Application{}, code: JSONRPC_RESOURCE_NOT_FOUND},
			{name: "repository failure", err: errors.New("private SQL detail"), code: JSONRPC_INTERNAL_ERROR},
			{name: "application lookup failure", appErr: errors.New("private lookup detail"), code: JSONRPC_INTERNAL_ERROR},
		} {
			t.Run(method+"/"+test.name, func(t *testing.T) {
				repo := &observerAPIRepository{app: test.app, err: test.err, appErr: test.appErr, total: 19}
				response, err := observerAPIRequest(t, repo, method, params)
				if test.app != nil && method == observerListStandingChangesMethod {
					require.NoError(t, err)
					require.JSONEq(t, `{"data":[],"pagination":{"total_count":19,"limit":50,"offset":0}}`, string(response))
				} else {
					var rpcErr *RPCError
					require.ErrorAs(t, err, &rpcErr)
					require.Equal(t, test.code, rpcErr.Code)
					require.NotContains(t, rpcErr.Message, "private")
				}
			})
		}
		for _, ctx := range []context.Context{func() context.Context { c, cancel := context.WithCancel(t.Context()); cancel(); return c }(),
			func() context.Context {
				c, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				t.Cleanup(cancel)
				return c
			}()} {
			repo := &observerAPIRepository{respectContext: true}
			s := newBatchTestService()
			s.repository = repo
			encoded, err := json.Marshal(params)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, "/rpc", nil).WithContext(ctx)
			_, err = jsonrpcHandlers[method](s, request, RPCRequest{Params: encoded})
			require.ErrorIs(t, err, ctx.Err())
			require.Same(t, ctx, repo.seenContext)
		}
	}
}

//nolint:goconst // Keep literal wire values independent from model serialization.
func TestStandingChangesWireFacts(t *testing.T) {
	count, err := Uint256FromBig(new(big.Int).Lsh(big.NewInt(1), 200))
	require.NoError(t, err)
	zero := common.Hash{}
	stamp := time.Date(2026, 10, 27, 1, 2, 3, 0, time.UTC)
	event := &StandingChanged{ApplicationID: 99, EpochIndex: 7, TournamentAddress: common.HexToAddress("0x12"), MatchCount: count,
		Dangling: &zero, ResultAt: 301, WinnerExpiresAt: 509, BlockNumber: 100,
		TxHash: common.HexToHash("0x34"), LogIndex: 0, CreatedAt: stamp, UpdatedAt: stamp}
	for _, method := range []string{observerListStandingChangesMethod, observerGetStandingChangeMethod} {
		params := map[string]any{observerApplicationKey: observerTestApplication, observerEpochIndexKey: "0x7",
			observerTournamentAddressKey: event.TournamentAddress.Hex()}
		if method == observerGetStandingChangeMethod {
			params[observerTxHashKey], params[observerLogIndexKey] = event.TxHash.Hex(), "0x0"
		}
		repo := &observerAPIRepository{standing: event, standings: []*StandingChanged{event}, total: 1}
		response, err := observerAPIRequest(t, repo, method, params)
		require.NoError(t, err)
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response, &envelope))
		if method == observerListStandingChangesMethod {
			var rows []json.RawMessage
			require.NoError(t, json.Unmarshal(envelope.Data, &rows))
			require.Len(t, rows, 1)
			envelope.Data = rows[0]
		}
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(envelope.Data, &fields))
		require.Len(t, fields, 12)
		for key, want := range map[string]string{
			observerEpochIndexKey: `"0x7"`, "match_count": `"0x100000000000000000000000000000000000000000000000000"`,
			"dangling": `"0x0000000000000000000000000000000000000000000000000000000000000000"`, "parent_commitment": obsNullJSON,
			"result_at": `"0x12d"`, "winner_expires_at": `"0x1fd"`, "block_number": `"0x64"`, observerLogIndexKey: `"0x0"`,
			"created_at": `"2026-10-27T01:02:03Z"`, "updated_at": `"2026-10-27T01:02:03Z"`} {
			require.Equal(t, want, string(fields[key]), key)
		}
		require.Equal(t, `"`+event.TournamentAddress.Hex()+`"`, string(fields[observerTournamentAddressKey]))
		require.Equal(t, `"`+event.TxHash.Hex()+`"`, string(fields[observerTxHashKey]))
		require.NotContains(t, fields, "application_id")
	}
}

func TestStandingChangesEmptyPageKeepsScopedTotal(t *testing.T) {
	repo := &observerAPIRepository{app: &Application{}, total: 3}
	response, err := observerAPIRequest(t, repo, observerListStandingChangesMethod,
		[]any{observerTestApplication, "0x0", common.HexToAddress("0x12").Hex(), 1, 100, true})
	require.NoError(t, err)
	require.JSONEq(t, `{"data":[],"pagination":{"total_count":3,"limit":1,"offset":100}}`, string(response))
	require.Equal(t, repository.Pagination{Limit: 1, Offset: 100}, repo.pagination)
	require.True(t, repo.descending)
	require.Equal(t, 1, repo.appLookups)
}

func TestAlpha6TournamentAndSealWireFacts(t *testing.T) {
	wide, err := Uint256FromBig(new(big.Int).Lsh(big.NewInt(1), 200))
	require.NoError(t, err)
	zero, other := common.Hash{}, common.HexToHash("0x22")
	for _, child := range []bool{false, true} {
		value := &Tournament{BondValue: wide}
		if child {
			value.Level = 1
			value.CreationEvent = &TournamentCreationEvent{BlockNumber: 7, LogIndex: 0}
			value.ContestedFinalStateOne = &zero
			value.ContestedFinalStateTwo = &other
		}
		response, err := observerAPIRequest(t, &observerAPIRepository{tournament: value}, "cartesi_getTournament",
			api.GetTournamentParams{Application: observerTestApplication, Address: common.HexToAddress("0x12").Hex()})
		require.NoError(t, err)
		var result struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response, &result))
		require.Equal(t, `"0x100000000000000000000000000000000000000000000000000"`, string(result.Data["bond_value"]))
		var creation map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result.Data["creation_event"], &creation))
		if child {
			require.Equal(t, `"0x7"`, string(creation["block_number"]))
			require.Equal(t, `"0x0"`, string(creation[observerLogIndexKey]))
			require.Equal(t, `"`+zero.Hex()+`"`, string(result.Data["contested_final_state_one"]))
			require.Equal(t, `"`+other.Hex()+`"`, string(result.Data["contested_final_state_two"]))
		} else {
			require.Equal(t, obsNullJSON, string(result.Data["creation_event"]))
			require.Equal(t, obsNullJSON, string(result.Data["contested_final_state_one"]))
			require.Equal(t, obsNullJSON, string(result.Data["contested_final_state_two"]))
		}
	}
	match := &Match{ResponderDeadline: 211, LeafSeal: &LeafMatchSeal{AgreeState: common.HexToHash("0x11"),
		DivergencePosition: wide, FinalStateOne: common.HexToHash("0x22"), FinalStateTwo: common.HexToHash("0x33"),
		DeadlineOne: 301, DeadlineTwo: 509, EliminableAt: 509}, DeletionReason: MatchDeletionReason_TIMEOUT,
		Snapshot: MatchSnapshot{Phase: MatchPhaseUninitialized}}
	response, err := observerAPIRequest(t, &observerAPIRepository{match: match}, "cartesi_getMatch",
		api.GetMatchParams{Application: observerTestApplication, EpochIndex: "0x0",
			TournamentAddress: common.HexToAddress("0x12").Hex(), IDHash: common.HexToHash("0x44").Hex()})
	require.NoError(t, err)
	var result struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response, &result))
	require.Equal(t, `"0xd3"`, string(result.Data["responder_deadline"]))
	var seal map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result.Data["leaf_seal"], &seal))
	for key, want := range map[string]string{"agree_state": `"` + common.HexToHash("0x11").Hex() + `"`,
		"divergence_position": `"0x100000000000000000000000000000000000000000000000000"`,
		"final_state_one":     `"` + common.HexToHash("0x22").Hex() + `"`, "final_state_two": `"` + common.HexToHash("0x33").Hex() + `"`,
		"deadline_one": `"0x12d"`, "deadline_two": `"0x1fd"`, "eliminable_at": `"0x1fd"`, observerLogIndexKey: `"0x0"`} {
		require.Equal(t, want, string(seal[key]), key)
	}
	var snapshot map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result.Data["snapshot"], &snapshot))
	require.Equal(t, obsNullJSON, string(snapshot["sealed"]), "deletion must not erase the durable leaf-seal facts")
}

type alpha6DiscoverySchema struct {
	Ref        string                           `json:"$ref"`
	Type       any                              `json:"type"`
	Format     string                           `json:"format"`
	Properties map[string]alpha6DiscoverySchema `json:"properties"`
	Required   []string                         `json:"required"`
	OneOf      []alpha6DiscoverySchema          `json:"oneOf"`
}

//nolint:goconst // Keep explicit schema names and field expectations independent from production metadata.
func TestAlpha6DiscoveryContract(t *testing.T) {
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	require.NoError(t, err)
	var spec struct {
		Methods []struct {
			Name   string `json:"name"`
			Params []struct {
				Name     string                `json:"name"`
				Required bool                  `json:"required"`
				Schema   alpha6DiscoverySchema `json:"schema"`
			} `json:"params"`
			Errors []struct {
				Ref string `json:"$ref"`
			} `json:"errors"`
			Result struct {
				Schema alpha6DiscoverySchema `json:"schema"`
			} `json:"result"`
		} `json:"methods"`
		Components struct {
			Schemas map[string]alpha6DiscoverySchema `json:"schemas"`
			Errors  map[string]struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(data, &spec))
	for _, methodName := range []string{observerListStandingChangesMethod, observerGetStandingChangeMethod} {
		found := false
		for _, method := range spec.Methods {
			if method.Name != methodName {
				continue
			}
			found = true
			want := []string{observerApplicationKey, observerEpochIndexKey, observerTournamentAddressKey, obsLimit, obsOffset, obsDescKey}
			wantRefs := []string{"NameOrAddress", "UnsignedInteger", "EthereumAddress", "", "", ""}
			errors := []string{"InvalidParams", "ApplicationNotFound", "InternalError", "TimeoutError", "ResponseSizeLimitExceeded"}
			result := "StandingChangedListResult"
			required := 3
			if methodName == observerGetStandingChangeMethod {
				want = []string{observerApplicationKey, observerEpochIndexKey, observerTournamentAddressKey,
					observerTxHashKey, observerLogIndexKey}
				wantRefs = []string{"NameOrAddress", "UnsignedInteger", "EthereumAddress", "Hash", "UnsignedInteger"}
				required = 5
				result = "StandingChangedGetResult"
				errors = append(errors, "StandingChangedNotFound")
			}
			require.Len(t, method.Params, len(want))
			for i, param := range method.Params {
				require.Equal(t, want[i], param.Name)
				require.Equal(t, i < required, param.Required)
				if wantRefs[i] != "" {
					require.Equal(t, "#/components/schemas/"+wantRefs[i], param.Schema.Ref)
				}
			}
			refs := make([]string, 0, len(method.Errors))
			for _, e := range method.Errors {
				refs = append(refs, strings.TrimPrefix(e.Ref, "#/components/errors/"))
			}
			require.ElementsMatch(t, errors, refs)
			require.Equal(t, "#/components/schemas/"+result, method.Result.Schema.Ref)
		}
		require.True(t, found, methodName)
	}
	require.Equal(t, -31001, spec.Components.Errors["StandingChangedNotFound"].Code)
	require.Equal(t, "Standing change not found", spec.Components.Errors["StandingChangedNotFound"].Message)
	for name, fields := range map[string]map[string]string{
		obsTournament:   {"bond_value": "Uint256"},
		"Match":         {"responder_deadline": "UnsignedInteger"},
		"MatchAdvanced": {"current_height": "UnsignedInteger", "responder_deadline": "UnsignedInteger"},
		"LeafMatchSeal": {"agree_state": "Hash", "divergence_position": "Uint256",
			"final_state_one": "Hash", "final_state_two": "Hash", "deadline_one": "UnsignedInteger", "deadline_two": "UnsignedInteger"},
		"StandingChanged": {observerEpochIndexKey: "UnsignedInteger", observerTournamentAddressKey: "EthereumAddress",
			"match_count": "Uint256", "result_at": "UnsignedInteger", "winner_expires_at": "UnsignedInteger",
			"block_number": "UnsignedInteger", observerTxHashKey: "Hash", observerLogIndexKey: "UnsignedInteger"},
	} {
		schema := spec.Components.Schemas[name]
		for field, ref := range fields {
			require.Contains(t, schema.Required, field)
			require.Equal(t, "#/components/schemas/"+ref, schema.Properties[field].Ref, name+"."+field)
		}
	}
	creation := spec.Components.Schemas[obsTournament]
	require.Contains(t, creation.Required, "creation_event")
	choices := creation.Properties["creation_event"].OneOf
	require.Len(t, choices, 2)
	require.Equal(t, "#/components/schemas/TournamentCreationEvent", choices[0].Ref)
	require.Equal(t, obsNullJSON, choices[1].Type)
	for name, fields := range map[string][]string{
		obsTournament:     {"contested_final_state_one", "contested_final_state_two"},
		"StandingChanged": {"dangling", "parent_commitment"},
	} {
		for _, field := range fields {
			schema := spec.Components.Schemas[name]
			require.Contains(t, schema.Required, field)
			choices := schema.Properties[field].OneOf
			require.Len(t, choices, 2)
			require.Equal(t, "#/components/schemas/Hash", choices[0].Ref)
			require.Equal(t, obsNullJSON, choices[1].Type)
		}
	}
	standing := spec.Components.Schemas["StandingChanged"]
	require.ElementsMatch(t, []string{observerEpochIndexKey, observerTournamentAddressKey, "match_count", "dangling", "parent_commitment",
		"result_at", "winner_expires_at", "block_number", observerTxHashKey, observerLogIndexKey, "created_at", "updated_at"},
		standing.Required)
	require.Len(t, standing.Properties, 12)
	for _, field := range []string{"created_at", "updated_at"} {
		require.Equal(t, "string", standing.Properties[field].Type)
		require.Equal(t, "date-time", standing.Properties[field].Format)
	}
}
