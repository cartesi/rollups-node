// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package validator provides components to create epoch claims and update
// rollups outputs with their proofs.
package validator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prtprotection"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
)

type Service struct {
	service.TickServiceTemplate

	repository ValidatorRepository

	// cached constants
	pristineRootHash    common.Hash
	pristinePostContext []common.Hash
}

type CreateInfo struct {
	Config     config.ValidatorConfig
	Logger     *slog.Logger
	Repository repository.Repository
}

func Create(ctx context.Context, c *CreateInfo) (service.SupervisedService, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}
	tickCfg := &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{
			Name:     config.ServiceValidator,
			Logger:   c.Logger,
			LogLevel: c.Config.LogLevel,
			LogColor: c.Config.LogColor,
		},
		PollInterval: c.Config.ValidatorPollingInterval,
	}
	err = service.InitTickServiceTemplate(&s.TickServiceTemplate, tickCfg, s)
	if err != nil {
		return nil, err
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on validator service Create is nil")
	}

	s.pristinePostContext = merkle.CreatePostContext()
	s.pristineRootHash = s.pristinePostContext[merkle.TREE_DEPTH]

	s.Logger.Info("Created", "config", c.Config)

	return s, nil
}

// Tick executes the Validator main logic of producing claims and/or proofs
// for processed epochs of all applications with a valid epoch prefix.
func (s *Service) Tick(ctx context.Context) (reschedule bool, err error) {
	defer func() {
		if errors.Is(ctx.Err(), context.Canceled) && errutil.IsOnlyCancellation(err) {
			s.Logger.Debug("Tick interrupted by shutdown", "error", err)
			err = nil
		}
	}()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	apps, _, err := getAllRunningApplications(ctx, s.repository)
	if err != nil {
		return false, fmt.Errorf("failed to get running applications. %w", err)
	}

	// validate each application
	errs := []error{}
	for idx := range apps {
		if err := ctx.Err(); err != nil {
			return false, errors.Join(append(errs, err)...)
		}
		if err := s.validateApplication(ctx, apps[idx]); err != nil {
			errs = append(errs, err)
		}
	}
	return false, errors.Join(errs...)
}

type ValidatorRepository interface {
	prtprotection.Repository
	ListApplications(
		ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool,
	) ([]*Application, uint64, error)
	UpdateApplicationStatus(ctx context.Context, appID int64, status ApplicationStatus, reason *string) error
	ListOutputs(
		ctx context.Context, nameOrAddress string, f repository.OutputFilter, p repository.Pagination, descending bool,
	) ([]*Output, uint64, error)
	GetOutput(ctx context.Context, nameOrAddress string, outputIndex uint64) (*Output, error)
	ListEpochs(
		ctx context.Context, nameOrAddress string, f repository.EpochFilter, p repository.Pagination, descending bool,
	) ([]*Epoch, uint64, error)
	GetLastInput(ctx context.Context, appAddress string, epochIndex uint64) (*Input, error) // FIXME migrate to list
	GetEpochByVirtualIndex(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	GetEpochCompletionState(ctx context.Context, nameOrAddress string, index uint64) (*repository.EpochCompletionState, error)
	StoreClaimAndProofs(ctx context.Context, epoch *Epoch, outputs []*Output) error
	StoreInvalidOutputsRoot(ctx context.Context, appID int64, epochIndex uint64, reason string) error
	CountEpochStateHashes(ctx context.Context, appID int64, epochIndex uint64) (uint64, error)
	ListEpochStateHashes(ctx context.Context, appID int64, epochIndex uint64, afterIndex *uint64, limit uint64) ([]EpochStateHash, error)
}

func getAllRunningApplications(ctx context.Context, er ValidatorRepository) ([]*Application, uint64, error) {
	return er.ListApplications(ctx, validationApplicationsFilter(), repository.Pagination{}, false)
}

func validationApplicationsFilter() repository.ApplicationFilter {
	return repository.ApplicationFilter{
		Enabled: new(true),
		Statuses: []ApplicationStatus{
			ApplicationStatus_OK,
			ApplicationStatus_MachineHalted,
			ApplicationStatus_GuestException,
			ApplicationStatus_McycleOverflow,
			ApplicationStatus_UnexpectedYield,
			ApplicationStatus_InvalidOutputsRoot,
		},
	}
}

func getProcessedEpochs(ctx context.Context, er ValidatorRepository, address string) ([]*Epoch, uint64, error) {
	f := repository.EpochFilter{Status: []EpochStatus{EpochStatus_InputsProcessed}}
	return er.ListEpochs(ctx, address, f, repository.Pagination{}, false)
}

func (s *Service) setApplicationCorrupted(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	reason := fmt.Sprintf(reasonFmt, args...)
	err := appstatus.SetCorrupted(ctx, s.Logger, s.repository, app, reason)
	prtprotection.ReportStop(ctx, s.Logger, s.repository, app, 0, errors.New(reason))
	return err
}

// validateApplication calculates, validates and stores the claim and/or proofs
// for each processed epoch of the application.
// Epochs are iterated in ascending virtual-index order as returned by getProcessedEpochs.
func (s *Service) validateApplication(ctx context.Context, app *Application) error {
	s.Logger.Debug("Starting validation", "application", app.Name)
	appAddress := app.IApplicationAddress.String()
	processedEpochs, _, err := getProcessedEpochs(ctx, s.repository, appAddress)
	if err != nil {
		return fmt.Errorf(
			"failed to get processed epochs of application %v. %w",
			app.IApplicationAddress, err,
		)
	}

	for _, epoch := range processedEpochs {
		if err := ctx.Err(); err != nil {
			return err
		}
		// The application list and epoch list are discovery only. Reload the
		// application, epoch and stop boundary together before computing work.
		state, err := s.repository.GetEpochCompletionState(ctx, appAddress, epoch.Index)
		if err != nil {
			return fmt.Errorf("failed to read validation state for application %s epoch %d: %w", appAddress, epoch.Index, err)
		}
		if state == nil || state.Application == nil || state.Application.ID != app.ID || !state.Application.Enabled {
			return nil
		}
		allowed, err := state.AllowsEpochCompletion()
		if err != nil {
			return fmt.Errorf("inconclusive validation state for application %s epoch %d: %w", appAddress, epoch.Index, err)
		}
		if !allowed {
			return nil
		}
		app, epoch = state.Application, state.Epoch
		if epoch.Status != EpochStatus_InputsProcessed {
			continue
		}
		if app.ForecloseBlock != 0 && epoch.LastBlock >= app.ForecloseBlock {
			s.Logger.Info("Skipping foreclosed epoch that cannot be accepted",
				"application", appAddress,
				"epoch_index", epoch.Index,
				"last_block", epoch.LastBlock,
				"foreclose_block", app.ForecloseBlock,
			)
			continue
		}
		s.Logger.Debug("Started calculating outputs merkle root",
			"application", appAddress,
			"epoch_index", epoch.Index,
			"last_block", epoch.LastBlock,
		)

		if epoch.MachineHash == nil {
			return s.setApplicationCorrupted(ctx, app,
				"epoch %v (%v) has no machine hash", epoch.Index, epoch.VirtualIndex)
		}
		if epoch.TxBufferDataBlock == nil {
			return s.setApplicationCorrupted(ctx, app,
				"epoch %v (%v) has no outputs merkle root", epoch.Index, epoch.VirtualIndex)
		}

		merkleRoot, outputs, err := s.computeMerkleTreeAndProofs(ctx, app, epoch)
		if err != nil {
			// Tick owns operational error reporting. The returned error already
			// identifies the failed operation, application, and epoch.
			return err
		}

		s.Logger.Info("OutputsMerkleRoot Computed",
			"application", appAddress,
			"epoch_index", epoch.Index,
			"outputs_merkle_root", *merkleRoot,
		)

		input, err := s.repository.GetLastInput(ctx, appAddress, epoch.Index)
		if err != nil {
			return fmt.Errorf(
				"failed to get the last Input for epoch %v of application %v. %w",
				epoch.Index, appAddress, err,
			)
		}

		if input != nil {
			if input.TxBufferDataBlock == nil {
				return s.setApplicationCorrupted(ctx, app,
					"inconsistent state: epoch %v last input (%v) outputs merkle root is not defined",
					epoch.Index, input.Index)
			}

			// These are two stored views of the same post-input state. Check
			// their identity before classifying the outputs-root rule below.
			if *epoch.TxBufferDataBlock != *input.TxBufferDataBlock {
				return s.setApplicationCorrupted(ctx, app,
					"epoch %v outputs merkle root does not match last input %v merkle root. Expected: %v, Got %v",
					epoch.Index, input.Index, *input.TxBufferDataBlock, *epoch.TxBufferDataBlock)
			}

			if input.MachineHash == nil {
				return s.setApplicationCorrupted(ctx, app,
					"inconsistent state: epoch %v last input (%v) machine hash is not defined",
					epoch.Index, input.Index)
			}
			if *epoch.MachineHash != *input.MachineHash {
				return s.setApplicationCorrupted(ctx, app,
					"epoch %v machine hash does not match epoch last input (%v) machine hash. Expected: %v, Got %v",
					epoch.Index, input.Index, *input.MachineHash, *epoch.MachineHash)
			}
		} else { // empty epochs
			// DaveConsensus can have empty epochs. Authority and Quorum can't.
			if !app.IsDaveConsensus() {
				return s.setApplicationCorrupted(ctx, app,
					"epoch %v (%v) has no inputs while not DaveConsensus", epoch.Index, epoch.VirtualIndex)
			}

			if epoch.VirtualIndex > 0 {
				previousEpoch, err := s.repository.GetEpochByVirtualIndex(ctx, appAddress, epoch.VirtualIndex-1)
				if err != nil {
					return fmt.Errorf(
						"failed to get previous epoch for epoch %v (%v) of application %v. %w",
						epoch.Index, epoch.VirtualIndex, appAddress, err,
					)
				}

				// nil is a valid result from GetEpochByVirtualIndex.
				// However it indicates some kind of corruption when combined with the epoch.VirtualIndex > 0 check.
				if previousEpoch == nil {
					return s.setApplicationCorrupted(ctx, app,
						"epoch %v (%v) has no previous epoch", epoch.Index, epoch.VirtualIndex)
				}
				if previousEpoch.MachineHash == nil {
					return s.setApplicationCorrupted(ctx, app,
						"previous epoch %v (%v) machine hash is not defined", previousEpoch.Index, previousEpoch.VirtualIndex)
				}
				if *epoch.MachineHash != *previousEpoch.MachineHash {
					return s.setApplicationCorrupted(ctx, app,
						"epoch %v machine hash does not match previous epoch %v machine hash. Expected: %v, Got %v",
						epoch.Index, previousEpoch.Index, *previousEpoch.MachineHash, *epoch.MachineHash)
				}
				if previousEpoch.TxBufferDataBlock == nil {
					return s.setApplicationCorrupted(ctx, app,
						"previous epoch %v (%v) outputs merkle root is not defined", previousEpoch.Index, previousEpoch.VirtualIndex)
				}
				if *epoch.TxBufferDataBlock != *previousEpoch.TxBufferDataBlock {
					return s.setApplicationCorrupted(ctx, app,
						"epoch %v outputs merkle root does not match previous epoch %v one. Expected: %v, Got %v",
						epoch.Index, previousEpoch.Index, *previousEpoch.TxBufferDataBlock, *epoch.TxBufferDataBlock)
				}
			} else { // first epoch
				if *epoch.MachineHash != app.TemplateHash {
					return s.setApplicationCorrupted(ctx, app,
						"epoch %v machine hash does not match for application template hash. Expected: %v, Got %v",
						epoch.Index, app.TemplateHash, *epoch.MachineHash)
				}
			}
		}

		// The guest supplies the cumulative outputs root. Compare its proved
		// word with the root calculated from stored outputs only after checking
		// the stored state identities. This also checks the pristine root of an
		// empty first epoch; a mismatch alone does not establish its cause.
		if *epoch.TxBufferDataBlock != *merkleRoot {
			reason := appstatus.NormalizeReason(fmt.Sprintf(
				"epoch %v: declared outputs root does not match the root calculated from stored outputs; declared=%s; calculated=%s",
				epoch.Index, epoch.TxBufferDataBlock.Hex(), merkleRoot.Hex()))
			if err := s.repository.StoreInvalidOutputsRoot(ctx, app.ID, epoch.Index, reason); err != nil {
				return fmt.Errorf("recording invalid outputs root for application %s epoch %d: %w", appAddress, epoch.Index, err)
			}
			prtprotection.ReportStop(ctx, s.Logger, s.repository, app, epoch.Index, errors.New(reason))
			s.Logger.Error("Invalid outputs root recorded", "application", app.Name, "address", appAddress,
				"epoch_index", epoch.Index, "reason", reason)
			return errors.New(reason)
		}

		if app.IsDaveConsensus() {
			commitment, proof, err := s.buildCommitment(ctx, app, epoch)
			if err != nil {
				return fmt.Errorf("failed to compute commitment for epoch %v (%v) of application %v. %w",
					epoch.Index, epoch.VirtualIndex, appAddress, err,
				)
			}
			epoch.Commitment = commitment
			epoch.CommitmentProof = proof.Siblings
		}

		epoch.Status = EpochStatus_ClaimComputed

		// store the epoch and proofs in the database
		err = s.repository.StoreClaimAndProofs(ctx, epoch, outputs)
		if err != nil {
			if errors.Is(err, repository.ErrEpochForeclosed) {
				s.Logger.Info("Epoch was foreclosed before claim publication; discarding obsolete claim and proofs",
					"application", appAddress, "epoch_index", epoch.Index)
				continue
			}
			return fmt.Errorf(
				"failed to store claim and proofs for epoch %v of application %v. %w",
				epoch.Index, appAddress, err,
			)
		}
	}

	if len(processedEpochs) == 0 {
		s.Logger.Debug("no processed epochs to validate",
			"app", app.IApplicationAddress,
		)
	}

	return nil
}

func (s *Service) buildCommitment(ctx context.Context, app *Application, epoch *Epoch) (*common.Hash, *merkle.Proof, error) {
	if app == nil || epoch == nil {
		return nil, nil, fmt.Errorf("application or epoch is nil")
	}
	if !app.IsDaveConsensus() {
		return nil, nil, nil
	}
	started := time.Now()
	s.Logger.Debug("DaveConsensus: Building commitment for epoch",
		"application", app.Name,
		"epoch", epoch.Index)

	if epoch.InputIndexLowerBound > epoch.InputIndexUpperBound {
		return nil, nil, s.setApplicationCorrupted(ctx, app,
			"invalid epoch %v (%v): lower bound (%v) > upper bound (%v)",
			epoch.Index, epoch.VirtualIndex, epoch.InputIndexLowerBound, epoch.InputIndexUpperBound)
	}
	if epoch.MachineHash == nil {
		return nil, nil, s.setApplicationCorrupted(ctx, app,
			"epoch %v (%v) machine hash is not defined", epoch.Index, epoch.VirtualIndex)
	}
	inputCount := epoch.InputIndexUpperBound - epoch.InputIndexLowerBound
	if inputCount > MaxAdvanceStatesPerEpoch {
		return nil, nil, s.setApplicationCorrupted(ctx, app,
			"input count is too large for epoch %v of application %v: max %v, got %v",
			epoch.Index, app.Name, MaxAdvanceStatesPerEpoch, inputCount)
	}

	builder := newEntryBuilder(*epoch.MachineHash, uint32(Log2StateHashesPerEntry))
	var rowsRead uint64
	if inputCount > 0 {
		total, err := s.repository.CountEpochStateHashes(ctx, app.ID, epoch.Index)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to count state hashes for epoch %d of application %s: %w", epoch.Index, app.Name, err)
		}
		if total < inputCount {
			return nil, nil, s.setApplicationCorrupted(ctx, app,
				"not enough state hashes for epoch %d of application %s: expected at least %d, got %d",
				epoch.Index, app.Name, inputCount, total)
		}
		rowsRead, err = s.appendEpochStateHashes(ctx, app, epoch, builder, total)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list state hashes for epoch %d of application %s: %w", epoch.Index, app.Name, err)
		}
		if rowsRead != total {
			if err := s.recheckStateHashRead(ctx, app, epoch, total); err != nil {
				return nil, nil, err
			}
			return nil, nil, s.setApplicationCorrupted(ctx, app,
				"inconsistent number of state hashes for epoch %d of application %s: expected %d, got %d",
				epoch.Index, app.Name, total, rowsRead)
		}
	}

	epochCommitmentTree, err := builder.finish(inputCount)
	if err != nil {
		return nil, nil, s.setApplicationCorrupted(ctx, app,
			"failed to build commitment for epoch %d of application %s with error: %v", epoch.Index, app.Name, err)
	}
	commitment, proof, err := s.proveCommitment(ctx, app, epoch, epochCommitmentTree)
	if err != nil {
		return nil, nil, err
	}
	s.Logger.Info("DaveConsensus epoch commitment built",
		"application", app.Name, "epoch", epoch.Index, "commitment", commitment.String(),
		"state_hash_rows", rowsRead, "elapsed", time.Since(started))
	return commitment, proof, nil
}

const stateHashPageSize uint64 = 10000

func (s *Service) appendEpochStateHashes(
	ctx context.Context, app *Application, epoch *Epoch, builder *entryBuilder, total uint64,
) (uint64, error) {
	var read uint64
	var after *uint64
	for {
		if err := ctx.Err(); err != nil {
			return read, err
		}
		page, err := s.repository.ListEpochStateHashes(ctx, app.ID, epoch.Index, after, stateHashPageSize)
		if err != nil {
			return read, err
		}
		if uint64(len(page)) > stateHashPageSize {
			return read, fmt.Errorf("state hash page has %d rows, above limit %d", len(page), stateHashPageSize)
		}
		read += uint64(len(page))
		if read > total {
			return read, nil
		}
		var last uint64
		hasPrevious := after != nil
		if hasPrevious {
			last = *after
		}
		for _, row := range page {
			if hasPrevious && row.Index <= last {
				return read, fmt.Errorf("state hash page is not strictly increasing: index %d follows %d", row.Index, last)
			}
			last = row.Index
			hasPrevious = true
			builder.appendRow(row)
		}
		if uint64(len(page)) < stateHashPageSize {
			return read, nil
		}
		// Copy the cursor value, so no row keeps its page alive.
		cursor := last
		after = &cursor
	}
}

func (s *Service) recheckStateHashRead(ctx context.Context, app *Application, epoch *Epoch, expected uint64) error {
	count, err := s.repository.CountEpochStateHashes(ctx, app.ID, epoch.Index)
	if err != nil {
		return fmt.Errorf("failed to recheck state hash count for epoch %d of application %s: %w", epoch.Index, app.Name, err)
	}
	state, err := s.repository.GetEpochCompletionState(ctx, app.IApplicationAddress.Hex(), epoch.Index)
	if err != nil {
		return fmt.Errorf("failed to recheck epoch %d of application %s after state hash read: %w", epoch.Index, app.Name, err)
	}
	if count != expected {
		return fmt.Errorf("state hash count changed for epoch %d of application %s during read: expected %d, got %d",
			epoch.Index, app.Name, expected, count)
	}
	if state == nil || state.Application == nil || state.Application.ID != app.ID {
		return fmt.Errorf("application %s disappeared or changed identity during state hash read for epoch %d", app.Name, epoch.Index)
	}
	if state.Epoch == nil || state.Epoch.ApplicationID != app.ID || state.Epoch.Index != epoch.Index ||
		state.Epoch.Status != EpochStatus_InputsProcessed {
		return fmt.Errorf("epoch %d of application %s disappeared or changed during state hash read", epoch.Index, app.Name)
	}
	return nil
}

// proveCommitment checks the completed tree and proves its final machine state.
// Invalid stored dimensions or hashes are corruption. A broken internal tree
// invariant is a computation failure, not evidence that persisted data is wrong.
func (s *Service) proveCommitment(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
	epochCommitmentTree *merkle.Tree,
) (*common.Hash, *merkle.Proof, error) {
	// The protocol leaf height combines Log2MaxAdvanceStatesPerEpoch and Log2InputComputationHashLeafCount.
	const expectedHeight = Log2EpochComputationHashLeafCount
	if uint64(epochCommitmentTree.Height) != expectedHeight {
		return nil, nil, s.setApplicationCorrupted(ctx, app,
			"epoch %v commitment tree height %v, expected %v — state hash repetitions are inconsistent",
			epoch.Index, epochCommitmentTree.Height, expectedHeight)
	}

	commitment := epochCommitmentTree.GetRootHash()
	proof, err := epochCommitmentTree.ProveLast()
	if err != nil {
		// ProveLast derives its own valid index. Any traversal failure in this
		// newly built tree is an internal error, even an out-of-range child index.
		failure := fmt.Errorf("failed to retrieve commitment proof for epoch %d of application %s: %w", epoch.Index, app.Name, err)
		if errors.Is(err, merkle.ErrOpaqueTree) {
			failure = fmt.Errorf("%w: the epoch's last leaf lies inside a stored bundle: the last input fills its span, "+
				"which needs a bundle opening that this node does not support, "+
				"or its stored padding row is not the epoch's final entry", failure)
		}
		statusErr := appstatus.SetFailed(ctx, s.Logger, s.repository, app, failure.Error())
		prtprotection.ReportStop(ctx, s.Logger, s.repository, app, 0, failure)
		return nil, nil, errors.Join(failure, statusErr)
	}
	// PRT reconstructs the root children from (epoch.MachineHash, proof).
	// The tree's last leaf must therefore be the epoch's final machine hash.
	if proof.Node != *epoch.MachineHash {
		return nil, nil, s.setApplicationCorrupted(ctx, app,
			"epoch %v commitment last leaf %v does not match machine hash %v",
			epoch.Index, proof.Node, *epoch.MachineHash)
	}
	return &commitment, proof, nil
}

// computeMerkleTreeAndProofs calculates the claim and proofs for an epoch. It returns
// the claim and the epoch outputs updated with their hash and proofs. In case
// the epoch has no outputs, there are no proofs and it returns the pristine
// claim for the first epoch or the previous epoch claim otherwise.
func (s *Service) computeMerkleTreeAndProofs(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
) (*common.Hash, []*Output, error) {
	appAddress := app.IApplicationAddress.String()
	// Scope the epoch's outputs by epoch index, not by block range. DaveConsensus
	// epochs overlap on one block (a sealed epoch's last block equals the next
	// epoch's first block), so a block-range lookup is ambiguous when inputs
	// straddle that boundary block: it would pull in the neighbouring epoch's
	// outputs. The epoch-index filter resolves each output through its input's
	// epoch, which is unambiguous.
	epochOutputs, _, err := s.repository.ListOutputs(ctx, appAddress, repository.OutputFilter{
		EpochIndex: &epoch.Index,
	},
		repository.Pagination{},
		false,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"failed to get outputs for epoch %v (%v) of application %v. %w",
			epoch.Index, epoch.VirtualIndex, appAddress, err,
		)
	}

	var previousEpoch *Epoch
	if epoch.VirtualIndex > 0 {
		previousEpoch, err = s.repository.GetEpochByVirtualIndex(ctx, appAddress, epoch.VirtualIndex-1)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"failed to get previous epoch for epoch %v (%v) of application %v. %w",
				epoch.Index, epoch.VirtualIndex, appAddress, err,
			)
		}
	}

	// if there are no outputs
	if len(epochOutputs) == 0 {
		// and there should be a previous epoch (guard against DB corruption)
		if epoch.VirtualIndex != 0 && previousEpoch == nil {
			return nil, nil, s.setApplicationCorrupted(ctx, app,
				"epoch %v (%v): previous epoch with virtual index %v is missing",
				epoch.Index, epoch.VirtualIndex, epoch.VirtualIndex-1)
		}

		// and there is no previous epoch
		if epoch.VirtualIndex == 0 {
			// this is the first epoch, return the pristine claim
			return &s.pristineRootHash, nil, nil
		}
		// if there are no outputs and there is a previous epoch, return its claim
		if previousEpoch.TxBufferDataBlock == nil {
			return nil, nil, s.setApplicationCorrupted(ctx, app,
				"invalid application state for epoch %v (%v) of application %v. Previous epoch has no claim.",
				epoch.Index, epoch.VirtualIndex, appAddress)
		}
		return previousEpoch.TxBufferDataBlock, nil, nil
	}

	// Build the pre-context for the cumulative outputs tree from the output that
	// immediately precedes this epoch's first output, identified by output index
	// rather than by block. Index-based lookup is required for the same reason as
	// the epoch-outputs query above: at the DaveConsensus epoch-boundary block a
	// block-based "last output before this epoch" is ambiguous.
	var pre []common.Hash
	var index uint64
	firstOutputIndex := epochOutputs[0].Index
	if firstOutputIndex == 0 {
		// this epoch holds the very first output: start from a pristine context
		pre = s.pristinePostContext
		index = 0
	} else {
		previousOutput, err := s.repository.GetOutput(ctx, appAddress, firstOutputIndex-1)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"failed to get previous output for epoch %v (%v) of application %v. %w",
				epoch.Index, epoch.VirtualIndex, appAddress, err,
			)
		}
		if previousOutput == nil || previousOutput.Hash == nil ||
			len(previousOutput.OutputHashesSiblings) != merkle.TREE_DEPTH {
			return nil, nil, s.setApplicationCorrupted(ctx, app,
				"Inconsistent application state (%v). Output (%d) preceding epoch %d is missing or has invalid hash siblings.",
				app.Name, firstOutputIndex-1, epoch.Index)
		}
		pre = merkle.CreatePreContextFromProof(previousOutput.Index, *previousOutput.Hash, previousOutput.OutputHashesSiblings)
		index = firstOutputIndex
	}

	// we have outputs to compute, gather the values to call ComputeSiblingsMatrix
	outputHashes := make([]common.Hash, 0, len(epochOutputs))
	for _, output := range epochOutputs {
		hash := crypto.Keccak256Hash(output.RawData)
		// update outputs with their hash
		output.Hash = &hash
		// add them to the leaves slice
		outputHashes = append(outputHashes, hash)
	}

	// compute and store siblings
	siblings, err := merkle.ComputeSiblingsMatrix(pre, outputHashes, s.pristinePostContext, index)
	if err != nil {
		return nil, nil, err
	}
	// update outputs with their siblings
	for idx, output := range epochOutputs {
		start := merkle.TREE_DEPTH * idx
		end := merkle.TREE_DEPTH * (idx + 1)
		output.OutputHashesSiblings = siblings[start:end]
	}
	rootHash := merkle.ComputeRootHashFromProof(epochOutputs[0].Index, *epochOutputs[0].Hash, epochOutputs[0].OutputHashesSiblings)
	return &rootHash, epochOutputs, nil
}
