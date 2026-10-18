// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestForeclosedClaimsDrainWhenRecoveryDiscoveryFails(t *testing.T) {
	f := newDeliveryFixture(t)
	f.d.submissionEnabled = true
	f.d.defaultBlock = model.DefaultBlock_Finalized
	f.app.ForecloseBlock = 90
	f.app.LastInputCheckBlock, f.app.LastEpochCheckBlock = 100, 100
	client := &ethClientMock{}
	header := &types.Header{Number: big.NewInt(100)}
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(header, nil).Once()
	client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(header, nil).Once()
	f.d.client = client
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)},
		repository.Pagination{}, false).Return([]*model.Epoch(nil), uint64(0), nil).Once()
	f.repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, f.app.ID, uint64(90)).Return(false, nil).Once()
	f.repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, f.app.ID, uint64(90)).Return(true, nil).Once()
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{{Index: 3}}, uint64(1), nil).Once()
	f.repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, f.app.ID, uint64(3)).Return(nil).Once()
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, f.app.IApplicationAddress.Hex()).Return(new(uint64(3)), nil)
	failure := errors.New("recovery query unavailable")
	f.d.repository = &recoveryRepositoryPort{deliveryRepoFake: f.repo, err: failure}
	observationErr := f.d.observeApplication(t.Context(), f.app, observerFacts(100))
	require.ErrorIs(t, errors.Join(observationErr, f.d.tick(t.Context())), failure)
	f.repo.AssertExpectations(t)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
}

func TestForeclosedDrainRetainsRecoveryErrorOnEveryReturn(t *testing.T) {
	const (
		observationError    = "observation error"
		ingestionIncomplete = "ingestion incomplete"
		inputsPending       = "inputs pending"
		inputQueryError     = "input query error"
		claimsDrained       = "claims drained"
		claimQueryError     = "claim query error"
		epochQueryError     = "epoch query error"
		epochWriteError     = "epoch write error"
		claimTerminalized   = "claim terminalized"
		shutdown            = "shutdown"
	)
	for _, path := range []string{observationError, ingestionIncomplete, inputsPending, inputQueryError,
		claimsDrained, claimQueryError, epochQueryError, epochWriteError, claimTerminalized, shutdown} {
		t.Run(path, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			f := newDeliveryFixture(t)
			f.d.submissionEnabled = true
			f.d.defaultBlock = model.DefaultBlock_Finalized
			f.app.ForecloseBlock = 90
			f.app.LastInputCheckBlock, f.app.LastEpochCheckBlock = 100, 100
			client := &ethClientMock{}
			header := &types.Header{Number: big.NewInt(100)}
			client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(header, nil).Once()
			client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(header, nil).Once()
			f.d.client = client
			drainErr := errors.New("drain failed")
			var observeErr error
			if path == observationError {
				observeErr = drainErr
			}
			f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)},
				repository.Pagination{}, false).Return([]*model.Epoch(nil), uint64(0), observeErr).Once()
			if path == ingestionIncomplete {
				f.app.LastInputCheckBlock = 1
			}
			if path != observationError && path != ingestionIncomplete {
				var inputErr error
				if path == inputQueryError {
					inputErr = drainErr
				}
				f.repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, f.app.ID, uint64(90)).
					Return(path == inputsPending, inputErr).Once()
				if path != inputsPending && inputErr == nil {
					var claimErr error
					if path == claimQueryError {
						claimErr = drainErr
					}
					f.repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, f.app.ID, uint64(90)).
						Return(path != claimsDrained, claimErr).Once()
					if path != claimsDrained && claimErr == nil {
						var epochErr error
						if path == epochQueryError {
							epochErr = drainErr
						}
						f.repo.On("ListEpochs", mock.Anything, f.app.Name,
							repository.EpochFilter{Status: model.NonTerminalEpochStatuses()}, repository.Pagination{}, false).
							Return([]*model.Epoch{{Index: 3}}, uint64(1), epochErr).Once()
						if epochErr == nil {
							if path == epochWriteError {
								epochErr = drainErr
							}
							f.repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, f.app.ID, uint64(3)).Return(epochErr).Once()
						}
					}
				}
			}
			f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, f.app.IApplicationAddress.Hex()).Return(new(uint64(3)), nil)
			failure := errors.New("recovery provider unavailable")
			r := &recoveryRepositoryPort{deliveryRepoFake: f.repo, err: failure}
			if path == shutdown {
				r.beforeQuery = cancel
				r.err = errors.Join(failure, context.Canceled)
			}
			f.d.repository = r
			observationErr := f.d.observeApplication(ctx, f.app, observerFacts(100))
			err := errors.Join(observationErr, f.d.tick(ctx))
			require.ErrorIs(t, err, failure)
			if path == shutdown {
				require.ErrorIs(t, err, context.Canceled)
			}
			if path == observationError || path == inputQueryError || path == claimQueryError ||
				path == epochQueryError || path == epochWriteError {
				require.ErrorIs(t, err, drainErr)
			}
			f.repo.AssertExpectations(t)
		})
	}
}
