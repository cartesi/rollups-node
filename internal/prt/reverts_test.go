// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	executionRevertedError = "execution reverted"
	applicationForeclosed  = "ApplicationForeclosed"
)

// daveConsensusRevertError creates a typed IDaveConsensus revert carrying only
// the 4-byte selector — sufficient for the classifiers to match by name.
func daveConsensusRevertError(name string) error {
	parsed, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	abiErr, ok := parsed.Errors[name]
	if !ok {
		panic(fmt.Sprintf("unknown IDaveConsensus error: %s", name))
	}
	return &rpcDataError{code: 3, msg: executionRevertedError, data: fmt.Sprintf("0x%x", abiErr.ID[:4])}
}

// daveRevertWithArgs creates a typed IDaveConsensus revert carrying the given
// arguments, ABI-encoded as the contract would emit it.
func daveRevertWithArgs(name string, args ...any) error {
	parsed, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	abiErr, ok := parsed.Errors[name]
	if !ok {
		panic(fmt.Sprintf("unknown IDaveConsensus error: %s", name))
	}
	packed, err := abiErr.Inputs.Pack(args...)
	if err != nil {
		panic(err)
	}
	payload := append(append([]byte{}, abiErr.ID[:4]...), packed...)
	return &rpcDataError{code: 3, msg: executionRevertedError, data: fmt.Sprintf("0x%x", payload)}
}

// tournamentRevertError creates a typed ITournament revert carrying only the
// 4-byte selector.
func tournamentRevertError(name string) error {
	parsed, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	abiErr, ok := parsed.Errors[name]
	if !ok {
		panic(fmt.Sprintf("unknown ITournament error: %s", name))
	}
	return &rpcDataError{code: 3, msg: executionRevertedError, data: fmt.Sprintf("0x%x", abiErr.ID[:4])}
}

func prtRevertTestApp() *model.Application {
	return &model.Application{
		ID:                  7,
		Name:                "prt-app",
		IApplicationAddress: common.BigToAddress(common.Big1),
		IConsensusAddress:   common.HexToAddress("0x100"),
		ConsensusType:       model.Consensus_PRT,
		Status:              model.ApplicationStatus_OK,
		Enabled:             true,
	}
}

func prtRevertTestEpoch() *model.Epoch {
	tournament := common.BigToAddress(common.Big2)
	commitment := common.HexToHash("0xabcd")
	machineHash := common.HexToHash("0x1234")
	return &model.Epoch{
		Index:             3,
		TournamentAddress: &tournament,
		Commitment:        &commitment,
		MachineHash:       &machineHash,
	}
}

// reasonContains builds a mock.MatchedBy predicate over a status reason.
func reasonContains(substrings ...string) func(*string) bool {
	return func(reason *string) bool {
		if reason == nil {
			return false
		}
		for _, want := range substrings {
			if !strings.Contains(*reason, want) {
				return false
			}
		}
		return true
	}
}

func TestHandleStageTournamentResultRevert(t *testing.T) {
	epoch := prtRevertTestEpoch()

	for _, name := range []string{
		"TournamentResultAlreadyStaged",
		"TournamentNotFinishedYet",
		applicationForeclosed,
	} {
		t.Run(name+"_waitsForSynchronization", func(t *testing.T) {
			s, r := newPRTServiceMock()
			defer r.AssertExpectations(t)
			err := s.handleStageTournamentResultRevert(
				context.Background(), prtRevertTestApp(), epoch, daveConsensusRevertError(name))
			assert.NoError(t, err)
		})
	}

	for _, name := range []string{
		"InvalidSiblingsArrayLength",
		"InvalidMachineMerkleProof",
		"InvalidPostEpochMachineIflagsYRegister",
		"InvalidPostEpochMachineHtifTohostRegister",
	} {
		t.Run(name+"_setsFailed", func(t *testing.T) {
			s, r := newPRTServiceMock()
			app := prtRevertTestApp()
			testEpoch := epoch
			if name == "InvalidMachineMerkleProof" {
				testEpoch = resultTestEpoch(model.EpochStatus_ClaimComputed)
				snapshot := resultTestSnapshot(testEpoch, false)
				client := &ethClientMock{}
				client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
					Return(&types.Header{Number: big.NewInt(20)}, nil).Once()
				consensus := &daveConsensusAdapterMock{}
				opts := mock.MatchedBy(resultCallOptsAtBlock(20))
				consensus.On("GetCurrentSealedEpoch", opts).Return(snapshot.sealed, nil).Once()
				consensus.On("CanStageTournamentResult", opts).Return(snapshot.stage, nil).Once()
				consensus.On("CanAcceptStagedTournamentResult", opts).Return(snapshot.accept, nil).Once()
				factory := &adapterFactoryMock{}
				factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
				s.client = client
				s.defaultBlock = model.DefaultBlock_Finalized
				s.adapterFactory = factory
				defer client.AssertExpectations(t)
				defer consensus.AssertExpectations(t)
				defer factory.AssertExpectations(t)
			}
			guidance := "Check proof serialization"
			if name == "InvalidPostEpochMachineIflagsYRegister" || name == "InvalidPostEpochMachineHtifTohostRegister" {
				guidance = "The proven post-epoch machine state cannot finalize"
			}
			r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
				mock.MatchedBy(reasonContains("StageTournamentResult", name, "epoch 3", guidance))).
				Return(nil).Once()
			assert.NoError(t, s.handleStageTournamentResultRevert(
				context.Background(), app, testEpoch, daveConsensusRevertError(name)))
			r.AssertExpectations(t)
		})
	}

	t.Run("TournamentFailedNoWinner_waitsForConfirmation", func(t *testing.T) {
		s, r := newPRTServiceMock()
		app := prtRevertTestApp()
		assert.NoError(t, s.handleStageTournamentResultRevert(
			context.Background(), app, epoch, tournamentRevertError("TournamentFailedNoWinner")))
		assert.Equal(t, model.ApplicationStatus_OK, app.Status)
		r.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		r.AssertExpectations(t)
	})

	t.Run("IncorrectEpochNumber_behind_waitsForSynchronization", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		assert.NoError(t, s.handleStageTournamentResultRevert(context.Background(), prtRevertTestApp(), epoch,
			daveRevertWithArgs("IncorrectEpochNumber", big.NewInt(3), big.NewInt(5))))
	})

	t.Run("IncorrectEpochNumber_ahead_retriesWithoutStatusChange", func(t *testing.T) {
		s, r := newPRTServiceMock()
		app := prtRevertTestApp()
		boom := daveRevertWithArgs("IncorrectEpochNumber", big.NewInt(7), big.NewInt(5))
		assert.ErrorIs(t, s.handleStageTournamentResultRevert(context.Background(), app, epoch, boom), boom)
		assert.Equal(t, model.ApplicationStatus_OK, app.Status)
		r.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		r.AssertExpectations(t)
	})

	for _, name := range []string{"ApplicationReverted", "IllformedApplicationReturnData"} {
		t.Run(name+"_setsFailedWithReturnData", func(t *testing.T) {
			s, r := newPRTServiceMock()
			app := prtRevertTestApp()
			r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
				mock.MatchedBy(reasonContains("StageTournamentResult", name, "0xdead"))).
				Return(nil).Once()
			assert.NoError(t, s.handleStageTournamentResultRevert(context.Background(), app, epoch,
				daveRevertWithArgs(name, common.BigToAddress(common.Big1), []byte{0xde, 0xad})))
			assert.Equal(t, model.ApplicationStatus_Failed, app.Status)
			r.AssertExpectations(t)
		})
	}

	t.Run("NonceTooLow_waitsForSynchronization", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		assert.NoError(t, s.handleStageTournamentResultRevert(
			context.Background(), prtRevertTestApp(), epoch, errors.New("nonce too low")))
	})

	t.Run("unknown_propagates", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		boom := errors.New("boom")
		assert.ErrorIs(t, s.handleStageTournamentResultRevert(
			context.Background(), prtRevertTestApp(), epoch, boom), boom)
	})
}

func TestHandleAcceptTournamentResultRevert(t *testing.T) {
	epoch := prtRevertTestEpoch()
	for _, name := range []string{
		"TournamentResultNotStaged",
		"ClaimStagingPeriodNotOverYet",
		applicationForeclosed,
	} {
		t.Run(name+"_waitsForSynchronization", func(t *testing.T) {
			s, r := newPRTServiceMock()
			defer r.AssertExpectations(t)
			assert.NoError(t, s.handleAcceptTournamentResultRevert(
				context.Background(), prtRevertTestApp(), epoch, daveConsensusRevertError(name)))
		})
	}

	t.Run("ApplicationNotDeployed_setsFailed", func(t *testing.T) {
		s, r := newPRTServiceMock()
		app := prtRevertTestApp()
		r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
			mock.MatchedBy(reasonContains("AcceptStagedTournamentResult", "ApplicationNotDeployed"))).
			Return(nil).Once()
		assert.NoError(t, s.handleAcceptTournamentResultRevert(
			context.Background(), app, epoch, daveConsensusRevertError("ApplicationNotDeployed")))
		r.AssertExpectations(t)
	})

	t.Run("unknown_propagates", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		boom := errors.New("factory failed")
		assert.ErrorIs(t, s.handleAcceptTournamentResultRevert(
			context.Background(), prtRevertTestApp(), epoch, boom), boom)
	})
}

// TestHandleJoinTournamentRevert covers the JoinTournament revert
// classification: an already-joined commitment retries silently (whether
// detected via ClockAlreadyInitialized or via the CommitmentStanding re-check
// behind a window revert), a genuinely missed join window marks the app
// FAILED, bad local commitment proofs mark it CORRUPTED, and unknown errors
// propagate.
func TestHandleJoinTournamentRevert(t *testing.T) {
	t.Run("ClockAlreadyInitialized_waitsForEventSync", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		err := s.handleJoinTournamentRevert(context.Background(), prtRevertTestApp(), prtRevertTestEpoch(),
			&tournamentAdapterMock{}, tournamentRevertError("ClockAlreadyInitialized"))
		assert.NoError(t, err)
	})

	for _, revertName := range []string{"TournamentIsClosed", "TournamentIsFinished"} {
		t.Run(revertName+"_confirmedNotJoined_setsFailed", func(t *testing.T) {
			s, r := newPRTServiceMock()
			defer r.AssertExpectations(t)
			app := prtRevertTestApp()
			epoch := prtRevertTestEpoch()
			r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
				mock.MatchedBy(reasonContains(
					"JoinTournament reverted with "+revertName,
					"epoch 3",
					epoch.TournamentAddress.Hex(),
					"before re-enabling"))).
				Return(nil).Once()
			adapter := &tournamentAdapterMock{}
			adapter.On("CommitmentStanding", mock.MatchedBy(func(opts *bind.CallOpts) bool {
				return opts != nil && opts.BlockNumber == nil
			}), [32]byte(*epoch.Commitment)).
				Return(CommitmentStanding{}, nil).Once()
			opts := mock.MatchedBy(resultCallOptsAtBlock(20))
			adapter.On("Standing", opts).
				Return(TournamentStanding{State: model.TournamentStandingRootFailed, FinishedAt: 19}, nil).Once()
			adapter.On("CommitmentStanding", opts, [32]byte(*epoch.Commitment)).
				Return(CommitmentStanding{}, nil).Once()
			client := &ethClientMock{}
			client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
				Return(&types.Header{Number: big.NewInt(20)}, nil).Once()
			s.client = client
			s.defaultBlock = model.DefaultBlock_Finalized
			err := s.handleJoinTournamentRevert(context.Background(), app, epoch,
				adapter, tournamentRevertError(revertName))
			assert.NoError(t, err)
			assert.Equal(t, model.ApplicationStatus_Failed, app.Status)
			adapter.AssertExpectations(t)
			client.AssertExpectations(t)
		})

		t.Run(revertName+"_configuredWindowOpen_waitsForConfirmation", func(t *testing.T) {
			s, r := newPRTServiceMock()
			app := prtRevertTestApp()
			epoch := prtRevertTestEpoch()
			adapter := &tournamentAdapterMock{}
			adapter.On("CommitmentStanding", mock.MatchedBy(func(opts *bind.CallOpts) bool {
				return opts != nil && opts.BlockNumber == nil
			}), [32]byte(*epoch.Commitment)).Return(CommitmentStanding{}, nil).Once()
			adapter.On("Standing", mock.MatchedBy(resultCallOptsAtBlock(20))).
				Return(TournamentStanding{State: model.TournamentStandingAwaitingClosure, AcceptsJoins: true}, nil).Once()
			client := &ethClientMock{}
			client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
				Return(&types.Header{Number: big.NewInt(20)}, nil).Once()
			s.client = client
			s.defaultBlock = model.DefaultBlock_Finalized

			assert.NoError(t, s.handleJoinTournamentRevert(context.Background(), app, epoch,
				adapter, tournamentRevertError(revertName)))
			assert.Equal(t, model.ApplicationStatus_OK, app.Status)
			r.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			r.AssertExpectations(t)
			adapter.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}

	t.Run("TournamentIsClosed_alreadyJoined_waitsForEventSync", func(t *testing.T) {
		// The contract checks the join window before the already-joined clock
		// check, so a commitment that joined just before the window closed
		// reverts with TournamentIsClosed on a rebroadcast. The re-check must
		// prevent a false FAILED.
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		epoch := prtRevertTestEpoch()
		adapter := &tournamentAdapterMock{}
		adapter.On("CommitmentStanding", mock.Anything, [32]byte(*epoch.Commitment)).
			Return(CommitmentStanding{Joined: true, FinalState: *epoch.MachineHash}, nil).Once()
		err := s.handleJoinTournamentRevert(context.Background(), prtRevertTestApp(), epoch,
			adapter, tournamentRevertError("TournamentIsClosed"))
		assert.NoError(t, err, "an already-joined commitment must not mark the app FAILED")
		adapter.AssertExpectations(t)
	})

	t.Run("TournamentIsClosed_recheckFails_retries", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		epoch := prtRevertTestEpoch()
		adapter := &tournamentAdapterMock{}
		adapter.On("CommitmentStanding", mock.Anything, [32]byte(*epoch.Commitment)).
			Return(CommitmentStanding{}, errors.New("rpc down")).Once()
		boom := tournamentRevertError("TournamentIsClosed")
		err := s.handleJoinTournamentRevert(context.Background(), prtRevertTestApp(), epoch,
			adapter, boom)
		assert.Equal(t, boom, err,
			"when the re-check fails the original revert must propagate for a retry, not FAILED")
		adapter.AssertExpectations(t)
	})

	for _, revertName := range []string{"CommitmentStateMismatch", "CommitmentProofWrongSize"} {
		t.Run(revertName+"_setsCorrupted", func(t *testing.T) {
			s, r := newPRTServiceMock()
			defer r.AssertExpectations(t)
			app := prtRevertTestApp()
			epoch := prtRevertTestEpoch()
			r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted,
				mock.MatchedBy(reasonContains(
					"JoinTournament reverted with "+revertName, "epoch 3"))).
				Return(nil).Once()
			err := s.handleJoinTournamentRevert(context.Background(), app, epoch,
				&tournamentAdapterMock{}, tournamentRevertError(revertName))
			assert.Error(t, err, "CORRUPTED is terminal; the handler must return the reason error")
		})
	}

	t.Run("NonceTooLow_retries", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		err := s.handleJoinTournamentRevert(context.Background(), prtRevertTestApp(), prtRevertTestEpoch(),
			&tournamentAdapterMock{}, errors.New("nonce too low"))
		assert.NoError(t, err)
	})

	t.Run("unknown_propagates", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		boom := errors.New("boom")
		err := s.handleJoinTournamentRevert(context.Background(), prtRevertTestApp(), prtRevertTestEpoch(),
			&tournamentAdapterMock{}, boom)
		assert.Equal(t, boom, err)
	})
}
