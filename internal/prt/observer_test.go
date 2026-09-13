// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTournamentDescriptorFromBinding_MapsFieldsAndOwnsBaseCycle(t *testing.T) {
	baseCycle := big.NewInt(123)
	binding := itournament.ITournamentTournamentDescriptor{
		BaseCycle:  baseCycle,
		Log2Stride: 20,
		Height:     48,
		Level:      2,
		Kind:       0,
	}

	descriptor, err := tournamentDescriptorFromBinding(binding)
	require.NoError(t, err)
	require.Equal(t, int64(123), descriptor.BaseCycle.Int64())
	require.Equal(t, binding.Log2Stride, descriptor.Log2Stride)
	require.Equal(t, binding.Height, descriptor.Height)
	require.Equal(t, binding.Level, descriptor.Level)
	require.Equal(t, model.TournamentKindLeaf, descriptor.Kind)

	baseCycle.SetInt64(456)
	require.Equal(t, int64(123), descriptor.BaseCycle.Int64(), "the DTO must own its base-cycle value")
	descriptor.BaseCycle.SetInt64(789)
	require.Equal(t, int64(456), binding.BaseCycle.Int64(), "the binding must not share the DTO base-cycle value")

	_, err = tournamentDescriptorFromBinding(itournament.ITournamentTournamentDescriptor{})
	require.ErrorContains(t, err, "base cycle is nil")
}

func TestTournamentStandingFromBinding_MapsAllFields(t *testing.T) {
	for _, flags := range [][2]bool{{true, false}, {false, true}} {
		binding := itournament.ITournamentTournamentStandingView{
			Standing:     4,
			AcceptsJoins: flags[0], HasCandidate: flags[1],
			Candidate: common.HexToHash("0x21"), FinalState: common.HexToHash("0x22"), FinishedAt: 101,
		}
		standing, err := tournamentStandingFromBinding(binding)
		require.NoError(t, err)
		require.Equal(t, TournamentStanding{
			State: model.TournamentStandingInnerWinner, AcceptsJoins: flags[0], HasCandidate: flags[1],
			Candidate: common.HexToHash("0x21"), FinalState: common.HexToHash("0x22"), FinishedAt: 101,
		}, standing)
	}
}

func TestCommitmentStandingFromBinding_MapsAllFields(t *testing.T) {
	binding := itournament.ITournamentCommitmentStandingView{
		Joined:     true,
		FinalState: common.HexToHash("0x31"),
		Claimer:    common.HexToAddress("0x32"),
	}

	standing := commitmentStandingFromBinding(binding)

	require.Equal(t, binding.Joined, standing.Joined)
	require.Equal(t, common.Hash(binding.FinalState), standing.FinalState)
	require.Equal(t, binding.Claimer, standing.Claimer)
}

func TestValidateTournamentDescriptor_LevelCountAndKind(t *testing.T) {
	valid := func(level uint64, kind model.TournamentKind) TournamentDescriptor {
		return TournamentDescriptor{
			BaseCycle: big.NewInt(0),
			Level:     level,
			Kind:      kind,
		}
	}
	tests := []struct {
		name          string
		descriptor    TournamentDescriptor
		expectedLevel TournamentLevel
		levelCount    uint64
		wantError     string
	}{
		{name: "root of three levels", descriptor: valid(0, model.TournamentKindNonLeaf), levelCount: 3},
		{name: "dynamic last level", descriptor: valid(3, model.TournamentKindLeaf), expectedLevel: 3, levelCount: 4},
		{name: "zero level count", descriptor: valid(0, model.TournamentKindLeaf), wantError: "level count is zero"},
		{
			name:       "nil base cycle",
			descriptor: TournamentDescriptor{Level: 0, Kind: model.TournamentKindNonLeaf},
			levelCount: 2,
			wantError:  "invalid base cycle",
		},
		{
			name:       "negative base cycle",
			descriptor: TournamentDescriptor{BaseCycle: big.NewInt(-1), Level: 0, Kind: model.TournamentKindNonLeaf},
			levelCount: 2,
			wantError:  "invalid base cycle",
		},
		{
			name:          "unexpected level",
			descriptor:    valid(1, model.TournamentKindNonLeaf),
			expectedLevel: 2,
			levelCount:    3,
			wantError:     "does not match expected level",
		},
		{
			name:          "level outside count",
			descriptor:    valid(2, model.TournamentKindLeaf),
			expectedLevel: 2,
			levelCount:    2,
			wantError:     "outside level count",
		},
		{
			name:       "unknown kind",
			descriptor: valid(0, model.TournamentKind("UNKNOWN")),
			levelCount: 2,
			wantError:  "unknown kind",
		},
		{
			name:       "leaf before last level",
			descriptor: valid(0, model.TournamentKindLeaf),
			levelCount: 3,
			wantError:  "expected NON_LEAF",
		},
		{
			name:          "non-leaf at last level",
			descriptor:    valid(2, model.TournamentKindNonLeaf),
			expectedLevel: 2,
			levelCount:    3,
			wantError:     "expected LEAF",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTournamentDescriptor(test.descriptor, test.expectedLevel, test.levelCount)
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestValidateTournamentStanding_AllStates(t *testing.T) {
	candidate := common.HexToHash("0x41")
	tests := []struct {
		name       string
		descriptor TournamentDescriptor
		standing   TournamentStanding
		terminal   bool
	}{
		{
			name:       "matches active",
			descriptor: TournamentDescriptor{Level: 0},
			standing:   TournamentStanding{State: model.TournamentStandingMatchesActive},
		},
		{
			name:       "matches active and accepts joins",
			descriptor: TournamentDescriptor{Level: 0},
			standing: TournamentStanding{
				State: model.TournamentStandingMatchesActive, AcceptsJoins: true,
			},
		},
		{
			name:       "matches active with candidate",
			descriptor: TournamentDescriptor{Level: 0},
			standing: TournamentStanding{
				State: model.TournamentStandingMatchesActive, HasCandidate: true, Candidate: candidate,
			},
		},
		{
			name:       "awaiting closure",
			descriptor: TournamentDescriptor{Level: 0},
			standing: TournamentStanding{
				State: model.TournamentStandingAwaitingClosure, AcceptsJoins: true,
			},
		},
		{
			name:       "root winner",
			descriptor: TournamentDescriptor{Level: 0},
			standing: TournamentStanding{
				State: model.TournamentStandingRootWinner, HasCandidate: true, Candidate: candidate, FinishedAt: 9,
			},
			terminal: true,
		},
		{
			name:       "root failed",
			descriptor: TournamentDescriptor{Level: 0},
			standing:   TournamentStanding{State: model.TournamentStandingRootFailed, FinishedAt: 9},
			terminal:   true,
		},
		{
			name:       "inner winner",
			descriptor: TournamentDescriptor{Level: 1},
			standing: TournamentStanding{
				State: model.TournamentStandingInnerWinner, HasCandidate: true, Candidate: candidate,
				FinishedAt: 9,
			},
			terminal: true,
		},
		{
			name:       "inner eliminable without winner",
			descriptor: TournamentDescriptor{Level: 1},
			standing:   TournamentStanding{State: model.TournamentStandingInnerEliminableNoWinner, FinishedAt: 9},
			terminal:   true,
		},
		{
			name:       "inner eliminable with expired winner",
			descriptor: TournamentDescriptor{Level: 1},
			standing: TournamentStanding{
				State:        model.TournamentStandingInnerEliminableWinnerExpired,
				HasCandidate: true, Candidate: candidate, FinishedAt: 9,
			},
			terminal: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, validateTournamentStanding(test.standing, 10))
			require.Equal(t, test.terminal, isTerminalTournamentStanding(test.standing.State))
			if test.terminal {
				require.NotZero(t, test.standing.FinishedAt)
			} else {
				require.Zero(t, test.standing.FinishedAt)
			}
		})
	}
}

func TestValidateTournamentStanding_RejectsInvalidStoredBoundary(t *testing.T) {
	for _, test := range []struct {
		name      string
		standing  TournamentStanding
		wantError string
	}{
		{name: unknownStandingCase, standing: TournamentStanding{State: unknownEnumValue}, wantError: unknownStandingCase},
		{name: "terminal without finished block", standing: TournamentStanding{State: model.TournamentStandingRootFailed},
			wantError: "zero finished block"},
		{name: "finished after selected head", standing: TournamentStanding{State: model.TournamentStandingRootWinner, FinishedAt: 11},
			wantError: "after observed block"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(t, validateTournamentStanding(test.standing, 10), test.wantError)
		})
	}
}

func TestReadTournament_PinsDescriptorAndStandingReads(t *testing.T) {
	const mostRecentBlock uint64 = 100
	app := &model.Application{ID: 1, Name: "app", IApplicationAddress: common.HexToAddress("0x91")}
	epoch := &model.Epoch{Index: 3}
	tournamentAddress := common.HexToAddress("0x92")
	descriptor := TournamentDescriptor{
		BaseCycle: big.NewInt(0), Level: uint64(RootLevel), Kind: model.TournamentKindNonLeaf,
	}
	standing := TournamentStanding{State: model.TournamentStandingMatchesActive}
	pinned := mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts != nil && opts.Context != nil && opts.BlockNumber != nil &&
			opts.BlockNumber.Uint64() == mostRecentBlock
	})

	adapter := &tournamentAdapterMock{}
	adapter.On("Descriptor", pinned).Return(descriptor, nil).Once()
	adapter.On("Standing", pinned).Return(standing, nil).Once()
	expectTournamentAuxiliaryReads(adapter, pinned, RootLevel, standing.State)
	s, repo := newPRTServiceMock()

	tournament, err := s.readTournament(
		context.Background(), app, epoch, RootLevel, nil, nil, tournamentAddress, adapter, 2, mostRecentBlock,
	)
	require.NoError(t, err)
	require.Equal(t, tournamentAddress, tournament.Address)
	require.Equal(t, uint64(2), tournament.MaxLevel)
	require.Equal(t, uint64(RootLevel), tournament.Level)
	require.Empty(t, repo.Calls, "reading a projection must not write it")
	repo.AssertExpectations(t)
	adapter.AssertExpectations(t)
}

func TestGatherTournamentData_UsesDynamicLastLevel(t *testing.T) {
	for _, test := range []struct {
		name          string
		level         TournamentLevel
		wantChildScan bool
	}{
		{name: "last level from four-level factory", level: 3},
		{name: "level before last", level: 2, wantChildScan: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, repo := newPRTServiceMock()
			f := &observerCheckpointFixture{t: t, s: s, repo: repo, app: prtRevertTestApp(), factory: &adapterFactoryMock{}}
			s.adapterFactory = f.factory
			epoch := checkpointEpoch(3, "0x92")
			tournament := f.tournament(epoch, *epoch.TournamentAddress, uint64(test.level), 4, 100, 100, &TournamentEvents{}, nil)
			if test.wantChildScan {
				f.children(epoch, tournament)
			}
			batches, err := s.gatherTournamentData(t.Context(), f.app, epoch, test.level, nil, nil,
				*epoch.TournamentAddress, 4, 100)
			require.NoError(t, err)
			require.Len(t, batches, 1)
			require.Equal(t, uint64(test.level), batches[0].Tournament.Level)
			repo.AssertExpectations(t)
			f.factory.AssertExpectations(t)
		})
	}
}

func TestValidateJoinedCommitment_FinalState(t *testing.T) {
	machineHash := common.HexToHash("0xa1")
	t.Run("missing local machine hash", func(t *testing.T) {
		s, repo := newPRTServiceMock()
		app := &model.Application{ID: 1, Status: model.ApplicationStatus_OK}
		epoch := &model.Epoch{Index: 3}
		repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted,
			mock.MatchedBy(func(reason *string) bool {
				return reason != nil && strings.Contains(*reason, "has no machine hash")
			})).Return(nil).Once()

		err := s.validateJoinedCommitment(
			context.Background(), app, epoch, CommitmentStanding{Joined: true},
		)
		require.ErrorContains(t, err, "has no machine hash")
		require.Equal(t, model.ApplicationStatus_Corrupted, app.Status)
		repo.AssertExpectations(t)
	})

	t.Run("match", func(t *testing.T) {
		s, repo := newPRTServiceMock()
		app := &model.Application{ID: 1, Status: model.ApplicationStatus_OK}
		epoch := &model.Epoch{Index: 3, MachineHash: &machineHash}

		require.NoError(t, s.validateJoinedCommitment(
			context.Background(), app, epoch, CommitmentStanding{Joined: true, FinalState: machineHash},
		))
		require.Equal(t, model.ApplicationStatus_OK, app.Status)
		repo.AssertExpectations(t)
	})

	t.Run("latest mismatch does not change application status", func(t *testing.T) {
		s, repo := newPRTServiceMock()
		app := &model.Application{ID: 1, Status: model.ApplicationStatus_OK}
		epoch := &model.Epoch{Index: 3, MachineHash: &machineHash}
		onchain := common.HexToHash("0xa2")

		err := s.validateJoinedCommitment(
			context.Background(), app, epoch, CommitmentStanding{Joined: true, FinalState: onchain},
		)
		require.ErrorContains(t, err, "inconsistent final state")
		require.ErrorContains(t, err, machineHash.String())
		require.ErrorContains(t, err, onchain.String())
		require.Equal(t, model.ApplicationStatus_OK, app.Status)
		repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		repo.AssertExpectations(t)
	})
}
