// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package validator provides components to create epoch claims and update
// rollups outputs with their proofs.
package validator

import (
	"context"
	"fmt"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type Service struct {
	service.Service
	repository ValidatorRepository
}

type CreateInfo struct {
	service.CreateInfo
	PostgresEndpoint config.Redacted[string]
	Repository       ValidatorRepository
	PollingInterval  time.Duration
	MaxStartupTime   time.Duration
}

func (c *CreateInfo) LoadEnv() {
	c.PostgresEndpoint.Value = config.GetPostgresEndpoint()
	c.PollInterval = config.GetValidatorPollingInterval()
	c.LogLevel = service.LogLevel(config.GetLogLevel())
	c.LogPretty = config.GetLogPrettyEnabled()
	c.MaxStartupTime = config.GetMaxStartupTime()
}

func Create(c *CreateInfo, s *Service) error {
	var err error

	err = service.Create(&c.CreateInfo, &s.Service)
	if err != nil {
		return err
	}

	return service.WithTimeout(c.MaxStartupTime, func() error {
		if c.Repository == nil {
			c.Repository, err = factory.NewRepositoryFromConnectionString(s.Context, c.PostgresEndpoint.Value)
			if err != nil {
				return err
			}
		}
		s.repository = c.Repository
		return nil
	})
}

func (s *Service) Alive() bool     { return true }
func (s *Service) Ready() bool     { return true }
func (s *Service) Reload() []error { return nil }
func (s *Service) Tick() []error {
	if err := s.Run(s.Context); err != nil {
		return []error{err}
	}
	return []error{}
}
func (s *Service) Stop(b bool) []error {
	return nil
}

func (s *Service) Start(context context.Context, ready chan<- struct{}) error {
	ready <- struct{}{}
	return s.Serve()
}
func (v *Service) String() string {
	return v.Name
}

// The maximum height for the Merkle tree of all outputs produced
// by an application
const MAX_OUTPUT_TREE_HEIGHT = 63

type ValidatorRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination) ([]*Application, error)

	// GetOutputsProducedInBlockRange returns outputs produced by inputs sent to
	// the application in the provided block range, inclusive. Outputs are
	// returned in ascending order by index.
	// GetOutputsProducedInBlockRange(
	// 	ctx context.Context,
	// 	application common.Address,
	// 	firstBlock, lastBlock uint64,
	// ) ([]Output, error)
	ListOutputs(ctx context.Context, nameOrAddress string, f repository.OutputFilter, p repository.Pagination) ([]*Output, error)

	// GetProcessedEpochs returns epochs from the application which had all
	// its inputs processed. Epochs are returned in ascending order by index.
	// GetProcessedEpochs(ctx context.Context, application common.Address) ([]Epoch, error)
	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter, p repository.Pagination) ([]*Epoch, error)

	// GetLastInputOutputsHash returns the outputs Merkle tree hash calculated
	// by the Cartesi Machine after it processed the last input in the epoch.
	GetLastInput(ctx context.Context, appAddress string, epochIndex uint64) (*Input, error) // FIXME migrate to list
	//ListInputs(ctx context.Context, nameOrAddress string, f repository.InputFilter, p repository.Pagination) ([]*Input, error)

	// GetPreviousEpoch returns the epoch that ended one block before the start
	// of the current epoch
	GetEpochByVirtualIndex(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)

	// ValidateEpochTransaction performs a database transaction
	// containing two operations:
	//
	// 1. Updates an epoch, adding its claim and modifying its status.
	//
	// 2. Updates outputs with their Keccak256 hash and proof.
	// SetEpochClaimAndInsertProofsTransaction(
	// 	ctx context.Context,
	// 	epoch Epoch,
	// 	outputs []Output,
	// ) error
	StoreClaimAndProofs(ctx context.Context, epoch *Epoch, outputs []*Output) error
}

func getAllRunningApplications(ctx context.Context, er ValidatorRepository) ([]*Application, error) {
	f := repository.ApplicationFilter{State: Pointer(ApplicationState_Enabled)}
	return er.ListApplications(ctx, f, repository.Pagination{})
}

// Run executes the Validator main logic of producing claims and/or proofs
// for processed epochs of all running applications. It is meant to be executed
// inside a loop. If an error occurs while processing any epoch, it halts and
// returns the error.
func (v *Service) Run(ctx context.Context) error {
	apps, err := getAllRunningApplications(ctx, v.repository)
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

func getProcessedEpochs(ctx context.Context, er ValidatorRepository, address string) ([]*Epoch, error) {
	f := repository.EpochFilter{Status: Pointer(EpochStatus_InputsProcessed)}
	return er.ListEpochs(ctx, address, f, repository.Pagination{})
}

// validateApplication calculates, validates and stores the claim and/or proofs
// for each processed epoch of the application.
func (v *Service) validateApplication(ctx context.Context, app *Application) error {
	v.Logger.Debug("Starting validation", "application", app.Name)
	appAddress := app.IApplicationAddress.String()
	processedEpochs, err := getProcessedEpochs(ctx, v.repository, appAddress)
	if err != nil {
		return fmt.Errorf(
			"failed to get processed epochs of application %v. %w",
			app.IApplicationAddress, err,
		)
	}

	for _, epoch := range processedEpochs {
		v.Logger.Debug("Started calculating claim",
			"app", app.IApplicationAddress,
			"epoch_index", epoch.Index,
			"last_block", epoch.LastBlock,
		)
		claim, outputs, err := v.createClaimAndProofs(ctx, appAddress, epoch)
		v.Logger.Info("Claim Computed",
			"app", app.IApplicationAddress,
			"epoch_index", epoch.Index,
			"last_block", epoch.LastBlock,
		)
		if err != nil {
			return err
		}

		// The Cartesi Machine calculates the root hash of the outputs Merkle
		// tree after each input. Therefore, the root hash calculated after the
		// last input in the epoch must match the claim hash calculated by the
		// Validator. We first retrieve the hash calculated by the
		// Cartesi Machine...
		input, err := v.repository.GetLastInput(
			ctx,
			appAddress,
			epoch.Index,
		)
		if err != nil {
			return fmt.Errorf(
				"failed to get the machine claim for epoch %v of application %v. %w",
				epoch.Index, app.IApplicationAddress, err,
			)
		}

		if input.OutputsHash == nil {
			return fmt.Errorf(
				"inconsistent state: machine claim for epoch %v of application %v was not found",
				epoch.Index, app.IApplicationAddress,
			)
		}

		// ...and compare it to the hash calculated by the Validator
		if *input.OutputsHash != *claim {
			return fmt.Errorf(
				"validator claim does not match machine claim for epoch %v of application %v",
				epoch.Index, app.IApplicationAddress,
			)
		}

		// update the epoch status and its claim
		epoch.Status = EpochStatus_ClaimComputed
		epoch.ClaimHash = claim

		// store the epoch and proofs in the database
		err = v.repository.StoreClaimAndProofs(ctx, epoch, outputs)
		if err != nil {
			return fmt.Errorf(
				"failed to store claim and proofs for epoch %v of application %v. %w",
				epoch.Index, app.IApplicationAddress, err,
			)
		}
	}

	if len(processedEpochs) == 0 {
		v.Logger.Debug("no processed epochs to validate",
			"app", app.IApplicationAddress,
		)
	}

	return nil
}

func getOutputsProducedInBlockRange(
	ctx context.Context,
	vr ValidatorRepository,
	address string,
	start uint64,
	end uint64,
) ([]*Output, error) {
	r := repository.Range{Start: start, End: end}
	f := repository.OutputFilter{BlockRange: Pointer(r)}
	return vr.ListOutputs(ctx, address, f, repository.Pagination{})
}

// createClaimAndProofs calculates the claim and proofs for an epoch. It returns
// the claim and the epoch outputs updated with their hash and proofs. In case
// the epoch has no outputs, there are no proofs and it returns the pristine
// claim for the first epoch or the previous epoch claim otherwise.
func (v *Service) createClaimAndProofs(
	ctx context.Context,
	appAddress string,
	epoch *Epoch,
) (*common.Hash, []*Output, error) {
	epochOutputs, err := getOutputsProducedInBlockRange(
		ctx,
		v.repository,
		appAddress,
		epoch.FirstBlock,
		epoch.LastBlock,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"failed to get outputs for epoch %v (%v) of application %v. %w",
			epoch.Index, epoch.VirtualIndex, appAddress, err,
		)
	}

	var previousEpoch *Epoch
	if epoch.VirtualIndex > 0 {
		previousEpoch, err = v.repository.GetEpochByVirtualIndex(ctx, appAddress, epoch.VirtualIndex-1)
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
			claim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
			if err != nil {
				return nil, nil, fmt.Errorf(
					"failed to create proofs for epoch %v (%v) of application %v. %w",
					epoch.Index, epoch.VirtualIndex, appAddress, err,
				)
			}
			return &claim, nil, nil
		}
	} else {
		// if epoch has outputs, calculate a new claim and proofs
		var previousOutputs []*Output
		if previousEpoch != nil {
			// get all outputs created before the current epoch
			previousOutputs, err = getOutputsProducedInBlockRange(
				ctx,
				v.repository,
				appAddress,
				0, // Current implementation requires all outputs
				previousEpoch.LastBlock,
			)
			if err != nil {
				return nil, nil, fmt.Errorf(
					"failed to get all outputs of application %v before epoch %d. %w",
					appAddress, epoch.Index, err,
				)
			}
		}
		// the leaves of the Merkle tree are the Keccak256 hashes of all the
		// outputs
		leaves := make([]common.Hash, 0, len(epochOutputs)+len(previousOutputs))
		for idx := range previousOutputs {
			if previousOutputs[idx].Hash == nil {
				// should never happen
				return nil, nil, fmt.Errorf(
					"missing hash of output %d from input %d",
					previousOutputs[idx].Index, previousOutputs[idx].InputIndex,
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
				epoch.Index, appAddress, err,
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
