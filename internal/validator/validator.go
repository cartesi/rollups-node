// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package validator provides components to create epoch claims and update
// rollups outputs with their proofs.
package validator

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	pkgm "github.com/cartesi/rollups-node/pkg/machine"
	"github.com/cartesi/rollups-node/pkg/service"
)

type Service struct {
	service.Service
	repository ValidatorRepository

	// cached constants
	pristineRootHash    common.Hash
	pristinePostContext []common.Hash
}

type CreateInfo struct {
	service.CreateInfo

	Config config.ValidatorConfig

	Repository repository.Repository
}

func Create(ctx context.Context, c *CreateInfo) (*Service, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}
	c.Impl = s

	err = service.Create(ctx, &c.CreateInfo, &s.Service)
	if err != nil {
		return nil, err
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on validator service Create is nil")
	}

	s.pristinePostContext = merkle.CreatePostContext()
	s.pristineRootHash = s.pristinePostContext[merkle.TREE_DEPTH]

	return s, nil
}

func (s *Service) Alive() bool     { return true }
func (s *Service) Ready() bool     { return true }
func (s *Service) Reload() []error { return nil }

// Tick executes the Validator main logic of producing claims and/or proofs
// for processed epochs of all running applications.
func (s *Service) Tick() []error {
	apps, _, err := getAllRunningApplications(s.Context, s.repository)
	if err != nil {
		// During shutdown the parent context is canceled and every in-
		// flight DB query returns context.Canceled. Suppress only the
		// graceful-shutdown case; deadline-exceeded (real failure) still
		// propagates. Mirrors internal/prt/service.go's Tick pattern.
		if s.IsStopping() && errors.Is(err, context.Canceled) {
			s.Logger.Warn("Tick interrupted by shutdown", "error", err)
			return nil
		}
		return []error{fmt.Errorf("failed to get running applications. %w", err)}
	}

	// validate each application
	errs := []error{}
	for idx := range apps {
		if err := s.validateApplication(s.Context, apps[idx]); err != nil {
			// Same shutdown-cancellation suppression as above, per-app.
			if s.IsStopping() && errors.Is(err, context.Canceled) {
				s.Logger.Warn("Tick interrupted by shutdown",
					"application", apps[idx].IApplicationAddress, "error", err)
				continue
			}
			errs = append(errs, err)
		}
	}
	return errs
}
func (s *Service) Stop(_ bool) []error {
	s.SetStopping()
	return nil
}

func (s *Service) String() string {
	return s.Name
}

// The maximum height for the Merkle tree of all outputs produced
// by an application
const MAX_OUTPUT_TREE_HEIGHT = merkle.TREE_DEPTH //nolint: revive

type ValidatorRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool) ([]*Application, uint64, error)
	UpdateApplicationStatus(ctx context.Context, appID int64, status ApplicationStatus, reason *string) error
	ListOutputs(ctx context.Context, nameOrAddress string, f repository.OutputFilter, p repository.Pagination, descending bool) ([]*Output, uint64, error)
	GetOutput(ctx context.Context, nameOrAddress string, outputIndex uint64) (*Output, error)
	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter, p repository.Pagination, descending bool) ([]*Epoch, uint64, error)
	GetLastInput(ctx context.Context, appAddress string, epochIndex uint64) (*Input, error) // FIXME migrate to list
	GetEpochByVirtualIndex(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	StoreClaimAndProofs(ctx context.Context, epoch *Epoch, outputs []*Output) error
	ListStateHashes(ctx context.Context, nameOrAddress string, f repository.StateHashFilter, p repository.Pagination, descending bool) ([]*StateHash, uint64, error)
}

func getAllRunningApplications(ctx context.Context, er ValidatorRepository) ([]*Application, uint64, error) {
	return er.ListApplications(ctx, validationApplicationsFilter(), repository.Pagination{}, false)
}

func validationApplicationsFilter() repository.ApplicationFilter {
	return repository.ApplicationFilter{
		Enabled:  new(true),
		Statuses: []ApplicationStatus{ApplicationStatus_OK},
	}
}

func getProcessedEpochs(ctx context.Context, er ValidatorRepository, address string) ([]*Epoch, uint64, error) {
	f := repository.EpochFilter{Status: []EpochStatus{EpochStatus_InputsProcessed}}
	return er.ListEpochs(ctx, address, f, repository.Pagination{}, false)
}

func (s *Service) setApplicationCorrupted(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	return appstatus.SetCorruptedf(ctx, s.Logger, s.repository, app, reasonFmt, args...)
}

// validateApplication calculates, validates and stores the claim and/or proofs
// for each processed epoch of the application.
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
		merkleRoot, outputs, err := s.computeMerkleTreeAndProofs(ctx, app, epoch)
		if err != nil {
			// Don't log shutdown-cancellation at ERR — every in-flight DB
			// query returns context.Canceled and Tick's outer suppression
			// (s.IsStopping() && errors.Is(err, context.Canceled)) handles
			// the propagation. DeadlineExceeded is a real failure and
			// must still be logged.
			if !(s.IsStopping() && errors.Is(err, context.Canceled)) {
				s.Logger.Error("failed to create claim and proofs.", "error", err)
			}
			return err
		}

		s.Logger.Info("OutputsMerkleRoot Computed",
			"application", appAddress,
			"epoch_index", epoch.Index,
			"outputs_merkle_root", *merkleRoot,
		)

		// The Cartesi Machine calculates the root hash of the outputs Merkle
		// tree after each input. Therefore, the root hash calculated after the
		// last input in the epoch must match the one calculated by the Validator
		// So we need to validate the application state.
		if *epoch.OutputsMerkleRoot != *merkleRoot {
			return s.setApplicationCorrupted(ctx, app,
				"epoch %v outputs merkle root does not match computed one. Expected: %v, Got %v",
				epoch.Index, *epoch.OutputsMerkleRoot, *merkleRoot)
		}

		input, err := s.repository.GetLastInput(ctx, appAddress, epoch.Index)
		if err != nil {
			return fmt.Errorf(
				"failed to get the last Input for epoch %v of application %v. %w",
				epoch.Index, appAddress, err,
			)
		}

		// DaveConsensus can have empty epochs. Authority and Quorum don't.
		if !app.IsDaveConsensus() || input != nil {
			if input.OutputsHash == nil {
				return s.setApplicationCorrupted(ctx, app,
					"inconsistent state: epoch %v last input (%v) outputs merkle root is not defined",
					epoch.Index, input.Index)
			}

			// ...and compare it to the hash calculated by the Validator
			if *epoch.OutputsMerkleRoot != *input.OutputsHash {
				return s.setApplicationCorrupted(ctx, app,
					"computed outputs merkle root does not match epoch %v last input %v merkle root. Expected: %v, Got %v",
					epoch.Index, input.Index, *input.OutputsHash, *epoch.OutputsMerkleRoot)
			}

			if *epoch.MachineHash != *input.MachineHash {
				return s.setApplicationCorrupted(ctx, app,
					"epoch %v machine hash does not match epoch last input (%v) machine hash. Expected: %v, Got %v",
					epoch.Index, input.Index, *input.MachineHash, *epoch.MachineHash)
			}
		} else { // empty epochs
			if epoch.VirtualIndex > 0 {
				previousEpoch, err := s.repository.GetEpochByVirtualIndex(ctx, appAddress, epoch.VirtualIndex-1)
				if err != nil {
					return fmt.Errorf(
						"failed to get previous epoch for epoch %v (%v) of application %v. %w",
						epoch.Index, epoch.VirtualIndex, appAddress, err,
					)
				}
				if *epoch.MachineHash != *previousEpoch.MachineHash {
					return s.setApplicationCorrupted(ctx, app,
						"epoch %v machine hash does not match previous epoch %v machine hash. Expected: %v, Got %v",
						epoch.Index, previousEpoch.Index, *previousEpoch.MachineHash, *epoch.MachineHash)
				}
				if *epoch.OutputsMerkleRoot != *previousEpoch.OutputsMerkleRoot {
					return s.setApplicationCorrupted(ctx, app,
						"epoch %v outputs merkle root does not match previous epoch %v one. Expected: %v, Got %v",
						epoch.Index, previousEpoch.Index, *previousEpoch.OutputsMerkleRoot, *epoch.OutputsMerkleRoot)
				}
			} else { // first epoch
				if *epoch.MachineHash != app.TemplateHash {
					return s.setApplicationCorrupted(ctx, app,
						"epoch %v machine hash does not match for application template hash. Expected: %v, Got %v",
						epoch.Index, app.TemplateHash, *epoch.MachineHash)
				}
				if *epoch.OutputsMerkleRoot != s.pristineRootHash {
					return s.setApplicationCorrupted(ctx, app,
						"epoch %v outputs merkle root does not match pristine root hash. Expected: %v, Got %v",
						epoch.Index, s.pristineRootHash, *epoch.OutputsMerkleRoot)
				}
			}
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
	s.Logger.Debug("DaveConsensus: Building commitment for epoch",
		"application", app.Name,
		"epoch", epoch.Index)

	builder := merkle.Builder{}
	inputCount := epoch.InputIndexUpperBound - epoch.InputIndexLowerBound
	if inputCount > 0 {
		statesHashes, total, err := s.repository.ListStateHashes(ctx, app.IApplicationAddress.String(),
			repository.StateHashFilter{EpochIndex: &epoch.Index}, repository.Pagination{}, false)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list state hashes for epoch %d of application %s: %w",
				epoch.Index, app.Name, err)
		}
		if total < inputCount {
			return nil, nil, fmt.Errorf("not enough state hashes for epoch %d of application %s: expected at least %d, got %d",
				epoch.Index, app.Name, inputCount, total)
		}
		if uint64(len(statesHashes)) != total {
			return nil, nil, fmt.Errorf("inconsistent number of state hashes for epoch %d of application %s: expected %d, got %d", epoch.Index, app.Name, total, len(statesHashes))
		}
		for _, stateHash := range statesHashes {
			builder.AppendRepeatedUint64(merkle.TreeLeaf(stateHash.MachineHash), stateHash.Repetitions)
		}
	}

	remainingInputs := pkgm.InputsPerEpoch - inputCount
	remainingStrides := remainingInputs << pkgm.Log2StridesPerInput
	if remainingStrides > 0 {
		builder.AppendRepeatedUint64(merkle.TreeLeaf(*epoch.MachineHash), remainingStrides)
	}

	epochCommitmentTree := builder.Build()
	commitment := epochCommitmentTree.GetRootHash()
	proof := epochCommitmentTree.ProveLast()
	s.Logger.Info("DaveConsensus epoch commitment built",
		"application", app.Name,
		"epoch", epoch.Index,
		"commitment", commitment.String())
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
		// and there is no previous epoch
		if previousEpoch == nil {
			// this is the first epoch, return the pristine claim
			return &s.pristineRootHash, nil, nil
		}
		// if there are no outputs and there is a previous epoch, return its claim
		if previousEpoch.OutputsMerkleRoot == nil {
			return nil, nil, s.setApplicationCorrupted(ctx, app,
				"invalid application state for epoch %v (%v) of application %v. Previous epoch has no claim.",
				epoch.Index, epoch.VirtualIndex, appAddress)
		}
		return previousEpoch.OutputsMerkleRoot, nil, nil
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
