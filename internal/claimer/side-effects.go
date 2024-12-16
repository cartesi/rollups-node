// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"iter"
	"math"
	"math/big"
	"os"

	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type sideEffects interface {
	// database
	selectClaimPairsPerApp() (
		map[address]claimRow,
		map[address]claimRow,
		error,
	)
	updateEpochWithSubmittedClaim(
		claim *claimRow,
		txHash hash,
	) error

	// blockchain
	findClaimSubmissionEventAndSucc(
		claim *claimRow,
	) (
		*iconsensus.IConsensus,
		*claimSubmissionEvent,
		*claimSubmissionEvent,
		error,
	)
	submitClaimToBlockchain(
		ic *iconsensus.IConsensus,
		claim *claimRow,
	) (
		hash,
		error,
	)
	pollTransaction(txHash hash) (
		bool,
		*types.Receipt,
		error,
	)
}

func (s *Service) selectClaimPairsPerApp() (
	map[address]claimRow,
	map[address]claimRow,
	error,
) {
	computed, accepted, err := s.Repository.SelectClaimPairsPerApp(s.Context)
	if err != nil {
		s.Logger.Error("selectClaimPairsPerApp:failed",
			"error", err)
	} else {
		s.Logger.Debug("selectClaimPairsPerApp:success",
			"len(computed)", len(computed),
			"len(accepted)", len(accepted))
	}
	return accepted, computed, err
}

/* update the database epoch status to CLAIM_SUBMITTED and add a transaction hash */
func (s *Service) updateEpochWithSubmittedClaim(
	claim *claimRow,
	txHash hash,
) error {
	err := s.Repository.UpdateEpochWithSubmittedClaim(s.Context, claim.EpochID, txHash)
	if err != nil {
		s.Logger.Error("updateEpochWithSubmittedClaim:failed",
			"appContractAddress", claim.AppContractAddress,
			"hash", claim.EpochHash,
			"txHash", txHash,
			"error", err)
	} else {
		s.Logger.Debug("updateEpochWithSubmittedClaim:success",
			"appContractAddress", claim.AppContractAddress,
			"hash", claim.EpochHash,
			"txHash", txHash)
	}
	return err
}

func (s *Service) findClaimSubmissionEventAndSucc(
	claim *claimRow,
) (
	*iconsensus.IConsensus,
	*claimSubmissionEvent,
	*claimSubmissionEvent,
	error,
) {
	ic, curr, next, err := s.FindClaimSubmissionEventAndSucc(claim, s.chunkSize)
	if err != nil {
		s.Logger.Error("findClaimSubmissionEventAndSucc:failed",
			"claim", claim,
			"error", err)
	} else {
		s.Logger.Debug("findClaimSubmissionEventAndSucc:success",
			"claim", claim,
			"currEvent", curr,
			"nextEvent", next,
		)
	}
	return ic, curr, next, err
}

func (s *Service) submitClaimToBlockchain(
	ic *iconsensus.IConsensus,
	claim *claimRow,
) (hash, error) {
	txHash := hash{}
	lastBlockNumber := new(big.Int).SetUint64(claim.EpochLastBlock)
	tx, err := ic.SubmitClaim(s.TxOpts, claim.AppContractAddress,
		lastBlockNumber, claim.EpochHash)
	if err != nil {
		s.Logger.Error("submitClaimToBlockchain:failed",
			"appContractAddress", claim.AppContractAddress,
			"claimHash", claim.EpochHash,
			"error", err)
	} else {
		txHash = tx.Hash()
		s.Logger.Debug("submitClaimToBlockchain:success",
			"appContractAddress", claim.AppContractAddress,
			"claimHash", claim.EpochHash,
			"TxHash", txHash)
	}
	return txHash, err
}

func (s *Service) pollTransaction(txHash hash) (bool, *types.Receipt, error) {
	ready, receipt, err := s.PollTransaction(txHash)
	if err != nil {
		s.Logger.Error("PollTransaction:failed",
			"tx", txHash,
			"error", err)
	} else if ready {
		s.Logger.Debug("PollTransaction:success",
			"tx", txHash,
			"ready", ready,
			"blockNumber", receipt.BlockNumber)
	} else {
		s.Logger.Debug("PollTransaction:pending",
			"tx", txHash,
			"ready", ready)
	}
	return ready, receipt, err
}

func chunkedFindSubmissionEvent(
	ctx context.Context,
	ic *iconsensus.IConsensus,
	client *ethclient.Client,
	chunk uint64,
	start uint64,
	end uint64,
) (iter.Seq2[*claimSubmissionEvent, error], error) {
	var err error

	if chunk == 0 || end < start {
		return nil, os.ErrInvalid
	}
	if end == math.MaxUint64 {
		end, err = client.BlockNumber(ctx)
		if err != nil {
			return nil, err
		}
	}

	return func(yield func(*claimSubmissionEvent, error) bool) {
		for start < end {
			// open range, othewise we get duplicates
			limit := min(start+chunk-1, end)
			it, err := ic.FilterClaimSubmission(&bind.FilterOpts{
				Context: ctx,
				Start:   start,
				End:     &limit,
			}, nil, nil)
			if err != nil {
				yield(nil, err)
				return
			}

			for it.Next() {
				yield(it.Event, nil)
			}
			if it.Error() != nil {
				yield(nil, err)
				return
			}

			start = limit + 1
		}
	}, nil
}

// scan the event stream for a claimSubmission event that matches claim.
// return this event and its successor
func (s *Service) FindClaimSubmissionEventAndSucc(
	claim *claimRow,
	chunkSize uint64,
) (
	*iconsensus.IConsensus,
	*claimSubmissionEvent,
	*claimSubmissionEvent,
	error,
) {
	ic, err := iconsensus.NewIConsensus(claim.AppIConsensusAddress, s.EthConn)
	if err != nil {
		return nil, nil, nil, err
	}

	it, err := chunkedFindSubmissionEvent(s.Context, ic, s.EthConn,
		chunkSize, claim.EpochLastBlock, math.MaxUint64)
	if err != nil {
		return nil, nil, nil, err
	}
	next, stop := iter.Pull2(it)
	defer stop()

	for {
		event, err, ok := next()
		if err != nil || !ok {
			return ic, nil, nil, err
		}
		lastBlock := event.LastProcessedBlockNumber.Uint64()

		if claimMatchesEvent(claim, event) {
			succ, err, ok := next()
			if err != nil || !ok {
				return ic, event, nil, err
			}
			return ic, event, succ, err
		} else if lastBlock > claim.EpochLastBlock {
			err = fmt.Errorf("claim not found, searched up to %v", event)
			return nil, nil, nil, err
		}
	}
}

/* poll a transaction hash for its submission status and receipt */
func (s *Service) PollTransaction(txHash hash) (bool, *types.Receipt, error) {
	_, isPending, err := s.EthConn.TransactionByHash(s.Context, txHash)
	if err != nil || isPending {
		return false, nil, err
	}

	receipt, err := s.EthConn.TransactionReceipt(s.Context, txHash)
	if err != nil {
		return false, nil, err
	}

	return receipt.Status == 1, receipt, err
}
