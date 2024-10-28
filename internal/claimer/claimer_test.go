// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"iter"
	"log/slog"
	"testing"

	. "github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	. "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type ServiceMock struct {
	mock.Mock
	Service
}

func (m *ServiceMock) submitClaimToBlockchain(
	instance *iconsensus.IConsensus,
	signer *bind.TransactOpts,
	claim *ComputedClaim,
) (Hash, error) {
	args := m.Called(nil, nil, claim)
	return args.Get(0).(Hash), args.Error(1)
}

func (m *ServiceMock) selectComputedClaims() ([]ComputedClaim, error) {
	args := m.Called()
	return args.Get(0).([]ComputedClaim), args.Error(1)
}

func (m *ServiceMock) updateEpochWithSubmittedClaim(
	dbConn *Database,
	context context.Context,
	claim *ComputedClaim,
	txHash Hash,
) error {
	args := m.Called(nil, nil, claim, txHash)
	return args.Error(0)
}

func (m *ServiceMock) enumerateSubmitClaimEventsSince(
	ethConn *ethclient.Client,
	context context.Context,
	appIConsensusAddr Address,
	epochLastBlock uint64,
) (
	iter.Seq2[*iconsensus.IConsensusClaimSubmission, error],
	*iconsensus.IConsensus,
	error,
) {
	args := m.Called()
	return args.Get(0).(iter.Seq2[*iconsensus.IConsensusClaimSubmission, error]),
		args.Get(1).(*iconsensus.IConsensus),
		args.Error(2)
}

func (m *ServiceMock) pollTransaction(txHash Hash) (bool, *types.Receipt, error) {
	args := m.Called(txHash)
	return args.Bool(0),
		args.Get(1).(*types.Receipt),
		args.Error(2)
}

func iteratorFromList[E any](slice []E) iter.Seq2[*E, error] {
	return func(yield func(*E, error) bool) {
		for _, element := range slice {
			cont := yield(&element, nil)
			if !cont {
				return
			}
		}
	}
}

// //////////////////////////////////////////////////////////////////////////////
// Test
// //////////////////////////////////////////////////////////////////////////////

// Do notghing when there are no claims to process
func TestEmptySelectComputedClaimsDoesNothing(t *testing.T) {
	m := &ServiceMock{}
	m.ClaimsInFlight = make(map[Hash]Hash)

	m.On("selectComputedClaims").Return([]ComputedClaim{}, nil)

	m.submitClaimsAndUpdateDatabase(m)

	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 0)
	m.AssertNumberOfCalls(t, "selectComputedClaims", 1)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 0)
	m.AssertNumberOfCalls(t, "enumerateSubmitClaimEventsSince", 0)
	m.AssertNumberOfCalls(t, "pollTransaction", 0)
}

// Got a claim.
// Submit if new (by checking the event logs)
func TestSubmitNewClaim(t *testing.T) {
	m := &ServiceMock{}

	newClaimHash := HexToHash("0x01")
	newClaimTxHash := HexToHash("0x10")
	newClaim := ComputedClaim{
		Hash: newClaimHash,
	}
	m.ClaimsInFlight = map[Hash]Hash{}
	m.On("selectComputedClaims").Return([]ComputedClaim{
		newClaim,
	}, nil)
	m.On("submitClaimToBlockchain", nil, nil, &newClaim).
		Return(newClaimTxHash, nil)
	m.On("enumerateSubmitClaimEventsSince").
		Return(iteratorFromList([]iconsensus.IConsensusClaimSubmission{}), &iconsensus.IConsensus{}, nil)
	m.On("pollTransaction", newClaimTxHash).
		Return(false, &types.Receipt{}, nil)
	assert.Equal(t, len(m.ClaimsInFlight), 0)

	m.submitClaimsAndUpdateDatabase(m)

	assert.Equal(t, len(m.ClaimsInFlight), 1)
	m.AssertNumberOfCalls(t, "enumerateSubmitClaimEventsSince", 1)
	m.AssertNumberOfCalls(t, "pollTransaction", 0)
	m.AssertNumberOfCalls(t, "selectComputedClaims", 1)
	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 1)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 0)
}

// Query the blockchain for the submitClaim transaction, it may not be ready yet
func TestClaimInFlightNotReadyDoesNothing(t *testing.T) {
	m := &ServiceMock{}

	claimInFlightHash := HexToHash("0x01")
	claimInFlightTxHash := HexToHash("0x10")
	claimInFlight := ComputedClaim{
		Hash: claimInFlightHash,
	}
	m.ClaimsInFlight = map[Hash]Hash{
		claimInFlightHash: claimInFlightTxHash,
	}
	m.On("selectComputedClaims").Return([]ComputedClaim{
		claimInFlight,
	}, nil)
	m.On("pollTransaction", claimInFlightTxHash).
		Return(false, &types.Receipt{}, nil)
	assert.Equal(t, len(m.ClaimsInFlight), 1)

	m.submitClaimsAndUpdateDatabase(m)

	assert.Equal(t, len(m.ClaimsInFlight), 1)
	m.AssertNumberOfCalls(t, "enumerateSubmitClaimEventsSince", 0)
	m.AssertNumberOfCalls(t, "pollTransaction", 1)
	m.AssertNumberOfCalls(t, "selectComputedClaims", 1)
	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 0)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 0)
}

// Update ClaimsInFlight and the database when a submitClaim transaction is completed
func TestUpdateClaimInFlightViaPollTransaction(t *testing.T) {
	m := &ServiceMock{}
	m.Logger = slog.Default()

	claimInFlightHash := HexToHash("0x01")
	claimInFlightTxHash := HexToHash("0x10")
	claimInFlight := ComputedClaim{
		Hash: claimInFlightHash,
	}
	m.ClaimsInFlight = map[Hash]Hash{
		claimInFlightHash: claimInFlightTxHash,
	}
	m.On("selectComputedClaims").Return([]ComputedClaim{
		claimInFlight,
	}, nil)
	m.On("pollTransaction", claimInFlightTxHash).
		Return(true, &types.Receipt{TxHash: claimInFlightTxHash}, nil)
	m.On("updateEpochWithSubmittedClaim",
		nil, nil, &claimInFlight, claimInFlightTxHash).Return(nil)
	assert.Equal(t, len(m.ClaimsInFlight), 1)

	m.submitClaimsAndUpdateDatabase(m)

	assert.Equal(t, len(m.ClaimsInFlight), 0)
	m.AssertNumberOfCalls(t, "enumerateSubmitClaimEventsSince", 0)
	m.AssertNumberOfCalls(t, "pollTransaction", 1)
	m.AssertNumberOfCalls(t, "selectComputedClaims", 1)
	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 0)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 1)
}

// This shouldn't happen normally,
// but a submitClaim transaction may be on the blockchain but the database not know it.
// The blockchain is the source of truth in any case.
// So search for the transaction in the event logs.
// And if found, update the database.
func TestUpdateClaimViaEventLog(t *testing.T) {
	m := &ServiceMock{}

	existingClaimHash := HexToHash("0x01")
	existingClaimTxHash := HexToHash("0x10")
	existingClaim := ComputedClaim{
		Hash: existingClaimHash,
	}
	m.ClaimsInFlight = map[Hash]Hash{}
	m.On("selectComputedClaims").Return([]ComputedClaim{
		existingClaim,
	}, nil)
	m.On("updateEpochWithSubmittedClaim",
		nil, nil, &existingClaim, existingClaimTxHash).Return(nil)
	m.On("enumerateSubmitClaimEventsSince").
		Return(iteratorFromList([]iconsensus.IConsensusClaimSubmission{
			{
				Claim: existingClaim.Hash,
				Raw: types.Log{
					TxHash: existingClaimTxHash,
				},
			},
		}), &iconsensus.IConsensus{}, nil)
	assert.Equal(t, len(m.ClaimsInFlight), 0)

	m.submitClaimsAndUpdateDatabase(m)

	assert.Equal(t, len(m.ClaimsInFlight), 0)
	m.AssertNumberOfCalls(t, "enumerateSubmitClaimEventsSince", 1)
	m.AssertNumberOfCalls(t, "pollTransaction", 0)
	m.AssertNumberOfCalls(t, "selectComputedClaims", 1)
	m.AssertNumberOfCalls(t, "submitClaimToBlockchain", 0)
	m.AssertNumberOfCalls(t, "updateEpochWithSubmittedClaim", 1)
}
