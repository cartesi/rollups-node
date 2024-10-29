// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"testing"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/cartesi/rollups-node/test/tooling/db"
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

	t.Run("EmptySelectComputedClaims", func(t *testing.T) {
		require, database, err := setup(t, ctx)

		computedClaims, err := database.SelectComputedClaims(ctx)
		require.Nil(err)
		require.Empty(computedClaims)
	})

	t.Run("SelectComputedClaims", func(t *testing.T) {
		require, database, err := setup(t, ctx)

		app := Application{
			Id:                 1,
			ContractAddress:    common.HexToAddress("deadbeef"),
			TemplateHash:       common.HexToHash("deadbeef"),
			LastProcessedBlock: 1,
			Status:             ApplicationStatusRunning,
			IConsensusAddress:  common.HexToAddress("ffffff"),
		}
		_, err = database.InsertApplication(ctx, &app)
		require.Nil(err)

		lastBlock := []uint64{99, 200}
		epochs := []Epoch{
			{
				Id:              1,
				Index:           0,
				FirstBlock:      0,
				LastBlock:       lastBlock[0],
				AppAddress:      app.ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			},{
				Id:              2,
				Index:           1,
				FirstBlock:      lastBlock[0]+1,
				LastBlock:       lastBlock[1],
				AppAddress:      app.ContractAddress,
				ClaimHash:       &common.Hash{},
				TransactionHash: nil,
				Status:          EpochStatusClaimComputed,
			},
		}

		for _, epoch := range(epochs) {
			_, err = database.InsertEpoch(ctx, &epoch)
			require.Nil(err)
		}

		computedClaims, err := database.SelectComputedClaims(ctx)
		require.Nil(err)
		require.Len(computedClaims, 2)

		for i, computedClaim := range(computedClaims) {
			require.Equal(computedClaim.EpochID, epochs[i].Id)
			require.Equal(computedClaim.Hash, *epochs[i].ClaimHash)
			require.Equal(computedClaim.AppContractAddress, app.ContractAddress)
			require.Equal(computedClaim.AppIConsensusAddress, app.IConsensusAddress)
		}
	})

	t.Run("TestUpdateEpochWithSubmittedClaim", func(t *testing.T) {
		require, database, err := setup(t, ctx)

		app := Application{
			Id:                 1,
			ContractAddress:    common.HexToAddress("deadbeef"),
			TemplateHash:       common.HexToHash("deadbeef"),
			LastProcessedBlock: 1,
			Status:             ApplicationStatusRunning,
			IConsensusAddress:  common.HexToAddress("ffffff"),
		}
		_, err = database.InsertApplication(ctx, &app)
		require.Nil(err)

		epoch := Epoch{
			Id:              1,
			Index:           0,
			FirstBlock:      0,
			LastBlock:       100,
			AppAddress:      app.ContractAddress,
			ClaimHash:       &common.Hash{},
			TransactionHash: nil,
			Status:          EpochStatusClaimComputed,
		}

		id, err := database.InsertEpoch(ctx, &epoch)
		require.Nil(err)

		transactionHash := common.HexToHash("0x10")
		err = database.UpdateEpochWithSubmittedClaim(ctx, id, transactionHash)
		require.Nil(err)

		updatedEpoch, err := database.GetEpoch(ctx, epoch.Index, epoch.AppAddress)
		require.Nil(err)
		require.Equal(updatedEpoch.Status, EpochStatusClaimSubmitted)
		require.Equal(updatedEpoch.TransactionHash, &transactionHash)
	})

	t.Run("TestFailUpdateEpochWithSubmittedClaim", func(t *testing.T) {
		require, database, err := setup(t, ctx)

		app := Application{
			Id:                 1,
			ContractAddress:    common.HexToAddress("deadbeef"),
			TemplateHash:       common.HexToHash("deadbeef"),
			LastProcessedBlock: 1,
			Status:             ApplicationStatusRunning,
			IConsensusAddress:  common.HexToAddress("ffffff"),
		}
		_, err = database.InsertApplication(ctx, &app)
		require.Nil(err)

		transactionHash := common.HexToHash("0x10")
		epoch := Epoch{
			Id:              1,
			Index:           0,
			FirstBlock:      0,
			LastBlock:       100,
			AppAddress:      app.ContractAddress,
			ClaimHash:       &common.Hash{},
			TransactionHash: &transactionHash,
			Status:          EpochStatusClaimSubmitted,
		}

		id, err := database.InsertEpoch(ctx, &epoch)
		require.Nil(err)

		err = database.UpdateEpochWithSubmittedClaim(ctx, id, transactionHash)
		require.Equal(err, ErrNoUpdate)
	})
}
