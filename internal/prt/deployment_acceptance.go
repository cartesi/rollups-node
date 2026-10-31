// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Receipt and event checks establish observed acceptance evidence.
// A transaction pointer alone is never validated acceptance knowledge.
func (s *Service) readEpochAcceptance(ctx context.Context, app *model.Application, epoch *model.Epoch,
	consensus DaveConsensusAdapter, observedBlock uint64,
) (*idaveconsensus.IDaveConsensusEpochSealed, error) {
	if epoch.ClaimTransactionHash == nil {
		return nil, nil
	}
	receipt, err := s.client.TransactionReceipt(ctx, *epoch.ClaimTransactionHash)
	if err != nil {
		s.logErrorUnlessShutdown(ctx, "failed to fetch transaction receipt for epoch", err, "application", app.Name,
			"epoch", epoch.Index, "tx", epoch.ClaimTransactionHash)
		return nil, err
	}
	if receipt == nil {
		return nil, fmt.Errorf("epoch %d: acceptance transaction receipt is nil", epoch.Index)
	}
	if receipt.TxHash != *epoch.ClaimTransactionHash {
		return nil, fmt.Errorf("epoch %d: acceptance receipt transaction hash %s differs from observed hash %s",
			epoch.Index, receipt.TxHash, epoch.ClaimTransactionHash)
	}
	receiptBlock, err := checkedUint64(receipt.BlockNumber, "acceptance receipt block")
	if err != nil {
		return nil, fmt.Errorf("epoch %d: %w", epoch.Index, err)
	}
	if receiptBlock > observedBlock {
		s.Logger.Debug("Acceptance transaction is newer than the published tournament window",
			"application", app.Name, "epoch", epoch.Index, "tx", epoch.ClaimTransactionHash,
			"receipt_block", receiptBlock, "snapshot_block", observedBlock)
		return nil, nil
	}
	if err := s.validateAcceptanceReceipt(ctx, receipt); err != nil {
		return nil, fmt.Errorf("epoch %d: %w", epoch.Index, err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("epoch %d: EpochSealed transaction hash points to failed transaction", epoch.Index)
	}
	expectedEpoch := new(big.Int).Add(new(big.Int).SetUint64(epoch.Index), common.Big1)
	for _, log := range receipt.Logs {
		if log == nil || log.Address != app.IConsensusAddress || log.TxHash != *epoch.ClaimTransactionHash ||
			log.BlockNumber != receiptBlock || log.BlockHash != receipt.BlockHash || log.Removed {
			continue
		}
		event, parseErr := consensus.ParseEpochSealed(*log)
		if parseErr != nil || event == nil || event.EpochNumber == nil || event.EpochNumber.Cmp(expectedEpoch) != 0 {
			continue
		}
		return event, nil
	}
	return nil, fmt.Errorf("%w: epoch %d: failed to find EpochSealed event in receipt logs", txsender.ErrInconclusive, epoch.Index)
}
