// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"fmt"
	"iter"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

type sideEffects interface {
	// database
	selectClaimSubmissionCandidatePairsPerApp() (
		map[common.Address]*ClaimRow,
		map[common.Address]*ClaimRow,
		error,
	)
	updateEpochWithSubmittedClaim(
		claim *ClaimRow,
		txHash common.Hash,
	) error

	// blockchain
	findClaimSubmissionEventAndSucc(
		claim *ClaimRow,
	) (
		*iconsensus.IConsensus,
		*iconsensus.IConsensusClaimSubmission,
		*iconsensus.IConsensusClaimSubmission,
		error,
	)
	submitClaimToBlockchain(
		ic *iconsensus.IConsensus,
		claim *ClaimRow,
	) (
		common.Hash,
		error,
	)
	pollTransaction(txHash common.Hash) (
		bool,
		*types.Receipt,
		error,
	)
}

func (s *Service) selectClaimSubmissionCandidatePairsPerApp() (
	map[common.Address]*ClaimRow,
	map[common.Address]*ClaimRow,
	error,
) {
	computed, accepted, err := s.Repository.SelectClaimSubmissionCandidatePairsPerApp(s.Context)
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
	claim *ClaimRow,
	txHash common.Hash,
) error {
	err := s.Repository.UpdateEpochWithSubmittedClaim(s.Context, claim.ApplicationID, claim.Index, txHash)
	if err != nil {
		s.Logger.Error("updateEpochWithSubmittedClaim:failed",
			"appContractAddress", claim.IApplicationAddress,
			"hash", claim.ClaimHash,
			"last_block", claim.LastBlock,
			"txHash", txHash,
			"error", err)
	} else {
		s.Logger.Debug("updateEpochWithSubmittedClaim:success",
			"appContractAddress", claim.IApplicationAddress,
			"last_block", claim.LastBlock,
			"hash", claim.ClaimHash,
			"txHash", txHash)
	}
	return err
}

func (s *Service) findClaimSubmissionEventAndSucc(
	claim *ClaimRow,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimSubmission,
	*iconsensus.IConsensusClaimSubmission,
	error,
) {
	ic, curr, next, err := s.FindClaimSubmissionEventAndSucc(claim)
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
	claim *ClaimRow,
) (common.Hash, error) {
	txHash := common.Hash{}
	lastBlockNumber := new(big.Int).SetUint64(claim.LastBlock)
	tx, err := ic.SubmitClaim(s.TxOpts, claim.IApplicationAddress,
		lastBlockNumber, *claim.ClaimHash)
	if err != nil {
		s.Logger.Error("submitClaimToBlockchain:failed",
			"appContractAddress", claim.IApplicationAddress,
			"claimHash", *claim.ClaimHash,
			"last_block", claim.LastBlock,
			"error", err)
	} else {
		txHash = tx.Hash()
		s.Logger.Debug("submitClaimToBlockchain:success",
			"appContractAddress", claim.IApplicationAddress,
			"claimHash", *claim.ClaimHash,
			"last_block", claim.LastBlock,
			"TxHash", txHash)
	}
	return txHash, err
}

func (s *Service) pollTransaction(txHash common.Hash) (bool, *types.Receipt, error) {
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

func unwrapClaimSubmission(
	ic *iconsensus.IConsensus,
	pull func() (log *types.Log, err error, ok bool),
) (
	*iconsensus.IConsensusClaimSubmission,
	bool,
	error,
) {
	log, err, ok := pull()
	if !ok || err != nil {
		return nil, false, err
	}
	ev, err := ic.ParseClaimSubmission(*log)
	return ev, true, err
}

// scan the event stream for a claimSubmission event that matches claim.
// return this event and its successor
func (s *Service) FindClaimSubmissionEventAndSucc(
	claim *ClaimRow,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimSubmission,
	*iconsensus.IConsensusClaimSubmission,
	error,
) {
	ic, err := iconsensus.NewIConsensus(claim.IConsensusAddress, s.EthConn)
	if err != nil {
		return nil, nil, nil, err
	}

	// filter must match:
	// - `ClaimSubmission` events
	// - submitter == nil (any)
	// - appContract == claim.IApplicationAddress
	c, err := iconsensus.IConsensusMetaData.GetAbi()
	topics, err := abi.MakeTopics(
		[]interface{}{c.Events["ClaimSubmission"].ID},
		nil,
		[]interface{}{claim.IApplicationAddress},
	)
	if err != nil {
		return nil, nil, nil, err
	}
	it, err := ethutil.ChunkedFilterLogs(s.Context, s.EthConn, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(claim.Epoch.LastBlock),
		ToBlock:   new(big.Int).SetInt64(s.endBlock),
		Addresses: []common.Address{claim.IConsensusAddress},
		Topics: topics,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	// pull events instead of iterating
	next, stop := iter.Pull2(it)
	defer stop()
	for {
		event, ok, err := unwrapClaimSubmission(ic, next)
		if !ok || err != nil {
			return ic, event, nil, err
		}
		lastBlock := event.LastProcessedBlockNumber.Uint64()

		if claimMatchesSubmissionEvent(claim, event) {
			// found the event, does it has a successor? try to fetch it
			succ, ok, err := unwrapClaimSubmission(ic, next)
			if !ok || err != nil {
				return ic, event, nil, err
			}
			return ic, event, succ, err
		} else if lastBlock > claim.Epoch.LastBlock {
			err = fmt.Errorf("No matching claim, searched up to %v", event)
			return nil, nil, nil, err
		}
	}
}

/* poll a transaction hash for its submission status and receipt */
func (s *Service) PollTransaction(txHash common.Hash) (bool, *types.Receipt, error) {
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

func GetDefaultBlockValue(defaultBlock DefaultBlock) (int64, error) {
	switch defaultBlock {
	case DefaultBlock_Pending:
		return rpc.PendingBlockNumber.Int64(), nil
	case DefaultBlock_Latest:
		return rpc.LatestBlockNumber.Int64(), nil
	case DefaultBlock_Finalized:
		return rpc.FinalizedBlockNumber.Int64(), nil
	case DefaultBlock_Safe:
		return rpc.SafeBlockNumber.Int64(), nil
	default:
		return 0, fmt.Errorf("default block '%v' not supported", defaultBlock)
	}
}
