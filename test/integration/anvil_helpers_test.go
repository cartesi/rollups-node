// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

// maxBlocksToMine is the sanity cap for mineForTournamentTimeout to prevent
// hanging if the tournament contract reports an unreasonably large allowance.
const maxBlocksToMine = 10_000

// Anvil devnet RPC helpers.

var anvilHTTPClient = &http.Client{Timeout: 30 * time.Second}

// anvilRPCCall calls an Anvil JSON-RPC method and returns the raw result.
func anvilRPCCall(ctx context.Context, method string, params ...any) (json.RawMessage, error) {
	endpoint := envOrDefault(
		"CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")

	rpcReq := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
		ID      int    `json:"id"`
	}{JSONRPC: "2.0", Method: method, Params: params, ID: 1}
	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := anvilHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", method, resp.StatusCode)
	}

	// Parse JSON-RPC response and check for error.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", method, err)
	}
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("%s: parse response: %w", method, err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("%s: JSON-RPC error %d: %s",
			method, rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

// anvilRPC calls an Anvil JSON-RPC method, discarding the result.
func anvilRPC(ctx context.Context, method string, params ...any) error {
	_, err := anvilRPCCall(ctx, method, params...)
	return err
}

// anvilSetBalance sets the ETH balance for an address on the Anvil devnet.
func anvilSetBalance(ctx context.Context, address string, weiHex string) error {
	return anvilRPC(ctx, "anvil_setBalance", address, weiHex)
}

// anvilMine mines the specified number of blocks on the Anvil devnet.
func anvilMine(ctx context.Context, numBlocks int) error {
	return anvilRPC(ctx, "anvil_mine", fmt.Sprintf("0x%x", numBlocks))
}

// mineForTournamentTimeout queries the tournament contract to determine
// the timeout block (startInstant + allowance) and mines enough blocks
// to reach it. Returns the number of blocks mined.
func mineForTournamentTimeout(
	ctx context.Context,
	client *ethclient.Client,
	tournamentAddr common.Address,
) (int, error) {
	tournament, err := itournament.NewITournament(tournamentAddr, client)
	if err != nil {
		return 0, fmt.Errorf("bind tournament: %w", err)
	}

	args, err := tournament.TournamentArguments(&bind.CallOpts{Context: ctx})
	if err != nil {
		return 0, fmt.Errorf("tournament arguments: %w", err)
	}

	currentBlock, err := client.BlockNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("block number: %w", err)
	}

	finishBlock := args.StartInstant + args.Allowance
	if finishBlock < args.StartInstant { // uint64 overflow
		return 0, fmt.Errorf("tournament timeout overflows: start=%d allowance=%d",
			args.StartInstant, args.Allowance)
	}
	if currentBlock >= finishBlock {
		return 0, nil // Already past the timeout.
	}

	gap := finishBlock - currentBlock + 1
	if gap > maxBlocksToMine {
		return 0, fmt.Errorf(
			"tournament needs %d blocks but cap is %d", gap, maxBlocksToMine)
	}
	blocksNeeded := int(gap)
	if err := anvilMine(ctx, blocksNeeded); err != nil {
		return 0, err
	}
	return blocksNeeded, nil
}

// PRT tournament helpers.

// waitForTournamentAndCommitment polls until a root tournament and a commitment
// exist for the given epoch index. Returns the tournament.
func waitForTournamentAndCommitment(
	ctx context.Context,
	t testing.TB,
	require *require.Assertions,
	appName string,
	epochIndex uint64,
) *model.Tournament {
	t.Helper()
	tctx, cancel := context.WithTimeout(ctx, claimAcceptedTimeout)
	defer cancel()

	var tournament *model.Tournament
	var lastErr error
	err := pollUntil(tctx, 5*time.Second, func() (bool, error) {
		resp, err := readTournaments(tctx, appName)
		if err != nil {
			if isCLIExitError(err) {
				lastErr = err
				t.Logf("    epoch %d: poll tournaments: %v (retrying)", epochIndex, err)
				return false, nil
			}
			return false, fmt.Errorf("poll tournaments: %w", err)
		}
		tournament = findRootTournament(resp.Data, epochIndex)
		return tournament != nil, nil
	})
	if err != nil && lastErr != nil {
		err = fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	require.NoError(err, "wait for epoch %d tournament", epochIndex)
	t.Logf("    epoch %d: root tournament created at %s", epochIndex, tournament.Address)

	lastErr = nil
	err = pollUntil(tctx, 5*time.Second, func() (bool, error) {
		resp, err := readCommitments(tctx, appName)
		if err != nil {
			if isCLIExitError(err) {
				lastErr = err
				t.Logf("    epoch %d: poll commitments: %v (retrying)", epochIndex, err)
				return false, nil
			}
			return false, fmt.Errorf("poll commitments: %w", err)
		}
		return findCommitmentForEpoch(resp.Data, epochIndex) != nil, nil
	})
	if err != nil && lastErr != nil {
		err = fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	require.NoError(err, "wait for epoch %d commitment", epochIndex)
	t.Logf("    epoch %d: commitment joined to tournament", epochIndex)

	return tournament
}

// waitForTournamentWinner polls until the root tournament for the given epoch
// has a winner commitment.
func waitForTournamentWinner(
	ctx context.Context,
	t testing.TB,
	require *require.Assertions,
	appName string,
	epochIndex uint64,
) {
	t.Helper()
	tctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var lastErr error
	err := pollUntil(tctx, 5*time.Second, func() (bool, error) {
		resp, err := readTournaments(tctx, appName)
		if err != nil {
			if isCLIExitError(err) {
				lastErr = err
				t.Logf("    epoch %d: poll winner: %v (retrying)", epochIndex, err)
				return false, nil
			}
			return false, fmt.Errorf("poll tournament winner: %w", err)
		}
		tournament := findRootTournament(resp.Data, epochIndex)
		return tournament != nil && tournament.WinnerCommitment != nil, nil
	})
	if err != nil && lastErr != nil {
		err = fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	require.NoError(err, "wait for epoch %d tournament winner", epochIndex)
}

// settleTournament runs the full tournament cycle for a given epoch:
// wait for tournament+commitment, mine past the timeout, wait for winner.
func settleTournament(
	ctx context.Context,
	t testing.TB,
	require *require.Assertions,
	client *ethclient.Client,
	appName string,
	epochIndex uint64,
) {
	t.Helper()
	defer timed(t, fmt.Sprintf("settle tournament epoch %d", epochIndex))()

	t.Logf("Waiting for PRT to create a root tournament and join with a commitment for epoch %d...",
		epochIndex)
	tournament := waitForTournamentAndCommitment(ctx, t, require, appName, epochIndex)

	block, err := client.BlockNumber(ctx)
	require.NoError(err, "get block number")
	t.Logf("Mining blocks to advance past the epoch %d tournament timeout (current block=%d)...",
		epochIndex, block)
	blocksMined, err := mineForTournamentTimeout(ctx, client, tournament.Address)
	require.NoError(err, "mine for epoch %d tournament timeout", epochIndex)
	t.Logf("    mined %d blocks to reach timeout", blocksMined)

	t.Logf("Waiting for the PRT service to settle epoch %d (uncontested single-commitment win)...",
		epochIndex)
	waitForTournamentWinner(ctx, t, require, appName, epochIndex)
	t.Logf("    epoch %d tournament settled — winner declared", epochIndex)
}

// findRootTournament returns the root tournament for the given epoch index,
// or nil if not found. Root tournaments have no parent.
func findRootTournament(tournaments []model.Tournament, epochIndex uint64) *model.Tournament {
	for i, t := range tournaments {
		if t.EpochIndex == epochIndex && t.ParentTournamentAddress == nil {
			return &tournaments[i]
		}
	}
	return nil
}

// findCommitmentForEpoch returns the first commitment matching the epoch index,
// or nil if not found.
func findCommitmentForEpoch(commitments []model.Commitment, epochIndex uint64) *model.Commitment {
	for i, c := range commitments {
		if c.EpochIndex == epochIndex {
			return &commitments[i]
		}
	}
	return nil
}
