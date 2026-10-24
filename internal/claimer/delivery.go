// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthority"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
	"time"
)

type claimKind uint8

const (
	submitClaimKind claimKind = iota + 1
	acceptClaimKind
	quorumSubmitClaimKind
)

// claimAction identifies immutable protocol content, not one signed attempt.
type claimAction struct {
	ApplicationID int64
	Address       common.Address
	Kind          claimKind
	Epoch         uint64
	Identity      common.Hash
}

type claimCandidate struct {
	app     *model.Application
	epoch   *model.Epoch
	action  claimAction
	request txsender.Request
}

type claimLookup interface {
	GetEpochCompletionState(context.Context, string, uint64) (*repository.EpochCompletionState, error)
}

type claimSignerMismatch struct{ reason string }

func (e *claimSignerMismatch) Error() string { return e.reason }
func (*claimSignerMismatch) Unwrap() error   { return txsender.ErrIneligible }

func claimCall(app *model.Application, epoch *model.Epoch, proof model.StateProof, submit bool) (claimCandidate, error) {
	contractABI, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		return claimCandidate{}, err
	}
	if epoch.MachineHash == nil {
		return claimCandidate{}, errors.New("claim has no machine root")
	}
	kind := acceptClaimKind
	var data []byte
	if submit {
		kind = submitClaimKind
		if app.ConsensusType == model.Consensus_Quorum {
			kind = quorumSubmitClaimKind
		}
		data, err = contractABI.Pack("submitClaim", app.IApplicationAddress,
			new(big.Int).SetUint64(epoch.LastBlock), proof.MachineHash, consensusMachineValidityProof(proof))
	} else {
		data, err = contractABI.Pack("acceptClaim", app.IApplicationAddress,
			new(big.Int).SetUint64(epoch.LastBlock), [32]byte(*epoch.MachineHash))
	}
	if err != nil {
		return claimCandidate{}, err
	}
	request := txsender.Request{To: app.IConsensusAddress, Data: data, Value: new(big.Int)}
	// A deciding Quorum vote can invoke the staging callbacks after a peer's
	// vote changes the branch. This measured bound covers our supported fixture.
	if kind == quorumSubmitClaimKind {
		request.GasLimit = quorumSubmitGasLimit
	}
	return claimCandidate{app: app, epoch: epoch, request: request, action: claimAction{
		ApplicationID: app.ID, Address: app.IApplicationAddress, Kind: kind, Epoch: epoch.Index,
		Identity: crypto.Keccak256Hash(app.IConsensusAddress.Bytes(), data),
	}}, nil
}

// decide reads current protocol state once for either initial planning or the
// chosen action's post-sign check. Stored status selects rows; chain state
// decides whether submission, acceptance, or no action is useful.
func (s *claimPublisher) decide(
	ctx context.Context, block txsender.BlockRef, work repository.ClaimWork, held bool,
) (claimCandidate, txsender.Eligibility, error) {
	state, err := s.lookup.GetEpochCompletionState(ctx, work.Application.IApplicationAddress.Hex(), work.Epoch.Index)
	if err != nil {
		return claimCandidate{}, 0, err
	}
	if state == nil || state.Application == nil {
		return claimCandidate{}, txsender.Obsolete, nil
	}
	app := state.Application
	status := app.Status
	if app.ID != work.Application.ID || !app.Enabled ||
		(status != model.ApplicationStatus_OK && !status.IsExecutionTerminal() && status != model.ApplicationStatus_InvalidOutputsRoot) ||
		(app.ConsensusType != model.Consensus_Authority && app.ConsensusType != model.Consensus_Quorum) {
		return claimCandidate{}, txsender.Obsolete, nil
	}
	if err := requireObservedBlock(block.Number, app.ForecloseBlock); err != nil {
		return claimCandidate{}, 0, err
	}
	if app.ForecloseBlock != 0 {
		return claimCandidate{}, txsender.Obsolete, nil
	}
	if state.Epoch == nil {
		if held {
			return claimCandidate{}, 0, txsender.ErrInconclusive
		}
		return claimCandidate{}, txsender.Obsolete, nil
	}
	allowed, err := state.AllowsEpochCompletion()
	if err != nil {
		return claimCandidate{}, 0, fmt.Errorf("%w: application %s (%s), epoch %d: %w",
			txsender.ErrInconclusive, app.Name, app.IApplicationAddress, work.Epoch.Index, err)
	}
	if !allowed {
		return claimCandidate{}, txsender.Obsolete, nil
	}
	epoch := state.Epoch
	if epoch.Index != work.Epoch.Index || epoch.ApplicationID != app.ID {
		return claimCandidate{}, txsender.Obsolete, nil
	}
	if !held && epoch.Status != model.EpochStatus_ClaimComputed && epoch.Status != model.EpochStatus_ClaimSubmitted &&
		epoch.Status != model.EpochStatus_ClaimStaged {
		return claimCandidate{}, txsender.Obsolete, nil
	}
	if err := requireObservedBlock(block.Number, epoch.LastBlock); err != nil {
		return claimCandidate{}, 0, err
	}
	if err := requireObservedStaging(epoch, block.Number); err != nil {
		return claimCandidate{}, 0, err
	}
	if epoch.MachineHash == nil || epoch.TxBufferDataBlock == nil {
		return claimCandidate{}, 0, fmt.Errorf("application %s (%s), epoch %d: "+
			"persisted claim is missing its machine or outputs root; repair stored data",
			app.Name, app.IApplicationAddress, epoch.Index)
	}
	appABI, err := iapplication.IApplicationMetaData.GetAbi()
	if err != nil {
		return claimCandidate{}, 0, err
	}
	related, err := s.readProtected(ctx, app.IApplicationAddress, block, appABI, "getOutputsMerkleRootValidator")
	if err != nil {
		return claimCandidate{}, 0, err
	}
	consensus, ok := firstValue[common.Address](related)
	if !ok {
		return claimCandidate{}, 0, txsender.ErrInconclusive
	}
	if consensus != app.IConsensusAddress {
		return claimCandidate{}, txsender.Obsolete, nil
	}
	values, err := s.readProtected(ctx, app.IApplicationAddress, block, appABI, "isForeclosed")
	if err != nil {
		return claimCandidate{}, 0, err
	}
	foreclosed, ok := firstValue[bool](values)
	if !ok {
		return claimCandidate{}, 0, txsender.ErrInconclusive
	}
	if foreclosed {
		return claimCandidate{}, txsender.Obsolete, nil
	}
	code, err := s.backend.Code(ctx, app.IConsensusAddress, block)
	if err != nil {
		return claimCandidate{}, 0, err
	}
	if len(code) == 0 {
		return claimCandidate{}, txsender.Obsolete, nil
	}
	contractABI, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		return claimCandidate{}, 0, err
	}
	values, err = s.readProtected(ctx, app.IConsensusAddress, block, contractABI, "getClaim",
		app.IApplicationAddress, new(big.Int).SetUint64(epoch.LastBlock), [32]byte(*epoch.MachineHash))
	if err != nil {
		return claimCandidate{}, 0, err
	}
	if len(values) != 1 {
		return claimCandidate{}, 0, txsender.ErrInconclusive
	}
	claim := *abi.ConvertType(values[0], new(iconsensus.IConsensusClaim)).(*iconsensus.IConsensusClaim)
	switch claim.Status {
	case claimStatusAccepted:
		return claimCandidate{}, txsender.Obsolete, nil
	case claimStatusStaged:
		if claim.StagedOutputsMerkleRoot != [32]byte(*epoch.TxBufferDataBlock) {
			return claimCandidate{}, txsender.Obsolete, nil
		}
		if claim.StagingBlockNumber == nil || !claim.StagingBlockNumber.IsUint64() ||
			block.Number < claim.StagingBlockNumber.Uint64() {
			return claimCandidate{}, 0, txsender.ErrInconclusive
		}
		candidate, err := claimCall(app, epoch, model.StateProof{}, false)
		if err != nil {
			return claimCandidate{}, 0, err
		}
		if block.Number-claim.StagingBlockNumber.Uint64() < app.ClaimStagingPeriod {
			return candidate, txsender.ProtocolWait, nil
		}
		return candidate, txsender.Useful, nil
	case claimStatusUnstaged:
		// Publication reads latest. A persisted stage under a non-provisional
		// observation policy cannot be repaired by resubmission: leave the
		// contradiction visible for the independently running observer.
		// requireObservedStaging above already excludes a head below the fact.
		if epoch.Status == model.EpochStatus_ClaimStaged && !s.observationPolicy.Provisional() {
			s.warnStagingContradiction(app, epoch, block.Number)
			return claimCandidate{}, 0, fmt.Errorf("%w: epoch %d recorded as CLAIM_STAGED is UNSTAGED at block %d",
				txsender.ErrInconclusive, epoch.Index, block.Number)
		}
		// submitClaim requires a block strictly after the epoch's final block.
		// This wait does not apply to accepting an already staged claim.
		if block.Number == epoch.LastBlock {
			return claimCandidate{}, txsender.ProtocolWait, nil
		}
		if err := s.authorizeSubmission(ctx, block, app, epoch); err != nil {
			if errors.Is(err, errClaimCompleted) {
				return claimCandidate{}, txsender.Obsolete, nil
			}
			return claimCandidate{}, 0, err
		}
		proof, err := epoch.StateProof()
		if err != nil {
			return claimCandidate{}, 0, err
		}
		candidate, err := claimCall(app, epoch, proof, true)
		return candidate, txsender.Useful, err
	default:
		return claimCandidate{}, 0, fmt.Errorf("%w: unsupported claim status %d", txsender.ErrUnsupported, claim.Status)
	}
}

var errClaimCompleted = fmt.Errorf("%w: matching vote already exists", txsender.ErrIneligible)

func (s *claimPublisher) authorizeSubmission(
	ctx context.Context, block txsender.BlockRef, app *model.Application, epoch *model.Epoch,
) error {
	if s.signer == (common.Address{}) {
		return fmt.Errorf("%w: claim signer is unavailable", txsender.ErrUnsupported)
	}
	switch app.ConsensusType {
	case model.Consensus_Authority:
		contractABI, err := iauthority.IAuthorityMetaData.GetAbi()
		if err != nil {
			return err
		}
		values, err := s.readProtected(ctx, app.IConsensusAddress, block, contractABI, "owner")
		if err != nil {
			return err
		}
		owner, ok := firstValue[common.Address](values)
		if !ok {
			return txsender.ErrInconclusive
		}
		if owner != s.signer {
			return &claimSignerMismatch{reason: fmt.Sprintf("application %s (%s): signer %s is not Authority owner %s",
				app.Name, app.IApplicationAddress, s.signer, owner)}
		}
	case model.Consensus_Quorum:
		contractABI, err := iquorum.IQuorumMetaData.GetAbi()
		if err != nil {
			return err
		}
		values, err := s.readProtected(ctx, app.IConsensusAddress, block, contractABI, "validatorId", s.signer)
		if err != nil {
			return err
		}
		validator, ok := firstValue[*big.Int](values)
		if !ok || validator == nil {
			return txsender.ErrInconclusive
		}
		if validator.Sign() <= 0 {
			return &claimSignerMismatch{reason: fmt.Sprintf("application %s (%s): signer %s is not a Quorum validator",
				app.Name, app.IApplicationAddress, s.signer)}
		}
		values, err = s.readProtected(ctx, app.IConsensusAddress, block, contractABI,
			"isValidatorInFavorOfAnyClaimInEpoch", app.IApplicationAddress, new(big.Int).SetUint64(epoch.LastBlock), validator)
		if err != nil {
			return err
		}
		voted, ok := firstValue[bool](values)
		if !ok {
			return txsender.ErrInconclusive
		}
		if voted {
			return errClaimCompleted
		}
	case model.Consensus_PRT:
		return txsender.ErrIneligible
	default:
		return txsender.ErrIneligible
	}
	return nil
}

func firstValue[T any](values []any) (T, bool) {
	var zero T
	if len(values) != 1 {
		return zero, false
	}
	value, ok := values[0].(T)
	return value, ok
}

func (s *claimPublisher) readProtected(
	ctx context.Context, address common.Address, block txsender.BlockRef, contractABI *abi.ABI, method string, args ...any,
) ([]any, error) {
	data, err := contractABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	result, err := s.backend.Call(ctx, ethereum.CallMsg{To: &address, Data: data}, block)
	if err != nil {
		return nil, err
	}
	return contractABI.Unpack(method, result)
}

func claimKindName(kind claimKind) string {
	switch kind {
	case submitClaimKind:
		return "authority-submit"
	case quorumSubmitClaimKind:
		return "quorum-vote"
	case acceptClaimKind:
		return "accept"
	default:
		return fmt.Sprintf("unknown-%d", kind)
	}
}

func isClaimSubmission(kind claimKind) bool {
	return kind == submitClaimKind || kind == quorumSubmitClaimKind
}

const (
	claimWarningInterval        = time.Minute
	quorumSubmitGasLimit uint64 = 650_000
)
