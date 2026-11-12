// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const joinClosedBeforeInclusion = "closed before inclusion"

type joinInclusionFixture struct {
	*deliveryFixture
	adapter          *tournamentAdapterMock
	descriptor       TournamentDescriptor
	events           *TournamentEvents
	joined           CommitmentStanding
	priorCommitments []*model.Commitment
	priorMatches     []*model.Match
}

func newJoinInclusionFixture(t *testing.T) *joinInclusionFixture {
	t.Helper()
	f := newDeliveryFixture(t)
	f.epoch.LastBlock = 100
	f.d.defaultBlock = model.DefaultBlock_Latest
	client := &ethClientMock{}
	client.On("HeaderByNumber", mock.Anything, mock.Anything).Return(func(_ context.Context, number *big.Int) *types.Header {
		if number == nil || number.Sign() < 0 {
			return f.backend.headers[f.backend.latest]
		}
		return f.backend.headers[number.Uint64()]
	}, nil)
	f.d.client = client
	descriptor := TournamentDescriptor{BaseCycle: new(big.Int), Kind: model.TournamentKindNonLeaf,
		Height: model.Log2EpochComputationHashLeafCount, Log2Stride: model.Log2RootTournamentStride,
		StartInstant: 100, Allowance: 50}
	f.repo.publishedRoots[f.app.IApplicationAddress.Hex()+f.epoch.TournamentAddress.Hex()] = &model.Tournament{
		ApplicationID: f.app.ID, EpochIndex: f.epoch.Index, Address: *f.epoch.TournamentAddress,
		MaxLevel: 2, Kind: descriptor.Kind, Height: descriptor.Height, Log2Step: descriptor.Log2Stride,
		StartInstant: descriptor.StartInstant, Allowance: descriptor.Allowance,
		Snapshot: model.TournamentSnapshot{AsOfBlock: 100},
	}
	f.snapshot(daveConsensusSnapshotForJoin(f.epoch))
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, mock.Anything).Return((*uint64)(nil), nil)
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(adapter, nil)
	return &joinInclusionFixture{deliveryFixture: f, adapter: adapter, descriptor: descriptor, events: &TournamentEvents{}}
}

func daveConsensusSnapshotForJoin(epoch *model.Epoch) daveConsensusSnapshot {
	view := resultTestSnapshot(epoch, false)
	view.stage.IsFinished = false
	return view
}

func (f *joinInclusionFixture) views(acceptsJoins bool) {
	f.adapter.ExpectedCalls = nil
	f.adapter.On("Descriptor", mock.Anything).Return(f.descriptor, nil)
	standing := TournamentStanding{State: model.TournamentStandingAwaitingClosure, AcceptsJoins: acceptsJoins}
	if len(f.priorMatches) != 0 {
		// Other participants keep their match active after the join window closes.
		standing.State = model.TournamentStandingMatchesActive
	}
	if f.joined.Joined {
		standing.HasCandidate, standing.Candidate = true, *f.epoch.Commitment
	}
	f.adapter.On("Standing", mock.Anything).Return(standing, nil)
	f.adapter.On("CommitmentStanding", mock.Anything, [32]byte(*f.epoch.Commitment)).Return(f.joined, nil)
	f.adapter.On("BondValue", mock.Anything).Return(big.NewInt(50), nil)
	f.adapter.On("BondRecovery", mock.Anything).
		Return(canonicalBondRecovery(model.BondDispositionTournamentRunning, common.Address{}, 0), nil)
	counts := structuralCountsForEvents(f.events)
	counts.CommitmentJoined.Add(counts.CommitmentJoined, new(big.Int).SetUint64(uint64(len(f.priorCommitments))))
	counts.MatchCreated.Add(counts.MatchCreated, new(big.Int).SetUint64(uint64(len(f.priorMatches))))
	f.adapter.On("StructuralEventCounts", mock.Anything).Return(counts, nil)
	for _, commitment := range f.priorCommitments {
		f.adapter.On("CommitmentStanding", mock.Anything, [32]byte(commitment.Commitment)).Return(CommitmentStanding{
			Joined: true, FinalState: commitment.FinalStateHash, Claimer: commitment.SubmitterAddress,
		}, nil)
	}
	for _, match := range f.priorMatches {
		f.adapter.On("MatchSnapshot", mock.Anything, [32]byte(match.CommitmentOne), [32]byte(match.CommitmentTwo)).
			Return(ObservedMatchSnapshot{Phase: model.MatchPhaseBisecting, TimeoutOutcome: model.MatchTimeoutTwoWins,
				Bisecting: &BisectingMatch{SegmentStartPosition: new(big.Int), SegmentStartCycle: new(big.Int),
					CurrentHeight: f.descriptor.Height - 1, Responder: model.CommitmentSideOne}}, nil)
	}
	f.adapter.On("RetrieveAllEvents", mock.Anything, mock.Anything).Return(f.events, nil)
}

// These joins and their match were already committed at block 100. The
// match can remain active after its running clock and the join window expire.
func (f *joinInclusionFixture) activeForeignMatch() {
	for _, hash := range []string{"0x601", "0x701"} {
		commitment := common.HexToHash(hash)
		f.priorCommitments = append(f.priorCommitments, &model.Commitment{
			ApplicationID: f.app.ID, EpochIndex: f.epoch.Index, TournamentAddress: *f.epoch.TournamentAddress,
			Commitment: commitment, FinalStateHash: commitment, SubmitterAddress: common.BytesToAddress(commitment[:]),
			BlockNumber: f.descriptor.StartInstant,
		})
	}
	f.priorMatches = []*model.Match{{ApplicationID: f.app.ID, EpochIndex: f.epoch.Index,
		TournamentAddress: *f.epoch.TournamentAddress, IDHash: common.HexToHash("0x801"),
		CommitmentOne: f.priorCommitments[0].Commitment, CommitmentTwo: f.priorCommitments[1].Commitment,
		BlockNumber: f.descriptor.StartInstant, DeletionReason: model.MatchDeletionReason_NOT_DELETED,
		ResponderDeadline: f.descriptor.StartInstant + f.descriptor.Allowance,
		EliminableAt:      f.descriptor.StartInstant + f.descriptor.Allowance}}
	f.repo.eventCounts = map[common.Address]repository.TournamentEventCounts{
		*f.epoch.TournamentAddress: {CommitmentJoined: 2, MatchCreated: 1, StandingChanged: 2},
	}
}

func (f *joinInclusionFixture) emitJoin(submitter common.Address, txHash common.Hash, block uint64) *types.Log {
	log := types.Log{Address: *f.epoch.TournamentAddress, TxHash: txHash, BlockNumber: block, BlockHash: f.block.Hash}
	f.events.CommitmentJoined = append(f.events.CommitmentJoined, &itournament.ITournamentCommitmentJoined{
		Commitment: *f.epoch.Commitment, FinalStateHash: *f.epoch.MachineHash, Submitter: submitter, Raw: log,
	})
	standingLog := log
	standingLog.Index++
	f.events.StandingChanged = append(f.events.StandingChanged, &itournament.ITournamentStandingChanged{
		MatchCount: new(big.Int), Dangling: *f.epoch.Commitment,
		ResultAt: f.descriptor.StartInstant + f.descriptor.Allowance, Raw: standingLog,
	})
	f.joined = CommitmentStanding{Joined: true, FinalState: *f.epoch.MachineHash, Claimer: submitter}
	return &log
}

// mine consumes the published nonce in the chain double. Only successful
// execution emits a join. The node never receives this synthetic receipt.
func (f *joinInclusionFixture) mine(t *testing.T, block, status uint64) *types.Receipt {
	t.Helper()
	require.Len(t, f.backend.published, 1)
	tx := f.backend.published[0]
	require.Equal(t, f.backend.nonce, tx.Nonce())
	f.backend.nonce++
	updatePRTView(f.deliveryFixture, block, daveConsensusSnapshotForJoin(f.epoch))
	receipt := &types.Receipt{TxHash: tx.Hash(), Status: status, BlockNumber: new(big.Int).SetUint64(block), BlockHash: f.block.Hash}
	if status == types.ReceiptStatusSuccessful {
		joinLog := f.emitJoin(f.d.txOptsFactory.From(), tx.Hash(), block)
		receipt.Logs = []*types.Log{joinLog, &f.events.StandingChanged[0].Raw}
	}
	return receipt
}

func TestPRTIncludedJoinReplansFromObservedFacts(t *testing.T) {
	for _, outcome := range []string{"still eligible", joinClosedBeforeInclusion, "peer joined first", "successful join control"} {
		t.Run(outcome, func(t *testing.T) {
			f := newJoinInclusionFixture(t)
			if outcome == joinClosedBeforeInclusion {
				f.activeForeignMatch()
			}
			f.views(true)
			require.NoError(t, f.d.tick(t.Context()))
			require.Len(t, f.backend.published, 1)
			original := f.backend.published[0]
			require.Equal(t, uint64(7), original.Nonce())
			status, block := uint64(types.ReceiptStatusFailed), uint64(111)
			if outcome == joinClosedBeforeInclusion {
				block = f.descriptor.StartInstant + f.descriptor.Allowance
			}
			if outcome == "successful join control" {
				status = types.ReceiptStatusSuccessful
			}
			receipt := f.mine(t, block, status)
			peer := common.HexToAddress("0x900")
			peerTxHash := common.HexToHash("0x901")
			if outcome == "peer joined first" {
				f.emitJoin(peer, peerTxHash, block)
			}
			f.views(outcome != joinClosedBeforeInclusion)
			f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(2), nil)
			f.repo.On("ListTournaments", mock.Anything, f.app.Name, repository.TournamentFilter{}, repository.Pagination{}, false).
				Return([]*model.Tournament{}, uint64(0), nil)
			f.repo.On("ListCommitments", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
				Return(f.priorCommitments, uint64(len(f.priorCommitments)), nil).Once()
			f.repo.On("ListMatches", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
				Return(f.priorMatches, uint64(len(f.priorMatches)), nil).Once()
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			require.NoError(t, f.d.observeWindow(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, f.block))
			require.Len(t, f.repo.windows, 1, "exercise a committed observation after inclusion")
			require.Len(t, f.repo.windows[0].Batches, 1)
			batch := f.repo.windows[0].Batches[0]
			if !f.joined.Joined {
				require.Len(t, batch.Commitments, len(f.priorCommitments),
					"without a join event, observation must preserve prior commitments")
				for _, commitment := range batch.Commitments {
					require.NotEqual(t, *f.epoch.Commitment, commitment.Commitment)
					require.NotEqual(t, f.d.txOptsFactory.From(), commitment.SubmitterAddress)
				}
			} else {
				require.Len(t, batch.Commitments, 1, "the same observer must store a real join event")
				if status == types.ReceiptStatusFailed {
					require.Equal(t, peer, batch.Commitments[0].SubmitterAddress)
					require.Equal(t, peerTxHash, batch.Commitments[0].TxHash)
				} else {
					require.Equal(t, f.d.txOptsFactory.From(), batch.Commitments[0].SubmitterAddress)
					require.Equal(t, receipt.TxHash, batch.Commitments[0].TxHash)
				}
			}
			require.NoError(t, f.d.tick(t.Context()))
			if outcome == "still eligible" {
				require.Len(t, f.backend.published, 2)
				retried := f.backend.published[1]
				require.Equal(t, original.Nonce()+1, retried.Nonce())
				require.Equal(t, original.To(), retried.To())
				require.Equal(t, original.Data(), retried.Data())
				require.Equal(t, original.Value(), retried.Value())
			} else {
				require.Len(t, f.backend.published, 1, "current facts must stop a redundant join")
			}
			if outcome == joinClosedBeforeInclusion {
				require.Contains(t, logs.String(), "level=WARN")
				require.Contains(t, logs.String(), "PRT cannot join tournament")
				require.Contains(t, logs.String(), "join deadline leaves no block")
			} else {
				require.NotContains(t, logs.String(), "PRT cannot join tournament")
			}
			require.NotContains(t, logs.String(), "level=ERROR")
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
			require.Nil(t, f.epoch.ClaimTransactionHash)
			require.False(t, f.d.publicationStopped())
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.repo.AssertExpectations(t)
		})
	}
}
