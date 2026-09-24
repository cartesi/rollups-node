// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestClaimPublicationOutcome(t *testing.T) {
	for _, test := range []struct {
		name     string
		storeErr error
		obsolete bool
	}{
		{name: "published"},
		{name: "foreclosed during publication", storeErr: fmt.Errorf("state changed: %w", repository.ErrEpochForeclosed), obsolete: true},
		{name: "unexpected state", storeErr: repository.ErrNoUpdate},
		{name: "database failure", storeErr: errors.New("database write failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockrepo()
			var output bytes.Buffer
			postContext := merkle.CreatePostContext()
			s := &Service{
				repository: repo, pristinePostContext: postContext, pristineRootHash: postContext[merkle.TREE_DEPTH],
			}
			s.Logger = slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
			// Keep the application snapshot stale. The repository outcome, not
			// this object's foreclosure field, must decide whether work is obsolete.
			app := &Application{ID: 7, Name: "publication-app", ConsensusType: Consensus_PRT,
				Status: ApplicationStatus_OK, TemplateHash: s.pristineRootHash}
			epochs := []*Epoch{
				{ApplicationID: app.ID, Index: 0, VirtualIndex: 0, LastBlock: 9, Status: EpochStatus_InputsProcessed,
					MachineHash: &s.pristineRootHash, TxBufferDataBlock: &s.pristineRootHash},
				{ApplicationID: app.ID, Index: 1, VirtualIndex: 1, FirstBlock: 10, LastBlock: 19, Status: EpochStatus_InputsProcessed,
					MachineHash: &s.pristineRootHash, TxBufferDataBlock: &s.pristineRootHash},
			}
			appAddress := app.IApplicationAddress.String()
			repo.On("ListEpochs", mock.Anything, appAddress, mock.Anything, repository.Pagination{}, false).
				Return(epochs, uint64(len(epochs)), nil).Once()
			wantContinued := test.obsolete || test.storeErr == nil
			for i, epoch := range epochs {
				if i > 0 && !wantContinued {
					break
				}
				repo.On("ListOutputs", mock.Anything, appAddress, repository.OutputFilter{EpochIndex: &epoch.Index},
					repository.Pagination{}, false).Return([]*Output{}, uint64(0), nil).Once()
				repo.On("GetLastInput", mock.Anything, appAddress, epoch.Index).Return((*Input)(nil), nil).Once()
				storeErr := test.storeErr
				if i > 0 {
					storeErr = nil
					repo.On("GetEpochByVirtualIndex", mock.Anything, appAddress, epoch.VirtualIndex-1).Return(epochs[0], nil).Twice()
				}
				repo.On("StoreClaimAndProofs", mock.Anything, epoch, mock.Anything).Return(storeErr).Once()
			}

			err := s.validateApplication(t.Context(), app)

			if wantContinued {
				require.NoError(t, err)
				repo.AssertNumberOfCalls(t, "StoreClaimAndProofs", 2)
			} else {
				require.ErrorIs(t, err, test.storeErr)
				repo.AssertNumberOfCalls(t, "StoreClaimAndProofs", 1)
			}
			const obsoleteMessage = "Epoch was foreclosed before claim publication; discarding obsolete claim and proofs"
			if test.obsolete {
				require.Contains(t, output.String(), "level=INFO msg=\""+obsoleteMessage+"\"")
			} else {
				require.NotContains(t, output.String(), obsoleteMessage)
			}
			require.Equal(t, ApplicationStatus_OK, app.Status)
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}
