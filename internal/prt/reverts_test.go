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

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	return &rpcDataError{code: 3, msg: "execution reverted", data: fmt.Sprintf("0x%x", abiErr.ID[:4])}
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
	return &rpcDataError{code: 3, msg: "execution reverted", data: fmt.Sprintf("0x%x", payload)}
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
	return &rpcDataError{code: 3, msg: "execution reverted", data: fmt.Sprintf("0x%x", abiErr.ID[:4])}
}

func prtRevertTestApp() *model.Application {
	return &model.Application{
		ID:                  7,
		Name:                "prt-app",
		IApplicationAddress: common.BigToAddress(common.Big1),
		ConsensusType:       model.Consensus_PRT,
		Status:              model.ApplicationStatus_OK,
		Enabled:             true,
	}
}

func prtRevertTestEpoch() *model.Epoch {
	tournament := common.BigToAddress(common.Big2)
	commitment := common.HexToHash("0xabcd")
	return &model.Epoch{
		Index:             3,
		TournamentAddress: &tournament,
		Commitment:        &commitment,
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

// TestHandleSettleRevert covers the Settle revert classification: already
// settled and transient conditions retry silently, bad local proofs mark the
// app CORRUPTED, a consensus/application binding mismatch marks it FAILED,
// and unknown errors propagate unchanged.
func TestHandleSettleRevert(t *testing.T) {
	const epochNumber = uint64(3)

	t.Run("IncorrectEpochNumber_undecodable_waitsForEventSync", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		err := s.handleSettleRevert(context.Background(), prtRevertTestApp(), epochNumber,
			daveConsensusRevertError("IncorrectEpochNumber"))
		assert.NoError(t, err)
	})

	t.Run("IncorrectEpochNumber_behind_waitsForEventSync", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		// received 3 < actual 5: the chain settled past us — already settled.
		err := s.handleSettleRevert(context.Background(), prtRevertTestApp(), epochNumber,
			daveRevertWithArgs("IncorrectEpochNumber", big.NewInt(3), big.NewInt(5)))
		assert.NoError(t, err)
	})

	t.Run("IncorrectEpochNumber_ahead_setsFailed", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		app := prtRevertTestApp()
		// received 7 > actual 5: local epoch index is ahead of the chain —
		// waiting for event sync would stall silently forever.
		r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
			mock.MatchedBy(reasonContains(
				"IncorrectEpochNumber", "epoch 7", "epoch 5", "ahead", "before re-enabling"))).
			Return(nil).Once()
		err := s.handleSettleRevert(context.Background(), app, epochNumber,
			daveRevertWithArgs("IncorrectEpochNumber", big.NewInt(7), big.NewInt(5)))
		assert.NoError(t, err)
	})

	t.Run("TournamentNotFinishedYet_retries", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		err := s.handleSettleRevert(context.Background(), prtRevertTestApp(), epochNumber,
			daveConsensusRevertError("TournamentNotFinishedYet"))
		assert.NoError(t, err, "a CanSettle/simulation race must retry, not surface an error")
	})

	for _, revertName := range []string{
		"InvalidOutputsMerkleRootProofSize",
		"InvalidOutputsMerkleRootProof",
	} {
		t.Run(revertName+"_setsCorrupted", func(t *testing.T) {
			s, r := newPRTServiceMock()
			defer r.AssertExpectations(t)
			app := prtRevertTestApp()
			r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted,
				mock.MatchedBy(reasonContains("Settle reverted with "+revertName, "epoch 3"))).
				Return(nil).Once()
			err := s.handleSettleRevert(context.Background(), app, epochNumber,
				daveConsensusRevertError(revertName))
			assert.Error(t, err, "CORRUPTED is terminal; the handler must return the reason error")
		})
	}

	t.Run("ApplicationForeclosed_retries", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		err := s.handleSettleRevert(context.Background(), prtRevertTestApp(), epochNumber,
			daveConsensusRevertError("ApplicationForeclosed"))
		assert.NoError(t, err, "must retry while the EVM reader records the foreclosure marker")
	})

	t.Run("ApplicationNotDeployed_setsFailed", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		app := prtRevertTestApp()
		r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
			mock.MatchedBy(reasonContains(
				"Settle reverted with ApplicationNotDeployed", "epoch 3", "before re-enabling"))).
			Return(nil).Once()
		err := s.handleSettleRevert(context.Background(), app, epochNumber,
			daveConsensusRevertError("ApplicationNotDeployed"))
		// SetFailedf returns nil on success; the FAILED write itself is
		// asserted by the mock expectation above.
		assert.NoError(t, err)
	})

	t.Run("ApplicationReverted_setsFailedWithReturnData", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		app := prtRevertTestApp()
		r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
			mock.MatchedBy(reasonContains(
				"Settle reverted with ApplicationReverted", "epoch 3",
				"Application return data: 0xdead", "before re-enabling"))).
			Return(nil).Once()
		err := s.handleSettleRevert(context.Background(), app, epochNumber,
			daveRevertWithArgs("ApplicationReverted",
				common.BigToAddress(common.Big1), []byte{0xde, 0xad}))
		assert.NoError(t, err)
	})

	t.Run("IllformedApplicationReturnData_setsFailed", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		app := prtRevertTestApp()
		r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
			mock.MatchedBy(reasonContains(
				"Settle reverted with IllformedApplicationReturnData", "epoch 3",
				"before re-enabling"))).
			Return(nil).Once()
		err := s.handleSettleRevert(context.Background(), app, epochNumber,
			daveConsensusRevertError("IllformedApplicationReturnData"))
		assert.NoError(t, err)
	})

	t.Run("NonceTooLow_retries", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		err := s.handleSettleRevert(context.Background(), prtRevertTestApp(), epochNumber,
			errors.New("nonce too low"))
		assert.NoError(t, err)
	})

	t.Run("unknown_propagates", func(t *testing.T) {
		s, r := newPRTServiceMock()
		defer r.AssertExpectations(t)
		boom := errors.New("boom")
		err := s.handleSettleRevert(context.Background(), prtRevertTestApp(), epochNumber, boom)
		assert.Equal(t, boom, err)
	})
}

// TestHandleJoinTournamentRevert covers the JoinTournament revert
// classification: an already-joined commitment retries silently (whether
// detected via ClockAlreadyInitialized or via the IsCommitmentJoined re-check
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
		t.Run(revertName+"_notJoined_setsFailed", func(t *testing.T) {
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
			adapter.On("IsCommitmentJoined", mock.Anything, [32]byte(*epoch.Commitment)).Return(false, nil).Once()
			err := s.handleJoinTournamentRevert(context.Background(), app, epoch,
				adapter, tournamentRevertError(revertName))
			assert.NoError(t, err)
			adapter.AssertExpectations(t)
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
		adapter.On("IsCommitmentJoined", mock.Anything, [32]byte(*epoch.Commitment)).Return(true, nil).Once()
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
		adapter.On("IsCommitmentJoined", mock.Anything, [32]byte(*epoch.Commitment)).Return(false, errors.New("rpc down")).Once()
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
