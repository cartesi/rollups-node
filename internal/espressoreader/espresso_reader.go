// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package espressoreader

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/EspressoSystems/espresso-sequencer-go/client"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/tidwall/gjson"
)

type EspressoReader struct {
	url                     string
	client                  client.Client
	startingBlock           uint64
	namespace               uint64
	repository              *repository.Database
	evmReader               *evmreader.EvmReader
	chainId                 uint64
	inputBoxDeploymentBlock uint64
}

func NewEspressoReader(url string, startingBlock uint64, namespace uint64, repository *repository.Database, evmReader *evmreader.EvmReader, chainId uint64, inputBoxDeploymentBlock uint64) EspressoReader {
	client := client.NewClient(url)
	return EspressoReader{url: url, client: *client, startingBlock: startingBlock, namespace: namespace, repository: repository, evmReader: evmReader, chainId: chainId, inputBoxDeploymentBlock: inputBoxDeploymentBlock}
}

func (e *EspressoReader) Run(ctx context.Context, ready chan<- struct{}) error {
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
	l1FinalizedPrevHeight := e.inputBoxDeploymentBlock - 1 // because we have a +1 in the loop

	ready <- struct{}{}

	// main polling loop
	for {
		// fetch latest espresso block height
		latestBlockHeight, err := e.client.FetchLatestBlockHeight(ctx)
		if err != nil {
			slog.Error("failed fetching latest espresso block height", "error", err)
			continue
		}
		slog.Debug("Espresso:", "latestBlockHeight", latestBlockHeight)

		// take a break :)
		if latestBlockHeight == currentBlockHeight {
			var delay time.Duration = 1000
			time.Sleep(delay * time.Millisecond)
			continue
		}

		for ; currentBlockHeight < latestBlockHeight; currentBlockHeight++ {
			slog.Debug("Espresso:", "currentBlockHeight", currentBlockHeight, "namespace", e.namespace)

			//** read base layer **//

			l1FinalizedCurrentHeight, l1FinalizedTimestamp := e.getL1FinalizedHeight(currentBlockHeight)
			// read L1 if there might be update
			if l1FinalizedCurrentHeight > l1FinalizedPrevHeight {
				slog.Debug("L1 finalized", "from", l1FinalizedPrevHeight, "to", l1FinalizedCurrentHeight)
				slog.Debug("Fetching InputBox between Espresso blocks", "from", previousBlockHeight, "to", currentBlockHeight)

				apps := e.getAppsForEvmReader(ctx)

				if len(apps) > 0 {
					// start reading from the block after the prev height
					e.evmReader.ReadAndStoreInputs(ctx, l1FinalizedPrevHeight+1, l1FinalizedCurrentHeight, apps)

					// check for claim status and output execution
					e.evmReader.CheckForClaimStatus(ctx, apps, l1FinalizedCurrentHeight)
					e.evmReader.CheckForOutputExecution(ctx, apps, l1FinalizedCurrentHeight)
				}

				l1FinalizedPrevHeight = l1FinalizedCurrentHeight
			}

			//** read espresso **//

			transactions, err := e.client.FetchTransactionsInBlock(ctx, currentBlockHeight, e.namespace)
			if err != nil {
				slog.Error("failed fetching espresso tx", "error", err)
				continue
			}

			numTx := len(transactions.Transactions)
			slog.Debug("Espresso:", "number of tx", numTx)

			for i := 0; i < numTx; i++ {
				transaction := transactions.Transactions[i]
				slog.Debug("Espresso:", "currentBlockHeight", currentBlockHeight)

				// assume the following encoding
				// transaction = JSON.stringify({
				//		 	signature,
				//		 	typedData: btoa(JSON.stringify(typedData)),
				//		 })
				msgSender, typedData, sigHash, err := ExtractSigAndData(string(transaction))
				if err != nil {
					slog.Error("failed to extract espresso tx", "error", err)
					continue
				}

				nonce := uint64(typedData.Message["nonce"].(float64))
				payload := typedData.Message["data"].(string)
				appAddressStr := typedData.Message["app"].(string)
				appAddress := common.HexToAddress(appAddressStr)
				slog.Info("Espresso input", "msgSender", msgSender, "nonce", nonce, "payload", payload, "appAddrss", appAddress, "tx-id", sigHash)

				// validate nonce
				nonceInDb, err := e.repository.GetEspressoNonce(ctx, msgSender, appAddress)
				if err != nil {
					slog.Error("failed to get espresso nonce from db", "error", err)
					continue
				}
				if nonce != nonceInDb {
					slog.Error("Espresso nonce is incorrect. May be a duplicate tx", "nonce from espresso", nonce, "nonce in db", nonceInDb)
					continue
				}

				payloadBytes := []byte(payload)
				if strings.HasPrefix(payload, "0x") {
					payload = payload[2:] // remove 0x
					payloadBytes, err = hex.DecodeString(payload)
					if err != nil {
						slog.Error("failed to decode hex string", "error", err)
						continue
					}
				}
				// abi encode payload
				abiObject := rollupsmachine.GetAbi()
				chainId := &big.Int{}
				chainId.SetInt64(int64(e.chainId))
				l1FinalizedCurrentHeightBig := &big.Int{}
				l1FinalizedCurrentHeightBig.SetUint64(l1FinalizedCurrentHeight)
				l1FinalizedTimestampBig := &big.Int{}
				l1FinalizedTimestampBig.SetUint64(l1FinalizedTimestamp)
				prevRandao, err := readPrevRandao(ctx, l1FinalizedCurrentHeight, e.evmReader.GetEthClient())
				if err != nil {
					slog.Error("failed to read prevrandao", "error", err)
				}
				index := &big.Int{}
				indexUint64, err := e.repository.GetInputIndex(ctx, appAddress)
				if err != nil {
					slog.Error("failed to read index", "error", err)
				}
				index.SetUint64(indexUint64)
				payloadAbi, err := abiObject.Pack("EvmAdvance", chainId, appAddress, msgSender, l1FinalizedCurrentHeightBig, l1FinalizedTimestampBig, prevRandao, index, payloadBytes)
				if err != nil {
					slog.Error("failed to abi encode", "error", err)
					continue
				}

				// build epochInputMap
				// Initialize epochs inputs map
				var epochInputMap = make(map[*model.Epoch][]model.Input)
				// get epoch length and last open epoch
				epochLength := e.evmReader.GetEpochLengthCache(appAddress)
				if epochLength == 0 {
					slog.Error("could not obtain epoch length", "err", err)
					continue
				}
				currentEpoch, err := e.repository.GetEpoch(ctx,
					epochLength, appAddress)
				if err != nil {
					slog.Error("could not obtain current epoch", "err", err)
					continue
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
				sigHashHexBytes, err := hex.DecodeString(sigHash[2:])
				if err != nil {
					slog.Error("could not obtain bytes for tx-id", "err", err)
					continue
				}
				input := model.Input{
					CompletionStatus: model.InputStatusNone,
					RawData:          payloadAbi,
					BlockNumber:      l1FinalizedCurrentHeight,
					AppAddress:       appAddress,
					TransactionId:    sigHashHexBytes,
				}
				currentInputs, ok := epochInputMap[currentEpoch]
				if !ok {
					currentInputs = []model.Input{}
				}
				epochInputMap[currentEpoch] = append(currentInputs, input)

				// Store everything
				// future optimization: bundle tx by address to fully utilize `epochInputMap``
				if len(epochInputMap) > 0 {
					_, _, err = e.repository.StoreEpochAndInputsTransaction(
						ctx,
						epochInputMap,
						l1FinalizedCurrentHeight,
						appAddress,
					)
					if err != nil {
						slog.Error("could not store Espresso input", "err", err)
						continue
					}
				}

				// update nonce
				err = e.repository.UpdateEspressoNonce(ctx, msgSender, appAddress)
				if err != nil {
					slog.Error("!!!could not update Espresso nonce!!!", "err", err)
					continue
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

func (e *EspressoReader) getL1FinalizedHeight(espressoBlockHeight uint64) (uint64, uint64) {
	espressoHeader := e.readEspressoHeader(espressoBlockHeight)

	l1FinalizedNumber := gjson.Get(espressoHeader, "fields.l1_finalized.number").Uint()

	l1FinalizedTimestampStr := gjson.Get(espressoHeader, "fields.l1_finalized.timestamp").Str
	l1FinalizedTimestampInt, err := strconv.ParseInt(l1FinalizedTimestampStr[2:], 16, 64)
	if err != nil {
		slog.Error("hex to int conversion failed", "err", err)
		os.Exit(1)
	}
	l1FinalizedTimestamp := uint64(l1FinalizedTimestampInt)
	return l1FinalizedNumber, l1FinalizedTimestamp
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
		applicationContract, consensusContract, err := e.evmReader.GetAppContracts(app)
		if err != nil {
			slog.Error("Error retrieving application contracts", "app", app, "error", err)
			continue
		}
		apps = append(apps, evmreader.TypeExportApplication{Application: app,
			ApplicationContract: applicationContract,
			ConsensusContract:   consensusContract})
	}

	if len(apps) == 0 {
		slog.Info("No correctly configured applications running")
	}

	return apps
}

func readPrevRandao(ctx context.Context, l1FinalizedCurrentHeight uint64, client *evmreader.EthClient) (*big.Int, error) {
	header, err := (*client).HeaderByNumber(ctx, big.NewInt(int64(l1FinalizedCurrentHeight)))
	if err != nil {
		return &big.Int{}, fmt.Errorf("espresso read block header error: %w", err)
	}
	prevRandao := header.MixDigest.Big()
	slog.Debug("readPrevRandao", "prevRandao", prevRandao, "blockNumber", l1FinalizedCurrentHeight)
	return prevRandao, nil
}
