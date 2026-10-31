// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Reproduce the ordering that used to lose E: execution has already stopped in
// P before the validator first discovers the complete earlier epoch E.
func TestValidatorCompletesEpochBeforeTerminalBoundary(t *testing.T) {
	for _, consensus := range model.ConsensusAllValues {
		for _, status := range []model.ApplicationStatus{
			model.ApplicationStatus_GuestException, model.ApplicationStatus_MachineHalted,
			model.ApplicationStatus_McycleOverflow, model.ApplicationStatus_UnexpectedYield,
			model.ApplicationStatus_InvalidOutputsRoot,
		} {
			t.Run(consensus.String()+"/"+status.String(), func(t *testing.T) {
				repo, validator, app, epoch := rootValidationFixture()
				app.Status = status
				app.ConsensusType = consensus
				// A nonempty Authority/Quorum epoch; the PRT first epoch can
				// be empty and uses its template as the pre-computation state.
				app.TemplateHash = *epoch.MachineHash
				repo.problem = &model.ApplicationProblem{EpochIndex: 1, Kind: status}
				if status.IsExecutionTerminal() {
					repo.problem.InputIndex = new(uint64(1))
				}
				epochs := []*model.Epoch{epoch,
					{ApplicationID: app.ID, Index: 1, VirtualIndex: 1, Status: model.EpochStatus_InputsProcessed},
					{ApplicationID: app.ID, Index: 2, VirtualIndex: 2, Status: model.EpochStatus_InputsProcessed},
				}
				repo.On("ListApplications", mock.Anything, validationApplicationsFilter(), repository.Pagination{}, false).
					Return([]*model.Application{app}, uint64(1), nil).Once()
				repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.String(), mock.Anything, repository.Pagination{}, false).
					Return(epochs, uint64(len(epochs)), nil).Once()
				expectValidPrefixEpoch(repo, app, epoch, consensus == model.Consensus_PRT)
				repo.On("StoreClaimAndProofs", mock.Anything, epoch, mock.Anything).Return(nil).Once()

				_, err := validator.Tick(t.Context())

				require.NoError(t, err)
				require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
				require.Equal(t, model.EpochStatus_InputsProcessed, epochs[1].Status)
				require.Equal(t, model.EpochStatus_InputsProcessed, epochs[2].Status)
				require.Equal(t, status, app.Status)
				repo.AssertNumberOfCalls(t, "StoreClaimAndProofs", 1)
				repo.AssertExpectations(t)
			})
		}
	}
}

func expectValidPrefixEpoch(repo *Mockrepo, app *model.Application, epoch *model.Epoch, empty bool) {
	address := app.IApplicationAddress.String()
	expectEpochOutputs(repo, app, epoch, []*model.Output{})
	var input *model.Input
	if !empty {
		input = &model.Input{MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock}
	}
	repo.On("GetLastInput", mock.Anything, address, epoch.Index).Return(input, nil).Once()
}

func TestValidatorReloadsPrefixBeforeComputation(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       model.ApplicationStatus
		problem      *model.ApplicationProblem
		disabled     bool
		missingEpoch bool
		readErr      error
		wantErr      string
		wantClaim    bool
	}{
		{name: "runtime stopped after discovery", status: model.ApplicationStatus_MachineHalted, wantClaim: true,
			problem: &model.ApplicationProblem{EpochIndex: 2, Kind: model.ApplicationStatus_MachineHalted, InputIndex: new(uint64(2))}},
		{name: "root finding after discovery", status: model.ApplicationStatus_InvalidOutputsRoot, wantClaim: true,
			problem: &model.ApplicationProblem{EpochIndex: 2, Kind: model.ApplicationStatus_InvalidOutputsRoot}},
		{name: "failed after discovery", status: model.ApplicationStatus_Failed},
		{name: "diverged after discovery", status: model.ApplicationStatus_Diverged},
		{name: "corrupted after discovery", status: model.ApplicationStatus_Corrupted},
		{name: "disabled after discovery", status: model.ApplicationStatus_OK, disabled: true},
		{name: "cut tightened to this epoch", status: model.ApplicationStatus_MachineHalted,
			problem: &model.ApplicationProblem{EpochIndex: 0, Kind: model.ApplicationStatus_InvalidOutputsRoot}},
		{name: "terminal without source", status: model.ApplicationStatus_MachineHalted, wantErr: "without an epoch stop boundary"},
		{name: "epoch missing", status: model.ApplicationStatus_OK, missingEpoch: true, wantErr: "source is unavailable"},
		{name: "read failed", readErr: errors.New("snapshot unavailable"), wantErr: "snapshot unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, validator, app, epoch := rootValidationFixture()
			current := *app
			current.Status = test.status
			current.Enabled = !test.disabled
			state := &repository.EpochCompletionState{Application: &current, Epoch: epoch, Problem: test.problem}
			if test.missingEpoch {
				state.Epoch = nil
			}
			repo.readEpochCompletionState = func(context.Context, string, uint64) (*repository.EpochCompletionState, error) {
				return state, test.readErr
			}
			repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.String(), mock.Anything, repository.Pagination{}, false).
				Return([]*model.Epoch{epoch}, uint64(1), nil).Once()

			if test.wantClaim {
				expectValidPrefixEpoch(repo, &current, epoch, false)
				repo.On("StoreClaimAndProofs", mock.Anything, epoch, mock.Anything).Return(nil).Once()
			}
			err := validator.validateApplication(t.Context(), app)

			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, model.ApplicationStatus_OK, app.Status, "discovery is not the current source of truth")
			if test.wantClaim {
				require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
				repo.AssertNumberOfCalls(t, "StoreClaimAndProofs", 1)
			} else {
				repo.AssertNotCalled(t, "GetEpochOutputRange", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "ScanOutputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "StoreClaimAndProofs", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "StoreClaimAndProofs", mock.Anything, mock.Anything, mock.Anything)
			}
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}

func TestValidatorEarlierRootFindingTightensRuntimeStop(t *testing.T) {
	repo, validator, app, epoch := rootValidationFixture()
	app.Status = model.ApplicationStatus_MachineHalted
	repo.problem = &model.ApplicationProblem{EpochIndex: 2, Kind: app.Status, InputIndex: new(uint64(2))}
	// The epoch and last input agree with each other, but their declared word
	// fails the independently computed outputs root rule.
	epoch.TxBufferDataBlock = &app.TemplateHash
	input := &model.Input{MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock}
	expectRootValidationReads(repo, app, epoch, input, nil)
	repo.On("StoreInvalidOutputsRoot", mock.Anything, app.ID, epoch.Index, mock.Anything).
		Return(nil).Once()

	err := validator.validateApplication(t.Context(), app)

	require.ErrorContains(t, err, "declared outputs root does not match")
	require.Equal(t, model.ApplicationStatus_MachineHalted, app.Status)
	repo.AssertNotCalled(t, "StoreClaimAndProofs", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

// Two applications sharing epoch indices must still get separate completion
// views. A single-application fake can hide a lookup that ignores its address.
func TestValidatorCompletesSeparateApplicationPrefixes(t *testing.T) {
	repo, svc, first, firstEpoch := rootValidationFixture()
	first.Status = model.ApplicationStatus_MachineHalted
	second := *first
	second.ID++
	second.Name = "other-prefix"
	second.IApplicationAddress = common.HexToAddress("0x2")
	secondEpoch := *firstEpoch
	secondEpoch.ApplicationID = second.ID
	repo.problem = &model.ApplicationProblem{EpochIndex: 1, Kind: first.Status, InputIndex: new(uint64(1))}
	repo.On("ListApplications", mock.Anything,
		mock.MatchedBy(func(filter repository.ApplicationFilter) bool {
			if filter.Enabled == nil || !*filter.Enabled {
				return false
			}
			for _, status := range filter.Statuses {
				if status == model.ApplicationStatus_MachineHalted {
					return true
				}
			}
			return false
		}), repository.Pagination{}, false).
		Return([]*model.Application{first, &second}, uint64(2), nil).Once()
	for _, pair := range []struct {
		app   *model.Application
		epoch *model.Epoch
	}{{first, firstEpoch}, {&second, &secondEpoch}} {
		repo.On("ListEpochs", mock.Anything, pair.app.IApplicationAddress.Hex(), mock.Anything, repository.Pagination{}, false).
			Return([]*model.Epoch{pair.epoch}, uint64(1), nil).Once()
		expectValidPrefixEpoch(repo, pair.app, pair.epoch, false)
		repo.On("StoreClaimAndProofs", mock.Anything, pair.epoch, mock.Anything).Return(nil).Once()
	}
	_, err := svc.Tick(t.Context())
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimComputed, firstEpoch.Status)
	require.Equal(t, model.EpochStatus_ClaimComputed, secondEpoch.Status)
	repo.AssertNumberOfCalls(t, "StoreClaimAndProofs", 2)
	repo.AssertExpectations(t)
}
