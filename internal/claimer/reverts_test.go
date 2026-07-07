// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDecodeClaimNotStagedStatus(t *testing.T) {
	t.Run("ValidStatuses", func(t *testing.T) {
		for _, s := range []uint8{0, 1, 2, 3} {
			err := claimNotStagedError(s)
			got, ok := decodeClaimNotStagedStatus(err)
			assert.True(t, ok, "status=%d should decode", s)
			assert.Equal(t, s, got, "status=%d should round-trip", s)
		}
	})

	t.Run("NilError", func(t *testing.T) {
		_, ok := decodeClaimNotStagedStatus(nil)
		assert.False(t, ok)
	})

	t.Run("PlainErrorNoData", func(t *testing.T) {
		_, ok := decodeClaimNotStagedStatus(fmt.Errorf("nope"))
		assert.False(t, ok)
	})

	t.Run("EmptyPayload", func(t *testing.T) {
		e := &rpcDataError{code: 3, msg: "execution reverted", data: "0x"}
		_, ok := decodeClaimNotStagedStatus(e)
		assert.False(t, ok)
	})

	t.Run("PayloadShorterThanSelector", func(t *testing.T) {
		e := &rpcDataError{code: 3, msg: "execution reverted", data: "0xabcd"}
		_, ok := decodeClaimNotStagedStatus(e)
		assert.False(t, ok)
	})

	t.Run("WrongSelector", func(t *testing.T) {
		e := &rpcDataError{
			code: 3,
			msg:  "execution reverted",
			// Valid 132-byte payload, but selector is for a different error.
			data: "0xdeadbeef" + strings.Repeat("00", 128),
		}
		_, ok := decodeClaimNotStagedStatus(e)
		assert.False(t, ok)
	})

	t.Run("RightSelectorTruncatedBody", func(t *testing.T) {
		parsed, _ := iconsensus.IConsensusMetaData.GetAbi()
		abiErr := parsed.Errors["ClaimNotStaged"]
		// Selector + only 1 slot — Unpack must fail rather than silently
		// returning a stale byte.
		payload := append(append([]byte{}, abiErr.ID[:4]...), make([]byte, 32)...)
		e := &rpcDataError{
			code: 3,
			msg:  "execution reverted",
			data: fmt.Sprintf("0x%x", payload),
		}
		_, ok := decodeClaimNotStagedStatus(e)
		assert.False(t, ok)
	})
}

// //////////////////////////////////////////////////////////////////////////////
// Success
// //////////////////////////////////////////////////////////////////////////////

func TestNotFirstClaimHandledGracefully(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)

	expectPreSubmitPath(b, app, currEpoch, endBlock)
	// submitClaim reverts with NotFirstClaim (caught by eth_estimateGas).
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(common.Hash{}, notFirstClaimError()).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(
		makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, len(m.claimsInFlight))
}

// TestNotFirstClaimQuorumRetriesForEventSync verifies that when submitClaim
// reverts with NotFirstClaim for a Quorum app, the claimer waits for event
// sync instead of changing app health from the selector alone. In v3,
// Quorum raises NotFirstClaim for any prior validator vote in the epoch,
// including a duplicate vote for the same machine root.
func TestNotFirstClaimQuorumRetriesForEventSync(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)

	expectPreSubmitPath(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(common.Hash{}, notFirstClaimError()).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(
		makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, len(m.claimsInFlight))
}

// TestApplicationForeclosedIsTransient verifies that a submitClaim revert
// with ApplicationForeclosed is treated as transient: no error is surfaced,
// no state transition happens, and the epoch stays in computedEpochs so the
// next tick can retry while the EVM reader records foreclosure and future
// claim broadcasts are skipped.
func TestApplicationForeclosedIsTransient(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)

	expectPreSubmitPath(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(common.Hash{}, consensusRevertError("ApplicationForeclosed")).Once()

	currEpochs := makeEpochMap(currEpoch)
	transitions, errs := m.submitClaimsAndUpdateDatabase(
		makeEpochMap(), currEpochs, makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, transitions, "no DB transition on transient revert")
	assert.Equal(t, 0, len(errs), "ApplicationForeclosed must not surface as an error")
	assert.Equal(t, 1, len(currEpochs), "epoch must remain in work map for retry")
	assert.Equal(t, 0, len(m.claimsInFlight), "no claim in flight")
}

// TestInvalidOutputsMerkleRootProofSizeSetsCorrupted verifies that a
// proof-size revert is treated as local data corruption — the app moves
// to CORRUPTED.
func TestInvalidOutputsMerkleRootProofSizeSetsCorrupted(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)

	expectPreSubmitPath(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(common.Hash{}, consensusRevertError("InvalidOutputsMerkleRootProofSize")).Once()
	r.On("UpdateApplicationStatus", mock.Anything, int64(0), model.ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).Once()

	currEpochs := makeEpochMap(currEpoch)
	_, errs := m.submitClaimsAndUpdateDatabase(
		makeEpochMap(), currEpochs, makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs), "CORRUPTED transition must surface a terminal error")
	assert.Equal(t, 0, len(currEpochs), "epoch must be dropped from work map")
	assert.Equal(t, 0, len(m.claimsInFlight))
}

// TestCallerIsNotValidatorSetsFailed verifies that a Quorum membership
// failure is treated as a recoverable operator-config error: FAILED, not a
// terminal DIVERGED/CORRUPTED status.
func TestCallerIsNotValidatorSetsFailed(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)

	expectPreSubmitPath(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(common.Hash{}, consensusRevertError("CallerIsNotValidator")).Once()
	r.On("UpdateApplicationStatus", mock.Anything, int64(0), model.ApplicationStatus_Failed, mock.Anything).
		Return(nil).Once()

	currEpochs := makeEpochMap(currEpoch)
	_, errs := m.submitClaimsAndUpdateDatabase(
		makeEpochMap(), currEpochs, makeApplicationMap(app), endBlock)
	// SetFailedf returns nil on success — the call site only surfaces an
	// error when state-update itself failed, so no error is expected here.
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, len(currEpochs), "epoch must be dropped from work map")
}

// TestNotPastBlockRetriesLater verifies that a submitClaim revert with
// NotPastBlock is treated as transient: the RPC provider may simulate against
// a block newer/different from the claimer's pinned reads, so the epoch stays
// in the work map for the next tick and no status changes.
func TestNotPastBlockRetriesLater(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)

	expectPreSubmitPath(b, app, currEpoch, endBlock)
	// The revert carries the contract's (lastProcessedBlockNumber, upperBound)
	// arguments, exercising the bounds decode in the warn path.
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(common.Hash{}, notPastBlockError(currEpoch.LastBlock, currEpoch.LastBlock-1)).Once()

	currEpochs := makeEpochMap(currEpoch)
	transitions, errs := m.submitClaimsAndUpdateDatabase(
		makeEpochMap(), currEpochs, makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, transitions, "no DB transition on transient revert")
	assert.Equal(t, 0, len(errs), "NotPastBlock must not surface as an error")
	assert.Equal(t, 1, len(currEpochs), "epoch must remain in work map for retry")
	assert.Equal(t, 0, len(m.claimsInFlight), "no claim in flight")
}

// TestSubmitClaimRevertsSetApplicationFailed verifies that the submit reverts
// pointing at operator/deploy/config problems mark the app FAILED (recoverable
// by operator action) rather than DIVERGED/CORRUPTED, with a reason carrying
// enough operational context. For the reverts that wrap the application's
// returndata, the reason must also carry it hex-encoded — it is the only
// evidence of why the application call failed.
func TestSubmitClaimRevertsSetApplicationFailed(t *testing.T) {
	cases := []struct {
		revertName string
		err        error
		// extraReason lists substrings expected in the FAILED reason beyond
		// the shared operational context (revert name, app address, epoch
		// index, last_block, operator hint).
		extraReason []string
	}{
		{
			revertName:  "ApplicationReverted",
			err:         appRevertDataError("ApplicationReverted", []byte{0xde, 0xad, 0xbe, 0xef}),
			extraReason: []string{"Application return data: 0xdeadbeef"},
		},
		{
			revertName:  "IllformedApplicationReturnData",
			err:         appRevertDataError("IllformedApplicationReturnData", []byte{0x01, 0x02}),
			extraReason: []string{"Application return data: 0x0102"},
		},
		{
			revertName: "NotEpochFinalBlock",
			err:        consensusRevertError("NotEpochFinalBlock"),
		},
		{
			revertName: "ApplicationNotDeployed",
			err:        consensusRevertError("ApplicationNotDeployed"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.revertName, func(t *testing.T) {
			m, r, b := newServiceMock()
			defer r.AssertExpectations(t)
			defer b.AssertExpectations(t)

			endBlock := big.NewInt(40)
			app := makeApplication()
			currEpoch := makeComputedEpoch(app, 3)

			expectPreSubmitPath(b, app, currEpoch, endBlock)
			b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
				Return(common.Hash{}, tc.err).Once()
			r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
				mock.MatchedBy(func(reason *string) bool {
					if reason == nil {
						return false
					}
					expected := append([]string{
						tc.revertName,
						app.IApplicationAddress.Hex(),
						fmt.Sprintf("epoch %d", currEpoch.Index),
						fmt.Sprintf("last_block %d", currEpoch.LastBlock),
						"before re-enabling",
					}, tc.extraReason...)
					for _, want := range expected {
						if !strings.Contains(*reason, want) {
							return false
						}
					}
					return true
				})).
				Return(nil).Once()

			currEpochs := makeEpochMap(currEpoch)
			transitions, errs := m.submitClaimsAndUpdateDatabase(
				makeEpochMap(), currEpochs, makeApplicationMap(app), endBlock)
			// SetFailedf returns nil on success — the call site only surfaces
			// an error when the status update itself failed.
			assert.Equal(t, 0, transitions, "FAILED is not a claim transition")
			assert.Equal(t, 0, len(errs))
			assert.Equal(t, 0, len(currEpochs), "epoch must be dropped from work map")
			assert.Equal(t, 0, len(m.claimsInFlight), "no claim in flight")
		})
	}
}

// TestSubmitClaimFailedRevertWithDBError verifies the submit-path behavior
// when the FAILED status write itself fails: the handler still returns
// AppHalted, so the epoch is dropped and the DB error surfaces.
func TestSubmitClaimFailedRevertWithDBError(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)

	expectPreSubmitPath(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(common.Hash{}, consensusRevertError("ApplicationReverted")).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
		Return(fmt.Errorf("db down")).Once()

	currEpochs := makeEpochMap(currEpoch)
	_, errs := m.submitClaimsAndUpdateDatabase(
		makeEpochMap(), currEpochs, makeApplicationMap(app), endBlock)
	assert.Equal(t, 1, len(errs), "status-update failure must surface as an error")
	if len(errs) == 1 {
		assert.ErrorContains(t, errs[0], "db down", "the surfaced error must be the DB error")
	}
	assert.Equal(t, 0, len(currEpochs), "epoch must be dropped from work map")
	assert.Equal(t, 0, len(m.claimsInFlight), "no claim in flight")
}

func TestHandleAcceptClaimRevert(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want acceptClaimRevertOutcome
	}{
		{
			name: "ClaimNotStaged_ACCEPTED_reconciles",
			err:  claimNotStagedError(claimStatusAccepted),
			want: acceptClaimReconciledAccepted,
		},
		{
			name: "ClaimNotStaged_UNSTAGED_retries",
			err:  claimNotStagedError(claimStatusUnstaged),
			want: acceptClaimRetryLater,
		},
		{
			name: "ClaimNotStaged_STAGED_retries",
			err:  claimNotStagedError(claimStatusStaged),
			want: acceptClaimRetryLater,
		},
		{
			// Selector matches ClaimNotStaged but the body is undecodable.
			// "Decode failed" is plausibly transient (a malformed RPC
			// response), so it must retry — NOT escalate. The decoded-but-
			// unmodeled case (status the contract returns that we don't model)
			// is the opposite and is covered by
			// TestClaimNotStagedUnmodeledStatusFailsClosed.
			name: "ClaimNotStaged_undecodable_retries",
			err:  consensusRevertError("ClaimNotStaged"),
			want: acceptClaimRetryLater,
		},
		{
			name: "ClaimStagingPeriodNotOverYet_retries",
			err:  consensusRevertError("ClaimStagingPeriodNotOverYet"),
			want: acceptClaimRetryLater,
		},
		{
			name: "ApplicationForeclosed_retries",
			err:  consensusRevertError("ApplicationForeclosed"),
			want: acceptClaimRetryLater,
		},
		{
			name: "NotPastBlock_retries",
			err:  consensusRevertError("NotPastBlock"),
			want: acceptClaimRetryLater,
		},
		{
			name: "NonceTooLow_retries",
			err:  fmt.Errorf("nonce too low"),
			want: acceptClaimRetryLater,
		},
		{
			name: "NonceTooLow_wrapped_retries",
			err:  fmt.Errorf("send transaction: %w", fmt.Errorf("[nonce too low]")),
			want: acceptClaimRetryLater,
		},
		{
			name: "unknown_error_returns_unknown",
			err:  fmt.Errorf("some non-typed RPC failure"),
			want: acceptClaimUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _ := newServiceMock()
			app := makeApplication()
			epoch := makeStagedEpoch(app, 3, 50)
			outcome, stateErr := m.handleAcceptClaimRevert(tc.err, app, epoch)
			assert.Equal(t, tc.want, outcome)
			assert.Nil(t, stateErr, "classifier must not mutate state")
		})
	}
}

// TestDecodeNotPastBlockBounds pins the ABI-decode path used by the
// NotPastBlock warn log. The (lastProcessedBlockNumber, upperBound) pair is
// what lets an operator tell a lagging provider from a node pointed at the
// wrong chain, so it must round-trip; undecodable payloads must report !ok
// rather than garbage.
func TestDecodeNotPastBlockBounds(t *testing.T) {
	t.Run("ValidPayload", func(t *testing.T) {
		lastProcessed, upperBound, ok := decodeNotPastBlockBounds(notPastBlockError(39, 12))
		assert.True(t, ok)
		assert.Equal(t, uint64(39), lastProcessed.Uint64())
		assert.Equal(t, uint64(12), upperBound.Uint64())
	})

	t.Run("SelectorOnly", func(t *testing.T) {
		_, _, ok := decodeNotPastBlockBounds(consensusRevertError("NotPastBlock"))
		assert.False(t, ok)
	})

	t.Run("WrongSelector", func(t *testing.T) {
		_, _, ok := decodeNotPastBlockBounds(consensusRevertError("NotEpochFinalBlock"))
		assert.False(t, ok)
	})
}

// TestClaimNotStagedUnmodeledStatusFailsClosed verifies the open-enum guard: a
// ClaimNotStaged revert that decodes cleanly but carries a ClaimStatus this
// node does not model (here 3 — the enum is 0/1/2) must escalate the app to
// FAILED, not silently retry forever. This distinguishes "decoded a value we
// do not model" (a contract newer than this node) from "decode failed" (a
// transient malformed response), which still retries.
func TestClaimNotStagedUnmodeledStatusFailsClosed(t *testing.T) {
	m, r, _ := newServiceMock()
	defer r.AssertExpectations(t)
	app := makeApplication()
	epoch := makeStagedEpoch(app, 3, 50)

	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
		Return(nil).Once()

	outcome, stateErr := m.handleAcceptClaimRevert(claimNotStagedError(3), app, epoch)
	assert.Equal(t, acceptClaimAppHalted, outcome,
		"a cleanly-decoded unmodeled ClaimStatus must escalate, not retry")
	// SetFailedf returns nil on success; the FAILED write itself is asserted by
	// the mock expectation above.
	assert.NoError(t, stateErr)
}

// TestAcceptClaimRevertsSetApplicationFailed verifies that acceptClaim shares
// submitClaim's classification for the reverts both methods can raise: the
// application foreclosure probe failures and NotEpochFinalBlock mark the app
// FAILED with a reason naming acceptClaim and carrying operational context.
func TestAcceptClaimRevertsSetApplicationFailed(t *testing.T) {
	for _, revertName := range []string{
		"ApplicationNotDeployed",
		"ApplicationReverted",
		"IllformedApplicationReturnData",
		"NotEpochFinalBlock",
	} {
		t.Run(revertName, func(t *testing.T) {
			m, r, _ := newServiceMock()
			defer r.AssertExpectations(t)
			app := makeApplication()
			epoch := makeStagedEpoch(app, 3, 50)

			r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
				mock.MatchedBy(func(reason *string) bool {
					if reason == nil {
						return false
					}
					for _, want := range []string{
						"acceptClaim reverted with " + revertName,
						app.IApplicationAddress.Hex(),
						fmt.Sprintf("epoch %d", epoch.Index),
						fmt.Sprintf("last_block %d", epoch.LastBlock),
						"before re-enabling",
					} {
						if !strings.Contains(*reason, want) {
							return false
						}
					}
					return true
				})).
				Return(nil).Once()

			outcome, stateErr := m.handleAcceptClaimRevert(consensusRevertError(revertName), app, epoch)
			assert.Equal(t, acceptClaimAppHalted, outcome)
			// SetFailedf returns nil on success; the FAILED write itself is
			// asserted by the mock expectation above.
			assert.NoError(t, stateErr)
		})
	}
}

// TestInvalidNodeIndexSetsCorrupted verifies that a submitClaim revert from
// the on-chain merkle library is treated as local data corruption: the stored
// outputs_merkle_proof does not form a valid machine-tree replacement proof,
// so the app moves to CORRUPTED.
func TestInvalidNodeIndexSetsCorrupted(t *testing.T) {
	m, r, _ := newServiceMock()
	defer r.AssertExpectations(t)
	app := makeApplication()
	epoch := makeComputedEpoch(app, 3)

	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).Once()

	outcome, stateErr := m.handleSubmitClaimRevert(consensusRevertError("InvalidNodeIndex"), app, epoch)
	assert.Equal(t, submitClaimAppHalted, outcome)
	assert.Error(t, stateErr, "CORRUPTED is terminal; the handler must return the reason error")
}

// TestHandleSubmitClaimRevert — dispatch matrix for the non-mutating typed
// reverts handleSubmitClaimRevert recognises plus the JSON-RPC
// "nonce too low" broadcast rejection. The classifier mutates state only
// for the AppHalted outcomes (InvalidOutputsMerkleRootProofSize,
// InvalidNodeIndex, CallerIsNotValidator, ApplicationNotDeployed,
// ApplicationReverted, IllformedApplicationReturnData, NotEpochFinalBlock);
// those paths are covered by the end-to-end submit pipeline tests above and
// the direct dispatch tests, with repository expectations for the status
// write.
func TestHandleSubmitClaimRevert(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want submitClaimRevertOutcome
	}{
		{
			name: "NotFirstClaim_Authority_alreadyOnChain",
			err:  consensusRevertError("NotFirstClaim"),
			want: submitClaimAlreadyOnChain,
		},
		{
			name: "ApplicationForeclosed_retries",
			err:  consensusRevertError("ApplicationForeclosed"),
			want: submitClaimRetryLater,
		},
		{
			name: "NotPastBlock_retries",
			err:  consensusRevertError("NotPastBlock"),
			want: submitClaimRetryLater,
		},
		{
			name: "NonceTooLow_retries",
			err:  fmt.Errorf("nonce too low"),
			want: submitClaimRetryLater,
		},
		{
			name: "NonceTooLow_wrapped_retries",
			err:  fmt.Errorf("send transaction: %w", fmt.Errorf("[nonce too low]")),
			want: submitClaimRetryLater,
		},
		{
			name: "unknown_error_returns_unknown",
			err:  fmt.Errorf("some non-typed RPC failure"),
			want: submitClaimUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _ := newServiceMock()
			app := makeApplication()
			// Authority is the default; NotFirstClaim returns
			// AlreadyOnChain for it. Quorum-specific routing is
			// covered by the existing end-to-end pipeline tests.
			app.ConsensusType = model.Consensus_Authority
			epoch := makeEpoch(app.ID, model.EpochStatus_ClaimComputed, 3)
			outcome, _ := m.handleSubmitClaimRevert(tc.err, app, epoch)
			assert.Equal(t, tc.want, outcome)
		})
	}
}

// TestVerifyClaimOutputsMismatch — pre-accept getClaim returns STAGED but
// with a stagedOutputsMerkleRoot that disagrees with the local epoch. The
// app is set DIVERGED with the chain_claim_outputs_mismatch reason; no
