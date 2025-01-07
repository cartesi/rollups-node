// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"cmp"
	"context"
	"strings"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

func (r *Service) checkForClaimStatus(
	ctx context.Context,
	apps []appContracts,
	mostRecentBlockNumber uint64,
) {

	r.Logger.Debug("Checking for new Claim Acceptance Events")

	// Classify them by lastClaimCheck block
	appsIndexedByLastCheck := indexApps(keyByLastClaimCheck, apps)

	for lastClaimCheck, apps := range appsIndexedByLastCheck {

		appAddresses := appsToAddresses(apps)

		// Safeguard: Only check blocks starting from the block where the InputBox
		// contract was deployed as Inputs can be added to that same block
		if lastClaimCheck < r.inputBoxDeploymentBlock {
			lastClaimCheck = r.inputBoxDeploymentBlock - 1
		}

		if mostRecentBlockNumber > lastClaimCheck {

			r.Logger.Debug("Checking claim acceptance for applications",
				"apps", appAddresses,
				"last claim check block", lastClaimCheck,
				"most recent block", mostRecentBlockNumber)

			r.readAndUpdateClaims(ctx, apps, lastClaimCheck, mostRecentBlockNumber)

		} else if mostRecentBlockNumber < lastClaimCheck {
			r.Logger.Warn(
				"Not reading claim acceptance: most recent block is lower than the last processed one", //nolint:lll
				"apps", appAddresses,
				"last claim check block", lastClaimCheck,
				"most recent block", mostRecentBlockNumber,
			)
		} else {
			r.Logger.Warn("Not reading claim acceptance: already checked the most recent blocks",
				"apps", appAddresses,
				"last claim check block", lastClaimCheck,
				"most recent block", mostRecentBlockNumber,
			)
		}

	}
}

func getPreviousEpochsWithSubmittedClaims(ctx context.Context, er EvmReaderRepository, appAddress string, block uint64) ([]*Epoch, error) {
	f := repository.EpochFilter{Status: Pointer(EpochStatus_ClaimSubmitted), BeforeBlock: Pointer(block)}
	return er.ListEpochs(ctx, appAddress, f, repository.Pagination{})
}

func (r *Service) readAndUpdateClaims(
	ctx context.Context,
	apps []appContracts,
	lastClaimCheck, mostRecentBlockNumber uint64,
) {

	// DISCLAIMER: The current algorithm will only handle Authority.
	// To handle Quorum, node needs to handle acceptance events
	// that can happen before claim submission

	// DISCLAIMER 2: The current algorithm does not consider that there might
	// be more than one claimAcceptance per block

	// Classify them by the same IConsensusAddress
	sameConsensusApps := indexApps(keyByIConsensus, apps)
	for iConsensusAddress, apps := range sameConsensusApps {

		appAddresses := appsToAddresses(apps)

		// All apps shares the same IConsensus
		// If there is a key on indexApps, there is at least one
		// application in the referred application slice
		consensusContract := apps[0].consensusContract
		epochLength := apps[0].application.EpochLength

		// Retrieve Claim Acceptance Events from blockchain
		appClaimAcceptanceEventMap, err := r.readClaimsAcceptance(
			ctx, consensusContract, appAddresses, lastClaimCheck+1, mostRecentBlockNumber)
		if err != nil {
			r.Logger.Error("Error reading claim acceptance status",
				"apps", apps,
				"IConsensus", iConsensusAddress,
				"start", lastClaimCheck,
				"end", mostRecentBlockNumber,
				"error", err)
			continue
		}

		// Check events against Epochs
	APP_LOOP:
		for app, claimAcceptances := range appClaimAcceptanceEventMap {
			appHexAddress := strings.ToLower(app.Hex())
			for _, claimAcceptance := range claimAcceptances {

				// Get Previous Epochs with submitted claims, If is there any,
				// Application is in an invalid State.
				previousEpochs, err := getPreviousEpochsWithSubmittedClaims(
					ctx, r.repository, appHexAddress, claimAcceptance.LastProcessedBlockNumber.Uint64())
				if err != nil {
					r.Logger.Error("Error retrieving previous submitted claims",
						"address", app,
						"block", claimAcceptance.LastProcessedBlockNumber.Uint64(),
						"error", err)
					continue APP_LOOP
				}
				if len(previousEpochs) > 0 {
					r.Logger.Error("Application got 'not accepted' claims. It is in an invalid state",
						"claim last block", claimAcceptance.LastProcessedBlockNumber,
						"app", app)
					continue APP_LOOP
				}

				// Get the Epoch for the current Claim Acceptance Event
				epoch, err := r.repository.GetEpoch(
					ctx, app.Hex(), calculateEpochIndex(
						epochLength,
						claimAcceptance.LastProcessedBlockNumber.Uint64()),
				)
				if err != nil {
					r.Logger.Error("Error retrieving Epoch",
						"address", app,
						"block", claimAcceptance.LastProcessedBlockNumber.Uint64(),
						"error", err)
					continue APP_LOOP
				}

				// Check Epoch
				if epoch == nil {
					if r.inputReaderEnabled {
						r.Logger.Error(
							"Found claim acceptance event for an unknown epoch. Application is in an invalid state", //nolint:lll
							"address", app,
							"claim last block", claimAcceptance.LastProcessedBlockNumber,
							"hash", claimAcceptance.Claim)
					} else {
						r.Logger.Warn(
							"Found claim acceptance event for an epoch that does not exist on the database",
							"address", app,
							"claim last block", claimAcceptance.LastProcessedBlockNumber,
						)

					}
					continue APP_LOOP
				}
				if epoch.ClaimHash == nil {
					r.Logger.Warn(
						"Found claim acceptance event, but claim hasn't been calculated yet",
						"address", app,
						"last_block", claimAcceptance.LastProcessedBlockNumber,
					)
					continue APP_LOOP
				}
				if claimAcceptance.Claim != *epoch.ClaimHash ||
					claimAcceptance.LastProcessedBlockNumber.Uint64() != epoch.LastBlock {
					r.Logger.Error("Accepted Claim does not match actual Claim. Application is in an invalid state", //nolint:lll
						"address", app,
						"last_block", epoch.LastBlock,
						"hash", epoch.ClaimHash)

					continue APP_LOOP
				}
				if epoch.Status == EpochStatus_ClaimAccepted {
					r.Logger.Debug("Claim already accepted. Skipping",
						"address", app,
						"last_block", claimAcceptance.LastProcessedBlockNumber.Uint64(),
						"claimStatus", epoch.Status,
						"hash", epoch.ClaimHash)
					continue
				}
				if epoch.Status != EpochStatus_ClaimSubmitted {
					// this happens when running on latest. EvmReader can see the event before
					// the claim is marked as submitted by the claimer.
					r.Logger.Debug("Claim status is not submitted. Skipping for now",
						"address", app,
						"last_block", claimAcceptance.LastProcessedBlockNumber.Uint64(),
						"claimStatus", epoch.Status,
						"hash", epoch.ClaimHash)
					continue APP_LOOP
				}

				// Update Epoch claim status
				r.Logger.Info("Claim Accepted",
					"address", app,
					"epoch_index", epoch.Index,
					"last_block", epoch.LastBlock,
					"hash", epoch.ClaimHash,
					"last_claim_check_block", claimAcceptance.Raw.BlockNumber)

				epoch.Status = EpochStatus_ClaimAccepted
				// Store epoch
				err = r.repository.UpdateEpochsClaimAccepted(
					ctx, appHexAddress, []*Epoch{epoch}, claimAcceptance.Raw.BlockNumber)
				if err != nil {
					r.Logger.Error("Error storing claims", "address", app, "error", err)
					continue
				}
			}

		}
	}
}

func (r *Service) readClaimsAcceptance(
	ctx context.Context,
	consensusContract ConsensusContract,
	appAddresses []common.Address,
	startBlock, endBlock uint64,
) (map[common.Address][]*iconsensus.IConsensusClaimAcceptance, error) {
	appClaimAcceptanceMap := make(map[common.Address][]*iconsensus.IConsensusClaimAcceptance)
	for _, address := range appAddresses {
		appClaimAcceptanceMap[address] = []*iconsensus.IConsensusClaimAcceptance{}
	}
	opts := &bind.FilterOpts{
		Context: ctx,
		Start:   startBlock,
		End:     &endBlock,
	}
	claimAcceptanceEvents, err := consensusContract.RetrieveClaimAcceptanceEvents(
		opts, appAddresses)
	if err != nil {
		return nil, err
	}
	for _, event := range claimAcceptanceEvents {
		appClaimAcceptanceMap[event.AppContract] = insertSorted(
			sortByLastBlockNumber, appClaimAcceptanceMap[event.AppContract], event)
	}
	return appClaimAcceptanceMap, nil
}

// keyByLastClaimCheck is a LastClaimCheck key extractor function intended
// to be used with `indexApps` function, see indexApps()
func keyByLastClaimCheck(app appContracts) uint64 {
	return app.application.LastClaimCheckBlock
}

// keyByIConsensus is a IConsensus address key extractor function intended
// to be used with `indexApps` function, see indexApps()
func keyByIConsensus(app appContracts) common.Address {
	return app.application.IConsensusAddress
}

// sortByLastBlockNumber is a ClaimAcceptance's  by last block number sorting function.
// Intended to be used with insertSorted function, see insertSorted()
func sortByLastBlockNumber(a, b *iconsensus.IConsensusClaimAcceptance) int {
	return cmp.Compare(a.LastProcessedBlockNumber.Uint64(), b.LastProcessedBlockNumber.Uint64())
}
