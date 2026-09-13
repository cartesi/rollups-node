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

func (m *prtRepositoryMock) UpdateEpochReconciledStaged(
	ctx context.Context,
	applicationID int64,
	index uint64,
	stagedAtBlock uint64,
) error {
	args := m.Called(ctx, applicationID, index, stagedAtBlock)
	return args.Error(0)
}

func (m *prtRepositoryMock) UpdateEpochWithAcceptedClaim(
	ctx context.Context,
	applicationID int64,
	index uint64,
	txHash *common.Hash,
) error {
	args := m.Called(ctx, applicationID, index, txHash)
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
	batches []*repository.TournamentEventBatch,
	blockNumber uint64,
) error {
	args := m.Called(ctx, appID, batches, blockNumber)
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

func (m *prtRepositoryMock) ListCommitments(ctx context.Context, app string, filter repository.CommitmentFilter,
	page repository.Pagination, descending bool) ([]*model.Commitment, uint64, error) {
	args := m.Called(ctx, app, filter, page, descending)
	rows, _ := args.Get(0).([]*model.Commitment)
	return rows, args.Get(1).(uint64), args.Error(2)
}

func (m *prtRepositoryMock) ListMatches(ctx context.Context, app string, filter repository.MatchFilter,
	page repository.Pagination, descending bool) ([]*model.Match, uint64, error) {
	args := m.Called(ctx, app, filter, page, descending)
	rows, _ := args.Get(0).([]*model.Match)
	return rows, args.Get(1).(uint64), args.Error(2)
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

func (m *ethClientMock) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	args := m.Called(ctx, number)
	header, _ := args.Get(0).(*types.Header)
	return header, args.Error(1)
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

func (m *daveConsensusAdapterMock) TournamentLevelCount(opts *bind.CallOpts) (uint64, error) {
	args := m.Called(opts)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *daveConsensusAdapterMock) GetCurrentSealedEpoch(opts *bind.CallOpts) (CurrentSealedEpoch, error) {
	args := m.Called(opts)
	result, _ := args.Get(0).(CurrentSealedEpoch)
	return result, args.Error(1)
}

func (m *daveConsensusAdapterMock) CanStageTournamentResult(
	opts *bind.CallOpts,
) (CanStageTournamentResult, error) {
	args := m.Called(opts)
	result, _ := args.Get(0).(CanStageTournamentResult)
	return result, args.Error(1)
}

func (m *daveConsensusAdapterMock) CanAcceptStagedTournamentResult(
	opts *bind.CallOpts,
) (CanAcceptStagedTournamentResult, error) {
	args := m.Called(opts)
	result, _ := args.Get(0).(CanAcceptStagedTournamentResult)
	return result, args.Error(1)
}

func (m *daveConsensusAdapterMock) StageTournamentResult(
	opts *bind.TransactOpts,
	epochNumber uint64,
	proof model.StateProof,
) (*types.Transaction, error) {
	args := m.Called(opts, epochNumber, proof)
	tx, _ := args.Get(0).(*types.Transaction)
	if fn, ok := args.Get(1).(func(*bind.TransactOpts, uint64, model.StateProof) error); ok {
		return tx, fn(opts, epochNumber, proof)
	}
	return tx, args.Error(1)
}

func (m *daveConsensusAdapterMock) AcceptStagedTournamentResult(
	opts *bind.TransactOpts,
	epochNumber uint64,
) (*types.Transaction, error) {
	args := m.Called(opts, epochNumber)
	tx, _ := args.Get(0).(*types.Transaction)
	if fn, ok := args.Get(1).(func(*bind.TransactOpts, uint64) error); ok {
		return tx, fn(opts, epochNumber)
	}
	return tx, args.Error(1)
}

type tournamentAdapterMock struct {
	mock.Mock
}

var _ TournamentAdapter = (*tournamentAdapterMock)(nil)

func (m *tournamentAdapterMock) RetrieveAllEvents(opts *bind.FilterOpts) (*TournamentEvents, error) {
	args := m.Called(opts)
	events, _ := args.Get(0).(*TournamentEvents)
	return events, args.Error(1)
}

func (m *tournamentAdapterMock) Descriptor(opts *bind.CallOpts) (TournamentDescriptor, error) {
	args := m.Called(opts)
	result, _ := args.Get(0).(TournamentDescriptor)
	return result, args.Error(1)
}

func (m *tournamentAdapterMock) Standing(opts *bind.CallOpts) (TournamentStanding, error) {
	args := m.Called(opts)
	result, _ := args.Get(0).(TournamentStanding)
	return result, args.Error(1)
}

func (m *tournamentAdapterMock) MatchSnapshot(opts *bind.CallOpts, one, two [32]byte) (ObservedMatchSnapshot, error) {
	args := m.Called(opts, one, two)
	result, _ := args.Get(0).(ObservedMatchSnapshot)
	return result, args.Error(1)
}

func (m *tournamentAdapterMock) InnerResult(opts *bind.CallOpts) (InnerResult, error) {
	args := m.Called(opts)
	result, _ := args.Get(0).(InnerResult)
	return result, args.Error(1)
}

func (m *tournamentAdapterMock) StructuralEventCounts(opts *bind.CallOpts) (StructuralEventCounts, error) {
	args := m.Called(opts)
	result, _ := args.Get(0).(StructuralEventCounts)
	return result, args.Error(1)
}

func (m *tournamentAdapterMock) BondValue(opts *bind.CallOpts) (*big.Int, error) {
	args := m.Called(opts)
	value, _ := args.Get(0).(*big.Int)
	return value, args.Error(1)
}

func (m *tournamentAdapterMock) BondRecovery(opts *bind.CallOpts) (BondRecovery, error) {
	args := m.Called(opts)
	recovery, _ := args.Get(0).(BondRecovery)
	return recovery, args.Error(1)
}

func (m *tournamentAdapterMock) CommitmentStanding(
	opts *bind.CallOpts,
	commitmentRoot [32]byte,
) (CommitmentStanding, error) {
	args := m.Called(opts, commitmentRoot)
	result, _ := args.Get(0).(CommitmentStanding)
	return result, args.Error(1)
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

func (m *tournamentAdapterMock) TryRecoveringBond(opts *bind.TransactOpts) (*types.Transaction, error) {
	args := m.Called(opts)
	tx, _ := args.Get(0).(*types.Transaction)
	if fn, ok := args.Get(1).(func(*bind.TransactOpts) error); ok {
		return tx, fn(opts)
	}
	return tx, args.Error(1)
}

func newPRTServiceMock() (*Service, *prtRepositoryMock) {
	repo := &prtRepositoryMock{}
	s := &Service{
		Service: service.Service{
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		repository:          repo,
		pendingTransactions: map[int64]pendingTournamentTransaction{},
		disputeWarnings:     map[common.Address]struct{}{},
		zeroStagingWarnings: map[int64]struct{}{},
		rootBondRecoveries:  map[int64][]*rootBondRecovery{},
		observationFailures: map[int64]tournamentObservationFailure{},
	}
	return s, repo
}
