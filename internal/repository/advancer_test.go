// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"testing"

	. "github.com/cartesi/rollups-node/internal/node/model"

	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/require"
)

func TestAdvancerRepository(t *testing.T) {
	ctx := context.Background()

	endpoint, err := db.Setup(ctx)
	require.Nil(t, err)

	database, err := Connect(ctx, endpoint)
	require.Nil(t, err)
	require.NotNil(t, database)

	app, _, _, err := populate(database)
	require.Nil(t, err)

	repository := &AdvancerRepository{Database: database}

	t.Run("GetUnprocessedInputs", func(t *testing.T) {
		t.Skip("TODO")
	})

	t.Run("StoreAdvanceResult", func(t *testing.T) {
		t.Skip("TODO")
	})

	t.Run("UpdateEpochs", func(t *testing.T) {
		require := require.New(t)

		err = repository.UpdateEpochs(ctx, app.ContractAddress)
		require.Nil(err)

		epoch0, err := repository.GetEpoch(ctx, 0, app.ContractAddress)
		require.Nil(err)
		require.NotNil(epoch0)

		epoch1, err := repository.GetEpoch(ctx, 1, app.ContractAddress)
		require.Nil(err)
		require.NotNil(epoch1)

		epoch2, err := repository.GetEpoch(ctx, 2, app.ContractAddress)
		require.Nil(err)
		require.NotNil(epoch2)

		epoch3, err := repository.GetEpoch(ctx, 3, app.ContractAddress)
		require.Nil(err)
		require.NotNil(epoch3)

		require.Equal(EpochStatusProcessedAllInputs, epoch0.Status)
		require.Equal(EpochStatusProcessedAllInputs, epoch1.Status)
		require.Equal(EpochStatusClosed, epoch2.Status)
		require.Equal(EpochStatusOpen, epoch3.Status)
	})
}

// ------------------------------------------------------------------------------------------------

func populate(database *Database) (*Application, []*Epoch, []*Input, error) {
	ctx := context.Background()

	app := &Application{
		ContractAddress:    common.HexToAddress("deadbeef"),
		IConsensusAddress:  common.HexToAddress("beefdead"),
		TemplateHash:       [32]byte{},
		LastProcessedBlock: 0,
		Status:             "RUNNING",
	}

	err := database.InsertApplication(ctx, app)
	if err != nil {
		return nil, nil, nil, err
	}

	epochs := []*Epoch{{
		FirstBlock: 0,
		LastBlock:  1,
		Status:     EpochStatusClosed,
	}, {
		FirstBlock: 2,
		LastBlock:  3,
		Status:     EpochStatusClosed,
	}, {
		FirstBlock: 4,
		LastBlock:  5,
		Status:     EpochStatusClosed,
	}, {
		FirstBlock: 6,
		LastBlock:  7,
		Status:     EpochStatusOpen,
	}}

	for i, epoch := range epochs {
		epoch.Index = uint64(i)
		epoch.AppAddress = app.ContractAddress
		epoch.Id, err = database.InsertEpoch(ctx, epoch)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	inputs := []*Input{{
		EpochId:          epochs[0].Id,
		CompletionStatus: InputStatusAccepted,
		RawData:          []byte("first input"),
	}, {
		EpochId:          epochs[0].Id,
		CompletionStatus: InputStatusRejected,
		RawData:          []byte("second input"),
	}, {
		EpochId:          epochs[1].Id,
		CompletionStatus: InputStatusException,
		RawData:          []byte("third input"),
	}, {
		EpochId:          epochs[1].Id,
		CompletionStatus: InputStatusAccepted,
		RawData:          []byte("fourth input"),
	}, {
		EpochId:          epochs[2].Id,
		CompletionStatus: InputStatusAccepted,
		RawData:          []byte("fifth input"),
	}, {
		EpochId:          epochs[2].Id,
		CompletionStatus: InputStatusNone,
		RawData:          []byte("sixth input"),
	}, {
		EpochId:          epochs[3].Id,
		CompletionStatus: InputStatusNone,
		RawData:          []byte("seventh input"),
	}}

	for i, input := range inputs {
		input.Index = uint64(i)
		input.BlockNumber = uint64(i)
		input.AppAddress = app.ContractAddress

		input.RawData, err = rollupsmachine.Input{Data: input.RawData}.Encode()
		if err != nil {
			return nil, nil, nil, err
		}

		input.Id, err = database.InsertInput(ctx, input)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return app, epochs, inputs, nil
}
