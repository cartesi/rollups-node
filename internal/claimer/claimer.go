// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"

	. "github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	. "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateInfo struct {
	service.CreateInfo

	Auth   Auth
	Signer *bind.TransactOpts

	BlockchainHttpEndpoint Redacted[string]
	EthConn                *ethclient.Client

	PostgresEndpoint Redacted[string]
	DBConn           *Database

	EnableSubmission bool
}

type claimKey struct {
	hash           Hash
	epochLastBlock uint64
}

type Service struct {
	service.Service

	submissionEnabled bool
	DBConn            *Database
	EthConn           *ethclient.Client
	Signer            *bind.TransactOpts
	ClaimsInFlight    map[claimKey]Hash // -> txHash
}

func Create(ci CreateInfo, s *Service) error {
	var err error

	err = service.Create(&ci.CreateInfo, &s.Service)
	if err != nil {
		return err
	}

	s.submissionEnabled = ci.EnableSubmission
	if s.EthConn == nil {
		if ci.EthConn == nil {
			ci.EthConn, err = ethclient.Dial(ci.BlockchainHttpEndpoint.Value)
			if err != nil {
				return err
			}
		}
		s.EthConn = ci.EthConn
	}

	if s.DBConn == nil {
		if ci.DBConn == nil {
			ci.DBConn, err = Connect(s.Context, ci.PostgresEndpoint.Value)
			if err != nil {
				return err
			}
		}
		s.DBConn = ci.DBConn
	}

	if s.ClaimsInFlight == nil {
		s.ClaimsInFlight = map[claimKey]Hash{}
	}

	if s.Signer == nil && s.submissionEnabled {
		if ci.Signer == nil {
			ci.Signer, err = CreateSignerFromAuth(ci.Auth, s.Context, s.EthConn)
			if err != nil {
				return err
			}
			s.Signer = ci.Signer
		}
	}

	return err
}

func (s *Service) Alive() bool {
	return true
}

func (s *Service) Ready() bool {
	return true
}

func (s *Service) Reload() []error {
	return nil
}

func (s *Service) Stop(bool) []error {
	return nil
}

func (s *Service) Tick() []error {
	err := s.submitClaimsAndUpdateDatabase(s)
	if err != nil {
		return []error{err}
	}
	return nil
}

func computedClaimToKey(c *ComputedClaim) claimKey {
	return claimKey{
		hash:           c.Hash,
		epochLastBlock: c.EpochLastBlock,
	}
}

func eventToKey(e *iconsensus.IConsensusClaimSubmission) claimKey {
	return claimKey{
		hash:           e.Claim,
		epochLastBlock: e.LastProcessedBlockNumber.Uint64(),
	}
}

func (s *Service) submitClaimsAndUpdateDatabase(se SideEffects) error {
	claims, err := se.selectComputedClaims()
	if err != nil {
		return err
	}

	computedClaimsMap := make(map[claimKey]*ComputedClaim)
	for i := 0; i < len(claims); i++ {
		computedClaimsMap[computedClaimToKey(&claims[i])] = &claims[i]
	}

	// check claims in flight
	for key, txHash := range s.ClaimsInFlight {
		ready, receipt, err := se.pollTransaction(txHash)
		if err != nil {
			return err
		}
		if !ready {
			continue
		}

		if claim, ok := computedClaimsMap[key]; ok {
			err = se.updateEpochWithSubmittedClaim(
				s.DBConn,
				s.Context,
				claim,
				receipt.TxHash)
			if err != nil {
				return err
			}
			delete(s.ClaimsInFlight, key)
			delete(computedClaimsMap, key)
			s.Logger.Info("claimer: Claim submitted",
				"app", claim.AppContractAddress,
				"claim", claim.Hash,
				"last_block", claim.EpochLastBlock,
				"tx", receipt.TxHash)
		}
	}

	// check event logs for the remaining claims, submit if not found
	for i := 0; i < len(claims); i++ {
		claimDB := &claims[i]
		key := computedClaimToKey(claimDB)
		_, isSelected := computedClaimsMap[key]
		_, isInFlight := s.ClaimsInFlight[key]
		if !isSelected || isInFlight {
			continue
		}

		s.Logger.Info("claimer: Checking if there was previous submitted claims",
			"app", claimDB.AppContractAddress,
			"claim", claimDB.Hash,
			"last_block", claimDB.EpochLastBlock,
		)
		it, inst, err := se.enumerateSubmitClaimEventsSince(
			s.EthConn, s.Context,
			claimDB.AppIConsensusAddress,
			claimDB.EpochLastBlock+1)
		if err != nil {
			return err
		}

		for it.Next() {
			event := it.Event()
			eventKey := eventToKey(event)
			claim, ok := computedClaimsMap[eventKey]

			if event.LastProcessedBlockNumber.Uint64() > claimDB.EpochLastBlock {
				// found a newer event than the claim we are processing.
				s.Logger.Error("claimer: Found a newer event than claim",
					"app", claimDB.AppContractAddress,
					"claim", Hash(event.Claim),
					"claim_last_block", claimDB.EpochLastBlock,
					"event_last_block", event.LastProcessedBlockNumber.Uint64(),
				)
				return fmt.Errorf("Application in invalid state")
				// TODO: put the application in an invalid state
			}
			s.Logger.Debug("claimer: Found previous submitted claim event",
				"app", claimDB.AppContractAddress,
				"claim", Hash(event.Claim),
				"last_block", event.LastProcessedBlockNumber,
			)
			if ok {
				s.Logger.Info("claimer: Claim was previously submitted, updating the database",
					"app", claimDB.AppContractAddress,
					"claim", claimDB.Hash,
					"last_block", claimDB.EpochLastBlock,
				)
				err := se.updateEpochWithSubmittedClaim(
					s.DBConn,
					s.Context,
					claim,
					event.Raw.TxHash)
				if err != nil {
					return err
				}
				delete(computedClaimsMap, eventKey)
				break
			}
		}
		if err := it.Error(); err != nil {
			return err
		}

		// submit if not found in the logs (fetch from hash again, can be stale)
		if claim, ok := computedClaimsMap[key]; ok && s.submissionEnabled {
			s.Logger.Info("Submitting claim to blockchain",
				"app", claim.AppContractAddress,
				"claim", claim.Hash,
				"last_block", claim.EpochLastBlock,
			)
			txHash, err := se.submitClaimToBlockchain(inst, s.Signer, claim)
			if err != nil {
				return err
			}
			s.ClaimsInFlight[key] = txHash
			delete(computedClaimsMap, key)
		}
	}
	return nil
}

func (s *Service) Start(context context.Context, ready chan<- struct{}) error {
	ready <- struct{}{}
	return s.Serve()
}
func (s *Service) String() string {
	return s.Name
}
