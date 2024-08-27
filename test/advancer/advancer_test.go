// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/cartesi/rollups-node/internal/node/model"

	"github.com/cartesi/rollups-node/internal/node/advancer"
	"github.com/cartesi/rollups-node/internal/nodemachine"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
	"github.com/cartesi/rollups-node/test/snapshot"
	"github.com/cartesi/rollups-node/test/tooling/db"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestAdvancer(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Setups the database.
	endpoint, err := db.Setup(ctx)
	require.Nil(err)
	database, err := repository.Connect(ctx, endpoint)
	require.Nil(err)
	require.NotNil(database)
	app, _, _, err := populate(database)
	require.Nil(err)

	// Creates the snapshot.
	script := "ioctl-echo-loop --vouchers=1 --notices=3 --reports=5 --verbose=1"
	snapshot, err := snapshot.FromScript(script, uint64(1_000_000_000))
	require.Nil(err)
	defer func() { require.Nil(snapshot.Close()) }()

	// Starts the server.
	verbosity := cartesimachine.ServerVerbosityInfo
	address, err := cartesimachine.StartServer(verbosity, 0, os.Stdout, os.Stderr)
	require.Nil(err)

	// Loads the cartesimachine.
	config := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cartesimachine.Load(ctx, snapshot.Path(), address, config)
	require.Nil(err)
	require.NotNil(cartesiMachine)

	// Wraps the cartesimachine with rollupsmachine.
	rollupsMachine, err := rollupsmachine.New(ctx, cartesiMachine, 50_000_000, 5_000_000_000)
	require.Nil(err)
	require.NotNil(rollupsMachine)

	// Wraps the rollupsmachine with nodemachine.
	nodeMachine, err := nodemachine.New(rollupsMachine, 0, time.Minute, time.Minute, 10)
	require.Nil(err)
	require.NotNil(nodeMachine)
	defer func() { require.Nil(nodeMachine.Close()) }()

	// Creates the machine pool.
	machines := advancer.Machines{app.ContractAddress: nodeMachine}

	// Creates the advancer's repository.
	repository := &repository.AdvancerRepository{Database: database}

	// Creates the advancer.
	advancer, err := advancer.New(machines, repository)
	require.Nil(err)
	require.NotNil(advancer)

	// Creates the poller from the advancer.
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

	t.Run("AssertThings!", func(t *testing.T) {
		t.Skip("TODO")
	})
}

func populate(database *repository.Database) (*Application, []*Epoch, []*Input, error) {
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
