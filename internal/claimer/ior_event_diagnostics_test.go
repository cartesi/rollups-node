// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
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

func TestIORAcceptedEventDiagnosticsRequireCommittedIOR(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		for _, computed := range []bool{false, true} {
			for _, invalid := range []bool{false, true} {
				for _, writeFails := range []bool{false, true} {
					name := consensus.String() + "/staged"
					if computed {
						name = consensus.String() + "/computed"
					}
					if invalid {
						name += "/" + claimerIORRootState
					} else {
						name += "/healthy"
					}
					if writeFails {
						name += "/write fails"
					}
					t.Run(name, func(t *testing.T) {
						s, repo, chain := newServiceMock(t)
						s.defaultBlock = model.DefaultBlock_Finalized
						var logs bytes.Buffer
						s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
						app := makeApplication()
						app.ConsensusType = consensus
						epoch := makeStagedEpoch(app, 3, 80)
						if computed {
							epoch = makeComputedEpoch(app, 3)
						} else {
							chain.On("getConsensusAddress", mock.Anything, app, mock.Anything).
								Return(app.IConsensusAddress, nil).Once()
						}
						if invalid {
							epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
						}
						if invalid && !computed {
							ref := txsender.BlockRef{Number: claimerIORObservedBlock, Hash: deliveryHeader(claimerIORObservedBlock).Hash()}
							chain.On("claimReference", mock.Anything, claimerIORObservedBlock).Return(ref, nil).Once()
							chain.On("getClaimStatus", mock.Anything, app, epoch, mock.Anything).
								Return(makeClaimStatus(claimStatusAccepted, epoch, *epoch.StagedAtBlock), nil).Once()
							chain.On("checkClaimReference", mock.Anything, ref).Return(nil).Once()
						}
						declared, calculated := *epoch.TxBufferDataBlock, *epoch.CalculatedOutputsRoot
						event := makeAcceptedEvent(app, epoch)
						event.Raw.TxHash = common.HexToHash("0xacce9")
						event.Raw.BlockNumber = claimerIORObservedBlock - 1
						chain.On("findClaimAcceptedEventAndSucc", mock.Anything, app, epoch,
							epoch.LastBlock+1, claimerIORObservedBlock).
							Return(&iconsensus.IConsensus{}, event, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
						var writeErr error
						if writeFails {
							writeErr = errors.New("acceptance projection unavailable")
						}
						repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, &event.Raw.TxHash).
							Return(writeErr).Once()
						work := claimWork{app: app, epoch: epoch}
						block := new(big.Int).SetUint64(claimerIORObservedBlock)
						var result claimStepResult
						if computed {
							var done bool
							result, done = s.reconcileComputedAcceptedEvent(t.Context(), work, block)
							require.True(t, done)
						} else {
							result = s.processAcceptedClaimEvent(t.Context(), work, block)
						}
						require.ErrorIs(t, result.err, writeErr)
						records := claimerIORRecords(t, &logs, claimerIORAccepted)
						if writeFails {
							require.Zero(t, result.progress)
						} else {
							require.Equal(t, 1, result.progress)
						}
						if writeFails || !invalid {
							require.Empty(t, records, "a failed write or healthy result must not report accepted IOR")
						} else {
							require.Len(t, records, 1)
							record := records[0]
							require.Equal(t, "ERROR", record["level"])
							require.Equal(t, "claim_accepted_event", record["observation_source"])
							require.Equal(t, float64(claimerIORObservedBlock), record["observation_block"])
							require.Equal(t, float64(event.Raw.BlockNumber), record["event_block"])
							require.Equal(t, event.Raw.TxHash.Hex(), record["event_transaction"])
							require.Equal(t, float64(epoch.Index), record["epoch_index"])
							require.Equal(t, declared.Hex(), record["declared_outputs_root"])
							require.Equal(t, calculated.Hex(), record["calculated_outputs_root"])
						}
						require.Equal(t, declared, *epoch.TxBufferDataBlock)
						require.Equal(t, calculated, *epoch.CalculatedOutputsRoot)
						require.Equal(t, model.ApplicationStatus_OK, app.Status)
						repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
						repo.AssertExpectations(t)
						chain.AssertExpectations(t)
					})
				}
			}
		}
	}
}
