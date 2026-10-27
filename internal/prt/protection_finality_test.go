// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProtectionReportsMidIntervalFinalityInSameObserverPass(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.s.defaultBlock = model.DefaultBlock_Finalized
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	second := *f.app
	second.ID++
	second.Name = "second application"
	second.IApplicationAddress = common.HexToAddress("0x991")
	second.IConsensusAddress = common.HexToAddress("0x992")
	secondConsensus := newCompatibleDaveConsensusAdapterMock(second.IConsensusAddress)
	f.factory.On("CreateDaveConsensusAdapter", second.IConsensusAddress).Return(secondConsensus, nil).Once()

	header := &types.Header{Number: big.NewInt(100)}
	conflict := types.CopyHeader(header)
	conflict.Extra = []byte("changed after interval reads")
	client := &ethClientMock{}
	f.client, f.s.client = client, client
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(header, nil).Once()
	checks := 0
	client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(func(context.Context, *big.Int) *types.Header {
		checks++
		if checks < 3 {
			return header
		}
		return conflict
	}, nil)
	facts := txsender.BlockRef{Number: 100, Hash: header.Hash()}
	filter := repository.ApplicationFilter{ConsensusType: new(model.Consensus_PRT), Enabled: new(true)}
	f.repo.On("GetApplicationIDBoundary", mock.Anything, filter).Return(second.ID, nil).Once()
	f.repo.On("ListApplicationsByID", mock.Anything, filter, int64(0), second.ID, deliveryApplicationPageSize).
		Return([]*model.Application{f.app, &second}, nil).Maybe()

	firstEpoch, secondEpoch := checkpointEpoch(0, "0x111"), checkpointEpoch(1, "0x222")
	secondEpoch.ApplicationID = second.ID
	require.True(t, protectionLocallyReady(firstEpoch))
	require.True(t, protectionLocallyReady(secondEpoch))
	firstRoot := &model.Tournament{ApplicationID: f.app.ID, EpochIndex: firstEpoch.Index,
		Address: *firstEpoch.TournamentAddress, StartInstant: firstEpoch.LastBlock, Allowance: 300,
		Snapshot: model.TournamentSnapshot{AsOfBlock: 100}}
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), firstRoot.Address.Hex()).Return(firstRoot, nil).Maybe()
	secondFixture := &observerCheckpointFixture{t: t, s: f.s, repo: f.repo, factory: f.factory,
		consensus: secondConsensus, client: client, app: &second}
	secondRoot := secondFixture.tournament(secondEpoch, *secondEpoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
	f.repo.On("GetTournament", mock.Anything, second.IApplicationAddress.Hex(), secondRoot.Address.Hex()).Return(secondRoot, nil).Maybe()

	for i, item := range []struct {
		app       *model.Application
		epoch     *model.Epoch
		root      *model.Tournament
		consensus *daveConsensusAdapterMock
	}{{f.app, firstEpoch, firstRoot, f.consensus}, {&second, secondEpoch, secondRoot, secondConsensus}} {
		f.repo.On("ListEpochs", mock.Anything, item.app.Name,
			repository.EpochFilter{HasTournament: new(true)}, repository.Pagination{}, false).
			Return([]*model.Epoch{item.epoch}, uint64(1), nil).Once()
		f.repo.On("ListTournaments", mock.Anything, item.app.Name, repository.TournamentFilter{}, repository.Pagination{}, false).
			Return([]*model.Tournament{item.root}, uint64(1), nil).Once()
		f.repo.On("GetEpoch", mock.Anything, item.app.IApplicationAddress.Hex(), item.epoch.Index).Return(item.epoch, nil).Maybe()
		view := resultTestSnapshot(item.epoch, false)
		if i == 0 {
			view.stage.IsFinished = false
			view.stage.WinnerCommitment, view.stage.WinnerPostEpochMachineStateHash = common.Hash{}, common.Hash{}
		}
		item.consensus.On("GetCurrentSealedEpoch", protectionOpts(facts)).Return(view.sealed, nil)
		item.consensus.On("CanStageTournamentResult", protectionOpts(facts)).Return(view.stage, nil).Once()
		item.consensus.On("CanAcceptStagedTournamentResult", protectionOpts(facts)).Return(view.accept, nil).Once()
		if i == 1 {
			item.consensus.On("TournamentLevelCount", protectionOpts(facts)).Return(uint64(1), nil).Once()
		}
		adapter := &tournamentAdapterMock{}
		call := f.factory.On("CreateTournamentAdapter", *item.epoch.TournamentAddress).Return(adapter, nil).Once()
		f.factory.ExpectedCalls = append([]*mock.Call{call}, f.factory.ExpectedCalls[:len(f.factory.ExpectedCalls)-1]...)
		adapter.On("Descriptor", protectionOpts(facts)).Return(TournamentDescriptor{
			StartInstant: item.epoch.LastBlock, Allowance: item.root.Allowance}, nil).Once()
		t.Cleanup(func() { adapter.AssertExpectations(t) })
		// Keep diagnostic reads optional so the alert assertions detect an omitted scan.
		f.repo.On("ListEpochs", mock.Anything, item.app.IApplicationAddress.Hex(), repository.EpochFilter{
			HasTournament: new(true), Status: model.NonTerminalEpochStatuses()}, repository.Pagination{}, false).
			Return([]*model.Epoch{item.epoch}, uint64(1), nil).Maybe()
	}

	require.True(t, f.s.Ready())
	err := f.s.observe(t.Context())
	require.ErrorIs(t, err, txsender.ErrFinality)
	require.True(t, f.s.publicationStopped())
	require.False(t, f.s.Ready())
	require.GreaterOrEqual(t, checks, 3, "contradiction must occur after interval reads")
	require.Empty(t, f.repo.committedTournaments)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	require.Equal(t, model.ApplicationStatus_OK, second.Status)
	require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
	require.Equal(t, uint64(50), second.LastTournamentCheckBlock)
	require.NotContains(t, output.String(), "PRT sealed epoch is not locally ready")
	require.Equal(t, 2, strings.Count(output.String(), "PRT protection stopped"), "this observer call must report both ready apps")
	require.Contains(t, output.String(), "application=prt-app")
	require.Contains(t, output.String(), "root_tournament="+firstRoot.Address.Hex())
	require.Contains(t, output.String(), "root_tournament="+secondRoot.Address.Hex())
	require.Contains(t, output.String(), "join_close_block=310")
	require.Contains(t, output.String(), "join_close_block=10")
	secondConsensus.AssertExpectations(t)
}
