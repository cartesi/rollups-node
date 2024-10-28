// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"

	. "github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/repository"
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
}

type Service struct {
	service.Service

	DBConn         *Database
	EthConn        *ethclient.Client
	Signer         *bind.TransactOpts
	ClaimsInFlight map[Hash]Hash // claimHash -> txHash
}

func Create(ci CreateInfo, s *Service) error {
	var err error

	err = service.Create(&ci.CreateInfo, &s.Service)
	if err != nil {
		return err
	}

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
		s.ClaimsInFlight = map[Hash]Hash{}
	}

	if s.Signer == nil {
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

func (s *Service) submitClaimsAndUpdateDatabase(se SideEffects) error {
	claims, err := se.selectComputedClaims()
	if err != nil {
		return err
	}

	claimFromHash := make(map[Hash]*ComputedClaim)
	for i := 0; i < len(claims); i++ {
		claimFromHash[claims[i].Hash] = &claims[i]
	}

	// check claims in flight
	for claimHash, txHash := range s.ClaimsInFlight {
		ready, receipt, err := se.pollTransaction(txHash)
		if err != nil {
			return err
		}
		if !ready {
			continue
		}

		if claim, ok := claimFromHash[claimHash]; ok {
			err = se.updateEpochWithSubmittedClaim(
				s.DBConn,
				s.Context,
				claim,
				receipt.TxHash)
			if err != nil {
				return err
			}
			delete(s.ClaimsInFlight, claimHash)
			delete(claimFromHash, claimHash)
			s.Logger.Info("claimer: Claim submitted",
				"app", claim.AppContractAddress,
				"claim", claimHash,
				"last_block", claim.EpochLastBlock,
				"tx", receipt.TxHash)
		}
	}

	// check event logs for the remaining claims, submit if not found
	for i := 0; i < len(claims); i++ {
		_, isSelected := claimFromHash[claims[i].Hash]
		_, isInFlight := s.ClaimsInFlight[claims[i].Hash]
		if !isSelected || isInFlight {
			continue
		}

		it, inst, err := se.enumerateSubmitClaimEventsSince(
			s.EthConn, s.Context,
			claims[i].AppIConsensusAddress,
			claims[i].EpochLastBlock)
		if err != nil {
			return err
		}

		for event, err := range it {
			if err != nil {
				return err
			}

			hash := Hash(event.Claim)
			claim, ok := claimFromHash[hash]
			if ok {
				err := se.updateEpochWithSubmittedClaim(
					s.DBConn,
					s.Context,
					claim,
					event.Raw.TxHash)
				if err != nil {
					return err
				}
				delete(claimFromHash, hash)
			}
		}

		// submit if not found in the logs (fetch from hash again, can be stale)
		if claim, ok := claimFromHash[claims[i].Hash]; ok {
			txHash, err := se.submitClaimToBlockchain(inst, s.Signer, claim)
			if err != nil {
				return err
			}
			s.ClaimsInFlight[claims[i].Hash] = txHash
			delete(claimFromHash, claim.Hash)
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
