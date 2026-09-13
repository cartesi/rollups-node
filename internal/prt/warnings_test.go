// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTournamentObservationWarnsOnlyForActiveDisputes(t *testing.T) {
	for _, test := range []struct {
		name  string
		state TournamentStandingState
		warn  bool
	}{
		{name: "active matches", state: TournamentStandingMatchesActive, warn: true},
		{name: "uncontested wait", state: TournamentStandingAwaitingClosure},
	} {
		for _, update := range []bool{false, true} {
			operation := projectionCreateCase
			if update {
				operation = projectionUpdateCase
			}
			t.Run(test.name+"/"+operation, func(t *testing.T) {
				s, repo := newPRTServiceMock()
				var output bytes.Buffer
				s.Logger = slog.New(slog.NewTextHandler(&output, nil))
				app := prtRevertTestApp()
				epoch := resultTestEpoch(EpochStatus_ClaimComputed)
				tournament := &Tournament{Address: *epoch.TournamentAddress, MaxLevel: 2, Height: 48, Kind: TournamentKindNonLeaf}
				adapter := &tournamentAdapterMock{}
				adapter.On("Descriptor", mock.Anything).Return(TournamentDescriptor{
					BaseCycle: big.NewInt(0), Height: 48, Kind: TournamentKindNonLeaf,
				}, nil).Twice()
				adapter.On("Standing", mock.Anything).Return(TournamentStanding{State: test.state}, nil).Twice()
				for range 2 {
					expectTournamentAuxiliaryReads(adapter, mock.Anything, RootLevel, test.state)
					if update {
						require.NoError(t, s.refreshTournament(t.Context(), app, epoch, RootLevel, adapter, tournament, 20))
					} else {
						_, err := s.readTournament(t.Context(), app, epoch, RootLevel, nil, nil, tournament.Address, adapter, 2, 20)
						require.NoError(t, err)
					}
				}
				wantWarnings := 0
				if test.warn {
					wantWarnings = 1
					require.Contains(t, output.String(), "sends no dispute moves")
					require.Contains(t, output.String(), "commitment can lose by timeout")
					require.Contains(t, output.String(), "application="+app.Name)
					require.Contains(t, output.String(), "epoch_index=3")
					require.Contains(t, output.String(), tournament.Address.Hex())
				}
				require.Equal(t, wantWarnings, strings.Count(output.String(), "level=WARN"))
				require.Equal(t, ApplicationStatus_OK, app.Status)
				require.Empty(t, repo.Calls)
				adapter.AssertExpectations(t)
			})
		}
	}
}

func TestStoredInnerTournamentWarningsFollowCurrentStanding(t *testing.T) {
	for _, test := range []struct {
		name         string
		state        TournamentStandingState
		finish       uint64
		wantWarnings int
	}{
		{name: "active after restart", state: TournamentStandingMatchesActive, wantWarnings: 2},
		{name: "finished history", state: TournamentStandingInnerEliminableWinnerExpired, finish: 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := prtRevertTestApp()
			epoch := resultTestEpoch(EpochStatus_ClaimComputed)
			inner := &Tournament{Address: *epoch.TournamentAddress, Level: 1, MaxLevel: 2, Kind: TournamentKindLeaf}
			var output bytes.Buffer
			for range 2 {
				s, repo := newPRTServiceMock()
				s.Logger = slog.New(slog.NewTextHandler(&output, nil))
				adapter := &tournamentAdapterMock{}
				for range 2 {
					adapter.On("Descriptor", mock.Anything).Return(TournamentDescriptor{
						BaseCycle: big.NewInt(0), Level: 1, Kind: TournamentKindLeaf,
					}, nil).Once()
					adapter.On("Standing", mock.Anything).Return(TournamentStanding{
						State: test.state, HasCandidate: true, Candidate: *epoch.Commitment, FinishedAt: test.finish,
					}, nil).Once()
					expectTournamentAuxiliaryReads(adapter, mock.Anything, 1, test.state)
					require.NoError(t, s.refreshTournament(t.Context(), app, epoch, 1, adapter, inner, 20))
				}
				require.Empty(t, repo.Calls)
				adapter.AssertExpectations(t)
			}
			require.Equal(t, test.wantWarnings, strings.Count(output.String(), "sends no dispute moves"))
			require.Equal(t, ApplicationStatus_OK, app.Status)
		})
	}
}
