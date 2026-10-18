// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"strings"
	"testing"
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

// Contract simulation is a volatile verdict. A failed simulation cannot write
// application health, and an unpublished plan releases only its own reservation.
func TestHandleStageTournamentResultRevert(t *testing.T) {
	for _, name := range []string{"TournamentResultAlreadyStaged", "TournamentNotFinishedYet", applicationForeclosed,
		"InvalidSiblingsArrayLength", "InvalidMachineMerkleProof", "InvalidPostEpochMachineIflagsYRegister",
		"InvalidPostEpochMachineHtifTohostRegister", "ApplicationNotDeployed"} {
		t.Run(name, func(t *testing.T) { assertSimulationFailure(t, stageTournament, daveConsensusRevertError(name)) })
	}
	for _, cause := range []error{
		tournamentRevertError("TournamentFailedNoWinner"),
		daveRevertWithArgs("IncorrectEpochNumber", big.NewInt(3), big.NewInt(5)),
		daveRevertWithArgs("IncorrectEpochNumber", big.NewInt(7), big.NewInt(5)),
		daveRevertWithArgs("ApplicationReverted", common.BigToAddress(common.Big1), []byte{0xde, 0xad}),
		daveRevertWithArgs("IllformedApplicationReturnData", common.BigToAddress(common.Big1), []byte{0xde, 0xad}),
		errors.New("nonce too low"), errors.New("unknown RPC failure"),
	} {
		assertSimulationFailure(t, stageTournament, cause)
	}
}

func TestHandleAcceptTournamentResultRevert(t *testing.T) {
	names := []string{"TournamentResultNotStaged", "ClaimStagingPeriodNotOverYet", applicationForeclosed, "ApplicationNotDeployed"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) { assertSimulationFailure(t, acceptTournament, daveConsensusRevertError(name)) })
	}
	assertSimulationFailure(t, acceptTournament, errors.New("factory failed"))
}

func TestHandleJoinTournamentRevert(t *testing.T) {
	for _, name := range []string{"ClockAlreadyInitialized", "TournamentIsClosed", "TournamentIsFinished",
		"CommitmentStateMismatch", "CommitmentProofWrongSize"} {
		t.Run(name, func(t *testing.T) { assertSimulationFailure(t, joinTournament, tournamentRevertError(name)) })
	}
	assertSimulationFailure(t, joinTournament, errors.New("nonce too low"))
	assertSimulationFailure(t, joinTournament, errors.New("unknown RPC failure"))
}

func assertSimulationFailure(t *testing.T, kind actionKind, cause error) {
	t.Helper()
	f := newDeliveryFixture(t)
	view := resultTestSnapshot(f.epoch, kind == acceptTournament)
	view.stage.IsFinished = kind != joinTournament
	view.accept.IsClaimStagingPeriodOver = true
	f.snapshot(view)
	if kind == joinTournament {
		deliveryJoinAdapter(f, f.epoch, 30)
	}
	call, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
	require.NoError(t, err)
	require.NotNil(t, call)
	f.backend.preflightError = cause
	report, err := f.d.sender.PublishNext(t.Context(), singleAction{f.d, *call})
	require.ErrorIs(t, err, cause)
	require.False(t, report.Attempted)
	require.Empty(t, f.backend.published)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
