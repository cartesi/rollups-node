// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
)

func TestCreateUsesMatchingPersistedPRTDefaultBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	rawConfig, err := json.Marshal(PersistentConfig{
		DefaultBlock:           model.DefaultBlock_Finalized,
		ClaimSubmissionEnabled: false,
		ChainID:                42,
	})
	require.NoError(t, err)
	repo := &prtBlockPolicyCreateRepository{}
	repo.On("LoadNodeConfigRaw", mock.Anything, PrtConfigKey).Return(rawConfig, nil).Once()
	client := &ethClientMock{}
	client.On("ChainID", mock.Anything).Return(big.NewInt(42), nil).Once()
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Once()

	s, err := Create(ctx, &CreateInfo{
		CreateInfo: service.CreateInfo{Context: ctx, PollInterval: time.Hour},
		Config: config.PrtConfig{
			BlockchainDefaultBlock:        model.DefaultBlock_Finalized,
			BlockchainId:                  42,
			FeatureClaimSubmissionEnabled: false,
		},
		Repository:     repo,
		EthClient:      client,
		AdapterFactory: &adapterFactoryMock{},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		s.Ticker.Stop()
		s.Cancel()
	})
	require.False(t, s.submissionEnabled)
	require.NotNil(t, s.pendingTransactions)
	require.NotNil(t, s.disputeWarnings)
	require.NotNil(t, s.zeroStagingWarnings)
	require.NotNil(t, s.rootBondRecoveries)
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

			s.Context = t.Context()
			repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
				Return([]*model.Application{prtRevertTestApp()}, uint64(1), nil).Once()
			errs := s.Tick()
			require.Len(t, errs, 1)
			if test.err != nil {
				require.ErrorIs(t, errs[0], test.err)
			}
			client.AssertNotCalled(t, "BlockNumber", mock.Anything)
			repo.AssertNotCalled(t, "ListEpochs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			client.AssertExpectations(t)
		})
	}
}

func TestPRTLatestStageCanAcceptWithoutPersistingStage(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, true)
	trusted := resultTestSnapshot(f.epoch, false)
	latest := resultTestSnapshot(f.epoch, true)
	latest.sealed.StagingBlockNumber = 118
	latest.accept.IsClaimStagingPeriodOver = true
	f.expectTick(100, 120, trusted, latest)
	acceptTx := types.NewTx(&types.LegacyTx{Nonce: 1})
	f.consensus.On("AcceptStagedTournamentResult", mock.Anything, f.epoch.Index).Return(acceptTx, nil).Once()

	require.NoError(t, f.tick(100))
	require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
	require.Nil(t, f.epoch.StagedAtBlock)
	require.Equal(t, acceptTx.Hash(), f.s.pendingTransactions[f.app.ID].Hash)
	f.assertNoStageWrite(t)
	f.assertNoPermanentStatusWrite(t)
}

func TestPRTLatestStageReorgDoesNotLatchLocalState(t *testing.T) {
	for _, restaged := range []bool{false, true} {
		name := "removed stage can be submitted again"
		if restaged {
			name = "same result restaged at a different block"
		}
		t.Run(name, func(t *testing.T) {
			f := newPRTBlockPolicyFixture(t, true)
			trusted := resultTestSnapshot(f.epoch, false)
			latest := resultTestSnapshot(f.epoch, true)
			latest.sealed.StagingBlockNumber = 118
			f.expectTick(100, 120, trusted, latest)
			require.NoError(t, f.tick(100))
			require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
			require.Nil(t, f.epoch.StagedAtBlock)

			changed := resultTestSnapshot(f.epoch, restaged)
			if restaged {
				changed.sealed.StagingBlockNumber = 121
			} else {
				f.consensus.On("StageTournamentResult", mock.Anything, f.epoch.Index, mock.Anything).
					Return(types.NewTx(&types.LegacyTx{Nonce: 2}), nil).Once()
			}
			f.expectTick(101, 122, trusted, changed)
			require.NoError(t, f.tick(101))
			require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
			require.Nil(t, f.epoch.StagedAtBlock)
			if !restaged {
				require.Equal(t, tournamentActionStage, f.s.pendingTransactions[f.app.ID].Action)
			}
			f.assertNoStageWrite(t)
			f.assertNoPermanentStatusWrite(t)
		})
	}
}

func TestPRTStageIsPersistedOnlyWhenTrustedHeadCatchesUp(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, true)
	unstaged := resultTestSnapshot(f.epoch, false)
	staged := resultTestSnapshot(f.epoch, true)
	staged.sealed.StagingBlockNumber = 118
	f.expectTick(100, 120, unstaged, staged)
	require.NoError(t, f.tick(100))
	f.assertNoStageWrite(t)
	require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)

	f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index, uint64(118)).Return(nil).Once()
	f.expectTick(119, 120, staged, staged)
	require.NoError(t, f.tick(119))
	require.Equal(t, model.EpochStatus_ClaimStaged, f.epoch.Status)
	require.NotNil(t, f.epoch.StagedAtBlock)
	require.Equal(t, uint64(118), *f.epoch.StagedAtBlock)
	f.assertNoPermanentStatusWrite(t)
}

func TestPRTLatestResultMismatchDoesNotChangeApplicationStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*daveConsensusSnapshot)
	}{
		{name: "winner commitment", mutate: func(s *daveConsensusSnapshot) {
			s.stage.WinnerCommitment = common.HexToHash("0xbad")
		}},
		{name: "winner machine", mutate: func(s *daveConsensusSnapshot) {
			s.stage.WinnerPostEpochMachineStateHash = common.HexToHash("0xbad")
			s.sealed.StagedPostEpochMachineStateHash = s.stage.WinnerPostEpochMachineStateHash
			s.accept.StagedPostEpochMachineStateHash = s.stage.WinnerPostEpochMachineStateHash
		}},
		{name: "staged outputs", mutate: func(s *daveConsensusSnapshot) {
			s.sealed.StagedPostEpochOutputsMerkleRoot = common.HexToHash("0xbad")
			s.accept.StagedPostEpochOutputsMerkleRoot = s.sealed.StagedPostEpochOutputsMerkleRoot
		}},
		{name: "input bounds", mutate: func(s *daveConsensusSnapshot) {
			s.sealed.InputIndexUpperBound++
		}},
		{name: "tournament address", mutate: func(s *daveConsensusSnapshot) {
			s.sealed.Tournament = common.HexToAddress("0xbad")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newPRTBlockPolicyFixture(t, true)
			trusted := resultTestSnapshot(f.epoch, false)
			latest := resultTestSnapshot(f.epoch, true)
			latest.sealed.StagingBlockNumber = 118
			latest.accept.IsClaimStagingPeriodOver = true
			test.mutate(&latest)
			f.expectTick(100, 120, trusted, latest)

			// A temporary mismatch can return an error or defer work. Neither
			// outcome permits a durable failure status or a result transaction.
			_ = f.tick(100)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.assertNoStageWrite(t)
			f.assertNoPermanentStatusWrite(t)
			f.consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
			f.consensus.AssertNotCalled(t, "AcceptStagedTournamentResult", mock.Anything, mock.Anything)
		})
	}
}

func TestPRTLatestProofRevertWaitsForTrustedWinner(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, true)
	trusted := resultTestSnapshot(f.epoch, false)
	trusted.stage.IsFinished = false
	trusted.stage.WinnerCommitment = common.Hash{}
	trusted.stage.WinnerPostEpochMachineStateHash = common.Hash{}
	latest := resultTestSnapshot(f.epoch, false)
	f.expectTick(100, 120, trusted, latest)
	f.consensus.On("StageTournamentResult", mock.Anything, f.epoch.Index, mock.Anything).
		Return((*types.Transaction)(nil), daveConsensusRevertError("InvalidMachineMerkleProof")).Once()
	// The error path must confirm the winner before it diagnoses the stored
	// proof. The tournament is still unfinished at the configured head.
	f.expectHead(100)
	f.expectSnapshot(100, trusted)

	require.ErrorContains(t, f.tick(100), "winner is not confirmed")
	f.assertNoStageWrite(t)
	f.assertNoPermanentStatusWrite(t)
	require.Empty(t, f.s.pendingTransactions)
}

func TestPRTObserverStopsAtTrustedHead(t *testing.T) {
	for _, finishedAt := range []uint64{0, 118} {
		name := "running tournament"
		if finishedAt != 0 {
			name = "stored finish block is above trusted head"
		}
		t.Run(name, func(t *testing.T) {
			f := newPRTBlockPolicyFixture(t, true)
			f.app.LastTournamentCheckBlock = 80
			f.app.LastEpochCheckBlock = 100
			f.epoch.LastBlock = 90
			trusted := resultTestSnapshot(f.epoch, false)
			trusted.stage.IsFinished = false
			trusted.stage.WinnerCommitment = common.Hash{}
			trusted.stage.WinnerPostEpochMachineStateHash = common.Hash{}
			latest := resultTestSnapshot(f.epoch, true)
			latest.sealed.StagingBlockNumber = 118
			f.expectTick(100, 120, trusted, latest)
			f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
				Return([]*model.Epoch{f.epoch}, uint64(1), nil).Once()
			f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
			f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.TournamentAddress.Hex()).
				Return(&model.Tournament{
					ApplicationID: f.app.ID,
					EpochIndex:    f.epoch.Index,
					Address:       *f.epoch.TournamentAddress,
					MaxLevel:      1,
					StartInstant:  90,
					Kind:          model.TournamentKindLeaf,
					Snapshot:      model.TournamentSnapshot{FinishedAtBlock: finishedAt},
				}, nil).Once()
			tournament := &tournamentAdapterMock{}
			f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(tournament, nil).Once()
			opts := mock.MatchedBy(resultCallOptsAtBlock(100))
			tournament.On("Descriptor", opts).
				Return(TournamentDescriptor{BaseCycle: big.NewInt(0), Kind: model.TournamentKindLeaf, StartInstant: 90}, nil).Once()
			tournament.On("Standing", opts).Return(TournamentStanding{State: model.TournamentStandingMatchesActive}, nil).Once()
			expectTournamentAuxiliaryReads(tournament, opts, RootLevel, model.TournamentStandingMatchesActive)
			tournament.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
				return opts != nil && opts.Start == 90 && opts.End != nil && *opts.End == 100
			})).Return(&TournamentEvents{}, nil).Once()
			tournament.On("StructuralEventCounts", opts).Return(zeroStructuralEventCounts(), nil).Once()
			observer := &observerCheckpointFixture{t: t, s: f.s, repo: f.repo, app: f.app}
			observer.emptyParticipants(f.epoch, *f.epoch.TournamentAddress)
			f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID,
				mock.Anything, uint64(100)).Return(nil).Once()
			observer.unaccepted(f.epoch, 0)

			require.NoError(t, runPRTApplicationTick(f.s, f.app))
			f.assertNoStageWrite(t)
			f.assertNoPermanentStatusWrite(t)
			tournament.AssertExpectations(t)
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
			f := newPRTBlockPolicyFixture(t, true)
			snapshot := resultTestSnapshot(f.epoch, false)
			snapshot.stage.IsFinished = false
			snapshot.stage.WinnerCommitment = common.Hash{}
			snapshot.stage.WinnerPostEpochMachineStateHash = common.Hash{}
			f.expectTick(100, 120, snapshot, snapshot)
			f.repo.On("GetCommitment", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.Index,
				f.epoch.TournamentAddress.Hex(), f.epoch.Commitment.String()).Return((*model.Commitment)(nil), nil).Once()
			finalState := *f.epoch.MachineHash
			if !matches {
				finalState = common.HexToHash("0xbad")
			}
			tournament := &tournamentAdapterMock{}
			tournament.On("CommitmentStanding", mock.MatchedBy(resultCallOptsAtBlock(120)), [32]byte(*f.epoch.Commitment)).
				Return(CommitmentStanding{Joined: true, FinalState: finalState}, nil).Once()
			f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(tournament, nil).Once()

			err := f.tick(100)
			if matches {
				require.NoError(t, err)
			}
			tournament.AssertNotCalled(t, "JoinTournament", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.assertNoPermanentStatusWrite(t)
			tournament.AssertExpectations(t)
		})
	}
}

func TestPRTReaderUsesOnlyConfiguredHead(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, false)
	f.expectHead(100)
	snapshot := resultTestSnapshot(f.epoch, true)
	snapshot.sealed.StagingBlockNumber = 99
	f.expectSnapshot(100, snapshot)
	f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index, uint64(99)).Return(nil).Once()

	require.NoError(t, f.tick(100))
	require.Equal(t, model.EpochStatus_ClaimStaged, f.epoch.Status)
	f.client.AssertNotCalled(t, "BlockNumber", mock.Anything)
	f.consensus.AssertNotCalled(t, "AcceptStagedTournamentResult", mock.Anything, mock.Anything)
	f.consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
}

type prtBlockPolicyFixture struct {
	s         *Service
	app       *model.Application
	epoch     *model.Epoch
	repo      *prtRepositoryMock
	client    *ethClientMock
	consensus *daveConsensusAdapterMock
	factory   *adapterFactoryMock
}

func newPRTBlockPolicyFixture(t *testing.T, submissionEnabled bool) *prtBlockPolicyFixture {
	t.Helper()
	f := &prtBlockPolicyFixture{
		app:       prtRevertTestApp(),
		epoch:     resultTestEpoch(model.EpochStatus_ClaimComputed),
		client:    &ethClientMock{},
		consensus: &daveConsensusAdapterMock{},
		factory:   &adapterFactoryMock{},
	}
	applyPRTStateProof(f.epoch, repotest.KeccakStateProof(common.HexToHash("0x500")))
	f.epoch.CommitmentProof = []common.Hash{common.HexToHash("0x700")}
	f.s, f.repo = newPRTServiceMock()
	f.s.adapterFactory = f.factory
	f.s.submissionTimeout = time.Second
	f.s.txOptsFactory = ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: common.HexToAddress("0x600")})
	f.s.defaultBlock = model.DefaultBlock_Finalized
	f.s.submissionEnabled = submissionEnabled
	f.s.client = f.client
	f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.Index).Return(f.epoch, nil)
	f.factory.On("CreateDaveConsensusAdapter", f.app.IConsensusAddress).Return(f.consensus, nil)
	t.Cleanup(func() {
		f.repo.AssertExpectations(t)
		f.client.AssertExpectations(t)
		f.consensus.AssertExpectations(t)
		f.factory.AssertExpectations(t)
	})
	return f
}

func (f *prtBlockPolicyFixture) expectHead(block uint64) {
	f.client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: new(big.Int).SetUint64(block)}, nil).Once()
}

func (f *prtBlockPolicyFixture) expectSnapshot(block uint64, snapshot daveConsensusSnapshot) {
	opts := mock.MatchedBy(resultCallOptsAtBlock(block))
	f.consensus.On("GetCurrentSealedEpoch", opts).Return(snapshot.sealed, nil).Once()
	f.consensus.On("CanStageTournamentResult", opts).Return(snapshot.stage, nil).Once()
	f.consensus.On("CanAcceptStagedTournamentResult", opts).Return(snapshot.accept, nil).Once()
}

func (f *prtBlockPolicyFixture) expectTick(trustedBlock, latestBlock uint64, trusted, latest daveConsensusSnapshot) {
	f.expectHead(trustedBlock)
	f.client.On("BlockNumber", mock.Anything).Return(latestBlock, nil).Once()
	f.expectSnapshot(trustedBlock, trusted)
	if latestBlock != trustedBlock {
		f.expectSnapshot(latestBlock, latest)
	}
}

func (f *prtBlockPolicyFixture) tick(trustedBlock uint64) error {
	// The existing observer has already scanned this trusted head. Keep this
	// fixture focused on the block policy for result and join actions.
	f.app.LastTournamentCheckBlock = trustedBlock
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{}, uint64(0), nil).Once()
	return runPRTApplicationTick(f.s, f.app)
}

func runPRTApplicationTick(s *Service, app *model.Application) error {
	block, err := s.getDefaultBlockNumber(context.Background())
	if err != nil {
		return err
	}
	return s.validateApplication(context.Background(), app, block)
}

func (f *prtBlockPolicyFixture) assertNoStageWrite(t *testing.T) {
	t.Helper()
	f.repo.AssertNotCalled(t, "UpdateEpochReconciledStaged", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func (f *prtBlockPolicyFixture) assertNoPermanentStatusWrite(t *testing.T) {
	t.Helper()
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
}
