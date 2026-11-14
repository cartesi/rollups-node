// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	claimerIORStage         = "ior_result_staged"
	claimerIORAccepted      = "ior_result_accepted"
	claimerIORObservedBlock = uint64(100)
	claimerIORRootState     = "IOR"
)

func claimerIORRecords(t *testing.T, logs *bytes.Buffer, condition string) []map[string]any {
	t.Helper()
	var records []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(logs.Bytes()))
	for decoder.More() {
		var record map[string]any
		require.NoError(t, decoder.Decode(&record))
		if record["condition"] == condition {
			records = append(records, record)
		}
	}
	return records
}

func TestIORSubmittedClaimObservationReportsVerifiedStage(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "healthy", true: "invalid root"}[invalid], func(t *testing.T) {
			s, repo, chain := newServiceMock(t)
			var logs bytes.Buffer
			s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			s.defaultBlock = model.DefaultBlock_Finalized
			app := makeApplication()
			epoch := makeSubmittedEpoch(app, 3)
			if invalid {
				epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
				chain.On("getClaimStatus", mock.Anything, app, epoch, mock.Anything).
					Return(makeClaimStatus(claimStatusUnstaged, epoch, 0), nil).Once()
				ref := txsender.BlockRef{Number: claimerIORObservedBlock, Hash: deliveryHeader(claimerIORObservedBlock).Hash()}
				chain.On("claimReference", mock.Anything, claimerIORObservedBlock).Return(ref, nil).Once()
			}
			event := makeStagedEvent(app, epoch)
			chain.On("getConsensusAddress", mock.Anything, app, mock.Anything).Return(app.IConsensusAddress, nil).Once()
			chain.On("findClaimStagedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, claimerIORObservedBlock).
				Return(&iconsensus.IConsensus{}, event, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
			repo.On("UpdateEpochToStaged", mock.Anything, app.ID, epoch.Index, event.Raw.BlockNumber).Return(nil).Once()
			result := s.processSubmittedClaim(t.Context(), claimWork{app: app, epoch: epoch},
				new(big.Int).SetUint64(claimerIORObservedBlock))
			require.NoError(t, result.err)
			require.Equal(t, 1, result.progress)
			records := claimerIORRecords(t, &logs, claimerIORStage)
			if invalid {
				require.Len(t, records, 1)
				require.Equal(t, "ERROR", records[0]["level"])
				require.Equal(t, epoch.TxBufferDataBlock.Hex(), records[0]["declared_outputs_root"])
				require.Equal(t, epoch.CalculatedOutputsRoot.Hex(), records[0]["calculated_outputs_root"])
				require.Equal(t, float64(event.Raw.BlockNumber), records[0]["staged_at_block"])
				require.Equal(t, float64(app.ClaimStagingPeriod), records[0]["staging_period"])
				require.Equal(t, float64(claimerIORObservedBlock), records[0]["observation_block"])
				require.Equal(t, "claim_staged_event", records[0]["observation_source"])
			} else {
				require.Empty(t, records)
			}
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
			chain.AssertExpectations(t)
		})
	}
}

func TestIORStoredStageObservationPrecedesWaitAndReaderReturns(t *testing.T) {
	const (
		stagedAt      = uint64(80)
		stagingPeriod = uint64(30)
		iterations    = 2
	)
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		t.Run(string(consensus), func(t *testing.T) {
			app := makeApplication()
			app.ConsensusType, app.ClaimStagingPeriod = consensus, stagingPeriod
			epochs := []*model.Epoch{makeStagedEpoch(app, 3, stagedAt), makeStagedEpoch(app, 4, stagedAt)}
			for _, epoch := range epochs {
				epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
			}
			for _, readerMode := range []bool{false, true} {
				// Each instance starts with only persisted application/epoch state.
				s, repo, chain := newServiceMock(t)
				s.submissionEnabled = !readerMode
				var logs bytes.Buffer
				s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
				block := stagedAt
				if readerMode {
					block += stagingPeriod
				}
				chain.On("getConsensusAddress", mock.Anything, app, mock.Anything).
					Return(app.IConsensusAddress, nil).Times(iterations * len(epochs))
				for range iterations {
					for _, epoch := range epochs {
						_, result, done := s.stagedClaimReadyForAccept(t.Context(), app, epoch, new(big.Int).SetUint64(block))
						require.True(t, done)
						require.NoError(t, result.err)
						require.Zero(t, result.progress)
					}
				}
				records := claimerIORRecords(t, &logs, claimerIORStage)
				require.Len(t, records, len(epochs), "alternating epochs must retain separate markers and restart must reconstruct both")
				require.Equal(t, float64(epochs[0].Index), records[0]["epoch_index"])
				require.Equal(t, float64(epochs[1].Index), records[1]["epoch_index"])
				require.Equal(t, "stored", records[0]["observation_source"])
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				chain.AssertNotCalled(t, "getClaimStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				chain.AssertExpectations(t)
			}
		})
	}
}

func TestIORClaimStatusDiagnosticsRequireSuccessfulProjection(t *testing.T) {
	const stagedAt = uint64(80)
	for _, accepted := range []bool{false, true} {
		for _, rootState := range []string{"healthy", claimerIORRootState, "unknown"} {
			for _, writeFails := range []bool{false, true} {
				name := "stage/" + rootState
				if accepted {
					name = "accept/" + rootState
				}
				if writeFails {
					name += "/write fails"
				}
				t.Run(name, func(t *testing.T) {
					s, repo, chain := newServiceMock(t)
					var logs bytes.Buffer
					s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
					app := makeApplication()
					epoch := makeSubmittedEpoch(app, 3)
					switch rootState {
					case claimerIORRootState:
						epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
					case "unknown":
						epoch.CalculatedOutputsRoot = nil
					}
					declared := *epoch.TxBufferDataBlock
					var calculated *common.Hash
					if epoch.CalculatedOutputsRoot != nil {
						calculated = new(*epoch.CalculatedOutputsRoot)
					}
					var writeErr error
					if writeFails {
						writeErr = errors.New("projection unavailable")
					}
					condition := claimerIORStage
					if accepted {
						condition = claimerIORAccepted
						repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, (*common.Hash)(nil)).
							Return(writeErr).Once()
						err := s.updateEpochAcceptedFromClaimStatus(t.Context(), app, epoch,
							makeClaimStatus(claimStatusAccepted, epoch, stagedAt), "test", claimerIORObservedBlock)
						require.ErrorIs(t, err, writeErr)
					} else {
						repo.On("UpdateEpochToStaged", mock.Anything, app.ID, epoch.Index, stagedAt).Return(writeErr).Once()
						_, err := s.updateEpochStagedFromClaimStatus(t.Context(), app, epoch,
							makeClaimStatus(claimStatusStaged, epoch, stagedAt), "test", claimerIORObservedBlock)
						require.ErrorIs(t, err, writeErr)
					}
					records := claimerIORRecords(t, &logs, condition)
					if writeFails || rootState != claimerIORRootState {
						require.Empty(t, records)
					} else {
						require.Len(t, records, 1)
						require.Equal(t, "ERROR", records[0]["level"])
						require.Equal(t, "claim_status", records[0]["observation_source"])
					}
					require.Equal(t, declared, *epoch.TxBufferDataBlock)
					require.Equal(t, calculated, epoch.CalculatedOutputsRoot)
					require.Equal(t, model.ApplicationStatus_OK, app.Status)
					repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
					require.Empty(t, chain.Calls, "projection uses the supplied observation without another chain read")
					repo.AssertExpectations(t)
				})
			}
		}
	}
}

func TestIORStoredStageDiagnosticsFollowChangedBlock(t *testing.T) {
	const (
		firstStage = uint64(80)
		nextStage  = uint64(81)
		period     = uint64(30)
	)
	for _, foreclosed := range []bool{false, true} {
		name := "open"
		if foreclosed {
			name = "after foreclosure"
		}
		t.Run(name, func(t *testing.T) {
			s, _, chain := newServiceMock(t)
			var logs bytes.Buffer
			s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			s.defaultBlock = model.DefaultBlock_Finalized
			app := makeApplication()
			app.ClaimStagingPeriod = period
			if foreclosed {
				app.ForecloseBlock = claimerIORObservedBlock
			}
			epoch := makeStagedEpoch(app, 3, firstStage)
			epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
			stages := []uint64{firstStage, firstStage, nextStage, nextStage}
			counts := []int{1, 1, 2, 2}
			chain.On("getConsensusAddress", mock.Anything, app, mock.Anything).
				Return(app.IConsensusAddress, nil).Times(len(stages))
			for index, stagedAt := range stages {
				epoch.StagedAtBlock = new(stagedAt)
				_, result, _ := s.stagedClaimReadyForAccept(t.Context(), app, epoch,
					new(big.Int).SetUint64(claimerIORObservedBlock))
				require.NoError(t, result.err)
				records := claimerIORRecords(t, &logs, claimerIORStage)
				require.Len(t, records, counts[index], "same block stays quiet; a newly observed staging block reports")
				require.Equal(t, float64(stagedAt), records[len(records)-1]["staged_at_block"])
				require.Equal(t, float64(app.ForecloseBlock), records[len(records)-1]["foreclose_block"])
			}
			chain.AssertExpectations(t)
		})
	}
}
