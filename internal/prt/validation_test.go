// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestLogErrorUnlessShutdown(t *testing.T) {
	tests := []struct {
		name      string
		stopping  bool
		err       error
		wantError bool
	}{
		{name: "ShutdownCancellation", stopping: true, err: context.Canceled, wantError: false},
		{name: "ShutdownDeadline", stopping: true, err: context.DeadlineExceeded, wantError: true},
		{
			name:      "ShutdownCancellationWithDeadline",
			stopping:  true,
			err:       errors.Join(context.Canceled, context.DeadlineExceeded),
			wantError: true,
		},
		{name: "RuntimeCancellation", stopping: false, err: context.Canceled, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			s := &Service{Service: service.Service{
				Logger: slog.New(slog.NewTextHandler(&output, nil)),
			}}
			if test.stopping {
				s.SetStopping()
			}

			s.logErrorUnlessShutdown("operation failed", test.err, "operation", "test")

			hasError := strings.Contains(output.String(), "level=ERROR")
			require.Equal(t, test.wantError, hasError, output.String())
		})
	}
}

func TestTrySettleOperationDeadlineDoesNotCancelServiceContext(t *testing.T) {
	s, app := newValidationService(t)
	ctx, cancel := context.WithTimeout(s.Context, 50*time.Millisecond)
	defer cancel()

	err := s.trySettle(ctx, app, 1)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, s.Context.Err())
}

func TestTrySettleShutdownCancelsOperationContext(t *testing.T) {
	s, app := newValidationService(t)
	ctx, cancel := context.WithTimeout(s.Context, time.Second)
	defer cancel()

	time.AfterFunc(50*time.Millisecond, s.Cancel)
	start := time.Now()
	err := s.trySettle(ctx, app, 1)

	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestReactToTournamentOperationDeadlineDoesNotCancelServiceContext(t *testing.T) {
	s, app := newValidationService(t)
	ctx, cancel := context.WithTimeout(s.Context, 50*time.Millisecond)
	defer cancel()

	err := s.reactToTournament(ctx, app, 1)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, s.Context.Err())
}

func TestReactToTournamentShutdownCancelsOperationContext(t *testing.T) {
	s, app := newValidationService(t)
	ctx, cancel := context.WithTimeout(s.Context, time.Second)
	defer cancel()

	time.AfterFunc(50*time.Millisecond, s.Cancel)
	start := time.Now()
	err := s.reactToTournament(ctx, app, 1)

	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func newValidationService(t *testing.T) (*Service, *model.Application) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	app := repotest.NewApplicationBuilder().
		WithEpochLength(10).
		Build()
	app.IConsensusAddress = common.HexToAddress("0x3")

	epoch := repotest.NewEpochBuilder(app.ID).
		WithStatus(model.EpochStatus_ClaimComputed).
		WithMachineHash(common.HexToHash("0x6")).
		WithTxBufferDataBlock(common.HexToHash("0x8")).
		Build()
	tournamentAddress := common.HexToAddress("0x4")
	commitment := common.HexToHash("0x5")
	epoch.TournamentAddress = &tournamentAddress
	epoch.Commitment = &commitment
	epoch.CommitmentProof = []common.Hash{common.HexToHash("0x7")}
	epoch.TxBufferProof = []common.Hash{}

	repo := &prtRepositoryMock{}
	repo.On("GetEpoch", mock.Anything, app.IApplicationAddress.Hex(), uint64(0)).
		Return(epoch, nil)
	repo.On("GetCommitment", mock.Anything, app.IApplicationAddress.Hex(), uint64(0),
		tournamentAddress.Hex(), commitment.String()).
		Return(nil, nil).
		Maybe()

	consensusAdapter := &daveConsensusAdapterMock{}
	consensusAdapter.On("CanSettle", mock.Anything).
		Return(CanSettleResult{IsFinished: true, EpochNumber: big.NewInt(0)}, nil)
	consensusAdapter.On("IsEpochSettled", mock.Anything, uint64(0)).
		Return(false, nil)
	consensusAdapter.On("Settle", mock.Anything, big.NewInt(0), [32]byte(common.HexToHash("0x8")), [][32]byte{}).
		Return((*types.Transaction)(nil), func(opts *bind.TransactOpts, _ *big.Int, _ [32]byte, _ [][32]byte) error {
			<-opts.Context.Done()
			return opts.Context.Err()
		})

	tournamentAdapter := &tournamentAdapterMock{}
	tournamentAdapter.On("IsCommitmentJoined", mock.Anything, [32]byte(commitment)).
		Return(false, nil)
	tournamentAdapter.On("BondValue", mock.Anything).
		Return(big.NewInt(0), nil)
	tournamentAdapter.On("JoinTournament",
		mock.Anything, [32]byte(*epoch.MachineHash), mock.Anything, mock.Anything, mock.Anything).
		Return((*types.Transaction)(nil), func(
			opts *bind.TransactOpts,
			_ [32]byte,
			_ [][32]byte,
			_ [32]byte,
			_ [32]byte,
		) error {
			<-opts.Context.Done()
			return opts.Context.Err()
		})

	adapterFactory := &adapterFactoryMock{}
	adapterFactory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).
		Return(consensusAdapter, nil)
	adapterFactory.On("CreateTournamentAdapter", tournamentAddress).
		Return(tournamentAdapter, nil)

	s := &Service{
		Service: service.Service{
			Context: ctx,
			Cancel:  cancel,
			Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		repository:        repo,
		adapterFactory:    adapterFactory,
		submissionEnabled: true,
		submissionTimeout: time.Second,
		txOptsFactory: ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{
			From: common.HexToAddress("0x9"),
		}),
		currentEpochIndex: map[int64]uint64{},
		settleInFlight:    map[int64]*common.Hash{},
		joinInFlight:      map[int64]*common.Hash{},
	}
	s.currentEpochIndex[app.ID] = 0
	t.Cleanup(cancel)
	return s, app
}
