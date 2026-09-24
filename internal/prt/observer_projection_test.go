// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"encoding/json"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	unknownEnumValue     = "UNKNOWN"
	projectionCreateCase = "create"
	projectionUpdateCase = "update"
	unknownStandingCase  = "unknown standing"
)

func TestTournamentProjectionPreservesWinnerPresence(t *testing.T) {
	for _, test := range []struct {
		name   string
		state  model.TournamentStandingState
		level  TournamentLevel
		winner bool
	}{
		{name: "failed root", state: model.TournamentStandingRootFailed},
		{name: "root result exists", state: model.TournamentStandingRootWinner, winner: true},
		{name: "inner without winner", state: model.TournamentStandingInnerEliminableNoWinner, level: 1},
		{name: "inner result exists", state: model.TournamentStandingInnerWinner, level: 1, winner: true},
		{name: "expired inner winner", state: model.TournamentStandingInnerEliminableWinnerExpired, level: 1},
	} {
		for _, update := range []bool{false, true} {
			operation := projectionCreateCase
			if update {
				operation = projectionUpdateCase
			}
			t.Run(test.name+"/"+operation, func(t *testing.T) {
				app := prtRevertTestApp()
				epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
				service, repo := newPRTServiceMock()
				descriptor := TournamentDescriptor{BaseCycle: big.NewInt(0), Level: uint64(test.level), Height: 7,
					Kind: model.TournamentKindNonLeaf}
				hasCandidate := test.winner || test.state == model.TournamentStandingInnerEliminableWinnerExpired
				standing := TournamentStanding{State: test.state, FinishedAt: 9, HasCandidate: hasCandidate}
				if hasCandidate {
					standing.Candidate = *epoch.Commitment
				}
				adapter := &tournamentAdapterMock{}
				opts := mock.MatchedBy(resultCallOptsAtBlock(10))
				adapter.On("Descriptor", opts).Return(descriptor, nil).Once()
				adapter.On("Standing", opts).Return(standing, nil).Once()
				expectTournamentAuxiliaryReads(adapter, opts, test.level, test.state)
				var stored *model.Tournament
				if update {
					tournament := &model.Tournament{Address: *epoch.TournamentAddress, MaxLevel: 3,
						Level: uint64(test.level), Height: descriptor.Height,
						Kind: model.TournamentKindNonLeaf, Snapshot: model.TournamentSnapshot{
							WinnerCommitment: new(common.HexToHash("0x111")), FinalStateHash: new(common.HexToHash("0x222"))}}
					require.NoError(t, service.refreshTournament(t.Context(), app, epoch, test.level, adapter, tournament, 10))
					stored = tournament
				} else {
					tournament, err := service.readTournament(t.Context(), app, epoch, test.level,
						nil, nil, *epoch.TournamentAddress, adapter, 3, 10)
					require.NoError(t, err)
					require.NotNil(t, tournament)
					stored = tournament
				}
				require.NotNil(t, stored)
				require.Equal(t, uint64(9), stored.Snapshot.FinishedAtBlock)
				data, err := json.Marshal(stored)
				require.NoError(t, err)
				var projection map[string]any
				require.NoError(t, json.Unmarshal(data, &projection))
				projection = projection["snapshot"].(map[string]any)
				require.Equal(t, uint64(10), stored.Snapshot.AsOfBlock)
				require.Equal(t, test.state, stored.Snapshot.Standing)
				if hasCandidate {
					require.Equal(t, epoch.Commitment, stored.Snapshot.Candidate)
				} else {
					require.Nil(t, stored.Snapshot.Candidate)
				}
				if test.winner {
					require.Equal(t, epoch.Commitment, stored.Snapshot.WinnerCommitment)
					require.NotNil(t, stored.Snapshot.FinalStateHash, "a zero final-state hash is a value, not absence")
					require.Zero(t, *stored.Snapshot.FinalStateHash)
					require.Equal(t, epoch.Commitment.Hex(), projection["winner_commitment"])
					require.Equal(t, (common.Hash{}).Hex(), projection["final_state_hash"])
				} else {
					require.Nil(t, stored.Snapshot.WinnerCommitment)
					require.Nil(t, stored.Snapshot.FinalStateHash)
					require.Nil(t, projection["winner_commitment"])
					require.Nil(t, projection["final_state_hash"])
				}
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				require.Empty(t, repo.Calls, "projection reads must not write tournament state")
				repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				repo.AssertExpectations(t)
				adapter.AssertExpectations(t)
			})
		}
	}
}

func TestTournamentProjectionClearsNonTerminalResult(t *testing.T) {
	for _, test := range []struct {
		name  string
		state model.TournamentStandingState
	}{
		{name: "active matches", state: model.TournamentStandingMatchesActive},
		{name: "awaiting closure", state: model.TournamentStandingAwaitingClosure},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
			projection := &model.Tournament{Address: *epoch.TournamentAddress, MaxLevel: 3, Log2Step: 20, Height: 7,
				Kind: model.TournamentKindNonLeaf, Snapshot: model.TournamentSnapshot{
					WinnerCommitment: new(common.HexToHash("0x111")), FinalStateHash: new(common.HexToHash("0x222"))}}
			adapter := &tournamentAdapterMock{}
			opts := mock.MatchedBy(resultCallOptsAtBlock(10))
			adapter.On("Descriptor", opts).Return(TournamentDescriptor{
				BaseCycle: big.NewInt(0), Kind: model.TournamentKindNonLeaf, Log2Stride: projection.Log2Step, Height: projection.Height,
			}, nil).Once()
			adapter.On("Standing", opts).Return(TournamentStanding{State: test.state}, nil).Once()
			expectTournamentAuxiliaryReads(adapter, opts, RootLevel, test.state)

			require.NoError(t, s.refreshTournament(t.Context(), app, epoch, RootLevel, adapter, projection, 10))
			require.Nil(t, projection.Snapshot.WinnerCommitment)
			require.Nil(t, projection.Snapshot.FinalStateHash)
			require.Empty(t, repo.Calls)
			adapter.AssertExpectations(t)
		})
	}
}

func TestTournamentProjectionRejectsChangedStoredGeometry(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*model.Tournament)
	}{
		{name: "level", mutate: func(p *model.Tournament) { p.Level++ }},
		{name: "stride", mutate: func(p *model.Tournament) { p.Log2Step++ }},
		{name: "height", mutate: func(p *model.Tournament) { p.Height++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
			projection := &model.Tournament{Address: *epoch.TournamentAddress, MaxLevel: 3, Log2Step: 20, Height: 7}
			adapter := &tournamentAdapterMock{}
			adapter.On("Descriptor", mock.MatchedBy(resultCallOptsAtBlock(10))).Return(TournamentDescriptor{
				BaseCycle: big.NewInt(0), Kind: model.TournamentKindNonLeaf, Log2Stride: projection.Log2Step, Height: projection.Height,
			}, nil).Once()
			test.mutate(projection)
			before := *projection

			err := s.refreshTournament(t.Context(), app, epoch, RootLevel, adapter, projection, 10)
			require.ErrorContains(t, err, "descriptor does not match stored geometry")
			require.Equal(t, before, *projection)
			require.Empty(t, repo.Calls)
			adapter.AssertNotCalled(t, "Standing", mock.Anything)
			adapter.AssertExpectations(t)
		})
	}
}

func TestTournamentProjectionLeavesReadErrorsUnchanged(t *testing.T) {
	for _, test := range []struct {
		name     string
		standing TournamentStanding
		readErr  error
	}{
		{name: "standing RPC error", readErr: errors.New("standing RPC unavailable")},
		{name: unknownStandingCase, standing: TournamentStanding{State: unknownEnumValue}},
		{name: "future result", standing: TournamentStanding{State: model.TournamentStandingRootWinner, FinishedAt: 11}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
			projection := &model.Tournament{Address: *epoch.TournamentAddress, MaxLevel: 3,
				Log2Step: 20, Height: 7, Kind: model.TournamentKindNonLeaf,
				Snapshot: model.TournamentSnapshot{
					WinnerCommitment: new(common.HexToHash("0x111")), FinalStateHash: new(common.HexToHash("0x222"))}}
			before := *projection
			adapter := &tournamentAdapterMock{}
			opts := mock.MatchedBy(resultCallOptsAtBlock(10))
			adapter.On("Descriptor", opts).Return(TournamentDescriptor{
				BaseCycle: big.NewInt(0), Kind: model.TournamentKindNonLeaf,
				Log2Stride: projection.Log2Step, Height: projection.Height,
			}, nil).Once()
			adapter.On("Standing", opts).Return(test.standing, test.readErr).Once()

			err := s.refreshTournament(t.Context(), app, epoch, RootLevel, adapter, projection, 10)
			require.Error(t, err)
			if test.readErr != nil {
				require.ErrorIs(t, err, test.readErr)
			}
			require.Equal(t, before, *projection)
			require.Same(t, before.Snapshot.WinnerCommitment, projection.Snapshot.WinnerCommitment)
			require.Same(t, before.Snapshot.FinalStateHash, projection.Snapshot.FinalStateHash)
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			require.Empty(t, repo.Calls)
			adapter.AssertExpectations(t)
		})
	}
}

func TestJoinChecksSupportedRootGeometry(t *testing.T) {
	for _, height := range []uint64{model.Log2EpochComputationHashLeafCount, 7} {
		t.Run(new(big.Int).SetUint64(height).String(), func(t *testing.T) {
			app := prtRevertTestApp()
			epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
			epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
			service, repo := newPRTServiceMock()
			service.submissionTimeout = time.Second
			service.txOptsFactory = ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: common.HexToAddress("0x600")})
			factory := &adapterFactoryMock{}
			service.adapterFactory = factory
			adapter := &tournamentAdapterMock{}
			factory.On("CreateTournamentAdapter", *epoch.TournamentAddress).Return(adapter, nil).Once()
			repo.On("GetCommitment", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index,
				epoch.TournamentAddress.Hex(), epoch.Commitment.Hex()).Return(nil, nil).Once()
			opts := mock.MatchedBy(func(opts *bind.CallOpts) bool { return resultCallOptsAtBlock(20)(opts) })
			adapter.On("CommitmentStanding", opts, [32]byte(*epoch.Commitment)).Return(CommitmentStanding{}, nil).Once()
			adapter.On("Descriptor", opts).Return(TournamentDescriptor{Height: height}, nil).Once()
			if height == model.Log2EpochComputationHashLeafCount {
				adapter.On("BondValue", opts).Return(big.NewInt(0), nil).Once()
				adapter.On("JoinTournament", mock.Anything, [32]byte(*epoch.MachineHash),
					mock.Anything, mock.Anything, mock.Anything).Return(types.NewTx(&types.LegacyTx{Nonce: 1}), nil).Once()
				joined, err := service.reactToTournament(t.Context(), app, epoch, 20)
				require.NoError(t, err)
				require.False(t, joined)
				require.Equal(t, tournamentActionJoin, service.pendingTransactions[app.ID].Action)
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
			} else {
				repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
					mock.MatchedBy(reasonContains("commitment height 48", "factory and node versions"))).Return(nil).Once()
				joined, err := service.reactToTournament(t.Context(), app, epoch, 20)
				require.NoError(t, err)
				require.False(t, joined)
				require.Equal(t, model.ApplicationStatus_Failed, app.Status)
				adapter.AssertNotCalled(t, "JoinTournament", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}
			adapter.AssertExpectations(t)
			factory.AssertExpectations(t)
			repo.AssertExpectations(t)
		})
	}
}

func TestRootWinnerMismatchDoesNotChangeLocalHealthDuringObservation(t *testing.T) {
	for _, update := range []bool{false, true} {
		operation := projectionCreateCase
		if update {
			operation = projectionUpdateCase
		}
		t.Run(operation, func(t *testing.T) {
			app := prtRevertTestApp()
			epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
			service, repo := newPRTServiceMock()
			adapter := &tournamentAdapterMock{}
			adapter.On("Descriptor", mock.Anything).Return(TournamentDescriptor{
				BaseCycle: big.NewInt(0), Kind: model.TournamentKindNonLeaf,
			}, nil).Once()
			adapter.On("Standing", mock.Anything).Return(TournamentStanding{
				State: model.TournamentStandingRootWinner, FinishedAt: 9, HasCandidate: true, Candidate: common.HexToHash("0xbad"),
			}, nil).Once()
			expectTournamentAuxiliaryReads(adapter, mock.Anything, RootLevel, model.TournamentStandingRootWinner)
			if update {
				err := service.refreshTournament(t.Context(), app, epoch, RootLevel, adapter,
					&model.Tournament{Address: *epoch.TournamentAddress, MaxLevel: 3, Kind: model.TournamentKindNonLeaf}, 10)
				require.NoError(t, err)
			} else {
				tournament, err := service.readTournament(t.Context(), app, epoch, RootLevel,
					nil, nil, *epoch.TournamentAddress, adapter, 3, 10)
				require.NoError(t, err)
				require.Equal(t, common.HexToHash("0xbad"), *tournament.Snapshot.WinnerCommitment)
			}
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			require.Empty(t, repo.Calls)
			repo.AssertExpectations(t)
			adapter.AssertExpectations(t)
		})
	}
}
