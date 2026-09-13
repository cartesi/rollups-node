// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func (f *passiveDisputeFixture) observedTournament(layer *passiveDisputeLayer) (*Tournament, *bind.CallOpts) {
	f.t.Helper()
	head, err := f.client.BlockNumber(f.ctx)
	f.r.NoError(err)
	var result *Tournament
	f.r.NoError(pollUntil(f.ctx, passiveObserverPoll, func() (bool, error) {
		var response api.ListResponse[*Tournament]
		err := f.rpc.Call(f.ctx, "cartesi_listTournaments", api.ListTournamentsParams{Application: f.appName}, &response)
		if err != nil {
			return false, err
		}
		for _, row := range response.Data {
			if row.Address == layer.address && row.Snapshot.AsOfBlock >= head {
				result = row
				return true, nil
			}
		}
		return false, nil
	}), "publish the complete tournament snapshot at block %d", head)
	f.r.Equal(head, result.Snapshot.AsOfBlock, "no interval block may race the API assertions")
	return result, &bind.CallOpts{Context: f.ctx, BlockNumber: new(big.Int).SetUint64(head)}
}

func (f *passiveDisputeFixture) observedMatch(layer *passiveDisputeLayer) *Match {
	f.t.Helper()
	var response api.SingleResponse[*Match]
	f.r.NoError(f.rpc.Call(f.ctx, "cartesi_getMatch", api.GetMatchParams{Application: f.appName,
		EpochIndex: "0x0", TournamentAddress: layer.address.Hex(), IDHash: layer.matchHash.Hex()}, &response))
	f.r.NotNil(response.Data)
	return response.Data
}

func (f *passiveDisputeFixture) assertCurrent(layer *passiveDisputeLayer, expected MatchPhase) {
	f.t.Helper()
	tournament, opts := f.observedTournament(layer)
	f.r.Equal(layer.descriptor.Height, tournament.Height)
	f.r.Equal(layer.descriptor.Log2Stride, tournament.Log2Step)
	f.r.Equal(layer.descriptor.StartInstant, tournament.StartInstant)
	f.r.Equal(layer.descriptor.Allowance, tournament.Allowance)
	f.r.Equal(common.Hash(layer.descriptor.InitialHash), tournament.InitialHash)
	f.r.Zero(layer.descriptor.BaseCycle.Cmp(tournament.BaseCycle.ToBig()))
	if layer.descriptor.Level > 0 {
		f.r.Greater(tournament.BaseCycle.ToBig().BitLen(), 64, "nested cycle positions must not truncate to uint64")
		f.r.NotNil(tournament.CreationEvent)
		f.r.NotNil(tournament.Snapshot.InnerResult)
	} else {
		f.r.Nil(tournament.CreationEvent)
		f.r.Nil(tournament.Snapshot.InnerResult)
	}
	match := f.observedMatch(layer)
	if expected == MatchPhaseUninitialized {
		f.r.Positive(match.DeletionBlockNumber)
		f.r.GreaterOrEqual(match.Snapshot.AsOfBlock, match.DeletionBlockNumber)
		f.r.LessOrEqual(match.Snapshot.AsOfBlock, tournament.Snapshot.AsOfBlock)
	} else {
		f.r.Equal(tournament.Snapshot.AsOfBlock, match.Snapshot.AsOfBlock)
	}
	f.r.Equal(expected, match.Snapshot.Phase)
	f.r.Equal(layer.trees[0].root(), match.CommitmentOne)
	f.r.Equal(layer.trees[1].root(), match.CommitmentTwo)
	timeout, err := layer.contract.ClassifyMatchTimeout(opts, layer.matchID)
	f.r.NoError(err)
	outcomes := []MatchTimeoutOutcome{MatchTimeoutNone, MatchTimeoutOneWins, MatchTimeoutTwoWins, MatchTimeoutEliminateBoth}
	f.r.Less(int(timeout.Outcome), len(outcomes))
	f.r.Equal(outcomes[timeout.Outcome], match.Snapshot.TimeoutOutcome)
	f.r.Equal(timeout.DeferredCharge, match.Snapshot.DeferredCharge)
	switch expected {
	case MatchPhaseBisecting:
		view, err := layer.contract.BisectingMatch(opts, layer.matchHash)
		f.r.NoError(err)
		f.r.Equal(uint8(1), view.ActualPhase)
		f.r.NotNil(match.Snapshot.Bisection)
		bisection := match.Snapshot.Bisection
		f.r.NotNil(bisection.CurrentHeight)
		f.r.Equal(view.Value.CurrentHeight, *bisection.CurrentHeight)
		f.assertBisection(bisection, view.Value.RevealingParent, view.Value.WaitingLeft, view.Value.WaitingRight,
			view.Value.SegmentStartPosition, view.Value.SegmentStartCycle, view.Value.Responder)
		f.r.Nil(match.Snapshot.Sealed)
	case MatchPhaseReadyToSeal:
		view, err := layer.contract.ReadyToSealMatch(opts, layer.matchHash)
		f.r.NoError(err)
		f.r.Equal(uint8(2), view.ActualPhase)
		f.r.NotNil(match.Snapshot.Bisection)
		f.r.Nil(match.Snapshot.Bisection.CurrentHeight)
		f.assertBisection(match.Snapshot.Bisection, view.Value.RevealingParent, view.Value.WaitingLeft, view.Value.WaitingRight,
			view.Value.SegmentStartPosition, view.Value.SegmentStartCycle, view.Value.Responder)
		f.r.Nil(match.Snapshot.Sealed)
	case MatchPhaseSealed:
		view, err := layer.contract.SealedMatch(opts, layer.matchHash)
		f.r.NoError(err)
		f.r.Equal(uint8(3), view.ActualPhase)
		f.r.Nil(match.Snapshot.Bisection)
		f.r.NotNil(match.Snapshot.Sealed)
		sealed := match.Snapshot.Sealed
		f.r.Equal(common.Hash(view.Value.AgreeState), sealed.AgreeState)
		f.r.Equal(*f.epoch.MachineHash, sealed.AgreeState)
		f.r.Zero(view.Value.DivergencePosition.Cmp(sealed.DivergencePosition.ToBig()))
		f.r.Zero(view.Value.DivergenceCycle.Cmp(sealed.DivergenceCycle.ToBig()))
		f.r.Equal(layer.trees[0].leafCount()-1, sealed.DivergencePosition.ToBig().Uint64())
		f.r.Greater(sealed.DivergenceCycle.ToBig().BitLen(), 64)
		f.r.Equal(common.Hash(view.Value.FinalStateOne), sealed.FinalStateOne)
		f.r.Equal(common.Hash(view.Value.FinalStateTwo), sealed.FinalStateTwo)
		if layer.descriptor.Level == 2 {
			f.r.NotNil(match.LeafSeal)
		} else {
			f.r.Nil(match.LeafSeal)
		}
	case MatchPhaseUninitialized:
		f.r.Nil(match.Snapshot.Bisection)
		f.r.Nil(match.Snapshot.Sealed)
		f.r.NotNil(match.DeletionTxHash)
		f.r.NotNil(match.DeletionLogIndex)
		f.r.Equal(WinnerCommitment_ONE, match.Winner)
		if layer.descriptor.Level == 2 {
			f.r.Equal(MatchDeletionReason_TIMEOUT, match.DeletionReason)
			f.r.NotNil(match.LeafSeal, "deletion must preserve the immutable leaf seal")
		} else {
			f.r.Equal(MatchDeletionReason_CHILD_TOURNAMENT, match.DeletionReason)
		}
	default:
		f.r.FailNow("unexpected test phase", "%s", expected)
	}
	f.assertCommitmentClocks(layer, opts)
}

func (f *passiveDisputeFixture) assertBisection(
	observed *MatchBisectionSnapshot, parent, left, right [32]byte, position, cycle *big.Int, responder uint8,
) {
	f.r.Equal(common.Hash(parent), observed.RevealingParent)
	f.r.Equal(common.Hash(left), observed.WaitingLeft)
	f.r.Equal(common.Hash(right), observed.WaitingRight)
	f.r.Zero(position.Cmp(observed.SegmentStartPosition.ToBig()))
	f.r.Zero(cycle.Cmp(observed.SegmentStartCycle.ToBig()))
	sides := []CommitmentSide{CommitmentSideOne, CommitmentSideTwo}
	f.r.Less(int(responder), len(sides))
	f.r.Equal(sides[responder], observed.Responder)
}

func (f *passiveDisputeFixture) assertCommitmentClocks(layer *passiveDisputeLayer, opts *bind.CallOpts) {
	for side, tree := range layer.trees {
		var response api.SingleResponse[*Commitment]
		f.r.NoError(f.rpc.Call(f.ctx, "cartesi_getCommitment", api.GetCommitmentParams{Application: f.appName,
			EpochIndex: "0x0", TournamentAddress: layer.address.Hex(), Commitment: tree.root().Hex()}, &response))
		f.r.NotNil(response.Data)
		commitment := response.Data
		view, err := layer.contract.CommitmentStanding(opts, tree.root())
		f.r.NoError(err)
		f.r.True(view.Joined)
		f.r.Equal(f.actors[side].From, commitment.SubmitterAddress, "the original participant must remain visible")
		f.r.Equal(common.Hash(view.FinalState), commitment.FinalStateHash)
		f.r.Equal(view.Claimer, commitment.Snapshot.Claimer)
		f.r.Equal(view.ClockRunning, commitment.Snapshot.ClockRunning)
		f.r.Equal(view.ClockDeadline, commitment.Snapshot.ClockDeadline)
		f.r.Equal(view.ClockAllowance, commitment.Snapshot.ClockAllowance)
		f.r.Equal(opts.BlockNumber.Uint64(), commitment.Snapshot.AsOfBlock)
	}
}

func (f *passiveDisputeFixture) assertWinner(layer *passiveDisputeLayer, expected TournamentStandingState) {
	tournament, opts := f.observedTournament(layer)
	view, err := layer.contract.TournamentStanding(opts)
	f.r.NoError(err)
	f.r.Equal(expected, tournament.Snapshot.Standing)
	f.r.NotNil(tournament.Snapshot.WinnerCommitment)
	f.r.Equal(layer.trees[0].root(), *tournament.Snapshot.WinnerCommitment)
	f.r.Equal(tournament.Snapshot.WinnerCommitment, tournament.Snapshot.Candidate)
	f.r.NotNil(tournament.Snapshot.FinalStateHash)
	f.r.Equal(layer.trees[0].final[0], *tournament.Snapshot.FinalStateHash)
	f.r.Equal(view.FinishedAt, tournament.Snapshot.FinishedAtBlock)
	f.r.Positive(view.FinishedAt)
	f.r.Equal(view.WinnerExpiresAt, tournament.Snapshot.WinnerExpiresAt)
	f.r.False(tournament.Snapshot.AcceptsJoins)
	if layer.descriptor.Level > 0 {
		inner, err := layer.contract.InnerResult(opts)
		f.r.NoError(err)
		f.r.NotNil(tournament.Snapshot.InnerResult)
		f.r.Equal(InnerTournamentWinner, tournament.Snapshot.InnerResult.Disposition)
		f.r.NotNil(tournament.Snapshot.InnerResult.ParentCommitment)
		f.r.Equal(common.Hash(inner.ParentCommitment), *tournament.Snapshot.InnerResult.ParentCommitment)
		f.r.Equal(inner.PausedAllowance, tournament.Snapshot.InnerResult.PausedAllowance)
		f.r.Greater(view.WinnerExpiresAt, opts.BlockNumber.Uint64()+uint64(layer.descriptor.Level))
	}
}

func (f *passiveDisputeFixture) assertRecoveredRoot() {
	root := f.layers[0]
	tournament, opts := f.observedTournament(root)
	f.r.Equal(TournamentStandingRootWinner, tournament.Snapshot.Standing)
	f.r.Equal(BondDispositionRecovered, tournament.Snapshot.BondRecovery.Disposition)
	f.r.Nil(tournament.Snapshot.BondRecovery.Claimer)
	f.r.Nil(tournament.Snapshot.BondRecovery.Payment)
	f.assertCommitmentClocks(root, opts)
	var response api.SingleResponse[*Commitment]
	f.r.NoError(f.rpc.Call(f.ctx, "cartesi_getCommitment", api.GetCommitmentParams{Application: f.appName,
		EpochIndex: "0x0", TournamentAddress: root.address.Hex(), Commitment: root.trees[0].root().Hex()}, &response))
	f.r.Equal(common.Address{}, response.Data.Snapshot.Claimer)
	f.r.Equal(f.actors[0].From, response.Data.SubmitterAddress)
}

func (f *passiveDisputeFixture) assertInnerWinnersExpire() {
	var expiry uint64
	deletedMatches := make(map[common.Address]*Match, len(f.layers)-1)
	for _, layer := range f.layers[1:] {
		standing, err := layer.contract.TournamentStanding(&bind.CallOpts{Context: f.ctx})
		f.r.NoError(err)
		f.r.Positive(standing.WinnerExpiresAt, "retain the winner long enough to propagate and accept")
		expiry = max(expiry, standing.WinnerExpiresAt)
		deletedMatches[layer.address] = f.observedMatch(layer)
	}
	// No transaction emits a log for this transition. The observer must still
	// refresh current views and remove winner fields which no longer apply.
	f.mineTo(expiry)
	for _, layer := range f.layers[1:] {
		tournament, _ := f.observedTournament(layer)
		snapshot := tournament.Snapshot
		deleted := deletedMatches[layer.address]
		f.r.Greater(snapshot.AsOfBlock, deleted.Snapshot.AsOfBlock, "observe a later window after match deletion")
		f.r.Equal(deleted, f.observedMatch(layer), "keep the certified snapshot, deletion facts, and row timestamps")
		f.r.Equal(TournamentStandingInnerEliminableWinnerExpired, snapshot.Standing)
		f.r.NotNil(snapshot.Candidate)
		f.r.Equal(layer.trees[0].root(), *snapshot.Candidate)
		f.r.Nil(snapshot.WinnerCommitment)
		f.r.Nil(snapshot.FinalStateHash)
		f.r.Nil(snapshot.ParentCommitment)
		f.r.Zero(snapshot.WinnerExpiresAt)
		f.r.Positive(snapshot.FinishedAtBlock)
		f.r.NotNil(snapshot.InnerResult)
		f.r.Equal(InnerTournamentEliminable, snapshot.InnerResult.Disposition)
		f.r.Nil(snapshot.InnerResult.ParentCommitment)
		f.r.Zero(snapshot.InnerResult.PausedAllowance)
		f.r.Equal(BondDispositionRecoverable, snapshot.BondRecovery.Disposition,
			"winner expiry must not hide the unrecovered bond")
	}
}

func (f *passiveDisputeFixture) assertLocalDivergence() {
	var response api.SingleResponse[*Application]
	f.r.NoError(pollUntil(f.ctx, passiveObserverPoll, func() (bool, error) {
		if err := f.rpc.Call(f.ctx, "cartesi_getApplication", api.GetApplicationParams{Application: f.appName}, &response); err != nil {
			return false, err
		}
		return response.Data != nil && response.Data.Status == ApplicationStatus_Diverged, nil
	}), "mark a losing local commitment as diverged after publishing the winner")
	f.r.True(response.Data.Enabled, "passive observation must continue while the application is diverged")
	f.r.NotNil(response.Data.Reason)
	f.r.Contains(*response.Data.Reason, "inconsistent commitment")
	epoch, err := readEpoch(f.ctx, f.appName, 0)
	f.r.NoError(err)
	f.r.Equal(EpochStatus_ClaimComputed, epoch.Status)
	f.r.Nil(epoch.ClaimTransactionHash)
	f.r.Nil(epoch.StagedAtBlock, "the incorrect winner must never use a fabricated machine-state proof")
	f.r.Equal(f.epoch.Commitment, epoch.Commitment)
}

func (f *passiveDisputeFixture) listAdvances(layer *passiveDisputeLayer) []*MatchAdvanced {
	var response api.ListResponse[*MatchAdvanced]
	f.r.NoError(f.rpc.Call(f.ctx, "cartesi_listMatchAdvances", api.ListMatchAdvancesParams{Application: f.appName,
		EpochIndex: "0x0", TournamentAddress: layer.address.Hex(), IDHash: layer.matchHash.Hex(), Limit: 100}, &response))
	f.r.Equal(layer.descriptor.Height-1, uint64(len(response.Data)))
	f.r.Equal(layer.descriptor.Height-1, response.Pagination.TotalCount)
	for _, event := range response.Data {
		var single api.SingleResponse[*MatchAdvanced]
		f.r.NoError(f.rpc.Call(f.ctx, "cartesi_getMatchAdvance", api.GetMatchAdvanceParams{Application: f.appName,
			EpochIndex: "0x0", TournamentAddress: layer.address.Hex(), IDHash: layer.matchHash.Hex(),
			TxHash: event.TxHash.Hex(), LogIndex: hexutil.EncodeUint64(event.LogIndex)}, &single))
		f.r.Equal(event, single.Data, "get and list must expose identical advance content")
	}
	return response.Data
}

func passiveEventIdentity(hash common.Hash, index uint64) string {
	return fmt.Sprintf("%s/%d", hash, index)
}
