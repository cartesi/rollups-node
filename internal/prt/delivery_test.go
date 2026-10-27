// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"slices"
	"sync"
	"testing"
)

const (
	largerContractBondCase = "larger exact contract bond"
	foreclosedCase         = "foreclosed"
	recipientCodeCase      = "recipient code"
	alreadyJoinedCase      = "already joined"
	runningCase            = "running"
	getTournamentMethod    = "GetTournament"
)

type deliveryRepoFake struct {
	pageMu sync.Mutex
	*prtRepositoryMock
	apps            []*model.Application
	pages           []int64
	recoveries      []repository.RootBondRecoveryCandidate
	signer          common.Address
	provenanceError error
	onProvenance    func(context.Context) error
	windows         []testTournamentInterval
	windowError     error
	targets         []testTournamentScan
	boundaryError   error
	pageError       error
	onBoundary      func()
	onPage          func()
	publishedRoots  map[string]*model.Tournament
	problems        map[int64]*model.ApplicationProblem
	stateRead       func(context.Context, string, uint64) (*repository.EpochCompletionState, error)
}

func (r *deliveryRepoFake) publishRoot(app *model.Application, epoch *model.Epoch, through uint64) {
	if r.publishedRoots == nil {
		r.publishedRoots = make(map[string]*model.Tournament)
	}
	r.publishedRoots[app.IApplicationAddress.Hex()+epoch.TournamentAddress.Hex()] = &model.Tournament{
		ApplicationID: app.ID, EpochIndex: epoch.Index, Address: *epoch.TournamentAddress,
		StartInstant: epoch.LastBlock, Snapshot: model.TournamentSnapshot{AsOfBlock: through},
	}
}

func (r *deliveryRepoFake) GetTournament(ctx context.Context, app, address string) (*model.Tournament, error) {
	for _, expectation := range r.ExpectedCalls {
		if expectation.Method == getTournamentMethod && expectation.Repeatability >= 0 {
			_, differences := expectation.Arguments.Diff([]any{ctx, app, address})
			if differences == 0 {
				return r.prtRepositoryMock.GetTournament(ctx, app, address)
			}
		}
	}
	for _, stored := range r.storedTournaments {
		if stored.Address == common.HexToAddress(address) {
			return stored, nil
		}
	}
	return r.publishedRoots[app+address], nil
}

type testTournamentInterval struct {
	ApplicationID                        int64
	ExpectedPreviousCursor, ThroughBlock uint64
	BlockHash                            common.Hash
	Batches                              []*repository.TournamentEventBatch
	CompletedScans                       []testTournamentScan
}
type testTournamentScan struct {
	EpochIndex              uint64
	Address                 common.Address
	FromBlock, ThroughBlock uint64
}

func (r *deliveryRepoFake) StoreTournamentInterval(_ context.Context, id int64, expected *uint64,
	batches []*repository.TournamentEventBatch) error {
	interval := testTournamentInterval{ApplicationID: id, Batches: batches}
	if expected != nil {
		interval.ExpectedPreviousCursor = *expected
	}
	if len(batches) > 0 {
		interval.ThroughBlock = batches[0].Tournament.Snapshot.AsOfBlock
		for i, batch := range batches {
			from := batch.Tournament.StartInstant
			if i == 0 && expected != nil {
				from = max(from, *expected+1)
			}
			interval.CompletedScans = append(interval.CompletedScans, testTournamentScan{EpochIndex: batch.Tournament.EpochIndex,
				Address: batch.Tournament.Address, FromBlock: from, ThroughBlock: interval.ThroughBlock})
		}
	}
	r.windows = append(r.windows, interval)
	if r.windowError != nil {
		return r.windowError
	}
	if r.publishedRoots == nil {
		r.publishedRoots = map[string]*model.Tournament{}
	}
	for _, app := range r.apps {
		if app.ID == id {
			for _, batch := range batches {
				r.publishedRoots[app.IApplicationAddress.Hex()+batch.Tournament.Address.Hex()] = batch.Tournament
				for i, stored := range r.storedTournaments {
					if stored.Address == batch.Tournament.Address {
						r.storedTournaments[i] = batch.Tournament
					}
				}

			}
			app.LastTournamentCheckBlock = max(app.LastTournamentCheckBlock, interval.ThroughBlock)
		}
	}
	return nil
}
func (r *deliveryRepoFake) ListTournaments(ctx context.Context, app string, filter repository.TournamentFilter,
	page repository.Pagination, descending bool) ([]*model.Tournament, uint64, error) {
	rows, count, err := r.prtRepositoryMock.ListTournaments(ctx, app, filter, page, descending)
	if err != nil || filter != (repository.TournamentFilter{}) {
		return rows, count, err
	}
	seen := map[common.Address]bool{}
	for _, row := range rows {
		seen[row.Address] = true
	}
	for key, row := range r.publishedRoots {
		if len(key) >= len(app) && key[:len(app)] == app && !seen[row.Address] {
			rows = append(rows, row)
			seen[row.Address] = true
		}
	}
	// Most callers select by name. Resolve the matching application before using
	// the fake's persisted-root map, whose key contains its address.
	for _, application := range r.apps {
		if application.Name == app {
			for key, row := range r.publishedRoots {
				address := application.IApplicationAddress.Hex()
				if len(key) >= len(address) && key[:len(address)] == address && !seen[row.Address] {
					rows = append(rows, row)
					seen[row.Address] = true
				}
			}
		}
	}
	return rows, uint64(len(rows)), nil
}

func (r *deliveryRepoFake) GetApplication(_ context.Context, address string) (*model.Application, error) {
	for _, app := range r.apps {
		if app.IApplicationAddress.Hex() == address {
			return app, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (r *deliveryRepoFake) GetEpochCompletionState(
	ctx context.Context, address string, index uint64,
) (*repository.EpochCompletionState, error) {
	if r.stateRead != nil {
		return r.stateRead(ctx, address, index)
	}
	app, err := r.GetApplication(ctx, address)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	epoch, err := r.GetEpoch(ctx, address, index)
	if err != nil {
		return nil, err
	}
	return &repository.EpochCompletionState{Application: app, Epoch: epoch, Problem: r.problems[app.ID]}, nil
}

func (r *deliveryRepoFake) GetApplicationIDBoundary(context.Context, repository.ApplicationFilter) (int64, error) {
	if r.onBoundary != nil {
		r.onBoundary()
	}
	var last int64
	for _, app := range r.apps {
		last = max(last, app.ID)
	}
	return last, r.boundaryError
}
func (r *deliveryRepoFake) ListApplicationsByID(
	_ context.Context, _ repository.ApplicationFilter, after, through int64, limit uint64,
) ([]*model.Application, error) {
	r.pageMu.Lock()
	r.pages = append(r.pages, after)
	r.pageMu.Unlock()
	if r.onPage != nil {
		r.onPage()
	}
	if r.pageError != nil {
		return nil, r.pageError
	}
	var result []*model.Application
	for _, app := range r.apps {
		if app.ID > after && app.ID <= through {
			result = append(result, app)
		}
	}
	slices.SortFunc(result, func(a, b *model.Application) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	if uint64(len(result)) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (r *deliveryRepoFake) ListRecoverableRootBonds(
	_ context.Context, _ int64, _ common.Address, _ uint64, after *uint64, through, limit uint64,
) ([]repository.RootBondRecoveryCandidate, error) {
	var result []repository.RootBondRecoveryCandidate
	for _, candidate := range r.recoveries {
		if candidate.EpochIndex <= through && (after == nil || candidate.EpochIndex > *after) {
			result = append(result, candidate)
		}
	}
	if uint64(len(result)) > limit {
		result = result[:limit]
	}
	return result, nil
}

type deliveryBackendFake struct {
	headerReads []string

	headers                   map[uint64]*types.Header
	latest, nonce             uint64
	balance                   *big.Int
	published                 []*types.Transaction
	raw                       [][]byte
	publishError, headerError error
	onPublish                 func(*types.Transaction)
	recoveryFalse             bool
	preflightError            error
	onPreflight               func(context.Context, ethereum.CallMsg, txsender.BlockRef) error
	onHeader                  func(context.Context) error

	txsender.Backend
	app            *model.Application
	apps           []*model.Application
	code           map[common.Address][]byte
	calls          []txsender.BlockRef
	foreclosure    bool
	wrongConsensus bool
	callError      error
}

func (b *deliveryBackendFake) Code(_ context.Context, address common.Address, _ txsender.BlockRef) ([]byte, error) {
	if code, ok := b.code[address]; ok {
		return code, nil
	}
	return []byte{1}, nil
}
func (b *deliveryBackendFake) Call(
	ctx context.Context, msg ethereum.CallMsg, ref txsender.BlockRef,
) ([]byte, error) {
	b.calls = append(b.calls, ref)
	if b.callError != nil {
		return nil, b.callError
	}

	if len(msg.Data) < 4 {
		return nil, nil
	}
	for _, meta := range []*bind.MetaData{itournament.ITournamentMetaData, idaveconsensus.IDaveConsensusMetaData} {
		contract, err := meta.GetAbi()
		if err != nil {
			return nil, err
		}
		method, err := contract.MethodById(msg.Data[:4])
		if err == nil {
			switch method.Name {
			case "tryRecoveringBond":
				return method.Outputs.Pack(!b.recoveryFalse)
			case "joinTournament", "stageTournamentResult", "acceptStagedTournamentResult":
				if b.onPreflight != nil {
					return nil, b.onPreflight(ctx, msg, ref)
				}
				return nil, b.preflightError
			}
		}
	}
	app := b.app
	for _, candidate := range b.apps {
		if msg.To != nil && (*msg.To == candidate.IApplicationAddress || *msg.To == candidate.IConsensusAddress) {
			app = candidate
			break
		}
	}
	appABI, err := iapplication.IApplicationMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	daveABI, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	if method, err := appABI.MethodById(msg.Data[:4]); err == nil {
		switch method.Name {
		case "isForeclosed":
			return method.Outputs.Pack(b.foreclosure)
		case "getOutputsMerkleRootValidator":
			address := app.IConsensusAddress
			if b.wrongConsensus {
				address = common.HexToAddress("0x999")
			}
			return method.Outputs.Pack(address)
		}
	}
	method, err := daveABI.MethodById(msg.Data[:4])
	if err != nil {
		return nil, err
	}
	switch method.Name {
	case "getApplicationContract":
		return method.Outputs.Pack(app.IApplicationAddress)
	case "getTournamentFactory":
		return method.Outputs.Pack(common.HexToAddress("0xfa"))
	}
	return nil, fmt.Errorf("unexpected call %s", method.Name)
}

func TestDeliveryLatestForeclosureAndRelationshipRefusePublication(t *testing.T) {
	for _, failure := range []string{foreclosedCase, "wrong consensus", "missing code", "unavailable view"} {
		t.Run(failure, func(t *testing.T) {
			f := newDeliveryFixture(t)
			f.snapshot(resultTestSnapshot(f.epoch, false))
			switch failure {
			case foreclosedCase:
				f.backend.foreclosure = true
			case "wrong consensus":
				f.backend.wrongConsensus = true
			case "missing code":
				f.backend.code[f.app.IConsensusAddress] = nil
			case "unavailable view":
				f.backend.callError = errors.New("view unavailable")
			}
			call, err := f.d.planAction(t.Context(), f.app, stageTournament, f.block, txsender.BlockRef{Number: 100})
			require.Error(t, err)
			require.Nil(t, call)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestDeliveryCallIdentityIncludesTargetValueAndCalldata(t *testing.T) {
	f := newDeliveryFixture(t)
	first := f.d.call(f.app, 3, joinTournament, common.HexToAddress("0x1"), []byte{1}, big.NewInt(1), 120)
	for _, call := range []plannedAction{
		f.d.call(f.app, 3, joinTournament, common.HexToAddress("0x2"), []byte{1}, big.NewInt(1), 120),
		f.d.call(f.app, 3, joinTournament, common.HexToAddress("0x1"), []byte{2}, big.NewInt(1), 120),
		f.d.call(f.app, 3, joinTournament, common.HexToAddress("0x1"), []byte{1}, big.NewInt(2), 120),
	} {
		require.NotEqual(t, first.Key, call.Key)
	}
}

func TestDeliveryRecoveryNeedsRecoverableConfiguredCodeFreeRecipient(t *testing.T) {
	for _, caseName := range []string{"recovered no-op", "foreign", recipientCodeCase, runningCase, "no winner"} {
		t.Run(caseName, func(t *testing.T) {
			f := newDeliveryFixture(t)
			target := common.HexToAddress("0x900")
			recovery := canonicalBondRecovery(model.BondDispositionRecoverable, f.repo.signer, 1)
			f.backend.code[recovery.Claimer] = nil
			switch caseName {
			case "recovered no-op":
				recovery.Disposition = model.BondDispositionRecovered
			case "foreign":
				recovery.Claimer = common.HexToAddress("0x700")
			case recipientCodeCase:
				f.backend.code[recovery.Claimer] = []byte{1}
			case runningCase:
				recovery.Disposition = model.BondDispositionTournamentRunning
			case "no winner":
				recovery.Disposition = model.BondDispositionNoWinner
			}
			adapter := &tournamentAdapterMock{}
			adapter.On("BondRecovery", mock.Anything).Return(recovery, nil).Once()
			f.factory.On("CreateTournamentAdapter", target).Return(adapter, nil).Once()
			call, err := f.d.recoveryCall(withLiveBlock(t.Context(), f.block.Number, f.block.Hash),
				f.app, 2, target, f.block, txsender.BlockRef{Number: 100})
			require.Nil(t, call)
			if caseName == recipientCodeCase {
				require.ErrorContains(t, err, "unsupported code")
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, f.backend.published)
		})
	}
}

func TestDeliveryJoinHasInclusiveDeadlineAndExactBondGuards(t *testing.T) {
	cases := []string{"valid", "deadline next block", largerContractBondCase, "wrong geometry", "missing target code", alreadyJoinedCase}
	for _, caseName := range cases {
		t.Run(caseName, func(t *testing.T) {
			f := newDeliveryFixture(t)
			target := *f.epoch.TournamentAddress
			adapter := &tournamentAdapterMock{}
			standing := CommitmentStanding{}
			descriptor := TournamentDescriptor{Level: 0, Height: model.Log2EpochComputationHashLeafCount,
				Log2Stride:   model.Log2RootTournamentStride,
				StartInstant: 100, Allowance: 20}
			bond := big.NewInt(50)
			switch caseName {
			case "deadline next block":
				descriptor.Allowance = 11
			case largerContractBondCase:
				bond = big.NewInt(51)
			case "wrong geometry":
				descriptor.Height--
			case "missing target code":
				f.backend.code[target] = nil
			case alreadyJoinedCase:
				standing.Joined = true
				standing.FinalState = *f.epoch.MachineHash
			}
			adapter.On("CommitmentStanding", mock.Anything, [32]byte(*f.epoch.Commitment)).Return(standing, nil).Maybe()
			adapter.On("Descriptor", mock.Anything).Return(descriptor, nil).Maybe()
			adapter.On("Standing", mock.Anything).Return(TournamentStanding{AcceptsJoins: true}, nil).Maybe()
			adapter.On("BondValue", mock.Anything).Return(bond, nil).Maybe()
			f.factory.On("CreateTournamentAdapter", target).Return(adapter, nil).Maybe()
			call, err := f.d.joinCall(withLiveBlock(t.Context(), f.block.Number, f.block.Hash), f.app, f.epoch, f.block)
			switch caseName {
			case "valid", largerContractBondCase:
				require.NoError(t, err)
				require.Equal(t, joinTournament, call.Key.Kind)
				require.Equal(t, bond, call.Value)
				require.Equal(t, uint64(120), call.Deadline)
				abi, err := itournament.ITournamentMetaData.GetAbi()
				require.NoError(t, err)
				method, err := abi.MethodById(call.Data[:4])
				require.NoError(t, err)
				require.Equal(t, "joinTournament", method.Name)
			case alreadyJoinedCase:
				require.NoError(t, err)
				require.Zero(t, call.Key.Kind)
			default:
				require.Error(t, err)
			}
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestDeliveryActionSweepUsesIDsAndFiniteBoundary(t *testing.T) {
	f := newDeliveryFixture(t)
	f.repo.apps = nil
	for id := int64(1); id <= 12; id++ {
		f.repo.apps = append(f.repo.apps, &model.Application{ID: id, Name: fmt.Sprint(20 - id)})
	}
	first, err := f.d.nextDeliveryApplications(t.Context(), recoverBond)
	require.NoError(t, err)
	require.Len(t, first, 8)
	require.Equal(t, int64(1), first[0].ID)
	f.repo.apps = append(f.repo.apps, &model.Application{ID: 13, Name: "a"})
	second, err := f.d.nextDeliveryApplications(t.Context(), recoverBond)
	require.NoError(t, err)
	require.Len(t, second, 4)
	require.Equal(t, int64(12), second[3].ID)
	third, err := f.d.nextDeliveryApplications(t.Context(), recoverBond)
	require.NoError(t, err)
	require.Len(t, third, 8)
	require.Equal(t, int64(1), third[0].ID)
	require.Equal(t, int64(13), f.d.deliverySweeps[recoverBond].through)
}

func (r *deliveryRepoFake) CheckSubmitterProvenance(ctx context.Context, key string) error {
	if key != PrtConfigKey {
		return errors.New("unexpected submitter provenance key")
	}
	if r.onProvenance != nil {
		return r.onProvenance(ctx)
	}
	return r.provenanceError
}

// chargeReverts settles n matched reverted stages at nonces no test reserves.

// dispatchRound is a test entry point for a publisher with an already selected observation reference.
