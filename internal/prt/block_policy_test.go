// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

func TestCreateUsesMatchingPersistedPRTDefaultBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	nodeConfig := PersistentConfig{
		DefaultBlock:           model.DefaultBlock_Finalized,
		ClaimSubmissionEnabled: false,
		ChainID:                42,
	}
	repo := &prtBlockPolicyCreateRepository{}
	repo.On("RegisterSubmitter", mock.Anything, PrtConfigKey, mock.MatchedBy(func(c PersistentConfig) bool {
		return c.ChainID == 42 && c.DefaultBlock == model.DefaultBlock_Finalized && !c.ClaimSubmissionEnabled &&
			c.ResolvedSignerAddress == (common.Address{})
	})).Return(nodeConfig, nil).Once()
	client := &ethClientMock{}
	client.On("ChainID", mock.Anything).Return(big.NewInt(42), nil).Once()
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Once()

	svc, err := Create(ctx, &CreateInfo{
		Config: config.PrtConfig{
			PrtPollingInterval:            time.Hour,
			BlockchainDefaultBlock:        model.DefaultBlock_Finalized,
			BlockchainId:                  42,
			FeatureClaimSubmissionEnabled: false,
		},
		Repository:     repo,
		EthClient:      client,
		AdapterFactory: &adapterFactoryMock{},
	})
	require.NoError(t, err)
	s := svc.(*Service)
	require.False(t, s.submissionEnabled)
	require.NotNil(t, s.delivery)
	require.NotNil(t, s.disputeWarnings)
	require.NotNil(t, s.zeroStagingWarnings)
	require.NotNil(t, s.delivery.rootBondRecoveryScans)
	block, err := s.getDefaultBlockNumber(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), block)
	client.AssertNotCalled(t, "BlockNumber", mock.Anything)
	client.AssertExpectations(t)
	repo.AssertExpectations(t)
}

type prtBlockPolicyCreateRepository struct {
	repository.Repository
	mock.Mock
	register func(context.Context, string, PersistentConfig) (PersistentConfig, error)
}

func (r *prtBlockPolicyCreateRepository) RegisterSubmitter(
	ctx context.Context, key string, requested PersistentConfig,
) (PersistentConfig, error) {
	if r.register != nil {
		return r.register(ctx, key, requested)
	}
	args := r.Called(ctx, key, requested)
	return args.Get(0).(PersistentConfig), args.Error(1)
}

func (r *prtBlockPolicyCreateRepository) LoadNodeConfigRaw(ctx context.Context, key string) (
	[]byte, time.Time, time.Time, error,
) {
	args := r.Called(ctx, key)
	return args.Get(0).([]byte), time.Time{}, time.Time{}, args.Error(1)
}

func TestPRTDefaultBlockFailureDoesNotFallBackToLatest(t *testing.T) {
	for _, test := range []struct {
		name   string
		header *types.Header
		err    error
	}{
		{name: "unavailable finalized head", err: errors.New("finalized head unavailable")},
		{name: "nil header"},
		{name: "nil number", header: &types.Header{}},
		{name: "negative number", header: &types.Header{Number: big.NewInt(-1)}},
		{name: "overflow number", header: &types.Header{Number: new(big.Int).Lsh(big.NewInt(1), 64)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, repo := newPRTServiceMock()
			s.defaultBlock = model.DefaultBlock_Finalized
			s.submissionEnabled = true
			client := &ethClientMock{}
			client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
				Return(test.header, test.err).Once()
			s.client = client

			repo.On("GetApplicationIDBoundary", mock.Anything, mock.Anything).
				Return(prtRevertTestApp().ID, nil).Once()
			reschedule, err := s.Tick(t.Context())
			require.False(t, reschedule)
			require.Error(t, err)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			}
			client.AssertNotCalled(t, "BlockNumber", mock.Anything)
			repo.AssertNotCalled(t, "ListEpochs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			client.AssertExpectations(t)
			repo.AssertExpectations(t)
		})
	}
}

func TestPRTLatestStageReorgDoesNotLatchLocalState(t *testing.T) {
	for _, restaged := range []bool{false, true} {
		name := "removed stage can be submitted again"
		if restaged {
			name = "same result restaged at a different block"
		}
		t.Run(name, func(t *testing.T) {
			f := newDeliveryFixture(t)
			f.block.Number = 120
			latest := resultTestSnapshot(f.epoch, true)
			latest.sealed.StagingBlockNumber = 118
			expectPRTBlockSnapshot(f, 120, latest)
			call, err := f.d.planAction(t.Context(), f.app, stageTournament, f.block, observerFacts(100))
			require.NoError(t, err)
			require.Nil(t, call)
			changed := resultTestSnapshot(f.epoch, restaged)
			if restaged {
				changed.sealed.StagingBlockNumber = 121
			}
			f.block.Number = 122
			f.block.Hash = common.HexToHash("0x122")
			expectPRTBlockSnapshot(f, 122, changed)
			call, err = f.d.planAction(t.Context(), f.app, stageTournament, f.block, observerFacts(101))
			require.NoError(t, err)
			if restaged {
				require.Nil(t, call)
			} else {
				require.NotNil(t, call)
				require.Equal(t, stageTournament, call.Key.Kind)
			}
			require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
			require.Nil(t, f.epoch.StagedAtBlock)
			assertPRTPlanDidNotWrite(t, f)
		})
	}
}

func TestPRTStageIsPersistedOnlyWhenTrustedHeadCatchesUp(t *testing.T) {
	f := newDeliveryFixture(t)
	f.app.LastTournamentCheckBlock = 100
	unstaged := resultTestSnapshot(f.epoch, false)
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{f.epoch}, uint64(1), nil).Twice()
	expectPRTBlockSnapshot(f, 100, unstaged)
	expectProtectionReads(t, f.factory, f.consensus, observerFacts(100), f.epoch)
	require.NoError(t, f.d.observeApplication(t.Context(), f.app, observerFacts(100)))
	assertPRTPlanDidNotWrite(t, f)
	staged := resultTestSnapshot(f.epoch, true)
	staged.sealed.StagingBlockNumber = 118
	f.app.LastTournamentCheckBlock = 119
	f.repo.publishRoot(f.app, f.epoch, 119)
	expectPRTBlockSnapshot(f, 119, staged)
	f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index, uint64(118)).Return(nil).Once()
	expectProtectionReads(t, f.factory, f.consensus, observerFacts(119), f.epoch)
	require.NoError(t, f.d.observeApplication(t.Context(), f.app, observerFacts(119)))
	require.Equal(t, model.EpochStatus_ClaimStaged, f.epoch.Status)
	require.Equal(t, new(uint64(118)), f.epoch.StagedAtBlock)
	f.repo.AssertExpectations(t)
}

func TestPRTLatestResultMismatchDoesNotChangeApplicationStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*daveConsensusSnapshot)
	}{
		{"winner machine", func(s *daveConsensusSnapshot) {
			s.stage.WinnerPostEpochMachineStateHash = common.HexToHash("0xbad")
			s.sealed.StagedPostEpochMachineStateHash = s.stage.WinnerPostEpochMachineStateHash
			s.accept.StagedPostEpochMachineStateHash = s.stage.WinnerPostEpochMachineStateHash
		}},
		{"staged outputs", func(s *daveConsensusSnapshot) {
			s.sealed.StagedPostEpochOutputsMerkleRoot = common.HexToHash("0xbad")
			s.accept.StagedPostEpochOutputsMerkleRoot = s.sealed.StagedPostEpochOutputsMerkleRoot
		}},
		{"input bounds", func(s *daveConsensusSnapshot) { s.sealed.InputIndexUpperBound++ }},
		{"tournament address", func(s *daveConsensusSnapshot) { s.sealed.Tournament = common.HexToAddress("0xbad") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newDeliveryFixture(t)
			latest := resultTestSnapshot(f.epoch, true)
			latest.sealed.StagingBlockNumber = 108
			latest.accept.IsClaimStagingPeriodOver = true
			test.mutate(&latest)
			expectPRTBlockSnapshot(f, f.block.Number, latest)
			call, err := f.d.planAction(t.Context(), f.app, acceptTournament, f.block, observerFacts(100))
			require.Error(t, err)
			require.Nil(t, call)
			require.Empty(t, f.backend.published)
			assertPRTPlanDidNotWrite(t, f)
		})
	}
}

func TestPRTObserverStopsAtTrustedHead(t *testing.T) {
	for _, finishedAt := range []uint64{0, 118} {
		name := "running tournament"
		if finishedAt != 0 {
			name = "stored finish block is above trusted head"
		}
		t.Run(name, func(t *testing.T) {
			f := newDeliveryIntervalFixture(t)
			f.repo.publishedRoots = nil
			f.epoch.LastBlock = 90
			f.app.LastTournamentCheckBlock = 80
			f.app.Status = model.ApplicationStatus_Failed // Chain observation still proceeds for a terminal app.
			f.checkpoint.epochs(f.epoch)
			f.levels(1)
			f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.TournamentAddress.Hex()).
				Return(&model.Tournament{ApplicationID: f.app.ID, EpochIndex: f.epoch.Index,
					Address: *f.epoch.TournamentAddress, MaxLevel: 1, StartInstant: 90, Kind: model.TournamentKindLeaf,
					Snapshot: model.TournamentSnapshot{AsOfBlock: 80, FinishedAtBlock: finishedAt}}, nil).Once()
			adapter := &tournamentAdapterMock{}
			f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(adapter, nil).Once()
			opts := mock.MatchedBy(resultCallOptsAtBlock(100))
			adapter.On("Descriptor", opts).Return(TournamentDescriptor{
				BaseCycle: big.NewInt(0), Kind: model.TournamentKindLeaf, StartInstant: 90,
			}, nil).Once()
			adapter.On("Standing", opts).Return(TournamentStanding{State: model.TournamentStandingMatchesActive}, nil).Once()
			expectTournamentAuxiliaryReads(adapter, opts, RootLevel, model.TournamentStandingMatchesActive)
			adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
				return opts.Start == 90 && opts.End != nil && *opts.End == 100
			})).Return(&TournamentEvents{}, nil).Once()
			adapter.On("StructuralEventCounts", opts).Return(zeroStructuralEventCounts(), nil).Once()
			f.checkpoint.emptyParticipants(f.epoch, *f.epoch.TournamentAddress)
			expectProtectionReads(t, f.factory, f.consensus, f.facts, f.epoch)
			require.NoError(t, f.d.observeApplication(t.Context(), f.app, f.facts))
			require.Len(t, f.repo.windows, 1)
			require.Equal(t, uint64(100), f.repo.windows[0].ThroughBlock)
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
			require.Empty(t, f.backend.published)
			adapter.AssertExpectations(t)
		})
	}
}

func TestPRTJoinUsesLatestCommitmentToSuppressDuplicate(t *testing.T) {
	for _, matches := range []bool{true, false} {
		name := "matching final state"
		if !matches {
			name = "different final state defers"
		}
		t.Run(name, func(t *testing.T) {
			f := newDeliveryFixture(t)
			view := resultTestSnapshot(f.epoch, false)
			view.stage.IsFinished = false
			view.stage.WinnerCommitment = common.Hash{}
			view.stage.WinnerPostEpochMachineStateHash = common.Hash{}
			expectPRTBlockSnapshot(f, f.block.Number, view)
			finalState := *f.epoch.MachineHash
			if !matches {
				finalState = common.HexToHash("0xbad")
			}
			adapter := &tournamentAdapterMock{}
			adapter.On("CommitmentStanding", mock.MatchedBy(resultCallOptsAtBlock(f.block.Number)),
				[32]byte(*f.epoch.Commitment)).Return(CommitmentStanding{Joined: true, FinalState: finalState}, nil).Once()
			f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(adapter, nil).Once()
			call, err := f.d.planAction(t.Context(), f.app, joinTournament, f.block, observerFacts(100))
			if matches {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "inconsistent final state")
			}
			require.Nil(t, call)
			require.Empty(t, f.backend.published)
			assertPRTPlanDidNotWrite(t, f)
			adapter.AssertExpectations(t)
		})
	}
}

func TestPRTReaderUsesOnlyConfiguredHead(t *testing.T) {
	f := newDeliveryFixture(t)
	f.d.submissionEnabled = false
	f.app.LastTournamentCheckBlock = 100
	view := resultTestSnapshot(f.epoch, true)
	view.sealed.StagingBlockNumber = 99
	expectPRTBlockSnapshot(f, 100, view)
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{f.epoch}, uint64(1), nil).Once()
	f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index, uint64(99)).Return(nil).Once()
	expectProtectionReads(t, f.factory, f.consensus, observerFacts(100), f.epoch)
	require.NoError(t, f.d.observeApplication(t.Context(), f.app, observerFacts(100)))
	require.Equal(t, model.EpochStatus_ClaimStaged, f.epoch.Status)
	require.Empty(t, f.backend.headerReads)
	require.Empty(t, f.backend.published)
	f.repo.AssertExpectations(t)
}

func expectPRTBlockSnapshot(f *deliveryFixture, block uint64, snapshot daveConsensusSnapshot) {
	opts := mock.MatchedBy(resultCallOptsAtBlock(block))
	f.consensus.On("GetCurrentSealedEpoch", opts).Return(snapshot.sealed, nil).Once()
	f.consensus.On("CanStageTournamentResult", opts).Return(snapshot.stage, nil).Once()
	f.consensus.On("CanAcceptStagedTournamentResult", opts).Return(snapshot.accept, nil).Once()
}

func assertPRTPlanDidNotWrite(t *testing.T, f *deliveryFixture) {
	t.Helper()
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateEpochReconciledStaged", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
