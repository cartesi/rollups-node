// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/claimer"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthority"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

const (
	independentWatchEpochLength   = uint64(256)
	independentWatchStagingPeriod = uint64(128)
	independentWatchFinalityBound = 128
	independentWatchConsensusFlag = "--consensus"
)

// E has real cumulative outputs and no claim. E+1 is empty and has no row.
// A real proof from E-1 is valid for submission but false for the prefix at E+1.
// Only the independent observer's own diagnostic satisfies this test's oracle.
func TestIndependentClaimWatch(t *testing.T) {
	if !isNodeSelfManaged() {
		t.Skip("requires test-managed execution and validation")
	}
	for _, quorum := range []bool{false, true} {
		name := "authority_reader_empty_successor"
		if quorum {
			name = "quorum_majority_empty_successor"
		}
		t.Run(name, func(t *testing.T) { runIndependentEmptyClaim(t, quorum, false) })
	}
	t.Run("authority_restart_before_validation", func(t *testing.T) { runIndependentEmptyClaim(t, false, true) })
}

func TestIndependentClaimObserverCorrectClaims(t *testing.T) {
	if !isNodeSelfManaged() {
		t.Skip("requires test-managed execution and validation")
	}
	for _, quorum := range []bool{false, true} {
		name := "authority_carry_forward"
		if quorum {
			name = "quorum_carry_forward"
		}
		t.Run(name, func(t *testing.T) { runIndependentCorrectEmptyClaim(t, quorum) })
	}
	t.Run("authority_initial_template", runIndependentInitialTemplateClaim)
}

func runIndependentCorrectEmptyClaim(t *testing.T, quorum bool) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	terminalPrefixMining(ctx, t)
	stopSharedNode(t)
	f := newTerminalPrefixFixture(ctx, t)
	readerConfig := independentWatchReaderConfig(t, f)
	app := deployIndependentWatchApplication(t, f, quorum)
	_, _, err := sendInput(ctx, app.Name, "NODE-1 correct carry-forward with real outputs")
	require.NoError(t, err)
	input := f.waitInput(app.Name, 0, model.InputCompletionStatus_Accepted)
	epoch, err := f.repo.GetEpoch(ctx, app.Name, input.EpochIndex)
	require.NoError(t, err)
	mineIndependentClaimPast(t, f, epoch.LastBlock)
	confirmIndependentWatchBlock(t, f, epoch.LastBlock+1)
	epoch = processedIndependentWatchEpoch(t, f, app.Name, epoch.Index)
	validateIndependentWatchEpochs(t, f, app.Name, epoch.Index)
	epoch, err = f.repo.GetEpoch(ctx, app.Name, epoch.Index)
	require.NoError(t, err)
	output, err := f.repo.GetOutput(ctx, app.Name, 0)
	require.NoError(t, err)
	require.NotNil(t, output, "the source execution must emit a real output")
	require.NotEmpty(t, output.RawData)
	require.NotEqual(t, merkle.CreatePostContext()[merkle.TREE_DEPTH], *epoch.TxBufferDataBlock)
	emptyIndex := epoch.Index + 1
	covered := (emptyIndex+1)*app.EpochLength - 1
	mineIndependentClaimPast(t, f, covered)
	confirmIndependentWatchBlock(t, f, covered)
	_, err = f.reader.Tick(ctx)
	require.NoError(t, err)
	empty, err := f.repo.GetEpoch(ctx, app.Name, emptyIndex)
	require.True(t, err == nil || errors.Is(err, repository.ErrNotFound))
	require.Nil(t, empty, "carry-forward must work without an E+1 row")
	_, staged := stageIndependentWatchClaim(t, f, app, epoch, covered, quorum)
	require.Equal(t, *epoch.MachineHash, common.Hash(staged.MachineMerkleRoot))
	require.Equal(t, *epoch.TxBufferDataBlock, common.Hash(staged.OutputsMerkleRoot))
	confirmIndependentWatchBlock(t, f, staged.Raw.BlockNumber)
	_, err = f.reader.Tick(ctx)
	require.NoError(t, err)
	assertIndependentClaimCompatible(t, f, readerConfig, app, staged)
	empty, err = f.repo.GetEpoch(ctx, app.Name, emptyIndex)
	require.True(t, err == nil || errors.Is(err, repository.ErrNotFound))
	require.Nil(t, empty)
}

func runIndependentInitialTemplateClaim(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	terminalPrefixMining(ctx, t)
	stopSharedNode(t)
	f := newTerminalPrefixFixture(ctx, t)
	readerConfig := independentWatchReaderConfig(t, f)
	app := deployIndependentWatchApplication(t, f, false)
	machineConfig := machine.DefaultConfig(app.TemplateURI)
	machineConfig.ExecutionParameters = app.ExecutionParameters
	runtime, err := machine.Load(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), machineConfig)
	if runtime != nil {
		t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	}
	require.NoError(t, err)
	state, err := runtime.StateProof(ctx)
	require.NoError(t, err)
	require.NoError(t, machine.ValidateAcceptedState(state))
	require.Equal(t, app.TemplateHash, common.Hash(state.MachineHash))
	// The real template proof supplies the claimed outputs root. The constant
	// checks the supported guest's initial convention, not the proof's source.
	require.Equal(t, merkle.CreatePostContext()[merkle.TREE_DEPTH], common.Hash(state.TxBufferProof.DataBlock))
	head, err := f.client.BlockNumber(ctx)
	require.NoError(t, err)
	emptyIndex := head / app.EpochLength
	covered := (emptyIndex+1)*app.EpochLength - 1
	mineIndependentClaimPast(t, f, covered)
	confirmIndependentWatchBlock(t, f, covered)
	_, err = f.reader.Tick(ctx)
	require.NoError(t, err)
	empty, err := f.repo.GetEpoch(ctx, app.Name, emptyIndex)
	require.True(t, err == nil || errors.Is(err, repository.ErrNotFound))
	require.Nil(t, empty, "the initial empty epoch has no local row")
	leaf := func(proof machine.LeafProof) iauthority.LeafProof {
		return iauthority.LeafProof{DataBlock: proof.DataBlock, Siblings: proof.Siblings}
	}
	proof := iauthority.MachineValidityProof{
		IflagsYProof: leaf(state.IflagsYProof), HtifTohostProof: leaf(state.HtifTohostProof), TxBufferProof: leaf(state.TxBufferProof),
	}
	_, staged := stageIndependentWatchProof(t, f, app, common.Hash(state.MachineHash), proof, covered, false)
	require.Equal(t, common.Hash(state.TxBufferProof.DataBlock), common.Hash(staged.OutputsMerkleRoot))
	confirmIndependentWatchBlock(t, f, staged.Raw.BlockNumber)
	_, err = f.reader.Tick(ctx)
	require.NoError(t, err)
	assertIndependentClaimCompatible(t, f, readerConfig, app, staged)
	stored, err := f.repo.GetApplication(ctx, app.Name)
	require.NoError(t, err)
	require.Zero(t, stored.ProcessedInputs, "the accepted proof is the initial template, before any input")
	input, err := f.repo.GetInput(ctx, app.Name, 0)
	require.True(t, err == nil || errors.Is(err, repository.ErrNotFound))
	require.Nil(t, input)
}

func assertIndependentClaimCompatible(t *testing.T, f *terminalPrefixFixture, cfg config.ClaimerConfig,
	app *model.Application, staged *iconsensus.IConsensusClaimStaged,
) {
	t.Helper()
	var captured bytes.Buffer
	observer := createIndependentWatchReader(t, f, cfg, &captured)
	safe, err := f.repo.GetEventLastCheckBlock(f.ctx, app.ID, model.MonitoredEvent_ClaimAccepted)
	require.NoError(t, err)
	require.Less(t, safe, staged.Raw.BlockNumber, "the observer has not checked this stage yet")
	for range 32 {
		_, err = observer.Tick(f.ctx)
		require.NoError(t, err)
		stored, readErr := f.repo.GetApplication(f.ctx, app.Name)
		require.NoError(t, readErr)
		require.Equal(t, model.ApplicationStatus_OK, stored.Status, "a real compatible proof must not cause DIVERGED")
		safe, err = f.repo.GetEventLastCheckBlock(f.ctx, app.ID, model.MonitoredEvent_ClaimAccepted)
		require.NoError(t, err)
		if safe >= staged.Raw.BlockNumber {
			break
		}
	}
	require.GreaterOrEqual(t, safe, staged.Raw.BlockNumber, "the observer must resolve the real staged claim")
	require.NotContains(t, captured.String(), "Divergent on-chain claim observed")
}

func runIndependentEmptyClaim(t *testing.T, quorum, lateValidation bool) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	terminalPrefixMining(ctx, t)
	stopSharedNode(t)
	f := newTerminalPrefixFixture(ctx, t)
	readerConfig := independentWatchReaderConfig(t, f)
	app := deployIndependentWatchApplication(t, f, quorum)
	_, _, err := sendInput(ctx, app.Name, "NODE-1 proof source before E")
	require.NoError(t, err)
	input := f.waitInput(app.Name, 0, model.InputCompletionStatus_Accepted)
	proofSource, err := f.repo.GetEpoch(ctx, app.Name, input.EpochIndex)
	require.NoError(t, err)
	mineIndependentClaimPast(t, f, proofSource.LastBlock)
	confirmIndependentWatchBlock(t, f, proofSource.LastBlock+1)
	proofSource = processedIndependentWatchEpoch(t, f, app.Name, proofSource.Index)
	_, block, err := sendInput(ctx, app.Name, "NODE-1 E changes the cumulative outputs")
	require.NoError(t, err)
	targetIndex := block / app.EpochLength
	require.Equal(t, proofSource.Index+1, targetIndex)
	targetInput := f.waitInput(app.Name, 1, model.InputCompletionStatus_Accepted)
	require.Equal(t, targetIndex, targetInput.EpochIndex)
	target, err := f.repo.GetEpoch(ctx, app.Name, targetIndex)
	require.NoError(t, err)
	mineIndependentClaimPast(t, f, target.LastBlock)
	confirmIndependentWatchBlock(t, f, target.LastBlock+1)
	target = processedIndependentWatchEpoch(t, f, app.Name, targetIndex)
	require.NotEqual(t, proofSource.MachineHash, target.MachineHash)
	require.NotEqual(t, proofSource.TxBufferDataBlock, target.TxBufferDataBlock, "E has additional real outputs")
	emptyIndex := targetIndex + 1
	covered := (emptyIndex+1)*app.EpochLength - 1
	mineIndependentClaimPast(t, f, covered)
	confirmIndependentWatchBlock(t, f, covered)
	_, err = f.reader.Tick(ctx)
	require.NoError(t, err)
	empty, err := f.repo.GetEpoch(ctx, app.Name, emptyIndex)
	require.True(t, err == nil || errors.Is(err, repository.ErrNotFound))
	require.Nil(t, empty, "an empty A/Q successor has no epoch row")
	consensus, staged := stageIndependentWatchClaim(t, f, app, proofSource, covered, quorum)
	confirmIndependentWatchBlock(t, f, staged.Raw.BlockNumber)
	claim, err := consensus.GetClaim(
		&bind.CallOpts{Context: ctx}, app.IApplicationAddress, staged.LastProcessedBlockNumber, staged.MachineMerkleRoot)
	require.NoError(t, err)
	deadline := new(big.Int).Add(claim.StagingBlockNumber, new(big.Int).SetUint64(independentWatchStagingPeriod))
	// E is also unclaimed. A later correct claim could cover it; its absence
	// alone is not the divergence oracle.
	onChainE, err := consensus.GetClaim(&bind.CallOpts{Context: ctx}, app.IApplicationAddress,
		new(big.Int).SetUint64(target.LastBlock), *target.MachineHash)
	require.NoError(t, err)
	require.Zero(t, onChainE.Status)
	var captured bytes.Buffer
	watch := createIndependentWatchReader(t, f, readerConfig, &captured)
	var stored *model.Application
	if lateValidation {
		warning := waitIndependentClaimDiagnostic(t, f, watch, &captured, app, false)
		require.Equal(t, strconv.FormatUint(covered, 10), warning["example_covered_block"])
		require.Equal(t, deadline.String(), warning["acceptance_from_contract_block"])
		stored, err = f.repo.GetApplication(ctx, app.Name)
		require.NoError(t, err)
		require.Equal(t, model.ApplicationStatus_OK, stored.Status, "IP bytes are not validated local evidence")
		cursor, err := f.repo.GetEventLastCheckBlock(ctx, app.ID, model.MonitoredEvent_ClaimAccepted)
		require.NoError(t, err)
		require.Less(t, cursor, staged.Raw.BlockNumber)
		// Crash reconstruction starts at the safe checkpoint, before the stage.
		// There is no persistent claim row and no recent-history cutoff.
		captured.Reset()
		watch = createIndependentWatchReader(t, f, readerConfig, &captured)
		replay := waitIndependentClaimDiagnostic(t, f, watch, &captured, app, false)
		require.Equal(t, strconv.FormatUint(covered, 10), replay["example_covered_block"])
		validateIndependentWatchEpochs(t, f, app.Name, proofSource.Index, target.Index)
	} else {
		// This is the QA shape: E is already CLAIM_COMPUTED; E+1 is empty.
		// A source-specific mismatch is still required, not any legacy status.
		validateIndependentWatchEpochs(t, f, app.Name, proofSource.Index, target.Index)
	}
	before, err := f.client.BlockNumber(ctx)
	require.NoError(t, err)
	captured.Reset()
	mismatch := waitIndependentClaimDiagnostic(t, f, watch, &captured, app, true)
	t.Logf("independent observer verdict: %s", captured.String())
	require.Equal(t, strconv.FormatUint(covered, 10), mismatch["covered_block"])
	require.Equal(t, common.Hash(staged.MachineMerkleRoot).Hex(), mismatch["machine_merkle_root"])
	require.Equal(t, common.Hash(staged.OutputsMerkleRoot).Hex(), mismatch["outputs_merkle_root"])
	require.Contains(t, mismatch["reason"], target.MachineHash.Hex())
	require.Contains(t, mismatch["reason"], target.TxBufferDataBlock.Hex())
	after, err := f.client.BlockNumber(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after, "ready evidence is rechecked at the same head")
	stored, err = f.repo.GetApplication(ctx, app.Name)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_Diverged, stored.Status)
	require.Contains(t, *stored.Reason, "independent_claim_mismatch")
	empty, err = f.repo.GetEpoch(ctx, app.Name, emptyIndex)
	require.True(t, err == nil || errors.Is(err, repository.ErrNotFound))
	require.Nil(t, empty)
	assertIndependentWatchWindow(t, f, staged, deadline)
}

func waitIndependentClaimDiagnostic(t *testing.T, f *terminalPrefixFixture, watch *claimer.Service,
	captured *bytes.Buffer, app *model.Application, mismatch bool) map[string]any {
	t.Helper()
	for range 32 {
		_, err := watch.Tick(f.ctx)
		require.NoError(t, err)
		decoder := json.NewDecoder(bytes.NewReader(captured.Bytes()))
		decoder.UseNumber()
		for {
			var record map[string]any
			err = decoder.Decode(&record)
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			if record["source"] != "independent_claim_observer" || record["application"] != app.Name {
				continue
			}
			if mismatch && record["msg"] == "Divergent on-chain claim observed" {
				return record
			}
			if !mismatch && record["msg"] == "Independent claim verification is incomplete" && record["example_phase"] == "staged" {
				return record
			}
		}
	}
	stored, statusErr := f.repo.GetApplication(f.ctx, app.Name)
	if stored != nil {
		reason := ""
		if stored.Reason != nil {
			reason = *stored.Reason
		}
		t.Logf("application state without the required diagnostic: status=%s reason=%q", stored.Status, reason)
	}
	t.Logf("application state read error: %v", statusErr)
	t.Fatalf("independent observer diagnostic absent (mismatch=%t): %s", mismatch, captured.String())
	return nil
}

// This is a stopped-node fixture override, following startReaderNode's saved
// config discipline. It never starts a background validator or publisher.
func independentWatchReaderConfig(t *testing.T, f *terminalPrefixFixture) config.ClaimerConfig {
	t.Helper()
	writer, ok := f.repo.(interface {
		SaveNodeConfigRaw(context.Context, string, []byte) error
	})
	require.True(t, ok)
	raw, _, _, err := f.repo.LoadNodeConfigRaw(f.ctx, claimer.ClaimerConfigKey)
	require.NoError(t, err)
	updated, err := patchPassiveObserverConfig(raw, map[string]any{passiveClaimSubmissionField: false})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		require.NoError(t, writer.SaveNodeConfigRaw(ctx, claimer.ClaimerConfigKey, raw))
	})
	require.NoError(t, writer.SaveNodeConfigRaw(f.ctx, claimer.ClaimerConfigKey, updated))
	cfg := *f.cfg.ToClaimerConfig()
	cfg.FeatureClaimSubmissionEnabled = false
	return cfg
}

func deployIndependentWatchApplication(t *testing.T, f *terminalPrefixFixture, quorum bool) *model.Application {
	t.Helper()
	name := uniqueAppName("independent-watch-authority")
	args := []string{terminalFixtureSaltFlag, uniqueSalt(), "--epoch-length", strconv.FormatUint(independentWatchEpochLength, 10),
		claimStagingPeriodFlag, strconv.FormatUint(independentWatchStagingPeriod, 10)}
	if quorum {
		name = uniqueAppName("independent-watch-quorum")
		quorumArgs := make([]string, 0, 16)
		quorumArgs = append(quorumArgs, deployCommand, string(withdrawalConsensusQuorum), jsonFlag,
			terminalFixtureSaltFlag, uniqueSalt(), "--epoch-length",
			strconv.FormatUint(independentWatchEpochLength, 10),
			claimStagingPeriodFlag, strconv.FormatUint(independentWatchStagingPeriod, 10))
		for _, address := range quorumValidatorAddresses(t) {
			quorumArgs = append(quorumArgs, "--validator", address.Hex())
		}
		out, err := runCLI(f.ctx, quorumArgs...)
		require.NoError(t, err)
		var deployment struct {
			Address string `json:"address"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &deployment))
		require.True(t, common.IsHexAddress(deployment.Address))
		args = []string{terminalFixtureSaltFlag, uniqueSalt(), independentWatchConsensusFlag, deployment.Address}
	}
	_, err := deployApplication(f.ctx, name, envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp"), args...)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		require.NoError(t, disableApplication(ctx, name))
	})
	app, err := f.repo.GetApplication(f.ctx, name)
	require.NoError(t, err)
	require.Equal(t, independentWatchEpochLength, app.EpochLength)
	require.Equal(t, independentWatchStagingPeriod, app.ClaimStagingPeriod)
	return app
}

func processedIndependentWatchEpoch(t *testing.T, f *terminalPrefixFixture, name string, index uint64) *model.Epoch {
	t.Helper()
	var epoch *model.Epoch
	err := pollUntil(f.ctx, time.Second, func() (bool, error) {
		if _, err := f.reader.Tick(f.ctx); err != nil {
			return false, err
		}
		if _, err := f.advancer.Tick(f.ctx); err != nil {
			return false, err
		}
		var err error
		epoch, err = f.repo.GetEpoch(f.ctx, name, index)
		return epoch != nil && epoch.Status == model.EpochStatus_InputsProcessed && epoch.HasCompleteStateProof(), err
	})
	require.NoError(t, err, "actual execution must provide an unvalidated complete proof")
	return epoch
}

func stageIndependentWatchClaim(t *testing.T, f *terminalPrefixFixture, app *model.Application,
	proofEpoch *model.Epoch, covered uint64, quorum bool,
) (*iconsensus.IConsensus, *iconsensus.IConsensusClaimStaged) {
	t.Helper()
	root, proof := authorityMachineValidityProof(t, proofEpoch)
	consensus, staged := stageIndependentWatchProof(t, f, app, root, proof, covered, quorum)
	old, err := consensus.GetClaim(&bind.CallOpts{Context: f.ctx}, app.IApplicationAddress,
		new(big.Int).SetUint64(proofEpoch.LastBlock), staged.MachineMerkleRoot)
	require.NoError(t, err)
	require.EqualValues(t, 0, old.Status, "the proof-source epoch has no staged claim")
	return consensus, staged
}

func stageIndependentWatchProof(t *testing.T, f *terminalPrefixFixture, app *model.Application,
	root common.Hash, proof iauthority.MachineValidityProof, covered uint64, quorum bool,
) (*iconsensus.IConsensus, *iconsensus.IConsensusClaimStaged) {
	t.Helper()
	consensus, err := iconsensus.NewIConsensus(app.IConsensusAddress, f.client)
	require.NoError(t, err)
	var staged *iconsensus.IConsensusClaimStaged
	if quorum {
		binding, err := iquorum.NewIQuorum(app.IConsensusAddress, f.client)
		require.NoError(t, err)
		quorumProof := iquorum.MachineValidityProof{
			IflagsYProof: iquorum.LeafProof(proof.IflagsYProof), HtifTohostProof: iquorum.LeafProof(proof.HtifTohostProof),
			TxBufferProof: iquorum.LeafProof(proof.TxBufferProof),
		}
		for _, index := range []uint32{quorumValidatorIndexA, quorumValidatorIndexB} {
			tx, err := binding.SubmitClaim(transactorForMnemonicIndex(f.ctx, t, f.client, index), app.IApplicationAddress,
				new(big.Int).SetUint64(covered), root, quorumProof)
			require.NoError(t, err)
			receipt := waitReceipt(f.ctx, t, f.client, tx)
			require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status)
			for _, log := range receipt.Logs {
				if event, err := consensus.ParseClaimStaged(*log); err == nil {
					staged = event
				}
			}
		}
	} else {
		binding, err := iauthority.NewIAuthority(app.IConsensusAddress, f.client)
		require.NoError(t, err)
		tx, err := binding.SubmitClaim(transactorForMnemonicIndex(f.ctx, t, f.client, 0), app.IApplicationAddress,
			new(big.Int).SetUint64(covered), root, proof)
		require.NoError(t, err)
		receipt := waitReceipt(f.ctx, t, f.client, tx)
		require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status)
		for _, log := range receipt.Logs {
			if event, err := consensus.ParseClaimStaged(*log); err == nil {
				staged = event
			}
		}
	}
	require.NotNil(t, staged, "the external Authority or Quorum validators must stage the claim")
	require.Equal(t, new(big.Int).SetUint64(covered), staged.LastProcessedBlockNumber)
	require.Equal(t, app.IApplicationAddress, staged.AppContract)
	require.Equal(t, root, common.Hash(staged.MachineMerkleRoot))
	require.Equal(t, common.Hash(proof.TxBufferProof.DataBlock), common.Hash(staged.OutputsMerkleRoot))
	return consensus, staged
}

func confirmIndependentWatchBlock(t *testing.T, f *terminalPrefixFixture, block uint64) {
	t.Helper()
	for range independentWatchFinalityBound {
		ready, err := confirmedBlockAtLeast(f.ctx, f.client.HeaderByNumber, f.cfg.BlockchainDefaultBlock, block)
		require.NoError(t, err)
		if ready {
			return
		}
		require.NoError(t, anvilMine(f.ctx, 1))
	}
	t.Fatalf("%s observation did not reach block %d within %d empty successors",
		f.cfg.BlockchainDefaultBlock, block, independentWatchFinalityBound)
}

func createIndependentWatchReader(
	t *testing.T, f *terminalPrefixFixture, cfg config.ClaimerConfig, captured *bytes.Buffer,
) *claimer.Service {
	t.Helper()
	svc, err := claimer.Create(f.ctx, &claimer.CreateInfo{Config: cfg, Repository: f.repo, EthConn: f.client,
		Logger: slog.New(slog.NewJSONHandler(captured, nil))})
	require.NoError(t, err)
	return svc.(*claimer.Service)
}

func assertIndependentWatchWindow(t *testing.T, f *terminalPrefixFixture, staged *iconsensus.IConsensusClaimStaged, deadline *big.Int) {
	t.Helper()
	observed, err := advanceFixtureFinality(f.ctx, f.client, f.cfg.BlockchainDefaultBlock)
	require.NoError(t, err)
	head, err := f.client.BlockNumber(f.ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, observed, staged.Raw.BlockNumber)
	require.Less(t, new(big.Int).SetUint64(head).Cmp(deadline), 0, "the alarm must occur before this fixture's actual acceptance threshold")
	t.Logf("before-acceptance detection opportunity (guardian response not exercised): "+
		"policy=%s stage=%d observed=%d latest=%d selected-view-lag=%d "+
		"period=%d acceptance_from=%s remaining=%s blocks",
		f.cfg.BlockchainDefaultBlock, staged.Raw.BlockNumber, observed, head, head-observed, independentWatchStagingPeriod,
		deadline, new(big.Int).Sub(deadline, new(big.Int).SetUint64(head)))
}

func validateIndependentWatchEpochs(t *testing.T, f *terminalPrefixFixture, name string, indices ...uint64) {
	t.Helper()
	svc, err := validator.Create(f.ctx, &validator.CreateInfo{Config: *f.cfg.ToValidatorConfig(), Repository: f.repo})
	require.NoError(t, err)
	err = pollUntil(f.ctx, time.Second, func() (bool, error) {
		if _, err := svc.(*validator.Service).Tick(f.ctx); err != nil {
			return false, err
		}
		for _, index := range indices {
			epoch, err := f.repo.GetEpoch(f.ctx, name, index)
			if err != nil {
				return false, err
			}
			if epoch.Status != model.EpochStatus_ClaimComputed {
				return false, nil
			}
		}
		return true, nil
	})
	require.NoError(t, err, "each expected result must come from the real validator")
}

// Keep each historical counter endpoint available in the Anvil fixture.
// A bulk anvil_mine call can omit intermediate state snapshots even when it
// returns their headers; those provider errors are not a NODE-1 test oracle.
func mineIndependentClaimPast(t *testing.T, f *terminalPrefixFixture, block uint64) {
	t.Helper()
	head, err := f.client.BlockNumber(f.ctx)
	require.NoError(t, err)
	if head > block {
		return
	}
	require.Less(t, block-head, uint64(maxBlocksToMine))
	for height := head; height <= block; height++ {
		require.NoError(t, anvilMine(f.ctx, 1))
	}
}
