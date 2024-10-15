// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package validator provides components to create epoch claims and update
// rollups outputs with their proofs.
package validator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/crypto"
)

// The maximum height for the Merkle tree of all outputs produced
// by an application
const MAX_OUTPUT_TREE_HEIGHT = 63

type ValidatorRepository interface {
	// GetAllRunningApplications returns a slice with the applications currently
	// being validated by the node.
	GetAllRunningApplications(ctx context.Context) ([]Application, error)
	// GetOutputsProducedInBlockRange returns outputs produced by inputs sent to
	// the application in the provided block range, inclusive. Outputs are
	// returned in ascending order by index.
	GetOutputsProducedInBlockRange(
		ctx context.Context,
		application Address,
		firstBlock, lastBlock uint64,
	) ([]Output, error)
	// GetProcessedEpochs returns epochs from the application which had all
	// its inputs processed. Epochs are returned in ascending order by index.
	GetProcessedEpochs(ctx context.Context, application Address) ([]Epoch, error)
	// GetLastInputOutputsHash returns the outputs Merkle tree hash calculated
	// by the Cartesi Machine after it processed the last input in the epoch.
	GetLastInputOutputsHash(
		ctx context.Context,
		epochIndex uint64,
		appAddress Address,
	) (*Hash, error)
	// GetPreviousEpoch returns the epoch that ended one block before the start
	// of the current epoch
	GetPreviousEpoch(ctx context.Context, currentEpoch Epoch) (*Epoch, error)
	// ValidateEpochTransaction performs a database transaction
	// containing two operations:
	//
	// 1. Updates an epoch, adding its claim and modifying its status.
	//
	// 2. Updates outputs with their Keccak256 hash and proof.
	SetEpochClaimAndInsertProofsTransaction(
		ctx context.Context,
		epoch Epoch,
		outputs []Output,
	) error
}

// Validator creates epoch claims and rollups outputs proofs for all running
// applications.
type Validator struct {
	repository              ValidatorRepository
	inputBoxDeploymentBlock uint64
}

func NewValidator(
	repo ValidatorRepository,
	inputBoxDeploymentBlock uint64,
) *Validator {
	return &Validator{repo, inputBoxDeploymentBlock}
}

func (v *Validator) String() string {
	return "validator"
}

// Run executes the Validator main logic of producing claims and/or proofs
// for processed epochs of all running applications. It is meant to be executed
// inside a loop. If an error occurs while processing any epoch, it halts and
// returns the error.
func (v *Validator) Run(ctx context.Context) error {
	apps, err := v.repository.GetAllRunningApplications(ctx)
	if err != nil {
		return fmt.Errorf("failed to get running applications. %w", err)
	}

	for idx := range apps {
		if err := v.validateApplication(ctx, apps[idx]); err != nil {
			return err
		}
	}
	return nil
}

// validateApplication calculates, validates and stores the claim and/or proofs
// for each processed epoch of the application.
func (v *Validator) validateApplication(ctx context.Context, app Application) error {
	slog.Info("starting validation", "application", app.ContractAddress, "service", v)
	processedEpochs, err := v.repository.GetProcessedEpochs(ctx, app.ContractAddress)
	if err != nil {
		return fmt.Errorf(
			"failed to get processed epochs of application %v. %w",
			app.ContractAddress, err,
		)
	}

	for _, epoch := range processedEpochs {
		slog.Info("started calculating claim",
			"epoch index", epoch.Index,
			"application", app.ContractAddress,
			"service", v,
		)
		claim, outputs, err := v.createClaimAndProofs(ctx, epoch)
		slog.Info("finished calculating claim",
			"epoch index", epoch.Index,
			"application", app.ContractAddress,
			"service", v,
		)
		if err != nil {
			return err
		}

		// The Cartesi Machine calculates the root hash of the outputs Merkle
		// tree after each input. Therefore, the root hash calculated after the
		// last input in the epoch must match the claim hash calculated by the
		// Validator. We first retrieve the hash calculated by the
		// Cartesi Machine...
		machineClaim, err := v.repository.GetLastInputOutputsHash(
			ctx,
			epoch.Index,
			epoch.AppAddress,
		)
		if err != nil {
			return fmt.Errorf(
				"failed to get the machine claim for epoch %v of application %v. %w",
				epoch.Index, epoch.AppAddress, err,
			)
		}

		if machineClaim == nil {
			return fmt.Errorf(
				"inconsistent state: machine claim for epoch %v of application %v was not found",
				epoch.Index, epoch.AppAddress,
			)
		}

		// ...and compare it to the hash calculated by the Validator
		if *machineClaim != *claim {
			return fmt.Errorf(
				"validator claim does not match machine claim for epoch %v of application %v",
				epoch.Index, epoch.AppAddress,
			)
		}

		// update the epoch status and its claim
		epoch.Status = EpochStatusClaimComputed
		epoch.ClaimHash = claim

		// store the epoch and proofs in the database
		err = v.repository.SetEpochClaimAndInsertProofsTransaction(ctx, epoch, outputs)
		if err != nil {
			return fmt.Errorf(
				"failed to store claim and proofs for epoch %v of application %v. %w",
				epoch.Index, epoch.AppAddress, err,
			)
		}
	}

	if len(processedEpochs) == 0 {
		slog.Info("no processed epochs to validate",
			"application", app.ContractAddress,
			"service", v,
		)
	}

	return nil
}

// createClaimAndProofs calculates the claim and proofs for an epoch. It returns
// the claim and the epoch outputs updated with their hash and proofs. In case
// the epoch has no outputs, there are no proofs and it returns the pristine
// claim for the first epoch or the previous epoch claim otherwise.
func (v *Validator) createClaimAndProofs(
	ctx context.Context,
	epoch Epoch,
) (*Hash, []Output, error) {
	epochOutputs, err := v.repository.GetOutputsProducedInBlockRange(
		ctx,
		epoch.AppAddress,
		epoch.FirstBlock,
		epoch.LastBlock,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"failed to get outputs for epoch %v of application %v. %w",
			epoch.Index, epoch.AppAddress, err,
		)
	}

	previousEpoch, err := v.repository.GetPreviousEpoch(ctx, epoch)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"failed to get previous epoch for epoch %v of application %v. %w",
			epoch.Index, epoch.AppAddress, err,
		)
	}

	// if there are no outputs
	if len(epochOutputs) == 0 {
		// and there is no previous epoch
		if previousEpoch == nil {
			// this is the first epoch, return the pristine claim
			claim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
			if err != nil {
				return nil, nil, fmt.Errorf(
					"failed to create proofs for epoch %v of application %v. %w",
					epoch.Index, epoch.AppAddress, err,
				)
			}
			return &claim, nil, nil
		}
	} else {
		// if epoch has outputs, calculate a new claim and proofs
		var previousOutputs []Output
		if previousEpoch != nil {
			// get all outputs created before the current epoch
			previousOutputs, err = v.repository.GetOutputsProducedInBlockRange(
				ctx,
				epoch.AppAddress,
				v.inputBoxDeploymentBlock,
				previousEpoch.LastBlock,
			)
			if err != nil {
				return nil, nil, fmt.Errorf(
					"failed to get all outputs of application %v before epoch %d. %w",
					epoch.AppAddress, epoch.Index, err,
				)
			}
		}
		// the leaves of the Merkle tree are the Keccak256 hashes of all the
		// outputs
		leaves := make([]Hash, 0, len(epochOutputs)+len(previousOutputs))
		for idx := range previousOutputs {
			if previousOutputs[idx].Hash == nil {
				// should never happen
				return nil, nil, fmt.Errorf(
					"missing hash of output %d from input %d",
					previousOutputs[idx].Index, previousOutputs[idx].InputId,
				)
			}
			leaves = append(leaves, *previousOutputs[idx].Hash)
		}
		for idx := range epochOutputs {
			hash := crypto.Keccak256Hash(epochOutputs[idx].RawData[:])
			// update outputs with their hash
			epochOutputs[idx].Hash = &hash
			// add them to the leaves slice
			leaves = append(leaves, hash)
		}

		claim, proofs, err := merkle.CreateProofs(leaves, MAX_OUTPUT_TREE_HEIGHT)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"failed to create proofs for epoch %d of application %v. %w",
				epoch.Index, epoch.AppAddress, err,
			)
		}

		// update outputs with their proof
		for idx := range epochOutputs {
			start := epochOutputs[idx].Index * MAX_OUTPUT_TREE_HEIGHT
			end := (epochOutputs[idx].Index * MAX_OUTPUT_TREE_HEIGHT) + MAX_OUTPUT_TREE_HEIGHT
			epochOutputs[idx].OutputHashesSiblings = proofs[start:end]
		}
		return &claim, epochOutputs, nil
	}
	// if there are no outputs and there is a previous epoch, return its claim
	return previousEpoch.ClaimHash, nil, nil
}

// ValidatorService extends the Validator utility by executing it with a polling
// strategy. It implements the `services.Service` interface.
type ValidatorService struct {
	validator       *Validator
	pollingInterval time.Duration
}

func NewValidatorService(
	repo ValidatorRepository,
	inputBoxDeploymentBlock uint64,
	pollingInterval time.Duration,
) *ValidatorService {
	service := &ValidatorService{pollingInterval: pollingInterval}
	service.validator = NewValidator(repo, inputBoxDeploymentBlock)
	return service
}

func (s *ValidatorService) String() string {
	return "validator"
}

func (s *ValidatorService) Start(ctx context.Context, ready chan<- struct{}) error {
	ready <- struct{}{}

	ticker := time.NewTicker(s.pollingInterval)
	defer ticker.Stop()

	for {
		if err := s.validator.Run(ctx); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
