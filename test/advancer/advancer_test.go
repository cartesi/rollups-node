// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/node/advancer"
	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/node/nodemachine"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/test/snapshot"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var appAddress model.Address

func TestAdvancer(t *testing.T) {
	require := require.New(t)

	// Creates the snapshot.
	script := "ioctl-echo-loop --vouchers=1 --notices=3 --reports=5 --verbose=1"
	snapshot, err := snapshot.FromScript(script, uint64(1_000_000_000))
	require.Nil(err)
	defer func() { require.Nil(snapshot.Close()) }()

	// Starts the server.
	verbosity := rollupsmachine.ServerVerbosityInfo
	address, err := rollupsmachine.StartServer(verbosity, 0, os.Stdout, os.Stderr)
	require.Nil(err)

	// Loads the rollupsmachine.
	config := &emulator.MachineRuntimeConfig{}
	rollupsMachine, err := rollupsmachine.Load(snapshot.Dir, address, config)
	require.Nil(err)
	require.NotNil(rollupsMachine)

	// Wraps the rollupsmachine with nodemachine.
	nodeMachine := nodemachine.New(rollupsMachine, time.Minute, 10)
	require.Nil(err)
	require.NotNil(nodeMachine)
	defer func() { require.Nil(nodeMachine.Close()) }()

	// Creates the machine pool.
	appAddress = common.HexToAddress("deadbeef")
	machines := advancer.Machines{appAddress: nodeMachine}

	// Creates the background context.
	ctx := context.Background()

	// Create the database container.
	databaseContainer, err := newDatabaseContainer(ctx)
	require.Nil(err)
	defer func() { require.Nil(databaseContainer.Terminate(ctx)) }()

	// Setups the database.
	database, err := newDatabase(ctx, databaseContainer)
	require.Nil(err)
	err = populateDatabase(ctx, database)
	require.Nil(err)
	defer database.Close()

	// Creates the advancer's repository.
	repository := &repository.AdvancerRepository{Database: database}

	// Creates the advancer.
	advancer, err := advancer.New(machines, repository)
	require.Nil(err)
	require.NotNil(advancer)
	poller, err := advancer.Poller(5 * time.Second)
	require.Nil(err)
	require.NotNil(poller)

	// Starts the advancer in a separate goroutine.
	done := make(chan struct{}, 1)
	go func() {
		ready := make(chan struct{}, 1)
		err = poller.Start(ctx, ready)
		<-ready
		require.Nil(err, "%v", err)
		done <- struct{}{}
	}()

	// Orders the advancer to stop after some time has passed.
	time.Sleep(5 * time.Second)
	poller.Stop()

wait:
	for {
		select {
		case <-done:
			fmt.Println("Done!")
			break wait
		default:
			fmt.Println("Waiting...")
			time.Sleep(time.Second)
		}
	}
}

func newDatabaseContainer(ctx context.Context) (*postgres.PostgresContainer, error) {
	dbName := "cartesinode"
	dbUser := "admin"
	dbPassword := "password"

	// Start the postgres container
	container, err := postgres.Run(
		ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second)),
	)

	return container, err
}

// func newLocalDatabase(ctx context.Context) (*repository.Database, error) {
// 	endpoint := "postgres://renan:renan@localhost:5432/renan?sslmode=disable"
//
// 	schemaManager, err := repository.NewSchemaManager(endpoint)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	err = schemaManager.DeleteAll()
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	err = schemaManager.Upgrade()
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	database, err := repository.Connect(ctx, endpoint)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	return database, nil
// }

func newDatabase(
	ctx context.Context,
	container *postgres.PostgresContainer,
) (*repository.Database, error) {
	endpoint, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, err
	}

	schemaManager, err := repository.NewSchemaManager(endpoint)
	if err != nil {
		return nil, err
	}

	err = schemaManager.Upgrade()
	if err != nil {
		return nil, err
	}

	database, err := repository.Connect(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	return database, nil
}

func populateDatabase(ctx context.Context, database *repository.Database) (err error) {
	application := &model.Application{
		ContractAddress:    appAddress,
		TemplateHash:       [32]byte{},
		SnapshotURI:        "invalid",
		LastProcessedBlock: 0,
		EpochLength:        0,
		Status:             "RUNNING",
	}
	err = database.InsertApplication(ctx, application)
	if err != nil {
		return
	}

	inputs := []*model.Input{{
		CompletionStatus: model.InputStatusAccepted,
		RawData:          []byte("first input"),
		AppAddress:       appAddress,
	}, {
		CompletionStatus: model.InputStatusNone,
		RawData:          []byte("second input"),
		AppAddress:       appAddress,
	}, {
		CompletionStatus: model.InputStatusNone,
		RawData:          []byte("third input"),
		AppAddress:       appAddress,
	}}

	for i, input := range inputs {
		input.Index = uint64(i)
		input.BlockNumber = uint64(i)
		input.RawData, err = rollupsmachine.Input{Data: input.RawData}.Encode()
		if err != nil {
			return
		}
		err = database.InsertInput(ctx, input)
		if err != nil {
			return
		}
	}

	return
}
