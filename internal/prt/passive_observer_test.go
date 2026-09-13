// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
)

func TestPassiveObservationIncludesLocalFailuresAndTerminalEpochs(t *testing.T) {
	for _, status := range model.ApplicationStatusAllValues {
		for _, epochStatus := range []model.EpochStatus{
			model.EpochStatus_Closed, model.EpochStatus_InputsProcessed, model.EpochStatus_ClaimComputed,
			model.EpochStatus_ClaimStaged, model.EpochStatus_ClaimAccepted, model.EpochStatus_ClaimForeclosed,
		} {
			t.Run(status.String()+"/"+epochStatus.String(), func(t *testing.T) {
				f := newObserverCheckpointFixture(t)
				f.s.defaultBlock = model.DefaultBlock_Finalized
				f.app.Status = status
				// These applications must remain passive even with signing enabled.
				f.s.submissionEnabled = status != model.ApplicationStatus_OK
				epoch := checkpointEpoch(0, "0x100")
				epoch.Status = epochStatus
				pendingClaim := epochStatus == model.EpochStatus_ClaimComputed || epochStatus == model.EpochStatus_ClaimStaged
				if pendingClaim {
					// A complete claim ensures that invalid local data cannot be
					// the reason an unhealthy application avoids submission.
					applyPRTStateProof(epoch, repotest.KeccakStateProof(common.HexToHash("0x1234")))
					_, err := epoch.StateProof()
					require.NoError(t, err)
				}
				snapshot := resultTestSnapshot(epoch, epochStatus == model.EpochStatus_ClaimStaged)
				if epochStatus == model.EpochStatus_ClaimStaged {
					epoch.StagedAtBlock = new(uint64(95))
					snapshot.sealed.StagingBlockNumber = *epoch.StagedAtBlock
					snapshot.accept.IsClaimStagingPeriodOver = true
				}
				if !pendingClaim {
					epoch.Commitment, epoch.MachineHash, epoch.TxBufferDataBlock = nil, nil, nil
				}
				f.repo.On("ListApplications", mock.Anything, repository.ApplicationFilter{
					Enabled: new(true), ConsensusType: new(model.Consensus_PRT),
				}, repository.Pagination{}, false).Return([]*model.Application{f.app}, uint64(1), nil).Once()
				f.client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
					Return(&types.Header{Number: big.NewInt(100)}, nil).Once()
				f.epochs(epoch)
				f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
				f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
				f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID,
					mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool {
						return len(batches) == 1 && batches[0].Tournament.Snapshot.AsOfBlock == 100
					}), uint64(100)).Return(nil).Once()
				if status == model.ApplicationStatus_OK {
					if pendingClaim {
						f.unaccepted(epoch, 90)
					}
					opts := mock.MatchedBy(resultCallOptsAtBlock(100))
					f.consensus.On("GetCurrentSealedEpoch", opts).Return(snapshot.sealed, nil).Once()
					f.consensus.On("CanStageTournamentResult", opts).Return(snapshot.stage, nil).Once()
					f.consensus.On("CanAcceptStagedTournamentResult", opts).Return(snapshot.accept, nil).Once()
					f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.Index).Return(epoch, nil).Once()
				}
				reschedule, err := f.s.Tick(t.Context())
				require.False(t, reschedule)
				require.NoError(t, err)
				require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
				require.Equal(t, status, f.app.Status)
				require.Equal(t, epochStatus, epoch.Status)
				require.Empty(t, f.s.pendingTransactions)
				f.client.AssertNotCalled(t, "BlockNumber", mock.Anything)
				f.consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
				f.consensus.AssertNotCalled(t, "AcceptStagedTournamentResult", mock.Anything, mock.Anything)
				f.repo.AssertNotCalled(t, "UpdateEpochReconciledStaged", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				f.repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				if status != model.ApplicationStatus_OK {
					// Tick must stop after observation, before any result-action
					// query. An unexpected broadcast also fails the strict mocks.
					f.consensus.AssertNotCalled(t, "GetCurrentSealedEpoch", mock.Anything)
					f.consensus.AssertNotCalled(t, "CanStageTournamentResult", mock.Anything)
					f.consensus.AssertNotCalled(t, "CanAcceptStagedTournamentResult", mock.Anything)
				}
			})
		}
	}
}

func TestHealthyReaderPublishesBeforeRecordingStagedClaim(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.s.defaultBlock = model.DefaultBlock_Finalized
	f.s.submissionEnabled = false
	epoch := checkpointEpoch(0, "0x100")
	snapshot := resultTestSnapshot(epoch, true)
	snapshot.sealed.StagingBlockNumber = 95
	snapshot.accept.IsClaimStagingPeriodOver = true
	f.repo.On("ListApplications", mock.Anything, repository.ApplicationFilter{
		Enabled: new(true), ConsensusType: new(model.Consensus_PRT),
	}, repository.Pagination{}, false).Return([]*model.Application{f.app}, uint64(1), nil).Once()
	f.client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Once()
	f.epochs(epoch)
	opts := mock.MatchedBy(resultCallOptsAtBlock(100))
	f.consensus.On("TournamentLevelCount", opts).Return(uint64(1), nil).Once()
	f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID,
		mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool {
			return len(batches) == 1 && batches[0].Tournament.Snapshot.AsOfBlock == 100
		}), uint64(100)).Return(nil).Once()
	f.unaccepted(epoch, 90)
	f.consensus.On("GetCurrentSealedEpoch", opts).Return(snapshot.sealed, nil).Once()
	f.consensus.On("CanStageTournamentResult", opts).Return(snapshot.stage, nil).Once()
	f.consensus.On("CanAcceptStagedTournamentResult", opts).Return(snapshot.accept, nil).Once()
	f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.Index).Return(epoch, nil).Once()
	f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, epoch.Index, uint64(95)).
		Run(func(mock.Arguments) { require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock) }).Return(nil).Once()

	reschedule, err := f.s.Tick(t.Context())
	require.False(t, reschedule)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
	require.Equal(t, new(uint64(95)), epoch.StagedAtBlock)
	require.Empty(t, f.s.pendingTransactions)
	f.client.AssertNotCalled(t, "BlockNumber", mock.Anything)
	f.consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
	f.consensus.AssertNotCalled(t, "AcceptStagedTournamentResult", mock.Anything, mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
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
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).Return((*model.Tournament)(nil), nil).Once()
	adapter.On("Descriptor", opts).Return(TournamentDescriptor{
		BaseCycle: big.NewInt(0), Kind: model.TournamentKindLeaf, StartInstant: 10,
	}, nil).Once()
	adapter.On("Standing", opts).Return(TournamentStanding{
		State: model.TournamentStandingRootWinner, HasCandidate: true, Candidate: foreign, FinishedAt: 90,
	}, nil).Once()
	expectTournamentAuxiliaryReads(adapter, opts, RootLevel, model.TournamentStandingRootWinner)
	adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		// A newly found clone includes its full history, even below the app cursor.
		return opts.Start == 10 && opts.End != nil && *opts.End == 100
	})).Return(&TournamentEvents{}, nil).Once()
	adapter.On("StructuralEventCounts", opts).Return(zeroStructuralEventCounts(), nil).Once()
	f.emptyParticipants(epoch, address)
	var published *model.Tournament
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, uint64(100)).
		Run(func(args mock.Arguments) {
			batches := args.Get(2).([]*repository.TournamentEventBatch)
			require.Len(t, batches, 1)
			published = batches[0].Tournament
			require.Equal(t, foreign, *published.Snapshot.WinnerCommitment)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
		}).Return(nil).Once()
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).
		Return(&model.Tournament{Snapshot: model.TournamentSnapshot{FinishedAtBlock: 90, WinnerCommitment: &foreign}}, nil).Once()
	f.repo.On("UpdateApplicationStatus", mock.Anything, f.app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Run(func(mock.Arguments) {
			require.NotNil(t, published)
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
		}).Return(nil).Once()
	deferActions, err := f.s.checkEpochs(t.Context(), f.app, 100)
	require.ErrorContains(t, err, "inconsistent commitment")
	require.True(t, deferActions)
	require.Equal(t, model.ApplicationStatus_Diverged, f.app.Status)
	require.Empty(t, f.s.pendingTransactions)
	adapter.AssertExpectations(t)
}
