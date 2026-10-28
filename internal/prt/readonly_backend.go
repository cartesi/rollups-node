// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Generated bindings are readers and event decoders. Publication belongs to
// txsender; refusing SendTransaction prevents accidental binding publication.
type readOnlyBackend struct{ *ethclient.Client }

func (b readOnlyBackend) CallContractAtHash(ctx context.Context, msg ethereum.CallMsg, hash common.Hash) ([]byte, error) {
	return (txsender.ProtectedClient{Client: b.Client}).CallContractAtHash(ctx, msg, hash)
}
func (b readOnlyBackend) CodeAtHash(ctx context.Context, addr common.Address, hash common.Hash) ([]byte, error) {
	return (txsender.ProtectedClient{Client: b.Client}).CodeAtHash(ctx, addr, hash)
}
func (b readOnlyBackend) SendTransaction(context.Context, *types.Transaction) error {
	return errors.New("PRT bindings cannot publish transactions")
}
