// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/cartesistatetransition"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/imultileveltournamentfactory"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	deploymentABIWordBytes      = 32
	deploymentUint64Bytes       = 8
	deploymentMethodMissingCode = -32601
	deploymentRulerBits         = model.Log2MaxAdvanceStatesPerEpoch + model.Log2MaxExecutionCycles + model.Log2MaxUarchCyclesPerMCycle
	deploymentWindowBits        = model.Log2MaxExecutionCycles + model.Log2MaxUarchCyclesPerMCycle
)

const (
	deploymentLevelsWord = iota
	deploymentStrideWord
	deploymentHeightWord
	deploymentResponseWord
	deploymentCommitmentWord
	deploymentAllowanceWord
)

type deploymentReadStatus uint8

const (
	deploymentReadAbsent deploymentReadStatus = iota
	deploymentReadValue
	deploymentReadMismatch
)

type deploymentReader struct {
	ctx        context.Context
	backend    readOnlyBackend
	consensus  abi.ABI
	factory    abi.ABI
	transition abi.ABI
	evidence   DeploymentEvidence
}

// ReadDeployment reads one selected hash without applying action or integrity-status policy.
func (a *DaveConsensusAdapterImpl) ReadDeployment(ctx context.Context, head txsender.BlockRef) (DeploymentEvidence, error) {
	evidence := DeploymentEvidence{Head: head, Consensus: a.address, Candidate: deploymentNone, Cause: deploymentCauseNone}
	if head.Hash == (common.Hash{}) {
		return evidence, fmt.Errorf("deployment observation requires an explicit block hash: %w", txsender.ErrInconclusive)
	}
	consensus, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	if err != nil {
		return evidence, err
	}
	factory, err := imultileveltournamentfactory.IMultiLevelTournamentFactoryMetaData.GetAbi()
	if err != nil {
		return evidence, err
	}
	transition, err := cartesistatetransition.CartesiStateTransitionMetaData.GetAbi()
	if err != nil {
		return evidence, err
	}
	r := deploymentReader{ctx: withLiveBlock(ctx, head.Number, head.Hash), backend: readOnlyBackend{Client: a.client},
		consensus: *consensus, factory: *factory, transition: *transition, evidence: evidence}
	return r.read()
}

func (r *deploymentReader) call(
	address common.Address, contractABI abi.ABI, name string, args ...any,
) ([]byte, deploymentReadStatus, error) {
	method, exists := contractABI.Methods[name]
	if !exists {
		return nil, deploymentReadAbsent, fmt.Errorf("deployment ABI has no method %s", name)
	}
	words, err := staticABIWordCount(method.Outputs)
	if err != nil {
		return nil, deploymentReadAbsent, err
	}
	input, err := contractABI.Pack(name, args...)
	if err != nil {
		return nil, deploymentReadAbsent, err
	}
	contract := bind.NewBoundContract(address, contractABI, r.backend, nil, nil)
	output, err := contract.CallRaw(pinnedCallOpts(r.ctx, r.evidence.Head.Number), input)
	if errors.Is(err, bind.ErrNoCode) {
		r.evidence.Cause = deploymentCauseEmpty
		return nil, deploymentReadAbsent, nil
	}
	if err != nil {
		var rpcError rpc.Error
		if errors.Is(err, bind.ErrNoBlockHashState) ||
			(errors.As(err, &rpcError) && rpcError.ErrorCode() == deploymentMethodMissingCode) {
			err = errors.Join(txsender.ErrUnsupported, err)
		}
		return nil, deploymentReadAbsent, fmt.Errorf("deployment %s at %s, block %d hash %s: %w",
			name, address, r.evidence.Head.Number, r.evidence.Head.Hash, err)
	}
	if len(output) == 0 {
		r.evidence.Cause = deploymentCauseEmpty
		return nil, deploymentReadAbsent, nil
	}
	malformed := func() ([]byte, deploymentReadStatus, error) {
		call := DeploymentCallEvidence{Target: address, Selector: [4]byte(method.ID),
			ReturnLength: uint64(len(output)), ReturnHash: crypto.Keccak256Hash(output)}
		if len(args) != 0 {
			call.HasLevel, call.Level = true, args[0].(uint64)
		}
		r.evidence.Malformed = &call
		r.evidence.Candidate, r.evidence.Cause = deploymentGeneration, deploymentCauseStaticShape
		return nil, deploymentReadMismatch, nil
	}
	if len(output) != words*deploymentABIWordBytes {
		return malformed()
	}
	_, err = method.Outputs.Unpack(output)
	if err == nil {
		for i, argument := range method.Outputs {
			if argument.Type.T == abi.AddressTy {
				start := i * deploymentABIWordBytes
				for _, padding := range output[start : start+deploymentABIWordBytes-common.AddressLength] {
					if padding != 0 {
						return malformed()
					}
				}
			}
		}
		return output, deploymentReadValue, nil
	}

	return malformed()
}

func deploymentWord(output []byte, index int) uint64 {
	end := (index + 1) * deploymentABIWordBytes
	return binary.BigEndian.Uint64(output[end-deploymentUint64Bytes : end])
}

func deploymentParameters(output []byte) DeploymentParameters {
	return DeploymentParameters{
		Levels:           deploymentWord(output, deploymentLevelsWord),
		Log2Step:         deploymentWord(output, deploymentStrideWord),
		Height:           deploymentWord(output, deploymentHeightWord),
		ResponseBudget:   deploymentWord(output, deploymentResponseWord),
		CommitmentBudget: deploymentWord(output, deploymentCommitmentWord),
		MaxAllowance:     deploymentWord(output, deploymentAllowanceWord),
	}
}

func (r *deploymentReader) read() (DeploymentEvidence, error) {
	e := &r.evidence
	output, status, err := r.call(e.Consensus, r.consensus, "getTournamentFactory")
	if status != deploymentReadValue {
		return *e, err
	}
	e.FactoryKnown, e.Factory = true, common.BytesToAddress(output)
	output, status, err = r.call(e.Factory, r.factory, "tournamentParameters", uint64(0))
	if status != deploymentReadValue {
		return *e, err
	}
	e.Alpha6Shape = true
	e.Rows = append(e.Rows, deploymentParameters(output))
	output, status, err = r.call(e.Factory, r.factory, "stateTransition")
	if status != deploymentReadValue {
		return *e, err
	}
	e.Transition = common.BytesToAddress(output)
	output, status, err = r.call(e.Transition, r.transition, "CM_MARCHID")
	if status != deploymentReadValue {
		return *e, err
	}
	e.MarchID = deploymentWord(output, 0)
	if e.MarchID != model.CartesiMachineMarchID {
		e.Candidate, e.Cause = deploymentGeneration, deploymentCauseMarchID
		return *e, nil
	}
	output, status, err = r.call(e.Transition, r.transition, "UARCH_PRISTINE_STATE_HASH")
	if status != deploymentReadValue {
		return *e, err
	}
	e.Pristine = common.BytesToHash(output)
	if e.Pristine != model.UarchPristineStateHash() {
		e.Candidate, e.Cause = deploymentGeneration, deploymentCausePristine
		return *e, nil
	}
	output, status, err = r.call(e.Factory, r.factory, "tournamentLevelCount")
	if status != deploymentReadValue {
		return *e, err
	}
	e.Count = deploymentWord(output, 0)
	if e.Count == 0 || e.Count > deploymentRulerBits {
		e.Candidate, e.Cause = deploymentGeneration, deploymentCauseCount
		return *e, nil
	}
	for level := uint64(1); level < e.Count; level++ {
		output, status, err = r.call(e.Factory, r.factory, "tournamentParameters", level)
		if status != deploymentReadValue {
			return *e, err
		}
		e.Rows = append(e.Rows, deploymentParameters(output))
	}
	e.Candidate, e.Cause = classifyDeploymentGeometry(e.Rows, e.Count), deploymentCauseComplete
	if e.Candidate == deploymentGeneration {
		e.Cause = deploymentCauseGeometry
	}
	return *e, nil
}

func classifyDeploymentGeometry(rows []DeploymentParameters, count uint64) deploymentState {
	if count == 0 || count > deploymentRulerBits || uint64(len(rows)) != count {
		return deploymentGeneration
	}
	for i, row := range rows {
		if row.Levels != count || row.Height == 0 || row.Log2Step > deploymentRulerBits ||
			row.Height > deploymentRulerBits || row.Log2Step+row.Height > deploymentRulerBits {
			return deploymentGeneration
		}
		if i == 0 && row.Log2Step+row.Height != deploymentRulerBits {
			return deploymentGeneration
		}
		if i != 0 && row.Log2Step+row.Height != rows[i-1].Log2Step {
			return deploymentGeneration
		}
	}
	if rows[len(rows)-1].Log2Step != 0 || rows[0].Log2Step < model.Log2MaxUarchCyclesPerMCycle ||
		rows[0].Log2Step > deploymentWindowBits {
		return deploymentGeneration
	}
	if count != model.TournamentLevelCount {
		return deploymentTable
	}
	for i, row := range rows {
		if !model.IsCanonicalTournamentLevel(uint64(i), row.Height, row.Log2Step) {
			return deploymentTable
		}
	}
	return deploymentCompatible
}
