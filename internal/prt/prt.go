// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/daveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/toptournament"
)

type PrtRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter,
		p repository.Pagination, descending bool) ([]*Application, uint64, error)
	UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error

	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter,
		p repository.Pagination, descending bool) ([]*Epoch, uint64, error)
	UpdateEpoch(ctx context.Context, nameOrAddress string, e *Epoch) error
	UpdateEpochStatus(ctx context.Context, nameOrAddress string, e *Epoch) error

	CreateTournament(ctx context.Context, nameOrAddress string, t *Tournament) error

	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)
}

// EthClientInterface defines the methods we need from ethclient.Client
type EthClientInterface interface {
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	ChainID(ctx context.Context) (*big.Int, error)
}

func getAllRunningApplications(ctx context.Context, r PrtRepository) ([]*Application, uint64, error) {
	f := repository.ApplicationFilter{State: Pointer(ApplicationState_Enabled), DaveConsensus: Pointer(true)}
	return r.ListApplications(ctx, f, repository.Pagination{}, false)
}

func getAllClaimComputedEpochs(ctx context.Context, r PrtRepository, nameOrAddress string) ([]*Epoch, uint64, error) {
	f := repository.EpochFilter{Status: Pointer(EpochStatus_ClaimComputed)}
	return r.ListEpochs(ctx, nameOrAddress, f, repository.Pagination{}, false)
}

// setApplicationInoperable marks an application as inoperable with the given reason,
// logs any error that occurs during the update, and returns an error with the reason.
func (s *Service) setApplicationInoperable(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	reason := fmt.Sprintf(reasonFmt, args...)
	appAddress := app.IApplicationAddress.String()

	// Log the reason first
	s.Logger.Error(reason, "application", appAddress)

	// Update application state
	err := s.repository.UpdateApplicationState(ctx, app.ID, ApplicationState_Inoperable, &reason)
	if err != nil {
		s.Logger.Error("failed to update application state to inoperable", "app", appAddress, "err", err)
	}

	// Return the error with the reason
	return errors.New(reason)
}

func (s *Service) checkFinalizedEpochs(ctx context.Context, app *Application) error {
	epochs, _, err := getAllClaimComputedEpochs(ctx, s.repository, app.Name)
	if err != nil {
		s.Logger.Error("failed to list epochs", "application", app.Name, "error", err)
		return err
	}
	if len(epochs) == 0 {
		return nil // nothing to do
	}

	// TODO: use adapters instead of direct contract calls
	// Type assertion to get the concrete client if possible
	ethClient, ok := s.client.(*ethclient.Client)
	if !ok {
		return fmt.Errorf("client is not an *ethclient.Client, cannot create dave consensus bind")
	}

	consensus, err := daveconsensus.NewDaveConsensus(app.IConsensusAddress, ethClient)
	if err != nil {
		s.Logger.Error("failed to bind dave consensus contract", "application", app.Name,
			"consensus_address", app.IConsensusAddress.String(), "error", err)
		return err
	}

	for _, epoch := range epochs {
		if epoch.ClaimTransactionHash == nil {
			break
		}
		receipt, err := ethClient.TransactionReceipt(ctx, *epoch.ClaimTransactionHash)
		if err != nil {
			s.Logger.Error("failed to fetch transaction receipt for epoch", "application", app.Name,
				"epoch", epoch.Index, "tx", epoch.ClaimTransactionHash, "error", err)
			return err
		}

		if receipt.Status != 1 {
			return fmt.Errorf("EpochSealed transaction hash points to failed transaction")
		}

		var event *daveconsensus.DaveConsensusEpochSealed
		for _, vLog := range receipt.Logs {
			event, err = consensus.ParseEpochSealed(*vLog)
			if err != nil {
				continue // Skip logs that don't match
			}
		}
		if event == nil {
			return fmt.Errorf("failed to find EpochSealed event in receipt logs")

		}

		if epoch.Index != event.EpochNumber.Uint64()-1 {
			return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent index between off-chain (%d) and on-chain (%d)",
				epoch.Index, epoch.Index, event.EpochNumber.Uint64()-1)
		}
		if *epoch.MachineHash != event.InitialMachineStateHash {
			return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent machine hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.MachineHash.String(), hexutil.Encode(event.InitialMachineStateHash[:]))
		}
		if *epoch.ClaimHash != event.OutputsMerkleRoot {
			return s.setApplicationInoperable(ctx, app, "Epoch %d has inconsistent claim hash between off-chain (%s) and on-chain (%s)",
				epoch.Index, epoch.ClaimHash.String(), hexutil.Encode(event.OutputsMerkleRoot[:]))
		}

		err = s.fetchTournamentData(ctx, app, epoch)
		if err != nil {
			s.Logger.Error("failed to fetch tournament data", "application", app.Name,
				"epoch", epoch.Index, "tournament", epoch.TournamentAddress.String(), "error", err)
			return err
		}

		s.Logger.Info("Found finalized epoch. OutputsMerkleRoot matched. Setting claim as accepted",
			"application", app.Name,
			"epoch", epoch.Index,
			"event_block_number", event.Raw.BlockNumber,
			"claim_hash", fmt.Sprintf("%x", event.OutputsMerkleRoot),
			"tx", epoch.ClaimTransactionHash,
		)

		epoch.Status = EpochStatus_ClaimAccepted
		err = s.repository.UpdateEpochStatus(ctx, app.Name, epoch)
		if err != nil {
			s.Logger.Error("failed to update epoch status to claim accepted", "application", app.Name, "epoch", epoch.Index, "error", err)
			return err
		}
	}
	return nil
}

func (s *Service) fetchTournamentData(ctx context.Context, app *Application, epoch *Epoch) error {
	// TODO: use adapters instead of direct contract calls
	// Type assertion to get the concrete client if possible
	ethClient, ok := s.client.(*ethclient.Client)
	if !ok {
		return fmt.Errorf("client is not an *ethclient.Client, cannot create dave consensus bind")
	}
	//println("epoch:", epoch.Index)
	//println("claim_transaction_hash:", epoch.ClaimTransactionHash.Hex())

	rootTournament, err := toptournament.NewTopTournament(*epoch.TournamentAddress, ethClient)
	if err != nil {
		s.Logger.Error("failed to bind top tournament contract", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", epoch.TournamentAddress.String(), "error", err)
		return err
	}
	//println("tournament:", epoch.TournamentAddress.String())
	constants, err := rootTournament.TournamentLevelConstants(nil)
	if err != nil {
		s.Logger.Error("failed to fetch tournament constants", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", epoch.TournamentAddress.String(), "error", err)
		return err
	}
	//println("constants:", constants.MaxLevel, constants.Level, constants.Log2step, constants.Height)
	t := Tournament{
		ApplicationID: app.ID,
		EpochIndex:    epoch.Index,
		Address:       *epoch.TournamentAddress,
		MaxLevel:      constants.MaxLevel,
		Level:         constants.Level,
		Log2Step:      constants.Log2step,
		Height:        constants.Height,
	}

	finished, timeFinished, err := rootTournament.TimeFinished(nil)
	if err != nil {
		s.Logger.Error("failed to fetch tournament finished at time", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", epoch.TournamentAddress.String(), "error", err)
		return err
	}
	//println("finished:", finished, timeFinished)
	if finished {
		t.FinishedAtBlock = timeFinished
	}

	err = s.repository.CreateTournament(ctx, app.IApplicationAddress.Hex(), &t)
	if err != nil {
		s.Logger.Error("failed to create tournament in database", "application", app.Name,
			"epoch", epoch.Index, "tournament_address", epoch.TournamentAddress.String(), "error", err)
		return err
	}

	return nil
}

func (s *Service) validateApplication(ctx context.Context, app *Application) error {
	s.Logger.Debug("Syncing PTR tournaments", "application", app.Name)
	return s.checkFinalizedEpochs(ctx, app)
}
