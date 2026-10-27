// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"time"
)

type prtRepositoryMock struct {
	mock.Mock
	repository.DeliveryRepository
	storedTournaments    []*model.Tournament
	committedTournaments map[common.Address]*model.Tournament
	eventCounts          map[common.Address]repository.TournamentEventCounts
	eventCountError      error
	eventCountRead       func(int64, uint64, common.Address, uint64)
	deploymentMu         sync.Mutex
	deploymentRows       map[int64]model.DeploymentAdmissionState
	deploymentRead       func(context.Context, repository.DeploymentIdentity) (*model.DeploymentAdmissionState, error)
	deploymentAck        func(context.Context, repository.DeploymentAssessment) (*model.DeploymentAdmissionState, error)
	deploymentCompletion func(context.Context, repository.DeploymentIdentity, uint64) (*repository.DeploymentCompletionState, error)
	deploymentDrain      func(context.Context, repository.DeploymentDrainRequest) error
}

func (m *prtRepositoryMock) GetMatch(ctx context.Context, app string, epoch uint64, tournament, id string) (*model.Match, error) {
	args := m.Called(ctx, app, epoch, tournament, id)
	match, _ := args.Get(0).(*model.Match)
	return match, args.Error(1)
}

func (m *prtRepositoryMock) GetTournamentEventCounts(
	_ context.Context, appID int64, epoch uint64, tournament common.Address, through uint64,
) (repository.TournamentEventCounts, error) {
	if m.eventCountRead != nil {
		m.eventCountRead(appID, epoch, tournament, through)
	}
	return m.eventCounts[tournament], m.eventCountError
}

func (m *prtRepositoryMock) GetApplication(ctx context.Context, address string) (*model.Application, error) {
	args := m.Called(ctx, address)
	app, _ := args.Get(0).(*model.Application)
	return app, args.Error(1)
}

func (m *prtRepositoryMock) GetEpochCompletionState(
	ctx context.Context, address string, index uint64,
) (*repository.EpochCompletionState, error) {
	args := m.Called(ctx, address, index)
	state, _ := args.Get(0).(*repository.EpochCompletionState)
	return state, args.Error(1)
}

func (m *prtRepositoryMock) GetApplicationIDBoundary(ctx context.Context, filter repository.ApplicationFilter) (int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(int64), args.Error(1)
}

func (m *prtRepositoryMock) ListApplicationsByID(
	ctx context.Context, filter repository.ApplicationFilter, after, through int64, limit uint64,
) ([]*model.Application, error) {
	args := m.Called(ctx, filter, after, through, limit)
	apps, _ := args.Get(0).([]*model.Application)
	return apps, args.Error(1)
}

func (m *prtRepositoryMock) StoreTournamentInterval(ctx context.Context, id int64, _ *uint64,
	batches []*repository.TournamentEventBatch) error {
	var head uint64
	if len(batches) > 0 {
		head = batches[0].Tournament.Snapshot.AsOfBlock
	}
	args := m.MethodCalled("StoreTournamentInterval", ctx, id, batches, head)
	if args.Error(0) == nil {
		if m.committedTournaments == nil {
			m.committedTournaments = map[common.Address]*model.Tournament{}
		}
		for _, batch := range batches {
			m.committedTournaments[batch.Tournament.Address] = batch.Tournament
		}
	}
	return args.Error(0)
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

func (m *prtRepositoryMock) GetLastNonOpenEpochIndex(ctx context.Context, nameOrAddress string) (*uint64, error) {
	args := m.Called(ctx, nameOrAddress)
	index, _ := args.Get(0).(*uint64)
	return index, args.Error(1)
}

func (m *prtRepositoryMock) ListRecoverableRootBonds(
	ctx context.Context, appID int64, claimer common.Address, observedBlock uint64,
	afterEpoch *uint64, throughEpoch, limit uint64,
) ([]repository.RootBondRecoveryCandidate, error) {
	args := m.Called(ctx, appID, claimer, observedBlock, afterEpoch, throughEpoch, limit)
	values, _ := args.Get(0).([]repository.RootBondRecoveryCandidate)
	return values, args.Error(1)
}

func (m *prtRepositoryMock) UpdateEpochStagingBlock(
	ctx context.Context, applicationID int64, index, expectedOldBlock, newBlock uint64,
) error {
	return m.Called(ctx, applicationID, index, expectedOldBlock, newBlock).Error(0)
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
	if committed := m.committedTournaments[common.HexToAddress(address)]; committed != nil {
		active := false
		for _, call := range m.ExpectedCalls {
			if call.Method == getTournamentMethod && call.Repeatability >= 0 {
				_, diff := call.Arguments.Diff([]any{ctx, nameOrAddress, address})
				if diff == 0 {
					active = true
					break
				}
			}
		}
		if !active {
			return committed, nil
		}
	}
	active := false
	for _, call := range m.ExpectedCalls {
		if call.Method == getTournamentMethod && call.Repeatability >= 0 {
			_, diff := call.Arguments.Diff([]any{ctx, nameOrAddress, address})
			if diff == 0 {
				active = true
				break
			}
		}
	}
	if !active {
		for _, stored := range m.storedTournaments {
			if stored.Address == common.HexToAddress(address) {
				return stored, nil
			}
		}
	}
	args := m.Called(ctx, nameOrAddress, address)
	tournament, _ := args.Get(0).(*model.Tournament)
	if args.Error(1) == nil {
		if committed := m.committedTournaments[common.HexToAddress(address)]; committed != nil {
			return committed, nil
		}
	}
	return tournament, args.Error(1)
}

func (m *prtRepositoryMock) ListTournaments(
	ctx context.Context,
	nameOrAddress string,
	f repository.TournamentFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Tournament, uint64, error) {
	if f == (repository.TournamentFilter{}) {
		explicit := false
		for _, call := range m.ExpectedCalls {
			if call.Method == "ListTournaments" {
				_, diff := call.Arguments.Diff([]any{ctx, nameOrAddress, f, p, descending})
				if diff == 0 {
					explicit = true
					break
				}
			}
		}
		if !explicit {
			stored := append([]*model.Tournament(nil), m.storedTournaments...)
			if len(stored) == 0 {
				for _, call := range m.ExpectedCalls {
					if call.Method == getTournamentMethod && len(call.ReturnArguments) > 0 {
						if t, ok := call.ReturnArguments[0].(*model.Tournament); ok && t != nil && t.Address != (common.Address{}) &&
							t.Snapshot.AsOfBlock > 0 {
							stored = append(stored, t)
						}
					}
				}
			}
			for i, row := range stored {
				if committed := m.committedTournaments[row.Address]; committed != nil {
					stored[i] = committed
				}
			}
			return stored, uint64(len(stored)), nil
		}
	}
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

func (m *prtRepositoryMock) InitializeNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error {
	args := m.Called(ctx, key, rawJSON)
	return args.Error(0)
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

func (m *ethClientMock) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	price, _ := args.Get(0).(*big.Int)
	return price, args.Error(1)
}

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
	if header, ok := args.Get(0).(func(context.Context, *big.Int) *types.Header); ok {
		return header(ctx, number), args.Error(1)
	}
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
	deploymentRead func(context.Context, txsender.BlockRef) (DeploymentEvidence, error)
}

var _ DaveConsensusAdapter = (*daveConsensusAdapterMock)(nil)

func (m *daveConsensusAdapterMock) ReadDeployment(ctx context.Context, head txsender.BlockRef) (DeploymentEvidence, error) {
	return m.deploymentRead(ctx, head)
}

func (m *daveConsensusAdapterMock) EpochSealedAt(
	ctx context.Context, blockHash common.Hash, epoch uint64,
) ([]*idaveconsensus.IDaveConsensusEpochSealed, error) {
	args := m.Called(ctx, blockHash, epoch)
	events, _ := args.Get(0).([]*idaveconsensus.IDaveConsensusEpochSealed)
	return events, args.Error(1)
}

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

func observeEpochsForTest(ctx context.Context, s *Service, app *model.Application, head uint64) (bool, error) {
	epochs, _, err := getTournamentObservationEpochs(ctx, s.repository, app.Name)
	if err != nil {
		return true, err
	}
	consensus, err := s.adapterFactory.CreateDaveConsensusAdapter(app.IConsensusAddress)
	if err != nil {
		return true, err
	}
	err = s.observeTournamentState(ctx, app, epochs, consensus, observerFacts(head))
	if err != nil {
		return true, err
	}
	if app.Status != model.ApplicationStatus_OK {
		return true, nil
	}
	if app.IsForeclosed() {
		return true, s.drainForeclosedClaims(ctx, app, epochs, consensus, head)
	}
	return s.reconcileAcceptedEpochs(ctx, app, epochs, consensus, head)
}

func (s *delivery) observeWindow(ctx context.Context, app *model.Application, epochs []*model.Epoch,
	consensus DaveConsensusAdapter, facts txsender.BlockRef) error {
	return s.observeTournamentState(ctx, app, epochs, consensus, facts)
}

func newPRTServiceMock() (*Service, *prtRepositoryMock) {
	repo := &prtRepositoryMock{}
	s := &Service{
		TickServiceTemplate: service.TickServiceTemplate{
			BaseTemplate: service.BaseTemplate{
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			},
		},
		repository:          repo,
		disputeWarnings:     map[common.Address]struct{}{},
		zeroStagingWarnings: map[int64]struct{}{},
		observationFailures: map[int64]tournamentObservationFailure{},
	}
	s.delivery = &delivery{Service: s, deliveryPolicy: txsender.RuntimePolicy()}
	// Protocol fixtures supply acknowledged compatible admission explicitly.
	// Deployment lifecycle tests replace this with an empty process cache.
	admitTestDeployment(s, repo, prtRevertTestApp())
	return s, repo
}
