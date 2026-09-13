// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
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

func TestPRTOperationContextCancellation(t *testing.T) {
	for _, test := range []struct {
		name     string
		staged   bool
		shutdown bool
	}{
		{name: "AcceptResult/OperationDeadline", staged: true},
		{name: "AcceptResult/Shutdown", staged: true, shutdown: true},
		{name: "ReactToTournament/OperationDeadline"},
		{name: "ReactToTournament/Shutdown", shutdown: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, app, epoch := newValidationService(t, test.staged)
			timeout := 50 * time.Millisecond
			if test.shutdown {
				timeout = time.Second
				time.AfterFunc(50*time.Millisecond, s.Cancel)
			}
			ctx, cancel := context.WithTimeout(s.Context, timeout)
			defer cancel()

			start := time.Now()
			var err error
			if test.staged {
				_, _, err = s.progressTournamentResult(ctx, app, 1, 1)
			} else {
				_, err = s.reactToTournament(ctx, app, epoch, 1)
			}
			if test.shutdown {
				require.ErrorIs(t, err, context.Canceled)
				require.ErrorIs(t, ctx.Err(), context.Canceled)
				require.Less(t, time.Since(start), 500*time.Millisecond)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.NoError(t, s.Context.Err())
			}
		})
	}
}

func newValidationService(t *testing.T, staged bool) (*Service, *model.Application, *model.Epoch) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	app := repotest.NewApplicationBuilder().
		WithEpochLength(10).
		Build()
	app.IConsensusAddress = common.HexToAddress("0x3")

	status := model.EpochStatus_ClaimComputed
	if staged {
		status = model.EpochStatus_ClaimStaged
	}
	epoch := repotest.NewEpochBuilder(app.ID).
		WithStatus(status).
		WithMachineHash(common.HexToHash("0x6")).
		WithTxBufferDataBlock(common.HexToHash("0x8")).
		Build()
	tournamentAddress := common.HexToAddress("0x4")
	commitment := common.HexToHash("0x5")
	epoch.TournamentAddress = &tournamentAddress
	epoch.Commitment = &commitment
	epoch.CommitmentProof = []common.Hash{common.HexToHash("0x7")}
	if staged {
		epoch.StagedAtBlock = new(uint64)
		*epoch.StagedAtBlock = 1
	}

	s, repo := newPRTServiceMock()
	repo.On("GetEpoch", mock.Anything, app.IApplicationAddress.Hex(), uint64(0)).
		Return(epoch, nil)
	repo.On("GetCommitment", mock.Anything, app.IApplicationAddress.Hex(), uint64(0),
		tournamentAddress.Hex(), commitment.String()).
		Return(nil, nil).
		Maybe()

	consensusAdapter := &daveConsensusAdapterMock{}
	consensusAdapter.On("GetCurrentSealedEpoch", mock.Anything).Return(CurrentSealedEpoch{
		EpochNumber:                      0,
		Tournament:                       tournamentAddress,
		IsTournamentResultStaged:         true,
		StagingBlockNumber:               1,
		StagedPostEpochMachineStateHash:  *epoch.MachineHash,
		StagedPostEpochOutputsMerkleRoot: *epoch.TxBufferDataBlock,
	}, nil)
	consensusAdapter.On("CanStageTournamentResult", mock.Anything).Return(CanStageTournamentResult{
		IsFinished:                      true,
		IsTournamentResultStaged:        true,
		EpochNumber:                     0,
		WinnerCommitment:                commitment,
		WinnerPostEpochMachineStateHash: *epoch.MachineHash,
	}, nil)
	consensusAdapter.On("CanAcceptStagedTournamentResult", mock.Anything).Return(CanAcceptStagedTournamentResult{
		IsTournamentResultStaged:         true,
		IsClaimStagingPeriodOver:         true,
		EpochNumber:                      0,
		StagedPostEpochMachineStateHash:  *epoch.MachineHash,
		StagedPostEpochOutputsMerkleRoot: *epoch.TxBufferDataBlock,
	}, nil)
	consensusAdapter.On("AcceptStagedTournamentResult", mock.Anything, uint64(0)).
		Return((*types.Transaction)(nil), func(opts *bind.TransactOpts, _ uint64) error {
			<-opts.Context.Done()
			return opts.Context.Err()
		})

	tournamentAdapter := &tournamentAdapterMock{}
	if !staged {
		tournamentAdapter.On("Descriptor", mock.Anything).
			Return(TournamentDescriptor{Height: model.Log2EpochComputationHashLeafCount}, nil).Once()
	}
	tournamentAdapter.On("CommitmentStanding", mock.Anything, [32]byte(commitment)).
		Return(CommitmentStanding{}, nil)
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

	s.Context, s.Cancel = ctx, cancel
	s.adapterFactory = adapterFactory
	s.submissionEnabled = true
	s.submissionTimeout = time.Second
	s.txOptsFactory = ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{
		From: common.HexToAddress("0x9"),
	})
	t.Cleanup(cancel)
	return s, app, epoch
}
