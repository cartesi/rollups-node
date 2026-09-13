// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type tournamentViewRPC struct{ mock.Mock }

func (m *tournamentViewRPC) Call(
	_ context.Context, call map[string]json.RawMessage, block rpc.BlockNumber,
) (hexutil.Bytes, error) {
	var address common.Address
	var input hexutil.Bytes
	if err := json.Unmarshal(call["to"], &address); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(call["input"], &input); err != nil {
		return nil, err
	}
	args := m.Called(address, []byte(input), block)
	return args.Get(0).([]byte), args.Error(1)
}

type tournamentViewFixture struct {
	t       *testing.T
	adapter TournamentAdapter
	backend *tournamentViewRPC
	abi     *abi.ABI
	address common.Address
	opts    *bind.CallOpts
}

func newTournamentViewFixture(t *testing.T) *tournamentViewFixture {
	t.Helper()
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	backend := &tournamentViewRPC{}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	address := common.HexToAddress("0x1234")
	adapter, err := NewITournamentAdapter(address, client, ethutil.Filter{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	require.NoError(t, err)
	t.Cleanup(func() { backend.AssertExpectations(t) })
	return &tournamentViewFixture{t: t, adapter: adapter, backend: backend, abi: contractABI,
		address: address, opts: &bind.CallOpts{Context: t.Context(), BlockNumber: big.NewInt(200)}}
}

func (f *tournamentViewFixture) expect(name string, inputs []any, outputs ...any) {
	f.t.Helper()
	encoded, err := f.abi.Methods[name].Outputs.Pack(outputs...)
	require.NoError(f.t, err)
	f.expectBytes(name, inputs, encoded, nil)
}

func (f *tournamentViewFixture) expectBytes(name string, inputs []any, output []byte, callErr error) {
	f.t.Helper()
	input, err := f.abi.Pack(name, inputs...)
	require.NoError(f.t, err)
	f.backend.On("Call", f.address, input, rpc.BlockNumber(f.opts.BlockNumber.Int64())).Return(output, callErr).Once()
}

func TestTournamentAdapterDecodesCompleteViews(t *testing.T) {
	f := newTournamentViewFixture(t)
	one, two, three := common.HexToHash("0x11"), common.HexToHash("0x22"), common.HexToHash("0x33")
	large := new(big.Int).Lsh(big.NewInt(1), 200)
	f.expect("tournamentDescriptor", nil, itournament.ITournamentTournamentDescriptor{
		InitialHash: one, BaseCycle: large, Log2Stride: 3, Height: 4, Level: 5, Kind: 1, StartInstant: 6, Allowance: 7,
	})
	descriptor, err := f.adapter.Descriptor(f.opts)
	require.NoError(t, err)
	require.Equal(t, TournamentDescriptor{InitialHash: one, BaseCycle: large, Log2Stride: 3, Height: 4, Level: 5,
		Kind: model.TournamentKindNonLeaf, StartInstant: 6, Allowance: 7}, descriptor)

	f.expect("tournamentStanding", nil, itournament.ITournamentTournamentStandingView{
		Standing: 4, HasCandidate: true, Candidate: one, FinalState: two, ParentCommitment: three, FinishedAt: 8, WinnerExpiresAt: 9,
	})
	standing, err := f.adapter.Standing(f.opts)
	require.NoError(t, err)
	require.Equal(t, TournamentStanding{State: model.TournamentStandingInnerWinner, HasCandidate: true,
		Candidate: one, FinalState: two, ParentCommitment: three, FinishedAt: 8, WinnerExpiresAt: 9}, standing)

	f.expect("commitmentStanding", []any{one}, itournament.ITournamentCommitmentStandingView{
		Joined: true, FinalState: two, Claimer: f.address, ClockRunning: true, ClockDeadline: 11, ClockAllowance: 12,
	})
	commitment, err := f.adapter.CommitmentStanding(f.opts, one)
	require.NoError(t, err)
	require.Equal(t, CommitmentStanding{Joined: true, FinalState: two, Claimer: f.address,
		ClockRunning: true, ClockDeadline: 11, ClockAllowance: 12}, commitment)

	f.expect("innerResult", nil, itournament.ITournamentInnerResultView{Disposition: 1, ParentCommitment: three, PausedAllowance: 13})
	inner, err := f.adapter.InnerResult(f.opts)
	require.NoError(t, err)
	require.Equal(t, InnerResult{Disposition: model.InnerTournamentWinner, ParentCommitment: three, PausedAllowance: 13}, inner)
	f.expect("bondRecovery", nil, uint8(2), f.address, large)
	bond, err := f.adapter.BondRecovery(f.opts)
	require.NoError(t, err)
	require.Equal(t, BondRecovery{Disposition: model.BondDispositionRecoverable, Claimer: f.address, Payment: large}, bond)
	f.expect("bondValue", nil, large)
	value, err := f.adapter.BondValue(f.opts)
	require.NoError(t, err)
	require.Equal(t, large, value)
}

func TestTournamentAdapterReadsSixDistinctCounters(t *testing.T) {
	f := newTournamentViewFixture(t)
	names := []string{"getCommitmentJoinedCount", "getMatchCreatedCount", "getMatchAdvancedCount",
		"getLeafMatchSealedCount", "getMatchDeletedCount", "getNewInnerTournamentCount"}
	values := make([]*big.Int, len(names))
	for index, name := range names {
		values[index] = new(big.Int).Lsh(big.NewInt(1), 200+uint(index))
		f.expect(name, nil, values[index])
	}
	counts, err := f.adapter.StructuralEventCounts(f.opts)
	require.NoError(t, err)
	require.Equal(t, StructuralEventCounts{CommitmentJoined: values[0], MatchCreated: values[1], MatchAdvanced: values[2],
		LeafMatchSealed: values[3], MatchDeleted: values[4], NewInnerTournament: values[5]}, counts)
}

func TestTournamentAdapterDecodesStandingOrdinalsAndTimeOnlyChanges(t *testing.T) {
	states := []struct {
		ordinal uint8
		state   model.TournamentStandingState
	}{
		{0, model.TournamentStandingMatchesActive}, {1, model.TournamentStandingAwaitingClosure},
		{2, model.TournamentStandingRootWinner}, {3, model.TournamentStandingRootFailed}, {4, model.TournamentStandingInnerWinner},
		{5, model.TournamentStandingInnerEliminableNoWinner}, {6, model.TournamentStandingInnerEliminableWinnerExpired},
	}
	for _, test := range states {
		t.Run(string(test.state), func(t *testing.T) {
			f := newTournamentViewFixture(t)
			f.expect("tournamentStanding", nil, itournament.ITournamentTournamentStandingView{Standing: test.ordinal})
			standing, err := f.adapter.Standing(f.opts)
			require.NoError(t, err)
			require.Equal(t, test.state, standing.State)
		})
	}
	f := newTournamentViewFixture(t)
	f.expect("tournamentStanding", nil, itournament.ITournamentTournamentStandingView{AcceptsJoins: true})
	open, err := f.adapter.Standing(f.opts)
	require.NoError(t, err)
	require.True(t, open.AcceptsJoins)
	require.False(t, open.HasCandidate)
	one := common.HexToHash("0x11")
	f.expect("tournamentStanding", nil, itournament.ITournamentTournamentStandingView{
		Standing: 6, HasCandidate: true, Candidate: one, FinishedAt: 100,
	})
	expired, err := f.adapter.Standing(f.opts)
	require.NoError(t, err)
	require.Equal(t, TournamentStanding{State: model.TournamentStandingInnerEliminableWinnerExpired,
		HasCandidate: true, Candidate: one, FinishedAt: 100}, expired)
}

func TestTournamentAdapterDecodesInactiveViews(t *testing.T) {
	f := newTournamentViewFixture(t)
	one := common.HexToHash("0x11")
	for _, binding := range []itournament.ITournamentCommitmentStandingView{
		{}, {Joined: true, FinalState: one, ClockAllowance: 12},
	} {
		f.expect("commitmentStanding", []any{one}, binding)
		standing, err := f.adapter.CommitmentStanding(f.opts, one)
		require.NoError(t, err)
		require.Equal(t, binding.Joined, standing.Joined)
		require.False(t, standing.ClockRunning)
		require.Zero(t, standing.ClockDeadline)
		require.Equal(t, binding.ClockAllowance, standing.ClockAllowance)
		require.Zero(t, standing.Claimer, "bond recovery can clear the claimer without clearing the join record")
	}
	for _, test := range []struct {
		raw  uint8
		want model.InnerTournamentDisposition
	}{{0, model.InnerTournamentUnsettled}, {2, model.InnerTournamentEliminable}} {
		f.expect("innerResult", nil, itournament.ITournamentInnerResultView{Disposition: test.raw})
		inner, err := f.adapter.InnerResult(f.opts)
		require.NoError(t, err)
		require.Equal(t, InnerResult{Disposition: test.want}, inner)
	}
	for _, test := range []struct {
		raw  uint8
		want model.BondDisposition
	}{{0, model.BondDispositionTournamentRunning}, {1, model.BondDispositionNoWinner}, {3, model.BondDispositionRecovered}} {
		f.expect("bondRecovery", nil, test.raw, common.Address{}, big.NewInt(0))
		bond, err := f.adapter.BondRecovery(f.opts)
		require.NoError(t, err)
		require.Equal(t, BondRecovery{Disposition: test.want, Payment: big.NewInt(0)}, bond)
	}
}

func TestTournamentAdapterDecodesEveryMatchPhase(t *testing.T) {
	one, two, three := common.HexToHash("0x11"), common.HexToHash("0x22"), common.HexToHash("0x33")
	position := new(big.Int).Lsh(big.NewInt(1), 100)
	cycle := new(big.Int).Lsh(big.NewInt(1), 200)
	for _, test := range []struct {
		name    string
		phase   uint8
		outcome uint8
		charge  uint64
		method  string
		payload any
		want    ObservedMatchSnapshot
	}{
		{name: "absent", want: ObservedMatchSnapshot{Phase: model.MatchPhaseUninitialized, TimeoutOutcome: model.MatchTimeoutNone}},
		{name: "bisecting", phase: 1, outcome: 2, charge: 7, method: "bisectingMatch",
			payload: itournament.ITournamentBisectingMatchView{RevealingParent: one, WaitingLeft: two, WaitingRight: three,
				SegmentStartPosition: position, SegmentStartCycle: cycle, CurrentHeight: 4, Responder: 0},
			want: ObservedMatchSnapshot{Phase: model.MatchPhaseBisecting, TimeoutOutcome: model.MatchTimeoutTwoWins, DeferredCharge: 7,
				Bisecting: &BisectingMatch{RevealingParent: one, WaitingLeft: two, WaitingRight: three,
					SegmentStartPosition: position, SegmentStartCycle: cycle, CurrentHeight: 4, Responder: model.CommitmentSideOne}}},
		{name: "ready to seal", phase: 2, outcome: 1, charge: 8, method: "readyToSealMatch",
			payload: itournament.ITournamentReadyToSealMatchView{RevealingParent: three, WaitingLeft: two, WaitingRight: one,
				SegmentStartPosition: position, SegmentStartCycle: cycle, Responder: 1},
			want: ObservedMatchSnapshot{Phase: model.MatchPhaseReadyToSeal, TimeoutOutcome: model.MatchTimeoutOneWins, DeferredCharge: 8,
				ReadyToSeal: &ReadyToSealMatch{RevealingParent: three, WaitingLeft: two, WaitingRight: one,
					SegmentStartPosition: position, SegmentStartCycle: cycle, Responder: model.CommitmentSideTwo}}},
		{name: "sealed", phase: 3, outcome: 3, method: "sealedMatch",
			payload: itournament.ITournamentSealedMatchView{AgreeState: one, DivergencePosition: position,
				DivergenceCycle: cycle, FinalStateOne: two, FinalStateTwo: three},
			want: ObservedMatchSnapshot{Phase: model.MatchPhaseSealed, TimeoutOutcome: model.MatchTimeoutEliminateBoth,
				Sealed: &SealedMatch{AgreeState: one, DivergencePosition: position, DivergenceCycle: cycle,
					FinalStateOne: two, FinalStateTwo: three}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newTournamentViewFixture(t)
			f.expect("classifyMatchTimeout", []any{itournament.MatchId{CommitmentOne: one, CommitmentTwo: two}},
				test.phase, test.outcome, test.charge)
			if test.method != "" {
				f.expect(test.method, []any{crypto.Keccak256Hash(one[:], two[:])}, test.phase, test.payload)
			}
			snapshot, err := f.adapter.MatchSnapshot(f.opts, one, two)
			require.NoError(t, err)
			require.Equal(t, test.want, snapshot)
		})
	}
}

func TestTournamentAdapterRejectsUnknownViewEnums(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		output []any
		read   func(TournamentAdapter, *bind.CallOpts) error
	}{
		{"kind", "tournamentDescriptor", []any{itournament.ITournamentTournamentDescriptor{Kind: 2, BaseCycle: big.NewInt(0)}},
			func(a TournamentAdapter, o *bind.CallOpts) error { _, err := a.Descriptor(o); return err }},
		{"standing", "tournamentStanding", []any{itournament.ITournamentTournamentStandingView{Standing: 7}},
			func(a TournamentAdapter, o *bind.CallOpts) error { _, err := a.Standing(o); return err }},
		{"inner result", "innerResult", []any{itournament.ITournamentInnerResultView{Disposition: 3}},
			func(a TournamentAdapter, o *bind.CallOpts) error { _, err := a.InnerResult(o); return err }},
		{"bond", "bondRecovery", []any{uint8(4), common.Address{}, big.NewInt(0)},
			func(a TournamentAdapter, o *bind.CallOpts) error { _, err := a.BondRecovery(o); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newTournamentViewFixture(t)
			f.expect(test.method, nil, test.output...)
			require.ErrorContains(t, test.read(f.adapter, f.opts), "unknown")
		})
	}
}

func TestTournamentAdapterRejectsInvalidMatchViews(t *testing.T) {
	one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
	for _, test := range []struct {
		name            string
		phase           uint8
		outcome         uint8
		charge          uint64
		projectionPhase uint8
		responder       uint8
		readProjection  bool
	}{
		{name: "unknown phase", phase: 4},
		{name: "unknown timeout", phase: 1, outcome: 4},
		{name: "absent timeout", outcome: 1},
		{name: "inactive charge", phase: 1, charge: 1},
		{name: "different pinned phase", phase: 1, projectionPhase: 2, readProjection: true},
		{name: "unknown responder", phase: 1, projectionPhase: 1, responder: 2, readProjection: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newTournamentViewFixture(t)
			f.expect("classifyMatchTimeout", []any{itournament.MatchId{CommitmentOne: one, CommitmentTwo: two}},
				test.phase, test.outcome, test.charge)
			if test.readProjection {
				f.expect("bisectingMatch", []any{crypto.Keccak256Hash(one[:], two[:])}, test.projectionPhase,
					itournament.ITournamentBisectingMatchView{SegmentStartPosition: big.NewInt(0), SegmentStartCycle: big.NewInt(0),
						CurrentHeight: 2, Responder: test.responder})
			}
			snapshot, err := f.adapter.MatchSnapshot(f.opts, one, two)
			require.Error(t, err)
			require.Equal(t, ObservedMatchSnapshot{}, snapshot)
		})
	}
}

func TestTournamentReadsRejectUnpinnedSnapshots(t *testing.T) {
	for _, opts := range []*bind.CallOpts{nil, {}, {BlockNumber: big.NewInt(-1)}, {BlockNumber: big.NewInt(10), Pending: true}} {
		f := newTournamentViewFixture(t)
		_, err := f.adapter.StructuralEventCounts(opts)
		require.ErrorContains(t, err, "pinned block")
		_, err = f.adapter.MatchSnapshot(opts, [32]byte{}, [32]byte{})
		require.ErrorContains(t, err, "pinned block")
	}
}

func TestTournamentAdapterChecksEachPinnedProjectionPhase(t *testing.T) {
	one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
	for _, test := range []struct {
		phase   uint8
		method  string
		payload any
	}{
		{1, "bisectingMatch", itournament.ITournamentBisectingMatchView{
			SegmentStartPosition: big.NewInt(0), SegmentStartCycle: big.NewInt(0)}},
		{2, "readyToSealMatch", itournament.ITournamentReadyToSealMatchView{
			SegmentStartPosition: big.NewInt(0), SegmentStartCycle: big.NewInt(0)}},
		{3, "sealedMatch", itournament.ITournamentSealedMatchView{
			DivergencePosition: big.NewInt(0), DivergenceCycle: big.NewInt(0)}},
	} {
		t.Run(test.method, func(t *testing.T) {
			f := newTournamentViewFixture(t)
			f.expect("classifyMatchTimeout", []any{itournament.MatchId{CommitmentOne: one, CommitmentTwo: two}},
				test.phase, uint8(0), uint64(0))
			f.expect(test.method, []any{crypto.Keccak256Hash(one[:], two[:])}, uint8(0), test.payload)
			_, err := f.adapter.MatchSnapshot(f.opts, one, two)
			require.ErrorContains(t, err, "phase changed")
		})
	}
}

func TestTournamentReadsPropagateRPCAndScalarErrors(t *testing.T) {
	t.Run("counter read error", func(t *testing.T) {
		f := newTournamentViewFixture(t)
		f.expect("getCommitmentJoinedCount", nil, big.NewInt(1))
		f.expectBytes("getMatchCreatedCount", nil, nil, errors.New("provider unavailable"))
		counts, err := f.adapter.StructuralEventCounts(f.opts)
		require.ErrorContains(t, err, "provider unavailable")
		require.Equal(t, StructuralEventCounts{}, counts)
	})
	t.Run("uint64 overflow", func(t *testing.T) {
		f := newTournamentViewFixture(t)
		encoded, err := f.abi.Methods["innerResult"].Outputs.Pack(itournament.ITournamentInnerResultView{Disposition: 1})
		require.NoError(t, err)
		const allowanceWordStart = 64
		encoded[allowanceWordStart] = 1
		f.expectBytes("innerResult", nil, encoded, nil)
		_, err = f.adapter.InnerResult(f.opts)
		require.Error(t, err)
	})
}

func TestTournamentUint256BoundsAndOwnership(t *testing.T) {
	maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	for _, value := range []*big.Int{nil, big.NewInt(-1), new(big.Int).Add(maximum, big.NewInt(1))} {
		_, err := tournamentUint256("test", value)
		require.Error(t, err)
	}
	copiedAmount, err := tournamentUint256("test", maximum)
	require.NoError(t, err)
	require.Equal(t, maximum, copiedAmount)
	copiedAmount.SetInt64(0)
	require.NotZero(t, maximum.Sign(), "the DTO owns its integer")
}
