// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"

	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/ethereum/go-ethereum/common"

	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/stretchr/testify/require"
)

func TestMachineRepository(t *testing.T) {
	ctx := context.Background()

	t.Run("GetMachineConfigurations", func(t *testing.T) {
		require := require.New(t)

		var err error
		endpoint, err := db.GetPostgresTestEndpoint()
		require.Nil(err)

		err = db.SetupTestPostgres(endpoint)
		require.Nil(err)

		database, err := Connect(ctx, endpoint)
		require.Nil(err)
		require.NotNil(database)

		apps, _, _, _, err := populate2(database)
		require.Nil(err)
		require.Len(apps, 3)

		repository := &MachineRepository{Database: database}

		// only running apps
		res, err := repository.GetMachineConfigurations(ctx)
		require.Nil(err)
		require.Len(res, 2)

		var config1, config2 *MachineConfig
		for _, config := range res {
			if config.AppAddress == apps[1].ContractAddress {
				config2 = config
			} else if config.AppAddress == apps[2].ContractAddress {
				config1 = config
			}
		}
		require.NotNil(config1)
		require.NotNil(config2)

		require.Equal(apps[1].ContractAddress, config2.AppAddress)
		require.Nil(config2.SnapshotInputIndex)
		require.Equal("path/to/template/uri/1", config2.SnapshotPath)

		require.Equal(apps[2].ContractAddress, config1.AppAddress)
		require.Nil(config1.SnapshotInputIndex)
		require.Equal("path/to/template/uri/2", config1.SnapshotPath)
	})

	t.Run("GetProcessedInputs", func(t *testing.T) {
		t.Skip("TODO")
	})

	t.Run("GetUnprocessedInputs", func(t *testing.T) {
		t.Skip("TODO")
	})

	t.Run("StoreAdvanceResult", func(t *testing.T) {
		t.Skip("TODO")
	})

	t.Run("UpdateEpochs", func(t *testing.T) {
		require := require.New(t)

		var err error
		endpoint, err := db.GetPostgresTestEndpoint()
		require.Nil(err)

		err = db.SetupTestPostgres(endpoint)
		require.Nil(err)

		database, err := Connect(ctx, endpoint)
		require.Nil(err)
		require.NotNil(database)

		app, _, _, err := populate1(database)
		require.Nil(err)
		repository := &MachineRepository{Database: database}

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

func populate1(database *Database) (*Application, []*Epoch, []*Input, error) {
	ctx := context.Background()

	app := &Application{
		ContractAddress:    common.HexToAddress("deadbeef"),
		IConsensusAddress:  common.HexToAddress("beefdead"),
		TemplateHash:       [32]byte{},
		LastProcessedBlock: 0,
		Status:             "RUNNING",
	}

	_, err := database.InsertApplication(ctx, app)
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

// ------------------------------------------------------------------------------------------------

func populate2(database *Database) ([]*Application, []*Epoch, []*Input, []*Snapshot, error) {
	ctx := context.Background()

	apps := []*Application{{
		ContractAddress: common.HexToAddress("dead"),
		TemplateUri:     "path/to/template/uri/0",
		Status:          ApplicationStatusNotRunning,
	}, {
		ContractAddress: common.HexToAddress("beef"),
		TemplateUri:     "path/to/template/uri/1",
		Status:          ApplicationStatusRunning,
	}, {
		ContractAddress: common.HexToAddress("bead"),
		TemplateUri:     "path/to/template/uri/2",
		Status:          ApplicationStatusRunning,
	}}
	if err := database.InsertApps(ctx, apps); err != nil {
		return nil, nil, nil, nil, err
	}

	epochs := []*Epoch{{
		Index:      0,
		Status:     EpochStatusClosed,
		AppAddress: apps[1].ContractAddress,
	}, {
		Index:      1,
		Status:     EpochStatusClosed,
		AppAddress: apps[1].ContractAddress,
	}, {
		Status:     EpochStatusClosed,
		AppAddress: apps[2].ContractAddress,
	}}
	err := database.InsertEpochs(ctx, epochs)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	inputs := []*Input{{
		Index:            0,
		CompletionStatus: InputStatusAccepted,
		RawData:          []byte("first"),
		AppAddress:       apps[1].ContractAddress,
		EpochId:          epochs[0].Id,
	}, {
		Index:            6,
		CompletionStatus: InputStatusAccepted,
		RawData:          []byte("second"),
		AppAddress:       apps[1].ContractAddress,
		EpochId:          epochs[1].Id,
	}}
	err = database.InsertInputs(ctx, inputs)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	snapshots := []*Snapshot{{
		URI:        "path/to/snapshot/1",
		InputId:    inputs[0].Id,
		AppAddress: apps[1].ContractAddress,
	}, {
		URI:        "path/to/snapshot/2",
		InputId:    inputs[1].Id,
		AppAddress: apps[1].ContractAddress,
	}}
	err = database.InsertSnapshots(ctx, snapshots)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return apps, epochs, inputs, snapshots, nil
}

// ------------------------------------------------------------------------------------------------

func (pg *Database) InsertApps(ctx context.Context, apps []*Application) error {
	var err error
	for _, app := range apps {
		app.Id, err = pg.InsertApplication(ctx, app)
		if err != nil {
			return err
		}
	}
	return nil
}

func (pg *Database) InsertEpochs(ctx context.Context, epochs []*Epoch) error {
	var err error
	for _, epoch := range epochs {
		epoch.Id, err = pg.InsertEpoch(ctx, epoch)
		if err != nil {
			return err
		}
	}
	return nil
}

func (pg *Database) InsertInputs(ctx context.Context, inputs []*Input) error {
	var err error
	for _, input := range inputs {
		input.Id, err = pg.InsertInput(ctx, input)
		if err != nil {
			return err
		}
	}
	return nil
}

func (pg *Database) InsertSnapshots(ctx context.Context, snapshots []*Snapshot) error {
	var err error
	for _, snapshot := range snapshots {
		snapshot.Id, err = pg.InsertSnapshot(ctx, snapshot)
		if err != nil {
			return err
		}
	}
	return nil
}
