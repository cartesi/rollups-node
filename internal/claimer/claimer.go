// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Algorithm for the state transition of computed claims. Possible actions are:
// - update epoch in the database
// - submit claim to blockchain
// - transition application to an invalid state
//
// 1. On startup of a clean blockchain there are no previous claims nor events.
//
//   - This configuration must submit a new computed claim.
//
//     2. Some time after the submission, the computed claim shows up as a claimSubmission
//     event in the blockchain. The claim and event must match.
//
//   - This configuration must update the epoch in the database: computed -> submitted
//
// 3. After the first epoch, additional checks must be done. Same as (1) otherwise.
// 3.1. No epoch was skipped:
//   - previous_claim.last_block < current_claim.first_block
//
// 4. After the first epoch, additional checks must be done. Same as (2) otherwise.
// 4.1. epochs are in order:
//   - previous_claim.last_block < current_claim.first_block
//
// 4.2. There are no events between the epochs
//   - next(previous_event) == current_event
//
// Other cases are errors.
//
// | n |      prev     |      curr     | action |
// |   | claim | event | claim | event |        |
// |---+-------+-------+-------+-------+--------+
// | 1 |   .   |   .   |  cc   |   .   | submit |
// | 2 |   .   |   .   |  cc   |  ce   | update |
// | 3 |  pc   |  pe   |  cc   |   .   | submit |
// | 4 |  pc   |  pe   |  cc   |  ce   | update |
package claimer

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	ErrClaimMismatch = fmt.Errorf("claim and antecessor mismatch")
	ErrEventMismatch = fmt.Errorf("Computed Claim mismatches ClaimSubmission event")
	ErrMissingEvent = fmt.Errorf("accepted claim has no matching blockchain event")
)

type address = common.Address
type hash = common.Hash
type claimRow = repository.ClaimRow
type claimSubmissionEvent = iconsensus.IConsensusClaimSubmission

type CreateInfo struct {
	service.CreateInfo

	Auth config.Auth

	BlockchainHttpEndpoint config.Redacted[string]
	EthConn                *ethclient.Client

	PostgresEndpoint config.Redacted[string]
	DBConn           *repository.Database

	EnableSubmission bool
}

type Service struct {
	service.Service

	submissionEnabled bool
	DBConn            *repository.Database
	EthConn           *ethclient.Client
	TxOpts            *bind.TransactOpts
	claimsInFlight    map[address]hash // -> txHash
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
			ci.DBConn, err = repository.Connect(s.Context, ci.PostgresEndpoint.Value)
			if err != nil {
				return err
			}
		}
		s.DBConn = ci.DBConn
	}

	if s.claimsInFlight == nil {
		s.claimsInFlight = map[address]hash{}
	}

	if s.submissionEnabled && s.TxOpts == nil {
		s.TxOpts, err = CreateTxOptsFromAuth(ci.Auth, s.Context, s.EthConn)
		if err != nil {
			return err
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
	return s.submitClaimsAndUpdateDatabase(s)
}

func (s *Service) submitClaimsAndUpdateDatabase(se sideEffects) []error {
	errs := []error{}
	prevClaims, currClaims, err := se.selectClaimPairsPerApp()
	if err != nil {
		errs = append(errs, err)
		return errs
	}

	// check claims in flight
	for key, txHash := range s.claimsInFlight {
		ready, receipt, err := se.pollTransaction(txHash)
		if err != nil {
			errs = append(errs, err)
			return errs
		}
		if !ready {
			continue
		}
		if claim, ok := currClaims[key]; ok {
			err = se.updateEpochWithSubmittedClaim(&claim, receipt.TxHash)
			if err != nil {
				errs = append(errs, err)
				return errs
			}
			s.Logger.Info("claimer: Claim submitted",
				"app", claim.AppContractAddress,
				"claim", claim.EpochHash,
				"last_block", claim.EpochLastBlock,
				"tx", txHash)
			delete(currClaims, key)
		} else {
			s.Logger.Warn("claimer: expected claim in flight to be in currClaims.",
				"tx", receipt.TxHash)
		}
		delete(s.claimsInFlight, key)
	}

	// check computed claims
	nextApp: for key, currClaimRow := range currClaims {
		var ic *iconsensus.IConsensus = nil
		var prevEvent *claimSubmissionEvent = nil
		var currEvent *claimSubmissionEvent = nil

		if _, isInFlight := s.claimsInFlight[key]; isInFlight {
			continue
		}

		if prevClaimRow, ok := prevClaims[key]; ok {
			err := checkClaimsConstraint(&prevClaimRow, &currClaimRow)
			if err != nil {
				s.Logger.Error("claimer: database mismatch",
					"prevClaim", prevClaimRow,
					"currClaim", currClaimRow,
					"err", err,
				)
				delete(currClaims, key)
				errs = append(errs, err)
				goto nextApp
			}

			// if prevClaimRow exists, there must be a matching event
			ic, prevEvent, currEvent, err =
				se.findClaimSubmissionEventAndSucc(&prevClaimRow)
			if err != nil {
				delete(currClaims, key)
				errs = append(errs, err)
				goto nextApp
			}
			if prevEvent == nil {
				s.Logger.Error("claimer: missing event",
					"claim", prevClaimRow,
					"err", ErrMissingEvent,
				)
				delete(currClaims, key)
				errs = append(errs, ErrMissingEvent)
				goto nextApp
			}
			if !claimMatchesEvent(&prevClaimRow, prevEvent) {
				s.Logger.Error("claimer: event mismatch",
					"claim", prevClaimRow,
					"event", prevEvent,
					"err", ErrEventMismatch,
				)
				delete(currClaims, key)
				errs = append(errs, ErrEventMismatch)
				goto nextApp
			}
		} else {
			// first claim
			ic, currEvent, _, err =
				se.findClaimSubmissionEventAndSucc(&currClaimRow)
			if err != nil {
				delete(currClaims, key)
				errs = append(errs, err)
				goto nextApp
			}
		}

		if currEvent != nil {
			if !claimMatchesEvent(&currClaimRow, currEvent) {
				s.Logger.Error("claimer: event mismatch",
					"claim", currClaimRow,
					"event", currEvent,
					"err", ErrEventMismatch,
				)
				delete(currClaims, key)
				errs = append(errs, ErrEventMismatch)
				goto nextApp
			}
			txHash := currEvent.Raw.TxHash
			err = se.updateEpochWithSubmittedClaim(&currClaimRow, txHash)
			if err != nil {
				delete(currClaims, key)
				errs = append(errs, err)
				goto nextApp
			}
			delete(s.claimsInFlight, key)
		} else if s.submissionEnabled {
			txHash, err := se.submitClaimToBlockchain(ic, &currClaimRow)
			if err != nil {
				delete(currClaims, key)
				errs = append(errs, err)
				goto nextApp
			}
			s.Logger.Info("claimer: Submitting claim to blockchain",
				"app",        currClaimRow.AppContractAddress,
				"claim",      currClaimRow.EpochHash,
				"last_block", currClaimRow.EpochLastBlock,
			)
			s.claimsInFlight[currClaimRow.AppContractAddress] = txHash
		}
	}
	return errs
}

func checkClaimConstraint(c *claimRow) error {
	zeroAddress := address{}

	if c.EpochFirstBlock > c.EpochLastBlock {
		return ErrClaimMismatch
	}
	if c.AppIConsensusAddress == zeroAddress {
		return ErrClaimMismatch
	}
	return nil
}

func checkClaimsConstraint(p *claimRow, c *claimRow) error {
	var err error

	err = checkClaimConstraint(c)
	if err != nil {
		return err
	}
	err = checkClaimConstraint(p)
	if err != nil {
		return err
	}

	// p, c consistent
	if p.AppContractAddress != c.AppContractAddress {
		return ErrClaimMismatch
	}
	if p.EpochLastBlock > c.EpochLastBlock {
		return ErrClaimMismatch
	}
	if p.EpochFirstBlock > c.EpochFirstBlock {
		return ErrClaimMismatch
	}
	if p.EpochIndex > c.EpochIndex {
		return ErrClaimMismatch
	}
	return nil
}

func claimMatchesEvent(c *claimRow, e *claimSubmissionEvent) bool {
	return  c.AppContractAddress == e.AppContract &&
		c.EpochLastBlock == e.LastProcessedBlockNumber.Uint64()
}

func (s *Service) Start(context context.Context, ready chan<- struct{}) error {
	ready <- struct{}{}
	return s.Serve()
}
func (s *Service) String() string {
	return s.Name
}
