// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"encoding/json"
	"errors"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
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

func TestJoinChecksSupportedRootGeometry(t *testing.T) {
	for _, test := range []struct {
		name                  string
		level, height, stride uint64
		allowed               bool
		observed              string
	}{
		{name: "supported", level: 0, height: 54, stride: 38, allowed: true},
		{name: "level", level: 1, height: 54, stride: 38, observed: "level=1 height=54 stride=38"},
		{name: "height", level: 0, height: 53, stride: 38, observed: "level=0 height=53 stride=38"},
		{name: "stride", level: 0, height: 54, stride: 44, observed: "level=0 height=54 stride=44"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newDeliveryFixture(t)
			view := resultTestSnapshot(f.epoch, false)
			view.stage.IsFinished = false
			view.stage.WinnerCommitment = common.Hash{}
			view.stage.WinnerPostEpochMachineStateHash = common.Hash{}
			f.snapshot(view)
			adapter := &tournamentAdapterMock{}
			f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(adapter, nil).Once()
			opts := mock.MatchedBy(resultCallOptsAtBlock(f.block.Number))
			adapter.On("CommitmentStanding", opts, [32]byte(*f.epoch.Commitment)).Return(CommitmentStanding{}, nil).Once()
			adapter.On("Descriptor", opts).Return(TournamentDescriptor{
				Level: test.level, Height: test.height, Log2Stride: test.stride, StartInstant: 100, Allowance: 20,
			}, nil).Once()
			adapter.On("Standing", opts).Return(TournamentStanding{AcceptsJoins: true}, nil).Maybe()
			adapter.On("BondValue", opts).Return(big.NewInt(0), nil).Maybe()
			call, err := f.d.planAction(t.Context(), f.app, joinTournament, f.block, observerFacts(100))
			if test.allowed {
				require.NoError(t, err)
				require.NotNil(t, call)
				require.Equal(t, joinTournament, call.Key.Kind)
			} else {
				require.ErrorIs(t, err, txsender.ErrIneligible)
				require.ErrorContains(t, err, test.observed)
				require.ErrorContains(t, err, "expected level=0 height=54 stride=38")
				require.Nil(t, call)
				require.Empty(t, f.backend.published)
				adapter.AssertNotCalled(t, "Standing", mock.Anything)
				adapter.AssertNotCalled(t, "BondValue", mock.Anything)
			}
			assertPRTPlanDidNotWrite(t, f)
			adapter.AssertExpectations(t)
			f.repo.AssertExpectations(t)
		})
	}
}
