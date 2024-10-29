// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"math/big"

	//. "github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	. "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type IClaimSubmissionIterator interface {
	Next() bool
	Error() error
	Event() *iconsensus.IConsensusClaimSubmission
}

type ClaimSubmissionIterator struct {
	iterator *iconsensus.IConsensusClaimSubmissionIterator
}

func (p *ClaimSubmissionIterator) Next() bool {
	return p.iterator.Next()
}

func (p *ClaimSubmissionIterator) Error() error {
	return p.iterator.Error()
}

func (p *ClaimSubmissionIterator) Event() *iconsensus.IConsensusClaimSubmission {
	return p.iterator.Event
}

type SideEffects interface {
	submitClaimToBlockchain(
		instance *iconsensus.IConsensus,
		signer *bind.TransactOpts,
		claim *ComputedClaim,
	) (Hash, error)

	selectComputedClaims() ([]ComputedClaim, error)

	updateEpochWithSubmittedClaim(
		DBConn *Database,
		context context.Context,
		claim *ComputedClaim,
		txHash Hash,
	) error

	enumerateSubmitClaimEventsSince(
		EthConn *ethclient.Client,
		context context.Context,
		appIConsensusAddr Address,
		epochLastBlock uint64,
	) (
		IClaimSubmissionIterator,
		*iconsensus.IConsensus,
		error)

	pollTransaction(txHash Hash) (bool, *types.Receipt, error)
}

func (s *Service) submitClaimToBlockchain(
	instance *iconsensus.IConsensus,
	signer *bind.TransactOpts,
	claim *ComputedClaim,
) (Hash, error) {
	txHash := Hash{}
	lastBlockNumber := new(big.Int).SetUint64(claim.EpochLastBlock)
	tx, err := instance.SubmitClaim(signer, claim.AppContractAddress,
		lastBlockNumber, claim.Hash)
	if err != nil {
		s.Logger.Error("submitClaimToBlockchain:failed",
			"service", s.Name,
			"appContractAddress", claim.AppContractAddress,
			"claimHash", claim.Hash,
			"txHash", tx.Hash(),
			"error", err)
	} else {
		txHash = tx.Hash()
		s.Logger.Debug("SubmitClaimToBlockchain:success",
			"service", s.Name,
			"appContractAddress", claim.AppContractAddress,
			"claimHash", claim.Hash,
			"TxHash", txHash)
	}
	return txHash, err
}

func (s *Service) selectComputedClaims() ([]ComputedClaim, error) {
	claims, err := s.DBConn.SelectComputedClaims(s.Context)
	if err != nil {
		s.Logger.Error("SelectComputedClaims:failed",
			"service", s.Name,
			"error", err)
	} else {
		var ids []uint64
		for _, claim := range claims {
			ids = append(ids, claim.EpochID)
		}
		s.Logger.Debug("SelectComputedClaims:success",
			"service", s.Name,
			"claims", len(claims),
			"ids", ids,
			"inFlight", len(s.ClaimsInFlight))
	}
	return claims, err
}

/* update the database epoch status to CLAIM_SUBMITTED and add a transaction hash */
func (s *Service) updateEpochWithSubmittedClaim(
	DBConn *Database,
	context context.Context,
	claim *ComputedClaim,
	txHash Hash,
) error {
	err := DBConn.UpdateEpochWithSubmittedClaim(context, claim.EpochID, txHash)
	if err != nil {
		s.Logger.Error("UpdateEpochWithSubmittedClaim:failed",
			"service", s.Name,
			"appContractAddress", claim.AppContractAddress,
			"hash", claim.Hash,
			"txHash", txHash,
			"error", err)
	} else {
		s.Logger.Debug("UpdateEpochWithSubmittedClaim:success",
			"service", s.Name,
			"appContractAddress", claim.AppContractAddress,
			"hash", claim.Hash,
			"txHash", txHash)
	}
	return err
}

func (s *Service) enumerateSubmitClaimEventsSince(
	EthConn *ethclient.Client,
	context context.Context,
	appIConsensusAddr Address,
	epochLastBlock uint64,
) (
	IClaimSubmissionIterator,
	*iconsensus.IConsensus,
	error,
) {
	it, ic, err := s.EnumerateSubmitClaimEventsSince(
		EthConn, context, appIConsensusAddr, epochLastBlock)

	if err != nil {
		s.Logger.Error("EnumerateSubmitClaimEventsSince:failed",
			"service", s.Name,
			"appIConsensusAddr", appIConsensusAddr,
			"epochLastBlock", epochLastBlock,
			"error", err)
	} else {
		s.Logger.Debug("EnumerateSubmitClaimEventsSince:success",
			"service", s.Name,
			"appIConsensusAddr", appIConsensusAddr,
			"epochLastBlock", epochLastBlock)
	}
	return it, ic, err
}

func (s *Service) EnumerateSubmitClaimEventsSince(
	EthConn *ethclient.Client,
	context context.Context,
	appIConsensusAddr Address,
	epochLastBlock uint64,
) (
	IClaimSubmissionIterator,
	*iconsensus.IConsensus,
	error,
) {
	ic, err := iconsensus.NewIConsensus(appIConsensusAddr, EthConn)
	if err != nil {
		return nil, nil, err
	}

	it, err := ic.FilterClaimSubmission(&bind.FilterOpts{
		Context: context,
		Start:   epochLastBlock,
	}, nil, nil)

	return &ClaimSubmissionIterator{iterator: it}, ic, nil
}

func (s *Service) pollTransaction(txHash Hash) (bool, *types.Receipt, error) {
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

func (s *Service) PollTransaction(txHash Hash) (bool, *types.Receipt, error) {
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
