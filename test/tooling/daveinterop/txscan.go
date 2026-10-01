// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
)

const selectorLength = 4

// signerTx is one transaction that a watched signer sent.
type signerTx struct {
	Hash   common.Hash `json:"transaction_hash"`
	Block  uint64      `json:"block"`
	Method string      `json:"method"`
	Failed bool        `json:"failed"`
	// GasUsed is the receipt's; Gas is its cost in wei, which on a test
	// chain with empty blocks says little, because the base fee decays.
	GasUsed uint64   `json:"gas_used"`
	Gas     *big.Int `json:"gas_cost"`
	Revert  string   `json:"revert,omitempty"`
}

// contractABIs are the contracts whose calls and errors the scan names.
var contractABIs = []struct {
	name string
	meta func() (*abi.ABI, error)
}{
	{"DaveConsensus", idaveconsensus.IDaveConsensusMetaData.GetAbi},
	{"Tournament", itournament.ITournamentMetaData.GetAbi},
	{"Application", iapplication.IApplicationMetaData.GetAbi},
	{"InputBox", iinputbox.IInputBoxMetaData.GetAbi},
}

// methodName names the call in transaction data, for example
// "DaveConsensus.stageTournamentResult".
func methodName(data []byte) string {
	if len(data) == 0 {
		return "transfer"
	}
	for _, contract := range contractABIs {
		parsed, err := contract.meta()
		if err != nil {
			continue
		}
		for _, method := range parsed.Methods {
			if len(method.ID) > 0 && strings.HasPrefix(string(data), string(method.ID)) {
				return contract.name + "." + method.Name
			}
		}
	}
	return fmt.Sprintf("unknown 0x%x", data[:min(selectorLength, len(data))])
}

// errorName decodes a custom error by its selector, for example
// "ApplicationForeclosed(0x…)".
func errorName(data []byte) string {
	if len(data) < selectorLength {
		return "revert without data"
	}
	for _, contract := range contractABIs {
		parsed, err := contract.meta()
		if err != nil {
			continue
		}
		for _, abiError := range parsed.Errors {
			if !strings.HasPrefix(string(data), string(abiError.ID[:selectorLength])) {
				continue
			}
			args, err := abiError.Inputs.Unpack(data[selectorLength:])
			if err != nil || len(args) == 0 {
				return abiError.Name
			}
			parts := make([]string, 0, len(args))
			for _, arg := range args {
				parts = append(parts, fmt.Sprint(arg))
			}
			return abiError.Name + "(" + strings.Join(parts, ", ") + ")"
		}
	}
	return fmt.Sprintf("error 0x%x", data[:selectorLength])
}

// transactionsFrom lists the transactions that the signers sent in blocks
// (from, to], with their outcome.
func (c *chain) transactionsFrom(ctx context.Context, signers []common.Address, from, to uint64,
) (map[common.Address][]signerTx, error) {
	watched := map[common.Address]bool{}
	for _, signer := range signers {
		watched[signer] = true
	}
	chainID, err := c.chainID(ctx)
	if err != nil {
		return nil, err
	}
	txSigner := types.LatestSignerForChainID(new(big.Int).SetUint64(chainID))
	found := map[common.Address][]signerTx{}
	for number := from + 1; number <= to; number++ {
		block, err := c.eth.BlockByNumber(ctx, new(big.Int).SetUint64(number))
		if err != nil {
			return nil, fmt.Errorf("reading block %d: %w", number, err)
		}
		for _, tx := range block.Transactions() {
			sender, err := types.Sender(txSigner, tx)
			if err != nil || !watched[sender] {
				continue
			}
			receipt, err := c.eth.TransactionReceipt(ctx, tx.Hash())
			if err != nil {
				return nil, fmt.Errorf("reading the receipt of %s: %w", tx.Hash(), err)
			}
			entry := signerTx{Hash: tx.Hash(), Block: number, Method: methodName(tx.Data()),
				Failed: receipt.Status != types.ReceiptStatusSuccessful, GasUsed: receipt.GasUsed, Gas: gasCost(receipt)}
			if entry.Failed {
				entry.Revert = c.revertReason(ctx, sender, tx, number)
			}
			found[sender] = append(found[sender], entry)
		}
	}
	return found, nil
}

// revertReason replays a failed transaction as a call on the state before its
// block and decodes the error. It is exact for the first transaction of the
// block and close enough for a signer whose calls revert in a modifier.
func (c *chain) revertReason(ctx context.Context, from common.Address, tx *types.Transaction, block uint64) string {
	_, err := c.eth.CallContract(ctx, ethereum.CallMsg{From: from, To: tx.To(), Gas: tx.Gas(), Value: tx.Value(),
		Data: tx.Data()}, new(big.Int).SetUint64(block-1))
	var dataErr rpc.DataError
	if err == nil {
		return "no revert when replayed"
	}
	if !errors.As(err, &dataErr) {
		return err.Error()
	}
	hexData, ok := dataErr.ErrorData().(string)
	if !ok {
		return err.Error()
	}
	data, decodeErr := hexutil.Decode(hexData)
	if decodeErr != nil {
		return err.Error()
	}
	return errorName(data)
}

// txSummary groups a signer's transactions and totals the reverted ones.
type txSummary struct {
	count, failed int
	gasUsed       uint64
	cost          *big.Int
	// text lists the calls, for example
	// "DaveConsensus.stageTournamentResult reverted with ApplicationForeclosed ×3; Tournament.joinTournament ×1".
	text string
}

func summarizeTxs(txs []signerTx) txSummary {
	summary := txSummary{count: len(txs), cost: new(big.Int)}
	counts := map[string]int{}
	for _, tx := range txs {
		key := tx.Method
		if tx.Failed {
			summary.failed++
			summary.gasUsed += tx.GasUsed
			summary.cost.Add(summary.cost, tx.Gas)
			key += " reverted with " + strings.SplitN(tx.Revert, "(", 2)[0] //nolint:mnd // name before the arguments
		}
		counts[key]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s ×%d", key, counts[key]))
	}
	summary.text = strings.Join(parts, "; ")
	return summary
}

// revertedText is, for example, "21 of 23 transactions reverted, using
// 504,000 gas (24,000 each on average; 0.0000047 ETH at this chain's fees)".
func (t txSummary) revertedText() string {
	return fmt.Sprintf("%d of %s reverted, using %s gas (%s each on average; %s at this chain's fees)", t.failed,
		countText(t.count, "transaction"), groupDigits(t.gasUsed), groupDigits(t.gasUsed/uint64(max(t.failed, 1))),
		formatWei(t.cost))
}

// groupDigits writes 1234567 as "1,234,567".
func groupDigits(n uint64) string {
	digits := strconv.FormatUint(n, 10)
	var b strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(digit)
	}
	return b.String()
}
