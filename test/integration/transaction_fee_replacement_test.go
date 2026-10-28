// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestTransactionFeeReplacement(t *testing.T) {
	if !isNodeSelfManaged() {
		t.Skip("fee replacement requires a test-managed node to pause publication")
	}
	ctx, backend := prepareFeeAnvil(t)
	t.Run("replace a pending bid after fee rise and restart", func(t *testing.T) {
		testAnvilPendingFeeReplacement(ctx, t, backend)
	})
	for _, scenario := range []struct {
		name           string
		capped, legacy bool
	}{
		{name: "fee spike after pool eviction"},
		{name: "fee ceiling then operator restart", capped: true},
		{name: "legacy fee spike after pool eviction", legacy: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			feeAnvilMine(ctx, t, big.NewInt(1_000_000_000))
			signer := newFeeSigner(t) // Real ECDSA signing, with a count of signing requests.
			require.NoError(t, anvilRPC(ctx, "anvil_setBalance", signer.From(), "0x56bc75e2d63100000"))
			// Increment storage slot zero. Its final value detects duplicate execution.
			target := crypto.CreateAddress(signer.From(), 0)
			require.NoError(t, anvilRPC(ctx, "anvil_setCode", target, "0x60005460010160005500"))
			planner := &feePlanner{}
			planner.request = txsender.Request{To: target, Value: new(big.Int)}
			policy := txsender.Policy{Legacy: scenario.legacy}
			if scenario.capped {
				policy.MaxFee = big.NewInt(4_000_000_000)
			}
			sender := feeAnvilSender(t, backend, signer, policy)
			first, err := sender.PublishNext(ctx, planner)
			require.NoError(t, err)
			require.Equal(t, txsender.Accepted, first.Class)
			original, pending, err := backend.Client.TransactionByHash(ctx, first.Hash)
			require.NoError(t, err)
			require.True(t, pending, "manual mining must leave the original in the real transaction pool")

			// Deliberately override one block's fee to make the old cap unusable.
			// This is a controlled stress case, not a claim about L1 fee-change cadence.
			spike := new(big.Int).Mul(original.GasFeeCap(), big.NewInt(2))
			feeAnvilMine(ctx, t, spike)
			header, err := backend.Client.HeaderByNumber(ctx, nil)
			require.NoError(t, err)
			require.Equal(t, spike, header.BaseFee)
			_, _, err = backend.Client.TransactionByHash(ctx, first.Hash)
			require.ErrorIs(t, err, ethereum.NotFound, "Anvil evicts a bid below the new base fee when mining")
			_, err = backend.Client.TransactionReceipt(ctx, first.Hash)
			require.ErrorIs(t, err, ethereum.NotFound, "the old cap cannot pay the new base fee")
			if scenario.capped {
				blocked, publishErr := sender.PublishNext(ctx, planner)
				require.ErrorIs(t, publishErr, txsender.ErrLimit)
				require.False(t, blocked.Attempted, "a cap below the base fee must stop before POST")
				require.Equal(t, 1, signer.signs, "the ceiling must not cause repeated signing")
				// A stopped service can be restarted with a higher ceiling. Keep the
				// same signer and chain; only the in-memory sender is reconstructed.
				policy.MaxFee = nil
				sender = feeAnvilSender(t, backend, signer, policy)
			}
			replacement, err := sender.PublishNext(ctx, planner)
			require.NoError(t, err)
			require.Equal(t, txsender.Accepted, replacement.Class)
			require.NotEqual(t, first.Hash, replacement.Hash)
			tx, pending, err := backend.Client.TransactionByHash(ctx, replacement.Hash)
			require.NoError(t, err)
			require.True(t, pending)
			require.Equal(t, original.Nonce(), tx.Nonce())
			require.Equal(t, original.To(), tx.To())
			require.Equal(t, original.Value(), tx.Value())
			require.Equal(t, original.Data(), tx.Data())
			require.Greater(t, tx.GasFeeCap().Cmp(original.GasFeeCap()), 0)
			require.GreaterOrEqual(t, tx.GasTipCap().Cmp(original.GasTipCap()), 0)
			require.Greater(t, tx.GasFeeCap().Cmp(spike), 0)
			from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(31337)), tx)
			require.NoError(t, err)
			require.Equal(t, signer.From(), from)
			signs := signer.signs
			repeated, err := sender.PublishNext(ctx, planner)
			require.NoError(t, err)
			require.Equal(t, replacement.Hash, repeated.Hash)
			require.Equal(t, signs, signer.signs, "ordinary polling must not keep raising an adequate bid")

			feeAnvilMine(ctx, t, spike)
			receipt, err := backend.Client.TransactionReceipt(ctx, replacement.Hash)
			require.NoError(t, err)
			require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status)
			_, err = backend.Client.TransactionReceipt(ctx, first.Hash)
			require.ErrorIs(t, err, ethereum.NotFound, "only the replacement can be included at this nonce")
			stored, err := backend.Client.StorageAt(ctx, target, common.Hash{}, nil)
			require.NoError(t, err)
			require.Equal(t, big.NewInt(1), new(big.Int).SetBytes(stored), "the contract call executes exactly once")
			planner.done = true
			retired, err := sender.PublishNext(ctx, planner)
			require.NoError(t, err)
			require.Nil(t, retired.Current)
			require.False(t, retired.Attempted)
			require.Equal(t, original.Nonce()+1, retired.Nonce)
			t.Logf("original=%s replacement=%s nonce=%d base_fee=%s old_cap=%s new_cap=%s new_tip=%s",
				first.Hash, replacement.Hash, tx.Nonce(), spike, original.GasFeeCap(), tx.GasFeeCap(), tx.GasTipCap())
		})
	}
}

func testAnvilPendingFeeReplacement(ctx context.Context, t *testing.T, backend txsender.ProtectedClient) {
	feeAnvilMine(ctx, t, big.NewInt(1_000_000_000))
	signer := newFeeSigner(t)
	require.NoError(t, anvilRPC(ctx, "anvil_setBalance", signer.From(), "0x56bc75e2d63100000"))
	target := crypto.CreateAddress(signer.From(), 0)
	require.NoError(t, anvilRPC(ctx, "anvil_setCode", target, "0x60005460010160005500"))
	header, err := backend.Client.HeaderByNumber(ctx, nil)
	require.NoError(t, err)
	tip, err := backend.Tip(ctx)
	require.NoError(t, err)
	oldTip := new(big.Int).Div(tip, big.NewInt(2))
	oldCap := new(big.Int).Add(new(big.Int).Mul(header.BaseFee, big.NewInt(2)), oldTip)
	old, err := signer.opts.Signer(signer.From(), types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(31337), Nonce: 0, To: &target, Gas: 200_000,
		GasFeeCap: oldCap, GasTipCap: oldTip,
	}))
	require.NoError(t, err)
	// The previous process signed this bid before two normal 12.5% fee rises.
	// Deliver it late so Anvil cannot mine it while we prepare the new head.
	baseFee := new(big.Int).Set(header.BaseFee)
	for range 2 {
		baseFee.Mul(baseFee, big.NewInt(9)).Div(baseFee, big.NewInt(8))
		feeAnvilMine(ctx, t, baseFee)
	}
	raw, err := old.MarshalBinary()
	require.NoError(t, err)
	_, err = backend.Publish(ctx, raw)
	require.NoError(t, err)
	_, pending, err := backend.Client.TransactionByHash(ctx, old.Hash())
	require.NoError(t, err)
	require.True(t, pending)

	// A restarted sender has no local bid history. An operator ceiling below
	// the pending cap cannot replace it. Record the real pool's refusals, then
	// restart with a sufficient ceiling without removing the pending bid.
	recorded := &feeAnvilPublications{ProtectedClient: backend}
	ceiling := new(big.Int).Sub(oldCap, big.NewInt(1))
	sender := feeAnvilSender(t, recorded, signer, txsender.Policy{MaxFee: ceiling})
	planner := &feePlanner{}
	planner.request = txsender.Request{To: target, Value: new(big.Int)}
	refused, err := sender.PublishNext(ctx, planner)
	require.Error(t, err)
	require.Equal(t, txsender.ReplacementUnderpriced, refused.Class)
	require.Equal(t, []txsender.ReplyClass{txsender.ReplacementUnderpriced, txsender.ReplacementUnderpriced}, recorded.replies)
	_, pending, err = backend.Client.TransactionByHash(ctx, old.Hash())
	require.NoError(t, err)
	require.True(t, pending, "both refusals must leave the old bid in the pool")
	sender = feeAnvilSender(t, recorded, signer, txsender.Policy{})
	report, err := sender.PublishNext(ctx, planner)
	require.NoError(t, err)
	require.Equal(t, []txsender.ReplyClass{
		txsender.ReplacementUnderpriced, txsender.ReplacementUnderpriced, txsender.Accepted,
	}, recorded.replies)
	require.Equal(t, txsender.Accepted, report.Class)
	require.NotEqual(t, old.Hash(), report.Hash)
	tx, pending, err := backend.Client.TransactionByHash(ctx, report.Hash)
	require.NoError(t, err)
	require.True(t, pending)
	require.Equal(t, old.Nonce(), tx.Nonce())
	require.Equal(t, old.To(), tx.To())
	require.True(t, bytes.Equal(old.Data(), tx.Data()), "replacement must keep the call data")
	require.Equal(t, old.Value(), tx.Value())
	require.Greater(t, tx.GasFeeCap().Cmp(old.GasFeeCap()), 0)
	require.Greater(t, tx.GasTipCap().Cmp(old.GasTipCap()), 0)
	_, err = backend.Client.TransactionReceipt(ctx, old.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	feeAnvilMine(ctx, t, baseFee)
	receipt, err := backend.Client.TransactionReceipt(ctx, report.Hash)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status)
	_, err = backend.Client.TransactionReceipt(ctx, old.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	stored, err := backend.Client.StorageAt(ctx, target, common.Hash{}, nil)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(1), new(big.Int).SetBytes(stored), "only the replacement executes")
	t.Logf("old=%s replacement=%s base_fee=%s old_cap=%s old_tip=%s new_cap=%s new_tip=%s replies=%v",
		old.Hash(), report.Hash, baseFee, oldCap, oldTip, tx.GasFeeCap(), tx.GasTipCap(), recorded.replies)
}

type feeAnvilPublications struct {
	txsender.ProtectedClient
	replies []txsender.ReplyClass
}

func (b *feeAnvilPublications) Publish(ctx context.Context, raw []byte) (common.Hash, error) {
	hash, err := b.ProtectedClient.Publish(ctx, raw)
	b.replies = append(b.replies, txsender.ClassifyPublication(err))
	return hash, err
}

func feeAnvilSender(t *testing.T, backend txsender.Backend, signer *feeSigner, policy txsender.Policy) *txsender.Sender[int] {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sender, err := txsender.New[int](backend, signer, 31337, policy, func(ctx context.Context) error { return ctx.Err() }, logger)
	require.NoError(t, err)
	return sender
}

func feeAnvilMine(ctx context.Context, t *testing.T, baseFee *big.Int) {
	t.Helper()
	require.NoError(t, anvilRPC(ctx, "anvil_setNextBlockBaseFeePerGas", hexutil.EncodeBig(baseFee)))
	require.NoError(t, anvilMine(ctx, 1))
}

// Stop only the test-managed node. Reuse the configured Anvil and restore its
// mining settings and base fee before restarting the same node and database.
func prepareFeeAnvil(t *testing.T) (context.Context, txsender.ProtectedClient) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	client := newIntegrationEthClient(ctx, t)
	t.Cleanup(client.Close)
	chainID, err := client.ChainID(ctx)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(31337), chainID)
	stopSharedNode(t)
	t.Cleanup(func() {
		startSharedNode(t)
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		before, err := client.BlockNumber(cleanupCtx)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			after, readErr := client.BlockNumber(cleanupCtx)
			return readErr == nil && after > before
		}, 5*time.Second, 100*time.Millisecond, "normal block production must resume after fee tests")
	})
	header, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, header.BaseFee)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		// Attempt every restoration even if an earlier RPC fails.
		if err := anvilRPC(cleanupCtx, "anvil_setNextBlockBaseFeePerGas", hexutil.EncodeBig(header.BaseFee)); err != nil {
			t.Errorf("restore Anvil base fee: %v", err)
		}
		if err := anvilMine(cleanupCtx, 1); err != nil {
			t.Errorf("mine restored Anvil base fee: %v", err)
		}
		restored, err := client.HeaderByNumber(cleanupCtx, nil)
		if err != nil {
			t.Errorf("read restored Anvil base fee: %v", err)
		} else if restored.BaseFee == nil || restored.BaseFee.Cmp(header.BaseFee) != 0 {
			t.Errorf("Anvil base fee was not restored: got %v, want %s", restored.BaseFee, header.BaseFee)
		}
		// Restore the same automine/one-second interval profile as the other
		// integration fixtures. Anvil's mode getters do not expose mixed mode.
		if err := anvilRPC(cleanupCtx, "evm_setAutomine", true); err != nil {
			t.Errorf("restore Anvil automine: %v", err)
		}
		if err := anvilRPC(cleanupCtx, "evm_setIntervalMining", 1); err != nil {
			t.Errorf("restore Anvil interval mining: %v", err)
		}
	})
	setAnvilIntervalMining(ctx, t, 0)
	setAnvilAutomine(ctx, t, false)
	return ctx, txsender.ProtectedClient{Client: client}
}

type feePlanner struct {
	request txsender.Request
	done    bool
}

func (p *feePlanner) Next(context.Context, txsender.BlockRef, *txsender.Current[int]) (txsender.Decision[int], error) {
	if p.done {
		return txsender.Wait[int]("fixture action completed"), nil
	}
	return txsender.Publish(1, p.request), nil
}

func (*feePlanner) Recheck(context.Context, txsender.BlockRef, int) (txsender.Eligibility, error) {
	return txsender.Useful, nil
}

type feeSigner struct {
	ethutil.TransactOptsFactory
	opts  *bind.TransactOpts
	signs int
}

func newFeeSigner(t *testing.T) *feeSigner {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	opts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(31337))
	require.NoError(t, err)
	signer := &feeSigner{opts: opts}
	sign := opts.Signer
	opts.Signer = func(address common.Address, tx *types.Transaction) (*types.Transaction, error) {
		signer.signs++
		return sign(address, tx)
	}
	signer.TransactOptsFactory = ethutil.NewStaticTransactOptsFactory(opts)
	return signer
}
