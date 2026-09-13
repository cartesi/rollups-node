// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	jsonrpcclient "github.com/cartesi/rollups-node/pkg/jsonrpc/client"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

const (
	passiveObserverTimeout = 10 * time.Minute
	passiveObserverPoll    = 250 * time.Millisecond
	passiveActorOneIndex   = 12
	passiveActorTwoIndex   = 13
	passiveLeafJoinDelay   = 64
)

// TestPrtPassiveDisputeObserver uses external actors, not node dispute logic.
// One actor submits exactly the node's empty-epoch commitment; the other changes
// only its final leaf. Both use valid membership proofs through the official
// three levels. Actor A wins by timeout; no VM step witness is fabricated.
// The node must expose all phases, both when its local commitment wins and when
// it loses. Only the matching winner is staged with a real machine-state proof.
func TestPrtPassiveDisputeObserver(t *testing.T) {
	if !isNodeSelfManaged() {
		t.Skip("passive dispute fixture requires a test-managed node to disable claim submission")
	}
	for _, scenario := range []struct {
		name      string
		localWins bool
	}{
		{name: "matching_external_winner", localWins: true},
		{name: "losing_local_commitment", localWins: false},
	} {
		t.Run(scenario.name, func(t *testing.T) { runPassiveDisputeObserver(t, scenario.localWins) })
	}
}

func runPassiveDisputeObserver(t *testing.T, localWins bool) {
	ctx, cancel := context.WithTimeout(context.Background(), passiveObserverTimeout)
	defer cancel()
	client := newIntegrationEthClient(ctx, t)
	defer client.Close()
	startReaderNode(ctx, t)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		require.NoError(t, anvilRPC(cleanupCtx, "evm_setAutomine", true))
		require.NoError(t, anvilRPC(cleanupCtx, "evm_setIntervalMining", 1))
	})
	setAnvilIntervalMining(ctx, t, 0)
	setAnvilAutomine(ctx, t, true)
	var logs LogChecker
	logs.StartLogCapture()
	t.Cleanup(func() { logs.CheckLogs(t) })
	fixture := newPassiveDisputeFixture(ctx, t, client, localWins)
	if !localWins {
		reason := regexp.QuoteMeta(fmt.Sprintf("Epoch 0 has inconsistent commitment between off-chain (%s) and on-chain (%s)",
			*fixture.epoch.Commitment, fixture.layers[0].trees[0].root()))
		logs.SetExpectedLogs(t,
			ExpectedLog{
				Pattern: regexp.MustCompile("marking application as diverged.*application=" + regexp.QuoteMeta(fixture.appName) +
					".*" + reason),
				Level: LevelError, Required: true,
				Reason: "the external timeout winner differs from this application's locally computed commitment",
			},
			ExpectedLog{
				Pattern: regexp.MustCompile("Tick.*service=prt.*validating PRT application " +
					regexp.QuoteMeta(fixture.appAddress.Hex()) + ": " + reason),
				Level: LevelError, Required: true,
				Reason: "the PRT service reports this application's expected divergence transition to the service loop",
			},
		)
	}
	fixture.run()
}

type passiveDisputeFixture struct {
	t          *testing.T
	r          *require.Assertions
	ctx        context.Context
	client     *ethclient.Client
	rpc        *jsonrpcclient.Client
	appName    string
	appAddress common.Address
	localWins  bool
	epoch      *Epoch
	consensus  *idaveconsensus.IDaveConsensus
	actors     [2]*bind.TransactOpts
	layers     []*passiveDisputeLayer
	receipts   []*types.Receipt
}

type passiveDisputeLayer struct {
	address    common.Address
	contract   *itournament.ITournament
	descriptor itournament.ITournamentTournamentDescriptor
	trees      [2]*sparseDisputeCommitment
	matchID    itournament.MatchId
	matchHash  common.Hash
}

func newPassiveDisputeFixture(ctx context.Context, t *testing.T, client *ethclient.Client, localWins bool) *passiveDisputeFixture {
	t.Helper()
	f := &passiveDisputeFixture{t: t, r: require.New(t), ctx: ctx, client: client,
		rpc:     jsonrpcclient.NewClient(envOrDefault("CARTESI_JSONRPC_API_URL", "http://localhost:10011/rpc")),
		appName: uniqueAppName("prt-passive-dispute"), localWins: localWins}
	f.actors[0] = transactorForMnemonicIndex(ctx, t, client, passiveActorOneIndex)
	f.actors[1] = transactorForMnemonicIndex(ctx, t, client, passiveActorTwoIndex)
	for _, actor := range f.actors {
		f.r.NoError(anvilSetBalance(ctx, actor.From.Hex(), "0x3635c9adc5dea00000"), "fund external dispute actor")
	}
	_, err := deployApplication(ctx, f.appName, envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp"),
		prtFlag, claimStagingPeriodFlag, "5", "--salt", uniqueSalt())
	f.r.NoError(err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		f.r.NoError(disableApplication(cleanupCtx, f.appName))
	})
	_, err = waitForEpochStatus(ctx, t, f.appName, 0, EpochStatus_ClaimComputed)
	f.r.NoError(err, "compute the empty epoch without node submission")
	dsn, err := config.GetDatabaseConnection()
	f.r.NoError(err)
	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	f.r.NoError(err)
	defer repo.Close()
	f.epoch, err = repo.GetEpoch(ctx, f.appName, 0)
	f.r.NoError(err)
	f.r.NotNil(f.epoch)
	f.r.NotNil(f.epoch.MachineHash)
	f.r.NotNil(f.epoch.Commitment)
	f.r.NotNil(f.epoch.TournamentAddress)
	f.r.Equal(f.epoch.InputIndexLowerBound, f.epoch.InputIndexUpperBound, "epoch 0 must be empty")
	app, err := repo.GetApplication(ctx, f.appName)
	f.r.NoError(err)
	f.r.NotNil(app)
	f.appAddress = app.IApplicationAddress
	f.consensus, err = idaveconsensus.NewIDaveConsensus(app.IConsensusAddress, client)
	f.r.NoError(err)
	f.newLayer(*f.epoch.TournamentAddress)
	localActor := 0
	if !localWins {
		localActor = 1
	}
	f.r.Equal(*f.epoch.Commitment, f.layers[0].trees[localActor].root(), "one actor must submit the node's exact commitment")
	f.r.Equal(*f.epoch.MachineHash, common.Hash(f.layers[0].descriptor.InitialHash), "empty epoch state must remain unchanged")
	return f
}

func (f *passiveDisputeFixture) run() {
	for level := 0; level < 3; level++ {
		layer := f.layers[level]
		f.joinPair(layer)
		f.assertCurrent(layer, MatchPhaseBisecting)
		f.bisect(layer)
		f.assertCurrent(layer, MatchPhaseReadyToSeal)
		child := f.seal(layer)
		f.assertCurrent(layer, MatchPhaseSealed)
		if child != (common.Address{}) {
			f.newLayer(child)
		}
	}
	f.winLeafByTimeout()
	for level := len(f.layers) - 2; level >= 0; level-- {
		parent, child := f.layers[level], f.layers[level+1]
		f.assertWinner(child, TournamentStandingInnerWinner)
		left, right, err := parent.trees[0].rightmostChildren(parent.descriptor.Height)
		f.r.NoError(err)
		tx, err := parent.contract.WinInnerTournament(f.actors[0], child.address, left, right)
		f.record(tx, err)
		f.assertCurrent(parent, MatchPhaseUninitialized)
	}
	f.assertWinner(f.layers[0], TournamentStandingRootWinner)
	if f.localWins {
		f.stageAndAccept()
	} else {
		f.assertLocalDivergence()
	}
	// The completed root remains observable after acceptance or divergence.
	tx, err := f.layers[0].contract.TryRecoveringBond(f.actors[0])
	f.record(tx, err)
	f.assertRecoveredRoot()
	// The root can now retire. Its children must still receive no-event
	// snapshot updates when their winner carryover windows expire.
	f.assertInnerWinnersExpire()
	f.assertAllEvents()
	f.assertOnlyExternalTransactions()
	if !f.localWins {
		f.assertLocalDivergence()
	}
}

func (f *passiveDisputeFixture) newLayer(address common.Address) {
	contract, err := itournament.NewITournament(address, f.client)
	f.r.NoError(err)
	descriptor, err := contract.TournamentDescriptor(&bind.CallOpts{Context: f.ctx})
	f.r.NoError(err)
	heights := []uint64{48, 17, 27}
	strides := []uint64{44, 27, 0}
	level := len(f.layers)
	f.r.Less(level, len(heights))
	f.r.Equal(uint64(level), descriptor.Level)
	f.r.Equal(heights[level], descriptor.Height)
	f.r.Equal(strides[level], descriptor.Log2Stride)
	f.r.Greater(descriptor.Allowance, descriptor.Height+passiveLeafJoinDelay,
		"official allowance must leave time for the complete fixture")
	state := *f.epoch.MachineHash
	falseState := crypto.Keccak256Hash(state[:], []byte("external incorrect final state"))
	correct, err := newSparseDisputeCommitment(descriptor.Height, state, state)
	f.r.NoError(err)
	incorrect, err := newSparseDisputeCommitment(descriptor.Height, state, falseState)
	f.r.NoError(err)
	oneTree, twoTree := correct, incorrect
	if !f.localWins {
		oneTree, twoTree = incorrect, correct
	}
	one, two := oneTree.root(), twoTree.root()
	f.layers = append(f.layers, &passiveDisputeLayer{address: address, contract: contract, descriptor: descriptor,
		trees:   [2]*sparseDisputeCommitment{oneTree, twoTree},
		matchID: itournament.MatchId{CommitmentOne: one, CommitmentTwo: two}, matchHash: crypto.Keccak256Hash(one[:], two[:])})
}

func (f *passiveDisputeFixture) record(tx *types.Transaction, err error) *types.Receipt {
	f.t.Helper()
	f.r.NoError(err, "send external participant transaction")
	f.r.NotNil(tx)
	ctx, cancel := context.WithTimeout(f.ctx, time.Minute)
	defer cancel()
	receipt := waitReceipt(ctx, f.t, f.client, tx)
	f.r.Equal(types.ReceiptStatusSuccessful, receipt.Status)
	f.receipts = append(f.receipts, receipt)
	return receipt
}

func (f *passiveDisputeFixture) mineTo(target uint64) {
	f.t.Helper()
	head, err := f.client.BlockNumber(f.ctx)
	f.r.NoError(err)
	if target <= head {
		return
	}
	f.r.LessOrEqual(target-head, uint64(maxBlocksToMine))
	f.r.NoError(anvilRPC(f.ctx, "anvil_mine", hexutil.EncodeUint64(target-head)), "mine a bounded timeout or staging interval")
}

func (f *passiveDisputeFixture) stageAndAccept() {
	epoch := f.epoch
	f.r.NotNil(epoch.IflagsYDataBlock)
	f.r.NotNil(epoch.HtifTohostDataBlock)
	f.r.NotNil(epoch.TxBufferDataBlock)
	leaf := func(data common.Hash, siblings []common.Hash) idaveconsensus.LeafProof {
		proof := make([][32]byte, len(siblings))
		for i, sibling := range siblings {
			proof[i] = sibling
		}
		return idaveconsensus.LeafProof{DataBlock: data, Siblings: proof}
	}
	proof := idaveconsensus.MachineValidityProof{
		IflagsYProof:    leaf(*epoch.IflagsYDataBlock, epoch.IflagsYProof),
		HtifTohostProof: leaf(*epoch.HtifTohostDataBlock, epoch.HtifTohostProof),
		TxBufferProof:   leaf(*epoch.TxBufferDataBlock, epoch.TxBufferProof),
	}
	tx, err := f.consensus.StageTournamentResult(f.actors[0], new(big.Int), proof)
	stagedReceipt := f.record(tx, err)
	staged, err := waitForEpochStatus(f.ctx, f.t, f.appName, 0, EpochStatus_ClaimStaged)
	f.r.NoError(err)
	f.r.NotNil(staged.StagedAtBlock)
	f.r.Equal(stagedReceipt.BlockNumber.Uint64(), *staged.StagedAtBlock)
	period, err := f.consensus.GetClaimStagingPeriod(&bind.CallOpts{Context: f.ctx})
	f.r.NoError(err)
	f.r.True(period.IsUint64())
	f.mineTo(*staged.StagedAtBlock + period.Uint64())
	tx, err = f.consensus.AcceptStagedTournamentResult(f.actors[0], new(big.Int))
	acceptedReceipt := f.record(tx, err)
	accepted, err := waitForEpochStatus(f.ctx, f.t, f.appName, 0, EpochStatus_ClaimAccepted)
	f.r.NoError(err)
	f.r.Equal(epoch.MachineHash, accepted.MachineHash)
	f.r.Equal(epoch.Commitment, accepted.Commitment)
	f.r.NotNil(accepted.ClaimTransactionHash)
	f.r.Equal(acceptedReceipt.TxHash, *accepted.ClaimTransactionHash)
	var app api.SingleResponse[*Application]
	f.r.NoError(f.rpc.Call(f.ctx, "cartesi_getApplication", api.GetApplicationParams{Application: f.appName}, &app))
	f.r.NotNil(app.Data)
	f.r.Equal(ApplicationStatus_OK, app.Data.Status, "an identical external winner must not cause divergence")
	f.r.True(app.Data.Enabled)
}
