// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/cobra"
)

const (
	LOCALHOST_URL  = "http://localhost:8545"
	CMD_NAME       = "input-reader"
	SLEEP_INTERVAL = 5
)

var Cmd = &cobra.Command{
	Use:   CMD_NAME,
	Short: "Reads inputs from RPC endpoint for a limited amount of time",
	Run:   run,
}

var (
	rpcEndpoint string
	startBlock  int
	timeout     int
	verboseLog  bool
)

func init() {
	Cmd.Flags().StringVarP(&rpcEndpoint,
		"rpc-endpoint",
		"r",
		LOCALHOST_URL,
		"RCP endpopint to be queried for inputs")
	Cmd.Flags().IntVarP(&startBlock,
		"start-block",
		"s",
		20,
		"starting block number")
	Cmd.Flags().IntVarP(&timeout,
		"timeout",
		"t",
		60,
		"timeout in seconds")
	Cmd.Flags().BoolVarP(&verboseLog,
		"verbose",
		"v",
		false,
		"enable verbose logging")
}

func main() {
	err := Cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	info("program timeout: %vs", timeout)
	readInputs(ctx, rpcEndpoint, big.NewInt(20))
}

func info(format string, a ...any) (n int, err error) {
	return fmt.Printf("info> "+format+"\n", a...)
}
func debug(format string, a ...any) (n int, err error) {
	if verboseLog {
		return fmt.Printf("debug> "+format+"\n", a...)
	}
	return 0, nil
}

func readInputs(ctx context.Context, url string, startingBlockNumber *big.Int) {
	client, err := ethclient.DialContext(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	info("client connected to %s", url)

	inputBox, err := contracts.NewInputBox(addresses.GetTestBook().InputBox, client)
	if err != nil {
		log.Fatal(err)
	}
	debug("contract bind succeeded")

	iteratorChannel := make(chan *contracts.InputBoxInputAddedIterator)
	// Start polling goroutine
	go func() {
		retry := false
		fromBlockNumber := startingBlockNumber
		toBlockNumber := startingBlockNumber
		filterOpts := new(bind.FilterOpts)
		filterOpts.Context = ctx

		for {
			if !retry {
				// XXX For some reason, infura's https endpoint always return a nil header
				latestFinalizedBlock, err :=
					client.HeaderByNumber(
						ctx,
						new(big.Int).SetInt64(rpc.FinalizedBlockNumber.Int64()))

				if err != nil {
					info("failed to retrieve latest finalized block. %v", err)
					retry = true
					time.Sleep(SLEEP_INTERVAL * time.Second)
					continue
				}

				if latestFinalizedBlock == nil {
					info("latest finalized block was invalid")
					time.Sleep(SLEEP_INTERVAL * time.Second)
					continue
				} else if latestFinalizedBlock.Number.Cmp(toBlockNumber) == 0 {
					info("latest finalized block still the same (%v)", toBlockNumber)
					time.Sleep(SLEEP_INTERVAL * time.Second)
					continue
				}

				toBlockNumber = latestFinalizedBlock.Number
			}

			//TODO check whether from is less than to or let query function fail?

			filterOpts.Start = fromBlockNumber.Uint64()
			toBlockUint64 := toBlockNumber.Uint64()
			filterOpts.End = &toBlockUint64 // XXX

			// Only valid on localhost
			//dapp := []common.Address{addresses.GetTestBook().CartesiDApp}
			info("querying blocks %v to %v", filterOpts.Start, *filterOpts.End)

			// Query all DApps
			iterator, err := inputBox.InputBoxFilterer.FilterInputAdded(filterOpts, nil, nil)
			if err != nil {
				log.Fatal(err)
				//retry = true
				//continue
				// TODO perhaps finish the goroutine and send a corresponding signal to an error channel?
				// It might be the case that the block numbers are wrong in the first try but not in the second one
				// Say fromBlockNumber is set to be 20 but the latest finalized block is 15.
				// Eventually, after a few loops, finalized will be greater than 20 and it should be fine to continue.
				// So we could set a limit for retries that could be used in
				// this case and when we get any error (some cases might not aplly, let's see)
			}

			iteratorChannel <- iterator

			fromBlockNumber.Add(toBlockNumber, common.Big1)
			time.Sleep(SLEEP_INTERVAL * time.Second)
		}
	}()

	for {
		select {
		case iterator := <-iteratorChannel:
			defer iterator.Close()
			i := 0
			for iterator.Next() {
				debug("event[%v] = {dapp: %v, inputIndex: %v, sender: %v, input: %v}",
					i,
					iterator.Event.Dapp,
					iterator.Event.InputIndex,
					iterator.Event.Sender,
					iterator.Event.Input)
				i += 1
			}
			info("found %v events", i)
		case <-ctx.Done():
			info("finished")
			return
		}
	}
}
