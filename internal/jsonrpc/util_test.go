// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
)

// histogram to gather statistics for each method
type hist map[string]int

func (h hist) inc(k string) {
	x, ok := h[k]
	if ok {
		h[k] = x + 1
	} else {
		h[k] = 1
	}
}

// auxiliary type to encode/decode hex strings in json
type hex64 uint64

func (x *hex64) MarshalJSON() ([]byte, error) {
	return json.Marshal(hexutil.EncodeUint64(uint64(*x)))
}

func (x *hex64) UnmarshalJSON(in []byte) error {
	var hexString string
	err := json.Unmarshal(in, &hexString)
	if err != nil {
		return err
	}
	hexValue, err := model.ParseHexUint64(hexString)
	if err != nil {
		return err
	}
	*x = hex64(hexValue)
	return nil
}

// RPC response type
type testRPCResponse[T any] struct {
	JSONRPC string `json:"jsonrpc"`
	Result  struct {
		Data T `json:"data"`
	} `json:"result"`
	Error *RPCError `json:"error,omitempty"`
	ID    any       `json:"id"`
}

func newTestService(t *testing.T, name string) *Service {
	ctx := context.Background()

	dbTestEndpoint, err := db.GetTestDatabaseEndpoint()
	assert.Nil(t, err)

	err = db.SetupTestPostgres(dbTestEndpoint)
	assert.Nil(t, err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dbTestEndpoint)
	assert.Nil(t, err)

	logLevel, err := config.GetLogLevel()
	assert.Nil(t, err)

	ci := CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:     name,
			LogLevel: logLevel,
			LogColor: true,
		},
		Repository: repo,
	}
	s, err := Create(ctx, &ci)
	assert.Nil(t, err, "on new test service")

	return s
}

func nameToNumber(in string) uint64 {
	xs := hexutil.MustDecode(in)
	return big.NewInt(0).SetBytes(xs).Uint64()
}

func numberToName(x uint64) string {
	return fmt.Sprintf("0x%016x", x)
}

// create an application with mostly stub values.
func (s *Service) newTestApplication(ctx context.Context, t *testing.T, test, i uint64) int64 {
	hex := numberToName(i)
	id, err := s.repository.CreateApplication(ctx, &model.Application{
		Name:                hex,
		IApplicationAddress: common.HexToAddress(hex),
		DataAvailability:    []byte{0x00, 0x00, 0x00, 0x00},
		State:               model.ApplicationState_Enabled,
		ConsensusType:       model.Consensus_Authority,
	}, false)
	assert.Nil(t, err, "on test case: %v, when creating application: %v", test, i)
	return id
}

func (s *Service) doRequest(t *testing.T, i uint64, reqData []byte) []byte {
	w := httptest.NewRecorder()
	s.handleRPC(
		w,
		httptest.NewRequest(
			http.MethodGet,
			"/rpc",
			bytes.NewReader(reqData),
		),
	)

	body := new(strings.Builder)
	_, err := io.Copy(body, w.Result().Body)
	assert.Nil(t, err, "on test case: %v", i)

	return []byte(body.String())
}

// input from ./cartesi-rollups-cli send echo-dapp -y ""
func emptyInput() []byte {
	raw, _ := hexutil.Decode("0x415bf363000000000000000000000000000000000000000000000000000000000000343a0000000000000000000000002e662c8a1a6c8008482a41ef6d3b333497e7f956000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb92266000000000000000000000000000000000000000000000000000000000000000c0000000000000000000000000000000000000000000000000000000068c97fb45a1ab2f3478ee32e84c0a464f70d9da8d470868984ba5f00d9da757bbcee2098000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000000") //nolint: lll
	return raw
}

// output from ./cartesi-rollups-cli send echo-dapp -y ""
func emptyVoucher() []byte {
	raw, _ := hexutil.Decode("0x237a816f000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb9226600000000000000000000000000000000000000000000000000000000deadbeef00000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000000000") //nolint: lll
	return raw
}

type listTournamentsResult struct {
	EpochIndex              hex64           `json:"epoch_index"`
	Address                 common.Address  `json:"address"`
	ParentTournamentAddress *common.Address `json:"parent_tournament_address"`
	ParentMatchIDHash       *common.Hash    `json:"parent_match_id_hash"`
	MaxLevel                hex64           `json:"max_level"`
	Level                   hex64           `json:"level"`
	Log2Step                hex64           `json:"log2step"`
	Height                  hex64           `json:"height"`
	WinnerCommitment        *common.Hash    `json:"winner_commitment"`
	FinalStateHash          *common.Hash    `json:"final_state_hash"`
	FinishedAtBlock         hex64           `json:"finished_at_block"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

type getCommitmentResult struct {
	ApplicationID     hex64          `sql:"primary_key" json:"-"`
	EpochIndex        hex64          `sql:"primary_key" json:"epoch_index"`
	TournamentAddress common.Address `sql:"primary_key" json:"tournament_address"`
	Commitment        common.Hash    `sql:"primary_key" json:"commitment"`
	FinalStateHash    common.Hash    `json:"final_state_hash"`
	SubmitterAddress  common.Address `json:"submitter_address"`
	BlockNumber       hex64          `json:"block_number"`
	TxHash            common.Hash    `json:"tx_hash"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type getMatchResult struct {
	EpochIndex          hex64                     `json:"epoch_index"`
	BlockNumber         hex64                     `json:"block_number"`
	DeletionBlockNumber hex64                     `json:"deletion_block_number"`
	TournamentAddress   common.Address            `json:"tournament_address"`
	IDHash              common.Hash               `json:"id_hash"`
	CommitmentOne       common.Hash               `json:"commitment_one"`
	CommitmentTwo       common.Hash               `json:"commitment_two"`
	LeftOfTwo           common.Hash               `json:"left_of_two"`
	TxHash              common.Hash               `json:"tx_hash"`
	Winner              model.WinnerCommitment    `json:"winner_commitment"`
	DeletionReason      model.MatchDeletionReason `json:"deletion_reason"`
	DeletionTxHash      common.Hash               `json:"deletion_tx_hash"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

type getMatchAdvancedResult struct {
	ApplicationID     hex64          `json:"-"`
	EpochIndex        hex64          `json:"epoch_index"`
	TournamentAddress common.Address `json:"tournament_address"`
	IDHash            common.Hash    `json:"id_hash"`
	OtherParent       common.Hash    `json:"other_parent"`
	LeftNode          common.Hash    `json:"left_node"`
	BlockNumber       hex64          `json:"block_number"`
	TxHash            common.Hash    `json:"tx_hash"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type listMatchesResult struct {
	EpochIndex          hex64                     `json:"epoch_index"`
	BlockNumber         hex64                     `json:"block_number"`
	DeletionBlockNumber hex64                     `json:"deletion_block_number"`
	TournamentAddress   common.Address            `json:"tournament_address"`
	IDHash              common.Hash               `json:"id_hash"`
	CommitmentOne       common.Hash               `json:"commitment_one"`
	CommitmentTwo       common.Hash               `json:"commitment_two"`
	LeftOfTwo           common.Hash               `json:"left_of_two"`
	TxHash              common.Hash               `json:"tx_hash"`
	Winner              model.WinnerCommitment    `json:"winner_commitment"`
	DeletionReason      model.MatchDeletionReason `json:"deletion_reason"`
	DeletionTxHash      common.Hash               `json:"deletion_tx_hash"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

type listMatchAdvancedResult struct {
	ApplicationID     hex64          `json:"-"`
	EpochIndex        hex64          `json:"epoch_index"`
	TournamentAddress common.Address `json:"tournament_address"`
	IDHash            common.Hash    `json:"id_hash"`
	OtherParent       common.Hash    `json:"other_parent"`
	LeftNode          common.Hash    `json:"left_node"`
	BlockNumber       hex64          `json:"block_number"`
	TxHash            common.Hash    `json:"tx_hash"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type listCommitmentResult struct {
	ApplicationID     hex64          `json:"-"`
	EpochIndex        hex64          `json:"epoch_index"`
	TournamentAddress common.Address `json:"tournament_address"`
	Commitment        common.Hash    `json:"commitment"`
	FinalStateHash    common.Hash    `json:"final_state_hash"`
	SubmitterAddress  common.Address `json:"submitter_address"`
	BlockNumber       hex64          `json:"block_number"`
	TxHash            common.Hash    `json:"tx_hash"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}
