// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/stretchr/testify/mock"
)

type claimerRepositoryMock struct {
	mock.Mock
}

type claimerCreateRepositoryMock struct {
	repository.Repository
	mock.Mock
}

func (m *claimerCreateRepositoryMock) SaveNodeConfigRaw(
	ctx context.Context,
	key string,
	rawJSON []byte,
) error {
	args := m.Called(ctx, key, rawJSON)
	return args.Error(0)
}

func (m *claimerCreateRepositoryMock) LoadNodeConfigRaw(ctx context.Context, key string) (
	rawJSON []byte,
	createdAt, updatedAt time.Time,
	err error,
) {
	args := m.Called(ctx, key)
	raw, _ := args.Get(0).([]byte)
	return raw, args.Get(1).(time.Time), args.Get(2).(time.Time), args.Error(3)
}

func (m *claimerRepositoryMock) SelectClaimsToSubmitPerApp(ctx context.Context) (
	map[int64]*model.Epoch,
	map[int64]*model.Epoch,
	map[int64]*model.Application,
	error,
) {
	args := m.Called(ctx)
	return args.Get(0).(map[int64]*model.Epoch),
		args.Get(1).(map[int64]*model.Epoch),
		args.Get(2).(map[int64]*model.Application),
		args.Error(3)
}

func (m *claimerRepositoryMock) SelectClaimsToStagePerApp(ctx context.Context) (
	map[int64]*model.Epoch,
	map[int64]*model.Epoch,
	map[int64]*model.Application,
	error,
) {
	args := m.Called(ctx)
	return args.Get(0).(map[int64]*model.Epoch),
		args.Get(1).(map[int64]*model.Epoch),
		args.Get(2).(map[int64]*model.Application),
		args.Error(3)
}
func (m *claimerRepositoryMock) UpdateEpochWithSubmittedClaim(
	ctx context.Context,
	appid int64,
	index uint64,
	txHash common.Hash,
) error {
	args := m.Called(ctx, appid, index, txHash)
	return args.Error(0)
}

func (m *claimerRepositoryMock) UpdateApplicationStatus(
	ctx context.Context,
	appID int64,
	state model.ApplicationStatus,
	reason *string,
) error {
	args := m.Called(ctx, appID, state, reason)
	return args.Error(0)
}

func (m *claimerRepositoryMock) RejectEpochAndSetApplicationDiverged(
	ctx context.Context,
	appID int64,
	index uint64,
	reason string,
) error {
	args := m.Called(ctx, appID, index, reason)
	return args.Error(0)
}

func (m *claimerRepositoryMock) HasUnreconciledClaimsBeforeBlock(
	ctx context.Context,
	appID int64,
	blockBound uint64,
) (bool, error) {
	args := m.Called(ctx, appID, blockBound)
	return args.Bool(0), args.Error(1)
}

func (m *claimerRepositoryMock) HasUndrainedEpochsBeforeBlock(
	ctx context.Context,
	appID int64,
	blockBound uint64,
) (bool, error) {
	args := m.Called(ctx, appID, blockBound)
	return args.Bool(0), args.Error(1)
}

func (m *claimerRepositoryMock) ForecloseUnacceptedEpochsAtOrAfterBlock(
	ctx context.Context,
	appID int64,
	blockBound uint64,
) (int64, error) {
	args := m.Called(ctx, appID, blockBound)
	return int64(args.Int(0)), args.Error(1)
}

func (m *claimerRepositoryMock) ListApplications(
	ctx context.Context,
	f repository.ApplicationFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Application, uint64, error) {
	args := m.Called(ctx, f, p, descending)
	var apps []*model.Application
	if a := args.Get(0); a != nil {
		apps = a.([]*model.Application)
	}
	return apps, uint64(args.Int(1)), args.Error(2)
}

func (m *claimerRepositoryMock) UpdateEpochWithAcceptedClaim(
	ctx context.Context,
	appid int64,
	index uint64,
	txHash *common.Hash,
) error {
	args := m.Called(ctx, appid, index, txHash)
	return args.Error(0)
}

func (m *claimerRepositoryMock) UpdateEpochWithForeclosedClaim(
	ctx context.Context,
	appid int64,
	index uint64,
) error {
	args := m.Called(ctx, appid, index)
	return args.Error(0)
}

func (m *claimerRepositoryMock) SelectClaimsToAcceptPerApp(ctx context.Context) (
	map[int64]*model.Epoch,
	map[int64]*model.Epoch,
	map[int64]*model.Application,
	error,
) {
	args := m.Called(ctx)
	return args.Get(0).(map[int64]*model.Epoch),
		args.Get(1).(map[int64]*model.Epoch),
		args.Get(2).(map[int64]*model.Application),
		args.Error(3)
}

func (m *claimerRepositoryMock) UpdateEpochToStaged(
	ctx context.Context,
	appid int64,
	index uint64,
	stagedAtBlock uint64,
) error {
	args := m.Called(ctx, appid, index, stagedAtBlock)
	return args.Error(0)
}

func (m *claimerRepositoryMock) UpdateEpochThroughStaging(
	ctx context.Context,
	appid int64,
	index uint64,
	txHash common.Hash,
	stagedAtBlock uint64,
) error {
	args := m.Called(ctx, appid, index, txHash, stagedAtBlock)
	return args.Error(0)
}

func (m *claimerRepositoryMock) UpdateEpochReconciledStaged(
	ctx context.Context,
	appid int64,
	index uint64,
	stagedAtBlock uint64,
) error {
	args := m.Called(ctx, appid, index, stagedAtBlock)
	return args.Error(0)
}

func (m *claimerRepositoryMock) SaveNodeConfigRaw(
	ctx context.Context,
	key string,
	rawJSON []byte,
) error {
	args := m.Called(ctx, key, rawJSON)
	return args.Error(0)
}

func (m *claimerRepositoryMock) LoadNodeConfigRaw(ctx context.Context, key string) (
	rawJSON []byte,
	createdAt, updatedAt time.Time,
	err error,
) {
	args := m.Called(ctx, key)
	return args.Get(0).([]byte), args.Get(1).(time.Time), args.Get(2).(time.Time), args.Error(3)
}

type claimerBlockchainMock struct {
	mock.Mock
	submitterAddress common.Address
	hasSubmitter     bool
}

func (m *claimerBlockchainMock) claimSubmitterAddress() (common.Address, bool) {
	return m.submitterAddress, m.hasSubmitter
}

func (m *claimerBlockchainMock) findClaimSubmittedEventAndSucc(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	[]*iconsensus.IConsensusClaimSubmitted,
	error,
) {
	args := m.Called(ctx, app, epoch, fromBlock, toBlock)
	if len(args) == 4 {
		return args.Get(0).(*iconsensus.IConsensus),
			compactSubmittedEvents(args.Get(1), args.Get(2)),
			args.Error(3)
	}
	return args.Get(0).(*iconsensus.IConsensus),
		submittedEventSliceArg(args.Get(1)),
		args.Error(2)
}

func compactSubmittedEvents(values ...any) []*iconsensus.IConsensusClaimSubmitted {
	events := []*iconsensus.IConsensusClaimSubmitted{}
	for _, value := range values {
		event, ok := value.(*iconsensus.IConsensusClaimSubmitted)
		if ok && event != nil {
			events = append(events, event)
		}
	}
	return events
}

func submittedEventSliceArg(value any) []*iconsensus.IConsensusClaimSubmitted {
	if value == nil {
		return nil
	}
	events, ok := value.([]*iconsensus.IConsensusClaimSubmitted)
	if ok {
		return events
	}
	return compactSubmittedEvents(value)
}

func (m *claimerBlockchainMock) findClaimAcceptedEventAndSucc(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimAccepted,
	*iconsensus.IConsensusClaimAccepted,
	error,
) {
	args := m.Called(ctx, app, epoch, fromBlock, toBlock)
	return args.Get(0).(*iconsensus.IConsensus),
		args.Get(1).(*iconsensus.IConsensusClaimAccepted),
		args.Get(2).(*iconsensus.IConsensusClaimAccepted),
		args.Error(3)
}

func (m *claimerBlockchainMock) submitClaimToBlockchain(
	ctx context.Context,
	instance *iconsensus.IConsensus,
	app *model.Application,
	epoch *model.Epoch,
) (common.Hash, error) {
	args := m.Called(instance, app, epoch)
	return args.Get(0).(common.Hash), args.Error(1)
}

func (m *claimerBlockchainMock) acceptClaimOnBlockchain(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
) (common.Hash, error) {
	args := m.Called(app, epoch)
	return args.Get(0).(common.Hash), args.Error(1)
}

func (m *claimerBlockchainMock) findClaimStagedEventAndSucc(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimStaged,
	*iconsensus.IConsensusClaimStaged,
	error,
) {
	args := m.Called(ctx, app, epoch, fromBlock, toBlock)
	return args.Get(0).(*iconsensus.IConsensus),
		args.Get(1).(*iconsensus.IConsensusClaimStaged),
		args.Get(2).(*iconsensus.IConsensusClaimStaged),
		args.Error(3)
}

func (m *claimerBlockchainMock) getClaimStatus(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	blockNumber *big.Int,
) (iconsensus.IConsensusClaim, error) {
	args := m.Called(ctx, app, epoch, blockNumber)
	return args.Get(0).(iconsensus.IConsensusClaim), args.Error(1)
}
func (m *claimerBlockchainMock) pollTransaction(
	ctx context.Context,
	txHash common.Hash,
	endBlock *big.Int,
) (bool, *types.Receipt, error) {
	args := m.Called(ctx, txHash, endBlock)
	return args.Bool(0),
		args.Get(1).(*types.Receipt),
		args.Error(2)
}
func (m *claimerBlockchainMock) getDefaultBlockNumber(ctx context.Context) (*big.Int, error) {
	args := m.Called(ctx)
	return args.Get(0).(*big.Int),
		args.Error(1)
}

func (m *claimerBlockchainMock) getConsensusAddress(
	ctx context.Context,
	app *model.Application,
	blockNumber *big.Int,
) (common.Address, error) {
	args := m.Called(ctx, app, blockNumber)
	return args.Get(0).(common.Address),
		args.Error(1)
}

// expectNoForeignClaimAccepted registers the ClaimAccepted scan expectation
// for a CLAIM_COMPUTED epoch where no foreign claim has been accepted.
// fromBlock matches prevEpoch.LastBlock+1 (if a prev exists) or
// epoch.LastBlock+1 (otherwise) — same logic as submitClaimsAndUpdateDatabase.
func expectNoForeignClaimAccepted(b *claimerBlockchainMock, app *model.Application, epoch *model.Epoch, fromBlock, toBlock uint64) {
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, epoch, fromBlock, toBlock).
		Return(
			&iconsensus.IConsensus{},
			(*iconsensus.IConsensusClaimAccepted)(nil),
			(*iconsensus.IConsensusClaimAccepted)(nil),
			nil,
		).Once()
}

// expectGetClaimStatusUnstaged registers the pre-submit getClaim reconciliation
// expectation for the common case where the chain has not yet seen our claim,
// so the caller proceeds to broadcast.
func expectGetClaimStatusUnstaged(b *claimerBlockchainMock, app *model.Application, epoch *model.Epoch, endBlock *big.Int) {
	b.On("getClaimStatus", mock.Anything, app, epoch, endBlock).
		Return(iconsensus.IConsensusClaim{Status: 0}, nil).Once()
}

// expectPreSubmitPath registers the pre-submit pipeline expectations shared by
// the submit-revert tests for an epoch with no previous epoch: consensus
// address lookup, no foreign ClaimAccepted, no prior ClaimSubmitted events,
// and an UNSTAGED getClaim read — the path that ends with a
// submitClaimToBlockchain broadcast.
func expectPreSubmitPath(b *claimerBlockchainMock, app *model.Application, epoch *model.Epoch, endBlock *big.Int) {
	var prevEvent, currEvent *iconsensus.IConsensusClaimSubmitted
	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, epoch, epoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, epoch, epoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	expectGetClaimStatusUnstaged(b, app, epoch, endBlock)
}

func makeClaimStatus(status uint8, epoch *model.Epoch, stagedAtBlock uint64) iconsensus.IConsensusClaim {
	claim := iconsensus.IConsensusClaim{Status: status}
	if epoch.OutputsMerkleRoot != nil {
		claim.StagedOutputsMerkleRoot = *epoch.OutputsMerkleRoot
	}
	if stagedAtBlock != 0 {
		claim.StagingBlockNumber = new(big.Int).SetUint64(stagedAtBlock)
	}
	return claim
}
