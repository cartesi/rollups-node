package claimer

import (
	"context"
	"iter"
	"log/slog"
	"math/big"

	//. "github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	. "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func (s *Service) submitClaimToBlockchain(
	instance *iconsensus.IConsensus,
	signer *bind.TransactOpts,
	claim *ComputedClaim,
) (*types.Transaction, error) {
	lastBlockNumber := new(big.Int).SetUint64(claim.EpochLastBlock)
	tx, err := instance.SubmitClaim(signer, claim.AppContractAddress,
		lastBlockNumber, claim.Hash)
	if err != nil {
		slog.Error("submitClaimToBlockchain:failed",
			"service", s.Name,
			"appContractAddress", claim.AppContractAddress,
			"claimHash", claim.Hash,
			"txHash", tx.Hash(),
			"error", err)
	} else {
		slog.Info("SubmitClaimToBlockchain:success",
			"service", s.Name,
			"appContractAddress", claim.AppContractAddress,
			"claimHash", claim.Hash,
			"TxHash", tx.Hash())
	}
	return tx, err
}

func (s *Service) selectComputedClaims() ([]ComputedClaim, error) {
	claims, err := s.dbConn.SelectComputedClaims(s.Context)
	if err != nil {
		slog.Error("SelectComputedClaims:failed",
			"service", s.Name,
			"error", err)
	} else {
		var ids []uint64
		for _, claim := range claims {
			ids = append(ids, claim.EpochID)
		}
		slog.Info("SelectComputedClaims:success",
			"service", s.Name,
			"claims", len(claims),
			"ids", ids)
	}
	return claims, err
}

/* update the database epoch status to CLAIM_SUBMITTED and add a transaction hash */
func (s *Service) updateEpochWithSubmittedClaim(
	dbConn *Database,
	context context.Context,
	claim *ComputedClaim,
	txHash Hash,
) error {
	_, err := dbConn.UpdateEpochWithSubmittedClaim(context, claim.EpochID, txHash)
	if err != nil {
		slog.Error("UpdateEpochWithSubmittedClaim:failed",
			"service", s.Name,
			"appContractAddress", claim.AppContractAddress,
			"hash", claim.Hash,
			"txHash", txHash,
			"error", err)
	} else {
		slog.Info("UpdateEpochWithSubmittedClaim:success",
			"service", s.Name,
			"appContractAddress", claim.AppContractAddress,
			"hash", claim.Hash,
			"txHash", txHash)
	}
	return err
}

func (s *Service) enumerateSubmitClaimEventsSince(
	ethConn          *ethclient.Client,
	context           context.Context,
	appIConsensusAddr Address,
	epochLastBlock    uint64,
) (
	iter.Seq2[*iconsensus.IConsensusClaimSubmission, error],
	*iconsensus.IConsensus,
	error,
) {
	ic, err := iconsensus.NewIConsensus(appIConsensusAddr, ethConn)
	if err != nil {
		return nil, nil, err
	}

	it, err := ic.FilterClaimSubmission(&bind.FilterOpts{
		Context: context,
		Start:   epochLastBlock,
	}, nil, nil)

	// make an iterator for the events
	return func(yield func(*iconsensus.IConsensusClaimSubmission, error) bool) {
		if !it.Next() {
			return
		}
		cont := yield(it.Event, it.Error())
		if !cont {
			return
		}
	}, ic, nil
}


