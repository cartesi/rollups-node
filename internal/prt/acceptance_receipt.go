// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Receipt RPCs cannot be pinned to the observation hash. Verify their block
// against the canonical header before using their events for durable decisions.
// The caller separately requires the receipt height to be observed already.
func (s *Service) validateAcceptanceReceipt(ctx context.Context, receipt *types.Receipt) error {
	if receipt.BlockHash == (common.Hash{}) {
		return fmt.Errorf("%w: acceptance receipt has no block hash", txsender.ErrInconclusive)
	}
	header, err := s.client.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		if errutil.IsOnlyCancellation(err) {
			return err
		}
		return fmt.Errorf("reading acceptance receipt's canonical header: %w", errors.Join(txsender.ErrInconclusive, err))
	}
	if header == nil || header.Number == nil || !header.Number.IsUint64() || header.Number.Cmp(receipt.BlockNumber) != 0 {
		return fmt.Errorf("%w: acceptance receipt's canonical header is unavailable or has a different height", txsender.ErrInconclusive)
	}
	if header.Hash() != receipt.BlockHash {
		return fmt.Errorf("%w: acceptance receipt block hash differs from the canonical header", txsender.ErrInconclusive)
	}
	return nil
}
