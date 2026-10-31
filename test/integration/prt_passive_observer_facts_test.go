// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func (f *passiveDisputeFixture) listStandingChanges(layer *passiveDisputeLayer) []*model.StandingChanged {
	f.t.Helper()
	var response api.ListResponse[*model.StandingChanged]
	f.r.NoError(f.rpc.Call(f.ctx, "cartesi_listStandingChanges", api.ListStandingChangesParams{
		Application: f.appName, EpochIndex: "0x0", TournamentAddress: layer.address.Hex(), Limit: 100,
	}, &response))
	f.r.Equal(uint64(len(response.Data)), response.Pagination.TotalCount)
	for _, row := range response.Data {
		var single api.SingleResponse[*model.StandingChanged]
		f.r.NoError(f.rpc.Call(f.ctx, "cartesi_getStandingChange", api.GetStandingChangeParams{
			Application: f.appName, EpochIndex: "0x0", TournamentAddress: layer.address.Hex(),
			TxHash: row.TxHash.Hex(), LogIndex: hexutil.EncodeUint64(row.LogIndex),
		}, &single))
		f.r.Equal(row, single.Data)
	}
	return response.Data
}

func (f *passiveDisputeFixture) assertStandingChangedEvent(
	layer *passiveDisputeLayer, raw *types.Log, row *model.StandingChanged,
) {
	f.t.Helper()
	f.r.NotNil(row)
	event, err := layer.contract.ParseStandingChanged(*raw)
	f.r.NoError(err)
	f.r.Equal(layer.address, row.TournamentAddress)
	f.r.Zero(row.EpochIndex)
	f.r.Zero(event.MatchCount.Cmp(row.MatchCount.ToBig()))
	f.assertStandingReference(event.Dangling, row.Dangling)
	f.assertStandingReference(event.ParentCommitment, row.ParentCommitment)
	f.r.Equal(event.ResultAt, row.ResultAt)
	f.r.Equal(event.WinnerExpiresAt, row.WinnerExpiresAt)
	f.assertLogIdentity(raw, row.BlockNumber, row.TxHash, row.LogIndex)
}

func (f *passiveDisputeFixture) assertStandingReference(event [32]byte, row *common.Hash) {
	f.t.Helper()
	if event == ([32]byte{}) {
		f.r.Nil(row)
		return
	}
	f.r.NotNil(row)
	f.r.Equal(common.Hash(event), *row)
}

func (f *passiveDisputeFixture) assertStoredFacts(
	layer *passiveDisputeLayer, match *model.Match, advances []*model.MatchAdvanced, standing []*model.StandingChanged,
) {
	f.t.Helper()
	dsn, err := config.GetDatabaseConnection()
	f.r.NoError(err)
	repo, err := factory.NewRepositoryFromConnectionString(f.ctx, dsn.Raw())
	f.r.NoError(err)
	defer repo.Close()
	var response api.SingleResponse[*model.Tournament]
	f.r.NoError(f.rpc.Call(f.ctx, "cartesi_getTournament", api.GetTournamentParams{
		Application: f.appName, Address: layer.address.Hex(),
	}, &response))
	f.r.NotNil(response.Data)
	tournament := response.Data
	storedTournament, err := repo.GetTournament(f.ctx, f.appName, layer.address.Hex())
	f.r.NoError(err)
	f.r.NotNil(storedTournament)
	f.r.Equal(storedTournament.BondValue, tournament.BondValue)
	f.r.Equal(storedTournament.ContestedFinalStateOne, tournament.ContestedFinalStateOne)
	f.r.Equal(storedTournament.ContestedFinalStateTwo, tournament.ContestedFinalStateTwo)
	f.r.Equal(storedTournament.CreationEvent, tournament.CreationEvent)
	storedMatch, err := repo.GetMatch(f.ctx, f.appName, 0, layer.address.Hex(), layer.matchHash.Hex())
	f.r.NoError(err)
	f.r.NotNil(storedMatch)
	f.r.Equal(storedMatch.ResponderDeadline, match.ResponderDeadline)
	f.r.Equal(storedMatch.LeafSeal, match.LeafSeal)
	for _, row := range advances {
		stored, readErr := repo.GetMatchAdvanced(f.ctx, f.appName, 0, layer.address.Hex(),
			layer.matchHash.Hex(), row.TxHash, row.LogIndex)
		f.r.NoError(readErr)
		f.r.NotNil(stored)
		f.r.Equal(stored.CurrentHeight, row.CurrentHeight)
		f.r.Equal(stored.ResponderDeadline, row.ResponderDeadline)
	}
	for _, row := range standing {
		stored, readErr := repo.GetStandingChange(f.ctx, f.appName, 0, layer.address.Hex(), row.TxHash, row.LogIndex)
		f.r.NoError(readErr)
		f.r.NotNil(stored)
		f.r.Equal(stored.EpochIndex, row.EpochIndex)
		f.r.Equal(stored.TournamentAddress, row.TournamentAddress)
		f.r.Equal(stored.MatchCount, row.MatchCount)
		f.r.Equal(stored.Dangling, row.Dangling)
		f.r.Equal(stored.ParentCommitment, row.ParentCommitment)
		f.r.Equal(stored.ResultAt, row.ResultAt)
		f.r.Equal(stored.WinnerExpiresAt, row.WinnerExpiresAt)
		f.r.Equal(stored.BlockNumber, row.BlockNumber)
		f.r.Equal(stored.TxHash, row.TxHash)
		f.r.Equal(stored.LogIndex, row.LogIndex)
		f.r.True(stored.CreatedAt.Equal(row.CreatedAt))
		f.r.True(stored.UpdatedAt.Equal(row.UpdatedAt))
	}
}
