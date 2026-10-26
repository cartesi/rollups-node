// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/advancer"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

// Run the real reader and advancer while the validator has not been created.
// The schedule, rather than a polling interval, guarantees ET(P) precedes
// validation of the complete earlier epoch E.
type terminalPrefixFixture struct {
	t          *testing.T
	ctx        context.Context
	repo       repository.Repository
	client     *ethclient.Client
	cfg        *config.NodeConfig
	reader     *evmreader.Service
	advancer   *advancer.Service
	machines   *manager.MachineManager
	supervisor service.Supervisor
	closed     bool
}

func newTerminalPrefixFixture(ctx context.Context, t *testing.T) *terminalPrefixFixture {
	t.Helper()
	require.Nil(t, sharedNode, "the fixture must own execution before the validator starts")
	_, _, err := loadRecoveryFixtureConfig()
	require.NoError(t, err)
	cfg, err := config.LoadNodeConfig()
	require.NoError(t, err)
	repo, err := factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	client := newIntegrationEthClient(ctx, t)
	t.Cleanup(client.Close)
	readerService, err := evmreader.Create(ctx, &evmreader.CreateInfo{
		Config: *cfg.ToEvmreaderConfig(), Repository: repo, EthClient: client,
	})
	require.NoError(t, err)
	f := &terminalPrefixFixture{t: t, ctx: ctx, repo: repo, client: client, cfg: cfg, reader: readerService.(*evmreader.Service)}
	logger := service.NewLogger("terminal-prefix-fixture", cfg.LogLevel, cfg.LogColor)
	f.machines = manager.NewMachineManager(repo, logger, cfg.FeatureMachineHashCheckEnabled, cfg.AdvancerInputBatchSize)
	f.supervisor, err = service.NewSupervisor(ctx, &service.SupervisorConfigs{
		BaseConfigs: service.BaseConfigs{Name: "terminal-prefix-fixture", Logger: logger},
		Factories: []service.FactoryFunction{func(ctx context.Context, sup service.Supervisor) (service.SupervisedService, error) {
			svc, createErr := advancer.Create(ctx, &advancer.CreateInfo{Config: *cfg.ToAdvancerConfig(), Logger: logger,
				Repository: repo, Machines: f.machines, Supervisor: sup})
			if createErr == nil {
				f.advancer = svc.(*advancer.Service)
			}
			return svc, createErr
		}},
	})
	require.NoError(t, err)
	t.Cleanup(f.closeExecution)
	return f
}

func (f *terminalPrefixFixture) closeExecution() {
	if !f.closed {
		f.machines.Close()
		f.supervisor.Stop()
		f.closed = true
	}
}

func (f *terminalPrefixFixture) waitInput(name string, index uint64, status model.InputCompletionStatus) *model.Input {
	f.t.Helper()
	var input *model.Input
	err := pollUntil(f.ctx, time.Second, func() (bool, error) {
		if _, err := advanceFixtureFinality(f.ctx, f.client, f.cfg.BlockchainDefaultBlock); err != nil {
			return false, err
		}
		if _, err := f.reader.Tick(f.ctx); err != nil {
			return false, err
		}
		if _, err := f.advancer.Tick(f.ctx); err != nil {
			return false, err
		}
		var err error
		input, err = f.repo.GetInput(f.ctx, name, index)
		return input != nil && input.Status == status, err
	})
	require.NoError(f.t, err, "reader/advancer must reach input %d %s without a validator", index, status)
	return input
}

func (f *terminalPrefixFixture) snapshotEveryInput(name string) {
	f.t.Helper()
	app, err := f.repo.GetApplication(f.ctx, name)
	require.NoError(f.t, err)
	app.ExecutionParameters.SnapshotPolicy = model.SnapshotPolicy_EveryInput
	require.NoError(f.t, f.repo.UpdateExecutionParameters(f.ctx, &app.ExecutionParameters))
}

func (f *terminalPrefixFixture) validateEarlier(name string, earlier, terminal *model.Input) *model.Epoch {
	f.t.Helper()
	require.Greater(f.t, terminal.EpochIndex, earlier.EpochIndex, "the terminal input must belong to a later epoch")
	app, err := f.repo.GetApplication(f.ctx, name)
	require.NoError(f.t, err)
	require.Equal(f.t, model.ApplicationStatus_MachineHalted, app.Status)
	epoch, err := f.repo.GetEpoch(f.ctx, name, earlier.EpochIndex)
	require.NoError(f.t, err)
	require.Equal(f.t, model.EpochStatus_InputsProcessed, epoch.Status)
	require.True(f.t, epoch.HasCompleteStateProof())
	require.Nil(f.t, epoch.Commitment)
	// This single Tick is the original-gate negative control on the baseline:
	// its OK-only application filter leaves E stuck at INPUTS_PROCESSED.
	svc, err := validator.Create(f.ctx, &validator.CreateInfo{Config: *f.cfg.ToValidatorConfig(), Repository: f.repo})
	require.NoError(f.t, err)
	_, err = svc.(*validator.Service).Tick(f.ctx)
	require.NoError(f.t, err)
	epoch, err = f.repo.GetEpoch(f.ctx, name, earlier.EpochIndex)
	require.NoError(f.t, err)
	require.Equal(f.t, model.EpochStatus_ClaimComputed, epoch.Status, "NODE-3: ET(P) must not hide valid E from the validator")
	stopped, err := f.repo.GetEpoch(f.ctx, name, terminal.EpochIndex)
	require.NoError(f.t, err)
	require.Nil(f.t, stopped.ClaimTransactionHash)
	require.Nil(f.t, stopped.Commitment)
	require.Contains(f.t, []model.EpochStatus{model.EpochStatus_Open, model.EpochStatus_Closed}, stopped.Status)
	return epoch
}

func (f *terminalPrefixFixture) preserveSnapshotOnTeardown(name string, input *model.Input) {
	f.t.Helper()
	require.NotNil(f.t, input.SnapshotURI)
	_, err := os.Stat(*input.SnapshotURI)
	require.NoError(f.t, err)
	app, err := f.repo.GetApplication(f.ctx, name)
	require.NoError(f.t, err)
	// UpdateMachines performs the real status-based teardown, independently of
	// pending earlier claims. It must retain the previously saved machine.
	_, err = f.advancer.Tick(f.ctx)
	require.NoError(f.t, err)
	require.False(f.t, f.machines.HasMachine(app.ID))
	stored, err := f.repo.GetInput(f.ctx, name, input.Index)
	require.NoError(f.t, err)
	require.Equal(f.t, input.SnapshotURI, stored.SnapshotURI)
	require.Equal(f.t, input.MachineHash, stored.MachineHash)
	_, err = os.Stat(*input.SnapshotURI)
	require.NoError(f.t, err, "terminal teardown preserves the recovery snapshot")
}

func (f *terminalPrefixFixture) waitEpoch(name string, index uint64, ready func(*model.Epoch) bool) *model.Epoch {
	f.t.Helper()
	var epoch *model.Epoch
	err := pollUntil(f.ctx, time.Second, func() (bool, error) {
		if _, err := advanceFixtureFinality(f.ctx, f.client, f.cfg.BlockchainDefaultBlock); err != nil {
			return false, err
		}
		var err error
		epoch, err = f.repo.GetEpoch(f.ctx, name, index)
		return epoch != nil && ready(epoch), err
	})
	require.NoError(f.t, err, "wait for epoch %d protocol progress", index)
	return epoch
}

func terminalPrefixMining(ctx context.Context, t *testing.T) {
	t.Helper()
	require.NoError(t, anvilRPC(ctx, "evm_setIntervalMining", 0))
	require.NoError(t, anvilRPC(ctx, "evm_setAutomine", true))
	t.Cleanup(func() {
		restore, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		require.NoError(t, anvilRPC(restore, "evm_setAutomine", true))
		require.NoError(t, anvilRPC(restore, "evm_setIntervalMining", 1))
		if sharedNode == nil {
			startSharedNode(t)
		}
	})
}

func (f *terminalPrefixFixture) minePast(block uint64) {
	f.t.Helper()
	head, err := f.client.BlockNumber(f.ctx)
	require.NoError(f.t, err)
	if head <= block {
		gap := block - head
		require.Less(f.t, gap, uint64(maxBlocksToMine))
		require.NoError(f.t, anvilMine(f.ctx, int(gap+1))) //nolint:gosec // Bounded by maxBlocksToMine above.
	}
}

func (f *terminalPrefixFixture) acceptClaim(name string, index uint64) *model.Epoch {
	f.t.Helper()
	staged := f.waitEpoch(name, index, func(epoch *model.Epoch) bool {
		return epoch.StagedAtBlock != nil || epoch.Status == model.EpochStatus_ClaimAccepted
	})
	app, err := f.repo.GetApplication(f.ctx, name)
	require.NoError(f.t, err)
	require.Greater(f.t, app.ClaimStagingPeriod, uint64(0))
	if staged.Status == model.EpochStatus_ClaimAccepted {
		return staged
	}
	f.minePast(*staged.StagedAtBlock + app.ClaimStagingPeriod)
	return f.waitEpoch(name, index, func(epoch *model.Epoch) bool { return epoch.Status == model.EpochStatus_ClaimAccepted })
}

func (s *TerminalMachineStatesSuite) TestEarlierEpochCompletesAfterLaterMachineHalt() {
	s.SetExpectedLogs(s.T(), anvilBlockOutOfRangeAllowlist)
	terminalPrefixMining(s.ctx, s.T())
	stopSharedNode(s.T())
	f := newTerminalPrefixFixture(s.ctx, s.T())
	s.appName = uniqueAppName("terminal-prefix-authority")
	path := envOrDefault("CARTESI_TEST_HALT_DAPP_PATH", "applications/halt-loop-dapp")
	withdrawal, _ := withdrawalConfigForGuardian(s.T(), 1)
	address, err := deployApplication(s.ctx, s.appName, path, "--salt", uniqueSalt(),
		claimStagingPeriodFlag, "5", "--withdrawal-config", withdrawal)
	s.Require().NoError(err)
	f.snapshotEveryInput(s.appName)
	_, _, err = sendInput(s.ctx, s.appName, "valid earlier input")
	s.Require().NoError(err)
	earlier := f.waitInput(s.appName, 0, model.InputCompletionStatus_Accepted)
	epoch, err := f.repo.GetEpoch(s.ctx, s.appName, earlier.EpochIndex)
	s.Require().NoError(err)
	f.minePast(epoch.LastBlock)
	_, _, err = sendInput(s.ctx, s.appName, "later terminal input")
	s.Require().NoError(err)
	terminal := f.waitInput(s.appName, 1, model.InputCompletionStatus_MachineHalted)
	f.validateEarlier(s.appName, earlier, terminal)
	f.preserveSnapshotOnTeardown(s.appName, earlier)
	f.closeExecution()
	startSharedNode(s.T())
	accepted := f.acceptClaim(s.appName, earlier.EpochIndex)

	// The same canonical chain is replayed with ET committed before validation.
	// Compare only the promised accepted prefix, not status/receipt history.
	s.Require().NoError(guardianForeclose(s.ctx, s.appName, 1))
	err = pollUntil(s.ctx, time.Second, func() (bool, error) {
		if _, err := advanceFixtureFinality(s.ctx, f.client, f.cfg.BlockchainDefaultBlock); err != nil {
			return false, err
		}
		app, err := f.repo.GetApplication(s.ctx, s.appName)
		if err != nil || app == nil || !app.IsForeclosed() || !app.ForeclosureScanCaughtUp() {
			return false, err
		}
		pending, err := f.repo.HasUnreconciledClaimsBeforeBlock(s.ctx, app.ID, app.ForecloseBlock)
		return !pending, err
	})
	s.Require().NoError(err)
	s.Require().NoError(disableApplication(s.ctx, s.appName))
	s.Require().NoError(removeApplication(s.ctx, s.appName))
	stopSharedNode(s.T())
	s.appName = uniqueAppName("terminal-prefix-replay")
	s.Require().NoError(registerApplication(s.ctx, s.appName, address, path))
	replay := newTerminalPrefixFixture(s.ctx, s.T())
	replay.snapshotEveryInput(s.appName)
	replayTerminal := replay.waitInput(s.appName, 1, model.InputCompletionStatus_MachineHalted)
	replayEarlier, err := replay.repo.GetInput(s.ctx, s.appName, 0)
	s.Require().NoError(err)
	replay.validateEarlier(s.appName, replayEarlier, replayTerminal)
	replay.preserveSnapshotOnTeardown(s.appName, replayEarlier)
	replay.closeExecution()
	startSharedNode(s.T())
	replayAccepted := replay.waitEpoch(s.appName, earlier.EpochIndex, func(epoch *model.Epoch) bool {
		return epoch.Status == model.EpochStatus_ClaimAccepted
	})
	compareReplayedEpoch(s.T(), s.Require(), accepted, replayAccepted, earlier.EpochIndex)
	compareReplayedInput(s.T(), s.Require(), earlier, replayEarlier, 0)
}

func (s *EchoQuorumSuite) TestEarlierEpochCompletesAfterLaterMachineHalt() {
	if !isNodeSelfManaged() {
		s.T().Skip("requires a test-managed node")
	}
	s.SetExpectedLogs(s.T(), anvilBlockOutOfRangeAllowlist)
	terminalPrefixMining(s.ctx, s.T())
	stopSharedNode(s.T())
	f := newTerminalPrefixFixture(s.ctx, s.T())
	deployment := s.deployQuorumApp("terminal-prefix-quorum", envOrDefault("CARTESI_TEST_HALT_DAPP_PATH", "applications/halt-loop-dapp"))
	f.snapshotEveryInput(s.appName)
	_, _, err := sendInput(s.ctx, s.appName, "valid earlier input")
	s.Require().NoError(err)
	earlier := f.waitInput(s.appName, 0, model.InputCompletionStatus_Accepted)
	epoch, err := f.repo.GetEpoch(s.ctx, s.appName, earlier.EpochIndex)
	s.Require().NoError(err)
	f.minePast(epoch.LastBlock)
	_, _, err = sendInput(s.ctx, s.appName, "later terminal input")
	s.Require().NoError(err)
	terminal := f.waitInput(s.appName, 1, model.InputCompletionStatus_MachineHalted)
	computed := f.validateEarlier(s.appName, earlier, terminal)
	f.preserveSnapshotOnTeardown(s.appName, earlier)
	s.submitQuorumClaim(deployment, computed, quorumValidatorIndexA, computed)
	f.closeExecution()
	startSharedNode(s.T())
	f.acceptClaim(s.appName, earlier.EpochIndex)
}

func (s *EchoPrtSuite) TestEarlierEpochCompletesAfterLaterMachineHalt() {
	if !isNodeSelfManaged() {
		s.T().Skip("requires a test-managed node")
	}
	s.SetExpectedLogs(s.T(), anvilBlockOutOfRangeAllowlist)
	terminalPrefixMining(s.ctx, s.T())
	s.appName = uniqueAppName("terminal-prefix-prt")
	_, err := deployApplication(s.ctx, s.appName, envOrDefault("CARTESI_TEST_HALT_DAPP_PATH", "applications/halt-loop-dapp"),
		"--salt", uniqueSalt(), prtFlag, claimStagingPeriodFlag, "5")
	s.Require().NoError(err)
	_, _, err = loadRecoveryFixtureConfig()
	s.Require().NoError(err)
	cfg, err := config.LoadNodeConfig()
	s.Require().NoError(err)
	repo, err := factory.NewRepositoryFromConnectionString(s.ctx, cfg.DatabaseConnection.Raw())
	s.Require().NoError(err)
	defer repo.Close()
	root0 := s.waitForRecoveryFixtureCommitment(repo, cfg.BlockchainDefaultBlock, 0)
	_, _, err = sendInput(s.ctx, s.appName, "valid epoch one input")
	s.Require().NoError(err)
	var earlier *model.Input
	err = pollUntil(s.ctx, time.Second, func() (bool, error) {
		if _, err := advanceFixtureFinality(s.ctx, s.ethClient, cfg.BlockchainDefaultBlock); err != nil {
			return false, err
		}
		var err error
		earlier, err = repo.GetInput(s.ctx, s.appName, 0)
		return earlier != nil && earlier.Status == model.InputCompletionStatus_Accepted, err
	})
	s.Require().NoError(err)
	stopSharedNode(s.T())
	app, err := repo.GetApplication(s.ctx, s.appName)
	s.Require().NoError(err)
	epoch0, err := repo.GetEpoch(s.ctx, s.appName, 0)
	s.Require().NoError(err)
	local, err := repo.GetCommitment(s.ctx, s.appName, 0, root0.Address.Hex(), epoch0.Commitment.Hex())
	s.Require().NoError(err)
	s.Require().NotNil(local)
	_, err = mineForTournamentTimeout(s.ctx, s.ethClient, root0.Address)
	s.Require().NoError(err)
	consensus, err := idaveconsensus.NewIDaveConsensus(app.IConsensusAddress, s.ethClient)
	s.Require().NoError(err)
	s.acceptStoppedPrtEpoch(consensus, app.IConsensusAddress, epoch0, local.SubmitterAddress)
	_, _, err = sendInput(s.ctx, s.appName, "later epoch two terminal input")
	s.Require().NoError(err)
	f := newTerminalPrefixFixture(s.ctx, s.T())
	terminal := f.waitInput(s.appName, 1, model.InputCompletionStatus_MachineHalted)
	computed := f.validateEarlier(s.appName, earlier, terminal)
	s.Require().Equal(uint64(1), computed.Index, "exercise a nonempty earlier PRT epoch")
	f.closeExecution()
	startSharedNode(s.T())
	root1 := s.waitForRecoveryFixtureCommitment(repo, cfg.BlockchainDefaultBlock, 1)
	// Joining is positively observed before any timeout mining for E=1.
	_, err = mineForTournamentTimeout(s.ctx, s.ethClient, root1.Address)
	s.Require().NoError(err)
	f.acceptClaim(s.appName, 1)
	stopped, err := repo.GetEpoch(s.ctx, s.appName, terminal.EpochIndex)
	s.Require().NoError(err)
	s.Require().Nil(stopped.Commitment)
	s.Require().Nil(stopped.ClaimTransactionHash)
	app, err = repo.GetApplication(s.ctx, s.appName)
	s.Require().NoError(err)
	s.Require().NotNil(app.Reason)
	root, closeBlock := prtProtectionEnd(s.ctx, s.T(), s.ethClient, app.IConsensusAddress, terminal.EpochIndex)
	requirePrtProtectionLogs(s.ctx, s.T(), &s.LogChecker, prtProtectionStopLog(s.appName, app.IApplicationAddress,
		terminal.EpochIndex, root, closeBlock, *app.Reason, prtPinnedClockSource))
}

// The real emulator returns a malformed accepted-root length in epoch 1.
// Epoch 0 is already complete, but its validation and all publication happen
// only after the runtime IOR has been committed by StoreAdvanceResult.
func (s *EchoPrtSuite) TestEarlierEpochCompletesAfterInvalidOutputsRoot() {
	if !isNodeSelfManaged() {
		s.T().Skip("requires a test-managed node")
	}
	s.appName = uniqueAppName("ior-prefix-prt")
	// The execution stop is produced by the in-process fixture below, not by
	// the shared node whose log file the suite scans. Assert its durable state.
	s.SetExpectedLogs(s.T(), anvilBlockOutOfRangeAllowlist)
	terminalPrefixMining(s.ctx, s.T())
	stopSharedNode(s.T())
	f := newTerminalPrefixFixture(s.ctx, s.T())
	path := envOrDefault("CARTESI_TEST_INVALID_OUTPUTS_ROOT_LENGTH_DAPP_PATH", "applications/invalid-outputs-root-length-dapp")
	_, err := deployApplication(s.ctx, s.appName, path, "--salt", uniqueSalt(), prtFlag, claimStagingPeriodFlag, "5")
	s.Require().NoError(err)
	_, _, err = sendInput(s.ctx, s.appName, "malformed root in later epoch")
	s.Require().NoError(err)
	terminal := f.waitInput(s.appName, 0, model.InputCompletionStatus_InvalidOutputsRoot)
	s.Require().Greater(terminal.EpochIndex, uint64(0))
	state, err := f.repo.GetEpochCompletionState(s.ctx, s.appName, 0)
	s.Require().NoError(err)
	s.Require().Equal(model.ApplicationStatus_InvalidOutputsRoot, state.Application.Status)
	s.Require().NotNil(state.Problem)
	s.Require().Equal(terminal.EpochIndex, state.Problem.EpochIndex)
	s.Require().Equal(model.EpochStatus_InputsProcessed, state.Epoch.Status)
	s.Require().True(state.Epoch.HasCompleteStateProof())
	s.Require().Nil(state.Epoch.Commitment)
	svc, err := validator.Create(s.ctx, &validator.CreateInfo{Config: *f.cfg.ToValidatorConfig(), Repository: f.repo})
	s.Require().NoError(err)
	_, err = svc.(*validator.Service).Tick(s.ctx)
	s.Require().NoError(err)
	earlier, err := f.repo.GetEpoch(s.ctx, s.appName, 0)
	s.Require().NoError(err)
	s.Require().Equal(model.EpochStatus_ClaimComputed, earlier.Status)
	f.closeExecution()
	startSharedNode(s.T())
	root := s.waitForRecoveryFixtureCommitment(f.repo, f.cfg.BlockchainDefaultBlock, 0)
	_, err = mineForTournamentTimeout(s.ctx, s.ethClient, root.Address)
	s.Require().NoError(err)
	f.acceptClaim(s.appName, 0)
	stopped, err := f.repo.GetEpochCompletionState(s.ctx, s.appName, terminal.EpochIndex)
	s.Require().NoError(err)
	s.Require().Equal(model.ApplicationStatus_InvalidOutputsRoot, stopped.Application.Status)
	s.Require().Equal(terminal.EpochIndex, stopped.Problem.EpochIndex)
	s.Require().Nil(stopped.Epoch.Commitment)
	s.Require().Nil(stopped.Epoch.ClaimTransactionHash)
	app := stopped.Application
	s.Require().NotNil(app.Reason)
	problemRoot, closeBlock := prtProtectionEnd(s.ctx, s.T(), s.ethClient, app.IConsensusAddress, terminal.EpochIndex)
	requirePrtProtectionLogs(s.ctx, s.T(), &s.LogChecker, prtProtectionStopLog(s.appName, app.IApplicationAddress,
		terminal.EpochIndex, problemRoot, closeBlock, *app.Reason, prtPinnedClockSource))
}
