// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewRootRereadsDescriptorUntilCompleteIntervalCommits(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(fmt.Sprint(ambiguous), func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			epoch := checkpointEpoch(0, "0xa0")
			epoch.Status = model.EpochStatus_Closed
			root := *epoch.TournamentAddress
			cause := errors.New("publication acknowledgment unavailable")
			turns := 2
			if ambiguous {
				turns = 1
			}
			f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(1), nil).Times(turns)
			f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(),
				root.Hex()).Return((*model.Tournament)(nil), nil).Times(turns)
			for turn := 0; turn < turns; turn++ {
				d := TournamentDescriptor{BaseCycle: big.NewInt(0), Kind: model.TournamentKindLeaf, Height: 68,
					StartInstant: 10, Allowance: 20}
				adapter := &tournamentAdapterMock{}
				f.factory.On("CreateTournamentAdapter", root).Return(adapter, nil).Once()
				adapter.On("Descriptor", mock.Anything).Return(d, nil).Once()
				adapter.On("BondValue", mock.Anything).Return(big.NewInt(1), nil).Once()
				adapter.On("Standing", mock.Anything).Return(TournamentStanding{State: model.TournamentStandingRootFailed,
					FinishedAt: 30}, nil).Once()
				expectTournamentAuxiliaryReads(adapter, mock.Anything, RootLevel, model.TournamentStandingRootFailed)
				adapter.On("RetrieveAllEvents", mock.Anything).Return(&TournamentEvents{}, nil).Once()
				adapter.On("StructuralEventCounts", mock.Anything).Return(zeroStructuralEventCounts(), nil).Once()
				f.emptyParticipants(epoch, root)
				var writeErr error
				if turn == 0 {
					writeErr = cause
				}
				call := f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(writeErr).Once()
				if ambiguous {
					call.Run(func(args mock.Arguments) {
						row := args.Get(2).([]*repository.TournamentEventBatch)[0].Tournament
						f.repo.storedTournaments = []*model.Tournament{row}
					})
				}
				t.Cleanup(func() { adapter.AssertExpectations(t) })
			}
			err := f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, observerFacts(100))
			require.ErrorIs(t, err, cause)
			require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
			require.NoError(t, f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, observerFacts(100)))
			if !ambiguous {
				require.Nil(t, f.repo.committedTournaments[root].CreationEvent)
			} else {
				require.Nil(t, f.repo.storedTournaments[0].CreationEvent)
			}
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
		})
	}
}

func TestNewRootRejectsDescriptorStartDifferentFromEpochSeal(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0xa0")
	address := *epoch.TournamentAddress
	d := TournamentDescriptor{BaseCycle: big.NewInt(0), Kind: model.TournamentKindLeaf, Height: 68, StartInstant: 11}
	adapter := &tournamentAdapterMock{}
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).Return((*model.Tournament)(nil), nil).Once()
	f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
	adapter.On("Descriptor", mock.Anything).Return(d, nil).Once()
	adapter.On("BondValue", mock.Anything).Return(big.NewInt(1), nil).Once()
	adapter.On("Standing", mock.Anything).Return(TournamentStanding{State: model.TournamentStandingAwaitingClosure}, nil).Once()
	expectTournamentAuxiliaryReads(adapter, mock.Anything, RootLevel, model.TournamentStandingAwaitingClosure)
	adapter.On("RetrieveAllEvents", mock.Anything).Return(&TournamentEvents{}, nil).Maybe()
	adapter.On("StructuralEventCounts", mock.Anything).Return(zeroStructuralEventCounts(), nil).Maybe()
	filterAddress := address.Hex()
	f.repo.On("ListCommitments", mock.Anything, f.app.Name, repository.CommitmentFilter{EpochIndex: &epoch.Index,
		TournamentAddress: &filterAddress}, repository.Pagination{}, false).Return([]*model.Commitment{}, uint64(0), nil).Maybe()
	f.repo.On("ListMatches", mock.Anything, f.app.Name, repository.MatchFilter{EpochIndex: &epoch.Index,
		TournamentAddress: &filterAddress}, repository.Pagination{}, false).Return([]*model.Match{}, uint64(0), nil).Maybe()
	batches, err := f.s.gatherTournamentData(t.Context(), f.app, epoch, RootLevel, nil, nil, address, 1, 100, creationFixture(t,
		address, d), nil)
	require.ErrorContains(t, err, "epoch seal")
	require.Nil(t, batches)
	f.repo.AssertNotCalled(t, "StoreTournamentInterval", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	adapter.AssertExpectations(t)
}

func TestRootObservationNeedsNoFactoryLogAcrossVerdicts(t *testing.T) {
	for _, state := range []deploymentState{deploymentNone, deploymentPending, deploymentTable, deploymentGeneration} {
		t.Run(string(state), func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			epoch := checkpointEpoch(0, "0xa0")
			epoch.Status = model.EpochStatus_Closed
			f.epochs(epoch)
			key := deploymentIdentity(f.app)
			capture := f.s.deployments.capture(key)
			snapshot := capture.snapshot
			snapshot.Assessment.State = state
			snapshot.Alpha6Shape = true
			snapshot.ShapeFactory = common.HexToAddress("0x160")
			f.s.deployments.publish(key, snapshot)
			f.consensus.deploymentRead = func(_ context.Context, head txsender.BlockRef) (DeploymentEvidence, error) {
				e := compatibleDeploymentEvidence(head)
				e.Candidate = deploymentNone
				e.FactoryKnown = false
				e.Factory = common.Address{}
				return e, nil
			}
			f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(1), nil).Once()
			address := *epoch.TournamentAddress
			f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(),
				address.Hex()).Return((*model.Tournament)(nil), nil).Once()
			d := TournamentDescriptor{BaseCycle: big.NewInt(0), Kind: model.TournamentKindLeaf, Height: 68, StartInstant: 10,
				Allowance: 300}
			adapter := &tournamentAdapterMock{}
			f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
			adapter.On("Descriptor", mock.Anything).Return(d, nil).Once()
			adapter.On("BondValue", mock.Anything).Return(big.NewInt(1), nil).Once()
			adapter.On("Standing", mock.Anything).Return(TournamentStanding{State: model.TournamentStandingAwaitingClosure}, nil).Once()
			expectTournamentAuxiliaryReads(adapter, mock.Anything, RootLevel, model.TournamentStandingAwaitingClosure)
			adapter.On("RetrieveAllEvents", mock.Anything).Return(&TournamentEvents{}, nil).Once()
			adapter.On("StructuralEventCounts", mock.Anything).Return(zeroStructuralEventCounts(), nil).Once()
			f.emptyParticipants(epoch, address)
			f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(nil).Once()
			expectProtectionReads(t, f.factory, f.consensus, observerFacts(100), epoch)
			require.NoError(t, f.s.observeApplication(t.Context(), f.app, observerFacts(100)))
			require.Nil(t, f.repo.committedTournaments[address].CreationEvent)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			adapter.AssertExpectations(t)
		})
	}
}

func TestRootCreationReloadsDurableTruthAfterAnOlderList(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0xa0")
	epoch.Status = model.EpochStatus_Closed
	f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(1), nil).Once()
	f.repo.On("ListTournaments", mock.Anything, f.app.Name, repository.TournamentFilter{}, repository.Pagination{}, false).
		Return([]*model.Tournament{}, uint64(0), nil).Once()
	// The list preceded a late backend COMMIT. The subsequent row read sees it.
	row := &model.Tournament{Address: *epoch.TournamentAddress, MaxLevel: 1, Kind: model.TournamentKindLeaf,
		Snapshot: model.TournamentSnapshot{AsOfBlock: 100}}
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), row.Address.Hex()).Return(row, nil).Once()
	require.NoError(t, f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, observerFacts(100)))
	f.factory.AssertNotCalled(t, "CreateTournamentAdapter", row.Address)
	require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
}
