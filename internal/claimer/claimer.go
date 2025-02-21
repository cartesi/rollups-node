// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package claimer

import (
	"context"
	"fmt"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/fsm"
	"github.com/cartesi/rollups-node/internal/model"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	ErrClaimMismatch = fmt.Errorf("claim and antecessor mismatch")
	ErrEventMismatch = fmt.Errorf("Computed Claim mismatches ClaimSubmission event")
	ErrMissingEvent  = fmt.Errorf("accepted claim has no matching blockchain event")
)

type CreateInfo struct {
	service.CreateInfo

	Auth config.Auth

	BlockchainHttpEndpoint config.Redacted[string]
	EthConn                *ethclient.Client
	PostgresEndpoint       config.Redacted[string]
	Repository             repository.Repository
	EnableSubmission       bool
	MaxStartupTime         time.Duration
	DefaultBlock           config.DefaultBlock
}

type Service struct {
	service.Service

	submissionEnabled bool
	Repository        repository.Repository
	EthConn           *ethclient.Client
	TxOpts            *bind.TransactOpts
	claimsInFlight    map[common.Address]common.Hash // -> txHash
	endBlock          int64
}

func (c *CreateInfo) LoadEnv() {
	c.EnableSubmission = config.GetFeatureClaimSubmissionEnabled()
	if c.EnableSubmission {
		c.Auth = config.AuthFromEnv()
	}
	c.BlockchainHttpEndpoint.Value = config.GetBlockchainHttpEndpoint()
	c.PostgresEndpoint.Value = config.GetPostgresEndpoint()
	c.PollInterval = config.GetClaimerPollingInterval()
	c.MaxStartupTime = config.GetMaxStartupTime()
	c.LogLevel = service.LogLevel(config.GetLogLevel())
	c.LogPretty = config.GetLogPrettyEnabled()
	c.DefaultBlock = config.GetEvmReaderDefaultBlock()
}

func Create(c *CreateInfo, s *Service) error {
	var err error

	err = service.Create(&c.CreateInfo, &s.Service)
	if err != nil {
		return err
	}

	return service.WithTimeout(c.MaxStartupTime, func() error {
		s.submissionEnabled = c.EnableSubmission
		if s.EthConn == nil {
			if c.EthConn == nil {
				c.EthConn, err = ethclient.Dial(c.BlockchainHttpEndpoint.Value)
				if err != nil {
					return err
				}
			}
			s.EthConn = c.EthConn
		}

		if s.Repository == nil {
			c.Repository, err = factory.NewRepositoryFromConnectionString(s.Context, c.PostgresEndpoint.Value)
			if err != nil {
				return err
			}
			s.Repository = c.Repository
		}

		if s.claimsInFlight == nil {
			s.claimsInFlight = map[common.Address]common.Hash{}
		}

		if s.submissionEnabled && s.TxOpts == nil {
			s.TxOpts, err = CreateTxOptsFromAuth(c.Auth, s.Context, s.EthConn)
			if err != nil {
				return err
			}
		}
		s.endBlock, err = GetDefaultBlockValue(c.DefaultBlock)
		return nil
	})
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

// state transition engine for claim:computed -> claim:submitted
type claimSubmission struct {
	s *Service
}
type claimSubmissionState ClaimRow
type claimSubmissionEvent iconsensus.IConsensusClaimSubmission

func (me *claimSubmission) CheckStateConstraint(
	state *claimSubmissionState,
) error {
	if state.Status == model.EpochStatus_ClaimComputed && state.ClaimHash == nil {
		me.s.Logger.Error("Constraint violation on ClaimHash", "claim", state)
		return fsm.ErrStateConstraintViolation
	}
	if (state.Status == model.EpochStatus_ClaimSubmitted || state.Status == model.EpochStatus_ClaimAccepted) && state.ClaimTransactionHash == nil {
		me.s.Logger.Error("Constraint violation on ClaimTransactionHash", "claim", state)
		return fsm.ErrStateConstraintViolation
	}
	if state.IApplicationAddress == (common.Address{}) {
		me.s.Logger.Error("Constraint violation on IApplicationAddress", "claim", state)
		return fsm.ErrStateConstraintViolation
	}
	return nil
}

func (me *claimSubmission) CheckStateTransitionConstraint(
	prev *claimSubmissionState,
	curr *claimSubmissionState,
) error {
	if prev.ApplicationID != curr.ApplicationID {
		me.s.Logger.Error("Constraint violation on ApplicationID", "prev", prev, "curr", curr)
		return fsm.ErrStateConstraintViolation
	}
	if prev.LastBlock > curr.LastBlock {
		me.s.Logger.Error("Constraint violation on LastBlock", "prev", prev, "curr", curr)
		return fsm.ErrStateConstraintViolation
	}
	if prev.FirstBlock > curr.FirstBlock {
		me.s.Logger.Error("Constraint violation on FirstBlock", "prev", prev, "curr", curr)
		return fsm.ErrStateConstraintViolation
	}
	if prev.Index > curr.Index {
		me.s.Logger.Error("Constraint violation on Index", "prev", prev, "curr", curr)
		return fsm.ErrStateConstraintViolation
	}
	if prev.VirtualIndex+1 != curr.VirtualIndex {
		me.s.Logger.Error("Constraint violation on VirtualIndex", "prev", prev, "curr", curr)
		return fsm.ErrStateConstraintViolation
	}
	return nil
}

func (me *claimSubmission) CheckEventConstraint(event *claimSubmissionEvent) error {
	if event == nil {
		me.s.Logger.Error("Constraint violation: nil event")
		return fsm.ErrStateConstraintViolation
	}
	return nil
}

func (me *claimSubmission) CheckEventTransitionConstraint(
	state *claimSubmissionState,
	event *claimSubmissionEvent,
) error {
	if state.IApplicationAddress != event.AppContract {
		me.s.Logger.Error("Constraint violation on IApplicationAddress", "state", state, "event", event)
		return fsm.ErrEventConstraintViolation
	}
	if *state.ClaimHash != event.Claim {
		me.s.Logger.Error("Constraint violation on ClaimHash", "state", state, "event", event)
		return fsm.ErrEventConstraintViolation
	}
	if state.LastBlock != event.LastProcessedBlockNumber.Uint64() {
		me.s.Logger.Error("Constraint violation on LastBlock", "state", state, "event", event)
		return fsm.ErrEventConstraintViolation
	}
	return nil
}

func (me *claimSubmission) FetchEventAndSucc(state *claimSubmissionState) (
	*claimSubmissionEvent,
	*claimSubmissionEvent,
	error,
) {
	_, prev, curr, err := me.s.FindClaimSubmissionEventAndSucc((*ClaimRow)(state))
	return (*claimSubmissionEvent)(prev), (*claimSubmissionEvent)(curr), err
}

func (s *Service) submitClaimsAndUpdateDatabase(se sideEffects) []error {
	errs := []error{}
	prevClaims, currClaims, err := se.selectClaimSubmissionCandidatePairsPerApp()
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
			err = se.updateEpochWithSubmittedClaim(claim, receipt.TxHash)
			if err != nil {
				errs = append(errs, err)
				return errs
			}
			s.Logger.Info("Claim submitted",
				"app", claim.IApplicationAddress,
				"claim_hash", fmt.Sprintf("%x", claim.ClaimHash),
				"last_block", claim.LastBlock,
				"tx", txHash)
			delete(currClaims, key)
		} else {
			s.Logger.Warn("expected claim in flight to be in currClaims.",
				"tx", receipt.TxHash)
		}
		delete(s.claimsInFlight, key)
	}

	// submit/update computed claims
	for key, currClaim := range currClaims {
		prevClaim := prevClaims[key]
		if _, isInFlight := s.claimsInFlight[key]; isInFlight {
			continue
		}

		action, _, currEvent, err := fsm.TryTransition(
			&claimSubmission{s},
			(*claimSubmissionState)(prevClaim),
			(*claimSubmissionState)(currClaim),
		)

		// TODO: disable dapp on constraint violation

		if err != nil {
			errs = append(errs, err)
			continue
		}

		switch action {
		case fsm.Submit:
			if s.submissionEnabled {
				if prevClaim != nil && prevClaim.Status != EpochStatus_ClaimAccepted {
					s.Logger.Debug("Waiting previous claim to be accepted before submitting new one. Previous:",
						"app", prevClaim.IApplicationAddress,
						"claim_hash", fmt.Sprintf("%x", prevClaim.ClaimHash),
						"last_block", prevClaim.LastBlock,
					)
					continue
				}
				ic, err := iconsensus.NewIConsensus(currClaim.IConsensusAddress, s.EthConn)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				txHash, err := se.submitClaimToBlockchain(ic, currClaim)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				s.claimsInFlight[key] = txHash
			} else {
				s.Logger.Debug("Claim submission disabled. Doing nothing",
					"app", currClaim.IApplicationAddress,
					"claim_hash", fmt.Sprintf("%x", currClaim.ClaimHash),
					"last_block", currClaim.LastBlock,
				)
			}

		case fsm.Update:
			txHash := currEvent.Raw.TxHash
			err = se.updateEpochWithSubmittedClaim(currClaim, txHash)
			if err != nil {
				errs = append(errs, err)
				continue
			}
		}
	}
	return errs
}

func (s *Service) Start(context context.Context, ready chan<- struct{}) error {
	ready <- struct{}{}
	return s.Serve()
}
func (s *Service) String() string {
	return s.Name
}
