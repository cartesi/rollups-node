// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
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
	computed, accepted, err := s.DBConn.SelectClaimPairsPerApp(s.Context)
	if err != nil {
		s.Logger.Error("selectClaimPairsPerApp:failed",
			"service", s.Name,
			"error", err)
	} else {
		s.Logger.Debug("selectClaimPairsPerApp:success",
			"service", s.Name,
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
	err := s.DBConn.UpdateEpochWithSubmittedClaim(s.Context, claim.EpochID, txHash)
	if err != nil {
		s.Logger.Error("updateEpochWithSubmittedClaim:failed",
			"service", s.Name,
			"appContractAddress", claim.AppContractAddress,
			"hash", claim.EpochHash,
			"txHash", txHash,
			"error", err)
	} else {
		s.Logger.Debug("updateEpochWithSubmittedClaim:success",
			"service", s.Name,
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
	ic, curr, next, err := s.FindClaimSubmissionEventAndSucc(claim)
	if err != nil {
		s.Logger.Error("findClaimSubmissionEventAndSucc:failed",
			"service", s.Name,
			"claim", claim,
			"error", err)
	} else {
		s.Logger.Debug("findClaimSubmissionEventAndSucc:success",
			"service", s.Name,
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
			"service", s.Name,
			"appContractAddress", claim.AppContractAddress,
			"claimHash", claim.EpochHash,
			"error", err)
	} else {
		txHash = tx.Hash()
		s.Logger.Debug("submitClaimToBlockchain:success",
			"service", s.Name,
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
			"service", s.Name,
			"tx", txHash,
			"error", err)
	} else if ready {
		s.Logger.Debug("PollTransaction:success",
			"service", s.Name,
			"tx", txHash,
			"ready", ready,
			"blockNumber", receipt.BlockNumber)
	} else {
		s.Logger.Debug("PollTransaction:pending",
			"service", s.Name,
			"tx", txHash,
			"ready", ready)
	}
	return ready, receipt, err
}

// scan the event stream for a claimSubmission event that matches claim.
// return this event and its successor
func (s *Service) FindClaimSubmissionEventAndSucc(
	claim *claimRow,
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

	it, err := ic.FilterClaimSubmission(&bind.FilterOpts{
		Context: s.Context,
		Start:   claim.EpochLastBlock,
	}, nil, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	for it.Next() {
		event := it.Event
		lastBlock := event.LastProcessedBlockNumber.Uint64()
		if claimMatchesEvent(claim, event) {
			var succ *claimSubmissionEvent = nil
			if it.Next() {
				succ = it.Event
			}
			if it.Error() != nil {
				return nil, nil, nil, it.Error()
			}
			return ic, event, succ, nil
		} else if lastBlock > claim.EpochLastBlock {
			err = fmt.Errorf("claim not found, searched up to %v", event)
		}
	}
	if it.Error() != nil {
		return nil, nil, nil, it.Error()
	}
	return ic, nil, nil, nil
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
