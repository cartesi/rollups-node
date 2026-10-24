// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// delivery belongs to the publisher goroutine. It stores traversal positions,
// cooldowns and diagnostics; signed requests belong only to txsender.
type delivery struct {
	*Service
	sender                *txsender.Sender[actionID]
	deliveryBackend       txsender.Backend
	deliveryPolicy        txsender.Policy
	deliverySweeps        map[actionKind]*applicationSweep
	rootBondRecoveryScans map[int64]*rootBondRecoveryScan
	candidateCooldown     map[actionID]time.Time
	observedFacts         txsender.BlockRef
	diagnostics           publicationDiagnostics
}

type actionID struct {
	ApplicationID int64
	Application   common.Address
	Target        common.Address
	Kind          actionKind
	Epoch         uint64
	Identity      common.Hash
	Deadline      uint64
}

type plannedAction struct {
	Key actionID
	txsender.Request
	Deadline uint64
}

type applicationSweep struct{ after, through int64 }

const (
	deliveryApplicationPageSize uint64 = 8
	joinWarningMargin           uint64 = 6
	publicationCandidateLimit          = 16
)

func (s *delivery) nextDeliveryApplications(ctx context.Context, kind actionKind) ([]*model.Application, error) {
	filter := repository.ApplicationFilter{ConsensusType: new(model.Consensus_PRT), Enabled: new(true)}
	sweep := s.deliverySweeps[kind]
	if sweep == nil {
		end, err := s.repository.GetApplicationIDBoundary(ctx, filter)
		if err != nil {
			return nil, err
		}
		sweep = &applicationSweep{through: end}
		s.deliverySweeps[kind] = sweep
	}
	apps, err := s.repository.ListApplicationsByID(ctx, filter, sweep.after, sweep.through, deliveryApplicationPageSize)
	if err == nil {
		if len(apps) == 0 {
			delete(s.deliverySweeps, kind)
		} else {
			sweep.after = apps[len(apps)-1].ID
			if sweep.after >= sweep.through {
				delete(s.deliverySweeps, kind)
			}
		}
	}
	return apps, err
}

func (s *delivery) guardPublication(ctx context.Context) error {
	if s.publicationStopped() {
		return s.publicationFailure()
	}
	return ctx.Err()
}

func (s *delivery) observationReference(ctx context.Context) (txsender.BlockRef, error) {
	if s.publicationStopped() {
		return txsender.BlockRef{}, s.publicationFailure()
	}
	candidate, err := s.validatedObservationReference(ctx, s.observedFacts)
	if errors.Is(err, txsender.ErrFinality) {
		s.stopPublication(err)
	}
	if err == nil {
		s.observedFacts = candidate
	}
	return candidate, err
}

func applicationEligible(app *model.Application, snapshot txsender.BlockRef) error {
	if !app.Enabled {
		return txsender.ErrIneligible
	}
	if app.ForecloseBlock > snapshot.Number {
		return txsender.ErrInconclusive
	}
	if !completionStatusAllowed(app.Status) || app.IsForeclosed() {
		return txsender.ErrIneligible
	}
	return nil
}

func (s *delivery) planAction(
	ctx context.Context, app *model.Application, kind actionKind, snap txsender.BlockRef, facts txsender.BlockRef,
) (*plannedAction, error) {
	return s.planEpochAction(ctx, app, kind, snap, facts, nil)
}

// Fixed-action validation remains independent of incomplete fresh-work observation.
// Current protected protocol checks still apply.
func (s *delivery) planEpochAction(
	ctx context.Context, app *model.Application, kind actionKind, snap txsender.BlockRef,
	facts txsender.BlockRef, expected *actionID,
) (*plannedAction, error) {
	ctx = withLiveBlock(ctx, snap.Number, snap.Hash)
	if kind == recoverBond {
		return s.planRecovery(ctx, app, snap, facts)
	}
	revalidating := expected != nil
	retained := revalidating
	if err := applicationEligible(app, snap); err != nil {
		return nil, err
	}
	consensus, err := s.adapterFactory.CreateDaveConsensusAdapter(app.IConsensusAddress)
	if err != nil {
		return nil, err
	}
	view, err := readDaveConsensusSnapshot(ctx, consensus, snap.Number)
	if err != nil {
		return nil, fmt.Errorf("application %s (%s), consensus snapshot block %d hash %s: %w",
			app.Name, app.IApplicationAddress, snap.Number, snap.Hash, err)
	}
	if revalidating && view.sealed.EpochNumber != expected.Epoch {
		return nil, txsender.ErrIneligible
	}
	state, err := s.repository.GetEpochCompletionState(ctx, app.IApplicationAddress.Hex(), view.sealed.EpochNumber)
	if err != nil {
		return nil, err
	}
	if state == nil || state.Application == nil || state.Application.ID != app.ID {
		return nil, txsender.ErrIneligible
	}
	current := state.Application
	if current.ConsensusType != model.Consensus_PRT || current.IConsensusAddress != app.IConsensusAddress {
		return nil, txsender.ErrIneligible
	}
	if err := applicationEligible(current, snap); err != nil {
		return nil, err
	}
	if state.Epoch == nil {
		if retained {
			return nil, txsender.ErrInconclusive
		}
		return nil, txsender.ErrIneligible
	}
	allowed, err := state.AllowsEpochCompletion()
	if err != nil {
		return nil, fmt.Errorf("%w: application %s epoch %d: %w", txsender.ErrInconclusive, current.Name, view.sealed.EpochNumber, err)
	}
	if !allowed {
		return nil, txsender.ErrIneligible
	}
	app, epoch := current, state.Epoch
	if epoch.Index != view.sealed.EpochNumber {
		return nil, txsender.ErrInconclusive
	}
	if revalidating {
		if epoch.StagedAtBlock != nil && *epoch.StagedAtBlock > snap.Number {
			return nil, txsender.ErrInconclusive
		}
		if epoch.TournamentAddress != nil {
			root, loadErr := s.repository.GetTournament(ctx, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex())
			if loadErr != nil {
				return nil, loadErr
			}
			if root != nil && root.Snapshot.AsOfBlock > snap.Number {
				return nil, txsender.ErrInconclusive
			}
		}
	}
	// Stored progress selects fresh work. A retained call instead follows the
	// current protocol state, even if a provisional terminal observation remains.
	if !retained && !pendingTournamentResult(epoch) {
		return nil, txsender.ErrIneligible
	}
	if epoch.TournamentAddress == nil || epoch.Commitment == nil || epoch.MachineHash == nil || epoch.TxBufferDataBlock == nil {
		if revalidating {
			return nil, txsender.ErrInconclusive
		}
		return nil, txsender.ErrIneligible
	}
	if err = matchConsensusSnapshotToEpoch(epoch, view); err != nil {
		if revalidating {
			return nil, fmt.Errorf("%w: %w", txsender.ErrIneligible, err)
		}
		return nil, err
	}
	if !revalidating && epoch.LastBlock > facts.Number {
		return nil, fmt.Errorf("%w: observation block %d precedes epoch %d seal block %d",
			txsender.ErrInconclusive, facts.Number, epoch.Index, epoch.LastBlock)
	}
	if view.sealed.Tournament != *epoch.TournamentAddress {
		return nil, txsender.ErrIneligible
	}
	if !revalidating {
		if err = s.requirePublishedRoot(ctx, app, epoch, facts); err != nil {
			return nil, err
		}
	}
	switch kind {
	case joinTournament:
		if view.stage.IsFinished || view.stage.IsTournamentFailed || view.sealed.IsTournamentResultStaged ||
			!retained && epoch.Status != model.EpochStatus_ClaimComputed {
			return nil, nil
		}
	case stageTournament:
		if !view.stage.IsFinished || view.stage.IsTournamentFailed || view.sealed.IsTournamentResultStaged {
			if revalidating && !view.stage.IsFinished && !view.stage.IsTournamentFailed &&
				!view.sealed.IsTournamentResultStaged {
				return nil, errProtocolWait
			}
			return nil, nil
		}
	case acceptTournament:
		if !view.sealed.IsTournamentResultStaged {
			return nil, nil
		}
		if !view.accept.DoAllSentriesAgreeWithStagedTournamentResult && !view.accept.IsClaimStagingPeriodOver {
			if revalidating {
				return nil, errProtocolWait
			}
			return nil, nil
		}
	case noAction:
		return nil, errors.New("neutral resolution has no PRT protocol plan")
	case recoverBond:
		return nil, errors.New("bond recovery uses its own plan")
	default:
		return nil, fmt.Errorf("unsupported PRT action %d", kind)
	}
	// Registration is the operator's trust boundary for deployed consensus,
	// factory, clone implementation and economic parameters. Verify relationships
	// and supported geometry here; code presence alone does not establish trust.
	if err = s.validateApplicationLinks(ctx, app, snap); err != nil {
		if err != txsender.ErrIneligible && errors.Is(err, txsender.ErrIneligible) {
			if kind == joinTournament {
				return nil, &joinRefusal{epoch: epoch.Index, tournament: *epoch.TournamentAddress, reason: err.Error()}
			}
			return nil, &consensusRefusal{key: actionID{ApplicationID: app.ID, Kind: kind, Epoch: epoch.Index}, reason: err}
		}
		return nil, err
	}
	var call plannedAction
	if kind == joinTournament {
		call, err = s.joinCall(ctx, app, epoch, snap)
	} else {
		data, e := consensusActionData(epoch, kind)
		if e != nil {
			return nil, e
		}
		call = s.call(app, epoch.Index, kind, app.IConsensusAddress, data, new(big.Int), 0)
	}
	if err != nil {
		return nil, err
	}
	if call.Key.Kind == 0 {
		return nil, nil
	}
	return &call, nil
}

// Fresh work requires the root's own committed observation, independently of siblings.
func (s *delivery) requirePublishedRoot(
	ctx context.Context, app *model.Application, epoch *model.Epoch, facts txsender.BlockRef,
) error {
	root, err := s.repository.GetTournament(ctx, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex())
	if errors.Is(err, repository.ErrNotFound) {
		return txsender.ErrInconclusive
	}
	if err != nil {
		return err
	}
	if root == nil || root.ApplicationID != app.ID || root.EpochIndex != epoch.Index ||
		root.Address != *epoch.TournamentAddress || root.Level != uint64(RootLevel) || root.ParentTournamentAddress != nil ||
		root.Snapshot.AsOfBlock == 0 || root.Snapshot.AsOfBlock < root.StartInstant || root.Snapshot.AsOfBlock < epoch.LastBlock ||
		root.Snapshot.AsOfBlock > facts.Number {
		return txsender.ErrInconclusive
	}
	return nil
}

func (s *delivery) call(
	app *model.Application, epoch uint64, kind actionKind, to common.Address, data []byte, value *big.Int, deadline uint64,
) plannedAction {
	// Include every content field, including target/value. model.Epoch is also explicit.
	valueBytes := common.LeftPadBytes(value.Bytes(), common.HashLength)
	key := actionID{ApplicationID: app.ID, Application: app.IApplicationAddress, Target: to, Kind: kind, Epoch: epoch,
		Identity: crypto.Keccak256Hash(to[:], valueBytes, data), Deadline: deadline}
	request := txsender.Request{To: to, Data: data, Value: value,
		Logger: s.Logger.With("application", app.Name, "address", app.IApplicationAddress,
			"action", actionName(kind), "epoch", epoch)}
	if kind == recoverBond {
		request.GasLimit = recoveryGasLimit
	}
	return plannedAction{Key: key, Request: request, Deadline: deadline}
}

func (s *delivery) validateApplicationLinks(ctx context.Context, app *model.Application, snap txsender.BlockRef) error {
	for _, addr := range []common.Address{app.IApplicationAddress, app.IConsensusAddress} {
		code, e := s.deliveryBackend.Code(ctx, addr, snap)
		if e != nil {
			return e
		}
		if len(code) == 0 {
			return fmt.Errorf("%w: registered contract %s has no code", txsender.ErrIneligible, addr)
		}
	}
	abi, err := iapplication.IApplicationMetaData.GetAbi()
	if err != nil {
		return err
	}
	data, err := abi.Pack("isForeclosed")
	if err != nil {
		return err
	}
	vals, err := s.readABI(ctx, snap, app.IApplicationAddress, data, abi.Methods["isForeclosed"].Outputs)
	if err != nil {
		return err
	}
	if len(vals) != 1 {
		return errors.New("invalid foreclosure view")
	}
	foreclosed, ok := vals[0].(bool)
	if !ok || foreclosed {
		return txsender.ErrIneligible
	}
	data, err = abi.Pack("getOutputsMerkleRootValidator")
	if err != nil {
		return err
	}
	vals, err = s.readABI(ctx, snap, app.IApplicationAddress, data, abi.Methods["getOutputsMerkleRootValidator"].Outputs)
	if err != nil {
		return err
	}
	if len(vals) != 1 || vals[0] != app.IConsensusAddress {
		return fmt.Errorf("%w: application's outputs validator differs from registered consensus %s",
			txsender.ErrIneligible, app.IConsensusAddress)
	}
	dave, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	if err != nil {
		return err
	}
	for _, name := range []string{"getApplicationContract", "getTournamentFactory"} {
		data, err = dave.Pack(name)
		if err != nil {
			return err
		}
		vals, err = s.readABI(ctx, snap, app.IConsensusAddress, data, dave.Methods[name].Outputs)
		if err != nil {
			return err
		}
		if len(vals) != 1 {
			return errors.New("invalid Dave relationship view")
		}
		addr, ok := vals[0].(common.Address)
		if !ok || addr == (common.Address{}) {
			return fmt.Errorf("%w: DaveConsensus %s has no valid %s", txsender.ErrIneligible, app.IConsensusAddress, name)
		}
		if name == "getApplicationContract" && addr != app.IApplicationAddress {
			return fmt.Errorf("%w: DaveConsensus application %s differs from registered application %s",
				txsender.ErrIneligible, addr, app.IApplicationAddress)
		}
		if name == "getTournamentFactory" {
			code, e := s.deliveryBackend.Code(ctx, addr, snap)
			if e != nil {
				return e
			}
			if len(code) == 0 {
				return fmt.Errorf("%w: registered tournament factory %s has no code", txsender.ErrIneligible, addr)
			}
		}
	}
	return nil
}

func (s *delivery) joinCall(
	ctx context.Context, app *model.Application, epoch *model.Epoch, snap txsender.BlockRef,
) (plannedAction, error) {
	target := *epoch.TournamentAddress
	code, err := s.deliveryBackend.Code(ctx, target, snap)
	if err != nil {
		return plannedAction{}, err
	}
	if len(code) == 0 {
		return plannedAction{}, &joinRefusal{epoch: epoch.Index, tournament: target, reason: "tournament has no code"}
	}
	if !app.Enabled {
		return plannedAction{}, txsender.ErrIneligible
	}
	adapter, err := s.adapterFactory.CreateTournamentAdapter(target)
	if err != nil {
		return plannedAction{}, err
	}
	opts := pinnedCallOpts(ctx, snap.Number)
	standing, err := adapter.CommitmentStanding(opts, *epoch.Commitment)
	if err != nil {
		return plannedAction{}, err
	}
	if standing.Joined {
		if standing.FinalState != *epoch.MachineHash {
			return plannedAction{}, fmt.Errorf("epoch %d joined commitment has an inconsistent final state", epoch.Index)
		}
		return plannedAction{}, nil
	}
	descriptor, err := adapter.Descriptor(opts)
	if err != nil {
		return plannedAction{}, err
	}
	var deadline uint64
	if descriptor.Allowance <= math.MaxUint64-descriptor.StartInstant {
		deadline = descriptor.StartInstant + descriptor.Allowance
	}
	if descriptor.Level != 0 || descriptor.Height != model.Log2EpochComputationHashLeafCount {
		return plannedAction{}, &joinRefusal{epoch: epoch.Index, tournament: target, deadline: deadline,
			reason: fmt.Sprintf("unsupported root geometry: level=%d height=%d; expected level=0 height=%d",
				descriptor.Level, descriptor.Height, model.Log2EpochComputationHashLeafCount)}
	}
	if descriptor.StartInstant > snap.Number || descriptor.Allowance > math.MaxUint64-descriptor.StartInstant {
		return plannedAction{}, &joinRefusal{epoch: epoch.Index, tournament: target,
			reason: fmt.Sprintf("invalid tournament clock: start=%d allowance=%d", descriptor.StartInstant, descriptor.Allowance)}
	}
	if snap.Number == math.MaxUint64 || snap.Number+1 >= deadline {
		return plannedAction{}, &joinRefusal{epoch: epoch.Index, tournament: target, deadline: deadline,
			reason: "join deadline leaves no block in which this transaction can execute"}
	}
	state, err := adapter.Standing(opts)
	if err != nil {
		return plannedAction{}, err
	}
	if !state.AcceptsJoins {
		return plannedAction{}, &joinRefusal{epoch: epoch.Index, tournament: target, deadline: deadline,
			reason: "tournament no longer accepts joins"}
	}
	bond, err := adapter.BondValue(opts)
	if err != nil {
		return plannedAction{}, err
	}
	if bond == nil || bond.Sign() < 0 {
		return plannedAction{}, fmt.Errorf("tournament join bond is not a non-negative amount")
	}
	data, err := joinData(epoch)
	if err != nil {
		return plannedAction{}, err
	}
	return s.call(app, epoch.Index, joinTournament, target, data, bond, deadline), nil
}

func joinData(epoch *model.Epoch) ([]byte, error) {
	idx := uint64(1<<model.Log2EpochComputationHashLeafCount) - 1
	left, right, err := merkle.RootChildrenFromProof(*epoch.MachineHash, epoch.CommitmentProof, idx)
	if err != nil {
		return nil, err
	}
	contract, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return contract.Pack("joinTournament", *epoch.MachineHash, hashSliceToByteSlice(epoch.CommitmentProof), left, right)
}

func consensusActionData(epoch *model.Epoch, kind actionKind) ([]byte, error) {
	contract, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	switch kind {
	case stageTournament:
		proof, err := epoch.StateProof()
		if err != nil {
			return nil, err
		}
		return contract.Pack("stageTournamentResult", new(big.Int).SetUint64(epoch.Index), daveMachineValidityProof(proof))
	case acceptTournament:
		return contract.Pack("acceptStagedTournamentResult", new(big.Int).SetUint64(epoch.Index))
	case noAction:
		return nil, errors.New("neutral resolution has no consensus calldata")
	case joinTournament, recoverBond:
		return nil, fmt.Errorf("unsupported consensus action %d", kind)
	default:
		return nil, fmt.Errorf("unsupported consensus action %d", kind)
	}
}

func (s *delivery) planRecovery(
	ctx context.Context, app *model.Application, snap txsender.BlockRef, facts txsender.BlockRef,
) (*plannedAction, error) {
	if !app.Enabled {
		return nil, nil
	}
	r := s.repository
	observed := facts.Number
	if observed == 0 {
		return nil, nil
	}
	scan := s.rootBondRecoveryScans[app.ID]
	if scan == nil {
		last, e := s.repository.GetLastNonOpenEpochIndex(ctx, app.IApplicationAddress.Hex())
		if e != nil {
			return nil, e
		}
		if last == nil {
			return nil, nil
		}
		scan = &rootBondRecoveryScan{ThroughEpoch: *last}
		s.rootBondRecoveryScans[app.ID] = scan
	}
	candidates, err := r.ListRecoverableRootBonds(
		ctx, app.ID, s.txOptsFactory.From(), observed, scan.AfterEpoch, scan.ThroughEpoch, rootBondRecoveryPageSize,
	)
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		scan.AfterEpoch = new(candidate.EpochIndex)
		call, e := s.recoveryCall(ctx, app, candidate.EpochIndex, candidate.Tournament, snap, facts)
		if e != nil {
			if txsender.PassInvalid(e) || ctx.Err() != nil {
				return nil, errors.Join(e, ctx.Err())
			}
			s.Logger.Warn("PRT bond recovery candidate deferred", "application", app.Name,
				"address", app.IApplicationAddress, "tournament", candidate.Tournament, "error", e)
			continue
		}
		if call != nil {
			return call, nil
		}
	}
	if len(candidates) < rootBondRecoveryPageSize || scan.AfterEpoch != nil && *scan.AfterEpoch >= scan.ThroughEpoch {
		delete(s.rootBondRecoveryScans, app.ID)
	}
	return nil, nil
}

func (s *delivery) recoveryCall(
	ctx context.Context, app *model.Application, epoch uint64, target common.Address,
	snap txsender.BlockRef, _ txsender.BlockRef,
) (*plannedAction, error) {
	if !app.Enabled {
		return nil, txsender.ErrIneligible
	}
	adapter, err := s.adapterFactory.CreateTournamentAdapter(target)
	if err != nil {
		return nil, err
	}
	recovery, err := adapter.BondRecovery(pinnedCallOpts(ctx, snap.Number))
	if err != nil {
		return nil, err
	}
	if recovery.Disposition != model.BondDispositionRecoverable {
		return nil, nil
	}
	if recovery.Claimer != s.txOptsFactory.From() {
		return nil, nil
	}
	code, err := s.deliveryBackend.Code(ctx, recovery.Claimer, snap)
	if err != nil {
		return nil, err
	}
	if len(code) != 0 {
		return nil, errors.New("configured bond recipient has unsupported code")
	}
	abi, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	data, err := abi.Pack("tryRecoveringBond")
	if err != nil {
		return nil, err
	}
	call := s.call(app, epoch, recoverBond, target, data, new(big.Int), 0)
	call.CheckReturn = func(raw []byte) bool {
		v, e := abi.Unpack("tryRecoveringBond", raw)
		if e != nil || len(v) != 1 {
			return false
		}
		useful, ok := v[0].(bool)
		return ok && useful
	}
	return &call, nil
}

func (s *delivery) readABI(
	ctx context.Context, snap txsender.BlockRef, to common.Address, data []byte, outputs abi.Arguments,
) ([]any, error) {
	msg := ethereum.CallMsg{From: s.txOptsFactory.From(), To: &to, Data: data, Gas: maximumJoinGas}
	raw, err := s.deliveryBackend.Call(ctx, msg, snap)
	if err != nil {
		return nil, err
	}
	return outputs.Unpack(raw)
}
