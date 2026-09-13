// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestResultWritesWaitForTournamentObservation(t *testing.T) {
	for _, staged := range []bool{false, true} {
		name := "opponent wins"
		if staged {
			name = "matching result is staged"
		}
		t.Run(name, func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			f.app.LastEpochCheckBlock = 99
			epoch := checkpointEpoch(0, "0x100")
			winner := *epoch.Commitment
			if !staged {
				winner = common.HexToHash("0xbad")
			}
			tournament := resultBoundaryTournament(f, epoch, winner)
			snapshot := resultTestSnapshot(epoch, staged)
			snapshot.stage.WinnerCommitment = winner
			observed := resultTestSnapshot(epoch, false)
			observed.stage.IsFinished = false
			observed.stage.WinnerCommitment = common.Hash{}
			observed.stage.WinnerPostEpochMachineStateHash = common.Hash{}
			if staged {
				snapshot.sealed.StagingBlockNumber = 100
			}
			var writeCoverage []uint64
			if staged {
				f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, epoch.Index, uint64(100)).
					Run(func(mock.Arguments) { writeCoverage = append(writeCoverage, f.app.LastTournamentCheckBlock) }).Return(nil).Once()
			} else {
				f.repo.On("UpdateApplicationStatus", mock.Anything, f.app.ID, model.ApplicationStatus_Diverged, mock.Anything).
					Run(func(mock.Arguments) { writeCoverage = append(writeCoverage, f.app.LastTournamentCheckBlock) }).Return(nil).Once()
			}
			f.expectResultBoundaryWindow(epoch, tournament, 99)
			f.expectResultBoundarySnapshot(epoch, observed, 99)

			require.NoError(t, f.s.validateApplication(t.Context(), f.app, 100))
			require.Equal(t, uint64(99), f.app.LastTournamentCheckBlock, "the older window must still publish successfully")
			require.Equal(t, uint64(99), tournament.Snapshot.AsOfBlock)
			require.Equal(t, model.TournamentStandingAwaitingClosure, tournament.Snapshot.Standing)
			require.Empty(t, writeCoverage, "a newer configured result must not change persistent local state")
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
			require.Nil(t, epoch.StagedAtBlock)

			// No-work at the same observation boundary is not evidence that the
			// configured result has become durable.
			f.epochs(epoch)
			f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), tournament.Address.Hex()).
				Return(tournament, nil).Once()
			f.expectResultBoundarySnapshot(epoch, observed, 99)
			require.NoError(t, f.s.validateApplication(t.Context(), f.app, 100))
			require.Empty(t, writeCoverage)

			f.app.LastEpochCheckBlock = 100
			f.expectResultBoundaryWindow(epoch, tournament, 100)
			if staged {
				f.expectResultBoundarySnapshot(epoch, snapshot, 100)
			}
			err := f.s.validateApplication(t.Context(), f.app, 100)
			if staged {
				require.NoError(t, err)
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
				require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
				require.Equal(t, new(uint64(100)), epoch.StagedAtBlock)
			} else {
				require.ErrorContains(t, err, "Epoch 0 has inconsistent commitment")
				require.Equal(t, model.ApplicationStatus_Diverged, f.app.Status)
			}
			require.Equal(t, []uint64{100}, writeCoverage, "the complete result window must precede the local write")
			require.Equal(t, uint64(100), tournament.Snapshot.AsOfBlock)
			require.Equal(t, &winner, tournament.Snapshot.WinnerCommitment)
			require.Empty(t, f.s.pendingTransactions, "reader mode must remain passive")
			f.client.AssertNotCalled(t, "BlockNumber", mock.Anything)
		})
	}
}

func resultBoundaryTournament(f *observerCheckpointFixture, epoch *model.Epoch, winner common.Hash) *model.Tournament {
	return &model.Tournament{
		ApplicationID: f.app.ID, EpochIndex: epoch.Index, Address: *epoch.TournamentAddress,
		MaxLevel: 1, Height: 1, Kind: model.TournamentKindLeaf, StartInstant: epoch.LastBlock, Allowance: 90,
		Snapshot: model.TournamentSnapshot{AsOfBlock: f.app.LastTournamentCheckBlock,
			Standing: model.TournamentStandingAwaitingClosure, AcceptsJoins: true, Candidate: &winner,
			BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionTournamentRunning}},
	}
}

func (f *observerCheckpointFixture) expectResultBoundarySnapshot(epoch *model.Epoch, snapshot daveConsensusSnapshot, head uint64) {
	f.t.Helper()
	opts := mock.MatchedBy(resultCallOptsAtBlock(head))
	f.consensus.On("GetCurrentSealedEpoch", opts).Return(snapshot.sealed, nil).Once()
	f.consensus.On("CanStageTournamentResult", opts).Return(snapshot.stage, nil).Once()
	f.consensus.On("CanAcceptStagedTournamentResult", opts).Return(snapshot.accept, nil).Once()
	f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.Index).Return(epoch, nil).Once()
}

func (f *observerCheckpointFixture) expectResultBoundaryWindow(epoch *model.Epoch, tournament *model.Tournament, head uint64) {
	f.t.Helper()
	f.epochs(epoch)
	opts := mock.MatchedBy(resultCallOptsAtBlock(head))
	f.consensus.On("TournamentLevelCount", opts).Return(uint64(1), nil).Once()
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), tournament.Address.Hex()).
		Return(tournament, nil).Twice() // observation load, then local reconciliation
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", tournament.Address).Return(adapter, nil).Once()
	adapter.On("Descriptor", opts).Return(TournamentDescriptor{BaseCycle: new(big.Int), Height: tournament.Height,
		Kind: tournament.Kind, StartInstant: tournament.StartInstant, Allowance: tournament.Allowance}, nil).Once()
	standing := TournamentStanding{State: model.TournamentStandingAwaitingClosure, AcceptsJoins: true,
		HasCandidate: true, Candidate: *tournament.Snapshot.Candidate}
	if head >= 100 {
		standing.State, standing.AcceptsJoins = model.TournamentStandingRootWinner, false
		standing.FinalState, standing.FinishedAt = *epoch.MachineHash, 100
	}
	adapter.On("Standing", opts).Return(standing, nil).Once()
	expectTournamentAuxiliaryReads(adapter, opts, RootLevel, standing.State)
	previous := f.app.LastTournamentCheckBlock
	adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == previous+1 && opts.End != nil && *opts.End == head
	})).Return(&TournamentEvents{}, nil).Once()
	counts := zeroStructuralEventCounts()
	counts.CommitmentJoined.SetUint64(1)
	adapter.On("StructuralEventCounts", mock.MatchedBy(resultCallOptsAtBlock(previous))).Return(counts, nil).Once()
	adapter.On("StructuralEventCounts", opts).Return(counts, nil).Once()
	adapter.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(previous))).
		Return(canonicalBondRecovery(tournament.Snapshot.BondRecovery.Disposition, common.Address{}, 0), nil).Once()
	address := tournament.Address.Hex()
	commitment := &model.Commitment{ApplicationID: f.app.ID, EpochIndex: epoch.Index, TournamentAddress: tournament.Address,
		Commitment: standing.Candidate, FinalStateHash: *epoch.MachineHash, BlockNumber: 20}
	f.repo.On("ListCommitments", mock.Anything, f.app.Name,
		repository.CommitmentFilter{EpochIndex: &epoch.Index, TournamentAddress: &address}, repository.Pagination{}, false).
		Return([]*model.Commitment{commitment}, uint64(1), nil).Once()
	f.repo.On("ListMatches", mock.Anything, f.app.Name,
		repository.MatchFilter{EpochIndex: &epoch.Index, TournamentAddress: &address}, repository.Pagination{}, false).
		Return([]*model.Match{}, uint64(0), nil).Once()
	adapter.On("CommitmentStanding", opts, [32]byte(commitment.Commitment)).
		Return(CommitmentStanding{Joined: true, FinalState: commitment.FinalStateHash, ClockAllowance: 80}, nil).Once()
	statusBeforePublication := epoch.Status
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, head).
		Run(func(args mock.Arguments) {
			batches := args.Get(2).([]*repository.TournamentEventBatch)
			require.Len(f.t, batches, 1)
			require.Equal(f.t, model.ApplicationStatus_OK, f.app.Status)
			require.Equal(f.t, statusBeforePublication, epoch.Status)
			*tournament = *batches[0].Tournament
		}).Return(nil).Once()
	f.t.Cleanup(func() { adapter.AssertExpectations(f.t) })
}

func TestObserverLagDoesNotBlockLatestStageAction(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, true)
	f.app.LastTournamentCheckBlock, f.app.LastEpochCheckBlock = 50, 99
	f.epoch.LastBlock = 10
	observer := &observerCheckpointFixture{t: t, s: f.s, repo: f.repo, factory: f.factory,
		consensus: f.consensus, client: f.client, app: f.app}
	tournament := resultBoundaryTournament(observer, f.epoch, *f.epoch.Commitment)
	observer.expectResultBoundaryWindow(f.epoch, tournament, 99)
	observed := resultTestSnapshot(f.epoch, false)
	observed.stage.IsFinished = false
	observed.stage.WinnerCommitment = common.Hash{}
	observed.stage.WinnerPostEpochMachineStateHash = common.Hash{}
	latest := resultTestSnapshot(f.epoch, false)
	f.client.On("BlockNumber", mock.Anything).Return(uint64(120), nil).Once()
	f.expectSnapshot(99, observed)
	f.expectSnapshot(120, latest)
	tx := types.NewTx(&types.LegacyTx{Nonce: 1})
	f.consensus.On("StageTournamentResult", mock.Anything, f.epoch.Index, mock.Anything).Return(tx, nil).Once()

	require.NoError(t, f.s.validateApplication(t.Context(), f.app, 100))
	require.Equal(t, tx.Hash(), f.s.pendingTransactions[f.app.ID].Hash)
	require.Equal(t, uint64(99), f.app.LastTournamentCheckBlock)
	require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
	f.assertNoStageWrite(t)
	f.assertNoPermanentStatusWrite(t)
}

func TestAcceptanceWaitsForTournamentObservation(t *testing.T) {
	s, repo := newPRTServiceMock()
	app := prtRevertTestApp()
	app.LastTournamentCheckBlock = 99
	epoch := checkpointEpoch(0, "0x100")
	epoch.ClaimTransactionHash = new(common.HexToHash("0x900"))
	tournament := &model.Tournament{Snapshot: model.TournamentSnapshot{
		AsOfBlock: 99, FinishedAtBlock: 90, WinnerCommitment: epoch.Commitment,
	}}
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
		Return(tournament, nil).Twice()
	raw := types.Log{Address: app.IConsensusAddress, TxHash: *epoch.ClaimTransactionHash, BlockNumber: 100}
	receipt := &types.Receipt{TxHash: raw.TxHash, BlockNumber: big.NewInt(100),
		Status: types.ReceiptStatusSuccessful, Logs: []*types.Log{&raw}}
	client := &ethClientMock{}
	client.On("TransactionReceipt", mock.Anything, raw.TxHash).Return(receipt, nil).Twice()
	s.client = client
	consensus := &daveConsensusAdapterMock{}
	consensus.On("ParseEpochSealed", raw).Return(&idaveconsensus.IDaveConsensusEpochSealed{
		EpochNumber: big.NewInt(1), InitialMachineStateHash: *epoch.MachineHash,
		OutputsMerkleRoot: *epoch.TxBufferDataBlock, Raw: raw,
	}, nil).Once()
	var writeCoverage []uint64
	repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, epoch.ClaimTransactionHash).
		Run(func(mock.Arguments) { writeCoverage = append(writeCoverage, app.LastTournamentCheckBlock) }).Return(nil).Once()

	deferActions, err := s.reconcileAcceptedEpochs(t.Context(), app, []*model.Epoch{epoch}, consensus, 100)
	require.NoError(t, err)
	require.True(t, deferActions, "the acceptance receipt is newer than the published tournament window")
	require.Empty(t, writeCoverage)
	consensus.AssertNotCalled(t, "ParseEpochSealed", mock.Anything)

	app.LastTournamentCheckBlock = 100
	tournament.Snapshot.AsOfBlock = 100
	deferActions, err = s.reconcileAcceptedEpochs(t.Context(), app, []*model.Epoch{epoch}, consensus, 100)
	require.NoError(t, err)
	require.False(t, deferActions)
	require.Equal(t, []uint64{100}, writeCoverage)
	repo.AssertExpectations(t)
	client.AssertExpectations(t)
	consensus.AssertExpectations(t)
}
