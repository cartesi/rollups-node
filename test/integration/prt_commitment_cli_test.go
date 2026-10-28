// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

// These protocol constants come from the emulator's rollup-defines.h.
// The period and height below come from the deployed root descriptor.
const (
	cliOracleInputBits      = uint64(24)
	cliOracleMCycleBits     = uint64(48)
	cliOracleUarchCycleBits = uint64(20)
	cliOracleRootHeight     = uint64(54)
)

type cliEpochRun struct {
	root            common.Hash
	storeDir        string
	lastOutputProof string
}

// The CLI shares the emulator collector with the node. Its unbundled tree,
// padding, chain inputs, epoch boundaries, and persistence provide the oracle.
// These fixtures do not cover a terminal ending or a full input span.
func TestPrtCommitmentMatchesMachineCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Minute)
	defer cancel()
	client := newIntegrationEthClient(ctx, t)
	defer client.Close()
	var logs LogChecker
	logs.StartLogCapture()
	logs.SetExpectedLogs(t, prtBlockOutOfRangeAllowlist)
	t.Cleanup(func() { logs.CheckLogs(t) })

	t.Run("echo_empty_and_second_epoch", func(t *testing.T) {
		path := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
		name := deployCLIOracleApplication(ctx, t, path)
		assertPrtCommitmentCLI(ctx, t, client, name, 0, path, "")
		for index, payload := range []string{"echo", strings.Repeat("a", 32), strings.Repeat("b", 65_000)} {
			input := sendCLIOracleInput(ctx, t, name, payload)
			require.Equal(t, uint64(index), input.Index)
			require.Equal(t, uint64(1), input.EpochIndex)
			require.Equal(t, model.InputCompletionStatus_Accepted, input.Status)
		}
		finalizePrtEpoch(ctx, t, require.New(t), client, name, 0)
		first := assertPrtCommitmentCLI(ctx, t, client, name, 1, path, "")
		assertCLIOracleMultipleSamples(ctx, t, name, 1, 2)
		require.NotEmpty(t, first.lastOutputProof, "the next epoch needs the cumulative outputs proof")
		input := sendCLIOracleInput(ctx, t, name, "next-epoch")
		require.Equal(t, uint64(3), input.Index)
		require.Equal(t, uint64(2), input.EpochIndex)
		require.Equal(t, model.InputCompletionStatus_Accepted, input.Status)
		finalizePrtEpoch(ctx, t, require.New(t), client, name, 1)
		assertPrtCommitmentCLI(ctx, t, client, name, 2, first.storeDir, first.lastOutputProof)
	})

	t.Run("rejected_second_input", func(t *testing.T) {
		path := envOrDefault("CARTESI_TEST_REJECT_DAPP_PATH", "applications/reject-loop-dapp")
		name := deployCLIOracleApplication(ctx, t, path)
		for index := range 3 {
			input := sendCLIOracleInput(ctx, t, name, fmt.Sprintf("reject-%d", index))
			require.Equal(t, uint64(index), input.Index)
			require.Equal(t, uint64(1), input.EpochIndex)
			status := model.InputCompletionStatus_Accepted
			if index == 1 {
				status = model.InputCompletionStatus_Rejected
			}
			require.Equal(t, status, input.Status)
		}
		finalizePrtEpoch(ctx, t, require.New(t), client, name, 0)
		assertPrtCommitmentCLI(ctx, t, client, name, 1, path, "")
	})
}

func deployCLIOracleApplication(ctx context.Context, t testing.TB, path string) string {
	t.Helper()
	name := uniqueAppName("cli-oracle")
	address, err := deployApplication(ctx, name, path, "--salt", uniqueSalt(), prtFlag)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		require.NoError(t, disableApplication(cleanupCtx, name))
	})
	require.NoError(t, anvilSetBalance(ctx, address, oneEtherWei))
	return name
}

func sendCLIOracleInput(ctx context.Context, t testing.TB, name, payload string) *model.Input {
	t.Helper()
	index, _, err := sendInput(ctx, name, payload)
	require.NoError(t, err)
	processCtx, cancel := context.WithTimeout(ctx, inputProcessingTimeout)
	defer cancel()
	input, err := waitForInputProcessed(processCtx, t, name, index)
	require.NoError(t, err)
	return input
}

func assertReloadedPrtCommitmentCLI(ctx context.Context, t testing.TB, client *ethclient.Client, name string) {
	t.Helper()
	first, err := readInput(ctx, name, 0)
	require.NoError(t, err)
	second, err := readInput(ctx, name, 1)
	require.NoError(t, err)
	require.Equal(t, first.EpochIndex, second.EpochIndex, "restart must not split the two inputs across epochs")
	require.Equal(t, uint64(1), first.EpochIndex)
	finalizePrtEpoch(ctx, t, require.New(t), client, name, 0)
	path := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	assertPrtCommitmentCLI(ctx, t, client, name, first.EpochIndex, path, "")
	finalizePrtEpoch(ctx, t, require.New(t), client, name, first.EpochIndex)
}

func assertPrtCommitmentCLI(
	ctx context.Context, t testing.TB, client *ethclient.Client,
	name string, epochIndex uint64, loadPath, lastOutputProof string,
) cliEpochRun {
	t.Helper()
	waitForTournamentAndCommitment(ctx, t, require.New(t), name, epochIndex)
	epoch, err := readEpoch(ctx, name, epochIndex)
	require.NoError(t, err)
	require.NotNil(t, epoch.Commitment)
	require.NotNil(t, epoch.TournamentAddress)
	dsn, err := config.GetDatabaseConnection()
	require.NoError(t, err)
	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	require.NoError(t, err)
	defer repo.Close()
	app, err := repo.GetApplication(ctx, name)
	require.NoError(t, err)
	consensus, err := idaveconsensus.NewIDaveConsensus(app.IConsensusAddress, client)
	require.NoError(t, err)
	head, err := client.BlockNumber(ctx)
	require.NoError(t, err)
	opts := &bind.FilterOpts{Start: 0, End: &head, Context: ctx}
	seals, err := consensus.FilterEpochSealed(opts, []*big.Int{new(big.Int).SetUint64(epochIndex)})
	require.NoError(t, err)
	defer seals.Close()
	require.True(t, seals.Next(), "the chain must seal the epoch")
	seal := seals.Event
	require.False(t, seals.Next(), "the epoch must have exactly one seal")
	require.NoError(t, seals.Error())
	require.True(t, seal.InputIndexLowerBound.IsUint64())
	require.True(t, seal.InputIndexUpperBound.IsUint64())
	lower, upper := seal.InputIndexLowerBound.Uint64(), seal.InputIndexUpperBound.Uint64()
	require.LessOrEqual(t, lower, upper)
	require.Equal(t, lower, epoch.InputIndexLowerBound)
	require.Equal(t, upper, epoch.InputIndexUpperBound)
	require.Equal(t, seal.Tournament, *epoch.TournamentAddress)
	callOpts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(head)}
	tournament, err := itournament.NewITournament(seal.Tournament, client)
	require.NoError(t, err)
	descriptor, err := tournament.TournamentDescriptor(callOpts)
	require.NoError(t, err)
	require.Zero(t, descriptor.Level)
	require.Equal(t, seal.InitialMachineStateHash, descriptor.InitialHash)
	require.GreaterOrEqual(t, descriptor.Log2Stride, cliOracleUarchCycleBits)
	period := descriptor.Log2Stride - cliOracleUarchCycleBits
	require.LessOrEqual(t, period, cliOracleMCycleBits)
	require.Equal(t, cliOracleInputBits+cliOracleMCycleBits-period, descriptor.Height)
	inputBoxAddress, err := consensus.GetInputBox(callOpts)
	require.NoError(t, err)
	inputBox, err := iinputbox.NewIInputBox(inputBoxAddress, client)
	require.NoError(t, err)
	inputs := make([][]byte, 0, upper-lower)
	if lower != upper {
		indices := make([]*big.Int, 0, upper-lower)
		for index := lower; index < upper; index++ {
			indices = append(indices, new(big.Int).SetUint64(index))
		}
		events, err := inputBox.FilterInputAdded(opts, []common.Address{app.IApplicationAddress}, indices)
		require.NoError(t, err)
		defer events.Close()
		for events.Next() {
			event := events.Event
			require.True(t, event.Index.IsUint64(), "chain input index must fit uint64")
			require.Equal(t, lower+uint64(len(inputs)), event.Index.Uint64(),
				"chain inputs must be complete and ordered")
			input, err := readInput(ctx, name, event.Index.Uint64())
			require.NoError(t, err)
			require.Equal(t, epochIndex, input.EpochIndex)
			require.Equal(t, event.Input, input.RawData, "node ingestion must preserve the exact chain input")
			inputs = append(inputs, event.Input)
		}
		require.NoError(t, events.Error())
	}
	require.Equal(t, upper-lower, uint64(len(inputs)))
	if epochIndex == 0 {
		require.Zero(t, lower)
		require.Zero(t, upper, "deployment must seal an empty epoch zero")
	} else {
		require.NotEmpty(t, inputs, "the nonempty epoch comparison must exercise execution")
	}
	run := runEpochCommitmentCLI(ctx, t, loadPath, lastOutputProof, period, inputs, common.Hash(descriptor.InitialHash))
	require.Equal(t, *epoch.Commitment, run.root, "unbundled CLI root must match the node commitment")
	if epochIndex == 0 {
		require.Equal(t, cliOracleRootHeight, descriptor.Height)
		repeated := common.Hash(descriptor.InitialHash)
		for range cliOracleRootHeight {
			repeated = crypto.Keccak256Hash(repeated[:], repeated[:])
		}
		require.Equal(t, repeated, run.root, "empty epoch root must independently equal 54 Keccak iterations")
	}
	joined, err := tournament.FilterCommitmentJoined(opts, nil, nil)
	require.NoError(t, err)
	defer joined.Close()
	require.True(t, joined.Next(), "node must publish CommitmentJoined")
	event := joined.Event
	require.False(t, joined.Next(), "this fixture has only the node as a tournament participant")
	require.NoError(t, joined.Error())
	require.Equal(t, run.root, common.Hash(event.Commitment), "CLI root must also match the on-chain commitment")
	commitments, err := readCommitments(ctx, name)
	require.NoError(t, err)
	stored := findCommitmentForEpoch(commitments.Data, epochIndex)
	require.NotNil(t, stored)
	require.Equal(t, seal.Tournament, stored.TournamentAddress)
	require.Equal(t, event.Submitter, stored.SubmitterAddress)
	require.Equal(t, event.Raw.TxHash, stored.TxHash)
	require.Equal(t, uint64(event.Raw.Index), stored.LogIndex)
	require.Equal(t, run.root, stored.Commitment)
	return run
}

func runEpochCommitmentCLI(
	ctx context.Context, t testing.TB, loadPath, lastOutputProof string,
	period uint64, inputs [][]byte, initialHash common.Hash,
) cliEpochRun {
	t.Helper()
	dir := t.TempDir()
	loadPath, err := filepath.Abs(loadPath)
	require.NoError(t, err)
	for index, input := range inputs {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("input-%d.bin", index)), input, 0o600))
	}
	advance := []string{"input_file_index_begin:0", "input_file_index_end:" + strconv.Itoa(len(inputs)),
		"format:json", "output_proof:output-%o-proof.json"}
	if len(inputs) != 0 {
		advance = append(advance, "input:"+filepath.Join(dir, "input-%i.bin"))
	}
	if lastOutputProof != "" {
		advance = append(advance, "last_output_proof:"+lastOutputProof)
	}
	rootFile, initialFile := filepath.Join(dir, "root.bin"), filepath.Join(dir, "initial-hash.bin")
	storeDir := filepath.Join(dir, "stored-machine")
	args := []string{"--remote-spawn", "--remote-shutdown", "--load=" + loadPath,
		"--store=" + storeDir, "--initial-hash=" + initialFile,
		"--cmio-advance-state=" + strings.Join(advance, ","),
		"--mcycle-computation-hash=filename:" + rootFile + ",log2_mcycle_period:" + strconv.FormatUint(period, 10)}
	version, err := exec.CommandContext(ctx, "cartesi-machine", "--version").CombinedOutput()
	require.NoError(t, err, "%s", version)
	pristine, err := emulator.GetUarchPristineHash()
	require.NoError(t, err)
	t.Logf("CLI version: %s; library version: %#x; pristine hash: %s", strings.TrimSpace(string(version)),
		emulator.GetVersion(), common.Hash(pristine).Hex())
	t.Logf("Unbundled CLI command: cartesi-machine %s", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "cartesi-machine", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	t.Logf("CLI output: %s", output)
	require.NoError(t, err)
	readHash := func(filename string) common.Hash {
		data, err := os.ReadFile(filename)
		require.NoError(t, err)
		require.Len(t, data, common.HashLength)
		return common.BytesToHash(data)
	}
	require.Equal(t, initialHash, readHash(initialFile), "CLI must start from the chain descriptor's state")
	result := cliEpochRun{root: readHash(rootFile), storeDir: storeDir}
	proofs, err := filepath.Glob(filepath.Join(dir, "output-*-proof.json"))
	require.NoError(t, err)
	var highest uint64
	for _, path := range proofs {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "output-"), "-proof.json")
		index, err := strconv.ParseUint(name, 10, 64)
		require.NoError(t, err)
		if result.lastOutputProof == "" || index > highest {
			highest, result.lastOutputProof = index, path
		}
	}
	return result
}

// Rows are inspected only for fixture coverage, never to calculate a root.
func assertCLIOracleMultipleSamples(ctx context.Context, t testing.TB, name string, epoch, input uint64) {
	t.Helper()
	require.Equal(t, uint64(6), model.Log2StateHashesPerEntry, "the node must bundle 64 CLI samples per stored entry")
	dsn, err := config.GetDatabaseConnection()
	require.NoError(t, err)
	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	require.NoError(t, err)
	defer repo.Close()
	app, err := repo.GetApplication(ctx, name)
	require.NoError(t, err)
	var cursor *uint64
	var sampled uint64
	const pageSize = 1000
	for {
		rows, err := repo.ListEpochStateHashes(ctx, app.ID, epoch, cursor, pageSize)
		require.NoError(t, err)
		for _, row := range rows {
			if row.InputIndex == input && row.Repetitions == 1 {
				sampled++
			}
		}
		if len(rows) < pageSize {
			break
		}
		last := rows[len(rows)-1].Index
		cursor = &last
	}
	require.GreaterOrEqual(t, sampled, uint64(2), "the large input must exercise multiple periodic state-hash rows")
}
