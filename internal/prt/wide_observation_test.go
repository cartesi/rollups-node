// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestWideAlternateTableObservationUsesShapeAcrossVerdicts(t *testing.T) {
	rows := []DeploymentParameters{{Levels: 2, Log2Step: 68, Height: 24, ResponseBudget: 11, CommitmentBudget: 22, MaxAllowance: 33},
		{Levels: 2, Log2Step: 0, Height: 68, ResponseBudget: 11, CommitmentBudget: 22, MaxAllowance: 33}}
	require.Equal(t, deploymentTable, classifyDeploymentGeometry(rows, 2))
	for _, state := range []deploymentState{deploymentNone, deploymentPending, deploymentTable, deploymentGeneration} {
		for _, valid := range []bool{true, false} {
			suffix := "inside height"
			if !valid {
				suffix = "outside height"
			}
			t.Run(string(state)+"/"+suffix, func(t *testing.T) {
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
					e.Rows = rows
					return e, nil
				}
				root := &model.Tournament{Address: *epoch.TournamentAddress, MaxLevel: 2, Log2Step: 68, Height: 24,
					Kind: model.TournamentKindNonLeaf,
					Snapshot: model.TournamentSnapshot{AsOfBlock: 50, FinishedAtBlock: 30,
						Standing:     model.TournamentStandingRootFailed,
						BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionNoWinner}}}
				child := &model.Tournament{ApplicationID: f.app.ID, EpochIndex: epoch.Index, Address: common.HexToAddress("0xa1"), Level: 1,
					MaxLevel: 2, Height: 68,
					Kind: model.TournamentKindLeaf, StartInstant: 10, CreationEvent: &model.TournamentCreationEvent{BlockNumber: 10},
					ParentTournamentAddress: &root.Address, ParentMatchIDHash: new(common.HexToHash("0x33")),
					ContestedFinalStateOne: new(common.Hash{}), ContestedFinalStateTwo: new(common.HexToHash("0x22")),
					Snapshot: model.TournamentSnapshot{AsOfBlock: 50}}
				f.repo.storedTournaments = []*model.Tournament{root, child}
				f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(2), nil).Once()
				adapter := &tournamentAdapterMock{}
				f.factory.On("CreateTournamentAdapter", child.Address).Return(adapter, nil).Once()
				adapter.On("Descriptor", mock.Anything).Return(TournamentDescriptor{BaseCycle: big.NewInt(0), Level: 1, Height: 68,
					Kind: model.TournamentKindLeaf, StartInstant: 10}, nil).Once()
				adapter.On("Standing", mock.Anything).Return(TournamentStanding{State: model.TournamentStandingMatchesActive}, nil).Once()
				expectTournamentAuxiliaryReads(adapter, mock.Anything, 1, model.TournamentStandingMatchesActive)
				one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
				position := new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 65), big.NewInt(7))
				if !valid {
					position = new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 68), big.NewInt(7))
				}
				events := &TournamentEvents{
					LeafMatchSealed: []*itournament.ITournamentLeafMatchSealed{{MatchIdHash: *child.ParentMatchIDHash,
						One: one, Two: two,
						DivergencePosition: position, FinalStateTwo: two, DeadlineOne: 71, DeadlineTwo: 83, EliminableAt: 83,
						Raw: types.Log{Address: child.Address, BlockNumber: 60, TxHash: common.HexToHash("0x44"), Index: 1}}},
					PartialBondRefund: []*itournament.ITournamentPartialBondRefund{{Value: big.NewInt(0)}}}
				adapter.On("RetrieveAllEvents", mock.Anything).Return(events, nil).Once()
				counts := zeroStructuralEventCounts()
				counts.CommitmentJoined.SetUint64(2)
				counts.MatchCreated.SetUint64(1)
				counts.LeafMatchSealed.SetUint64(1)
				adapter.On("StructuralEventCounts", mock.Anything).Return(counts, nil).Once()
				f.repo.eventCounts = map[common.Address]repository.TournamentEventCounts{child.Address: {CommitmentJoined: 2,
					MatchCreated:    1,
					StandingChanged: 2}}
				address := child.Address.Hex()
				f.repo.On("ListCommitments", mock.Anything, f.app.Name, repository.CommitmentFilter{EpochIndex: &epoch.Index,
					TournamentAddress: &address}, repository.Pagination{}, false).Return([]*model.Commitment{}, uint64(0), nil).Once()
				f.repo.On("ListMatches", mock.Anything, f.app.Name, repository.MatchFilter{EpochIndex: &epoch.Index,
					TournamentAddress: &address}, repository.Pagination{}, false).Return([]*model.Match{{IDHash: *child.ParentMatchIDHash,
					CommitmentOne: one, CommitmentTwo: two}}, uint64(1), nil).Once()
				if valid {
					adapter.On("MatchSnapshot", mock.Anything, [32]byte(one),
						[32]byte(two)).Return(ObservedMatchSnapshot{Phase: model.MatchPhaseSealed,
						Sealed: &SealedMatch{DivergencePosition: position,
							DivergenceCycle: big.NewInt(0)}}, nil).Once()
					f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID, mock.Anything,
						uint64(100)).Run(func(args mock.Arguments) {
						batch := args.Get(2).([]*repository.TournamentEventBatch)[0]
						require.Zero(t, position.Cmp(batch.Matches[0].LeafSeal.DivergencePosition.ToBig()))
						require.Equal(t, uint64(71), batch.Matches[0].LeafSeal.DeadlineOne)
					}).Return(nil).Once()
				}
				expectProtectionReads(t, f.factory, f.consensus, observerFacts(100), epoch)
				err := f.s.observeApplication(t.Context(), f.app, observerFacts(100))
				if valid {
					require.NoError(t, err)
					require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
				} else {
					require.ErrorContains(t, err, "divergence position")
					require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
					f.repo.AssertNotCalled(t, "StoreTournamentInterval", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				}
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
				adapter.AssertExpectations(t)
			})
		}
	}
}
