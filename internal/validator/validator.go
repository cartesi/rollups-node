// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/crypto"
)

// The maximum height for the Merkle tree of all outputs produced
// by an application
const MAX_OUTPUT_TREE_HEIGHT = 63

type Validator struct {
	repository              ValidatorRepository
	inputBoxDeploymentBlock uint64
	pollingInterval         time.Duration
}

type ValidatorRepository interface {
	// GetAllRunningApplications returns a slice with the applications being
	// actively validated by the node.
	GetAllRunningApplications(ctx context.Context) ([]Application, error)
	// GetOutputs returns outputs produced by inputs sent to the application
	// between start and end blocks, inclusive. Outputs are in ascending order
	// by index.
	GetOutputs(
		ctx context.Context,
		application Address,
		startBlock, endBlock uint64,
	) ([]*Output, error)
	// GetProcessedEpochs returns epochs from the application which had all
	// its inputs processed. Epochs are in ascending order by index.
	GetProcessedEpochs(ctx context.Context, application Address) ([]*Epoch, error)
	// GetLastInputOutputHash returns the outputs Merkle tree hash calculated
	// by the Cartesi Machine after it processed the last input in the provided
	// epoch.
	GetLastInputOutputHash(ctx context.Context, epoch *Epoch) (*Hash, error)
	// GetPreviousEpoch returns the epoch that ended one block before the start
	// of the current epoch
	GetPreviousEpoch(ctx context.Context, currentEpoch *Epoch) (*Epoch, error)
	// ValidateEpochTransaction performs a database transaction
	// containing two operations:
	//
	// 1. Updates an epoch, adding its claim and modifying its status.
	//
	// 2. Updates several outputs with their Keccak256 hash and proof.
	SetEpochClaimAndInsertProofsTransaction(
		ctx context.Context,
		epoch *Epoch,
		outputs []*Output,
	) error
}

func (v Validator) String() string {
	return "validator"
}

func (v Validator) Start(ctx context.Context, ready chan<- struct{}) error {
	ready <- struct{}{}

	ticker := time.NewTicker(v.pollingInterval)
	defer ticker.Stop()

	for {
		err := v.Run(ctx)
		if err != nil {
			slog.Error(err.Error(), "service", v)
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func NewValidator(
	repo ValidatorRepository,
	inputBoxDeploymentBlock uint64,
	pollingInterval time.Duration,
) *Validator {
	return &Validator{repo, inputBoxDeploymentBlock, pollingInterval}
}

func (v *Validator) Run(ctx context.Context) error {
	apps, err := v.repository.GetAllRunningApplications(ctx)
	if err != nil {
		return fmt.Errorf("failed to get running applications %w", err)
	}

	for _, app := range apps {
		if err := v.validateApplication(ctx, app); err != nil {
			return err
		}
	}
	return nil
}

// validateApplication creates, validates and stores the claim and proofs for
// each processed epoch of the application.
func (v *Validator) validateApplication(ctx context.Context, app Application) error {
	processedEpochs, err := v.repository.GetProcessedEpochs(ctx, app.ContractAddress)
	if err != nil {
		return fmt.Errorf(
			"failed to get processed epochs for app: %v. %w",
			app.ContractAddress, err,
		)
	}

	for _, epoch := range processedEpochs {
		slog.Info(
			fmt.Sprintf(
				"started calculating claim for epoch %d of app %v",
				epoch.Index, app.ContractAddress,
			),
			"service", v,
		)
		claim, outputs, err := v.createClaimAndProofs(ctx, epoch)
		slog.Info(
			fmt.Sprintf(
				"finished calculating claim for epoch %d of app %v",
				epoch.Index, app.ContractAddress,
			),
			"service", v,
		)
		if err != nil {
			return err
		}

		machineOutputsRootHash, err := v.repository.GetLastInputOutputHash(ctx, epoch)
		if err != nil {
			return fmt.Errorf("failed to get the outputs root hash from the machine. %w", err)
		}

		// the Cartesi Machine calculates the root hash of the outputs Merkle
		// tree after each input. Therefore, the root hash for the last input
		// in the epoch must match the claim calculated by the validator or
		// there is a mismatch between the two implementations
		if *machineOutputsRootHash != *claim {
			return errors.New("outputs Merkle tree root hash mismatch")
		}

		epoch.Claim = claim
		epoch.Status = EpochStatusCalculatedClaim

		if err := v.repository.SetEpochClaimAndInsertProofsTransaction(
			ctx,
			epoch,
			outputs,
		); err != nil {
			return fmt.Errorf("failed to store claim and proofs. %w", err)
		}
	}
	return nil
}

// createClaimAndProofs calculates the claim and proofs for an epoch. It returns
// the claim and the epoch outputs updated with their hash and proofs. In case
// the epoch has no outputs, it returns the pristine claim when it is the first
// one or the previous epoch claim otherwise.
func (v *Validator) createClaimAndProofs(
	ctx context.Context,
	epoch *Epoch,
) (*Hash, []*Output, error) {
	epochOutputs, err := v.repository.GetOutputs(
		ctx,
		epoch.AppAddress,
		epoch.StartBlock,
		epoch.EndBlock,
	)
	if err != nil {
		wrappedErr := fmt.Errorf(
			"failed to get outputs for epoch %d of app %v. %w",
			epoch.Index, epoch.AppAddress, err,
		)
		return nil, nil, wrappedErr
	}

	previousEpoch, err := v.repository.GetPreviousEpoch(ctx, epoch)
	if err != nil {
		wrappedErr := fmt.Errorf(
			"failed to get previous epoch for epoch %d of app %v. %w",
			epoch.Index, epoch.AppAddress, err,
		)
		return nil, nil, wrappedErr
	}

	// if there are no outputs
	if len(epochOutputs) == 0 {
		// and there is no previous epoch
		if previousEpoch == nil {
			// this is the first epoch, return the pristine claim
			claim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create proofs. %w", err)
			}
			return &claim, nil, nil
		}
	} else {
		// if epoch has outputs, calculate a new claim and proofs
		var previousOutputs []*Output
		if previousEpoch != nil {
			// get all outputs created before the current epoch
			previousOutputs, err = v.repository.GetOutputs(
				ctx,
				epoch.AppAddress,
				v.inputBoxDeploymentBlock,
				previousEpoch.EndBlock,
			)
			if err != nil {
				wrappedErr := fmt.Errorf(
					"failed to get all outputs of app %v before epoch %d. %w",
					epoch.AppAddress, epoch.Index, err,
				)
				return nil, nil, wrappedErr
			}
		}
		// the leaves of the Merkle tree are the Keccak256 hashes of all the
		// outputs
		leaves := make([]Hash, 0, len(epochOutputs)+len(previousOutputs))
		for _, output := range previousOutputs {
			if output.Hash == nil {
				// should never happen
				err := fmt.Errorf(
					"missing hash of output %d of epoch %d",
					output.Index, epoch.Index,
				)
				return nil, nil, err
			}
			leaves = append(leaves, *output.Hash)
		}
		for _, output := range epochOutputs {
			hash := crypto.Keccak256Hash(output.RawData[:])
			// update current epoch outputs with their hash
			output.Hash = &hash
			// add them to the leaves slice
			leaves = append(leaves, *output.Hash)
		}

		claim, proofs, err := merkle.CreateProofs(leaves, MAX_OUTPUT_TREE_HEIGHT)
		if err != nil {
			wrappedErr := fmt.Errorf(
				"failed to create proofs for epoch %d of app %v. %w",
				epoch.Index, epoch.AppAddress, err,
			)
			return nil, nil, wrappedErr
		}

		// update current epoch outputs with their proof
		for _, output := range epochOutputs {
			start := output.Index * MAX_OUTPUT_TREE_HEIGHT
			end := (output.Index * MAX_OUTPUT_TREE_HEIGHT) + MAX_OUTPUT_TREE_HEIGHT
			output.OutputHashesSiblings = proofs[start:end]
		}
		return &claim, epochOutputs, nil
	}
	// if there are no outputs and there is a previous epoch, return its claim
	return previousEpoch.Claim, nil, nil
}
