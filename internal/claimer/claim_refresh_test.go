// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProvisionalObserverRefreshesMatchingStageMetadataAtSelectedView(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe} {
		for _, sameHeight := range []bool{false, true} {
			t.Run(policy.String()+map[bool]string{false: "/new block", true: "/same block"}[sameHeight], func(t *testing.T) {
				s, repo, chain := newServiceMock(t)
				s.defaultBlock = policy
				app := makeApplication()
				epoch := makeStagedEpoch(app, 3, 44)
				oldHash := *epoch.ClaimTransactionHash
				staged := uint64(46)
				if sameHeight {
					staged = 44
				}
				event := makeStagedEvent(app, epoch)
				event.Raw.BlockNumber, event.Raw.TxHash = staged, common.HexToHash("0xcafe")
				if sameHeight {
					chain.On("claimTransactionCanonical", mock.Anything, oldHash, uint64(60)).Return(false, nil).Once()
				}
				chain.On("findClaimStagedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
					Return(&iconsensus.IConsensus{}, event, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
				repo.On("RefreshClaimObservation", mock.Anything, epoch, new(event.Raw.TxHash), new(staged)).
					Run(func(args mock.Arguments) {
						expected := args.Get(1).(*model.Epoch)
						require.Equal(t, oldHash, *expected.ClaimTransactionHash)
						require.Equal(t, uint64(44), *expected.StagedAtBlock)
					}).Return(nil).Once()
				result, done := s.handlePreAcceptClaimStatus(t.Context(), app, epoch,
					makeClaimStatus(claimStatusStaged, epoch, staged), 60)
				require.NoError(t, result.err)
				require.False(t, done)
				require.Equal(t, event.Raw.TxHash, *epoch.ClaimTransactionHash)
				require.Equal(t, staged, *epoch.StagedAtBlock)
				require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
				chain.AssertExpectations(t)
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestProvisionalObserverRejectsInconsistentOrConcurrentStageRefresh(t *testing.T) {
	for _, conflict := range []string{"other epoch", "different root", "different block", "concurrent acceptance"} {
		t.Run(conflict, func(t *testing.T) {
			concurrent := conflict == "concurrent acceptance"
			s, repo, chain := newServiceMock(t)
			s.defaultBlock = model.DefaultBlock_Latest
			app := makeApplication()
			epoch := makeStagedEpoch(app, 3, 44)
			beforeHash, beforeBlock := *epoch.ClaimTransactionHash, *epoch.StagedAtBlock
			event := makeStagedEvent(app, epoch)
			event.Raw.BlockNumber, event.Raw.TxHash = 46, common.HexToHash("0xcafe")
			switch conflict {
			case "other epoch":
				event.LastProcessedBlockNumber = new(big.Int).SetUint64(epoch.LastBlock + 10)
			case "different root":
				event.OutputsMerkleRoot = common.HexToHash("0xbad")
			case "different block":
				event.Raw.BlockNumber = 47
			}
			chain.On("findClaimStagedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
				Return(&iconsensus.IConsensus{}, event, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
			if concurrent {
				repo.On("RefreshClaimObservation", mock.Anything, epoch, new(event.Raw.TxHash), new(uint64(46))).
					Return(repository.ErrNoUpdate).Once()
			}
			err := s.refreshStagedClaim(t.Context(), app, epoch, makeClaimStatus(claimStatusStaged, epoch, 46), 60)
			if concurrent {
				require.ErrorIs(t, err, repository.ErrNoUpdate)
			} else {
				require.ErrorIs(t, err, txsender.ErrInconclusive)
			}
			require.Equal(t, beforeHash, *epoch.ClaimTransactionHash)
			require.Equal(t, beforeBlock, *epoch.StagedAtBlock)
			repo.AssertExpectations(t)
		})
	}
}

func TestProvisionalSubmittedObserverRefreshesOnlyMatchingLocalVote(t *testing.T) {
	s, repo, chain := newServiceMock(t)
	s.defaultBlock = model.DefaultBlock_Safe
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	epoch := makeComputedEpoch(app, 3)
	epoch.Status = model.EpochStatus_ClaimSubmitted
	other := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0x1234"))
	other.Submitter = common.HexToAddress("0xbeef")
	local := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0xcafe"))
	local.Submitter = chain.submitterAddress
	chain.On("claimTransactionCanonical", mock.Anything, *epoch.ClaimTransactionHash, uint64(60)).Return(false, nil).Once()
	chain.On("findClaimSubmittedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{other, local}, nil).Once()
	repo.On("RefreshClaimObservation", mock.Anything, epoch, new(local.Raw.TxHash), (*uint64)(nil)).Return(nil).Once()
	require.NoError(t, s.refreshSubmittedClaim(t.Context(), app, epoch, 60))
	require.Equal(t, local.Raw.TxHash, *epoch.ClaimTransactionHash)
	require.Equal(t, model.EpochStatus_ClaimSubmitted, epoch.Status)
	require.Nil(t, epoch.StagedAtBlock)
	repo.AssertExpectations(t)
}

func TestClaimMetadataRefreshRejectsSubmissionAboveSelectedView(t *testing.T) {
	s, repo, chain := newServiceMock(t)
	app := makeApplication()
	epoch := makeComputedEpoch(app, 3)
	epoch.Status = model.EpochStatus_ClaimSubmitted
	original := *epoch.ClaimTransactionHash
	event := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0xcafe"))
	event.Raw.BlockNumber = 61
	chain.On("claimTransactionCanonical", mock.Anything, original, uint64(60)).Return(false, nil).Once()
	chain.On("findClaimSubmittedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{event}, nil).Once()
	require.ErrorIs(t, s.refreshSubmittedClaim(t.Context(), app, epoch, 60), txsender.ErrInconclusive)
	require.Equal(t, original, *epoch.ClaimTransactionHash)
	repo.AssertNotCalled(t, "RefreshClaimObservation", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestUnchangedCanonicalClaimMetadataDoesNotRescanHistory(t *testing.T) {
	s, repo, chain := newServiceMock(t)
	epoch := makeStagedEpoch(makeApplication(), 3, 44)
	chain.On("claimTransactionCanonical", mock.Anything, *epoch.ClaimTransactionHash, uint64(60)).Return(true, nil).Twice()
	require.NoError(t, s.refreshStagedClaim(t.Context(), makeApplication(), epoch, makeClaimStatus(claimStatusStaged, epoch, 44), 60))
	require.NoError(t, s.refreshSubmittedClaim(t.Context(), makeApplication(), epoch, 60))
	chain.AssertNotCalled(t, "findClaimStagedEventAndSucc", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	chain.AssertNotCalled(t, "findClaimSubmittedEventAndSucc", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "RefreshClaimObservation", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestRepairedStageAcceptedBetweenPollsRefreshesAuditBeforeTerminalWrite(t *testing.T) {
	for _, eventPath := range []bool{false, true} {
		t.Run(map[bool]string{false: "state read", true: "event"}[eventPath], func(t *testing.T) {
			s, repo, chain := newServiceMock(t)
			s.defaultBlock = model.DefaultBlock_Safe
			app := makeApplication()
			epoch := makeStagedEpoch(app, 3, 44)
			staged := makeStagedEvent(app, epoch)
			staged.Raw.BlockNumber, staged.Raw.TxHash = 46, common.HexToHash("0xcafe")
			claim := makeClaimStatus(claimStatusAccepted, epoch, 46)
			chain.On("findClaimStagedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
				Return(&iconsensus.IConsensus{}, staged, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
			refresh := repo.On("RefreshClaimObservation", mock.Anything, epoch, new(staged.Raw.TxHash), new(uint64(46))).Return(nil).Once()
			if eventPath {
				accepted := makeAcceptedEvent(app, epoch)
				accepted.Raw.TxHash = common.HexToHash("0xacce")
				chain.On("getConsensusAddress", mock.Anything, app, big.NewInt(60)).Return(app.IConsensusAddress, nil).Once()
				chain.On("findClaimAcceptedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
					Return(&iconsensus.IConsensus{}, accepted, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
				chain.On("getClaimStatus", mock.Anything, app, epoch, big.NewInt(60)).Return(claim, nil).Once()
				repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, new(accepted.Raw.TxHash)).
					Return(nil).NotBefore(refresh).Once()
				result := s.processAcceptedClaimEvent(t.Context(), claimWork{app: app, epoch: epoch}, big.NewInt(60))
				require.NoError(t, result.err)
				require.Equal(t, 1, result.progress)
			} else {
				repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, (*common.Hash)(nil)).
					Return(nil).NotBefore(refresh).Once()
				result, done := s.handlePreAcceptClaimStatus(t.Context(), app, epoch, claim, 60)
				require.True(t, done)
				require.NoError(t, result.err)
			}
			require.Equal(t, uint64(46), *epoch.StagedAtBlock)
			require.Equal(t, staged.Raw.TxHash, *epoch.ClaimTransactionHash)
			chain.AssertExpectations(t)
			repo.AssertExpectations(t)
		})
	}
}

func TestSubmittedObservationFallbackRepairsTheLocalVoteHash(t *testing.T) {
	s, repo, chain := newServiceMock(t)
	s.defaultBlock = model.DefaultBlock_Safe
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	epoch := makeSubmittedEpoch(app, 3)
	old := *epoch.ClaimTransactionHash
	event := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0xcafe"))
	event.Submitter, event.Raw.BlockNumber = chain.submitterAddress, 45
	chain.On("getConsensusAddress", mock.Anything, app, big.NewInt(60)).Return(app.IConsensusAddress, nil).Once()
	chain.On("findClaimStagedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
		Return(&iconsensus.IConsensus{}, (*iconsensus.IConsensusClaimStaged)(nil), (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
	chain.On("claimTransactionCanonical", mock.Anything, old, uint64(60)).Return(false, nil).Once()
	chain.On("findClaimSubmittedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{event}, nil).Once()
	repo.On("RefreshClaimObservation", mock.Anything, epoch, new(event.Raw.TxHash), (*uint64)(nil)).Return(nil).Once()
	result := s.processSubmittedClaim(t.Context(), claimWork{app: app, epoch: epoch}, big.NewInt(60))
	require.NoError(t, result.err)
	require.Zero(t, result.progress)
	require.Equal(t, event.Raw.TxHash, *epoch.ClaimTransactionHash)
	require.Equal(t, model.EpochStatus_ClaimSubmitted, epoch.Status)
	chain.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestProvisionalAcceptedEventCannotOverrideDisagreeingClaimState(t *testing.T) {
	s, repo, chain := newServiceMock(t)
	s.defaultBlock = model.DefaultBlock_Latest
	app := makeApplication()
	epoch := makeStagedEpoch(app, 3, 44)
	event := makeAcceptedEvent(app, epoch)
	chain.On("getConsensusAddress", mock.Anything, app, big.NewInt(60)).Return(app.IConsensusAddress, nil).Once()
	chain.On("findClaimAcceptedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
		Return(&iconsensus.IConsensus{}, event, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
	chain.On("getClaimStatus", mock.Anything, app, epoch, big.NewInt(60)).
		Return(makeClaimStatus(claimStatusStaged, epoch, 44), nil).Once()
	result := s.processAcceptedClaimEvent(t.Context(), claimWork{app: app, epoch: epoch}, big.NewInt(60))
	require.ErrorIs(t, result.err, txsender.ErrInconclusive)
	require.Zero(t, result.progress)
	require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
	repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	chain.AssertExpectations(t)
}
