// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
)

// DaveConsensusAdapterImpl wraps the generated IDaveConsensus binding.
type DaveConsensusAdapterImpl struct {
	consensus *idaveconsensus.IDaveConsensus
}

// NewDaveConsensusAdapter creates a new DaveConsensusAdapter backed by the IDaveConsensus contract.
func NewDaveConsensusAdapter(
	addr common.Address,
	client *ethclient.Client,
) (DaveConsensusAdapter, error) {
	consensus, err := idaveconsensus.NewIDaveConsensus(addr, client)
	if err != nil {
		return nil, err
	}
	return &DaveConsensusAdapterImpl{consensus: consensus}, nil
}

func (a *DaveConsensusAdapterImpl) ParseEpochSealed(log types.Log) (*idaveconsensus.IDaveConsensusEpochSealed, error) {
	return a.consensus.ParseEpochSealed(log)
}

func (a *DaveConsensusAdapterImpl) CanSettle(opts *bind.CallOpts) (CanSettleResult, error) {
	result, err := a.consensus.CanSettle(opts)
	if err != nil {
		return CanSettleResult{}, err
	}
	return CanSettleResult{
		IsFinished:       result.IsFinished,
		EpochNumber:      result.EpochNumber,
		WinnerCommitment: result.WinnerCommitment,
	}, nil
}

// IsEpochSettled checks on-chain whether an epoch has already been settled by
// comparing the given epoch number against the current sealed epoch. If the
// sealed epoch has advanced past it, the settlement was already performed.
// This prevents duplicate Settle calls after a node restart.
func (a *DaveConsensusAdapterImpl) IsEpochSettled(
	opts *bind.CallOpts, epochNumber uint64,
) (bool, error) {
	sealed, err := a.consensus.GetCurrentSealedEpoch(opts)
	if err != nil {
		return false, err
	}
	return sealed.EpochNumber.Uint64() > epochNumber, nil
}

func (a *DaveConsensusAdapterImpl) Settle(
	opts *bind.TransactOpts, epochNumber *big.Int,
	outputsMerkleRoot [32]byte, proof [][32]byte,
) (*types.Transaction, error) {
	return a.consensus.Settle(opts, epochNumber, outputsMerkleRoot, proof)
}
