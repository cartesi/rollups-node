// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/ierc20metadata"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	withdrawalGuardianIndex uint32 = 1
	withdrawalUserIndex     uint32 = 8

	withdrawalDepositAmount       uint64 = 100
	withdrawalPreForecloseAmount  uint64 = 25
	withdrawalPostForecloseAmount uint64 = withdrawalDepositAmount - withdrawalPreForecloseAmount

	defaultDevnetERC20PortalAddress             = "0x3332DE61a8BB9aC84893b2f552Fe81C9a6dC5419"
	defaultDevnetTestUsdcAddress                = "0x7a051EDffC0884cd88d4a377F4C87BE074CF6c81"
	defaultDevnetWithdrawalOutputBuilderAddress = "0xB4D253c7a110241561B3eD6d632846dF7d4e9Af7"

	accountsDriveLog2AccountSize      = uint8(5)
	accountsDriveLog2MaxNumOfAccounts = uint8(17)
	accountsDriveLog2LeavesPerAccount = uint8(0)
	accountsDriveLog2Size             = accountsDriveLog2AccountSize + accountsDriveLog2MaxNumOfAccounts + accountsDriveLog2LeavesPerAccount
	accountsDriveSize                 = uint64(1) << accountsDriveLog2Size

	machineToolBinary = "cartesi-rollups-machine-tool"
)

type withdrawalConsensus string

const (
	withdrawalConsensusAuthority withdrawalConsensus = "authority"
	withdrawalConsensusQuorum    withdrawalConsensus = "quorum"
	withdrawalConsensusPRT       withdrawalConsensus = "prt"
)

type accountsDriveKind string

const (
	accountsDriveFlash accountsDriveKind = "flash_drive"
	accountsDriveNVRAM accountsDriveKind = "nvram"
)

type WithdrawalLifecycleSuite struct {
	suite.Suite
	LogChecker
	ctx     context.Context
	cancel  context.CancelFunc
	client  *ethclient.Client
	chainID *big.Int
	appName string
}

func TestWithdrawalLifecycle(t *testing.T) {
	suite.Run(t, new(WithdrawalLifecycleSuite))
}

func (s *WithdrawalLifecycleSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 30*time.Minute)

	endpoint := envOrDefault("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	client, err := ethclient.DialContext(s.ctx, endpoint)
	s.Require().NoError(err, "dial ethclient")
	s.client = client

	chainID, err := client.ChainID(s.ctx)
	s.Require().NoError(err, "fetch chain id")
	s.chainID = chainID
}

func (s *WithdrawalLifecycleSuite) TearDownSuite() {
	if s.client != nil {
		s.client.Close()
	}
	s.cancel()
}

func (s *WithdrawalLifecycleSuite) SetupTest() {
	s.StartLogCapture()
	s.appName = ""
}

func (s *WithdrawalLifecycleSuite) TearDownTest() {
	if s.appName != "" {
		_ = disableApplication(s.ctx, s.appName) //nolint:errcheck
	}
	s.CheckLogs(s.T())
}

func (s *WithdrawalLifecycleSuite) TestAuthorityPostForeclosureWithdrawalLifecycle() {
	s.runWithdrawalLifecycle(withdrawalConsensusAuthority, accountsDriveFlash)
}

func (s *WithdrawalLifecycleSuite) TestAuthorityNVRAMPostForeclosureWithdrawalLifecycle() {
	s.runWithdrawalLifecycle(withdrawalConsensusAuthority, accountsDriveNVRAM)
}

func (s *WithdrawalLifecycleSuite) TestQuorumPostForeclosureWithdrawalLifecycle() {
	s.runWithdrawalLifecycle(withdrawalConsensusQuorum, accountsDriveFlash)
}

func (s *WithdrawalLifecycleSuite) TestPRTPostForeclosureWithdrawalLifecycle() {
	s.SetExpectedLogs(s.T(), prtBlockOutOfRangeAllowlist)
	s.runWithdrawalLifecycle(withdrawalConsensusPRT, accountsDriveFlash)
}

type withdrawalAppDeployment struct {
	appName          string
	appAddress       common.Address
	consensusAddress common.Address
	quorum           *iquorum.IQuorum
	withdrawalConfig model.WithdrawalConfig
}

func (s *WithdrawalLifecycleSuite) runWithdrawalLifecycle(consensus withdrawalConsensus, driveKind accountsDriveKind) {
	r := s.Require()
	defer timed(s.T(), fmt.Sprintf("full %s %s withdrawal lifecycle", consensus, driveKind))()

	dappPath := envOrDefault("CARTESI_TEST_ERC20_WITHDRAWAL_DAPP_PATH", "applications/erc20-withdrawal-dapp")
	if driveKind == accountsDriveNVRAM {
		dappPath = envOrDefault("CARTESI_TEST_ERC20_WITHDRAWAL_NVRAM_DAPP_PATH", "applications/erc20-withdrawal-nvram-dapp")
	}
	portalAddr := devnetAddress(s.T(), "CARTESI_DEVNET_ERC20_PORTAL_ADDRESS", defaultDevnetERC20PortalAddress)
	tokenAddr := devnetAddress(s.T(), "CARTESI_DEVNET_TEST_USDC_ADDRESS", defaultDevnetTestUsdcAddress)
	userAddr := mnemonicAddress(s.T(), withdrawalUserIndex)
	initialUserBalance := s.tokenBalance(tokenAddr, userAddr)

	s.T().Logf("--- Setup: %s %s ERC-20 withdrawal lifecycle ---", consensus, driveKind)
	s.T().Logf("    dapp=%s", dappPath)
	s.T().Logf("    user mnemonic[%d]=%s", withdrawalUserIndex, userAddr.Hex())

	r.NoError(anvilSetBalance(s.ctx, userAddr.Hex(), oneEtherWei), "fund user with ETH")
	s.mintTestToken(tokenAddr, withdrawalUserIndex, new(big.Int).SetUint64(withdrawalDepositAmount))
	s.requireTokenBalance(tokenAddr, userAddr, tokenBalanceWithDelta(initialUserBalance, withdrawalDepositAmount), "user after mint")

	deployment := s.deployWithdrawalApp(consensus, dappPath)
	s.appName = deployment.appName
	requireAccountsDriveRange(s.T(), dappPath, driveKind, deployment.withdrawalConfig)
	if consensus != withdrawalConsensusPRT {
		s.waitForIConsensusInputCursor(deployment.appName)
	}

	inputBoxAddr := inputBoxAddress(s.T())
	depositInputIndex := inputBoxInputCount(s.ctx, s.T(), s.client, inputBoxAddr, deployment.appAddress)
	s.depositERC20(deployment.appName, portalAddr, tokenAddr, withdrawalUserIndex, withdrawalDepositAmount)

	depositInput := s.waitForAcceptedInput(deployment.appName, depositInputIndex)
	s.T().Logf("    deposit input accepted: input=%d epoch=%d", depositInput.Index, depositInput.EpochIndex)
	s.requireTokenBalance(tokenAddr, userAddr, initialUserBalance, "user after portal deposit")
	s.requireTokenBalance(tokenAddr, deployment.appAddress, tokenAmount(withdrawalDepositAmount), "application after portal deposit")

	withdrawInputIndex := s.sendWithdrawalRequest(deployment.appName, withdrawalPreForecloseAmount)
	withdrawInput := s.waitForAcceptedInput(deployment.appName, withdrawInputIndex)
	s.T().Logf("    pre-foreclosure withdrawal input accepted: input=%d epoch=%d",
		withdrawInput.Index, withdrawInput.EpochIndex)

	withdrawOutputIndex := s.waitForVoucherOutput(deployment.appName, withdrawInput.Index)
	finalEpoch := s.finalizeWithdrawalEpoch(consensus, deployment, depositInput.EpochIndex, withdrawInput.EpochIndex)

	txHash, err := executeOutput(s.ctx, deployment.appName, withdrawOutputIndex)
	r.NoError(err, "execute pre-foreclosure withdrawal output")
	r.NotEmpty(txHash)
	execCtx, execCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	r.NoError(waitForExecutionRecorded(execCtx, s.T(), deployment.appName, withdrawOutputIndex),
		"wait for pre-foreclosure withdrawal output execution")
	execCancel()
	s.requireTokenBalance(tokenAddr, userAddr, tokenBalanceWithDelta(initialUserBalance, withdrawalPreForecloseAmount),
		"user after pre-foreclosure output")
	s.requireTokenBalance(tokenAddr, deployment.appAddress, tokenAmount(withdrawalPostForecloseAmount),
		"application after pre-foreclosure output")

	r.NoError(guardianForeclose(s.ctx, deployment.appName, withdrawalGuardianIndex), "guardian foreclose")
	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), deployment.appName), "node did not record foreclosure")
	forecloseCancel()

	driveProof, withdrawProof, accountIndex := s.generateWithdrawalProofs(deployment, dappPath, driveKind, finalEpoch)
	_, err = runCLI(s.ctx, "prove-drive-root", deployment.appName, "--proof-file", driveProof, "--yes")
	r.NoError(err, "prove accounts-drive root")
	s.waitForAccountsDriveProved(deployment.appName)

	_, err = runCLI(s.ctx, "withdraw", deployment.appName, "--proof-file", withdrawProof, "--yes")
	r.NoError(err, "post-foreclosure withdraw")
	s.waitForWithdrawalRecorded(deployment.appName, accountIndex)

	s.requireTokenBalance(tokenAddr, userAddr, tokenBalanceWithDelta(initialUserBalance, withdrawalDepositAmount),
		"user after post-foreclosure withdraw")
	s.requireTokenBalance(tokenAddr, deployment.appAddress, tokenAmount(0), "application after post-foreclosure withdraw")
}

func (s *WithdrawalLifecycleSuite) deployWithdrawalApp(
	consensus withdrawalConsensus,
	dappPath string,
) withdrawalAppDeployment {
	r := s.Require()
	appName := uniqueAppName(fmt.Sprintf("withdraw-%s", consensus))
	driveStartIndex := accountsDriveStartIndex(s.T(), dappPath)
	withdrawalConfigJSON := withdrawalConfigForAccountsDrive(s.T(), withdrawalGuardianIndex, driveStartIndex)

	var appAddrStr string
	var consensusAddrStr string
	var quorumBinding *iquorum.IQuorum
	var err error

	switch consensus {
	case withdrawalConsensusAuthority:
		appAddrStr, err = deployApplication(s.ctx, appName, dappPath,
			"--salt", uniqueSalt(),
			"--withdrawal-config", withdrawalConfigJSON,
			"--enable=false",
		)
		r.NoError(err, "deploy authority withdrawal app")
	case withdrawalConsensusPRT:
		appAddrStr, err = deployApplication(s.ctx, appName, dappPath,
			"--salt", uniqueSalt(),
			"--prt",
			"--withdrawal-config", withdrawalConfigJSON,
			"--enable=false",
		)
		r.NoError(err, "deploy PRT withdrawal app")
	case withdrawalConsensusQuorum:
		validators := quorumValidatorAddresses(s.T())
		quorumArgs := []string{
			"deploy", "quorum",
			"--json",
			"--salt", uniqueSalt(),
			claimStagingPeriodFlag, strconv.FormatUint(quorumClaimStagingPeriod, 10),
		}
		for _, validator := range validators {
			quorumArgs = append(quorumArgs, "--validator", validator.Hex())
		}
		out, qErr := runCLI(s.ctx, quorumArgs...)
		r.NoError(qErr, "deploy quorum")
		var quorumDeployment struct {
			Address string `json:"address"`
		}
		r.NoError(json.Unmarshal([]byte(out), &quorumDeployment), "parse quorum deployment")
		r.NotEmpty(quorumDeployment.Address, "quorum deployment missing address")

		appAddrStr, consensusAddrStr, err = deployApplicationWithConsensus(
			s.ctx,
			appName,
			dappPath,
			"--consensus", quorumDeployment.Address,
			"--salt", uniqueSalt(),
			"--withdrawal-config", withdrawalConfigJSON,
			"--enable=false",
		)
		r.NoError(err, "deploy quorum withdrawal app")
		r.Equal(common.HexToAddress(quorumDeployment.Address), common.HexToAddress(consensusAddrStr),
			"application must use the freshly deployed quorum consensus")
		quorumBinding, err = iquorum.NewIQuorum(common.HexToAddress(consensusAddrStr), s.client)
		r.NoError(err, "bind quorum consensus")
	default:
		r.FailNowf("unknown consensus", "unknown consensus %s", consensus)
	}

	withdrawalConfig, err := ethutil.GetApplicationWithdrawalConfig(s.ctx, s.client, common.HexToAddress(appAddrStr))
	r.NoError(err, "read deployed application withdrawal config")
	r.Equal(driveStartIndex, withdrawalConfig.AccountsDriveStartIndex, "deployed config must match the fixture accounts address")
	r.Equal(accountsDriveLog2MaxNumOfAccounts, withdrawalConfig.Log2MaxNumOfAccounts)
	r.Equal(accountsDriveLog2LeavesPerAccount, withdrawalConfig.Log2LeavesPerAccount)

	_, err = runCLI(s.ctx, "app", "execution-parameters", "set",
		appName, "snapshot_policy", string(model.SnapshotPolicy_EveryEpoch))
	r.NoError(err, "set snapshot policy")
	_, err = runCLI(s.ctx, "app", "status", appName, "enabled", "--yes")
	r.NoError(err, "enable app after setting snapshot policy")

	return withdrawalAppDeployment{
		appName:          appName,
		appAddress:       common.HexToAddress(appAddrStr),
		consensusAddress: common.HexToAddress(consensusAddrStr),
		quorum:           quorumBinding,
		withdrawalConfig: model.WithdrawalConfig(withdrawalConfig),
	}
}

func (s *WithdrawalLifecycleSuite) finalizeWithdrawalEpoch(
	consensus withdrawalConsensus,
	deployment withdrawalAppDeployment,
	epochIndexes ...uint64,
) *model.Epoch {
	r := s.Require()
	r.NotEmpty(epochIndexes, "at least one epoch index is required")
	epochIndexes = uniqueEpochIndexes(epochIndexes)
	targetEpochIndex := epochIndexes[len(epochIndexes)-1]

	switch consensus {
	case withdrawalConsensusAuthority:
		minePastEpochBoundary(s.ctx, s.T(), r, deployment.appName, targetEpochIndex)
		claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
		epoch, err := waitForEpochStatus(claimCtx, s.T(), deployment.appName, targetEpochIndex, model.EpochStatus_ClaimAccepted)
		claimCancel()
		r.NoError(err, "wait for authority claim accepted")
		return epoch
	case withdrawalConsensusQuorum:
		minePastEpochBoundary(s.ctx, s.T(), r, deployment.appName, targetEpochIndex)
		var finalEpoch *model.Epoch
		for _, epochIndex := range epochIndexes {
			finalEpoch = s.finalizeQuorumEpoch(deployment, epochIndex)
		}
		return finalEpoch
	case withdrawalConsensusPRT:
		for i := uint64(0); i <= targetEpochIndex; i++ {
			finalizePrtEpoch(s.ctx, s.T(), r, s.client, deployment.appName, i)
		}
		claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
		epoch, err := waitForEpochStatus(claimCtx, s.T(), deployment.appName, targetEpochIndex, model.EpochStatus_ClaimAccepted)
		claimCancel()
		r.NoError(err, "wait for PRT claim accepted")
		return epoch
	default:
		r.FailNowf("unknown consensus", "unknown consensus %s", consensus)
		return nil
	}
}

func uniqueEpochIndexes(epochIndexes []uint64) []uint64 {
	unique := make([]uint64, 0, len(epochIndexes))
	seen := map[uint64]struct{}{}
	for _, epochIndex := range epochIndexes {
		if _, ok := seen[epochIndex]; ok {
			continue
		}
		seen[epochIndex] = struct{}{}
		unique = append(unique, epochIndex)
	}
	return unique
}

func (s *WithdrawalLifecycleSuite) finalizeQuorumEpoch(
	deployment withdrawalAppDeployment,
	epochIndex uint64,
) *model.Epoch {
	epoch := s.waitForQuorumEpochWithClaim(deployment.appName, epochIndex)
	switch epoch.Status {
	case model.EpochStatus_ClaimAccepted:
		return epoch
	case model.EpochStatus_ClaimStaged:
		return s.waitForQuorumAccepted(deployment.appName, epochIndex)
	case model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted:
		s.submitQuorumClaim(deployment, epoch, quorumValidatorIndexA)
		s.submitQuorumClaim(deployment, epoch, quorumValidatorIndexB)
		return s.waitForQuorumAccepted(deployment.appName, epochIndex)
	default:
		s.Require().FailNowf("unexpected quorum epoch status",
			"epoch %d has claim data but status %s cannot be finalized by the test", epochIndex, epoch.Status)
		return nil
	}
}

func (s *WithdrawalLifecycleSuite) waitForQuorumEpochWithClaim(appName string, epochIndex uint64) *model.Epoch {
	ctx, cancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	defer cancel()

	var result *model.Epoch
	var lastErr error
	err := pollUntil(ctx, 2*time.Second, func() (bool, error) {
		epoch, err := readEpoch(ctx, appName, epochIndex)
		if err != nil {
			if isCLIExitError(err) {
				lastErr = err
				s.T().Logf("poll epoch %d claim: %v (retrying)", epochIndex, err)
				return false, nil
			}
			return false, fmt.Errorf("poll epoch %d claim: %w", epochIndex, err)
		}
		if epoch.HasCompleteStateProof() && isQuorumClaimReadyStatus(epoch.Status) {
			result = epoch
			return true, nil
		}
		s.T().Logf("    waiting for quorum claim for epoch %d (status=%s)", epochIndex, epoch.Status)
		return false, nil
	})
	if err != nil && lastErr != nil {
		err = fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	s.Require().NoError(err, "wait for quorum epoch %d claim computation", epochIndex)
	return result
}

func (s *WithdrawalLifecycleSuite) submitQuorumClaim(
	deployment withdrawalAppDeployment,
	epoch *model.Epoch,
	accountIndex uint32,
) {
	r := s.Require()
	r.NotNil(deployment.quorum, "quorum binding is required")
	machineRoot, proof := quorumMachineValidityProof(s.T(), epoch)

	key, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, accountIndex)
	r.NoError(err, "derive validator key %d", accountIndex)
	opts, err := bind.NewKeyedTransactorWithChainID(key, s.chainID)
	r.NoError(err, "new validator transactor %d", accountIndex)
	opts.Context = s.ctx

	tx, err := deployment.quorum.SubmitClaim(
		opts,
		deployment.appAddress,
		new(big.Int).SetUint64(epoch.LastBlock),
		machineRoot,
		proof,
	)
	r.NoError(err, "validator %d submit quorum claim", accountIndex)
	receipt, err := bind.WaitMined(s.ctx, s.client, tx)
	r.NoError(err, "wait for validator %d quorum submit tx", accountIndex)
	r.Equal(types.ReceiptStatusSuccessful, receipt.Status, "validator %d quorum submit tx must succeed", accountIndex)
}

func (s *WithdrawalLifecycleSuite) waitForQuorumAccepted(appName string, epochIndex uint64) *model.Epoch {
	r := s.Require()
	stagedCtx, stagedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	staged, err := waitForEpochStatus(stagedCtx, s.T(), appName, epochIndex, model.EpochStatus_ClaimStaged)
	stagedCancel()
	r.NoError(err, "wait for quorum claim to stage")

	if staged.StagedAtBlock != nil {
		s.minePastBlock(*staged.StagedAtBlock + quorumClaimStagingPeriod)
	} else {
		r.NoError(anvilMine(s.ctx, int(quorumClaimStagingPeriod)+1), "mine past claim staging period")
	}

	acceptedCtx, acceptedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	accepted, err := waitForEpochStatus(acceptedCtx, s.T(), appName, epochIndex, model.EpochStatus_ClaimAccepted)
	acceptedCancel()
	r.NoError(err, "wait for quorum claim accepted")
	return accepted
}

func (s *WithdrawalLifecycleSuite) minePastBlock(block uint64) {
	currentBlock, err := s.client.BlockNumber(s.ctx)
	s.Require().NoError(err, "read current block")
	if currentBlock > block {
		return
	}
	blocksToMine := int(block - currentBlock + 1)
	s.Require().NoError(anvilMine(s.ctx, blocksToMine), "mine past block %d", block)
}

func (s *WithdrawalLifecycleSuite) depositERC20(
	appName string,
	portal common.Address,
	token common.Address,
	userIndex uint32,
	amount uint64,
) {
	out, err := runCLIWithEnv(s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", userIndex)},
		"deposit", "erc20", appName,
		"--portal", portal.Hex(),
		"--token", token.Hex(),
		"--amount", strconv.FormatUint(amount, 10),
		"--approve",
		"--yes",
		"--json",
	)
	s.Require().NoError(err, "ERC-20 deposit CLI call: %s", out)
}

func (s *WithdrawalLifecycleSuite) sendWithdrawalRequest(
	appName string,
	amount uint64,
) uint64 {
	payload := withdrawalRequestPayload(amount)
	out, err := runCLIWithEnv(s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", withdrawalUserIndex)},
		"send", appName, payload, "--hex", "--yes", "--json",
	)
	s.Require().NoError(err, "send withdrawal request")
	var result struct {
		InputIndex string `json:"input_index"`
	}
	s.Require().NoError(json.Unmarshal([]byte(out), &result), "parse send output")
	inputIndex, err := strconv.ParseUint(strings.TrimPrefix(result.InputIndex, "0x"), 16, 64)
	s.Require().NoError(err, "parse withdrawal input index")
	return inputIndex
}

func (s *WithdrawalLifecycleSuite) waitForAcceptedInput(appName string, inputIndex uint64) *model.Input {
	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	input, err := waitForInputProcessed(processCtx, s.T(), appName, inputIndex)
	processCancel()
	s.Require().NoError(err, "wait for input %d processing", inputIndex)
	s.Require().Equal(model.InputCompletionStatus_Accepted, input.Status, "input %d should be accepted", inputIndex)
	return input
}

func (s *WithdrawalLifecycleSuite) waitForIConsensusInputCursor(appName string) {
	r := s.Require()
	currentBlock, err := s.client.BlockNumber(s.ctx)
	r.NoError(err, "read current block before first input")

	dsn, err := config.GetDatabaseConnection()
	r.NoError(err, "get database connection")
	repo, err := factory.NewRepositoryFromConnectionString(s.ctx, dsn.Raw())
	r.NoError(err, "open repository")
	defer repo.Close()

	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()
	err = pollUntil(ctx, 3*time.Second, func() (bool, error) {
		// On an automine devnet no block is produced until a transaction is
		// sent, but the EVM reader only advances LastInputCheckBlock when it
		// processes a new block header. The first deposit has not been sent
		// yet, so mine an empty block each poll to give the reader a header to
		// scan; a single scan catches the cursor up to head. Without this the
		// wait stalls on an idle chain until the timeout.
		if err := anvilMine(ctx, 1); err != nil {
			return false, err
		}
		app, err := repo.GetApplication(ctx, appName)
		if err != nil {
			return false, err
		}
		if app == nil {
			return false, nil
		}
		return app.LastInputCheckBlock >= currentBlock, nil
	})
	r.NoError(err, "wait for initial input sync before first deposit")
}

func (s *WithdrawalLifecycleSuite) waitForVoucherOutput(appName string, inputIndex uint64) uint64 {
	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()

	var outputIndex uint64
	err := pollUntil(ctx, 2*time.Second, func() (bool, error) {
		outputs, err := readOutputs(ctx, appName)
		if err != nil {
			if isCLIExitError(err) {
				return false, nil
			}
			return false, err
		}
		for _, output := range outputs.Data {
			if output.InputIndex == inputIndex && output.DecodedData != nil &&
				output.DecodedData.Type == "Voucher" {
				outputIndex = output.Index
				return true, nil
			}
		}
		return false, nil
	})
	s.Require().NoError(err, "wait for voucher output for input %d", inputIndex)
	return outputIndex
}

func (s *WithdrawalLifecycleSuite) generateWithdrawalProofs(
	deployment withdrawalAppDeployment,
	dappPath string,
	driveKind accountsDriveKind,
	finalEpoch *model.Epoch,
) (string, string, string) {
	r := s.Require()
	r.NotNil(finalEpoch.MachineHash, "finalized epoch must have machine hash")

	tmp := s.T().TempDir()
	snapshotPath := filepath.Join(tmp, "snapshot")
	replayOut := runMachineTool(s.ctx, s.T(),
		"replay",
		"--template", dappPath,
		"--application", deployment.appName,
		"--database-connection", envOrDefault("CARTESI_DATABASE_CONNECTION", ""),
		"--to-epoch", strconv.FormatUint(finalEpoch.Index, 10),
		"--store", snapshotPath,
	)
	var replaySummary struct {
		MachineRoot string `json:"machine_root"`
	}
	r.NoError(json.Unmarshal([]byte(replayOut), &replaySummary), "parse machine replay summary")
	r.Equal(strings.ToLower(finalEpoch.MachineHash.Hex()), strings.ToLower(replaySummary.MachineRoot),
		"replayed machine root must match finalized epoch machine hash")
	requireAccountsDriveRange(s.T(), snapshotPath, driveKind, deployment.withdrawalConfig)

	driveProofPath := filepath.Join(tmp, "drive-root-proof.json")
	withdrawProofPath := filepath.Join(tmp, "withdraw-proof.json")
	withdrawalConfig := deployment.withdrawalConfig
	proveOut := runMachineTool(s.ctx, s.T(),
		"prove", "accounts-drive",
		"--snapshot", snapshotPath,
		"--accounts-drive-start-index", strconv.FormatUint(withdrawalConfig.AccountsDriveStartIndex, 10),
		"--log2-max-num-of-accounts", strconv.Itoa(int(withdrawalConfig.Log2MaxNumOfAccounts)),
		"--log2-leaves-per-account", strconv.Itoa(int(withdrawalConfig.Log2LeavesPerAccount)),
		"--account", mnemonicAddress(s.T(), withdrawalUserIndex).Hex(),
		"--out-drive-root-proof", driveProofPath,
		"--out-withdraw-proof", withdrawProofPath,
	)
	var proveSummary struct {
		AccountIndex string `json:"account_index"`
		MachineRoot  string `json:"machine_root"`
	}
	r.NoError(json.Unmarshal([]byte(proveOut), &proveSummary), "parse accounts-drive proof summary")
	r.Equal(strings.ToLower(finalEpoch.MachineHash.Hex()), strings.ToLower(proveSummary.MachineRoot),
		"proof machine root must match finalized epoch machine hash")
	r.NotEmpty(proveSummary.AccountIndex, "proof summary must include account index")
	return driveProofPath, withdrawProofPath, proveSummary.AccountIndex
}

func (s *WithdrawalLifecycleSuite) waitForAccountsDriveProved(appName string) {
	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()
	err := pollUntil(ctx, 3*time.Second, func() (bool, error) {
		status, err := readApplicationStatus(ctx, appName)
		if err != nil {
			return false, err
		}
		return strings.Contains(status, "Accounts drive proved block:"), nil
	})
	s.Require().NoError(err, "node did not record accounts-drive proof")
}

func (s *WithdrawalLifecycleSuite) waitForWithdrawalRecorded(appName string, accountIndex string) {
	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()
	err := pollUntil(ctx, 3*time.Second, func() (bool, error) {
		withdrawals, err := listWithdrawals(ctx, appName, accountIndex)
		if err != nil {
			return false, err
		}
		return withdrawals.Pagination.TotalCount > 0, nil
	})
	s.Require().NoError(err, "node did not record withdrawal")
}

func (s *WithdrawalLifecycleSuite) mintTestToken(tokenAddr common.Address, userIndex uint32, amount *big.Int) {
	tokenABI, err := abi.JSON(strings.NewReader(testFungibleTokenABIJSON))
	s.Require().NoError(err, "parse test token ABI")
	token := bind.NewBoundContract(tokenAddr, tokenABI, s.client, s.client, s.client)
	opts := transactorForMnemonicIndex(s.ctx, s.T(), s.client, userIndex)
	tx, err := token.Transact(opts, "mint", amount)
	s.Require().NoError(err, "mint test token")
	receipt, err := bind.WaitMined(s.ctx, s.client, tx)
	s.Require().NoError(err, "wait for mint tx")
	s.Require().Equal(types.ReceiptStatusSuccessful, receipt.Status, "mint tx must succeed")
}

func (s *WithdrawalLifecycleSuite) requireTokenBalance(
	tokenAddr common.Address,
	account common.Address,
	want *big.Int,
	label string,
) {
	got := s.tokenBalance(tokenAddr, account)
	s.Require().Zero(got.Cmp(want), label)
}

func (s *WithdrawalLifecycleSuite) tokenBalance(tokenAddr common.Address, account common.Address) *big.Int {
	token, err := ierc20metadata.NewIERC20Metadata(tokenAddr, s.client)
	s.Require().NoError(err, "bind token")
	got, err := token.BalanceOf(&bind.CallOpts{Context: s.ctx}, account)
	s.Require().NoError(err, "read token balance")
	return got
}

func tokenAmount(amount uint64) *big.Int {
	return new(big.Int).SetUint64(amount)
}

func tokenBalanceWithDelta(base *big.Int, delta uint64) *big.Int {
	return new(big.Int).Add(new(big.Int).Set(base), tokenAmount(delta))
}

const testFungibleTokenABIJSON = `[
  {
    "type": "function",
    "name": "mint",
    "inputs": [{"name": "value", "type": "uint256"}],
    "outputs": [],
    "stateMutability": "nonpayable"
  }
]`

func withdrawalRequestPayload(amount uint64) string {
	return fmt.Sprintf("0x01%016x", amount)
}

func withdrawalConfigForAccountsDrive(t testing.TB, guardianIndex uint32, accountsDriveStartIndex uint64) string {
	t.Helper()
	builder := devnetAddress(t, "CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS",
		defaultDevnetWithdrawalOutputBuilderAddress)
	guardianAddr := mnemonicAddress(t, guardianIndex)
	return strings.NewReplacer("\n", "", "\t", "").Replace(fmt.Sprintf(`{
		"guardian": "%s",
		"log2_leaves_per_account": %d,
		"log2_max_num_of_accounts": %d,
		"accounts_drive_start_index": %d,
		"withdrawal_output_builder": "%s"
	}`, guardianAddr.Hex(), accountsDriveLog2LeavesPerAccount, accountsDriveLog2MaxNumOfAccounts,
		accountsDriveStartIndex, builder.Hex()))
}

func TestAccountsDriveStartIndex_MemoryRanges_ReturnsStartIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name:   "flash drive without nvram field",
			config: `{"config":{"flash_drive":[{"start":16777216,"length":4194304}]}}`,
		},
		{
			name:   "nvram without flash drive field",
			config: `{"config":{"nvram":[{"start":16777216,"length":4194304}]}}`,
		},
		{
			name:   "nvram with empty flash drive list",
			config: `{"config":{"flash_drive":[],"nvram":[{"start":16777216,"length":4194304}]}}`,
		},
		{
			name: "nvram with unrelated flash drive",
			config: `{"config":{
				"flash_drive":[{"start":33554432,"length":8388608}],
				"nvram":[{"start":16777216,"length":4194304}]
			}}`,
		},
		{
			name: "ambiguous flash and nvram accounts candidates",
			config: `{"config":{
				"flash_drive":[{"start":33554432,"length":4194304}],
				"nvram":[{"start":16777216,"length":4194304}]
			}}`,
			wantErr: "expected exactly one accounts-sized range, found 2",
		},
		{
			name: "ambiguous nvram accounts candidates",
			config: `{"config":{"nvram":[
				{"start":16777216,"length":4194304},
				{"start":33554432,"length":4194304}
			]}}`,
			wantErr: "expected exactly one accounts-sized range, found 2",
		},
		{
			name:    "missing accounts candidate",
			config:  `{"config":{"flash_drive":[],"nvram":[]}}`,
			wantErr: "expected exactly one accounts-sized range, found 0",
		},
		{
			name:    "misaligned nvram accounts candidate",
			config:  `{"config":{"nvram":[{"start":4096,"length":4194304}]}}`,
			wantErr: "accounts drive start must be aligned to drive size",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			template := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(template, "config.json"), []byte(tt.config), 0600))

			if tt.wantErr != "" {
				cfg := readWithdrawalMachineConfig(t, template)
				_, err := accountsDriveStartIndexFromConfig(cfg)
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.Equal(t, uint64(4), accountsDriveStartIndex(t, template))
		})
	}
}

type withdrawalMemoryRange struct {
	Start  uint64 `json:"start"`
	Length uint64 `json:"length"`
}

type withdrawalMachineConfig struct {
	Config struct {
		FlashDrive []withdrawalMemoryRange `json:"flash_drive"`
		NVRAM      []withdrawalMemoryRange `json:"nvram"`
	} `json:"config"`
}

func readWithdrawalMachineConfig(t testing.TB, machinePath string) withdrawalMachineConfig {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(machinePath, "config.json"))
	require.NoError(t, err, "read ERC-20 withdrawal machine config")
	var cfg withdrawalMachineConfig
	require.NoError(t, json.Unmarshal(raw, &cfg), "parse ERC-20 withdrawal machine config")
	return cfg
}

func accountsDriveStartIndex(t testing.TB, templatePath string) uint64 {
	t.Helper()
	index, err := accountsDriveStartIndexFromConfig(readWithdrawalMachineConfig(t, templatePath))
	require.NoError(t, err, "identify accounts drive before deployment")
	return index
}

func accountsDriveStartIndexFromConfig(cfg withdrawalMachineConfig) (uint64, error) {
	var start uint64
	var matches int
	for _, drives := range [][]withdrawalMemoryRange{cfg.Config.FlashDrive, cfg.Config.NVRAM} {
		for _, drive := range drives {
			if drive.Length == accountsDriveSize {
				start = drive.Start
				matches++
			}
		}
	}
	if matches != 1 {
		return 0, fmt.Errorf("expected exactly one accounts-sized range, found %d", matches)
	}
	if start%accountsDriveSize != 0 {
		return 0, errors.New("accounts drive start must be aligned to drive size")
	}
	return start >> accountsDriveLog2Size, nil
}

func requireAccountsDriveRange(t testing.TB, machinePath string, kind accountsDriveKind, wc model.WithdrawalConfig) {
	t.Helper()
	cfg := readWithdrawalMachineConfig(t, machinePath)
	var ranges []withdrawalMemoryRange
	switch kind {
	case accountsDriveFlash:
		ranges = cfg.Config.FlashDrive
	case accountsDriveNVRAM:
		ranges = cfg.Config.NVRAM
	default:
		require.FailNowf(t, "unknown accounts drive kind", "unknown kind %s", kind)
	}
	log2DriveSize := accountsDriveLog2AccountSize + wc.Log2MaxNumOfAccounts + wc.Log2LeavesPerAccount
	driveSize := uint64(1) << log2DriveSize
	driveStart := wc.AccountsDriveStartIndex << log2DriveSize
	var matches int
	for _, memoryRange := range ranges {
		if memoryRange.Start == driveStart && memoryRange.Length >= driveSize {
			matches++
		}
	}
	require.Equal(t, 1, matches, "%s must contain the configured accounts range in %s at 0x%x", machinePath, kind, driveStart)
}

func devnetAddress(t testing.TB, key, fallback string) common.Address {
	t.Helper()
	value := envOrDefault(key, fallback)
	require.True(t, common.IsHexAddress(value), "%s must be an Ethereum address", key)
	return common.HexToAddress(value)
}

func mnemonicAddress(t testing.TB, index uint32) common.Address {
	t.Helper()
	key, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, index)
	require.NoError(t, err, "derive mnemonic[%d] key", index)
	return crypto.PubkeyToAddress(key.PublicKey)
}

func runMachineTool(ctx context.Context, t testing.TB, args ...string) string {
	t.Helper()
	cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, machineToolBinary, args...)
	cmd.Env = os.Environ()
	workDir := machineToolWorkDir(t)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		require.NoError(t, err, "%s %v failed: %s", machineToolBinary, args, stderr)
	}
	return string(out)
}

func machineToolWorkDir(t testing.TB) string {
	t.Helper()
	artifactsDir := os.Getenv("CARTESI_TEST_ARTIFACTS_DIR")
	if artifactsDir == "" {
		var err error
		artifactsDir, err = integrationArtifactsDir()
		require.NoError(t, err, "prepare integration artifacts dir")
		os.Setenv("CARTESI_TEST_ARTIFACTS_DIR", artifactsDir)
	}

	workDir := filepath.Join(artifactsDir, "machine-tool")
	require.NoError(t, os.MkdirAll(workDir, 0755), "create machine-tool artifact dir") //nolint:mnd
	return workDir
}

func listWithdrawals(
	ctx context.Context,
	appName string,
	accountIndex string,
) (*api.ListResponse[*model.Withdrawal], error) {
	accountIndexCopy := accountIndex
	req := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
		ID      int    `json:"id"`
	}{
		JSONRPC: "2.0",
		Method:  "cartesi_listWithdrawals",
		Params: api.ListWithdrawalsParams{
			Application:  appName,
			AccountIndex: &accountIndexCopy,
			Limit:        10,
		},
		ID: 1,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := envOrDefault("CARTESI_JSONRPC_API_URL", "http://localhost:10011/rpc")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := anvilHTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rpcResp struct {
		Result *api.ListResponse[*model.Withdrawal] `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("cartesi_listWithdrawals error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if rpcResp.Result == nil {
		return nil, fmt.Errorf("cartesi_listWithdrawals returned no result")
	}
	return rpcResp.Result, nil
}
