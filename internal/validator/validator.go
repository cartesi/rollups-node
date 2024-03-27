// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"time"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

const TICK_INTERVAL = 100 * time.Millisecond

// ------------------------------------------------------------------------------------------------
// Types
// ------------------------------------------------------------------------------------------------

type Validator struct {
	repo                    ValidatorRepository
	epochDuration           uint64
	inputBoxDeploymentBlock uint64
}

type ValidatorRepository interface {
	// GetMostRecentBlock returns the number of the last finalized block
	// processed by InputReader
	GetMostRecentBlock(ctx context.Context) (uint64, error)
	// GetCurrentEpoch returns the current epoch
	GetCurrentEpoch(ctx context.Context) (Epoch, error)
	// GetMachineStateHash returns the hash of the state of the Cartesi Machine
	// after processing the input at `inputIndex`
	GetMachineStateHash(ctx context.Context, inputIndex uint64) (hexutil.Bytes, error)
	// GetAllOutputsFromProcessedInputs returns an ordered slice of all outputs
	// produced by inputs sent between `startBlock` and `endBlock` (inclusive).
	// If one or more inputs are still unprocessed,
	// it waits for its status to change until a timeout is reached
	GetAllOutputsFromProcessedInputs(
		ctx context.Context,
		startBlock uint64,
		endBlock uint64,
		timeout *time.Duration,
	) ([]Output, error)
	// InsertFirstEpochTransaction performs a database transaction
	// containing two operations:
	//
	// 1. Inserts a new Epoch
	//
	// 2. Updates the current Epoch to the newly created Epoch
	//
	// This should only be called once to set up the first Epoch in the database.
	// If there's a current Epoch already, this call will have no effect
	InsertFirstEpochTransaction(
		ctx context.Context,
		epoch Epoch,
	) error
	// FinishEmptyEpochTransaction performs a database transaction
	// containing two operations:
	//
	// 	1. Inserts a new Epoch
	//
	// 	2. Updates the current Epoch to the newly created Epoch
	FinishEmptyEpochTransaction(ctx context.Context, nextEpoch Epoch) error
	// FinishEpochTransaction performs a database transaction
	// containing four operations:
	//
	// 	1. Inserts a new Epoch
	//
	// 	2. Updates the current Epoch to the newly created Epoch
	//
	// 	3. Inserts a new Claim
	//
	//	4. Inserts all the proofs from the last Epoch
	FinishEpochTransaction(
		ctx context.Context,
		nextEpoch Epoch,
		claim *Claim,
		proofs []Proof,
	) error
}

// ------------------------------------------------------------------------------------------------
// Service implementation
// ------------------------------------------------------------------------------------------------

func (v Validator) String() string {
	return "validator"
}

func (v Validator) Start(ctx context.Context, ready chan<- struct{}) error {
	// create and attempt to insert the first epoch
	epoch := Epoch{
		StartBlock: v.inputBoxDeploymentBlock,
		EndBlock:   v.inputBoxDeploymentBlock + v.epochDuration,
	}
	if err := v.repo.InsertFirstEpochTransaction(ctx, epoch); err != nil {
		return err
	}
	ready <- struct{}{}

	ticker := time.NewTicker(TICK_INTERVAL)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		// the current epoch might be the one we just created or another
		// if this isn't the first run
		currentEpoch, err := v.repo.GetCurrentEpoch(ctx)
		if err != nil {
			return err
		}
		latestBlock, err := v.repo.GetMostRecentBlock(ctx)
		if err != nil {
			return err
		}
		// test if should finish epoch
		if latestBlock < currentEpoch.EndBlock {
			continue
		}

		outputs, err := v.repo.GetAllOutputsFromProcessedInputs(
			ctx,
			currentEpoch.StartBlock,
			currentEpoch.EndBlock,
			nil,
		)
		if err != nil {
			return err
		}
		newEpoch := Epoch{
			StartBlock: currentEpoch.EndBlock + 1,
			EndBlock:   currentEpoch.EndBlock + v.epochDuration,
		}
		if len(outputs) == 0 {
			err := v.repo.FinishEmptyEpochTransaction(ctx, newEpoch)
			if err != nil {
				return err
			}
		} else {
			lastIndex := len(outputs) - 1
			inputRange := InputRange{
				First: outputs[0].InputIndex,
				Last:  outputs[lastIndex].InputIndex,
			}
			machineStateHash, err := v.repo.GetMachineStateHash(ctx, inputRange.Last)
			if err != nil {
				return err
			}
			proofs, err := generateProofs(
				ctx,
				inputRange,
				machineStateHash,
				outputs,
			)
			if err != nil {
				return err
			}
			outputsEpochRootHash := proofs[0].OutputsEpochRootHash
			epochHash := crypto.Keccak256(outputsEpochRootHash, machineStateHash)
			claim := &Claim{InputRange: inputRange, EpochHash: epochHash}
			if err = v.repo.FinishEpochTransaction(
				ctx,
				newEpoch,
				claim,
				proofs,
			); err != nil {
				return err
			}
		}
	}
}

// ------------------------------------------------------------------------------------------------
// Validator functions
// ------------------------------------------------------------------------------------------------

func NewValidator(
	repo ValidatorRepository,
	epochDuration uint64,
	inputBoxDeploymentBlock uint64,
) Validator {
	return Validator{repo, epochDuration, inputBoxDeploymentBlock}
}
