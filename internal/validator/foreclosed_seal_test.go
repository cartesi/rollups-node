// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestValidatorForeclosureRequiresPositivePRTSeal(t *testing.T) {
	for _, tc := range []struct {
		name       string
		consensus  model.Consensus
		first      uint64
		last       uint64
		tournament bool
		open       bool
		wantClaim  bool
	}{
		{name: "PRT before foreclosure", consensus: model.Consensus_PRT, first: 90, last: 99, tournament: true, wantClaim: true},
		{name: "PRT seal in foreclosure block", consensus: model.Consensus_PRT, first: 100, last: 100, tournament: true, wantClaim: true},
		{name: "PRT missing seal identity", consensus: model.Consensus_PRT, first: 90, last: 99},
		{name: "PRT future seal", consensus: model.Consensus_PRT, first: 101, last: 101, tournament: true},
		{name: "PRT OPEN does not become sealed", consensus: model.Consensus_PRT, first: 90, last: 100, tournament: true, open: true},
		{name: "Authority before foreclosure", consensus: model.Consensus_Authority, first: 90, last: 99, wantClaim: true},
		{name: "Authority overlapping foreclosure", consensus: model.Consensus_Authority, first: 90, last: 100},
		{name: "Quorum before foreclosure", consensus: model.Consensus_Quorum, first: 90, last: 99, wantClaim: true},
		{name: "Quorum overlapping foreclosure", consensus: model.Consensus_Quorum, first: 90, last: 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, svc, app, epoch := rootValidationFixture()
			app.ConsensusType, app.ForecloseBlock = tc.consensus, 100
			epoch.FirstBlock, epoch.LastBlock = tc.first, tc.last
			if tc.tournament {
				address := common.HexToAddress("0x70")
				epoch.TournamentAddress = &address
			}
			if tc.open {
				epoch.Status = model.EpochStatus_Open
			}
			repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(),
				repository.EpochFilter{Status: []model.EpochStatus{model.EpochStatus_InputsProcessed}},
				repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
			if tc.wantClaim {
				expectValidPrefixEpoch(repo, app, epoch, tc.consensus == model.Consensus_PRT)
				repo.On("StoreClaimAndProofs", mock.Anything, epoch, mock.Anything).Return(nil).Once()
			}

			err := svc.validateApplication(t.Context(), app)

			require.NoError(t, err)
			if tc.wantClaim {
				require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
				if tc.consensus == model.Consensus_PRT {
					require.NotNil(t, epoch.Commitment)
				}
				repo.AssertNumberOfCalls(t, "StoreClaimAndProofs", 1)
			} else {
				repo.AssertNotCalled(t, "StoreClaimAndProofs", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "GetEpochOutputRange", mock.Anything, mock.Anything, mock.Anything)
			}
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			repo.AssertExpectations(t)
		})
	}
}
