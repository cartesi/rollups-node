// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"cmp"
	"context"
	"log/slog"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

func (r *EvmReader) checkForClaimStatus(
	ctx context.Context,
	apps []application,
	mostRecentBlockNumber uint64,
) {

	slog.Debug("Checking for new Claim Acceptance Events")

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

			slog.Info("Checking claim acceptance for applications",
				"apps", appAddresses,
				"last claim check block", lastClaimCheck,
				"most recent block", mostRecentBlockNumber)

			r.readAndUpdateClaims(ctx, apps, lastClaimCheck, mostRecentBlockNumber)

		} else if mostRecentBlockNumber < lastClaimCheck {
			slog.Warn(
				"Not reading claim acceptance: most recent block is lower than the last processed one", //nolint:lll
				"apps", appAddresses,
				"last claim check block", lastClaimCheck,
				"most recent block", mostRecentBlockNumber,
			)
		} else {
			slog.Info("Not reading claim acceptance: already checked the most recent blocks",
				"apps", appAddresses,
				"last claim check block", lastClaimCheck,
				"most recent block", mostRecentBlockNumber,
			)
		}

	}
}

func (r *EvmReader) readAndUpdateClaims(
	ctx context.Context,
	apps []application,
	lastClaimCheck, mostRecentBlockNumber uint64,
) {

	// DISCLAIMER: The current algorithm will only handle Authority.
	// To handle Quorum, node needs to handle acceptance events
	// that can happen before claim submission

	// Classify them by the same IConsensusAddress
	sameConsensusApps := indexApps(keyByIConsensus, apps)
	for iConsensusAddress, apps := range sameConsensusApps {

		appAddresses := appsToAddresses(apps)

		// All apps shares the same IConsensus
		// If there is a key on indexApps, there is at least one
		// application in the referred application slice
		consensusContract := apps[0].consensusContract

		// Retrieve Claim Acceptance Events from blockchain
		appClaimAcceptanceEventMap, err := r.readClaimsAcceptance(
			ctx, consensusContract, appAddresses, lastClaimCheck+1, mostRecentBlockNumber)
		if err != nil {
			slog.Error("Error reading claim acceptance status",
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

			epochs := []*Epoch{}
			for _, claimAcceptance := range claimAcceptances {

				// Get Previous Epochs with submitted claims, If is there any,
				// Application is in an invalid State.
				previousEpochs, err := r.repository.GetPreviousEpochsWithOpenClaims(
					ctx, app, claimAcceptance.LastProcessedBlockNumber.Uint64())
				if err != nil {
					slog.Error("Error retrieving previous submitted claims",
						"app", app,
						"block", claimAcceptance.LastProcessedBlockNumber.Uint64(),
						"error", err)
					continue APP_LOOP
				}
				if len(previousEpochs) == 0 { // FIXME review this condition
					slog.Error("Application got 'not accepted' claims. It is in an invalid state",
						"app", app)
					continue APP_LOOP
				}

				// Get the Epoch for the current Claim Acceptance Event
				epoch, err := r.repository.GetEpoch(
					ctx, calculateEpochIndex(
						r.epochLengthCache[app],
						claimAcceptance.LastProcessedBlockNumber.Uint64()),
					app)
				if err != nil {
					slog.Error("Error retrieving Epoch",
						"app", app,
						"block", claimAcceptance.LastProcessedBlockNumber.Uint64(),
						"error", err)
					continue APP_LOOP
				}

				// Check Epoch
				if epoch == nil {
					slog.Error(
						"Got a claim acceptance event for an unknown epoch. Application is in an invalid state", //nolint:lll
						"app", app,
						"claim last block", claimAcceptance.LastProcessedBlockNumber,
						"hash", claimAcceptance.Claim)
					continue APP_LOOP
				}
				if claimAcceptance.Claim != *epoch.ClaimHash ||
					claimAcceptance.LastProcessedBlockNumber.Uint64() != epoch.LastBlock {
					slog.Error("Accepted Claim does not match actual Claim. Application is in an invalid state", //nolint:lll
						"app", app,
						"lastBlock", epoch.LastBlock,
						"hash", epoch.ClaimHash)

					continue APP_LOOP
				}

				// Update Epoch claim status
				slog.Info("Claim Accepted",
					"app", app,
					"lastBlock", epoch.LastBlock,
					"hash", epoch.ClaimHash)

				epoch.Status = EpochStatusClaimAccepted
				epochs = append(epochs, epoch)
			}

			// Store everything
			err = r.repository.UpdateEpochs(
				ctx, app, epochs, mostRecentBlockNumber)
			if err != nil {
				slog.Error("Error storing claims", "app", app, "error", err)
				continue
			}
		}
	}
}

func (r *EvmReader) readClaimsAcceptance(
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
func keyByLastClaimCheck(app application) uint64 {
	return app.LastClaimCheckBlock
}

// keyByIConsensus is a IConsensus address key extractor function intended
// to be used with `indexApps` function, see indexApps()
func keyByIConsensus(app application) Address {
	return app.IConsensusAddress
}

// sortByLastBlockNumber is a ClaimAcceptance's  by last block number sorting function.
// Intended to be used with insertSorted function, see insertSorted()
func sortByLastBlockNumber(a, b *iconsensus.IConsensusClaimAcceptance) int {
	return cmp.Compare(a.LastProcessedBlockNumber.Uint64(), b.LastProcessedBlockNumber.Uint64())
}
