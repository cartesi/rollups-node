// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
)

type prtRepositoryMock struct {
	mock.Mock
}

func (m *prtRepositoryMock) ListApplications(
	ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool,
) ([]*model.Application, uint64, error) {
	args := m.Called(ctx, f, p, descending)
	return args.Get(0).([]*model.Application), args.Get(1).(uint64), args.Error(2)
}

func (m *prtRepositoryMock) UpdateApplicationHealth(
	ctx context.Context, appID int64, state model.ApplicationHealth, reason *string,
) error {
	args := m.Called(ctx, appID, state, reason)
	return args.Error(0)
}

func (m *prtRepositoryMock) ListEpochs(
	ctx context.Context, nameOrAddress string, f repository.EpochFilter,
	p repository.Pagination, descending bool,
) ([]*model.Epoch, uint64, error) {
	args := m.Called(ctx, nameOrAddress, f, p, descending)
	return args.Get(0).([]*model.Epoch), args.Get(1).(uint64), args.Error(2)
}

func (m *prtRepositoryMock) GetEpoch(
	ctx context.Context, nameOrAddress string, index uint64,
) (*model.Epoch, error) {
	args := m.Called(ctx, nameOrAddress, index)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Epoch), args.Error(1)
}

func (m *prtRepositoryMock) UpdateEpochStatus(
	ctx context.Context, nameOrAddress string, e *model.Epoch,
) error {
	args := m.Called(ctx, nameOrAddress, e)
	return args.Error(0)
}

func (m *prtRepositoryMock) CreateTournament(
	ctx context.Context, nameOrAddress string, t *model.Tournament,
) error {
	args := m.Called(ctx, nameOrAddress, t)
	return args.Error(0)
}

func (m *prtRepositoryMock) GetTournament(
	ctx context.Context, nameOrAddress string, address string,
) (*model.Tournament, error) {
	args := m.Called(ctx, nameOrAddress, address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Tournament), args.Error(1)
}

func (m *prtRepositoryMock) UpdateTournament(
	ctx context.Context, nameOrAddress string, t *model.Tournament,
) error {
	args := m.Called(ctx, nameOrAddress, t)
	return args.Error(0)
}

func (m *prtRepositoryMock) ListTournaments(
	ctx context.Context, nameOrAddress string, f repository.TournamentFilter,
	p repository.Pagination, descending bool,
) ([]*model.Tournament, uint64, error) {
	args := m.Called(ctx, nameOrAddress, f, p, descending)
	return args.Get(0).([]*model.Tournament), args.Get(1).(uint64), args.Error(2)
}

func (m *prtRepositoryMock) StoreTournamentEvents(
	ctx context.Context, appID int64, commitments []*model.Commitment, matches []*model.Match,
	matchAdvanced []*model.MatchAdvanced, matchDeleted []*model.Match, lastBlock uint64,
) error {
	args := m.Called(ctx, appID, commitments, matches, matchAdvanced, matchDeleted, lastBlock)
	return args.Error(0)
}

func (m *prtRepositoryMock) GetCommitment(
	ctx context.Context, nameOrAddress string, epochIndex uint64,
	tournamentAddress string, commitmentHex string,
) (*model.Commitment, error) {
	args := m.Called(ctx, nameOrAddress, epochIndex, tournamentAddress, commitmentHex)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Commitment), args.Error(1)
}

func (m *prtRepositoryMock) AcknowledgeAppStopped(
	ctx context.Context, appID int64, serviceName string,
) error {
	args := m.Called(ctx, appID, serviceName)
	return args.Error(0)
}

func (m *prtRepositoryMock) GetAppsNeedingAck(
	ctx context.Context, serviceName string, consensusTypes []model.Consensus,
) ([]int64, error) {
	args := m.Called(ctx, serviceName, consensusTypes)
	return args.Get(0).([]int64), args.Error(1)
}

func (m *prtRepositoryMock) SaveNodeConfigRaw(
	ctx context.Context, key string, rawJSON []byte,
) error {
	args := m.Called(ctx, key, rawJSON)
	return args.Error(0)
}

func (m *prtRepositoryMock) LoadNodeConfigRaw(
	ctx context.Context, key string,
) ([]byte, time.Time, time.Time, error) {
	args := m.Called(ctx, key)
	return args.Get(0).([]byte), args.Get(1).(time.Time), args.Get(2).(time.Time), args.Error(3)
}

type prtClientMock struct {
	mock.Mock
}

func (m *prtClientMock) TransactionReceipt(
	ctx context.Context, txHash common.Hash,
) (*types.Receipt, error) {
	args := m.Called(ctx, txHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Receipt), args.Error(1)
}

func (m *prtClientMock) ChainID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	return args.Get(0).(*big.Int), args.Error(1)
}

func (m *prtClientMock) BlockNumber(ctx context.Context) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *prtClientMock) TransactionByHash(
	ctx context.Context, hash common.Hash,
) (*types.Transaction, bool, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*types.Transaction), args.Bool(1), args.Error(2)
}

func newPRTServiceMock() (*Service, *prtRepositoryMock, *prtClientMock) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &prtRepositoryMock{}
	client := &prtClientMock{}

	svc := &Service{
		Service: service.Service{
			Logger:  logger,
			Context: context.Background(),
		},
		repository:        repo,
		client:            client,
		submissionEnabled: false,
		currentEpochIndex: map[int64]uint64{},
		settleInFlight:    map[int64]*common.Hash{},
		joinInFlight:      map[int64]*common.Hash{},
	}
	return svc, repo, client
}

func makePRTApp(id int64) *model.Application {
	return &model.Application{
		ID:                  id,
		Name:                "prt-app",
		IApplicationAddress: common.HexToAddress("0x1000000000000000000000000000000000000001"),
		ConsensusType:       model.Consensus_PRT,
		Enabled:             true,
		Health:              model.ApplicationHealth_Running,
	}
}

func TestTickDefersAckWhileSoftDeletedPrtTxPending(t *testing.T) {
	svc, repo, client := newPRTServiceMock()
	defer repo.AssertExpectations(t)
	defer client.AssertExpectations(t)

	tx := common.HexToHash("0x1")
	svc.currentEpochIndex[42] = 7
	svc.settleInFlight[42] = &tx
	svc.joinInFlight[42] = &tx

	repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Application{}, uint64(0), nil).Once()
	repo.On("GetAppsNeedingAck",
		mock.Anything,
		repository.ServicePRT,
		repository.ConsensusTypesForService(repository.ServicePRT),
	).Return([]int64{42}, nil).Once()
	client.On("TransactionByHash", mock.Anything, tx).
		Return((*types.Transaction)(nil), true, nil).Twice()

	errs := svc.Tick()
	assert.Empty(t, errs)
	assert.Contains(t, svc.currentEpochIndex, int64(42))
	assert.Contains(t, svc.settleInFlight, int64(42))
	assert.Contains(t, svc.joinInFlight, int64(42))
	repo.AssertNotCalled(t, "AcknowledgeAppStopped", mock.Anything, int64(42), repository.ServicePRT)
}

func TestTickAcksSoftDeletedAppAfterDrainingTxClears(t *testing.T) {
	svc, repo, client := newPRTServiceMock()
	defer repo.AssertExpectations(t)
	defer client.AssertExpectations(t)

	tx := common.HexToHash("0x2")
	svc.currentEpochIndex[42] = 9
	svc.settleInFlight[42] = &tx

	repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Application{}, uint64(0), nil).Once()
	repo.On("GetAppsNeedingAck",
		mock.Anything,
		repository.ServicePRT,
		repository.ConsensusTypesForService(repository.ServicePRT),
	).Return([]int64{42}, nil).Once()
	client.On("TransactionByHash", mock.Anything, tx).
		Return((*types.Transaction)(nil), false, nil).Once()
	repo.On("AcknowledgeAppStopped", mock.Anything, int64(42), repository.ServicePRT).
		Return(nil).Once()

	errs := svc.Tick()
	assert.Empty(t, errs)
	assert.NotContains(t, svc.settleInFlight, int64(42))
	assert.Contains(t, svc.currentEpochIndex, int64(42))
}
