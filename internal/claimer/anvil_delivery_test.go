// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package claimer

import (
	"context"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorumfactory"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	"math/big"
	"os"
	"testing"
	"time"
)

const anvilClaimFixtureTimeout = 2 * time.Minute

// X12 runs against the installed alpha.10 factories with independently funded
// random accounts. It does not reset Anvil, revert snapshots, or use a node key.
func TestAnvilQuorumDecidingVoteGas(t *testing.T) {
	if os.Getenv("CARTESI_CLAIMER_ANVIL_TEST") != "1" {
		t.Skip("set CARTESI_CLAIMER_ANVIL_TEST=1 to run fresh-contract Anvil release fixtures")
	}
	ctx, cancel := context.WithTimeout(t.Context(), anvilClaimFixtureTimeout)
	defer cancel()
	endpoint := os.Getenv("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8545"
	}
	client, err := ethclient.DialContext(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	chainID, err := client.ChainID(ctx)
	require.NoError(t, err)
	quorumFactory, err := iquorumfactory.NewIQuorumFactory(anvilFixtureAddress(
		"CARTESI_CONTRACTS_QUORUM_FACTORY_ADDRESS", "0x0754D5Eb680c71bf469B39e48C5b64AB0813fdb9"), client)
	require.NoError(t, err)
	version, err := quorumFactory.Version(&bind.CallOpts{Context: ctx})
	require.NoError(t, err)
	require.EqualValues(t, 3, version.Major)
	require.Equal(t, "alpha.10", version.PreRelease, "this profile is calibrated for the repository's supported version")
	appFactory, err := iapplicationfactory.NewIApplicationFactory(anvilFixtureAddress(
		"CARTESI_CONTRACTS_APPLICATION_FACTORY_ADDRESS", "0x35Cd91f13141Bb6A6FC69E1eeDD241bbA1Ddd45F"), client)
	require.NoError(t, err)
	inputBox := anvilFixtureAddress("CARTESI_CONTRACTS_INPUT_BOX_ADDRESS", "0xEbE9f4Dfc04ae10bBeE663859c3dc5A23f94eA3C")
	for _, fixture := range []struct {
		name       string
		expensive  bool
		calibrated bool
	}{
		{"ordinary_estimate_only", false, false},
		{"ordinary_calibrated", false, true},
		{"expensive_estimate_only", true, false},
		{"expensive_calibrated", true, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			node, rival, other := anvilClaimSigner(ctx, t, client, chainID),
				anvilClaimSigner(ctx, t, client, chainID), anvilClaimSigner(ctx, t, client, chainID)
			validators := []common.Address{node.From, rival.From, other.From}
			salt := crypto.Keccak256Hash(node.From.Bytes())
			quorumAddress, err := quorumFactory.CalculateQuorumAddress(
				&bind.CallOpts{Context: ctx}, validators, big.NewInt(1), new(big.Int), salt)
			require.NoError(t, err)
			tx, err := quorumFactory.NewQuorum(node, validators, big.NewInt(1), new(big.Int), salt)
			require.NoError(t, err)
			anvilClaimMined(ctx, t, client, tx)
			withdrawal := iapplicationfactory.WithdrawalConfig{}
			appAddress, err := appFactory.CalculateApplicationAddress(
				&bind.CallOpts{Context: ctx}, quorumAddress, node.From, salt, inputBox, withdrawal, salt)
			require.NoError(t, err)
			tx, err = appFactory.NewApplication0(node, quorumAddress, node.From, salt, inputBox, withdrawal, salt)
			require.NoError(t, err)
			deployment := anvilClaimMined(ctx, t, client, tx)
			if fixture.expensive {
				appAddress, deployment = anvilExpensiveAppProxy(ctx, t, client, node, appAddress)
			}
			facts := anvilClaimFinalized(ctx, t, client, deployment.BlockNumber.Uint64())
			app, epoch := deliveryApplication(1, model.Consensus_Quorum)
			app.IApplicationAddress, app.IConsensusAddress = appAddress, quorumAddress
			app.ClaimStagingPeriod = 0
			epoch.LastBlock = facts.Number // epoch length one: every block is an epoch final block
			s, _, lookup := newDeliveryFixture(t)
			backend := &decidingVoteBackend{ProtectedClient: txsender.ProtectedClient{Client: client}}
			s.publisher.backend, s.publisher.signer = backend, node.From
			s.blockchain = &claimerBlockchain{txOptsFactory: ethutil.NewStaticTransactOptsFactory(node)}
			lookup.add(app, epoch)
			s.publisher.sender, err = txsender.New[claimAction](backend, ethutil.NewStaticTransactOptsFactory(node), chainID.Uint64(),
				txsender.Policy{MaxFee: big.NewInt(1_000_000_000_000)}, func(context.Context) error { return nil }, s.Logger)
			require.NoError(t, err)
			candidate, err := claimCall(app, epoch, mustEpochProof(t, epoch), true)
			require.NoError(t, err)

			before, err := client.EstimateGas(ctx, ethereum.CallMsg{From: node.From, To: &quorumAddress, Data: candidate.request.Data})
			require.NoError(t, err)
			quorum, err := iconsensus.NewIConsensus(quorumAddress, client)
			require.NoError(t, err)
			var after uint64
			// The rival executes after our final preflight, immediately before raw
			// publication. This exercises the unavoidable post-check state race.
			backend.beforePublication = func() {
				rivalTx, rivalErr := quorum.SubmitClaim(rival, appAddress, new(big.Int).SetUint64(epoch.LastBlock),
					[32]byte(*epoch.MachineHash), consensusMachineValidityProof(mustEpochProof(t, epoch)))
				require.NoError(t, rivalErr)
				anvilClaimMined(ctx, t, client, rivalTx)
				after, rivalErr = client.EstimateGas(ctx, ethereum.CallMsg{
					From: node.From, To: &quorumAddress, Data: candidate.request.Data})
				require.NoError(t, rivalErr)
				require.Greater(t, after, before)
			}
			var hash common.Hash
			if fixture.calibrated {
				planner := &claimFixturePlanner{publisher: s.publisher, candidate: candidate}
				report, publishErr := s.publisher.sender.PublishNext(ctx, planner)
				require.NoError(t, publishErr)
				require.True(t, report.Attempted)
				hash = report.Hash
			} else {
				hash = anvilPublishEstimateControl(ctx, t, backend, node, chainID, candidate.request, before)
			}

			mined, _, err := client.TransactionByHash(ctx, hash)
			require.NoError(t, err)
			t.Logf("selected transaction gas=%d", mined.Gas())
			receipt, err := bind.WaitMined(ctx, client, mined)
			require.NoError(t, err)
			if fixture.calibrated {
				require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status)
				require.GreaterOrEqual(t, mined.Gas(), quorumSubmitGasLimit)
			} else {
				require.Equal(t, uint64(types.ReceiptStatusFailed), receipt.Status,
					"the estimate-only control must expose the deciding-vote gas race")
			}
			binding, err := iconsensus.NewIConsensus(quorumAddress, nil)
			require.NoError(t, err)
			staged := false
			for _, log := range receipt.Logs {
				if _, err := binding.ParseClaimStaged(*log); err == nil {
					staged = true
				}
			}
			require.Equal(t, fixture.calibrated, staged, "only the calibrated deciding vote must stage the claim")

			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status,
				"even an actual status-1 execution needs Observe before durable domain progress")
			code, err := client.CodeAt(ctx, quorumAddress, nil)
			require.NoError(t, err)
			t.Logf("Quorum %s: before-estimate=%d deciding-estimate=%d selected-gas=%d gas-used=%d code=%s",
				version.PreRelease, before, after, mined.Gas(), receipt.GasUsed, crypto.Keccak256Hash(code))
		})
	}
}

// The negative control deliberately signs the raw estimate as an exact limit.
// Request.GasLimit is a floor, so passing it through Sender would add padding
// and stop testing the insufficient-gas envelope. Both paths run the rival only
// after their last exact preflight and before publishing the signed bytes.
func anvilPublishEstimateControl(
	ctx context.Context, t *testing.T, backend *decidingVoteBackend, signer *bind.TransactOpts,
	chainID *big.Int, request txsender.Request, gas uint64,
) common.Hash {
	t.Helper()
	header, err := backend.Header(ctx, "latest")
	require.NoError(t, err)
	block := txsender.BlockRef{Number: header.Number.Uint64(), Hash: header.Hash()}
	nonce, err := backend.Nonce(ctx, signer.From, block)
	require.NoError(t, err)
	tip, err := backend.Tip(ctx)
	require.NoError(t, err)
	feeCap := new(big.Int).Add(new(big.Int).Mul(header.BaseFee, big.NewInt(2)), tip)
	unsigned := types.NewTx(&types.DynamicFeeTx{
		ChainID: chainID, Nonce: nonce, To: &request.To, Value: request.Value, Data: request.Data,
		Gas: gas, GasFeeCap: feeCap, GasTipCap: tip,
	})
	signed, err := signer.Signer(signer.From, unsigned)
	require.NoError(t, err)
	raw, err := signed.MarshalBinary()
	require.NoError(t, err)
	_, err = backend.Call(ctx, ethereum.CallMsg{
		From: signer.From, To: &request.To, Value: request.Value, Data: request.Data,
		Gas: gas, GasFeeCap: feeCap, GasTipCap: tip,
	}, block)
	require.NoError(t, err, "the raw estimate must work before the competing vote")
	hash, err := backend.Publish(ctx, raw)
	require.NoError(t, err)
	require.Equal(t, signed.Hash(), hash)
	return hash
}

type decidingVoteBackend struct {
	txsender.ProtectedClient
	beforePublication func()
}

func (b *decidingVoteBackend) Publish(ctx context.Context, raw []byte) (common.Hash, error) {
	if callback := b.beforePublication; callback != nil {
		b.beforePublication = nil
		callback()
	}
	return b.ProtectedClient.Publish(ctx, raw)
}

func anvilFixtureAddress(key, fallback string) common.Address {
	address := os.Getenv(key)
	if address == "" {
		address = fallback
	}
	return common.HexToAddress(address)
}

func anvilClaimSigner(ctx context.Context, t *testing.T, client *ethclient.Client, chainID *big.Int) *bind.TransactOpts {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	opts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	require.NoError(t, err)
	opts.Context = ctx
	var funded any
	require.NoError(t, client.Client().CallContext(ctx, &funded, "anvil_setBalance", opts.From, "0x56bc75e2d63100000"))
	return opts
}

func anvilClaimMined(ctx context.Context, t *testing.T, client *ethclient.Client, tx *types.Transaction) *types.Receipt {
	t.Helper()
	receipt, err := bind.WaitMined(ctx, client, tx)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status)
	return receipt
}

func anvilClaimFinalized(ctx context.Context, t *testing.T, client *ethclient.Client, atLeast uint64) txsender.BlockRef {
	t.Helper()
	wait, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	poll := time.NewTicker(250 * time.Millisecond)
	defer poll.Stop()
	backend := txsender.ProtectedClient{Client: client}
	for {
		header, err := backend.Header(wait, "finalized")
		require.NoError(t, err)
		if header.Number.Uint64() >= atLeast && header.Number.Sign() > 0 {
			latest, err := backend.Header(wait, "latest")
			require.NoError(t, err)
			t.Logf("natural producer: latest=%d finalized=%d finality lag=%d blocks",
				latest.Number.Uint64(), header.Number.Uint64(), latest.Number.Uint64()-header.Number.Uint64())
			return txsender.BlockRef{Number: header.Number.Uint64(), Hash: header.Hash()}
		}
		select {
		case <-wait.Done():
			t.Fatalf("fresh application or receipt did not become naturally finalized: %v", wait.Err())
		case <-poll.C:
		}
	}
}

// The proxy expands memory before forwarding every STATICCALL to a genuine
// supported Application. Both consensus prerequisite callbacks can therefore
// succeed at high cost. It propagates the exact return or revert bytes.
func anvilExpensiveAppProxy(
	ctx context.Context, t *testing.T, client *ethclient.Client, opts *bind.TransactOpts, target common.Address,
) (common.Address, *types.Receipt) {
	t.Helper()
	runtime := "600062040000523660006000376000600036600073" + common.Bytes2Hex(target[:]) +
		"5afa6038573d600060003e3d6000fd5b3d600060003e3d6000f3"
	init := common.FromHex("0x6043600c60003960436000f3" + runtime)
	address, tx, _, err := bind.DeployContract(opts, abi.ABI{}, init, client)
	require.NoError(t, err)
	return address, anvilClaimMined(ctx, t, client, tx)
}

// Calibrated cases use the actual Claimer post-sign protocol check and sender.
type claimFixturePlanner struct {
	publisher *claimPublisher
	candidate claimCandidate
}

func (p *claimFixturePlanner) Next(context.Context, txsender.BlockRef, *txsender.Current[claimAction]) (
	txsender.Decision[claimAction], error,
) {
	return txsender.Publish(p.candidate.action, p.candidate.request), nil
}
func (p *claimFixturePlanner) Recheck(ctx context.Context, block txsender.BlockRef, action claimAction) (
	txsender.Eligibility, error,
) {
	return p.publisher.Recheck(ctx, block, action)
}
