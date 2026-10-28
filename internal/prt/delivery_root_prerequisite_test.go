// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDeliveryNewRootRequiresItsOwnPublishedCompleteWindow(t *testing.T) {
	for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			f := newDeliveryFixture(t)
			f.epoch.LastBlock = 95
			view := resultTestSnapshot(f.epoch, kind == acceptTournament)
			view.stage.IsFinished = kind != joinTournament
			view.accept.IsClaimStagingPeriodOver = true
			f.snapshot(view)
			f.repo.publishedRoots = nil
			facts := observerFacts(100)
			call, err := f.d.planAction(t.Context(), f.app, kind, f.block, facts)
			require.ErrorIs(t, err, txsender.ErrInconclusive)
			require.Nil(t, call, "an older complete application window did not include this newly discovered root")

			f.repo.publishRoot(f.app, f.epoch, 110)
			call, err = f.d.planAction(t.Context(), f.app, kind, f.block, facts)
			require.ErrorIs(t, err, txsender.ErrInconclusive)
			require.Nil(t, call, "a projection beyond the finalized application cursor has not been published")

			f.repo.publishRoot(f.app, f.epoch, 100)
			if kind == joinTournament {
				deliveryJoinAdapter(f, f.epoch, 30)
			}
			call, err = f.d.planAction(t.Context(), f.app, kind, f.block, facts)
			require.NoError(t, err)
			require.NotNil(t, call)
			require.Empty(t, f.backend.published)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestDeliveryPublishedRootRejectsWrongIdentityAndIncompleteHistory(t *testing.T) {
	for _, corrupt := range []func(*model.Tournament){
		func(root *model.Tournament) { root.ApplicationID++ },
		func(root *model.Tournament) { root.EpochIndex++ },
		func(root *model.Tournament) { root.Address = common.HexToAddress("0xbad") },
		func(root *model.Tournament) { root.Level = 1 },
		func(root *model.Tournament) { root.ParentTournamentAddress = new(common.HexToAddress("0xbad")) },
		func(root *model.Tournament) { root.StartInstant = 101 },
		func(root *model.Tournament) { root.Snapshot.AsOfBlock = 89 },
		func(root *model.Tournament) { root.Snapshot.AsOfBlock = 0 },
	} {
		f := newDeliveryFixture(t)
		f.epoch.LastBlock = 90
		f.snapshot(resultTestSnapshot(f.epoch, false))
		f.repo.publishRoot(f.app, f.epoch, 100)
		root := f.repo.publishedRoots[f.app.IApplicationAddress.Hex()+f.epoch.TournamentAddress.Hex()]
		corrupt(root)
		call, err := f.d.planAction(t.Context(), f.app, stageTournament, f.block, observerFacts(100))
		require.ErrorIs(t, err, txsender.ErrInconclusive)
		require.Nil(t, call)
	}
}
