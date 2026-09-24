// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func TestForeclosedClaimsDrainWhileTournamentTransactionIsPending(t *testing.T) {
	for _, action := range []tournamentAction{tournamentActionJoin, tournamentActionAccept} {
		t.Run(string(action), func(t *testing.T) {
			app := prtForeclosedApp(1, 100)
			s := newRootBondTestService(common.HexToAddress("0x600"), &adapterFactoryMock{})
			s.submissionEnabled = true
			s.queueRootBondRecovery(app.ID, 3, common.HexToAddress("0x300"))
			txHash := common.HexToHash("0x400")
			s.pendingTransactions[app.ID] = pendingTournamentTransaction{Action: action, Hash: txHash, EpochIndex: 3}
			client := &ethClientMock{}
			s.client = client
			client.On("BlockNumber", mock.Anything).Return(uint64(120), nil).Once()
			client.On("TransactionByHash", mock.Anything, txHash).
				Return((*types.Transaction)(nil), true, nil).Once()
			repo := s.repository.(*prtRepositoryMock)
			repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(false, nil).Once()
			repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(true, nil).Once()
			repo.On("ListEpochs", mock.Anything, app.Name,
				repository.EpochFilter{Status: model.NonTerminalEpochStatuses()}, repository.Pagination{}, false).
				Return([]*model.Epoch{{Index: 3}, {Index: 2, ClaimTransactionHash: &txHash}}, uint64(2), nil).Once()
			repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(3)).Return(nil).Once()
			require.NoError(t, s.handleForeclosedApp(t.Context(), app, expectEmptyForeclosedObservation(repo, app)))
			require.Contains(t, s.pendingTransactions, app.ID)
			require.Nil(t, s.rootBondRecoveries[app.ID][0].TxHash, "no recovery broadcast while another transaction is pending")
			repo.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(2))
			repo.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

func TestForeclosedDrainRetainsRecoveryErrorOnEveryReturn(t *testing.T) {
	const (
		observationErrorCase    = "observation error"
		ingestionIncompleteCase = "ingestion incomplete"
		inputsPendingCase       = "inputs pending"
		inputQueryErrorCase     = "input query error"
		claimsDrainedCase       = "claims drained"
		claimQueryErrorCase     = "claim query error"
		epochQueryErrorCase     = "epoch query error"
		epochWriteErrorCase     = "epoch write error"
		shutdownCase            = "shutdown"
	)
	recoveryErr := errors.New("recovery provider unavailable")
	drainErr := errors.New("drain failed")
	for _, path := range []string{
		observationErrorCase, ingestionIncompleteCase, inputsPendingCase, inputQueryErrorCase, claimsDrainedCase,
		claimQueryErrorCase, epochQueryErrorCase, epochWriteErrorCase, "claim terminalized", shutdownCase,
	} {
		t.Run(path, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			app := prtForeclosedApp(1, 100)
			s := newRootBondTestService(common.HexToAddress("0x600"), &adapterFactoryMock{})
			s.submissionEnabled = true
			s.queueRootBondRecovery(app.ID, 3, common.HexToAddress("0x300"))
			client := &ethClientMock{}
			s.client = client
			client.On("BlockNumber", mock.Anything).Run(func(mock.Arguments) {
				if path == shutdownCase {
					cancel()
				}
			}).Return(uint64(0), recoveryErr).Once()
			repo := s.repository.(*prtRepositoryMock)
			var observation func() (uint64, error)
			if path == observationErrorCase {
				observation = func() (uint64, error) { return 0, drainErr }
			} else {
				observation = expectEmptyForeclosedObservation(repo, app)
			}
			if path == ingestionIncompleteCase {
				app.LastInputCheckBlock = 99
			}
			if path != observationErrorCase && path != ingestionIncompleteCase && path != shutdownCase {
				var inputErr error
				if path == inputQueryErrorCase {
					inputErr = drainErr
				}
				repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).
					Return(path == inputsPendingCase, inputErr).Once()
				if path != inputsPendingCase && inputErr == nil {
					var claimErr error
					if path == claimQueryErrorCase {
						claimErr = drainErr
					}
					repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).
						Return(path != claimsDrainedCase, claimErr).Once()
					if path != claimsDrainedCase && claimErr == nil {
						var epochErr error
						if path == epochQueryErrorCase {
							epochErr = drainErr
						}
						repo.On("ListEpochs", mock.Anything, app.Name,
							repository.EpochFilter{Status: model.NonTerminalEpochStatuses()}, repository.Pagination{}, false).
							Return([]*model.Epoch{{Index: 3}}, uint64(1), epochErr).Once()
						if epochErr == nil {
							if path == epochWriteErrorCase {
								epochErr = drainErr
							}
							repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(3)).Return(epochErr).Once()
						}
					}
				}
			}

			err := s.handleForeclosedApp(ctx, app, observation)
			require.ErrorIs(t, err, recoveryErr)
			if path == shutdownCase {
				require.ErrorIs(t, err, context.Canceled)
			}
			if path == observationErrorCase || path == inputQueryErrorCase || path == claimQueryErrorCase ||
				path == epochQueryErrorCase || path == epochWriteErrorCase {
				require.ErrorIs(t, err, drainErr)
			}
			repo.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}
