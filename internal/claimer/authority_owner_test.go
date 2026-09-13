// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthority"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

func TestAuthoritySubmissionOwnerDiagnosis(t *testing.T) {
	signer := common.HexToAddress("0x1")
	other := common.HexToAddress("0x2")
	revert := &rpcDataError{code: 3, msg: "execution reverted: original failure", data: "0xdeadbeef"}
	transportErr := errors.New("RPC transport failed")
	noDataErr := &rpcDataError{code: 3, msg: "submission reverted without data"}

	for _, test := range []struct {
		name            string
		consensus       model.Consensus
		submissionErr   error
		configuredOwner common.Address
		latestOwner     common.Address
		wantReads       bool
		wantOutcome     submitClaimRevertOutcome
	}{
		{name: "wrong agreed owner", consensus: model.Consensus_Authority, submissionErr: revert,
			configuredOwner: other, latestOwner: other, wantReads: true, wantOutcome: submitClaimAppHalted},
		{name: "correct owner unrelated revert", consensus: model.Consensus_Authority, submissionErr: revert,
			configuredOwner: signer, latestOwner: signer, wantReads: true},
		{name: "ownership transferred away in latest", consensus: model.Consensus_Authority, submissionErr: revert,
			configuredOwner: signer, latestOwner: other, wantReads: true, wantOutcome: submitClaimRetryLater},
		{name: "ownership transferred to signer in latest", consensus: model.Consensus_Authority, submissionErr: revert,
			configuredOwner: other, latestOwner: signer, wantReads: true, wantOutcome: submitClaimRetryLater},
		{name: "other owner changed between views", consensus: model.Consensus_Authority, submissionErr: revert,
			configuredOwner: other, latestOwner: common.HexToAddress("0x3"), wantReads: true, wantOutcome: submitClaimRetryLater},
		{name: "accept-only ClaimNotStaged", consensus: model.Consensus_Authority,
			submissionErr: claimNotStagedError(claimStatusAccepted), configuredOwner: other, latestOwner: other,
			wantReads: true, wantOutcome: submitClaimAppHalted},
		{name: "accept-only ClaimStagingPeriodNotOverYet", consensus: model.Consensus_Authority,
			submissionErr: consensusRevertError("ClaimStagingPeriodNotOverYet"), configuredOwner: other, latestOwner: other,
			wantReads: true, wantOutcome: submitClaimAppHalted},
		{name: "nonce rejection with revert data", consensus: model.Consensus_Authority,
			submissionErr: &rpcDataError{code: 3, msg: "nonce too low", data: "0xdeadbeef"}, wantOutcome: submitClaimRetryLater},
		{name: "transport failure", consensus: model.Consensus_Authority, submissionErr: transportErr},
		{name: "no revert data", consensus: model.Consensus_Authority, submissionErr: noDataErr},
		{name: "Quorum unchanged", consensus: model.Consensus_Quorum, submissionErr: revert},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, repo, _ := newServiceMock(t)
			app := makeApplication()
			app.ConsensusType = test.consensus
			epoch := makeComputedEpoch(app, 3)
			backend := &authorityOwnerRPC{}
			if test.wantReads {
				expectAuthorityHeader(backend)
				expectAuthorityOwner(t, backend, app.IConsensusAddress, 20, test.configuredOwner, nil)
				expectAuthorityOwner(t, backend, app.IConsensusAddress, rpc.LatestBlockNumber, test.latestOwner, nil)
			}
			blockchain := &claimerBlockchain{
				client: newAuthorityOwnerClient(t, backend), logger: s.Logger, defaultBlock: model.DefaultBlock_Finalized,
				txOptsFactory: &reportedSignerFactory{
					TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: signer}),
				},
			}
			submitter := &revertingClaimSubmitter{err: test.submissionErr}
			_, err := blockchain.submitClaimToBlockchain(t.Context(), submitter, app, epoch, model.StateProof{})
			require.ErrorIs(t, err, test.submissionErr)
			require.Equal(t, signer, submitter.from)
			if test.wantOutcome == submitClaimUnknown {
				require.Same(t, test.submissionErr, err, "an unrelated error must remain unchanged")
			}
			if test.wantOutcome == submitClaimAppHalted {
				repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
					mock.MatchedBy(func(reason *string) bool {
						return reason != nil && strings.Contains(*reason, signer.String()) &&
							strings.Contains(*reason, other.String()) && strings.Contains(*reason, "CARTESI_AUTH_*") &&
							strings.Contains(*reason, test.submissionErr.Error())
					})).Return(nil).Once()
			}
			outcome, statusErr := s.handleSubmitClaimRevert(err, app, epoch)
			require.NoError(t, statusErr)
			require.Equal(t, test.wantOutcome, outcome)
			if test.wantOutcome == submitClaimAppHalted {
				require.Equal(t, model.ApplicationStatus_Failed, app.Status)
			} else {
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}
			backend.AssertExpectations(t)
			repo.AssertExpectations(t)
		})
	}
}

func TestAuthorityOwnerReadFailurePreservesSubmissionError(t *testing.T) {
	for _, failureAt := range []string{"header", "configured owner", "latest owner"} {
		t.Run(failureAt, func(t *testing.T) {
			s, repo, _ := newServiceMock(t)
			app := makeApplication()
			app.ConsensusType = model.Consensus_Authority
			backend := &authorityOwnerRPC{}
			lookupErr := errors.New("owner RPC unavailable")
			if failureAt == "header" {
				backend.On("GetBlockByNumber", rpc.FinalizedBlockNumber, false).
					Return((*types.Header)(nil), lookupErr).Once()
			} else {
				expectAuthorityHeader(backend)
				if failureAt == "configured owner" {
					expectAuthorityOwner(t, backend, app.IConsensusAddress, 20, common.Address{}, lookupErr)
				} else {
					expectAuthorityOwner(t, backend, app.IConsensusAddress, 20, common.HexToAddress("0x2"), nil)
					expectAuthorityOwner(t, backend, app.IConsensusAddress, rpc.LatestBlockNumber, common.Address{}, lookupErr)
				}
			}
			blockchain := &claimerBlockchain{
				client: newAuthorityOwnerClient(t, backend), defaultBlock: model.DefaultBlock_Finalized,
			}
			original := &rpcDataError{code: 3, msg: "original submit failure", data: "0xdeadbeef"}
			err := blockchain.diagnoseAuthorityOwner(t.Context(), app, common.HexToAddress("0x1"), original)
			require.ErrorIs(t, err, original)
			require.ErrorContains(t, err, lookupErr.Error())
			outcome, statusErr := s.handleSubmitClaimRevert(err, app, makeComputedEpoch(app, 3))
			require.NoError(t, statusErr)
			require.Equal(t, submitClaimUnknown, outcome)
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			backend.AssertExpectations(t)
			repo.AssertExpectations(t)
		})
	}
}

func TestAuthorityOwnerDiagnosisPreservesKnownReverts(t *testing.T) {
	for _, test := range []struct {
		name string
		want submitClaimRevertOutcome
	}{
		{name: "NotFirstClaim", want: submitClaimAlreadyOnChain},
		{name: "ApplicationForeclosed", want: submitClaimRetryLater},
		{name: "CallerIsNotValidator", want: submitClaimAppHalted},
		{name: "InvalidSiblingsArrayLength", want: submitClaimAppHalted},
		{name: "InvalidMachineMerkleProof", want: submitClaimAppHalted},
		{name: "InvalidPostEpochMachineIflagsYRegister", want: submitClaimAppHalted},
		{name: "InvalidPostEpochMachineHtifTohostRegister", want: submitClaimAppHalted},
		{name: applicationNotDeployedRevert, want: submitClaimAppHalted},
		{name: applicationRevertedRevert, want: submitClaimAppHalted},
		{name: illformedApplicationReturnDataRevert, want: submitClaimAppHalted},
		{name: notEpochFinalBlockRevert, want: submitClaimAppHalted},
		{name: "NotPastBlock", want: submitClaimRetryLater},
	} {
		for _, ownerEvidence := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/owner_evidence_%t", test.name, ownerEvidence), func(t *testing.T) {
				s, repo, _ := newServiceMock(t)
				app := makeApplication()
				app.ConsensusType = model.Consensus_Authority
				epoch := makeComputedEpoch(app, 3)
				signer := common.HexToAddress("0x1")
				backend := &authorityOwnerRPC{}
				// No RPC expectation: a known submit revert must not query either
				// the configured head or the Authority owner.
				blockchain := &claimerBlockchain{
					client: newAuthorityOwnerClient(t, backend), logger: s.Logger, defaultBlock: model.DefaultBlock_Finalized,
					txOptsFactory: ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: signer}),
				}
				revert := consensusRevertError(test.name)
				submitter := &revertingClaimSubmitter{err: fmt.Errorf("estimating submitClaim: %w", revert)}
				_, err := blockchain.submitClaimToBlockchain(t.Context(), submitter, app, epoch, model.StateProof{})
				require.ErrorIs(t, err, revert)
				require.Same(t, submitter.err, err, "the wrapped submission error must remain unchanged")
				require.Empty(t, backend.Calls)
				if ownerEvidence {
					// Keep known-revert precedence explicit even if owner evidence is
					// supplied by another caller. This transfer must not mask a proof error.
					err = &authorityOwnerMismatch{signer: signer, configuredOwner: common.HexToAddress("0x2"),
						latestOwner: signer, submissionErr: err}
				}
				if test.want == submitClaimAppHalted {
					repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
						mock.MatchedBy(func(reason *string) bool {
							return reason != nil && strings.Contains(*reason, test.name) && !strings.Contains(*reason, "CARTESI_AUTH_*")
						})).Return(nil).Once()
				}
				outcome, statusErr := s.handleSubmitClaimRevert(err, app, epoch)
				require.NoError(t, statusErr)
				require.Equal(t, test.want, outcome)
				if test.want == submitClaimAppHalted {
					require.Equal(t, model.ApplicationStatus_Failed, app.Status)
				} else {
					require.Equal(t, model.ApplicationStatus_OK, app.Status)
				}
				backend.AssertExpectations(t)
				repo.AssertExpectations(t)
			})
		}
	}
}

// From deliberately differs from the signer returned in the transaction
// options. Owner diagnosis must use the signer on the attempted transaction.
type reportedSignerFactory struct{ ethutil.TransactOptsFactory }

func (*reportedSignerFactory) From() common.Address { return common.HexToAddress("0xffff") }

type revertingClaimSubmitter struct {
	err  error
	from common.Address
}

func (s *revertingClaimSubmitter) SubmitClaim(
	opts *bind.TransactOpts, _ common.Address, _ *big.Int, _ [32]byte, _ iconsensus.MachineValidityProof,
) (*types.Transaction, error) {
	s.from = opts.From
	return nil, s.err
}

type authorityOwnerRPC struct{ mock.Mock }

func (m *authorityOwnerRPC) GetBlockByNumber(
	_ context.Context, block rpc.BlockNumber, full bool,
) (*types.Header, error) {
	args := m.Called(block, full)
	header, _ := args.Get(0).(*types.Header)
	return header, args.Error(1)
}

func (m *authorityOwnerRPC) Call(
	_ context.Context, call map[string]json.RawMessage, block rpc.BlockNumber,
) (hexutil.Bytes, error) {
	var address common.Address
	var input hexutil.Bytes
	if err := json.Unmarshal(call["to"], &address); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(call["input"], &input); err != nil {
		return nil, err
	}
	args := m.Called(address, []byte(input), block)
	owner := args.Get(0).(common.Address)
	return common.LeftPadBytes(owner.Bytes(), common.HashLength), args.Error(1)
}

func newAuthorityOwnerClient(t *testing.T, backend *authorityOwnerRPC) *ethclient.Client {
	t.Helper()
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	return client
}

func expectAuthorityHeader(backend *authorityOwnerRPC) {
	backend.On("GetBlockByNumber", rpc.FinalizedBlockNumber, false).
		Return(&types.Header{Number: big.NewInt(20), Difficulty: big.NewInt(0), Extra: []byte{}}, nil).Once()
}

func expectAuthorityOwner(
	t *testing.T, backend *authorityOwnerRPC, address common.Address, block rpc.BlockNumber, owner common.Address, err error,
) {
	t.Helper()
	contractABI, parseErr := iauthority.IAuthorityMetaData.GetAbi()
	require.NoError(t, parseErr)
	backend.On("Call", address, contractABI.Methods["owner"].ID, block).Return(owner, err).Once()
}
