// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type publicationRefusal string

func (e publicationRefusal) Error() string { return string(e) }
func (publicationRefusal) ErrorCode() int  { return -32000 }

func TestClaimRefusedPostEndsCandidateSelection(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, _ := queueDelivery(t, backend, lookup, 1)
	lookup.add(app, makeComputedEpoch(app, 4))
	backend.publishErr = publicationRefusal("insufficient funds for gas * price + value")
	require.Error(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 1, "one POST per pass")
}

func TestClaimPublisherDoesNotAcceptForeignStagedRoot(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Latest)
	app, epoch := deliveryApplication(1, model.Consensus_Authority)
	epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
	lookup.add(app, epoch)
	backend.apps[app.IApplicationAddress] = app
	claim := makeClaimStatus(claimStatusStaged, epoch, 30)
	claim.StagedOutputsMerkleRoot = [32]byte{0xbe, 0xef}
	backend.claims[app.ID] = claim
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Empty(t, backend.published, "a divergent staged claim is not accepted by our signer")
}

func TestClaimPublisherAcceptanceMaturityBoundary(t *testing.T) {
	for _, staged := range []uint64{40, 41} { // latest 50, period 10: 40 is mature, 41 is not
		s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Latest)
		app, epoch := deliveryApplication(1, model.Consensus_Authority)
		app.ClaimStagingPeriod = 10
		epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(staged)
		lookup.add(app, epoch)
		backend.apps[app.IApplicationAddress] = app
		backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, staged)
		require.NoError(t, s.publisher.tick(t.Context()))
		require.Equal(t, staged == 40, len(backend.published) == 1, "staged at %d", staged)
	}
}

func TestClaimPublisherPrefersReadyAcceptanceAcrossSources(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Latest)
	app, resubmit := deliveryApplication(1, model.Consensus_Authority)
	resubmit.Status = model.EpochStatus_ClaimSubmitted // chain lost it (reorg): resubmission candidate
	ready := makeComputedEpoch(app, 4)                 // DB lags; chain shows it staged and mature
	lookup.add(app, resubmit)
	lookup.add(app, ready)
	backend.apps[app.IApplicationAddress] = app
	backend.claimsAt = map[uint64]iconsensus.IConsensusClaim{ready.LastBlock: makeClaimStatus(claimStatusStaged, ready, 30)}
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 1)
	require.Equal(t, "acceptClaim", publishedClaimMethod(t, backend.published[0].Data()))
}

// The factory observes the deadline installed by the sender constructed in
// Create. Returning a sentinel ends this fixture before signing or publication.
type captureSigningDeadline struct {
	until   time.Time
	failure error
}

func (*captureSigningDeadline) From() common.Address { return common.HexToAddress("0xa1") }
func (f *captureSigningDeadline) NewTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	f.until, _ = ctx.Deadline()
	return nil, f.failure
}

type signingPolicyRPC struct {
	chainIDRPC
	finalizedViewRPC
}

func (*signingPolicyRPC) GetTransactionCount(context.Context, common.Address, rpc.BlockNumberOrHash) hexutil.Uint64 {
	return 0
}
func (*signingPolicyRPC) GetBalance(context.Context, common.Address, rpc.BlockNumberOrHash) *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(1_000_000_000_000_000_000))
}
func (*signingPolicyRPC) GetCode(context.Context, common.Address, rpc.BlockNumberOrHash) hexutil.Bytes {
	return nil
}
func (*signingPolicyRPC) MaxPriorityFeePerGas(context.Context) *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(1))
}
func (*signingPolicyRPC) EstimateGas(context.Context, map[string]any, rpc.BlockNumberOrHash) hexutil.Uint64 {
	return 21_000
}
func (*signingPolicyRPC) Call(context.Context, map[string]any, rpc.BlockNumberOrHash) hexutil.Bytes {
	return nil
}

type signingPolicyPlanner struct{}

func (signingPolicyPlanner) Next(context.Context, txsender.BlockRef, *txsender.Current[claimAction]) (
	txsender.Decision[claimAction], error,
) {
	return txsender.Publish(claimAction{}, txsender.Request{To: common.HexToAddress("0xb1")}), nil
}
func (signingPolicyPlanner) Recheck(_ context.Context, _ txsender.BlockRef, _ claimAction) (txsender.Eligibility, error) {
	return txsender.Useful, nil
}

func TestClaimCreateWiresSigningTimeout(t *testing.T) {
	const signingTimeout = 7 * time.Second
	factory := &captureSigningDeadline{failure: errors.New("signing deadline captured")}
	requested := PersistentConfig{DefaultBlock: model.DefaultBlock_Finalized, ChainID: 42,
		ClaimSubmissionEnabled: true, ResolvedSignerAddress: factory.From()}
	repo := &claimerCreateRepositoryMock{}
	repo.On("RegisterSubmitter", mock.Anything, ClaimerConfigKey, requested).Return(requested, nil).Once()
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", &signingPolicyRPC{
		chainIDRPC: chainIDRPC{chainID: 42}, finalizedViewRPC: finalizedViewRPC{final: 50}}))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	svc, err := Create(t.Context(), &CreateInfo{Config: config.ClaimerConfig{
		ClaimerPollingInterval: time.Hour, BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
		FeatureClaimSubmissionEnabled: true, BlockchainHttpRequestTimeout: signingTimeout,
	}, EthConn: client, Repository: repo, TransactOptsFactory: factory})
	require.NoError(t, err)
	s := svc.(*Service)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	before := time.Now()
	_, err = s.publisher.sender.PublishNext(ctx, signingPolicyPlanner{})
	require.ErrorIs(t, err, factory.failure)
	require.WithinDuration(t, before.Add(signingTimeout), factory.until, time.Second,
		"the configured request timeout bounds signing, rather than inheriting the one-minute caller deadline")
	repo.AssertExpectations(t)
}
