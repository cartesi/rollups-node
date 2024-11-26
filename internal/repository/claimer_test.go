// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	// "github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func setup(t *testing.T, ctx context.Context) (*require.Assertions, *Database, error) {
	require := require.New(t)

	var err error
	endpoint, err := db.GetPostgresTestEndpoint()
	require.Nil(err)

	err = db.SetupTestPostgres(endpoint)
	require.Nil(err)

	database, err := Connect(ctx, endpoint)
	require.Nil(err)
	require.NotNil(database)

	return require, database, nil
}

func TestClaimerRepository(t *testing.T) {
	ctx := context.Background()

	// Must return an empty array for a database with no computed claims
	t.Run("EmptyArrayOnCleanDB", func(t *testing.T) {
		require, database, err := setup(t, ctx)
		require.Nil(err)

		computed, err := database.SelectOldestComputedClaimPerApp(ctx)
		require.Nil(err)
		require.Empty(computed)
	})

	// Check that we select the correct epochs on a "complex" situation.
	// The query must return 0 or 1 entries per application with the smallest
	// epoch index and status == 'COMPUTED_CLAIM'.
	//
	// Application 0 has 4 epochs, 1 already accepted (with lowest index)
	// and 3 computed (the candidates to be selected).
	//
	// Application 1 has no computed epochs and must not be returned.
	//
	// Application 2 has 3 epochs, all computed.
	//
	// We expect 2 values on the array.
	// 1) {index = 1, apps[0], ...}
	// 2) {index = 2, apps[1], ...}
	//
	t.Run("MustRetrieveOldestComputedClaimForEachApp", func(t *testing.T) {
		require, database, err := setup(t, ctx)
		require.Nil(err)

		apps := []Application{
			{
				Id:                 0,
				ContractAddress:    common.HexToAddress("0"),
				Status:             ApplicationStatusRunning,
			}, {
				Id:                 1,
				ContractAddress:    common.HexToAddress("1"),
				Status:             ApplicationStatusRunning,
			}, {
				Id:                 2,
				ContractAddress:    common.HexToAddress("2"),
				Status:             ApplicationStatusRunning,
			},
		}
		for _, app := range(apps) {
			_, err = database.InsertApplication(ctx, &app)
			require.Nil(err)
		}

		epochs := []Epoch{
			// epochs of apps[0]
			{
				Index:           3, // not this
				AppAddress:      apps[0].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			}, {
				Index:           2, // not this
				AppAddress:      apps[0].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			}, {
				Index:           1, // this!
				AppAddress:      apps[0].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			}, {
				Index:           0, // not this
				AppAddress:      apps[0].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimAccepted,
			},

			// epochs of apps[1]
			{
				Index:           0,
				AppAddress:      apps[1].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimAccepted,
			},

			// epochs of apps[2]
			{
				Index:           3, // not this
				AppAddress:      apps[2].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			}, {
				Index:           2, // this!
				AppAddress:      apps[2].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			}, {
				Index:           4, // not this
				AppAddress:      apps[2].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			},
		}
		for _, epoch := range(epochs) {
			_, err = database.InsertEpoch(ctx, &epoch)
			require.Nil(err)
		}

		computed, err := database.SelectOldestComputedClaimPerApp(ctx)
		require.Nil(err)
		require.Len(computed, 2)

		require.Equal(computed[apps[0].ContractAddress].EpochIndex, uint64(1))
		require.Equal(computed[apps[0].ContractAddress].AppContractAddress,
			apps[0].ContractAddress)

		require.Equal(computed[apps[2].ContractAddress].EpochIndex, uint64(2))
		require.Equal(computed[apps[2].ContractAddress].AppContractAddress,
			apps[2].ContractAddress)
	})

	t.Run("MustRetrieveNewestComputedClaimForEachApp", func(t *testing.T) {
		require, database, err := setup(t, ctx)
		require.Nil(err)

		apps := []Application{
			{
				Id:                 0,
				ContractAddress:    common.HexToAddress("0"),
				Status:             ApplicationStatusRunning,
			}, {
				Id:                 1,
				ContractAddress:    common.HexToAddress("1"),
				Status:             ApplicationStatusRunning,
			}, {
				Id:                 2,
				ContractAddress:    common.HexToAddress("2"),
				Status:             ApplicationStatusRunning,
			},
		}
		for _, app := range(apps) {
			_, err = database.InsertApplication(ctx, &app)
			require.Nil(err)
		}

		epochs := []Epoch{
			// epochs of apps[0]
			{
				Index:           3, // not this
				AppAddress:      apps[0].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			}, {
				Index:           2, // not this
				AppAddress:      apps[0].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			}, {
				Index:           1, // this!
				AppAddress:      apps[0].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimAccepted,
			}, {
				Index:           0, // not this
				AppAddress:      apps[0].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimAccepted,
			},

			// epochs of apps[1]
			{
				Index:           0, // not this
				AppAddress:      apps[1].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			},

			// epochs of apps[2]
			{
				Index:           3, // not this
				AppAddress:      apps[2].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			}, {
				Index:           2, // this!
				AppAddress:      apps[2].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimAccepted,
			}, {
				Index:           4, // not this
				AppAddress:      apps[2].ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			},
		}
		for _, epoch := range(epochs) {
			_, err = database.InsertEpoch(ctx, &epoch)
			require.Nil(err)
		}

		accepted, err := database.SelectNewestAcceptedClaimPerApp(ctx)
		require.Nil(err)
		require.Len(accepted, 2)

		require.Equal(accepted[apps[0].ContractAddress].EpochIndex, uint64(1))
		require.Equal(accepted[apps[0].ContractAddress].AppContractAddress,
			apps[0].ContractAddress)

		require.Equal(accepted[apps[2].ContractAddress].EpochIndex, uint64(2))
		require.Equal(accepted[apps[2].ContractAddress].AppContractAddress,
			apps[2].ContractAddress)
	})
}
