// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	iorStagedLocalVoteTransaction = "0x10ca1"
	iorStagedLocalVoteSubmitter   = "0x01"
)

func TestIORStagedScanFollowsPolicyAndAcceptsFirstUnreadEvent(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Finalized, model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		t.Run(policy.String(), func(t *testing.T) {
			s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimAcceptPage)
			s.defaultBlock = policy
			first := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
			require.NoError(t, first.err)
			require.Zero(t, first.progress)
			require.Equal(t, []iorScanWindow{{iorAcceptedStream, epoch.LastBlock + 1, chain.head}}, chain.windows)
			initial := append([]iorScanWindow(nil), chain.windows...)
			again := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
			require.NoError(t, again.err)
			require.Zero(t, again.progress)
			if policy == model.DefaultBlock_Finalized {
				require.Equal(t, initial, chain.windows, "finalized scans retain completed acceptance history")
			} else {
				require.Equal(t, append(initial, initial...), chain.windows, "provisional scans repeat acceptance history")
				require.Empty(t, s.iorScans)
			}
			before := len(chain.windows)
			chain.head++
			chain.pointStatus = claimStatusAccepted
			chain.accepted = makeAcceptedEvent(app, epoch)
			chain.accepted.Raw.BlockNumber, chain.accepted.Raw.TxHash = chain.head, common.HexToHash("0xacce7")
			repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, new(chain.accepted.Raw.TxHash)).Return(nil).Once()
			accepted := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
			require.NoError(t, accepted.err)
			require.Equal(t, 1, accepted.progress)
			from := epoch.LastBlock + 1
			if policy == model.DefaultBlock_Finalized {
				from = chain.head
			}
			require.Equal(t, []iorScanWindow{{iorAcceptedStream, from, chain.head}}, chain.windows[before:])
			require.Empty(t, s.iorScans)
			records := claimerIORRecords(t, logs, claimerIORAccepted)
			require.Len(t, records, 1)
			require.Equal(t, "ERROR", records[0]["level"])
			require.Equal(t, "claim_accepted_event", records[0]["observation_source"])
			require.Equal(t, chain.accepted.Raw.TxHash.Hex(), records[0]["event_transaction"])
			repo.AssertExpectations(t)
		})
	}
}

func newIORStagedAuditFixture(t *testing.T, policy model.DefaultBlock) (
	*Service, *claimerRepositoryMock, *iorScanTestChain, *model.Application, *model.Epoch, *bytes.Buffer,
) {
	t.Helper()
	s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimAcceptPage)
	s.defaultBlock, app.ConsensusType = policy, model.Consensus_Quorum
	epoch.ClaimTransactionHash = nil
	chain.staged = makeStagedEvent(app, epoch)
	chain.staged.Raw.BlockNumber, chain.staged.Raw.TxHash = iorScanTestStage, common.HexToHash("0xdec1de")
	return s, repo, chain, app, epoch, logs
}

func TestIORStagedAuditBeginsWithCompleteVoteHistoryWhenWaitEnds(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		for _, accepted := range []bool{false, true} {
			name := policy.String() + "/staged"
			if accepted {
				name = policy.String() + "/accepted"
			}
			t.Run(name, func(t *testing.T) {
				s, repo, chain, app, epoch, logs := newIORStagedAuditFixture(t, policy)
				app.ClaimStagingPeriod = chain.head - iorScanTestStage + 1
				local := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash(iorStagedLocalVoteTransaction))
				local.Raw.BlockNumber, local.Submitter = iorScanTestStage-1, common.HexToAddress(iorStagedLocalVoteSubmitter)
				chain.submitted = append(chain.submitted, local)
				waiting := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
				require.NoError(t, waiting.err)
				require.Zero(t, waiting.progress)
				require.Equal(t, []iorScanWindow{{iorAcceptedStream, epoch.LastBlock + 1, chain.head}}, chain.windows)
				require.Nil(t, epoch.ClaimTransactionHash)
				before := len(chain.windows)
				refresh := repo.On("RefreshClaimObservation", mock.Anything, epoch,
					new(local.Raw.TxHash), new(iorScanTestStage)).Return(nil).Once()
				chain.head++
				if accepted {
					chain.pointStatus = claimStatusAccepted
					repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, (*common.Hash)(nil)).
						Return(nil).NotBefore(refresh).Once()
				}
				ready := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
				require.NoError(t, ready.err)
				require.Equal(t, []iorScanWindow{
					{iorAcceptedStream, epoch.LastBlock + 1, chain.head},
					{iorStagedStream, iorScanTestStage, iorScanTestStage},
					{iorSubmittedStream, epoch.LastBlock + 1, chain.head},
				}, chain.windows[before:], "the earlier wait did not audit old local votes")
				require.Equal(t, new(local.Raw.TxHash), epoch.ClaimTransactionHash)
				if accepted {
					require.Equal(t, 1, ready.progress)
					require.Len(t, claimerIORRecords(t, logs, claimerIORAccepted), 1)
					require.Empty(t, s.iorScans)
				} else {
					require.Zero(t, ready.progress)
					before = len(chain.windows)
					require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
					require.Equal(t, []iorScanWindow{{iorAcceptedStream, epoch.LastBlock + 1, chain.head}}, chain.windows[before:],
						"the local vote is retained but provisional acceptance history is read again")
				}
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestIORStagedAuditRepairsOrphanThenReplaysFullHistory(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		t.Run(policy.String(), func(t *testing.T) {
			s, repo, chain, app, epoch, _ := newIORStagedAuditFixture(t, policy)
			epoch.ClaimTransactionHash = new(common.HexToHash("0x0a9"))
			chain.orphanedTransaction = true
			repo.On("RefreshClaimObservation", mock.Anything, epoch, (*common.Hash)(nil), new(iorScanTestStage)).Return(nil).Once()
			require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
			require.Nil(t, epoch.ClaimTransactionHash, "an orphaned local vote must not become the peer's staging hash")
			require.Equal(t, []iorScanWindow{
				{iorAcceptedStream, epoch.LastBlock + 1, chain.head},
				{iorStagedStream, iorScanTestStage, iorScanTestStage},
				{iorSubmittedStream, epoch.LastBlock + 1, chain.head},
			}, chain.windows)
			before := len(chain.windows)
			require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
			require.Equal(t, []iorScanWindow{
				{iorAcceptedStream, epoch.LastBlock + 1, chain.head},
				{iorStagedStream, iorScanTestStage, iorScanTestStage},
				{iorSubmittedStream, epoch.LastBlock + 1, chain.head},
			}, chain.windows[before:])
			before = len(chain.windows)
			chain.head++
			require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
			require.Equal(t, []iorScanWindow{
				{iorAcceptedStream, epoch.LastBlock + 1, chain.head},
				{iorStagedStream, iorScanTestStage, iorScanTestStage},
				{iorSubmittedStream, epoch.LastBlock + 1, chain.head},
			}, chain.windows[before:])
			repo.AssertExpectations(t)
		})
	}
}

func TestIORStagedMissingVoteHashFindsLaterVoteOnFullReplay(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		t.Run(policy.String(), func(t *testing.T) {
			s, repo, chain, app, epoch, _ := newIORStagedAuditFixture(t, policy)
			require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
			require.Nil(t, epoch.ClaimTransactionHash)
			before := len(chain.windows)
			chain.head++
			local := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash(iorStagedLocalVoteTransaction))
			local.Raw.BlockNumber, local.Submitter = chain.head, common.HexToAddress(iorStagedLocalVoteSubmitter)
			chain.submitted = append(chain.submitted, local)
			repo.On("RefreshClaimObservation", mock.Anything, epoch, new(local.Raw.TxHash), new(iorScanTestStage)).Return(nil).Once()
			require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
			require.Equal(t, new(local.Raw.TxHash), epoch.ClaimTransactionHash)
			require.Equal(t, []iorScanWindow{
				{iorAcceptedStream, epoch.LastBlock + 1, chain.head},
				{iorStagedStream, iorScanTestStage, iorScanTestStage},
				{iorSubmittedStream, epoch.LastBlock + 1, chain.head},
			}, chain.windows[before:])
			before = len(chain.windows)
			require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
			require.Equal(t, []iorScanWindow{{iorAcceptedStream, epoch.LastBlock + 1, chain.head}}, chain.windows[before:])
			require.Empty(t, s.iorScans)
			repo.AssertExpectations(t)
		})
	}
}

func TestIORStagedAcceptedPointReconcilesReaderAndForeclosedApps(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Finalized, model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		for _, foreclosed := range []bool{false, true} {
			name := policy.String() + "/reader"
			if foreclosed {
				name = policy.String() + "/foreclosure"
			}
			t.Run(name, func(t *testing.T) {
				s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimAcceptPage)
				s.defaultBlock, s.submissionEnabled = policy, false
				if foreclosed {
					app.ForecloseBlock = iorScanTestHead - 1
				}
				chain.pointStatus = claimStatusAccepted
				repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, (*common.Hash)(nil)).Return(nil).Once()
				result := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
				require.NoError(t, result.err)
				require.Equal(t, 1, result.progress)
				require.Len(t, claimerIORRecords(t, logs, claimerIORAccepted), 1)
				require.Empty(t, s.iorScans)
				repo.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestIORStagedScanFailureRequiresProvisionalAuditBeforeAcceptance(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		for _, failure := range []string{"no failure", "stage audit", "vote audit", "audit write", "accepted write"} {
			t.Run(policy.String()+"/"+failure, func(t *testing.T) {
				s, repo, chain, app, epoch, logs := newIORStagedAuditFixture(t, policy)
				chain.pointStatus = claimStatusAccepted
				scanErr := errors.New("accepted history incomplete")
				chain.scanErrors[iorAcceptedStream] = scanErr
				failureErr := errors.New("audit or terminal write failed")
				local := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash(iorStagedLocalVoteTransaction))
				local.Raw.BlockNumber, local.Submitter = iorScanTestStage-1, common.HexToAddress(iorStagedLocalVoteSubmitter)
				chain.submitted = append(chain.submitted, local)
				switch failure {
				case "stage audit":
					chain.scanErrors[iorStagedStream] = failureErr
				case "vote audit":
					chain.scanErrors[iorSubmittedStream] = failureErr
				default:
					var auditErr error
					if failure == "audit write" {
						auditErr = failureErr
					}
					refresh := repo.On("RefreshClaimObservation", mock.Anything, epoch,
						new(local.Raw.TxHash), new(iorScanTestStage)).Return(auditErr).Once()
					if auditErr == nil {
						var acceptedErr error
						if failure == "accepted write" {
							acceptedErr = failureErr
						}
						repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, (*common.Hash)(nil)).
							Return(acceptedErr).NotBefore(refresh).Once()
					}
				}
				result := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
				require.ErrorIs(t, result.err, scanErr, "projection must not hide the historical scan failure")
				if failure == "no failure" {
					require.Equal(t, 1, result.progress)
					require.Len(t, claimerIORRecords(t, logs, claimerIORAccepted), 1)
				} else {
					require.ErrorIs(t, result.err, failureErr)
					require.Zero(t, result.progress)
					require.Empty(t, claimerIORRecords(t, logs, claimerIORAccepted))
				}
				require.Empty(t, s.iorScans)
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestIORStagedPointFromRemovedHeadCannotAccept(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		t.Run(policy.String(), func(t *testing.T) {
			s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimAcceptPage)
			s.defaultBlock, chain.pointStatus = policy, claimStatusAccepted
			chain.onScan = func(stream string) {
				if stream == iorAcceptedStream {
					chain.branchChanged = true
					chain.scanErrors[stream] = txsender.ErrNonCanonical
				}
			}
			result := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
			require.ErrorIs(t, result.err, txsender.ErrNonCanonical)
			require.Zero(t, result.progress)
			require.Empty(t, repo.Calls)
			require.Empty(t, claimerIORRecords(t, logs, claimerIORAccepted))
			require.Empty(t, s.iorScans)
		})
	}
}

func TestIORStagedCancelledSuffixRemainsUnread(t *testing.T) {
	s, _, chain, app, epoch, logs := newIORScanFixture(t, claimAcceptPage)
	require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
	chain.head++
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	chain.onScan = func(string) { cancel() }
	result := runIORScan(ctx, s, claimAcceptPage, app, epoch, chain.head)
	require.ErrorIs(t, result.err, context.Canceled)
	key := iorScanKey{app.ID, epoch.Index, claimAcceptPage}
	require.Equal(t, iorScanTestHead, s.iorScans[key].through.Number)
	require.Empty(t, claimerIORRecords(t, logs, claimerIORAccepted))
	before := len(chain.windows)
	chain.onScan = nil
	require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
	require.Equal(t, []iorScanWindow{{iorAcceptedStream, chain.head, chain.head}}, chain.windows[before:])
}

func TestIORHealthyStagedEventPathKeepsExistingTraversal(t *testing.T) {
	s, _, chain, app, epoch, logs := newIORScanFixture(t, claimAcceptPage)
	epoch.CalculatedOutputsRoot = new(*epoch.TxBufferDataBlock)
	for range 2 {
		result := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
		require.NoError(t, result.err)
		require.Zero(t, result.progress)
	}
	require.Equal(t, []iorScanWindow{
		{iorAcceptedStream, epoch.LastBlock + 1, chain.head},
		{iorAcceptedStream, epoch.LastBlock + 1, chain.head},
	}, chain.windows)
	require.Equal(t, []string{iorAcceptedStream, iorAcceptedStream}, chain.order)
	require.Empty(t, s.iorScans)
	require.Empty(t, claimerIORRecords(t, logs, claimerIORAccepted))
}

func TestIORStagedAuditCannotAcceptPointFromRemovedHead(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		t.Run(policy.String(), func(t *testing.T) {
			s, repo, chain, app, epoch, logs := newIORStagedAuditFixture(t, policy)
			chain.pointStatus = claimStatusAccepted
			local := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash(iorStagedLocalVoteTransaction))
			local.Raw.BlockNumber, local.Submitter = iorScanTestStage-1, common.HexToAddress(iorStagedLocalVoteSubmitter)
			chain.submitted = append(chain.submitted, local)
			chain.onScan = func(stream string) {
				if stream == iorSubmittedStream {
					chain.branchChanged = true
				}
			}
			repo.On("RefreshClaimObservation", mock.Anything, epoch,
				new(local.Raw.TxHash), new(iorScanTestStage)).Return(nil).Once()
			result := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
			require.ErrorIs(t, result.err, txsender.ErrNonCanonical)
			require.Zero(t, result.progress)
			repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			require.Empty(t, claimerIORRecords(t, logs, claimerIORAccepted))
			require.Empty(t, s.iorScans)
			repo.AssertExpectations(t)
		})
	}
}

func TestIORStagedFailedAcceptedWriteKeepsUnreadSuffix(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Finalized, model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		t.Run(policy.String(), func(t *testing.T) {
			s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimAcceptPage)
			s.defaultBlock = policy
			require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
			chain.head++
			chain.pointStatus = claimStatusAccepted
			chain.accepted = makeAcceptedEvent(app, epoch)
			chain.accepted.Raw.BlockNumber = chain.head
			writeErr := errors.New("accepted projection unavailable")
			repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, new(chain.accepted.Raw.TxHash)).
				Return(writeErr).Once()
			result := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
			require.ErrorIs(t, result.err, writeErr)
			require.Zero(t, result.progress)
			key := iorScanKey{app.ID, epoch.Index, claimAcceptPage}
			if policy == model.DefaultBlock_Finalized {
				require.Equal(t, iorScanTestHead, s.iorScans[key].through.Number)
			} else {
				require.Empty(t, s.iorScans)
			}
			require.Empty(t, claimerIORRecords(t, logs, claimerIORAccepted))
			before := len(chain.windows)
			repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, new(chain.accepted.Raw.TxHash)).
				Return(nil).Once()
			retry := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
			require.NoError(t, retry.err)
			require.Equal(t, 1, retry.progress)
			from := epoch.LastBlock + 1
			if policy == model.DefaultBlock_Finalized {
				from = chain.head
			}
			require.Equal(t, []iorScanWindow{{iorAcceptedStream, from, chain.head}}, chain.windows[before:])
			require.Len(t, claimerIORRecords(t, logs, claimerIORAccepted), 1)
			require.Empty(t, s.iorScans)
			repo.AssertExpectations(t)
		})
	}
}

func TestIORIncompleteAcceptedScanNeverTerminalizesFromLocalPointAbsence(t *testing.T) {
	const (
		absentLocalStageCase  = "local key has no stage"
		presentLocalStageCase = "local key has stage"
		failedPointReadCase   = "point lookup fails"
	)
	for _, state := range []string{absentLocalStageCase, presentLocalStageCase, failedPointReadCase} {
		for _, closed := range []bool{false, true} {
			name := state + "/active"
			if closed {
				name = state + "/closed"
			}
			t.Run(name, func(t *testing.T) {
				s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimAcceptPage)
				s.defaultBlock, app.ConsensusType = model.DefaultBlock_Safe, model.Consensus_Quorum
				require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
				chain.head++
				chain.branchChanged = true
				if closed {
					app.ForecloseBlock = chain.head
				}
				switch state {
				case absentLocalStageCase:
					chain.pointStatus = claimStatusUnstaged
				case failedPointReadCase:
					chain.pointErr = errors.New("point state unavailable")
				}
				scanErr := errors.New("foreign acceptance stream incomplete")
				chain.scanErrors[iorAcceptedStream] = scanErr
				repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, epoch.Index).Return(nil).Maybe()
				result := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
				require.ErrorIs(t, result.err, scanErr)
				require.Zero(t, result.progress, "incomplete acceptance stream must not terminalize local claim")
				require.Empty(t, repo.Calls, "incomplete acceptance evidence cannot terminalize or change health")
				require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				require.Empty(t, claimerIORRecords(t, logs, claimerIORAccepted))
				require.Empty(t, s.iorScans)
				// The replacement branch's different accepted key must remain
				// comparable after the provider returns the complete history.
				delete(chain.scanErrors, iorAcceptedStream)
				chain.pointErr = nil
				foreign := makeAcceptedEvent(app, epoch)
				foreign.Raw.BlockNumber = chain.head
				foreign.MachineMerkleRoot[0] ^= 1
				chain.accepted = foreign
				repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
					Return(nil).Once()
				retried := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
				require.Error(t, retried.err)
				require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
				repo.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertExpectations(t)
			})
		}
	}
}
