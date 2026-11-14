// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRetiredRootDoesNotHideLaterStaging(t *testing.T) {
	for _, mode := range []string{testRecoveredRootMode, testReaderRootMode, testForeignRootMode} {
		for _, invalid := range []bool{false, true} {
			name := mode + "/healthy"
			if invalid {
				name = mode + "/IOR"
			}
			t.Run(name, func(t *testing.T) {
				f := newObserverCheckpointFixture(t)
				var logs bytes.Buffer
				f.s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
				f.app.LastTournamentCheckBlock = 90
				epoch := checkpointEpoch(0, "0x100")
				if invalid {
					epoch.CalculatedOutputsRoot = new(common.HexToHash("0xdecaf"))
				}
				f.epochs(epoch)
				root := resultBoundaryTournament(f, epoch, *epoch.Commitment)
				root.Snapshot.AsOfBlock = 90
				root.Snapshot.FinishedAtBlock = 80
				root.Allowance = 70
				root.Snapshot.Standing = model.TournamentStandingRootWinner
				root.Snapshot.AcceptsJoins = false
				root.Snapshot.WinnerCommitment = epoch.Commitment
				root.CreationEvent = &model.TournamentCreationEvent{BlockNumber: 10}
				root.Snapshot.BondRecovery.Disposition = model.BondDispositionRecovered
				setCompletedRootTestMode(f.s, root, mode)
				require.True(t, f.s.tournamentObservationComplete(root, 90, 100))
				f.repo.storedTournaments = []*model.Tournament{root}
				f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), root.Address.Hex()).Return(root, nil)
				snapshot := resultTestSnapshot(epoch, true)
				snapshot.sealed.StagingBlockNumber = 100
				f.expectResultBoundarySnapshot(epoch, snapshot, 100)
				f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, epoch.Index, uint64(100)).Return(nil).Once()
				f.consensus.On("GetCurrentSealedEpoch", protectionOpts(observerFacts(100))).Return(snapshot.sealed, nil).Once()
				require.NoError(t, f.s.observeApplication(t.Context(), f.app, observerFacts(100)))
				f.factory.AssertNotCalled(t, "CreateTournamentAdapter", root.Address)
				require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
				require.Equal(t, uint64(90), root.Snapshot.AsOfBlock, "an immutable root needs no new tournament scan")
				records := prtIORRecords(t, &logs, stagedIORCondition)
				if invalid {
					require.Len(t, records, 1, "a retired tournament cannot hide external staging of a computed IOR epoch")
					require.Equal(t, "ERROR", records[0]["level"])
					require.Equal(t, epoch.TxBufferDataBlock.Hex(), records[0]["declared_outputs_root"])
					require.Equal(t, epoch.CalculatedOutputsRoot.Hex(), records[0]["calculated_outputs_root"])
				} else {
					require.Empty(t, records)
				}
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			})
		}
	}
}

func TestRetiredRootDoesNotHideLaterAcceptance(t *testing.T) {
	for _, mode := range []string{testRecoveredRootMode, testReaderRootMode, testForeignRootMode} {
		t.Run(mode, func(t *testing.T) {
			f := newAcceptanceReceiptFixture(t)
			root := f.repo.ExpectedCalls[0].ReturnArguments.Get(0).(*model.Tournament)
			root.Snapshot.AsOfBlock = 90
			root.Snapshot.Standing = model.TournamentStandingRootWinner
			root.CreationEvent = &model.TournamentCreationEvent{BlockNumber: 10}
			root.Snapshot.BondRecovery.Disposition = model.BondDispositionRecovered
			setCompletedRootTestMode(f.service, root, mode)
			require.True(t, f.service.tournamentObservationComplete(root, 90, 100))
			f.expectReceipt()
			f.expectAcceptance()

			deferred, err := f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, 100)
			require.NoError(t, err)
			require.False(t, deferred, "a canonical consensus acceptance at finalized100 can follow retirement at90")

		})
	}
}
