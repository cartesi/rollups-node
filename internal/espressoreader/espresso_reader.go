// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package espressoreader

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/EspressoSystems/espresso-sequencer-go/client"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/tidwall/gjson"
)

type EspressoReader struct {
	url           string
	client        client.Client
	startingBlock uint64
	namespace     uint64
	repository    *repository.Database
	evmReader     *evmreader.EvmReader
}

func NewEspressoReader(url string, startingBlock uint64, namespace uint64, repository *repository.Database, evmReader *evmreader.EvmReader) EspressoReader {
	client := client.NewClient(url)
	return EspressoReader{url: url, client: *client, startingBlock: startingBlock, namespace: namespace, repository: repository, evmReader: evmReader}
}

func (e *EspressoReader) Run(ctx context.Context) error {
	currentBlockHeight := e.startingBlock
	if currentBlockHeight == 0 {
		lastestEspressoBlockHeight, err := e.client.FetchLatestBlockHeight(ctx)
		if err != nil {
			return err
		}
		currentBlockHeight = lastestEspressoBlockHeight
		slog.Info("Espresso: starting from latest block height", "lastestEspressoBlockHeight", lastestEspressoBlockHeight)
	}
	previousBlockHeight := currentBlockHeight
	l1FinalizedPrevHeight := e.getL1FinalizedHeight(previousBlockHeight)

	// main polling loop
	for {
		// fetch latest espresso block height
		latestBlockHeight, err := e.client.FetchLatestBlockHeight(ctx)
		if err != nil {
			slog.Error("failed fetching latest espresso block height", "error", err)
			return err
		}
		slog.Info("Espresso:", "latestBlockHeight", latestBlockHeight)

		// take a break :)
		if latestBlockHeight == currentBlockHeight {
			var delay time.Duration = 800
			time.Sleep(delay * time.Millisecond)
			continue
		}

		for ; currentBlockHeight < latestBlockHeight; currentBlockHeight++ {
			slog.Info("Espresso:", "currentBlockHeight", currentBlockHeight, "namespace", e.namespace)

			//** read inputbox **//

			l1FinalizedCurrentHeight := e.getL1FinalizedHeight(currentBlockHeight)
			// read L1 if there might be update
			if l1FinalizedCurrentHeight > l1FinalizedPrevHeight || currentBlockHeight == e.startingBlock {
				slog.Info("L1 finalized", "from", l1FinalizedPrevHeight, "to", l1FinalizedCurrentHeight)
				slog.Info("Fetching InputBox between Espresso blocks", "from", previousBlockHeight, "to", currentBlockHeight)

				e.evmReader.ReadAndStoreInputs(ctx, l1FinalizedPrevHeight, l1FinalizedCurrentHeight, e.getAppsForEvmReader(ctx))

				l1FinalizedPrevHeight = l1FinalizedCurrentHeight
			}

			//** read espresso **//

			transactions, err := e.client.FetchTransactionsInBlock(ctx, currentBlockHeight, e.namespace)
			if err != nil {
				slog.Error("failed fetching espresso tx", "error", err)
				return err
			}

			numTx := len(transactions.Transactions)
			slog.Info("Espresso:", "number of tx", numTx)

			for i := 0; i < numTx; i++ {
				transaction := transactions.Transactions[i]
				slog.Info("Espresso:", "currentBlockHeight", currentBlockHeight)

				// assume the following encoding
				// transaction = JSON.stringify({
				//		 	signature,
				//		 	typedData: btoa(JSON.stringify(typedData)),
				//		 })
				msgSender, typedData, _, err := ExtractSigAndData(string(transaction))
				if err != nil {
					return err
				}

				nonce := typedData.Message["nonce"]
				payload := typedData.Message["data"].(string)
				appAddressStr := typedData.Message["app"].(string)
				appAddress := common.HexToAddress(appAddressStr)
				slog.Info("Espresso input", "msgSender", msgSender, "nonce", nonce, "payload", payload, "appAddrss", appAddress)

				// TODO: handle nonce updates

				payloadBytes := []byte(payload)
				if strings.HasPrefix(payload, "0x") {
					payload = payload[2:] // remove 0x
					payloadBytes, err = hex.DecodeString(payload)
					if err != nil {
						return err
					}
				}

				// build epochInputMap
				// Initialize epochs inputs map
				var epochInputMap = make(map[*model.Epoch][]model.Input)
				// get epoch length and last open epoch
				epochLength := e.evmReader.GetEpochLengthCache(appAddress)
				if epochLength == 0 {
					slog.Error("could not obtain epoch length", "err", err)
					os.Exit(1)
				}
				currentEpoch, err := e.repository.GetEpoch(ctx,
					epochLength, appAddress)
				if err != nil {
					slog.Error("could not obtain current epoch", "err", err)
					os.Exit(1)
				}
				// if currect epoch is not nil, assume the epoch is open
				// espresso inputs do not close epoch
				epochIndex := evmreader.CalculateEpochIndex(epochLength, l1FinalizedCurrentHeight)
				if currentEpoch == nil {
					currentEpoch = &model.Epoch{
						Index:      epochIndex,
						FirstBlock: epochIndex * epochLength,
						LastBlock:  (epochIndex * epochLength) + epochLength - 1,
						Status:     model.EpochStatusOpen,
						AppAddress: appAddress,
					}
				}
				// build input
				input := model.Input{
					Index:            55555, // FIXME. There's a constraint (index, appAdress)
					CompletionStatus: model.InputStatusNone,
					RawData:          payloadBytes,
					BlockNumber:      l1FinalizedCurrentHeight,
					AppAddress:       appAddress,
				}
				currentInputs, ok := epochInputMap[currentEpoch]
				if !ok {
					currentInputs = []model.Input{}
				}
				epochInputMap[currentEpoch] = append(currentInputs, input)

				// Store everything
				// room for optimization: bundle tx by address to fully utilize `epochInputMap``
				if len(epochInputMap) > 0 {
					_, _, err = e.repository.StoreEpochAndInputsTransaction(
						ctx,
						epochInputMap,
						l1FinalizedCurrentHeight,
						appAddress,
					)
					if err != nil {
						slog.Error("could not store Espresso input", "err", err)
						os.Exit(1)
					}
				}
			}

		}
	}

}

func (e *EspressoReader) readEspressoHeader(espressoBlockHeight uint64) string {
	requestURL := fmt.Sprintf("%s/availability/header/%d", e.url, espressoBlockHeight)
	res, err := http.Get(requestURL)
	if err != nil {
		slog.Error("error making http request", "err", err)
		os.Exit(1)
	}
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		slog.Error("could not read response body", "err", err)
		os.Exit(1)
	}

	return string(resBody)
}

func (e *EspressoReader) getL1FinalizedHeight(espressoBlockHeight uint64) uint64 {
	espressoHeader := e.readEspressoHeader(espressoBlockHeight)
	value := gjson.Get(espressoHeader, "fields.l1_finalized.number")
	return value.Uint()
}

//////// evm reader related ////////

func (e *EspressoReader) getAppsForEvmReader(ctx context.Context) []evmreader.TypeExportApplication {
	// Get All Applications
	runningApps, err := e.repository.GetAllRunningApplications(ctx)
	if err != nil {
		slog.Error("Error retrieving running applications",
			"error",
			err,
		)
	}

	// Build Contracts
	var apps []evmreader.TypeExportApplication
	for _, app := range runningApps {
		_, consensusContract, err := e.evmReader.GetAppContracts(app)
		if err != nil {
			slog.Error("Error retrieving application contracts", "app", app, "error", err)
			continue
		}
		apps = append(apps, evmreader.TypeExportApplication{Application: app,
			ConsensusContract: consensusContract})
	}

	if len(apps) == 0 {
		slog.Info("No correctly configured applications running")
	}

	return apps
}
