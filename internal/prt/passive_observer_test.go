// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPassiveObservationIncludesLocalFailuresAndTerminalEpochs(t *testing.T) {
	for _, status := range []ApplicationStatus{
		ApplicationStatus_OK, ApplicationStatus_Failed, ApplicationStatus_Corrupted, ApplicationStatus_Diverged,
	} {
		for _, epochStatus := range []EpochStatus{
			EpochStatus_Closed, EpochStatus_InputsProcessed, EpochStatus_ClaimAccepted, EpochStatus_ClaimForeclosed,
		} {
			t.Run(status.String()+"/"+epochStatus.String(), func(t *testing.T) {
				f := newObserverCheckpointFixture(t)
				f.s.Context, f.s.defaultBlock = t.Context(), DefaultBlock_Finalized
				f.app.Status = status
				// These applications must remain passive even with signing enabled.
				f.s.submissionEnabled = status != ApplicationStatus_OK
				epoch := checkpointEpoch(0, "0x100")
				snapshot := resultTestSnapshot(epoch, false)
				epoch.Status = epochStatus
				epoch.Commitment, epoch.MachineHash, epoch.TxBufferDataBlock = nil, nil, nil
				f.repo.On("ListApplications", mock.Anything, repository.ApplicationFilter{
					Enabled: new(true), ConsensusType: new(Consensus_PRT),
				}, repository.Pagination{}, false).Return([]*Application{f.app}, uint64(1), nil).Once()
				f.client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
					Return(&types.Header{Number: big.NewInt(100)}, nil).Once()
				f.epochs(epoch)
				f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
				f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
				f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID,
					mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool {
						return len(batches) == 1 && batches[0].Tournament.Snapshot.AsOfBlock == 100
					}), uint64(100)).Return(nil).Once()
				if status == ApplicationStatus_OK {
					opts := mock.MatchedBy(resultCallOptsAtBlock(100))
					f.consensus.On("GetCurrentSealedEpoch", opts).Return(snapshot.sealed, nil).Once()
					f.consensus.On("CanStageTournamentResult", opts).Return(snapshot.stage, nil).Once()
					f.consensus.On("CanAcceptStagedTournamentResult", opts).Return(snapshot.accept, nil).Once()
					f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.Index).Return(epoch, nil).Once()
				}
				require.Empty(t, f.s.Tick())
				require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
				require.Equal(t, status, f.app.Status)
				require.Empty(t, f.s.pendingTransactions)
				f.client.AssertNotCalled(t, "BlockNumber", mock.Anything)
				f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	}
}

func TestLosingRootPublishesBeforeLocalDivergence(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	foreign := common.HexToHash("0xbad")
	address := *epoch.TournamentAddress
	f.epochs(epoch)
	opts := mock.MatchedBy(resultCallOptsAtBlock(100))
	f.consensus.On("TournamentLevelCount", opts).Return(uint64(1), nil).Once()
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).Return((*Tournament)(nil), nil).Once()
	adapter.On("Descriptor", opts).Return(TournamentDescriptor{
		BaseCycle: big.NewInt(0), Kind: TournamentKindLeaf, StartInstant: 10,
	}, nil).Once()
	adapter.On("Standing", opts).Return(TournamentStanding{
		State: TournamentStandingRootWinner, HasCandidate: true, Candidate: foreign, FinishedAt: 90,
	}, nil).Once()
	expectTournamentAuxiliaryReads(adapter, opts, RootLevel, TournamentStandingRootWinner)
	adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		// A newly found clone includes its full history, even below the app cursor.
		return opts.Start == 10 && opts.End != nil && *opts.End == 100
	})).Return(&TournamentEvents{}, nil).Once()
	adapter.On("StructuralEventCounts", opts).Return(zeroStructuralEventCounts(), nil).Once()
	f.emptyParticipants(epoch, address)
	var published *Tournament
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, uint64(100)).
		Run(func(args mock.Arguments) {
			batches := args.Get(2).([]*repository.TournamentEventBatch)
			require.Len(t, batches, 1)
			published = batches[0].Tournament
			require.Equal(t, foreign, *published.Snapshot.WinnerCommitment)
			require.Equal(t, ApplicationStatus_OK, f.app.Status)
		}).Return(nil).Once()
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).
		Return(&Tournament{Snapshot: TournamentSnapshot{FinishedAtBlock: 90, WinnerCommitment: &foreign}}, nil).Once()
	f.repo.On("UpdateApplicationStatus", mock.Anything, f.app.ID, ApplicationStatus_Diverged, mock.Anything).
		Run(func(mock.Arguments) {
			require.NotNil(t, published)
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
		}).Return(nil).Once()
	deferActions, err := f.s.checkEpochs(t.Context(), f.app, 100)
	require.ErrorContains(t, err, "inconsistent commitment")
	require.True(t, deferActions)
	require.Equal(t, ApplicationStatus_Diverged, f.app.Status)
	require.Empty(t, f.s.pendingTransactions)
	adapter.AssertExpectations(t)
}
