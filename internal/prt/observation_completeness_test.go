// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIncompleteTournamentEventsRetryTheSameIntervalAndKeepPersistentAlarm(t *testing.T) {
	for _, selected := range []struct {
		policy          model.DefaultBlock
		tag             rpc.BlockNumber
		missingStanding bool
	}{
		{model.DefaultBlock_Latest, rpc.LatestBlockNumber, false},
		{model.DefaultBlock_Finalized, rpc.FinalizedBlockNumber, false},
		{model.DefaultBlock_Latest, rpc.LatestBlockNumber, true},
		{model.DefaultBlock_Finalized, rpc.FinalizedBlockNumber, true},
	} {
		missing := "CommitmentJoined"
		if selected.missingStanding {
			missing = testStandingChangedEvent
		}
		t.Run(selected.policy.String()+"/"+missing, func(t *testing.T) {
			f, client := tickFixture(t)
			f.d.defaultBlock = selected.policy
			var output bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&output, nil))
			f.d.submissionEnabled = true
			f.d.publicationReady.Store(true)
			root := f.repo.publishedRoots[f.app.IApplicationAddress.Hex()+f.epoch.TournamentAddress.Hex()]
			root.MaxLevel, root.Kind = 1, model.TournamentKindLeaf
			before := *root
			view := resultTestSnapshot(f.epoch, false)
			view.stage.IsFinished = false
			f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
				Return([]*model.Epoch{f.epoch}, uint64(1), nil).Times(6)
			f.consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(view.sealed, nil).Times(6)
			f.consensus.On("CanStageTournamentResult", mock.Anything).Return(view.stage, nil).Times(6)
			f.consensus.On("CanAcceptStagedTournamentResult", mock.Anything).Return(view.accept, nil).Times(6)
			f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(1), nil).Times(6)
			adapter := &tournamentAdapterMock{}
			f.factory.On("CreateTournamentAdapter", root.Address).Return(adapter, nil).Times(6)
			adapter.On("Descriptor", mock.Anything).Return(TournamentDescriptor{
				BaseCycle: new(big.Int), Kind: root.Kind, StartInstant: root.StartInstant,
			}, nil).Times(6)
			adapter.On("Standing", mock.Anything).
				Return(TournamentStanding{State: model.TournamentStandingAwaitingClosure}, nil).Times(6)
			adapter.On("BondRecovery", mock.Anything).
				Return(canonicalBondRecovery(model.BondDispositionTournamentRunning, common.Address{}, 0), nil).Times(6)
			counts := zeroStructuralEventCounts()
			counts.CommitmentJoined.SetUint64(1)
			adapter.On("StructuralEventCounts", mock.Anything).Return(counts, nil).Times(6)
			var starts []uint64
			recordInterval := func(args mock.Arguments) { starts = append(starts, args.Get(0).(*bind.FilterOpts).Start) }
			events := &TournamentEvents{CommitmentJoined: []*itournament.ITournamentCommitmentJoined{{
				Commitment: *f.epoch.Commitment, FinalStateHash: *f.epoch.MachineHash,
				Raw: types.Log{BlockNumber: 101, TxHash: common.HexToHash("0x123")},
			}}}
			events.StandingChanged = []*itournament.ITournamentStandingChanged{{MatchCount: new(big.Int),
				Dangling: *f.epoch.Commitment, ResultAt: 120,
				Raw: types.Log{BlockNumber: 101, TxHash: common.HexToHash("0x123"), Index: 1}}}
			partial := &TournamentEvents{}
			if selected.missingStanding {
				partial.CommitmentJoined = events.CommitmentJoined
			}
			adapter.On("RetrieveAllEvents", mock.Anything).Run(recordInterval).Return(partial, nil).Times(5)
			adapter.On("RetrieveAllEvents", mock.Anything).Run(recordInterval).Return(events, nil).Once()
			adapter.On("CommitmentStanding", mock.Anything, [32]byte(*f.epoch.Commitment)).
				Return(CommitmentStanding{Joined: true, FinalState: *f.epoch.MachineHash}, nil).Once()
			checkpoint := &observerCheckpointFixture{t: t, repo: f.repo.prtRepositoryMock, app: f.app}
			checkpoint.emptyParticipants(f.epoch, root.Address)
			ready := f.d.Ready()
			require.True(t, ready)
			for head := uint64(101); head <= 106; head++ {
				client.On("HeaderByNumber", mock.Anything, big.NewInt(selected.tag.Int64())).
					Return(&types.Header{Number: new(big.Int).SetUint64(head)}, nil).Once()
				expectProtectionReads(t, f.factory, f.consensus, observerFacts(head), f.epoch)
				reschedule, err := f.d.Tick(t.Context())
				require.NoError(t, err, "a count/page disagreement must retry without a per-pass ERROR")
				require.False(t, reschedule)
				require.Equal(t, ready, f.d.Ready(), "observation completeness is not a signer prerequisite")
				if head <= 105 {
					require.Empty(t, f.repo.windows, "an incomplete interval must not publish any prefix")
					require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
					require.Equal(t, before, *root, "the repository-owned projection must not change before commit")
					require.Equal(t, head-100, uint64(f.d.observationFailures[f.app.ID].failedHeads))
				}
				if head < 105 {
					require.NotContains(t, output.String(), "level=ERROR")
				}
			}
			require.Equal(t, []uint64{101, 101, 101, 101, 101, 101}, starts, "retry must use the unchanged cursor")
			require.Len(t, f.repo.windows, 1)
			require.Len(t, f.repo.windows[0].Batches, 1)
			require.Len(t, f.repo.windows[0].Batches[0].Commitments, 1)
			require.Equal(t, uint64(106), f.app.LastTournamentCheckBlock)
			require.Empty(t, f.d.observationFailures, "only a complete committed retry clears the alarm")
			require.Equal(t, 1, strings.Count(output.String(), "PRT tournament observation is stalled"))
			require.Contains(t, output.String(), missing+" event count mismatch: expected 1, fetched 0")
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			adapter.AssertExpectations(t)
			f.repo.AssertExpectations(t)
			f.consensus.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

func TestFinancialEventLengthDisagreementsAreInconclusive(t *testing.T) {
	for _, test := range []struct {
		name          string
		before, after model.BondDisposition
		events        TournamentEvents
	}{
		{"missing refund", model.BondDispositionTournamentRunning, model.BondDispositionTournamentRunning,
			TournamentEvents{MatchDeleted: []*itournament.ITournamentMatchDeleted{{}}}},
		{"extra refund", model.BondDispositionTournamentRunning, model.BondDispositionTournamentRunning,
			TournamentEvents{PartialBondRefund: []*itournament.ITournamentPartialBondRefund{{}}}},
		{"missing recovery", model.BondDispositionRecoverable, model.BondDispositionRecovered, TournamentEvents{}},
		{"extra recovery", model.BondDispositionRecovered, model.BondDispositionRecovered,
			TournamentEvents{BondRecovered: []*itournament.ITournamentBondRecovered{{}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateTournamentFinancialEvents(test.before, test.after, &test.events)
			require.ErrorIs(t, err, txsender.ErrInconclusive)
		})
	}
	err := validateTournamentFinancialEvents(model.BondDispositionRecovered, model.BondDispositionRecoverable, &TournamentEvents{})
	require.ErrorContains(t, err, "disposition regressed")
	require.NotErrorIs(t, err, txsender.ErrInconclusive)
}
