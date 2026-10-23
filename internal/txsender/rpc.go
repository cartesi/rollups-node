// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// ProtectedClient adapts the existing protected HTTP client. EIP-1898 reads
// explicitly require canonicality; geth's AtHash helpers do not.
type ProtectedClient struct{ Client *ethclient.Client }

func blockArg(b BlockRef) rpc.BlockNumberOrHash {
	return rpc.BlockNumberOrHashWithHash(b.Hash, true)
}

// A missing method or explicit rejection of the hash-selector schema is a hard
// capability refusal. Some providers also use invalid-params for missing block
// state, so the code alone cannot identify unsupported EIP-1898 reads. Ambiguous
// failures stay inconclusive. No execution-revert message text is consulted.
func capabilityError(err error) error {
	if err == nil {
		return nil
	}
	if errutil.IsOnlyCancellation(err) {
		return err
	}
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) && (rpcErr.ErrorCode() == -32601 ||
		(rpcErr.ErrorCode() == -32602 && rejectsHashSelector(rpcErr.Error()))) {
		return errors.Join(ErrUnsupported, err)
	}
	return inconclusive(err)
}

// This allowlist applies only to account reads, whose sole object parameter is
// the block selector. An unknown error may delay progress but cannot relax the
// required hash and canonicality fields or trigger a permanent capability latch.
func rejectsHashSelector(message string) bool {
	message = strings.ToLower(message)
	if message == "object block argument unsupported" {
		return true
	}
	for _, field := range []string{"requirecanonical", "blockhash"} {
		if strings.Contains(message, field) && (strings.Contains(message, "unknown field") ||
			strings.Contains(message, "unknown parameter") || strings.Contains(message, "not supported") ||
			strings.Contains(message, "unsupported")) {
			return true
		}
	}
	if strings.Contains(message, "invalid type: map, expected a string") ||
		strings.Contains(message, "invalid type: map, expected a block number") {
		return true
	}
	return strings.Contains(message, "argument 1") &&
		strings.Contains(message, "cannot unmarshal object into go value of type") &&
		(strings.Contains(message, "type string") || strings.Contains(message, "type hexutil.uint64") ||
			strings.Contains(message, "type rpc.blocknumber"))
}

func (c ProtectedClient) Header(ctx context.Context, tag string) (*types.Header, error) {
	var h *types.Header
	err := c.Client.Client().CallContext(ctx, &h, "eth_getBlockByNumber", tag, false)
	if err != nil {
		return nil, err
	}
	if h == nil || h.Number == nil || !h.Number.IsUint64() {
		return nil, ErrInconclusive
	}
	return h, nil
}

func (c ProtectedClient) Canonical(ctx context.Context, b BlockRef) (bool, error) {
	h, err := c.Header(ctx, hexutil.EncodeUint64(b.Number))
	if err != nil {
		return false, err
	}
	return h.Hash() == b.Hash, nil
}

func (c ProtectedClient) Nonce(ctx context.Context, a common.Address, b BlockRef) (uint64, error) {
	var n hexutil.Uint64
	err := c.Client.Client().CallContext(ctx, &n, "eth_getTransactionCount", a, blockArg(b))
	return uint64(n), capabilityError(err)
}

func (c ProtectedClient) Balance(ctx context.Context, a common.Address, b BlockRef) (*big.Int, error) {
	var n hexutil.Big
	err := c.Client.Client().CallContext(ctx, &n, "eth_getBalance", a, blockArg(b))
	return new(big.Int).Set((*big.Int)(&n)), capabilityError(err)
}

func (c ProtectedClient) Code(ctx context.Context, a common.Address, b BlockRef) ([]byte, error) {
	var data hexutil.Bytes
	err := c.Client.Client().CallContext(ctx, &data, "eth_getCode", a, blockArg(b))
	return data, capabilityError(err)
}

func callArg(m ethereum.CallMsg) map[string]any {
	a := map[string]any{"from": m.From}
	// Binding views and sizing hints leave gas unspecified. Exact transaction
	// simulations supply their selected nonzero gas limit explicitly.
	if m.Gas != 0 {
		a["gas"] = hexutil.Uint64(m.Gas)
	}
	if m.To != nil {
		a["to"] = *m.To
	}
	if m.Value != nil {
		a["value"] = (*hexutil.Big)(m.Value)
	}
	if m.Data != nil {
		a["input"] = hexutil.Bytes(m.Data)
	}
	if m.GasPrice != nil {
		a["gasPrice"] = (*hexutil.Big)(m.GasPrice)
	}
	if m.GasFeeCap != nil {
		a["maxFeePerGas"] = (*hexutil.Big)(m.GasFeeCap)
	}
	if m.GasTipCap != nil {
		a["maxPriorityFeePerGas"] = (*hexutil.Big)(m.GasTipCap)
	}
	if m.AccessList != nil {
		a["accessList"] = m.AccessList
	}
	return a
}

func (c ProtectedClient) Call(ctx context.Context, m ethereum.CallMsg, b BlockRef) ([]byte, error) {
	var data hexutil.Bytes
	err := c.Client.Client().CallContext(ctx, &data, "eth_call", callArg(m), blockArg(b))
	return data, err
}

func (c ProtectedClient) Estimate(ctx context.Context, m ethereum.CallMsg, b BlockRef) (uint64, error) {
	var n hexutil.Uint64
	// Sizing does not authorize a send. Exact calls and fresh canonicality
	// checks remain hash-pinned, including after a same-height reorg.
	err := c.Client.Client().CallContext(ctx, &n, "eth_estimateGas", callArg(m), hexutil.EncodeUint64(b.Number))
	if err != nil {
		return 0, fmt.Errorf("eth_estimateGas at block %d: %w", b.Number, err)
	}
	return uint64(n), err
}

func (c ProtectedClient) Tip(ctx context.Context) (*big.Int, error) {
	return c.Client.SuggestGasTipCap(ctx)
}
func (c ProtectedClient) Price(ctx context.Context) (*big.Int, error) {
	return c.Client.SuggestGasPrice(ctx)
}
func (c ProtectedClient) Publish(ctx context.Context, raw []byte) (common.Hash, error) {
	var hash common.Hash
	err := c.Client.Client().CallContext(ctx, &hash, "eth_sendRawTransaction", hexutil.Bytes(raw))
	return hash, err
}

// These methods implement the hash-capable read-binding interface. Numeric
// reads remain available for existing event reconciliation at its selected policy.
func (c ProtectedClient) CallContract(ctx context.Context, m ethereum.CallMsg, n *big.Int) ([]byte, error) {
	return c.Client.CallContract(ctx, m, n)
}
func (c ProtectedClient) CodeAt(ctx context.Context, a common.Address, n *big.Int) ([]byte, error) {
	return c.Client.CodeAt(ctx, a, n)
}
func (c ProtectedClient) CallContractAtHash(ctx context.Context, m ethereum.CallMsg, h common.Hash) ([]byte, error) {
	return c.Call(ctx, m, BlockRef{Hash: h})
}
func (c ProtectedClient) CodeAtHash(ctx context.Context, a common.Address, h common.Hash) ([]byte, error) {
	return c.Code(ctx, a, BlockRef{Hash: h})
}

func headerRef(h *types.Header) (BlockRef, error) {
	if h == nil || h.Number == nil || !h.Number.IsUint64() {
		return BlockRef{}, fmt.Errorf("%w: missing header", ErrInconclusive)
	}
	return BlockRef{Number: h.Number.Uint64(), Hash: h.Hash()}, nil
}
