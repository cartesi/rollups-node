// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
)

type prtRepositoryMock struct {
	mock.Mock
}

var _ prtRepository = (*prtRepositoryMock)(nil)

func (m *prtRepositoryMock) ListApplications(
	ctx context.Context,
	f repository.ApplicationFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Application, uint64, error) {
	args := m.Called(ctx, f, p, descending)
	apps, _ := args.Get(0).([]*model.Application)
	return apps, args.Get(1).(uint64), args.Error(2)
}

func (m *prtRepositoryMock) UpdateApplicationStatus(
	ctx context.Context,
	appID int64,
	status model.ApplicationStatus,
	reason *string,
) error {
	args := m.Called(ctx, appID, status, reason)
	return args.Error(0)
}

func (m *prtRepositoryMock) HasUndrainedEpochsBeforeBlock(
	ctx context.Context,
	appID int64,
	blockBound uint64,
) (bool, error) {
	args := m.Called(ctx, appID, blockBound)
	return args.Bool(0), args.Error(1)
}

func (m *prtRepositoryMock) HasUnreconciledClaimsBeforeBlock(
	ctx context.Context,
	appID int64,
	blockBound uint64,
) (bool, error) {
	args := m.Called(ctx, appID, blockBound)
	return args.Bool(0), args.Error(1)
}

func (m *prtRepositoryMock) UpdateEpochWithForeclosedClaim(
	ctx context.Context,
	applicationID int64,
	index uint64,
) error {
	args := m.Called(ctx, applicationID, index)
	return args.Error(0)
}

func (m *prtRepositoryMock) ListEpochs(
	ctx context.Context,
	nameOrAddress string,
	f repository.EpochFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Epoch, uint64, error) {
	args := m.Called(ctx, nameOrAddress, f, p, descending)
	epochs, _ := args.Get(0).([]*model.Epoch)
	return epochs, args.Get(1).(uint64), args.Error(2)
}

func (m *prtRepositoryMock) GetEpoch(
	ctx context.Context,
	nameOrAddress string,
	index uint64,
) (*model.Epoch, error) {
	args := m.Called(ctx, nameOrAddress, index)
	epoch, _ := args.Get(0).(*model.Epoch)
	return epoch, args.Error(1)
}

func (m *prtRepositoryMock) UpdateEpochStatus(
	ctx context.Context,
	nameOrAddress string,
	e *model.Epoch,
) error {
	args := m.Called(ctx, nameOrAddress, e)
	return args.Error(0)
}

func (m *prtRepositoryMock) CreateTournament(
	ctx context.Context,
	nameOrAddress string,
	t *model.Tournament,
) error {
	args := m.Called(ctx, nameOrAddress, t)
	return args.Error(0)
}

func (m *prtRepositoryMock) GetTournament(
	ctx context.Context,
	nameOrAddress string,
	address string,
) (*model.Tournament, error) {
	args := m.Called(ctx, nameOrAddress, address)
	tournament, _ := args.Get(0).(*model.Tournament)
	return tournament, args.Error(1)
}

func (m *prtRepositoryMock) UpdateTournament(
	ctx context.Context,
	nameOrAddress string,
	t *model.Tournament,
) error {
	args := m.Called(ctx, nameOrAddress, t)
	return args.Error(0)
}

func (m *prtRepositoryMock) ListTournaments(
	ctx context.Context,
	nameOrAddress string,
	f repository.TournamentFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Tournament, uint64, error) {
	args := m.Called(ctx, nameOrAddress, f, p, descending)
	tournaments, _ := args.Get(0).([]*model.Tournament)
	return tournaments, args.Get(1).(uint64), args.Error(2)
}

func (m *prtRepositoryMock) StoreTournamentEvents(
	ctx context.Context,
	appID int64,
	commitments []*model.Commitment,
	matches []*model.Match,
	advancedMatches []*model.MatchAdvanced,
	deletedMatches []*model.Match,
	blockNumber uint64,
) error {
	args := m.Called(ctx, appID, commitments, matches, advancedMatches, deletedMatches, blockNumber)
	return args.Error(0)
}

func (m *prtRepositoryMock) GetCommitment(
	ctx context.Context,
	nameOrAddress string,
	epochIndex uint64,
	tournamentAddress string,
	commitment string,
) (*model.Commitment, error) {
	args := m.Called(ctx, nameOrAddress, epochIndex, tournamentAddress, commitment)
	c, _ := args.Get(0).(*model.Commitment)
	return c, args.Error(1)
}

func (m *prtRepositoryMock) SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error {
	args := m.Called(ctx, key, rawJSON)
	return args.Error(0)
}

func (m *prtRepositoryMock) LoadNodeConfigRaw(
	ctx context.Context,
	key string,
) ([]byte, time.Time, time.Time, error) {
	args := m.Called(ctx, key)
	raw, _ := args.Get(0).([]byte)
	return raw, args.Get(1).(time.Time), args.Get(2).(time.Time), args.Error(3)
}

type ethClientMock struct {
	mock.Mock
}

var _ EthClientInterface = (*ethClientMock)(nil)

func (m *ethClientMock) TransactionReceipt(
	ctx context.Context,
	txHash common.Hash,
) (*types.Receipt, error) {
	args := m.Called(ctx, txHash)
	receipt, _ := args.Get(0).(*types.Receipt)
	return receipt, args.Error(1)
}

func (m *ethClientMock) ChainID(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	chainID, _ := args.Get(0).(*big.Int)
	return chainID, args.Error(1)
}

func (m *ethClientMock) BlockNumber(ctx context.Context) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *ethClientMock) TransactionByHash(
	ctx context.Context,
	hash common.Hash,
) (*types.Transaction, bool, error) {
	args := m.Called(ctx, hash)
	tx, _ := args.Get(0).(*types.Transaction)
	return tx, args.Bool(1), args.Error(2)
}

type adapterFactoryMock struct {
	mock.Mock
}

var _ AdapterFactory = (*adapterFactoryMock)(nil)

func (m *adapterFactoryMock) CreateTournamentAdapter(addr common.Address) (TournamentAdapter, error) {
	args := m.Called(addr)
	adapter, _ := args.Get(0).(TournamentAdapter)
	return adapter, args.Error(1)
}

func (m *adapterFactoryMock) CreateDaveConsensusAdapter(addr common.Address) (DaveConsensusAdapter, error) {
	args := m.Called(addr)
	adapter, _ := args.Get(0).(DaveConsensusAdapter)
	return adapter, args.Error(1)
}

type daveConsensusAdapterMock struct {
	mock.Mock
}

var _ DaveConsensusAdapter = (*daveConsensusAdapterMock)(nil)

func (m *daveConsensusAdapterMock) ParseEpochSealed(
	log types.Log,
) (*idaveconsensus.IDaveConsensusEpochSealed, error) {
	args := m.Called(log)
	event, _ := args.Get(0).(*idaveconsensus.IDaveConsensusEpochSealed)
	return event, args.Error(1)
}

func (m *daveConsensusAdapterMock) CanSettle(opts *bind.CallOpts) (CanSettleResult, error) {
	args := m.Called(opts)
	result, _ := args.Get(0).(CanSettleResult)
	return result, args.Error(1)
}

func (m *daveConsensusAdapterMock) IsEpochSettled(
	opts *bind.CallOpts,
	epochNumber uint64,
) (bool, error) {
	args := m.Called(opts, epochNumber)
	return args.Bool(0), args.Error(1)
}

func (m *daveConsensusAdapterMock) Settle(
	opts *bind.TransactOpts,
	epochNumber *big.Int,
	outputsMerkleRoot [32]byte,
	proof [][32]byte,
) (*types.Transaction, error) {
	args := m.Called(opts, epochNumber, outputsMerkleRoot, proof)
	tx, _ := args.Get(0).(*types.Transaction)
	if fn, ok := args.Get(1).(func(*bind.TransactOpts, *big.Int, [32]byte, [][32]byte) error); ok {
		return tx, fn(opts, epochNumber, outputsMerkleRoot, proof)
	}
	return tx, args.Error(1)
}

type tournamentAdapterMock struct {
	mock.Mock
}

var _ TournamentAdapter = (*tournamentAdapterMock)(nil)

func (m *tournamentAdapterMock) RetrieveCommitmentJoinedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentCommitmentJoined, error) {
	args := m.Called(opts)
	events, _ := args.Get(0).([]*itournament.ITournamentCommitmentJoined)
	return events, args.Error(1)
}

func (m *tournamentAdapterMock) RetrieveMatchAdvancedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentMatchAdvanced, error) {
	args := m.Called(opts)
	events, _ := args.Get(0).([]*itournament.ITournamentMatchAdvanced)
	return events, args.Error(1)
}

func (m *tournamentAdapterMock) RetrieveMatchCreatedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentMatchCreated, error) {
	args := m.Called(opts)
	events, _ := args.Get(0).([]*itournament.ITournamentMatchCreated)
	return events, args.Error(1)
}

func (m *tournamentAdapterMock) RetrieveMatchDeletedEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentMatchDeleted, error) {
	args := m.Called(opts)
	events, _ := args.Get(0).([]*itournament.ITournamentMatchDeleted)
	return events, args.Error(1)
}

func (m *tournamentAdapterMock) RetrieveNewInnerTournamentEvents(
	opts *bind.FilterOpts,
) ([]*itournament.ITournamentNewInnerTournament, error) {
	args := m.Called(opts)
	events, _ := args.Get(0).([]*itournament.ITournamentNewInnerTournament)
	return events, args.Error(1)
}

func (m *tournamentAdapterMock) RetrieveAllEvents(opts *bind.FilterOpts) (*TournamentEvents, error) {
	args := m.Called(opts)
	events, _ := args.Get(0).(*TournamentEvents)
	return events, args.Error(1)
}

func (m *tournamentAdapterMock) Result(opts *bind.CallOpts) (bool, [32]byte, [32]byte, error) {
	args := m.Called(opts)
	return args.Bool(0), args.Get(1).([32]byte), args.Get(2).([32]byte), args.Error(3)
}

func (m *tournamentAdapterMock) Constants(opts *bind.CallOpts) (TournamentConstants, error) {
	args := m.Called(opts)
	constants, _ := args.Get(0).(TournamentConstants)
	return constants, args.Error(1)
}

func (m *tournamentAdapterMock) TimeFinished(opts *bind.CallOpts) (bool, uint64, error) {
	args := m.Called(opts)
	return args.Bool(0), args.Get(1).(uint64), args.Error(2)
}

func (m *tournamentAdapterMock) BondValue(opts *bind.CallOpts) (*big.Int, error) {
	args := m.Called(opts)
	value, _ := args.Get(0).(*big.Int)
	return value, args.Error(1)
}

func (m *tournamentAdapterMock) IsCommitmentJoined(
	opts *bind.CallOpts,
	commitmentRoot [32]byte,
) (bool, error) {
	args := m.Called(opts, commitmentRoot)
	return args.Bool(0), args.Error(1)
}

func (m *tournamentAdapterMock) JoinTournament(
	opts *bind.TransactOpts,
	finalState [32]byte,
	proof [][32]byte,
	leftNode [32]byte,
	rightNode [32]byte,
) (*types.Transaction, error) {
	args := m.Called(opts, finalState, proof, leftNode, rightNode)
	tx, _ := args.Get(0).(*types.Transaction)
	if fn, ok := args.Get(1).(func(*bind.TransactOpts, [32]byte, [][32]byte, [32]byte, [32]byte) error); ok {
		return tx, fn(opts, finalState, proof, leftNode, rightNode)
	}
	return tx, args.Error(1)
}

func newPRTServiceMock() (*Service, *prtRepositoryMock) {
	repo := &prtRepositoryMock{}
	s := &Service{
		TickServiceTemplate: service.TickServiceTemplate{
			BaseTemplate: service.BaseTemplate{
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			},
		},
		repository: repo,
	}
	return s, repo
}
