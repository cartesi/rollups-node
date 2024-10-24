package claimer

/*
submitClaimsAndUpdateDB:
	computedClaims := selectComputedClaims()

	for claim in computedClaims:
		for event in enumerateSubmitClaimEventsSince(claim.LastBlock):
			UpdateEpochWithClaim(claimsInFlight[event.Claim])
			delete(claimsInFlight, event.Hash)
			continue

		if ! claimsInFlight[claim]:
			submitClaim(claim)
			claimsInFlight[claim.Hash] = struct{}
	
	for claim in claimInFlight:
		if age(claim) > threashold:
			resubmit?
*/

import (
	"github.com/cartesi/rollups-node/pkg/service"
	. "github.com/cartesi/rollups-node/internal/repository"
	. "github.com/cartesi/rollups-node/internal/config"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	. "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateInfo struct {
	service.CreateInfo

	Auth                   Auth
	BlockchainHttpEndpoint Redacted[string]
	EthConn                ethclient.Client
	PostgresEndpoint       Redacted[string]
}

type Service struct {
	service.Service

	dbConn  *Database
	ethConn *ethclient.Client
	signer  *bind.TransactOpts
	claimsInFlight map[Hash]uint64
}

func Create(ci CreateInfo, s *Service) error {
	var err error

	err = service.Create(ci.CreateInfo, s, &s.Service)
	if err != nil {
		return err
	}

	s.ethConn, err = ethclient.Dial(ci.BlockchainHttpEndpoint.Value)
	if err != nil {
		return err
	}

	s.dbConn, err = Connect(ci.Context, ci.PostgresEndpoint.Value)
	if err != nil {
		return err
	}

	// on startup, assume that all fetched claims are in flight
	s.claimsInFlight = make(map[Hash]uint64)
	computedClaims, err := s.dbConn.SelectComputedClaims(s.Context)
	for i := 0; i < len(computedClaims); i++ {
		// assume they got submitted on this block
		s.claimsInFlight[computedClaims[i].Hash] =
			computedClaims[i].EpochLastBlock+1
	}

	return nil
}

func (s *Service) Alive() bool {
	return true
}

func (s *Service) Ready() bool {
	return true
}

func (s *Service) Reload() bool {
	return true
}

func (s *Service) Tick() bool {
	computedClaims, err := s.dbConn.SelectComputedClaims(s.Context)
	if err != nil {
		return false
	}
	claimFromHash := make(map[Hash]*ComputedClaim)

	for i := 0; i < len(computedClaims); i++ {
		claimFromHash[computedClaims[i].Hash] = &computedClaims[i]
	}

	for i := 0; i < len(computedClaims); i++ {
		it, inst, err := s.enumerateSubmitClaimEventsSince(
			s.ethConn, s.Context,
			computedClaims[i].AppIConsensusAddress,
			computedClaims[i].EpochLastBlock)
		if err != nil {
			return false
		}

		// update the database for each submitClaim event that is:
		// - a computed claim and,
		// - is in flight
		for event, err := range it {
			if err != nil {
				return false
			}

			if _, ok := s.claimsInFlight[event.Claim]; !ok {
				// found a claim that was submitted but not updated
			}
			if claim, ok := claimFromHash[event.Claim]; ok {
				s.updateEpochWithSubmittedClaim(
					s.dbConn,
					s.Context,
					claim,
					event.Raw.TxHash)
				delete(s.claimsInFlight, event.Claim)
			}
		}

		appContractAddress := computedClaims[i].AppContractAddress
		for n := len(computedClaims); i < n &&
			computedClaims[i].AppContractAddress == appContractAddress; i++ {
			if _, ok := s.claimsInFlight[computedClaims[i].Hash]; !ok {
				_, err := s.submitClaimToBlockchain(inst, s.signer, &computedClaims[i])
				if err != nil {
					return false
				}
			}
		}
	}

	// assume failed if old
	blockNumber, err := s.ethConn.BlockNumber(s.Context)
	if err != nil {
		return false
	}
	for hash, age := range s.claimsInFlight {
		if age + 7 < blockNumber {
			delete(s.claimsInFlight, hash)
		}
	}

	return true
}
