// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	foreclosureMissingAcceptanceLog    = "acceptance log unavailable"
	foreclosureAcceptanceAboveHead     = "acceptance receipt above observation head"
	foreclosureAcceptedOutputsMismatch = "accepted outputs root mismatch"
	foreclosureKnownRootAbsent         = "known accepted root absent"
	foreclosureAcceptanceMatches       = "matching accepted evidence"
	foreclosureIndependentExit         = "foreclose 8"
	foreclosureMachineHashField        = "machine hash"
)

// These fixtures represent the terminal root result, not bond retirement.
// A finished level-zero winner remains the result while its bond is recoverable.
func foreclosureTerminalRoot(app *model.Application, epoch *model.Epoch, asOf uint64) *model.Tournament {
	return &model.Tournament{ApplicationID: app.ID, EpochIndex: epoch.Index, Address: *epoch.TournamentAddress,
		Snapshot: model.TournamentSnapshot{AsOfBlock: asOf, FinishedAtBlock: 80,
			Standing: model.TournamentStandingRootWinner, FinalStateHash: epoch.MachineHash,
			WinnerCommitment: epoch.Commitment, Candidate: epoch.Commitment,
			BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionRecoverable,
				Claimer: new(common.HexToAddress("0x123")), Payment: new(model.Uint256)}}}
}

func TestForeclosedRootResultCovers_ExplicitEvidenceVectors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.Tournament)
		want   bool
	}{
		{"old recoverable winner", func(*model.Tournament) {}, true},
		{"winner at head", func(root *model.Tournament) { root.Snapshot.AsOfBlock = 120 }, true},
		{"recovered winner", func(root *model.Tournament) {
			root.Snapshot.BondRecovery.Disposition = model.BondDispositionRecovered
		}, true},
		{"terminal root failure", func(root *model.Tournament) {
			root.Snapshot.Standing = model.TournamentStandingRootFailed
			root.Snapshot.FinalStateHash, root.Snapshot.WinnerCommitment = nil, nil
			root.Snapshot.BondRecovery.Disposition = model.BondDispositionNoWinner
		}, true},
		{"future snapshot", func(root *model.Tournament) { root.Snapshot.AsOfBlock = 121 }, false},
		{"unfinished root", func(root *model.Tournament) {
			root.Snapshot.FinishedAtBlock = 0
			root.Snapshot.Standing = model.TournamentStandingMatchesActive
			root.Snapshot.FinalStateHash, root.Snapshot.WinnerCommitment = nil, nil
		}, false},
		{"finish after observation", func(root *model.Tournament) { root.Snapshot.FinishedAtBlock = 91 }, false},
		// Identity and shape vectors test rejection. They are not claims that
		// constrained PostgreSQL rows can be produced by normal ingestion.
		{"rejection vector other application", func(root *model.Tournament) { root.ApplicationID++ }, false},
		{"rejection vector other epoch", func(root *model.Tournament) { root.EpochIndex++ }, false},
		{"rejection vector other address", func(root *model.Tournament) { root.Address = common.HexToAddress("0x999") }, false},
		{"rejection vector inner level", func(root *model.Tournament) { root.Level = 1 }, false},
		{"rejection vector winner lacks final state", func(root *model.Tournament) { root.Snapshot.FinalStateHash = nil }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := prtForeclosedApp(1, 100)
			epoch := checkpointEpoch(6, "0x600")
			epoch.ApplicationID = app.ID
			root := foreclosureTerminalRoot(app, epoch, 90)
			tc.change(root)
			require.Equal(t, tc.want, foreclosedRootResultCovers(app, epoch, root, 120))
		})
	}
	t.Run("absent root", func(t *testing.T) {
		app := prtForeclosedApp(1, 100)
		require.False(t, foreclosedRootResultCovers(app, checkpointEpoch(6, "0x600"), nil, 120))
	})
}

func TestPRTForeclosureOldRecoverableRootAcceptsLaterReceipt(t *testing.T) {
	for _, submission := range []bool{false, true} {
		for _, owner := range []common.Address{common.HexToAddress("0x123"), common.HexToAddress("0x456")} {
			t.Run(fmt.Sprintf("submission=%t owner=%s", submission, owner), func(t *testing.T) {
				f := newAcceptanceReceiptFixture(t)
				f.app.ForecloseBlock = 105
				f.epoch.ApplicationID = f.app.ID
				*f.root = *foreclosureTerminalRoot(f.app, f.epoch, 90)
				f.root.Snapshot.BondRecovery.Claimer = &owner
				f.service.submissionEnabled = submission
				f.service.txOptsFactory = ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: common.HexToAddress("0x123")})
				f.expectReceipt()
				f.expectAcceptance()

				deferred, err := f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, 120)

				require.NoError(t, err)
				require.False(t, deferred, "the terminal result at block 90 covers acceptance at block 100 before foreclosure at block 105")
				f.repo.AssertNumberOfCalls(t, "UpdateEpochWithAcceptedClaim", 1)
			})
		}
	}
}

func TestPRTForeclosureKnownAcceptancePrecedesLaterDivergence(t *testing.T) {
	for _, pending := range []string{
		foreclosureKnownRootAbsent, "root still active", foreclosureMissingAcceptanceLog, foreclosureAcceptanceAboveHead,
	} {
		for _, mismatch := range []string{foreclosureAcceptanceMatches, "accepted state", foreclosureAcceptedOutputsMismatch} {
			t.Run(pending+"/"+mismatch, func(t *testing.T) {
				s, repo := newPRTServiceMock()
				app := prtForeclosedApp(1, 95)
				admitTestDeployment(s, repo, app)
				earlier, later := checkpointEpoch(6, "0x600"), checkpointEpoch(7, "0x700")
				earlier.ApplicationID, later.ApplicationID = app.ID, app.ID
				earlier.ClaimTransactionHash = new(common.HexToHash("0xabc6"))
				open := &model.Epoch{ApplicationID: app.ID, Index: 8, FirstBlock: 90, Status: model.EpochStatus_Open}
				var firstRoot *model.Tournament
				if pending != foreclosureKnownRootAbsent {
					firstRoot = foreclosureTerminalRoot(app, earlier, 90)
					if pending == "root still active" {
						firstRoot.Snapshot = model.TournamentSnapshot{AsOfBlock: 70, Standing: model.TournamentStandingMatchesActive}
					}
				}
				laterRoot := foreclosureTerminalRoot(app, later, 90)
				laterRoot.Snapshot.FinalStateHash = new(common.HexToHash("0xbad7"))
				laterRoot.Snapshot.WinnerCommitment = new(common.HexToHash("0xbad70"))
				laterRoot.Snapshot.Candidate = laterRoot.Snapshot.WinnerCommitment
				repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(false, nil).Twice()
				repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(true, nil).Twice()
				repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), earlier.TournamentAddress.Hex()).
					Return(firstRoot, nil).Once()
				if pending != foreclosureMissingAcceptanceLog {
					repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
						repository.Pagination{}, false).Return([]*model.Epoch{earlier, later, open}, uint64(3), nil).Once()
					repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, later.Index, uint64(100)).Return(repository.ErrNoUpdate).Once()
				}
				var order []string
				var reasons []string
				repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
					Run(func(args mock.Arguments) {
						reason := *args.Get(3).(*string)
						reasons = append(reasons, reason)
						var index uint64
						_, err := fmt.Sscanf(reason, "Epoch %d", &index)
						require.NoError(t, err)
						order = append(order, fmt.Sprintf("diverge %d", index))
					}).Return(nil).Maybe()
				repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(),
					repository.EpochFilter{HasTournament: new(true), Status: model.NonTerminalEpochStatuses()},
					repository.Pagination{}, false).
					Return([]*model.Epoch{}, uint64(0), nil).Maybe()
				// The empty retry page lets a missing later verdict fail its semantic
				// assertion instead of causing an unexpected-mock panic.
				repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
					repository.Pagination{}, false).Return([]*model.Epoch{}, uint64(0), nil).Maybe()
				if pending != foreclosureMissingAcceptanceLog {
					repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, open.Index, uint64(100)).
						Run(func(mock.Arguments) { order = append(order, foreclosureIndependentExit) }).Return(nil).Once()
				}
				client := &ethClientMock{}
				s.client = client
				consensus := newCompatibleDaveConsensusAdapterMock()
				header := &types.Header{Number: big.NewInt(90)}
				raw := &types.Log{Address: app.IConsensusAddress, TxHash: *earlier.ClaimTransactionHash,
					BlockNumber: 90, BlockHash: header.Hash()}
				receipt := &types.Receipt{TxHash: raw.TxHash, BlockNumber: big.NewInt(90), BlockHash: raw.BlockHash,
					Status: types.ReceiptStatusSuccessful, Logs: []*types.Log{raw}}
				if pending == foreclosureAcceptanceAboveHead {
					future := *receipt
					future.BlockNumber = big.NewInt(110)
					// A future receipt is a rejection vector for a changing provider
					// view, not coherent acceptance after the recorded foreclosure.
					client.On("TransactionReceipt", mock.Anything, *earlier.ClaimTransactionHash).Return(&future, nil).Once()
				}
				if pending == foreclosureMissingAcceptanceLog {
					missing := *receipt
					missing.Logs = nil
					client.On("TransactionReceipt", mock.Anything, *earlier.ClaimTransactionHash).Return(&missing, nil).Once()
					client.On("HeaderByNumber", mock.Anything, big.NewInt(90)).Return(header, nil).Once()
				}
				var logs bytes.Buffer
				s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))

				firstErr := s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{earlier, later, open}, consensus, 100)
				if pending == foreclosureMissingAcceptanceLog {
					require.ErrorIs(t, firstErr, txsender.ErrInconclusive)
					require.Empty(t, order)
				} else {
					require.NoError(t, firstErr)
					require.Equal(t, []string{foreclosureIndependentExit}, order)
				}
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				repo.AssertNotCalled(t, "GetTournament", mock.Anything, app.IApplicationAddress.Hex(), later.TournamentAddress.Hex())
				if pending == foreclosureMissingAcceptanceLog {
					require.Empty(t, loggedForeclosedEpochs(t, &logs))
				} else {
					require.Equal(t, []uint64{8}, loggedForeclosedEpochs(t, &logs))
				}
				repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

				// A later head permits comparison of the earlier known acceptance.
				// Its state and outputs root are independent fixture values; a later
				// false root must neither skip this comparison nor suppress itself.
				repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), earlier.TournamentAddress.Hex()).
					Return(foreclosureTerminalRoot(app, earlier, 90), nil).Once()
				client.On("TransactionReceipt", mock.Anything, *earlier.ClaimTransactionHash).Return(receipt, nil).Once()
				client.On("HeaderByNumber", mock.Anything, big.NewInt(90)).Return(header, nil).Once()
				acceptedState, acceptedOutputs := common.HexToHash("0x400"), common.HexToHash("0x500")
				require.Equal(t, acceptedState, *earlier.MachineHash)
				require.Equal(t, acceptedOutputs, *earlier.TxBufferDataBlock)
				if mismatch == "accepted state" {
					acceptedState = common.HexToHash("0xbad6")
				}
				if mismatch == foreclosureAcceptedOutputsMismatch {
					acceptedOutputs = common.HexToHash("0xbad6")
				}
				consensus.On("ParseEpochSealed", *raw).Return(&idaveconsensus.IDaveConsensusEpochSealed{
					EpochNumber: big.NewInt(7), InitialMachineStateHash: acceptedState, OutputsMerkleRoot: acceptedOutputs, Raw: *raw,
				}, nil).Once()
				wantEpoch, wantField := uint64(7), foreclosureMachineHashField
				if mismatch == foreclosureAcceptanceMatches {
					repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, earlier.Index, earlier.ClaimTransactionHash).
						Run(func(mock.Arguments) { order = append(order, "accept 6") }).Return(nil).Once()
					repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), later.TournamentAddress.Hex()).
						Return(laterRoot, nil).Once()
				} else {
					wantEpoch = 6
					if mismatch == foreclosureAcceptedOutputsMismatch {
						wantField = "claim hash"
					}
				}
				cause := fmt.Sprintf("Epoch %d has inconsistent %s", wantEpoch, wantField)

				err := s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{earlier, later}, consensus, 121)

				require.ErrorContains(t, err, cause)
				require.Len(t, reasons, 1)
				require.Contains(t, reasons[0], cause)
				var expectedOrder []string
				if pending != foreclosureMissingAcceptanceLog {
					expectedOrder = append(expectedOrder, foreclosureIndependentExit)
				}
				if mismatch == foreclosureAcceptanceMatches {
					expectedOrder = append(expectedOrder, "accept 6", "diverge 7")
				} else {
					expectedOrder = append(expectedOrder, "diverge 6")
				}
				require.Equal(t, expectedOrder, order)
				require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
				repo.AssertExpectations(t)
				client.AssertExpectations(t)
				consensus.AssertExpectations(t)
			})
		}
	}
}

func TestPRTForeclosurePropagatesObservedHeadBeyondForeclosure(t *testing.T) {
	s, repo := newPRTServiceMock()
	app := prtForeclosedApp(1, 95)
	admitTestDeployment(s, repo, app)
	epoch := checkpointEpoch(6, "0x600")
	epoch.ApplicationID = app.ID
	repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(false, nil).Once()
	repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(true, nil).Once()
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
		Return(foreclosureTerminalRoot(app, epoch, 90), nil).Once()
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
	var writtenHead uint64
	repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, epoch.Index, mock.Anything).
		Run(func(args mock.Arguments) { writtenHead = args.Get(3).(uint64) }).Return(nil).Once()

	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch}, nil, 120))
	require.Equal(t, uint64(120), writtenHead, "the repository must receive the selected head, not the foreclosure block")
	repo.AssertExpectations(t)
}

func TestPRTForeclosureRootWaitSurvivesFullPruningPasses(t *testing.T) {
	s, repo := newPRTServiceMock()
	app := prtForeclosedApp(1, 95)
	admitTestDeployment(s, repo, app)
	epoch := checkpointEpoch(6, "0x600")
	epoch.ApplicationID = app.ID
	epoch.ClaimTransactionHash = new(common.HexToHash("0xabc6"))
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(false, nil).Twice()
	repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(true, nil).Twice()
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).Return(nil, nil).Twice()
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Twice()
	for _, head := range []uint64{100, 101} {
		s.reportProtectionStops(t.Context(), app, nil, txsender.BlockRef{Number: head}, deploymentCompatible, 0, nil)
		require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch}, nil, head))
	}
	require.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("Foreclosed PRT epoch awaits")))
	require.Empty(t, loggedForeclosedEpochs(t, &logs))
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{}, uint64(0), nil).Once()
	require.NoError(t, s.foreclosePendingClaimEpochs(t.Context(), app, 102))
	require.Empty(t, s.protectionNotices[app.ID])
	repo.AssertExpectations(t)
}

func TestPRTForeclosureConditionalRefusalNoticeAcrossFullPasses(t *testing.T) {
	for _, generation := range []bool{false, true} {
		t.Run(fmt.Sprintf("generation=%t", generation), func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtForeclosedApp(1, 95)
			admitTestDeployment(s, repo, app)
			if generation {
				setTestDeployment(s, repo, app, 1, deploymentGeneration, new(model.DeploymentTierGeneration))
			}
			epoch := checkpointEpoch(6, "0x600")
			epoch.ApplicationID = app.ID
			var logs bytes.Buffer
			s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(false, nil).Times(3)
			repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(true, nil).Times(3)
			repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
				repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Times(3)
			if generation {
				attempt := 0
				repo.deploymentDrain = func(_ context.Context, request repository.DeploymentDrainRequest) error {
					require.Equal(t, epoch.Index, request.EpochIndex)
					attempt++
					if attempt < 3 {
						return repository.ErrNoUpdate
					}
					return nil
				}
			} else {
				for _, head := range []uint64{100, 101} {
					repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, epoch.Index, head).Return(repository.ErrNoUpdate).Once()
				}
				repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, epoch.Index, uint64(125)).Return(nil).Once()
			}
			for _, head := range []uint64{100, 101} {
				s.reportProtectionStops(t.Context(), app, nil, txsender.BlockRef{Number: head}, deploymentCompatible, 0, nil)
				require.NoError(t, s.drainForeclosedClaims(t.Context(), app, nil, nil, head))
			}
			require.NotEmpty(t, s.protectionNotices[app.ID], "an unresolved conditional refusal must retain its bounded notice")
			require.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("write deferred")))
			require.NotContains(t, logs.String(), "Terminalized")

			require.NoError(t, s.drainForeclosedClaims(t.Context(), app, nil, nil, 125))

			require.Empty(t, s.protectionNotices[app.ID], "a successful write clears the refusal incident")
			require.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("Terminalized")))
			repo.AssertExpectations(t)
		})
	}
}

func TestPRTForeclosureRetiredRootOutlivesLocalValidation(t *testing.T) {
	s, repo := newPRTServiceMock()
	app := prtForeclosedApp(1, 95)
	admitTestDeployment(s, repo, app)
	epoch := checkpointEpoch(6, "0x600")
	epoch.ApplicationID, epoch.Status = app.ID, model.EpochStatus_Closed
	root := foreclosureTerminalRoot(app, epoch, 90)
	root.Snapshot.BondRecovery = model.TournamentBondRecovery{Disposition: model.BondDispositionRecovered}
	published := *epoch
	epoch.Commitment, epoch.CommitmentProof = nil, nil
	epoch.MachineHash, epoch.TxBufferDataBlock = nil, nil
	require.True(t, s.tournamentObservationComplete(root, 90, 100))
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(false, nil).Twice()
	repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(true, nil).Twice()
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Twice()
	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch}, nil, 100))
	repo.AssertNotCalled(t, "ForeclosePRTEpoch", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	// Supply the existing validator's later publication. The root observation
	// stays unchanged; local readiness must not require another tournament scan.
	*epoch = published
	epoch.Status = model.EpochStatus_ClaimComputed
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).Return(root, nil).Once()
	repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, epoch.Index, uint64(120)).Return(nil).Once()
	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch}, nil, 120))
	require.Equal(t, uint64(90), root.Snapshot.AsOfBlock)
	require.NotContains(t, logs.String(), "Foreclosed PRT epoch awaits")
	require.Equal(t, []uint64{6}, loggedForeclosedEpochs(t, &logs))
	repo.AssertExpectations(t)
}

func TestPRTForeclosureEarlierAcceptancePrecedesLaterAcceptedResult(t *testing.T) {
	for _, mismatch := range []string{foreclosureMachineHashField, "claim hash"} {
		t.Run(mismatch, func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtForeclosedApp(1, 95)
			admitTestDeployment(s, repo, app)
			earlier, later := checkpointEpoch(6, "0x600"), checkpointEpoch(7, "0x700")
			earlier.ApplicationID, later.ApplicationID = app.ID, app.ID
			earlier.ClaimTransactionHash = new(common.HexToHash("0xabc6"))
			later.ClaimTransactionHash = new(common.HexToHash("0xabc7"))
			open := &model.Epoch{ApplicationID: app.ID, Index: 8, FirstBlock: 90, Status: model.EpochStatus_Open}
			repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(false, nil).Twice()
			repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(true, nil).Twice()
			repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), earlier.TournamentAddress.Hex()).
				Return((*model.Tournament)(nil), nil).Once()
			repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
				repository.Pagination{}, false).Return([]*model.Epoch{earlier, later, open}, uint64(3), nil).Once()
			var order []string
			repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, open.Index, uint64(100)).
				Run(func(mock.Arguments) { order = append(order, foreclosureIndependentExit) }).Return(nil).Once()
			client := &ethClientMock{}
			s.client = client
			consensus := newCompatibleDaveConsensusAdapterMock()

			require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{earlier, later, open}, consensus, 100))

			require.Equal(t, []string{foreclosureIndependentExit}, order)
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			repo.AssertNotCalled(t, "GetTournament", mock.Anything, app.IApplicationAddress.Hex(), later.TournamentAddress.Hex())
			client.AssertNotCalled(t, "TransactionReceipt", mock.Anything, *later.ClaimTransactionHash)
			for _, epoch := range []*model.Epoch{earlier, later} {
				repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
					Return(foreclosureTerminalRoot(app, epoch, 90), nil).Once()
				header := &types.Header{Number: new(big.Int).SetUint64(epoch.Index + 84)}
				raw := &types.Log{Address: app.IConsensusAddress, TxHash: *epoch.ClaimTransactionHash,
					BlockNumber: header.Number.Uint64(), BlockHash: header.Hash()}
				receipt := &types.Receipt{TxHash: raw.TxHash, BlockNumber: header.Number, BlockHash: raw.BlockHash,
					Status: types.ReceiptStatusSuccessful, Logs: []*types.Log{raw}}
				client.On("TransactionReceipt", mock.Anything, *epoch.ClaimTransactionHash).Return(receipt, nil).Once()
				client.On("HeaderByNumber", mock.Anything, header.Number).Return(header, nil).Once()
				state, outputs := common.HexToHash("0x400"), common.HexToHash("0x500")
				require.Equal(t, state, *epoch.MachineHash)
				require.Equal(t, outputs, *epoch.TxBufferDataBlock)
				if epoch == later {
					if mismatch == foreclosureMachineHashField {
						state = common.HexToHash("0xbad7")
					} else {
						outputs = common.HexToHash("0xbad7")
					}
				}
				consensus.On("ParseEpochSealed", *raw).Return(&idaveconsensus.IDaveConsensusEpochSealed{
					EpochNumber: new(big.Int).SetUint64(epoch.Index + 1), InitialMachineStateHash: state,
					OutputsMerkleRoot: outputs, Raw: *raw,
				}, nil).Once()
			}
			repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, earlier.Index, earlier.ClaimTransactionHash).
				Run(func(mock.Arguments) { order = append(order, "accept 6") }).Return(nil).Once()
			repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
				Run(func(mock.Arguments) { order = append(order, "diverge 7") }).Return(nil).Once()
			repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(),
				repository.EpochFilter{HasTournament: new(true), Status: model.NonTerminalEpochStatuses()},
				repository.Pagination{}, false).Return([]*model.Epoch{}, uint64(0), nil).Once()

			err := s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{earlier, later}, consensus, 120)

			require.ErrorContains(t, err, "Epoch 7 has inconsistent "+mismatch)
			require.Equal(t, []string{foreclosureIndependentExit, "accept 6", "diverge 7"}, order)
			require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
			repo.AssertExpectations(t)
			client.AssertExpectations(t)
			consensus.AssertExpectations(t)
		})
	}
}

func TestPRTForeclosureRootWaitBecomesConditionalRefusal(t *testing.T) {
	s, repo := newPRTServiceMock()
	app := prtForeclosedApp(1, 95)
	admitTestDeployment(s, repo, app)
	epoch := checkpointEpoch(6, "0x600")
	epoch.ApplicationID = app.ID
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(false, nil).Times(3)
	repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(95)).Return(true, nil).Times(3)
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Times(3)
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
		Return((*model.Tournament)(nil), nil).Once()
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
		Return(foreclosureTerminalRoot(app, epoch, 90), nil).Twice()
	for _, head := range []uint64{100, 101} {
		repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, epoch.Index, head).Return(repository.ErrNoUpdate).Once()
	}
	repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, epoch.Index, uint64(102)).Return(nil).Once()

	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch}, nil, 100))
	require.Contains(t, logs.String(), "Foreclosed PRT epoch awaits")
	require.NotContains(t, logs.String(), "write deferred")
	require.Contains(t, s.protectionNotices[app.ID], protectionNoticeKey{root: *epoch.TournamentAddress, kind: protectionForeclosureWait})

	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch}, nil, 101))
	require.NotContains(t, s.protectionNotices[app.ID],
		protectionNoticeKey{root: *epoch.TournamentAddress, kind: protectionForeclosureWait})
	require.Contains(t, s.protectionNotices[app.ID],
		protectionNoticeKey{root: *epoch.TournamentAddress, kind: protectionForeclosureRefusal})
	require.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("write deferred")))
	require.Empty(t, loggedForeclosedEpochs(t, &logs))

	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch}, nil, 102))
	require.Empty(t, s.protectionNotices[app.ID])
	require.Equal(t, []uint64{epoch.Index}, loggedForeclosedEpochs(t, &logs))
	repo.AssertExpectations(t)
}

func TestPRTForeclosurePendingRootPreservesLaterResult(t *testing.T) {
	s, repo := newPRTServiceMock()
	app := prtRevertTestApp()
	app.ForecloseBlock = 100
	admitTestDeployment(s, repo, app)
	earlier, later := checkpointEpoch(6, "0x600"), checkpointEpoch(7, "0x700")
	earlier.ApplicationID, later.ApplicationID = app.ID, app.ID
	// Ingestion stores this hash before creating the successor's seal.
	earlier.ClaimTransactionHash = new(common.HexToHash("0xabc6"))
	laterRoot := foreclosureTerminalRoot(app, later, 100)
	laterRoot.Snapshot.FinalStateHash = new(common.HexToHash("0xbad7"))
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), earlier.TournamentAddress.Hex()).
		Return((*model.Tournament)(nil), nil).Once()
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), later.TournamentAddress.Hex()).
		Return(laterRoot, nil).Maybe()
	repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Maybe()
	repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(),
		repository.EpochFilter{HasTournament: new(true), Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{}, uint64(0), nil).Maybe()

	deferred, err := s.reconcileAcceptedEpochs(t.Context(), app, []*model.Epoch{earlier, later}, nil, 100)

	require.NoError(t, err)
	require.False(t, deferred)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	require.Equal(t, model.EpochStatus_ClaimComputed, later.Status)
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}
