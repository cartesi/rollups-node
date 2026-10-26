// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prtprotection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

type clockRepository struct {
	epochs         []*model.Epoch
	roots          map[common.Address]*model.Tournament
	readErrors     map[common.Address]error
	discoveryError error
	reads          []common.Address
	filter         repository.EpochFilter
	calls          int
}

func (r *clockRepository) ListEpochs(
	_ context.Context, _ string, f repository.EpochFilter, _ repository.Pagination, _ bool,
) ([]*model.Epoch, uint64, error) {
	r.calls++
	r.filter = f
	return r.epochs, uint64(len(r.epochs)), r.discoveryError
}
func (r *clockRepository) GetTournament(_ context.Context, _ string, address string) (*model.Tournament, error) {
	root := common.HexToAddress(address)
	r.reads = append(r.reads, root)
	return r.roots[root], r.readErrors[root]
}
func clockFixture() (*clockRepository, *model.Application) {
	app := &model.Application{ID: 4, Name: "clock-app",
		IApplicationAddress: common.HexToAddress("0x123"), ConsensusType: model.Consensus_PRT}
	repo := &clockRepository{roots: make(map[common.Address]*model.Tournament), readErrors: make(map[common.Address]error)}
	for index := uint64(0); index < 3; index++ {
		address := common.BigToAddress(new(big.Int).SetUint64(index + 1))
		epoch := &model.Epoch{ApplicationID: app.ID, Index: index,
			LastBlock: 100 + index, TournamentAddress: new(address), Status: model.EpochStatus_Closed}
		repo.epochs = append(repo.epochs, epoch)
		repo.roots[address] = &model.Tournament{ApplicationID: app.ID, EpochIndex: index, Address: address,
			StartInstant: epoch.LastBlock, Allowance: 20 + index}
	}
	return repo, app
}

func TestClosingBlock(t *testing.T) {
	for _, tc := range []struct {
		start, allowance, want uint64
		overflow               bool
	}{
		{100, 20, 120, false}, {0, 0, 0, false}, {math.MaxUint64, 0, math.MaxUint64, false}, {1, math.MaxUint64, 0, true},
	} {
		got, err := ClosingBlock(tc.start, tc.allowance)
		if tc.overflow {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		}
	}
}

func TestStoredClocksRetainEveryRootAndError(t *testing.T) {
	repo, app := clockFixture()
	failure := errors.New("root read unavailable")
	repo.readErrors[*repo.epochs[0].TournamentAddress] = failure
	clocks, err := StoredClocks(t.Context(), repo, app, 0)
	require.NoError(t, err)
	require.Len(t, clocks, 3)
	require.Len(t, repo.reads, 3)
	require.ErrorIs(t, clocks[0].Err, failure)
	require.Nil(t, clocks[0].ClosingBlock)
	require.Equal(t, uint64(122), *clocks[1].ClosingBlock)
	require.Equal(t, uint64(124), *clocks[2].ClosingBlock)
	require.Equal(t, model.NonTerminalEpochStatuses(), repo.filter.Status)
	require.NotNil(t, repo.filter.HasTournament)
	require.True(t, *repo.filter.HasTournament)
}

func TestStoredClocksExcludeValidPrefixAndRetiredRoots(t *testing.T) {
	repo, app := clockFixture()
	repo.epochs[1].ClaimTransactionHash = new(common.HexToHash("0xabc"))
	clocks, err := StoredClocks(t.Context(), repo, app, 1)
	require.NoError(t, err)
	require.Len(t, clocks, 1)
	require.Equal(t, uint64(2), clocks[0].EpochIndex)
	require.Equal(t, []common.Address{*repo.epochs[2].TournamentAddress}, repo.reads)
	repo.epochs[2].Status = model.EpochStatus_ClaimAccepted
	clocks, err = StoredClocks(t.Context(), repo, app, 1)
	require.NoError(t, err)
	require.Empty(t, clocks)
}

func TestStoredClockUnavailablePreservesIdentity(t *testing.T) {
	for _, mutate := range []struct {
		name   string
		change func(*model.Tournament)
	}{
		{"missing", nil},
		{"application", func(root *model.Tournament) { root.ApplicationID++ }},
		{"epoch", func(root *model.Tournament) { root.EpochIndex++ }},
		{"address", func(root *model.Tournament) { root.Address = common.Address{} }},
		{"child", func(root *model.Tournament) { root.Level = 1 }},
		{"start", func(root *model.Tournament) { root.StartInstant++ }},
		{"overflow", func(root *model.Tournament) { root.Allowance = math.MaxUint64 }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			repo, app := clockFixture()
			address := *repo.epochs[0].TournamentAddress
			if mutate.change == nil {
				delete(repo.roots, address)
			} else {
				mutate.change(repo.roots[address])
			}
			clocks, err := StoredClocks(t.Context(), repo, app, 0)
			require.NoError(t, err)
			require.Equal(t, address, clocks[0].Root)
			require.Equal(t, uint64(0), clocks[0].EpochIndex)
			require.Nil(t, clocks[0].ClosingBlock)
			require.Error(t, clocks[0].Err)
			require.NotNil(t, clocks[1].ClosingBlock)
		})
	}
}

func TestProtectionStopLogsClockOrExplicitUnavailable(t *testing.T) {
	repo, app := clockFixture()
	repo.readErrors[*repo.epochs[0].TournamentAddress] = errors.New("RPC unavailable")
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	ReportStop(t.Context(), logger, repo, app, 1, errors.New("committed execution stop"))
	decoder := json.NewDecoder(&output)
	for _, wantEpoch := range []uint64{1, 2} {
		var record map[string]any
		require.NoError(t, decoder.Decode(&record))
		require.Equal(t, "ERROR", record["level"])
		require.Equal(t, "PRT protection stopped", record["msg"])
		require.Equal(t, float64(wantEpoch), record["epoch_index"])
		require.Equal(t, repo.epochs[wantEpoch].TournamentAddress.Hex(), record["root_tournament"])
		require.Equal(t, true, record["clock_available"])
		require.Equal(t, "committed execution stop", record["cause"])
		require.Equal(t, float64(120+wantEpoch*2), record["join_close_block"])
	}
	output.Reset()
	ReportStop(t.Context(), logger, repo, app, 0, errors.New("local failure"))
	var unavailable map[string]any
	require.NoError(t, json.NewDecoder(&output).Decode(&unavailable))
	require.Equal(t, false, unavailable["clock_available"])
	require.Equal(t, float64(0), unavailable["epoch_index"])
	require.NotContains(t, unavailable, "join_close_block")
	require.Equal(t, "RPC unavailable", unavailable["clock_error"])
	output.Reset()
	repo.discoveryError = errors.New("database unavailable")
	ReportStop(t.Context(), logger, repo, app, 0, errors.New("local failure"))
	var discovery map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(output.Bytes()), &discovery))
	require.NotContains(t, discovery, "epoch_index")
	require.NotContains(t, discovery, "root_tournament")
	require.Equal(t, false, discovery["clock_available"])
}

func TestOtherConsensusDoesNotReadProtectionMetadata(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		repo, app := clockFixture()
		app.ConsensusType = consensus
		ReportStop(t.Context(), slog.Default(), repo, app, 0, errors.New("failure"))
		require.Zero(t, repo.calls)
		require.Empty(t, repo.reads)
	}
}
