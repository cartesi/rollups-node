// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestMatchProjectionPreservesCurrentPhaseAndFullWidth(t *testing.T) {
	position := new(big.Int).Lsh(big.NewInt(1), 200)
	cycle := new(big.Int).Add(position, big.NewInt(3))
	left, right, parent := common.HexToHash("0x11"), common.HexToHash("0x22"), common.HexToHash("0x33")
	for _, phase := range []MatchPhase{MatchPhaseBisecting, MatchPhaseReadyToSeal, MatchPhaseSealed, MatchPhaseUninitialized} {
		t.Run(string(phase), func(t *testing.T) {
			view := ObservedMatchSnapshot{Phase: phase, TimeoutOutcome: MatchTimeoutTwoWins, DeferredCharge: 7}
			switch phase {
			case MatchPhaseBisecting:
				view.Bisecting = &BisectingMatch{RevealingParent: parent, WaitingLeft: left, WaitingRight: right,
					SegmentStartPosition: new(big.Int).Set(position), SegmentStartCycle: new(big.Int).Set(cycle),
					CurrentHeight: 0, Responder: CommitmentSideTwo}
			case MatchPhaseReadyToSeal:
				view.ReadyToSeal = &ReadyToSealMatch{RevealingParent: parent, WaitingLeft: left, WaitingRight: right,
					SegmentStartPosition: new(big.Int).Set(position), SegmentStartCycle: new(big.Int).Set(cycle),
					Responder: CommitmentSideOne}
			case MatchPhaseSealed:
				view.Sealed = &SealedMatch{AgreeState: parent, FinalStateOne: left, FinalStateTwo: right,
					DivergencePosition: new(big.Int).Set(position), DivergenceCycle: new(big.Int).Set(cycle)}
			case MatchPhaseUninitialized:
				view.TimeoutOutcome, view.DeferredCharge = MatchTimeoutNone, 0
			}
			projection, err := projectMatchSnapshot(view, 100, phase == MatchPhaseUninitialized)
			require.NoError(t, err)
			require.Equal(t, uint64(100), projection.AsOfBlock)
			require.Equal(t, phase, projection.Phase)
			require.Equal(t, view.TimeoutOutcome, projection.TimeoutOutcome)
			require.Equal(t, view.DeferredCharge, projection.DeferredCharge)
			switch phase {
			case MatchPhaseBisecting, MatchPhaseReadyToSeal:
				require.NotNil(t, projection.Bisection)
				require.Nil(t, projection.Sealed)
				require.Equal(t, parent, projection.Bisection.RevealingParent)
				require.Equal(t, left, projection.Bisection.WaitingLeft)
				require.Equal(t, right, projection.Bisection.WaitingRight)
				require.Equal(t, position, projection.Bisection.SegmentStartPosition.ToBig())
				require.Equal(t, cycle, projection.Bisection.SegmentStartCycle.ToBig())
				if phase == MatchPhaseBisecting {
					require.NotNil(t, projection.Bisection.CurrentHeight, "zero height is present in BISECTING")
					require.Zero(t, *projection.Bisection.CurrentHeight)
					require.Equal(t, CommitmentSideTwo, projection.Bisection.Responder)
					view.Bisecting.SegmentStartPosition.SetUint64(0)
				} else {
					require.Nil(t, projection.Bisection.CurrentHeight)
					require.Equal(t, CommitmentSideOne, projection.Bisection.Responder)
					view.ReadyToSeal.SegmentStartPosition.SetUint64(0)
				}
				require.Equal(t, position, projection.Bisection.SegmentStartPosition.ToBig())
			case MatchPhaseSealed:
				require.Nil(t, projection.Bisection)
				require.Equal(t, parent, projection.Sealed.AgreeState)
				require.Equal(t, left, projection.Sealed.FinalStateOne)
				require.Equal(t, right, projection.Sealed.FinalStateTwo)
				require.Equal(t, position, projection.Sealed.DivergencePosition.ToBig())
				require.Equal(t, cycle, projection.Sealed.DivergenceCycle.ToBig())
				view.Sealed.DivergencePosition.SetUint64(0)
				require.Equal(t, position, projection.Sealed.DivergencePosition.ToBig())
			case MatchPhaseUninitialized:
				require.Nil(t, projection.Bisection)
				require.Nil(t, projection.Sealed)
			}
		})
	}
}

func TestMatchProjectionRejectsInvalidPhasePayloads(t *testing.T) {
	for _, view := range []ObservedMatchSnapshot{
		{Phase: unknownEnumValue},
		{Phase: MatchPhaseBisecting},
		{Phase: MatchPhaseReadyToSeal},
		{Phase: MatchPhaseSealed},
		{Phase: MatchPhaseBisecting, Bisecting: &BisectingMatch{SegmentStartPosition: big.NewInt(-1), SegmentStartCycle: new(big.Int)}},
		{Phase: MatchPhaseReadyToSeal, ReadyToSeal: &ReadyToSealMatch{SegmentStartPosition: new(big.Int), SegmentStartCycle: nil}},
		{Phase: MatchPhaseSealed, Sealed: &SealedMatch{
			DivergencePosition: new(big.Int).Lsh(big.NewInt(1), 256), DivergenceCycle: new(big.Int)}},
		{Phase: MatchPhaseBisecting, Bisecting: &BisectingMatch{}, Sealed: &SealedMatch{}},
		{Phase: MatchPhaseUninitialized, TimeoutOutcome: MatchTimeoutOneWins},
	} {
		_, err := projectMatchSnapshot(view, 100, false)
		require.Error(t, err)
	}
	_, err := projectMatchSnapshot(ObservedMatchSnapshot{Phase: MatchPhaseBisecting}, 100, true)
	require.ErrorContains(t, err, "deleted match")
	_, err = projectMatchSnapshot(ObservedMatchSnapshot{Phase: MatchPhaseUninitialized, TimeoutOutcome: MatchTimeoutNone}, 100, false)
	require.ErrorContains(t, err, "no matching deletion")
}

func TestInnerCurrentProjectionExpiresWithoutRestoringHistoricalWinner(t *testing.T) {
	s, repo := newPRTServiceMock()
	app, epoch := prtRevertTestApp(), checkpointEpoch(0, "0x100")
	projection := &Tournament{Address: common.HexToAddress("0x101"), Level: 1, MaxLevel: 2}
	adapter := &tournamentAdapterMock{}
	candidate, parent, final := common.HexToHash("0x11"), common.HexToHash("0x22"), common.HexToHash("0x33")
	for _, block := range []uint64{100, 101, 102} {
		opts := mock.MatchedBy(resultCallOptsAtBlock(block))
		standing := TournamentStanding{State: TournamentStandingInnerWinner, HasCandidate: true, Candidate: candidate,
			FinalState: final, ParentCommitment: parent, FinishedAt: 90, WinnerExpiresAt: 102}
		inner := InnerResult{Disposition: InnerTournamentWinner, ParentCommitment: parent, PausedAllowance: 102 - block}
		if block == 102 {
			standing.State, standing.FinalState, standing.ParentCommitment, standing.WinnerExpiresAt =
				TournamentStandingInnerEliminableWinnerExpired, common.Hash{}, common.Hash{}, 0
			inner = InnerResult{Disposition: InnerTournamentEliminable}
		}
		adapter.On("Standing", opts).Return(standing, nil).Once()
		adapter.On("InnerResult", opts).Return(inner, nil).Once()
		adapter.On("BondRecovery", opts).
			Return(canonicalBondRecovery(BondDispositionRecoverable, common.HexToAddress("0x777"), 0), nil).Once()
		require.NoError(t, s.updateTournamentStanding(t.Context(), app, epoch, 1, adapter, projection, block))
		require.Equal(t, block, projection.Snapshot.AsOfBlock)
		require.Equal(t, candidate, *projection.Snapshot.Candidate)
		require.NotNil(t, projection.Snapshot.BondRecovery.Payment, "zero recovery amount is still present")
		if block < 102 {
			require.Equal(t, candidate, *projection.Snapshot.WinnerCommitment)
			require.Equal(t, final, *projection.Snapshot.FinalStateHash)
			require.Equal(t, parent, *projection.Snapshot.ParentCommitment)
			require.Equal(t, uint64(102), projection.Snapshot.WinnerExpiresAt)
			require.Equal(t, uint64(102-block), projection.Snapshot.InnerResult.PausedAllowance)
		} else {
			require.Nil(t, projection.Snapshot.WinnerCommitment)
			require.Nil(t, projection.Snapshot.FinalStateHash)
			require.Nil(t, projection.Snapshot.ParentCommitment)
			require.Zero(t, projection.Snapshot.WinnerExpiresAt)
			require.Equal(t, InnerTournamentEliminable, projection.Snapshot.InnerResult.Disposition)
			require.Nil(t, projection.Snapshot.InnerResult.ParentCommitment)
			require.Zero(t, projection.Snapshot.InnerResult.PausedAllowance)
		}
	}
	require.Empty(t, repo.Calls)
	adapter.AssertNotCalled(t, "CommitmentStanding", mock.Anything, mock.Anything)
	adapter.AssertExpectations(t)
}
