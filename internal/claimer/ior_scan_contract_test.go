// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIORWarmPrefixPreservesForeignAcceptanceAtFirstUnreadBlock(t *testing.T) {
	s, repo, chain, app, epoch, _ := newIORScanFixture(t, claimSubmitPage)
	app.ConsensusType = model.Consensus_Quorum
	require.NoError(t, runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head).err)
	require.Len(t, s.iorScans, 1)
	chain.head++
	foreign := makeAcceptedEvent(app, epoch)
	foreign.MachineMerkleRoot[0] ^= 1
	foreign.Raw.BlockNumber = chain.head
	chain.accepted = foreign
	repo.On("RejectEpochAndSetApplicationDiverged", mock.Anything, app.ID, epoch.Index, mock.Anything).
		Return(repository.RejectEpochAndDivergeResult{EpochRejected: true, ApplicationDiverged: true}, nil).Once()
	result := runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head)
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status,
		"foreign acceptance in the first unread block must not hide behind an UNSTAGED local key")
	require.Equal(t, model.EpochStatus_ClaimRejected, epoch.Status)
	require.Error(t, result.err)
	repo.AssertExpectations(t)
}

func TestIORSuccessfulScansCannotCertifyAReplacementHead(t *testing.T) {
	for _, phase := range []int{claimSubmitPage, claimStagePage, claimAcceptPage} {
		t.Run(iorScanPhaseName(phase), func(t *testing.T) {
			s, _, chain, app, epoch, _ := newIORScanFixture(t, phase)
			lastStream := iorSubmittedStream
			switch phase {
			case claimStagePage:
				lastStream = iorStagedStream
			case claimAcceptPage:
				lastStream = iorAcceptedStream
			}
			// This is an injected provider contradiction, not an expected
			// replacement of a correctly finalized chain.
			chain.onScan = func(stream string) {
				if stream == lastStream {
					chain.branchChanged = true
				}
			}
			result := runIORScan(t.Context(), s, phase, app, epoch, chain.head)
			require.ErrorIs(t, result.err, txsender.ErrNonCanonical,
				"the original pre-scan reference must be checked after the history reads")
			require.Empty(t, s.iorScans, "a completed scan cannot certify a replacement branch")
		})
	}
}

func TestIORPointReadUsesSelectedObservationHeight(t *testing.T) {
	for _, phase := range []int{claimSubmitPage, claimStagePage, claimAcceptPage} {
		t.Run(iorScanPhaseName(phase), func(t *testing.T) {
			s, _, chain, app, epoch, _ := newIORScanFixture(t, phase)
			chain.head++
			require.NoError(t, runIORScan(t.Context(), s, phase, app, epoch, chain.head).err)
			require.Equal(t, "point", chain.order[0])
		})
	}
}

func TestIORHardScanFailureNeverProjectsPointEvidence(t *testing.T) {
	for _, phase := range []int{claimSubmitPage, claimStagePage, claimAcceptPage} {
		t.Run(iorScanPhaseName(phase), func(t *testing.T) {
			s, repo, chain, app, epoch, logs := newIORScanFixture(t, phase)
			stream := iorAcceptedStream
			chain.pointStatus = claimStatusStaged
			switch phase {
			case claimSubmitPage:
				repo.On("UpdateEpochReconciledStaged", mock.Anything, app.ID, epoch.Index, chain.pointBlock).Return(nil).Maybe()
			case claimStagePage:
				stream = iorStagedStream
				repo.On("UpdateEpochToStaged", mock.Anything, app.ID, epoch.Index, chain.pointBlock).Return(nil).Maybe()
			default:
				chain.pointStatus = claimStatusAccepted
				repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, (*common.Hash)(nil)).Return(nil).Maybe()
			}
			chain.scanErrors[stream] = txsender.ErrFinality
			result := runIORScan(t.Context(), s, phase, app, epoch, chain.head)
			require.ErrorIs(t, result.err, txsender.ErrFinality)
			require.Zero(t, result.progress, "a hard scan failure must not become a successful local projection")
			require.Empty(t, repo.Calls, "a valid point cannot override a hard scan failure")
			require.Empty(t, claimerIORRecords(t, logs, claimerIORStage))
			require.Empty(t, claimerIORRecords(t, logs, claimerIORAccepted))
		})
	}
}

func failIORStoredPrefixCheck(chain *iorScanTestChain, failure error) {
	chain.referenceError = func(ref txsender.BlockRef) error {
		if ref.Number == iorScanTestHead {
			return failure
		}
		return nil
	}
}

func TestIORTransientPrefixFailureRetainsHistoryForRetry(t *testing.T) {
	for _, phase := range []int{claimSubmitPage, claimStagePage, claimAcceptPage} {
		t.Run(iorScanPhaseName(phase), func(t *testing.T) {
			s, _, chain, app, epoch, _ := newIORScanFixture(t, phase)
			require.NoError(t, runIORScan(t.Context(), s, phase, app, epoch, chain.head).err)
			key := iorScanKey{app.ID, epoch.Index, phase}
			require.Equal(t, iorScanTestHead, s.iorScans[key].through.Number)
			before := len(chain.windows)
			chain.head++
			failure := errors.New("cached prefix header temporarily unavailable")
			failIORStoredPrefixCheck(chain, failure)
			result := runIORScan(t.Context(), s, phase, app, epoch, chain.head)
			require.ErrorIs(t, result.err, failure)
			require.Zero(t, result.progress)
			require.Equal(t, before, len(chain.windows), "a transient reference failure must not trigger full replay")
			require.Equal(t, iorScanTestHead, s.iorScans[key].through.Number)
			chain.referenceError = nil
			require.NoError(t, runIORScan(t.Context(), s, phase, app, epoch, chain.head).err)
			for _, window := range chain.windows[before:] {
				require.Equal(t, chain.head, window.from, "retry must resume at the first unread block")
				require.Equal(t, chain.head, window.to)
			}
			require.Greater(t, len(chain.windows), before)
			require.Equal(t, chain.head, s.iorScans[key].through.Number)
		})
	}
}

func TestIORTransientPrefixFailureStillProjectsVerifiedPositivePoint(t *testing.T) {
	for _, phase := range []int{claimSubmitPage, claimStagePage} {
		for _, point := range []uint8{claimStatusStaged, claimStatusAccepted} {
			name := iorScanPhaseName(phase) + "/" + map[uint8]string{claimStatusStaged: "stage", claimStatusAccepted: "accept"}[point]
			t.Run(name, func(t *testing.T) {
				s, repo, chain, app, epoch, logs := newIORScanFixture(t, phase)
				require.NoError(t, runIORScan(t.Context(), s, phase, app, epoch, chain.head).err)
				before := len(chain.windows)
				chain.head++
				chain.pointStatus = point
				failure := errors.New("cached prefix header temporarily unavailable")
				failIORStoredPrefixCheck(chain, failure)
				condition := claimerIORStage
				switch {
				case point == claimStatusAccepted:
					condition = claimerIORAccepted
					repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, (*common.Hash)(nil)).Return(nil).Once()
				case phase == claimSubmitPage:
					repo.On("UpdateEpochReconciledStaged", mock.Anything, app.ID, epoch.Index, chain.pointBlock).Return(nil).Once()
				default:
					repo.On("UpdateEpochToStaged", mock.Anything, app.ID, epoch.Index, chain.pointBlock).Return(nil).Once()
				}
				result := runIORScan(t.Context(), s, phase, app, epoch, chain.head)
				require.ErrorIs(t, result.err, failure, "a verified positive projection must not hide the prefix-check failure")
				require.Equal(t, 1, result.progress)
				require.Equal(t, before, len(chain.windows))
				require.Len(t, claimerIORRecords(t, logs, condition), 1)
				require.Empty(t, s.iorScans, "confirmed progress retires the previous phase's prefix")
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestIORStagedPrefixFailureUsesOnlyPositiveAcceptance(t *testing.T) {
	for _, state := range []string{iorAcceptedStream, iorStagedStream, "unstaged", "point error", "foreclosed unstaged"} {
		t.Run(state, func(t *testing.T) {
			s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimAcceptPage)
			require.NoError(t, runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head).err)
			before := len(chain.windows)
			chain.head++
			switch state {
			case iorAcceptedStream:
				chain.pointStatus = claimStatusAccepted
				repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, (*common.Hash)(nil)).Return(nil).Once()
			case "unstaged":
				chain.pointStatus = claimStatusUnstaged
			case "foreclosed unstaged":
				chain.pointStatus, app.ForecloseBlock = claimStatusUnstaged, chain.head
			case "point error":
				chain.pointErr = errors.New("point state unavailable")
			}
			failure := errors.New("cached prefix header temporarily unavailable")
			failIORStoredPrefixCheck(chain, failure)
			repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, epoch.Index).Return(nil).Maybe()
			result := runIORScan(t.Context(), s, claimAcceptPage, app, epoch, chain.head)
			require.ErrorIs(t, result.err, failure)
			require.Equal(t, before, len(chain.windows))
			if state == iorAcceptedStream {
				require.Equal(t, 1, result.progress)
				require.Len(t, claimerIORRecords(t, logs, claimerIORAccepted), 1)
				require.Empty(t, s.iorScans)
			} else {
				require.Zero(t, result.progress, "an incomplete accepted stream must not terminalize from local absence")
				require.Empty(t, repo.Calls)
				require.Empty(t, claimerIORRecords(t, logs, claimerIORAccepted))
				key := iorScanKey{app.ID, epoch.Index, claimAcceptPage}
				require.Equal(t, iorScanTestHead, s.iorScans[key].through.Number)
			}
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			repo.AssertExpectations(t)
		})
	}
}

func iorScanPhaseName(phase int) string {
	return map[int]string{
		claimSubmitPage: computedVisibilityPath,
		claimStagePage:  iorSubmittedStream,
		claimAcceptPage: iorStagedStream,
	}[phase]
}
