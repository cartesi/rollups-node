// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package txsender

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

// The fixture uses a fresh random funded account. It does not reset, snapshot,
// revert, or borrow any configured node or CLI nonce sequence.
func TestProviderAnvilProtectedPublicationAndLatestConsumption(t *testing.T) {
	if os.Getenv("CARTESI_TXSENDER_ANVIL_TEST") != "1" {
		t.Skip("CARTESI_TXSENDER_ANVIL_TEST=1 is required to fund and publish on the declared Anvil fixture")
	}
	endpoint := os.Getenv("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT")
	if endpoint == "" {
		t.Skip("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT is required for the Anvil provider fixture")
	}
	client, err := ethutil.DialEthClient(t.Context(), endpoint)
	require.NoError(t, err)
	defer client.Close()
	chainID, err := client.ChainID(t.Context())
	require.NoError(t, err)
	require.Equal(t, big.NewInt(31337), chainID, "this funding fixture is for the declared Anvil profile")
	signer := newFakeSigner(t)
	var funded any
	err = client.Client().CallContext(t.Context(), &funded, "anvil_setBalance",
		signer.From(), hexutil.EncodeBig(big.NewInt(1_000_000_000_000_000_000)))
	require.NoError(t, err)
	backend := ProtectedClient{Client: client}
	s, err := New[int](backend, signer, 31337, Policy{MaxFee: big.NewInt(100_000_000_000)},
		func(ctx context.Context) error { return ctx.Err() }, testLogger())
	require.NoError(t, err)
	planner := newPlanner()
	planner.request = Request{To: signer.From(), Value: big.NewInt(1), GasLimit: 21_000}
	observation, err := s.PublishNext(t.Context(), planner)
	require.NoError(t, err)
	prepared := s.current
	require.True(t, observation.Attempted)
	require.Equal(t, Accepted, observation.Class)
	var tx *types.Transaction
	require.Eventually(t, func() bool {
		readCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		var pending bool
		var readErr error
		tx, pending, readErr = client.TransactionByHash(readCtx, observation.Hash)
		return readErr == nil && !pending
	}, 10*time.Second, time.Second, "the existing devnet producer must include the fixture's transaction")
	require.Equal(t, prepared.gas, tx.Gas())
	require.Equal(t, prepared.cap, tx.GasFeeCap())
	require.Equal(t, prepared.tip, tx.GasTipCap())
	// Retirement depends on latest consumption even while selected observers lag.
	planner.decision = waitDecision
	require.Eventually(t, func() bool {
		readCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		report, readErr := s.PublishNext(readCtx, planner)
		return readErr == nil && report.Current == nil && report.Nonce > observation.Nonce
	}, 10*time.Second, time.Second, "latest mined nonce must retire the publication cache")
	_, err = backend.Call(t.Context(), ethereum.CallMsg{
		From: signer.From(), To: &planner.request.To, Gas: prepared.gas, Value: planner.request.Value,
		GasFeeCap: prepared.cap, GasTipCap: prepared.tip,
	}, observation.Block)
	require.NoError(t, err, "a historical exact execution must honor requireCanonical hash selectors")
	t.Run("gas estimate stays on preparation block after inclusion", func(t *testing.T) {
		testProtectedGasEstimateAfterInclusion(t, backend, signer)
	})
}

func testProtectedGasEstimateAfterInclusion(t *testing.T, backend ProtectedClient, signer *fakeSigner) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	opts := *signer.opts
	opts.Context = ctx
	// The first call initializes storage; a repeated call reverts with
	// ClockAlreadyInitialized(), reproducing a completed tournament join.
	init := hexutil.MustDecode("0x6018600c60003960186000f35f5415601257630ef476875f526004601cfd5b60015f5500")
	target, deployment, contract, err := bind.DeployContract(&opts, abi.ABI{}, init, backend.Client)
	require.NoError(t, err)
	receipt, err := bind.WaitMined(ctx, backend.Client, deployment)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status)
	header, err := backend.Header(ctx, latestBlockTag)
	require.NoError(t, err)
	prepared := BlockRef{Number: header.Number.Uint64(), Hash: header.Hash()}
	msg := ethereum.CallMsg{From: opts.From, To: &target}
	before, err := backend.Estimate(ctx, msg, prepared)
	require.NoError(t, err)
	tx, err := contract.RawTransact(&opts, nil)
	require.NoError(t, err)
	receipt, err = bind.WaitMined(ctx, backend.Client, tx)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status)
	_, err = backend.Client.EstimateGasAtBlock(ctx, msg, nil)
	require.ErrorContains(t, err, "0x0ef47687", "latest now contains the already completed action")
	pinned, err := backend.Estimate(ctx, msg, prepared)
	require.NoError(t, err, "sizing must use the same canonical block as the prepared action")
	require.Equal(t, before, pinned)
	msg.Gas = pinned
	_, err = backend.Call(ctx, msg, prepared)
	require.NoError(t, err, "the prepared-block exact preflight and estimate must agree")
	t.Logf("prepared_block=%d included_block=%d pinned_estimate=%d latest=ClockAlreadyInitialized",
		prepared.Number, receipt.BlockNumber.Uint64(), pinned)
}
