// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestQuorumStagingPreservesLocalVoteIdentityThroughBothObservationPaths(t *testing.T) {
	const submittedPath, canonicalLocalVote = "submitted to staged", "canonical local vote"
	const readerEvidence = "reader mode"
	for _, path := range []string{submittedPath, "refresh staged"} {
		for _, evidence := range []string{canonicalLocalVote, "replacement local vote", "only peer votes", readerEvidence} {
			t.Run(path+"/"+evidence, func(t *testing.T) {
				s, repo, chain := newServiceMock(t)
				s.defaultBlock = model.DefaultBlock_Latest
				app := makeApplication()
				app.ConsensusType = model.Consensus_Quorum
				epoch := makeStagedEpoch(app, 3, 44)
				if path == submittedPath {
					epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimSubmitted, nil
				}
				old := *epoch.ClaimTransactionHash
				staged := makeStagedEvent(app, epoch)
				staged.Raw.BlockNumber, staged.Raw.TxHash = 46, common.HexToHash("0xdec1de")
				local := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0x10ca1"))
				local.Raw.BlockNumber, local.Submitter = 45, chain.submitterAddress
				foreign := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0xf0e1"))
				foreign.Submitter = common.HexToAddress("0xbeef")
				want := new(old)
				if evidence == readerEvidence {
					s.submissionEnabled = false
					want = new(staged.Raw.TxHash)
				} else {
					chain.On("claimTransactionCanonical", mock.Anything, old, uint64(60)).
						Return(evidence == canonicalLocalVote, nil).Once()
					if evidence != canonicalLocalVote {
						events := []*iconsensus.IConsensusClaimSubmitted{foreign}
						want = nil
						if evidence == "replacement local vote" {
							events = append(events, local)
							want = new(local.Raw.TxHash)
						}
						chain.On("findClaimSubmittedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
							Return(&iconsensus.IConsensus{}, events, nil).Once()
					}
				}
				chain.On("findClaimStagedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
					Return(&iconsensus.IConsensus{}, staged, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
				if path == submittedPath {
					chain.On("getConsensusAddress", mock.Anything, app, big.NewInt(60)).Return(app.IConsensusAddress, nil).Once()
					if evidence != canonicalLocalVote {
						repo.On("RefreshClaimObservation", mock.Anything, epoch, want, (*uint64)(nil)).Return(nil).Once()
					}
					repo.On("UpdateEpochToStaged", mock.Anything, app.ID, epoch.Index, uint64(46)).Return(nil).Once()
					result := s.processSubmittedClaim(t.Context(), claimWork{app: app, epoch: epoch}, big.NewInt(60))
					require.NoError(t, result.err)
					require.Equal(t, 1, result.progress, "a peer majority stages even when our orphaned vote has no replacement")
				} else {
					repo.On("RefreshClaimObservation", mock.Anything, epoch, want, new(uint64(46))).Return(nil).Once()
					require.NoError(t, s.refreshStagedClaim(t.Context(), app, epoch, makeClaimStatus(claimStatusStaged, epoch, 46), 60))
					require.EqualValues(t, 46, *epoch.StagedAtBlock)
				}
				require.Equal(t, want, epoch.ClaimTransactionHash)
				if evidence != readerEvidence && want != nil {
					require.NotEqual(t, staged.Raw.TxHash, *want, "a peer's deciding vote cannot become our vote hash")
				}
				chain.AssertExpectations(t)
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestUnrelatedClaimEventCannotSetDivergenceForTheSelectedEpoch(t *testing.T) {
	const wrongApplication = "application"
	for _, path := range []string{"computed acceptance", "submitted staging"} {
		for _, unrelated := range []string{wrongApplication, "epoch"} {
			t.Run(path+"/"+unrelated, func(t *testing.T) {
				s, repo, chain := newServiceMock(t)
				app := makeApplication()
				epoch := makeComputedEpoch(app, 3)
				if path == "computed acceptance" {
					event := makeAcceptedEvent(app, epoch)
					event.MachineMerkleRoot = common.HexToHash("0xbad")
					if unrelated == wrongApplication {
						event.AppContract = common.HexToAddress("0xbeef")
					} else {
						event.LastProcessedBlockNumber = new(big.Int).SetUint64(epoch.LastBlock + 10)
					}
					chain.On("findClaimAcceptedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
						Return(&iconsensus.IConsensus{}, event, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
					result, done := s.reconcileComputedAcceptedEvent(t.Context(), claimWork{app: app, epoch: epoch}, big.NewInt(60))
					require.NoError(t, result.err)
					require.False(t, done)
				} else {
					epoch.Status = model.EpochStatus_ClaimSubmitted
					event := makeStagedEvent(app, epoch)
					event.MachineMerkleRoot = common.HexToHash("0xbad")
					if unrelated == wrongApplication {
						event.AppContract = common.HexToAddress("0xbeef")
					} else {
						event.LastProcessedBlockNumber = new(big.Int).SetUint64(epoch.LastBlock + 10)
					}
					chain.On("getConsensusAddress", mock.Anything, app, big.NewInt(60)).Return(app.IConsensusAddress, nil).Once()
					chain.On("findClaimStagedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, uint64(60)).
						Return(&iconsensus.IConsensus{}, event, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
					require.NoError(t, s.processSubmittedClaim(t.Context(), claimWork{app: app, epoch: epoch}, big.NewInt(60)).err)
				}
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "RejectEpochAndSetApplicationDiverged", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				chain.AssertExpectations(t)
			})
		}
	}
}
