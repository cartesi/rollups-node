// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	stoppedRootMessage    = "Stopped observing completed tournament root"
	testReaderRootMode    = "reader"
	testForeignRootMode   = "foreign"
	testRecoveredRootMode = "recovered"
)

func pendingRootBond(claimer common.Address) model.TournamentBondRecovery {
	payment := model.Uint256{}
	return model.TournamentBondRecovery{Disposition: model.BondDispositionRecoverable, Claimer: &claimer, Payment: &payment}
}

func TestRootObservationStopsAccordingToConfiguredMode(t *testing.T) {
	ours, foreign := common.HexToAddress("0x777"), common.HexToAddress("0x888")
	base := model.Tournament{StartInstant: 10, Snapshot: model.TournamentSnapshot{
		AsOfBlock: 50, FinishedAtBlock: 40, Standing: model.TournamentStandingRootWinner, BondRecovery: pendingRootBond(ours)}}
	for _, test := range []struct {
		name   string
		submit bool
		signer common.Address
		change func(*model.Tournament)
		stop   bool
	}{
		{"reader own", false, common.Address{}, func(*model.Tournament) {}, true},
		{"reader foreign", false, common.Address{}, func(v *model.Tournament) { v.Snapshot.BondRecovery = pendingRootBond(foreign) }, true},
		{"submit own", true, ours, func(*model.Tournament) {}, false},
		{"submit foreign", true, foreign, func(*model.Tournament) {}, true},
		{"missing signer", true, common.Address{}, func(*model.Tournament) {}, false},
		{"missing beneficiary", false, common.Address{}, func(v *model.Tournament) { v.Snapshot.BondRecovery.Claimer = nil }, false},
		{"zero beneficiary", false, common.Address{}, func(v *model.Tournament) {
			v.Snapshot.BondRecovery = pendingRootBond(common.Address{})
		}, false},
		{"missing payment", false, common.Address{}, func(v *model.Tournament) { v.Snapshot.BondRecovery.Payment = nil }, false},
		{"unfinished", false, common.Address{}, func(v *model.Tournament) { v.Snapshot.FinishedAtBlock = 0 }, false},
		{"finish not covered", false, common.Address{}, func(v *model.Tournament) { v.Snapshot.FinishedAtBlock = 51 }, false},
		{"before creation", false, common.Address{}, func(v *model.Tournament) { v.StartInstant = 51 }, false},
		{"root with parent", false, common.Address{}, func(v *model.Tournament) { v.ParentTournamentAddress = &foreign }, false},
		{"root with match", false, common.Address{}, func(v *model.Tournament) { v.ParentMatchIDHash = new(common.Hash) }, false},
		{"root with child provenance", false, common.Address{}, func(v *model.Tournament) {
			v.CreationEvent = &model.TournamentCreationEvent{}
		}, false},
		{"inner winner can expire", false, common.Address{}, func(v *model.Tournament) {
			v.Level, v.CreationEvent, v.Snapshot.Standing = 1, &model.TournamentCreationEvent{}, model.TournamentStandingInnerWinner
		}, false},
		{"child as root winner", false, common.Address{}, func(v *model.Tournament) {
			v.Level = 1
			v.CreationEvent = &model.TournamentCreationEvent{}
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.change(&value)
			s := &Service{submissionEnabled: test.submit}
			if test.signer != (common.Address{}) {
				s.txOptsFactory = ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: test.signer})
			}
			require.Equal(t, test.stop, s.tournamentObservationComplete(&value, 50, 100))
			require.Equal(t, test.stop, s.rootObservationCovers(&value, 100))
			require.False(t, s.tournamentObservationComplete(&value, 50, 49))
		})
	}
	s := &Service{submissionEnabled: true,
		txOptsFactory: ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{})}
	require.False(t, s.tournamentObservationComplete(&base, 50, 100), "a zero signer does not select reader-only mode")
}

func TestRootObservationStopsOnlyAfterSuccessfulCommit(t *testing.T) {
	for _, submit := range []bool{false, true} {
		for _, failCommit := range []bool{false, true} {
			name := testReaderRootMode
			if submit {
				name = testForeignRootMode
			}
			if failCommit {
				name += " failed commit then retry"
			}
			t.Run(name, func(t *testing.T) {
				f := newObserverCheckpointFixture(t)
				f.s.submissionEnabled = submit
				if submit {
					f.s.txOptsFactory = ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: common.HexToAddress("0x888")})
				}
				var output bytes.Buffer
				f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
				epoch := checkpointEpoch(0, "0x100")
				address := *epoch.TournamentAddress
				root := &model.Tournament{ApplicationID: f.app.ID, EpochIndex: epoch.Index, Address: address,
					MaxLevel: 1, Kind: model.TournamentKindLeaf, StartInstant: 10, Allowance: 100,
					Snapshot: model.TournamentSnapshot{AsOfBlock: 50, Standing: model.TournamentStandingAwaitingClosure,
						Candidate:    epoch.Commitment,
						BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionTournamentRunning}}}
				before := *root
				turns := 1
				if failCommit {
					turns = 2
				}
				f.repo.storedTournaments = []*model.Tournament{root}
				f.repo.eventCounts = map[common.Address]repository.TournamentEventCounts{address: {CommitmentJoined: 1, StandingChanged: 1}}
				f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).Return(root, nil).Times(turns)
				f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(1), nil).Times(turns)
				adapter := &tournamentAdapterMock{}
				f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Times(turns)
				opts := mock.MatchedBy(resultCallOptsAtBlock(200))
				adapter.On("Descriptor", opts).Return(TournamentDescriptor{BaseCycle: new(big.Int), Kind: root.Kind,
					StartInstant: 10, Allowance: 100}, nil).Times(turns)
				adapter.On("Standing", opts).Return(TournamentStanding{State: model.TournamentStandingRootWinner,
					HasCandidate: true, Candidate: *epoch.Commitment, FinalState: *epoch.MachineHash, FinishedAt: 110}, nil).Times(turns)
				adapter.On("BondRecovery", opts).Return(canonicalBondRecovery(model.BondDispositionRecoverable,
					common.HexToAddress("0x777"), 0), nil).Times(turns)
				current := zeroStructuralEventCounts()
				current.CommitmentJoined.SetInt64(1)
				adapter.On("StructuralEventCounts", opts).Return(current, nil).Times(turns)
				adapter.On("RetrieveAllEvents", mock.Anything, mock.MatchedBy(func(q TournamentEventQuery) bool {
					return q.Start == 51 && q.End == 200 && q.Previous.CommitmentJoined.Int64() == 1 &&
						q.Current.CommitmentJoined.Int64() == 1 && !q.FindRecovery
				})).Return(&TournamentEvents{}, nil).Times(turns)
				commitment := &model.Commitment{ApplicationID: f.app.ID, EpochIndex: epoch.Index, TournamentAddress: address,
					Commitment: *epoch.Commitment, FinalStateHash: *epoch.MachineHash, BlockNumber: 20}
				filterAddress := address.Hex()
				f.repo.On("ListCommitments", mock.Anything, f.app.Name,
					repository.CommitmentFilter{EpochIndex: &epoch.Index, TournamentAddress: &filterAddress},
					repository.Pagination{}, false).
					Return([]*model.Commitment{commitment}, uint64(1), nil).Times(turns)
				f.repo.On("ListMatches", mock.Anything, f.app.Name,
					repository.MatchFilter{EpochIndex: &epoch.Index, TournamentAddress: &filterAddress}, repository.Pagination{}, false).
					Return([]*model.Match{}, uint64(0), nil).Times(turns)
				adapter.On("CommitmentStanding", opts, [32]byte(*epoch.Commitment)).
					Return(CommitmentStanding{Joined: true, FinalState: *epoch.MachineHash}, nil).Times(turns)
				storeArgs := []any{mock.Anything, f.app.ID, mock.Anything, uint64(200)}
				if failCommit {
					failure := errors.New("interval commit failed")
					f.repo.On("StoreTournamentInterval", storeArgs...).Return(failure).Once()
					require.ErrorIs(t, f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch},
						f.consensus, observerFacts(200)), failure)
					require.Equal(t, before, *root)
					require.Empty(t, f.repo.committedTournaments)
					require.NotContains(t, output.String(), stoppedRootMessage)
					require.False(t, f.s.tournamentObservationComplete(root, root.Snapshot.AsOfBlock, 200))
				}
				f.repo.On("StoreTournamentInterval", storeArgs...).Return(nil).Once()
				require.NoError(t, f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, observerFacts(200)))
				committed := f.repo.committedTournaments[address]
				require.True(t, f.s.tournamentObservationComplete(committed, 200, 200))
				require.Equal(t, 1, strings.Count(output.String(), stoppedRootMessage))
				reason := "reader_only"
				if submit {
					reason = "foreign_beneficiary"
					require.Contains(t, output.String(), "signer=")
				}
				require.Contains(t, output.String(), "reason="+reason)
				require.Contains(t, output.String(), "last_observed_block=200")
				frozen := *committed
				require.NoError(t, f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, observerFacts(300)))
				restarted, _ := newPRTServiceMock()
				restarted.repository, restarted.adapterFactory, restarted.client = f.repo, f.factory, f.client
				restarted.submissionEnabled, restarted.txOptsFactory = f.s.submissionEnabled, f.s.txOptsFactory
				restarted.Logger = f.s.Logger
				require.NoError(t, restarted.observeTournamentState(t.Context(), f.app,
					[]*model.Epoch{epoch}, f.consensus, observerFacts(400)))
				batches, err := restarted.gatherTournamentData(t.Context(), f.app, epoch, RootLevel, nil, nil, address, 1, 400, nil, nil)
				require.NoError(t, err)
				require.Empty(t, batches, "gather applies the same durable stop rule before creating an adapter")
				require.Equal(t, frozen, *committed)
				require.Equal(t, 1, strings.Count(output.String(), stoppedRootMessage))
				adapter.AssertExpectations(t)
				f.factory.AssertNumberOfCalls(t, "CreateTournamentAdapter", turns)
			})
		}
	}
}

func TestRootObservationDoesNotLogOwnPendingBondAsStopped(t *testing.T) {
	claimer := common.HexToAddress("0x777")
	s := &Service{submissionEnabled: true,
		txOptsFactory: ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: claimer})}
	var output bytes.Buffer
	s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	app := prtRevertTestApp()

	for _, block := range []uint64{100, 101} {
		root := &model.Tournament{ApplicationID: app.ID, Address: common.HexToAddress("0x100"), StartInstant: 10,
			Snapshot: model.TournamentSnapshot{AsOfBlock: block, FinishedAtBlock: 40,
				Standing: model.TournamentStandingRootWinner, BondRecovery: pendingRootBond(claimer)}}
		// Each call represents the post-commit log step of a complete observation.
		s.logCompletedRootObservation(app, []*repository.TournamentEventBatch{{Tournament: root}}, block)
		require.NotContains(t, output.String(), stoppedRootMessage, "own bond is still pending at block %d", block)
	}
}

func setCompletedRootTestMode(s *Service, root *model.Tournament, mode string) {
	root.CreationEvent = nil
	if mode == testRecoveredRootMode {
		return
	}
	root.Snapshot.BondRecovery = pendingRootBond(common.HexToAddress("0x777"))
	s.submissionEnabled = mode == testForeignRootMode
	if s.submissionEnabled {
		s.txOptsFactory = ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: common.HexToAddress("0x888")})
	}
}
