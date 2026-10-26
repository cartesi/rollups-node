// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func addSecondStageApplication(f *deliveryFixture) *model.Application {
	app := *f.app
	app.ID, app.Name = 8, secondApplicationName
	app.IApplicationAddress, app.IConsensusAddress = common.HexToAddress("0x801"), common.HexToAddress("0x802")
	f.repo.apps = append(f.repo.apps, &app)
	f.backend.apps = f.repo.apps
	epoch := *f.epoch
	epoch.ApplicationID = app.ID
	f.repo.On("GetEpoch", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index).Return(&epoch, nil)
	f.repo.publishRoot(&app, &epoch, app.LastTournamentCheckBlock)
	view := resultTestSnapshot(&epoch, false)
	adapter := &daveConsensusAdapterMock{}
	adapter.On("GetCurrentSealedEpoch", mock.Anything).Return(view.sealed, nil)
	adapter.On("CanStageTournamentResult", mock.Anything).Return(view.stage, nil)
	adapter.On("CanAcceptStagedTournamentResult", mock.Anything).Return(view.accept, nil)
	f.factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(adapter, nil)
	return &app
}

func TestDeliveryHardPublicationFailureLatchesReadiness(t *testing.T) {
	causes := []error{txsender.ErrFinality, txsender.ErrContent, txsender.ErrUnsupported, txsender.ErrLimit, txsender.ErrStale}
	for _, cause := range causes {
		t.Run(cause.Error(), func(t *testing.T) {
			f, client := tickFixture(t)
			f.d.submissionEnabled = true
			f.d.publicationReady.Store(true)
			require.True(t, f.d.Ready())
			view := resultTestSnapshot(f.epoch, false)
			opts := mock.MatchedBy(resultCallOptsAtBlock(100))
			f.consensus.On("GetCurrentSealedEpoch", opts).Return(view.sealed, nil)
			f.consensus.On("CanStageTournamentResult", opts).Return(view.stage, nil)
			f.consensus.On("CanAcceptStagedTournamentResult", opts).Return(view.accept, nil)
			f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, mock.Anything, false).Return([]*model.Epoch{}, uint64(0), nil)
			tickHead(client, 100)
			f.backend.headerError = cause
			if errors.Is(cause, txsender.ErrFinality) || errors.Is(cause, txsender.ErrContent) {
				expectStoredProtection(t, f.repo.prtRepositoryMock, f.app, f.epoch)
			}
			err := f.d.tick(t.Context())
			require.ErrorIs(t, err, cause)
			hard := errors.Is(cause, txsender.ErrFinality) || errors.Is(cause, txsender.ErrContent)

			require.Equal(t, !hard, f.d.Ready())
			require.Equal(t, hard, f.d.publicationStopped())
			require.Empty(t, f.backend.published)
			require.Empty(t, f.d.candidateCooldown)
			if hard {
				require.ErrorIs(t, f.d.guardPublication(t.Context()), cause)
			}
		})
	}
}

func TestDeliveryHardObservationFailureBlocksSamePassPublication(t *testing.T) {
	f, client := tickFixture(t)
	f.d.submissionEnabled = true
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, mock.Anything, false).
		Return([]*model.Epoch(nil), uint64(0), txsender.ErrUnsupported)
	tickHead(client, 100)
	expectProtectionReads(t, f.factory, f.consensus, observerFacts(100))
	_, err := f.d.Tick(t.Context())
	require.ErrorIs(t, err, txsender.ErrUnsupported)
	require.False(t, f.d.publicationStopped())
	require.False(t, f.d.publicationStopped())
	require.Empty(t, f.backend.headerReads)
	require.Empty(t, f.backend.published)
	require.Empty(t, f.d.candidateCooldown)
}
