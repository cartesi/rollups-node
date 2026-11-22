// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const (
	prtDisabledAdmissionCase = "disabled before foreclosure admission"
	prtWrongEpochCase        = "root belongs to another epoch"
	prtForeclosureHead       = uint64(20)
	prtAwaitingClosureCase   = "awaiting closure"
	prtWrongApplicationCase  = "root belongs to another application"
	prtOlderRecoverableCase  = "old recoverable observation"
	prtProblemAtTargetCase   = "problem at target"
)

func prtForeclosureFixture(t *testing.T, status model.EpochStatus) (*PostgresRepository, *model.Application, *model.Epoch) {
	t.Helper()
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch := seedEpochPublication(t, r)
	address := repotest.UniqueAddress()
	_, err := r.db.Exec(t.Context(), `UPDATE epoch SET tournament_address=$3 WHERE application_id=$1 AND index=$2`,
		app.ID, epoch.Index, address[:])
	require.NoError(t, err)
	if status != model.EpochStatus_Closed {
		require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, repotest.DummyStateProof()))
	}
	if status == model.EpochStatus_ClaimComputed {
		completeDeploymentDrainProofs(t, r, app, epoch)
	}
	require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID,
		prtForeclosureHead, repotest.UniqueHash(), prtForeclosureHead))
	_, err = r.db.Exec(t.Context(), `UPDATE application SET last_epoch_check_block=$2,last_input_check_block=$2 WHERE id=$1`,
		app.ID, prtForeclosureHead)
	require.NoError(t, err)
	epoch, err = r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	return r, app, epoch
}

func prtForeclosureRoot(app *model.Application, epoch *model.Epoch) *model.Tournament {
	root := repotest.NewTournamentBuilder(app.ID).WithAddress(*epoch.TournamentAddress).WithEpochIndex(epoch.Index).Build()
	root.StartInstant = epoch.LastBlock
	// A different commitment identity does not prove different execution.
	winner := repotest.UniqueHash()
	root.Snapshot = model.TournamentSnapshot{
		AsOfBlock: prtForeclosureHead, Standing: model.TournamentStandingRootWinner,
		Candidate: &winner, WinnerCommitment: &winner, FinalStateHash: epoch.MachineHash,
		FinishedAtBlock: prtForeclosureHead - 1,
		BondRecovery:    model.TournamentBondRecovery{Disposition: model.BondDispositionRecovered},
	}
	return root
}

func TestPRTForeclosureRequiresExactCoveredTerminalRoot(t *testing.T) {
	for _, tc := range []struct {
		name                                               string
		allow, missing, wrongAddress, wrongEpoch, wrongApp bool
		differentState, recoverable                        bool
		standing                                           model.TournamentStandingState
		asOf, finished, head                               uint64
	}{
		{name: "matching winner", allow: true},
		{name: "failed root", standing: model.TournamentStandingRootFailed, allow: true},
		{name: "older failed root", standing: model.TournamentStandingRootFailed, head: 25, allow: true},
		{name: "retired older root", asOf: 19, finished: 18, allow: true},
		{name: "recoverable at head", recoverable: true, allow: true},
		{name: "recoverable above foreclosure at head", recoverable: true, asOf: 25, head: 25, allow: true},
		{name: prtOlderRecoverableCase, recoverable: true, head: 25, allow: true},
		{name: "missing root", missing: true},
		{name: "active matches", standing: model.TournamentStandingMatchesActive},
		{name: prtAwaitingClosureCase, standing: model.TournamentStandingAwaitingClosure},
		{name: "wrong address", wrongAddress: true},
		{name: prtWrongEpochCase, wrongEpoch: true},
		{name: prtWrongApplicationCase, wrongApp: true},
		{name: "different final state", differentState: true},
		{name: "older recoverable different final state", differentState: true, recoverable: true, head: 25},
		{name: "future observation", asOf: 21},
		// The root is covered at head 19. Only foreclosure admission refuses it.
		{name: "head before foreclosure", head: 19, asOf: 18, finished: 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, app, epoch := prtForeclosureFixture(t, model.EpochStatus_ClaimComputed)
			root := prtForeclosureRoot(app, epoch)
			rootApp := app
			head := prtForeclosureHead
			if tc.head != 0 {
				head = tc.head
			}
			if tc.asOf != 0 {
				root.Snapshot.AsOfBlock = tc.asOf
			}
			if tc.finished != 0 {
				root.Snapshot.FinishedAtBlock = tc.finished
			}
			if tc.standing != "" {
				root.Snapshot.Standing = tc.standing
				root.Snapshot.Candidate, root.Snapshot.WinnerCommitment, root.Snapshot.FinalStateHash = nil, nil, nil
				root.Snapshot.BondRecovery.Disposition = model.BondDispositionNoWinner
				if tc.standing != model.TournamentStandingRootFailed {
					root.Snapshot.FinishedAtBlock = 0
					root.Snapshot.BondRecovery.Disposition = model.BondDispositionTournamentRunning
				}
			}
			if tc.recoverable {
				root.Snapshot.BondRecovery = model.TournamentBondRecovery{Disposition: model.BondDispositionRecoverable,
					Claimer: new(repotest.UniqueAddress()), Payment: new(model.Uint256)}
			}
			if tc.wrongAddress {
				root.Address = repotest.UniqueAddress()
			}
			if tc.wrongEpoch || tc.wrongApp {
				otherIndex := uint64(1)
				if tc.wrongApp {
					rootApp = repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
					otherIndex = 0
				}
				other := repotest.NewEpochBuilder(rootApp.ID).WithIndex(otherIndex).
					WithStatus(model.EpochStatus_Closed).WithBlocks(9, 19).WithInputBounds(1, 1).Build()
				other.TournamentAddress = &root.Address
				require.NoError(t, r.CreateEpochsAndInputs(t.Context(), rootApp.Name,
					map[*model.Epoch][]*model.Input{other: {}}, prtForeclosureHead))
				root.ApplicationID, root.EpochIndex = rootApp.ID, otherIndex
			}
			if tc.differentState {
				root.Snapshot.FinalStateHash = new(repotest.UniqueHash())
			}
			if !tc.missing {
				require.NoError(t, r.CreateTournament(t.Context(), rootApp.Name, root))
			}
			outputBefore, err := r.GetOutput(t.Context(), app.Name, 0)
			require.NoError(t, err)
			err = r.ForeclosePRTEpoch(t.Context(), app.ID, epoch.Index, head)
			if tc.allow {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, repository.ErrNoUpdate)
			}
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			expected := *epoch
			if tc.allow {
				expected.Status, expected.UpdatedAt = model.EpochStatus_ClaimForeclosed, after.UpdatedAt
			}
			require.Equal(t, expected, *after, "closure must preserve the complete claim and state proof")
			outputAfter, err := r.GetOutput(t.Context(), app.Name, 0)
			require.NoError(t, err)
			require.Equal(t, outputBefore, outputAfter)
		})
	}
}

func TestPRTForeclosureAdmissionAndPrefix(t *testing.T) {
	for _, tc := range []struct {
		name  string
		allow bool
	}{
		{"healthy", true}, {"physical prefix", true},
		{prtDisabledAdmissionCase, false}, {string(model.ApplicationStatus_Failed), false},
		{string(model.ApplicationStatus_Diverged), false}, {string(model.ApplicationStatus_Corrupted), false},
		{"incomplete input scan", false}, {"incomplete epoch scan", false},
		{"known transaction", false}, {"accepted", false}, {"wrong consensus", false},
		{prtProblemAtTargetCase, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, app, epoch := prtForeclosureFixture(t, model.EpochStatus_ClaimComputed)
			require.NoError(t, r.CreateTournament(t.Context(), app.Name, prtForeclosureRoot(app, epoch)))
			var err error
			switch tc.name {
			case prtDisabledAdmissionCase:
				err = r.UpdateApplicationEnabled(t.Context(), app.ID, false)
			case string(model.ApplicationStatus_Failed), string(model.ApplicationStatus_Diverged),
				string(model.ApplicationStatus_Corrupted):
				err = r.UpdateApplicationStatus(t.Context(), app.ID, model.ApplicationStatus(tc.name), new("independent stop"))
			case "incomplete input scan":
				_, err = r.db.Exec(t.Context(), `UPDATE application SET last_input_check_block=19 WHERE id=$1`, app.ID)
			case "incomplete epoch scan":
				_, err = r.db.Exec(t.Context(), `UPDATE application SET last_epoch_check_block=19 WHERE id=$1`, app.ID)
			case "known transaction":
				err = r.UpdateEpochWithSubmittedClaim(t.Context(), app.ID, epoch.Index, repotest.UniqueHash())
			case "accepted":
				err = r.UpdateEpochWithAcceptedClaim(t.Context(), app.ID, epoch.Index, new(repotest.UniqueHash()))
			case "wrong consensus":
				_, err = r.db.Exec(t.Context(), `UPDATE application SET consensus_type=$2 WHERE id=$1`,
					app.ID, model.Consensus_Authority.String())
			case "physical prefix", prtProblemAtTargetCase:
				kind, problem := model.ApplicationStatus_MachineHalted, uint64(1)
				input := uint64(1)
				if tc.name == prtProblemAtTargetCase {
					problem = 0
				}
				_, err = r.db.Exec(t.Context(), `UPDATE application SET status=$2,reason='typed stop',
				 problem_epoch_index=$3,problem_kind=$2,problem_input_index=$4 WHERE id=$1`, app.ID, kind.String(), problem, input)
			}
			require.NoError(t, err)
			before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			err = r.ForeclosePRTEpoch(t.Context(), app.ID, epoch.Index, prtForeclosureHead)
			if tc.allow {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, repository.ErrNoUpdate)
			}
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			if !tc.allow {
				require.Equal(t, before, after)
			}
		})
	}
}

func TestPRTForeclosurePreservesSealedProducerIncludingSameBlock(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_Closed, model.EpochStatus_InputsProcessed} {
		for _, sealBlock := range []uint64{9, prtForeclosureHead} {
			t.Run(status.String()+"/"+strconv.FormatUint(sealBlock, 10), func(t *testing.T) {
				r, app, epoch := prtForeclosureFixture(t, status)
				_, err := r.db.Exec(t.Context(), `UPDATE epoch SET last_block=$3 WHERE application_id=$1 AND index=$2`,
					app.ID, epoch.Index, sealBlock)
				require.NoError(t, err)
				before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
				require.NoError(t, err)
				require.ErrorIs(t, r.ForeclosePRTEpoch(t.Context(), app.ID, epoch.Index, prtForeclosureHead), repository.ErrNoUpdate)
				after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
				require.NoError(t, err)
				require.Equal(t, before, after)
				retained, err := r.HasUnpublishedEpochBeforeBlock(t.Context(), app.ID, prtForeclosureHead)
				require.NoError(t, err)
				require.Equal(t, status == model.EpochStatus_Closed, retained)
			})
		}
	}
}

func TestPRTForeclosureStaleIPDiscoveryUsesDurableComparison(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "running root refuses", true: "matching root closes"}[allow], func(t *testing.T) {
			r, app, selected := prtForeclosureFixture(t, model.EpochStatus_InputsProcessed)
			require.Equal(t, model.EpochStatus_InputsProcessed, selected.Status)
			completeDeploymentDrainProofs(t, r, app, selected)
			computed, err := r.GetEpoch(t.Context(), app.Name, selected.Index)
			require.NoError(t, err)
			root := prtForeclosureRoot(app, computed)
			if !allow {
				root.Snapshot = model.TournamentSnapshot{AsOfBlock: prtForeclosureHead, Standing: model.TournamentStandingMatchesActive,
					BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionTournamentRunning}}
			}
			require.NoError(t, r.CreateTournament(t.Context(), app.Name, root))
			err = r.ForeclosePRTEpoch(t.Context(), app.ID, selected.Index, prtForeclosureHead)
			if allow {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, repository.ErrNoUpdate)
			}
		})
	}
}

func TestPRTForeclosureCurrentTargetRecheck(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "claim transaction", true: "accepted target"}[accepted], func(t *testing.T) {
			r, app, epoch := prtForeclosureFixture(t, model.EpochStatus_ClaimComputed)
			require.NoError(t, r.CreateTournament(t.Context(), app.Name, prtForeclosureRoot(app, epoch)))
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			lock, err := r.db.Begin(ctx)
			require.NoError(t, err)
			defer rollbackDeliveryTx(lock)
			var marker int
			require.NoError(t, lock.QueryRow(ctx, `SELECT 1 FROM epoch WHERE application_id=$1 AND index=$2 FOR UPDATE`,
				app.ID, epoch.Index).Scan(&marker))
			done := make(chan error, 1)
			go func() { done <- r.ForeclosePRTEpoch(ctx, app.ID, epoch.Index, prtForeclosureHead) }()
			waitDeploymentEpochBlocker(t, r, lock.Conn().PgConn().PID())
			status := model.EpochStatus_ClaimSubmitted
			if accepted {
				status = model.EpochStatus_ClaimAccepted
			}
			hash := repotest.UniqueHash()
			_, err = lock.Exec(ctx, `UPDATE epoch SET status=$3,claim_transaction_hash=$4 WHERE application_id=$1 AND index=$2`,
				app.ID, epoch.Index, status.String(), hash[:])
			require.NoError(t, err)
			require.NoError(t, lock.Commit(ctx))
			require.ErrorIs(t, <-done, repository.ErrNoUpdate)
			after, err := r.GetEpoch(ctx, app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, status, after.Status)
			require.Equal(t, epoch.Commitment, after.Commitment)
			require.Equal(t, epoch.CommitmentProof, after.CommitmentProof)
		})
	}
}

func TestPRTForeclosureStatementSnapshotAdmission(t *testing.T) {
	for _, stop := range []string{
		string(model.ApplicationStatus_Failed), string(model.ApplicationStatus_Diverged), prtDisabledAdmissionCase,
	} {
		t.Run(stop, func(t *testing.T) {
			r, app, epoch := prtForeclosureFixture(t, model.EpochStatus_ClaimComputed)
			require.NoError(t, r.CreateTournament(t.Context(), app.Name, prtForeclosureRoot(app, epoch)))
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			lock, err := r.db.Begin(ctx)
			require.NoError(t, err)
			defer rollbackDeliveryTx(lock)
			var marker int
			require.NoError(t, lock.QueryRow(ctx, `SELECT 1 FROM epoch WHERE application_id=$1 AND index=$2 FOR UPDATE`,
				app.ID, epoch.Index).Scan(&marker))
			done := make(chan error, 1)
			go func() { done <- r.ForeclosePRTEpoch(ctx, app.ID, epoch.Index, prtForeclosureHead) }()
			waitDeploymentEpochBlocker(t, r, lock.Conn().PgConn().PID())
			if stop == prtDisabledAdmissionCase {
				err = r.UpdateApplicationEnabled(ctx, app.ID, false)
			} else {
				err = r.UpdateApplicationStatus(ctx, app.ID, model.ApplicationStatus(stop), new("after SQL admission"))
			}
			require.NoError(t, err, "the waiting terminal write must not lock the application")
			require.NoError(t, lock.Rollback(ctx))
			require.NoError(t, <-done, "an admitted safe computed write can finish after a concurrent stop")
			after, err := r.GetEpoch(ctx, app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_ClaimForeclosed, after.Status)
			require.Equal(t, epoch.Commitment, after.Commitment)
			require.Equal(t, epoch.CommitmentProof, after.CommitmentProof)
		})
	}
}

func TestPRTForeclosureOpenBoundaryRequiresNoPendingExecutableInput(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "pending input"}[pending], func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
			epoch := repotest.NewEpochBuilder(app.ID).WithBlocks(10, 30).Build()
			var inputs []*model.Input
			if pending {
				inputs = []*model.Input{repotest.NewInputBuilder().WithIndex(0).WithBlockNumber(prtForeclosureHead).Build()}
			}
			require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
				map[*model.Epoch][]*model.Input{epoch: inputs}, prtForeclosureHead))
			require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID,
				prtForeclosureHead, repotest.UniqueHash(), prtForeclosureHead))
			_, err := r.db.Exec(t.Context(), `UPDATE application SET last_epoch_check_block=$2,last_input_check_block=$2 WHERE id=$1`,
				app.ID, prtForeclosureHead)
			require.NoError(t, err)
			err = r.ForeclosePRTEpoch(t.Context(), app.ID, epoch.Index, prtForeclosureHead)
			if pending {
				require.ErrorIs(t, err, repository.ErrNoUpdate)
			} else {
				require.NoError(t, err)
			}
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Nil(t, after.Commitment)
			require.Nil(t, after.TournamentAddress)
		})
	}
}

func prtInputStopFixture(t *testing.T, status model.InputCompletionStatus) (
	*PostgresRepository, *model.Application, *model.Epoch, *model.AdvanceResult,
) {
	t.Helper()
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
	epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).WithInputBounds(0, 2).Build()
	epoch.TournamentAddress = new(repotest.UniqueAddress())
	first := repotest.NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
	second := repotest.NewInputBuilder().WithIndex(1).WithBlockNumber(6).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
		map[*model.Epoch][]*model.Input{epoch: {first, second}}, prtForeclosureHead))
	result := &model.AdvanceResult{EpochIndex: epoch.Index, InputIndex: 0, Status: status,
		StateProof: *repotest.DummyStateProof()}
	if status == model.InputCompletionStatus_Exception {
		result.ExceptionData = []byte("terminal witness")
	}
	require.NoError(t, r.StoreAdvanceResult(t.Context(), app.ID, result))
	state, err := r.GetEpochCompletionState(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.NotNil(t, state.Problem)
	require.EqualValues(t, 0, state.Problem.EpochIndex)
	require.NotNil(t, state.Problem.InputIndex)
	require.EqualValues(t, 0, *state.Problem.InputIndex)
	require.Equal(t, state.Application.Status, state.Problem.Kind)
	require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID,
		prtForeclosureHead, repotest.UniqueHash(), prtForeclosureHead))
	_, err = r.db.Exec(t.Context(), `UPDATE application SET last_epoch_check_block=$2,last_input_check_block=$2 WHERE id=$1`,
		app.ID, prtForeclosureHead)
	require.NoError(t, err)
	epoch, err = r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	return r, app, epoch, result
}

func foreclosePRTTestEpoch(t *testing.T, r *PostgresRepository, app *model.Application, index uint64, generation bool) error {
	t.Helper()
	if !generation {
		return r.ForeclosePRTEpoch(t.Context(), app.ID, index, prtForeclosureHead)
	}
	key := deploymentKey(app)
	readDeployment(t, r, key)
	deploymentRestrict(t, r, key, 0, model.DeploymentTierGeneration, "confirmed generation mismatch")
	return r.ForecloseDeploymentEpoch(t.Context(), repository.DeploymentDrainRequest{Identity: key, EpochIndex: index})
}

func TestPRTForeclosureInputStopPreservesBoundaryWithoutInventedValidation(t *testing.T) {
	for _, tc := range []struct {
		status model.InputCompletionStatus
		kind   model.ApplicationStatus
	}{
		{model.InputCompletionStatus_Exception, model.ApplicationStatus_GuestException},
		{model.InputCompletionStatus_MachineHalted, model.ApplicationStatus_MachineHalted},
		{model.InputCompletionStatus_Overflow, model.ApplicationStatus_McycleOverflow},
		{model.InputCompletionStatus_UnexpectedYield, model.ApplicationStatus_UnexpectedYield},
		{model.InputCompletionStatus_InvalidOutputsRoot, model.ApplicationStatus_InvalidOutputsRoot},
	} {
		for _, generation := range []bool{false, true} {
			for _, suffix := range []bool{false, true} {
				name := tc.status.String() + "/generation=" + strconv.FormatBool(generation) + "/suffix=" + strconv.FormatBool(suffix)
				t.Run(name, func(t *testing.T) {
					r, app, epoch, result := prtInputStopFixture(t, tc.status)
					state, err := r.GetEpochCompletionState(t.Context(), app.Name, epoch.Index)
					require.NoError(t, err)
					require.Equal(t, tc.kind, state.Application.Status)
					require.Equal(t, tc.kind, state.Problem.Kind)
					if suffix {
						epoch = repotest.NewEpochBuilder(app.ID).WithIndex(1).WithStatus(model.EpochStatus_Closed).
							WithBlocks(10, 19).WithInputBounds(2, 2).Build()
						epoch.TournamentAddress = new(repotest.UniqueAddress())
						require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
							map[*model.Epoch][]*model.Input{epoch: {}}, prtForeclosureHead))
					}
					before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
					require.NoError(t, err)
					inputBefore, err := r.GetInput(t.Context(), app.Name, result.InputIndex)
					require.NoError(t, err)
					boundaryBefore, err := r.GetEpoch(t.Context(), app.Name, result.EpochIndex)
					require.NoError(t, err)
					require.Equal(t, result.MachineHash, *boundaryBefore.MachineHash)
					require.NoError(t, foreclosePRTTestEpoch(t, r, app, epoch.Index, generation))
					after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
					require.NoError(t, err)
					expected := *before
					expected.Status, expected.UpdatedAt = model.EpochStatus_ClaimForeclosed, after.UpdatedAt
					require.Equal(t, expected, *after, "closure must preserve the entire state proof")
					require.Nil(t, after.Commitment)
					var validated bool
					require.NoError(t, r.db.QueryRow(t.Context(),
						`SELECT claim_validated FROM epoch WHERE application_id=$1 AND index=$2`,
						app.ID, epoch.Index).Scan(&validated))
					require.False(t, validated)
					inputAfter, err := r.GetInput(t.Context(), app.Name, result.InputIndex)
					require.NoError(t, err)
					require.Equal(t, inputBefore, inputAfter)
					require.Equal(t, tc.status, inputAfter.Status)
					require.Equal(t, result.ExceptionData, inputAfter.ExceptionData)
					if suffix {
						boundaryAfter, err := r.GetEpoch(t.Context(), app.Name, result.EpochIndex)
						require.NoError(t, err)
						require.Equal(t, boundaryBefore, boundaryAfter)
					}
				})
			}
		}
	}
}

func TestPRTForeclosureRefusesHealthyPrefixBelowInputStop(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_Closed, model.EpochStatus_InputsProcessed} {
		for _, generation := range []bool{false, true} {
			t.Run(status.String()+"/generation="+strconv.FormatBool(generation), func(t *testing.T) {
				r, app, epoch := prtForeclosureFixture(t, status)
				later := repotest.NewEpochBuilder(app.ID).WithIndex(1).WithStatus(model.EpochStatus_Closed).
					WithBlocks(10, 19).WithInputBounds(1, 2).Build()
				later.TournamentAddress = new(repotest.UniqueAddress())
				input := repotest.NewInputBuilder().WithIndex(1).WithBlockNumber(15).Build()
				require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
					map[*model.Epoch][]*model.Input{later: {input}}, prtForeclosureHead))
				require.NoError(t, r.StoreAdvanceResult(t.Context(), app.ID, &model.AdvanceResult{
					EpochIndex: later.Index, InputIndex: input.Index, Status: model.InputCompletionStatus_MachineHalted,
					StateProof: *repotest.DummyStateProof(),
				}))
				// CLOSED below a later executed stop is a defensive refusal vector;
				// normal Advancer ordering finalizes that prefix before the stop.
				require.ErrorIs(t, foreclosePRTTestEpoch(t, r, app, epoch.Index, generation), repository.ErrNoUpdate)
				after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
				require.NoError(t, err)
				require.Equal(t, epoch, after)
			})
		}
	}
}

func TestPRTForeclosureInputStopRefusesPendingInputBelowCursor(t *testing.T) {
	for _, generation := range []bool{false, true} {
		t.Run("generation="+strconv.FormatBool(generation), func(t *testing.T) {
			r, app, epoch, _ := prtInputStopFixture(t, model.InputCompletionStatus_MachineHalted)
			// This defensive vector leaves NONE input 1 below processed_inputs.
			_, err := r.db.Exec(t.Context(), `UPDATE application SET processed_inputs=2 WHERE id=$1`, app.ID)
			require.NoError(t, err)
			require.ErrorIs(t, foreclosePRTTestEpoch(t, r, app, epoch.Index, generation), repository.ErrNoUpdate)
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, epoch, after)
		})
	}
}

func TestPRTForeclosureRetainsComputedInvalidOutputsRoot(t *testing.T) {
	for _, standing := range []model.TournamentStandingState{model.TournamentStandingRootWinner, model.TournamentStandingRootFailed} {
		t.Run(standing.String(), func(t *testing.T) {
			r, app, epoch := prtForeclosureFixture(t, model.EpochStatus_InputsProcessed)
			epoch.CalculatedOutputsRoot = new(common.HexToHash("0xcafe"))
			epoch.Commitment = new(repotest.UniqueHash())
			epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
			require.True(t, epoch.HasInvalidOutputsRoot())
			output, err := r.GetOutput(t.Context(), app.Name, 0)
			require.NoError(t, err)
			output.Hash = new(repotest.UniqueHash())
			output.OutputHashesSiblings = []common.Hash{repotest.UniqueHash()}
			require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, repotest.WriteProofs(output)))

			before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_ClaimComputed, before.Status)
			require.True(t, before.HasInvalidOutputsRoot())
			require.Equal(t, epoch.Commitment, before.Commitment)
			require.Equal(t, epoch.CommitmentProof, before.CommitmentProof)
			state, err := r.GetEpochCompletionState(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, model.ApplicationStatus_OK, state.Application.Status)
			require.Nil(t, state.Problem, "a final root mismatch does not create an input stop")
			root := prtForeclosureRoot(app, before)
			if standing == model.TournamentStandingRootFailed {
				root.Snapshot.Standing = standing
				root.Snapshot.Candidate, root.Snapshot.WinnerCommitment, root.Snapshot.FinalStateHash = nil, nil, nil
				root.Snapshot.BondRecovery = model.TournamentBondRecovery{Disposition: model.BondDispositionNoWinner}
			}
			require.NoError(t, r.CreateTournament(t.Context(), app.Name, root))
			outputBefore, err := r.GetOutput(t.Context(), app.Name, 0)
			require.NoError(t, err)

			require.NoError(t, r.ForeclosePRTEpoch(t.Context(), app.ID, epoch.Index, prtForeclosureHead))

			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			expected := *before
			expected.Status, expected.UpdatedAt = model.EpochStatus_ClaimForeclosed, after.UpdatedAt
			require.Equal(t, expected, *after, "closure retains the execution commitment and both outputs roots")
			outputAfter, err := r.GetOutput(t.Context(), app.Name, 0)
			require.NoError(t, err)
			require.Equal(t, outputBefore, outputAfter, "closure retains the local output proof")
		})
	}
}

func TestPRTForeclosureStagedKnownTransaction(t *testing.T) {
	for _, generation := range []bool{false, true} {
		t.Run("generation="+strconv.FormatBool(generation), func(t *testing.T) {
			r, app, epoch := prtForeclosureFixture(t, model.EpochStatus_ClaimComputed)
			require.NoError(t, r.UpdateEpochReconciledStaged(t.Context(), app.ID, epoch.Index, 15))
			hash := repotest.UniqueHash()
			epoch.ClaimTransactionHash = &hash
			require.NoError(t, r.UpdateEpochClaimTransactionHash(t.Context(), app.Name, epoch))
			before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			err = foreclosePRTTestEpoch(t, r, app, epoch.Index, generation)
			if generation {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, repository.ErrNoUpdate)
			}
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			expected := *before
			if generation {
				expected.Status, expected.UpdatedAt = model.EpochStatus_ClaimForeclosed, after.UpdatedAt
			}
			require.Equal(t, expected, *after, "the complete staged result and known transaction must remain")
		})
	}
}

func TestPRTForeclosureRootRetiredBeforeLocalValidation(t *testing.T) {
	r, app, epoch := prtForeclosureFixture(t, model.EpochStatus_Closed)
	proof, err := epoch.StateProof()
	require.NoError(t, err)
	root := prtForeclosureRoot(app, epoch)
	root.Snapshot.FinalStateHash = &proof.MachineHash
	root.Snapshot.BondRecovery = model.TournamentBondRecovery{Disposition: model.BondDispositionRecoverable,
		Claimer: new(repotest.UniqueAddress()), Payment: new(model.Uint256)}
	require.NoError(t, r.CreateTournament(t.Context(), app.Name, root))
	require.ErrorIs(t, r.ForeclosePRTEpoch(t.Context(), app.ID, epoch.Index, 25), repository.ErrNoUpdate)
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, &proof))
	completeDeploymentDrainProofs(t, r, app, epoch)
	before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.NoError(t, r.ForeclosePRTEpoch(t.Context(), app.ID, epoch.Index, 25))
	after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	expected := *before
	expected.Status, expected.UpdatedAt = model.EpochStatus_ClaimForeclosed, after.UpdatedAt
	require.Equal(t, expected, *after)
	storedRoot, err := r.GetTournament(t.Context(), app.Name, root.Address.Hex())
	require.NoError(t, err)
	require.EqualValues(t, prtForeclosureHead, storedRoot.Snapshot.AsOfBlock, "validation must not refresh the retired root")
}

func TestPRTForeclosureInputStopClosesOpenSuffix(t *testing.T) {
	for _, generation := range []bool{false, true} {
		t.Run("generation="+strconv.FormatBool(generation), func(t *testing.T) {
			r, app, _, _ := prtInputStopFixture(t, model.InputCompletionStatus_InvalidOutputsRoot)
			suffix := repotest.NewEpochBuilder(app.ID).WithIndex(1).WithBlocks(10, 30).Build()
			pending := repotest.NewInputBuilder().WithIndex(2).WithBlockNumber(15).Build()
			require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
				map[*model.Epoch][]*model.Input{suffix: {pending}}, prtForeclosureHead))
			before, err := r.GetEpoch(t.Context(), app.Name, suffix.Index)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_Open, before.Status)
			require.NoError(t, foreclosePRTTestEpoch(t, r, app, suffix.Index, generation))
			after, err := r.GetEpoch(t.Context(), app.Name, suffix.Index)
			require.NoError(t, err)
			expected := *before
			expected.Status, expected.UpdatedAt = model.EpochStatus_ClaimForeclosed, after.UpdatedAt
			require.Equal(t, expected, *after)
			input, err := r.GetInput(t.Context(), app.Name, pending.Index)
			require.NoError(t, err)
			require.Equal(t, model.InputCompletionStatus_None, input.Status, "the input above the stop is not executable")
		})
	}
}

func TestPRTForeclosureInnerResultDoesNotCoverRoot(t *testing.T) {
	r, app, epoch := prtForeclosureFixture(t, model.EpochStatus_ClaimComputed)
	parent := repotest.NewTournamentBuilder(app.ID).WithEpochIndex(epoch.Index).Build()
	require.NoError(t, r.CreateTournament(t.Context(), app.Name, parent))
	first := repotest.NewCommitmentBuilder(app.ID).WithEpochIndex(epoch.Index).WithTournamentAddress(parent.Address).Build()
	second := repotest.NewCommitmentBuilder(app.ID).WithEpochIndex(epoch.Index).WithTournamentAddress(parent.Address).Build()
	require.NoError(t, r.CreateCommitment(t.Context(), app.Name, first))
	require.NoError(t, r.CreateCommitment(t.Context(), app.Name, second))
	matchID := repotest.UniqueHash()
	match := repotest.NewMatchBuilder(app.ID).WithEpochIndex(epoch.Index).WithTournamentAddress(parent.Address).
		WithIDHash(matchID).WithCommitmentOne(first.Commitment).WithCommitmentTwo(second.Commitment).Build()
	require.NoError(t, r.CreateMatch(t.Context(), app.Name, match))
	inner := repotest.NewTournamentBuilder(app.ID).WithEpochIndex(epoch.Index).
		WithAddress(*epoch.TournamentAddress).WithLevel(1).WithParent(parent.Address, matchID).Build()
	inner.StartInstant, inner.CreationEvent.BlockNumber = 15, 15
	inner.Snapshot.AsOfBlock, inner.Snapshot.FinishedAtBlock = 20, 19
	inner.Snapshot.Standing = model.TournamentStandingInnerEliminableNoWinner
	inner.Snapshot.BondRecovery.Disposition = model.BondDispositionNoWinner
	require.NoError(t, r.CreateTournament(t.Context(), app.Name, inner))
	require.ErrorIs(t, r.ForeclosePRTEpoch(t.Context(), app.ID, epoch.Index, 25), repository.ErrNoUpdate)
	after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, epoch, after)
}

func TestPRTForeclosureGenerationRetainsSealedProducers(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_Closed, model.EpochStatus_InputsProcessed} {
		t.Run(status.String(), func(t *testing.T) {
			r, app, epoch := prtForeclosureFixture(t, status)
			_, err := r.db.Exec(t.Context(), `UPDATE epoch SET last_block=$3 WHERE application_id=$1 AND index=$2`,
				app.ID, epoch.Index, prtForeclosureHead)
			require.NoError(t, err)
			key := deploymentKey(app)
			readDeployment(t, r, key)
			deploymentRestrict(t, r, key, 0, model.DeploymentTierGeneration, "confirmed mismatch")
			require.ErrorIs(t, r.ForecloseDeploymentEpoch(t.Context(), repository.DeploymentDrainRequest{
				Identity: key, EpochIndex: epoch.Index,
			}), repository.ErrNoUpdate)
		})
	}
}

func TestDeploymentDrainRejectsFreshStopAndIncompleteHistory(t *testing.T) {
	for _, name := range []string{
		string(model.ApplicationStatus_Failed), string(model.ApplicationStatus_Diverged), string(model.ApplicationStatus_Corrupted),
		prtDisabledAdmissionCase, "input scan", "epoch scan",
	} {
		t.Run(name, func(t *testing.T) {
			r, app, epoch, request := deploymentDrainFixture(t, true)
			var err error
			switch name {
			case prtDisabledAdmissionCase:
				err = r.UpdateApplicationEnabled(t.Context(), app.ID, false)
			case "input scan":
				_, err = r.db.Exec(t.Context(), `UPDATE application SET last_input_check_block=19 WHERE id=$1`, app.ID)
			case "epoch scan":
				_, err = r.db.Exec(t.Context(), `UPDATE application SET last_epoch_check_block=19 WHERE id=$1`, app.ID)
			default:
				err = r.UpdateApplicationStatus(t.Context(), app.ID, model.ApplicationStatus(name), new("before SQL admission"))
			}
			require.NoError(t, err)
			require.ErrorIs(t, r.ForecloseDeploymentEpoch(t.Context(), request), repository.ErrNoUpdate)
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, epoch, after)
		})
	}
}

func TestPRTForeclosurePriorClaimObservationDoesNotWaitForRoot(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_ClaimSubmitted, model.EpochStatus_ClaimStaged} {
		t.Run(status.String(), func(t *testing.T) {
			r, app, epoch := prtForeclosureFixture(t, model.EpochStatus_ClaimComputed)
			if status == model.EpochStatus_ClaimStaged {
				require.NoError(t, r.UpdateEpochReconciledStaged(t.Context(), app.ID, epoch.Index, 15))
			} else {
				// PRT does not normally use SUBMITTED. Retain the existing legacy
				// observation branch, with no transaction pending for reconciliation.
				_, err := r.db.Exec(t.Context(), `UPDATE epoch SET status=$3 WHERE application_id=$1 AND index=$2`,
					app.ID, epoch.Index, status.String())
				require.NoError(t, err)
			}
			require.NoError(t, r.ForeclosePRTEpoch(t.Context(), app.ID, epoch.Index, prtForeclosureHead))
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_ClaimForeclosed, after.Status)
			require.Equal(t, epoch.Commitment, after.Commitment)
		})
	}
}

func TestPRTMachinePublicationGateWaitsForEveryClosedDuty(t *testing.T) {
	r, app, first := prtForeclosureFixture(t, model.EpochStatus_Closed)
	second := repotest.NewEpochBuilder(app.ID).WithIndex(1).WithStatus(model.EpochStatus_Closed).
		WithBlocks(9, 9).WithInputBounds(1, 1).Build()
	second.TournamentAddress = new(repotest.UniqueAddress())
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
		map[*model.Epoch][]*model.Input{second: {}}, prtForeclosureHead))
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, first.Index, repotest.DummyStateProof()))
	retained, err := r.HasUnpublishedEpochBeforeBlock(t.Context(), app.ID, prtForeclosureHead)
	require.NoError(t, err)
	require.True(t, retained, "a finalized target must not hide a sibling's finalization duty")
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, second.Index, repotest.DummyStateProof()))
	retained, err = r.HasUnpublishedEpochBeforeBlock(t.Context(), app.ID, prtForeclosureHead)
	require.NoError(t, err)
	require.False(t, retained, "validation and root comparison alone do not need a machine")
}

func TestPRTForeclosureIntegrityStopRetainsInputBoundary(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted} {
		for _, generation := range []bool{false, true} {
			name := status.String() + "/normal"
			if generation {
				name = status.String() + "/generation"
			}
			t.Run(name, func(t *testing.T) {
				r := newEpochPublicationRepository(t).(*PostgresRepository)
				app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
				epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).
					WithBlocks(0, 9).WithInputBounds(0, 1).Build()
				epoch.TournamentAddress = new(repotest.UniqueAddress())
				input := repotest.NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
				require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
					map[*model.Epoch][]*model.Input{epoch: {input}}, prtForeclosureHead))
				require.NoError(t, r.StoreAdvanceResult(t.Context(), app.ID, &model.AdvanceResult{
					EpochIndex: epoch.Index, InputIndex: input.Index, Status: model.InputCompletionStatus_MachineHalted,
					StateProof: *repotest.DummyStateProof(),
				}))
				require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID,
					prtForeclosureHead, repotest.UniqueHash(), prtForeclosureHead))
				_, err := r.db.Exec(t.Context(), `UPDATE application SET last_epoch_check_block=$2,last_input_check_block=$2 WHERE id=$1`,
					app.ID, prtForeclosureHead)
				require.NoError(t, err)
				require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, status, new("independent integrity evidence")))
				before, err := r.GetEpochCompletionState(t.Context(), app.Name, epoch.Index)
				require.NoError(t, err)
				wantProblem := &model.ApplicationProblem{EpochIndex: epoch.Index, Kind: model.ApplicationStatus_MachineHalted,
					InputIndex: new(input.Index)}
				require.Equal(t, status, before.Application.Status)
				require.Equal(t, wantProblem, before.Problem)

				err = foreclosePRTTestEpoch(t, r, app, epoch.Index, generation)

				require.ErrorIs(t, err, repository.ErrNoUpdate)
				after, err := r.GetEpochCompletionState(t.Context(), app.Name, epoch.Index)
				require.NoError(t, err)
				require.Equal(t, before.Epoch, after.Epoch, "the integrity finding must preserve the full epoch row")
				require.Equal(t, status, after.Application.Status)
				require.Equal(t, wantProblem, after.Problem, "the retained input boundary is evidence, not foreclosure permission")
			})
		}
	}
}
