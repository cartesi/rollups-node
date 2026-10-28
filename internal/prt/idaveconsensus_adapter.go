// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/imultileveltournamentfactory"
)

// DaveConsensusAdapterImpl wraps the generated IDaveConsensus binding.
type DaveConsensusAdapterImpl struct {
	consensus *idaveconsensus.IDaveConsensus
	client    *ethclient.Client
	address   common.Address
}

// NewDaveConsensusAdapter creates a new DaveConsensusAdapter backed by the IDaveConsensus contract.
func NewDaveConsensusAdapter(
	addr common.Address,
	client *ethclient.Client,
) (DaveConsensusAdapter, error) {
	consensus, err := idaveconsensus.NewIDaveConsensus(addr, readOnlyBackend{Client: client})
	if err != nil {
		return nil, err
	}
	return &DaveConsensusAdapterImpl{consensus: consensus, client: client, address: addr}, nil
}

func (a *DaveConsensusAdapterImpl) ParseEpochSealed(log types.Log) (*idaveconsensus.IDaveConsensusEpochSealed, error) {
	return a.consensus.ParseEpochSealed(log)
}

// Query only the exact accepting block, consensus and next-epoch event. A lost
// publication race must not require one receipt RPC per unrelated transaction.
func (a *DaveConsensusAdapterImpl) EpochSealedAt(
	ctx context.Context, blockHash common.Hash, epoch uint64,
) ([]*idaveconsensus.IDaveConsensusEpochSealed, error) {
	contract, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	logs, err := a.client.FilterLogs(ctx, ethereum.FilterQuery{BlockHash: &blockHash,
		Addresses: []common.Address{a.address},
		Topics:    [][]common.Hash{{contract.Events["EpochSealed"].ID}, {common.BigToHash(new(big.Int).SetUint64(epoch))}},
	})
	if err != nil {
		return nil, err
	}
	events := make([]*idaveconsensus.IDaveConsensusEpochSealed, 0, len(logs))
	for _, log := range logs {
		event, err := a.ParseEpochSealed(log)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (a *DaveConsensusAdapterImpl) TournamentLevelCount(opts *bind.CallOpts) (uint64, error) {
	factoryAddress, err := a.consensus.GetTournamentFactory(opts)
	if err != nil {
		return 0, err
	}
	factory, err := imultileveltournamentfactory.NewIMultiLevelTournamentFactory(factoryAddress, readOnlyBackend{Client: a.client})
	if err != nil {
		return 0, err
	}
	levelCount, err := factory.TournamentLevelCount(opts)
	if err != nil {
		return 0, err
	}
	if levelCount == 0 {
		return 0, errors.New("tournament level count is zero")
	}
	return levelCount, nil
}

func checkedUint64(value *big.Int, field string) (uint64, error) {
	if value == nil || value.Sign() < 0 || !value.IsUint64() {
		return 0, fmt.Errorf("%s is not a uint64", field)
	}
	return value.Uint64(), nil
}

func (a *DaveConsensusAdapterImpl) GetCurrentSealedEpoch(opts *bind.CallOpts) (CurrentSealedEpoch, error) {
	result, err := a.consensus.GetCurrentSealedEpoch(opts)
	if err != nil {
		return CurrentSealedEpoch{}, err
	}
	epoch, err := checkedUint64(result.EpochNumber, "sealed epoch number")
	if err != nil {
		return CurrentSealedEpoch{}, err
	}
	lowerBound, err := checkedUint64(result.InputIndexLowerBound, "sealed epoch input lower bound")
	if err != nil {
		return CurrentSealedEpoch{}, err
	}
	upperBound, err := checkedUint64(result.InputIndexUpperBound, "sealed epoch input upper bound")
	if err != nil {
		return CurrentSealedEpoch{}, err
	}
	stagingBlock, err := checkedUint64(result.StagingBlockNumber, "sealed epoch staging block")
	if err != nil {
		return CurrentSealedEpoch{}, err
	}
	return CurrentSealedEpoch{
		EpochNumber:                      epoch,
		InputIndexLowerBound:             lowerBound,
		InputIndexUpperBound:             upperBound,
		Tournament:                       result.Tournament,
		IsTournamentResultStaged:         result.IsTournamentResultStaged,
		StagingBlockNumber:               stagingBlock,
		StagedPostEpochMachineStateHash:  result.StagedPostEpochMachineStateHash,
		StagedPostEpochOutputsMerkleRoot: result.StagedPostEpochOutputsMerkleRoot,
	}, nil
}

func (a *DaveConsensusAdapterImpl) CanStageTournamentResult(
	opts *bind.CallOpts,
) (CanStageTournamentResult, error) {
	result, err := a.consensus.CanStageTournamentResult(opts)
	if err != nil {
		return CanStageTournamentResult{}, err
	}
	epoch, err := checkedUint64(result.EpochNumber, "stage readiness epoch number")
	if err != nil {
		return CanStageTournamentResult{}, err
	}
	return CanStageTournamentResult{
		IsFinished:                      result.IsFinished,
		IsTournamentFailed:              result.IsTournamentFailed,
		IsTournamentResultStaged:        result.IsTournamentResultStaged,
		EpochNumber:                     epoch,
		WinnerCommitment:                result.WinnerCommitment,
		WinnerPostEpochMachineStateHash: result.WinnerPostEpochMachineStateHash,
	}, nil
}

func (a *DaveConsensusAdapterImpl) CanAcceptStagedTournamentResult(
	opts *bind.CallOpts,
) (CanAcceptStagedTournamentResult, error) {
	result, err := a.consensus.CanAcceptStagedTournamentResult(opts)
	if err != nil {
		return CanAcceptStagedTournamentResult{}, err
	}
	epoch, err := checkedUint64(result.EpochNumber, "accept readiness epoch number")
	if err != nil {
		return CanAcceptStagedTournamentResult{}, err
	}
	return CanAcceptStagedTournamentResult{
		IsTournamentResultStaged:                     result.IsTournamentResultStaged,
		DoAllSentriesAgreeWithStagedTournamentResult: result.DoAllSentriesAgreeWithStagedTournamentResult,
		IsClaimStagingPeriodOver:                     result.IsClaimStagingPeriodOver,
		EpochNumber:                                  epoch,
		StagedPostEpochMachineStateHash:              result.StagedPostEpochMachineStateHash,
		StagedPostEpochOutputsMerkleRoot:             result.StagedPostEpochOutputsMerkleRoot,
	}, nil
}

func daveMachineValidityProof(proof model.StateProof) idaveconsensus.MachineValidityProof {
	return idaveconsensus.MachineValidityProof{
		IflagsYProof: idaveconsensus.LeafProof{
			DataBlock: proof.IflagsYDataBlock,
			Siblings:  append([][32]byte(nil), proof.IflagsYProof...),
		},
		HtifTohostProof: idaveconsensus.LeafProof{
			DataBlock: proof.HtifTohostDataBlock,
			Siblings:  append([][32]byte(nil), proof.HtifTohostProof...),
		},
		TxBufferProof: idaveconsensus.LeafProof{
			DataBlock: proof.TxBufferDataBlock,
			Siblings:  append([][32]byte(nil), proof.TxBufferProof...),
		},
	}
}
