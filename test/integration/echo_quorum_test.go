// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	quorumClaimStagingPeriod uint64 = 8
	quorumNodeValidatorIndex uint32 = 0
	quorumValidatorIndexA    uint32 = 2
	quorumValidatorIndexB    uint32 = 3
)

type quorumAppDeployment struct {
	appName          string
	appAddress       common.Address
	consensusAddress common.Address
	quorum           *iquorum.IQuorum
}

// EchoQuorumSuite covers the Authority-like happy path plus Quorum-specific
// voting order and minority/majority divergence cases. The non-node validators
// are direct SubmitClaim calls signed with Foundry mnemonic account indexes 2
// and 3; no extra node processes are needed.
type EchoQuorumSuite struct {
	suite.Suite
	LogChecker
	ctx     context.Context
	cancel  context.CancelFunc
	client  *ethclient.Client
	chainID *big.Int
	appName string
}

func TestEchoQuorum(t *testing.T) {
	suite.Run(t, new(EchoQuorumSuite))
}

func (s *EchoQuorumSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 30*time.Minute)

	endpoint := envOrDefault("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	client, err := ethclient.DialContext(s.ctx, endpoint)
	s.Require().NoError(err, "dial ethclient")
	s.client = client

	chainID, err := client.ChainID(s.ctx)
	s.Require().NoError(err, "fetch chain id")
	s.chainID = chainID
}

func (s *EchoQuorumSuite) TearDownSuite() {
	if s.client != nil {
		s.client.Close()
	}
	s.cancel()
}

func (s *EchoQuorumSuite) SetupTest() {
	s.StartLogCapture()
	s.appName = ""
}

func (s *EchoQuorumSuite) TearDownTest() {
	if s.appName != "" {
		s.T().Logf("Disabling application %s", s.appName)
		if err := disableApplication(s.ctx, s.appName); err != nil {
			s.T().Errorf("failed to disable application %s: %v", s.appName, err)
		}
	}
	s.CheckLogs(s.T())
}

func (s *EchoQuorumSuite) TestEchoQuorumLifecycle() {
	r := s.Require()

	app := s.deployQuorumEchoApp("echo-quorum-lifecycle")
	epoch := s.prepareQuorumEpoch(app.appName, "hello cartesi (quorum lifecycle)")

	outputsResp, err := readOutputs(s.ctx, app.appName)
	r.NoError(err, "read quorum lifecycle outputs")
	r.Equal(uint64(echoOutputsPerInput), outputsResp.Pagination.TotalCount,
		"expected %d outputs (voucher + delegatecall voucher + notice)", echoOutputsPerInput)
	r.Len(outputsResp.Data, echoOutputsPerInput)

	var voucherIdx, noticeIdx uint64
	voucherFound, delegateVoucherFound, noticeFound := false, false, false
	for _, out := range outputsResp.Data {
		r.Equal(epoch.Index, out.EpochIndex, "output %d should belong to quorum lifecycle epoch", out.Index)
		if out.DecodedData == nil {
			continue
		}
		switch out.DecodedData.Type {
		case "Voucher":
			voucherIdx = out.Index
			voucherFound = true
		case "DelegateCallVoucher":
			delegateVoucherFound = true
		case "Notice":
			noticeIdx = out.Index
			noticeFound = true
		}
	}
	r.True(voucherFound, "voucher output not found")
	r.True(delegateVoucherFound, "delegate call voucher output not found")
	r.True(noticeFound, "notice output not found")

	reportsResp, err := readReports(s.ctx, app.appName)
	r.NoError(err, "read quorum lifecycle reports")
	r.Equal(uint64(echoReportsPerInput), reportsResp.Pagination.TotalCount,
		"expected %d report(s)", echoReportsPerInput)

	submittedCtx, submittedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	epoch, err = waitForEpochStatus(submittedCtx, s.T(), app.appName, epoch.Index, model.EpochStatus_ClaimSubmitted)
	submittedCancel()
	r.NoError(err, "wait for node to submit quorum claim")

	s.submitQuorumClaim(app, epoch, quorumValidatorIndexA, *epoch.OutputsMerkleRoot)
	s.submitQuorumClaim(app, epoch, quorumValidatorIndexB, *epoch.OutputsMerkleRoot)
	s.waitForQuorumAccepted(app.appName, epoch.Index)

	verifyClaimAndExecute(s.ctx, s.T(), r, verifyAndExecuteConfig{
		AppName:          app.appName,
		EpochIndex:       epoch.Index,
		EpochOutputs:     outputsResp.Data,
		VoucherIdx:       voucherIdx,
		NoticeIdx:        noticeIdx,
		CheckReExecution: true,
	})
}

func (s *EchoQuorumSuite) TestNodeVoteFirstThenOtherValidatorsStageAndAccept() {
	app := s.deployQuorumEchoApp("echo-quorum-node-first")
	epoch := s.prepareQuorumEpoch(app.appName, "hello cartesi (quorum node first)")

	submittedCtx, submittedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	epoch, err := waitForEpochStatus(submittedCtx, s.T(), app.appName, epoch.Index, model.EpochStatus_ClaimSubmitted)
	submittedCancel()
	s.Require().NoError(err, "wait for node to submit quorum claim")

	s.submitQuorumClaim(app, epoch, quorumValidatorIndexA, *epoch.OutputsMerkleRoot)
	s.submitQuorumClaim(app, epoch, quorumValidatorIndexB, *epoch.OutputsMerkleRoot)

	s.waitForQuorumAccepted(app.appName, epoch.Index)
}

func (s *EchoQuorumSuite) TestExternalValidatorThenNodeVoteStagesAndAccepts() {
	if !isNodeSelfManaged() {
		s.T().Skip("skipping: validator-order test requires test-managed node to slow claimer polling")
	}
	s.SetExpectedLogs(s.T(), ExpectedLog{
		Pattern: regexp.MustCompile(`service=.*context canceled`),
		Level:   LevelError,
		Reason:  "benign shutdown noise from restarting the shared node with different claimer polling",
	})

	stopSharedNode(s.T())
	startSharedNodeWithEnv(s.T(), "CARTESI_CLAIMER_POLLING_INTERVAL=3600")
	slowClaimer := true
	defer func() {
		if slowClaimer {
			stopSharedNode(s.T())
			startSharedNode(s.T())
		}
	}()

	app := s.deployQuorumEchoApp("echo-quorum-node-second")
	epoch := s.prepareQuorumEpoch(app.appName, "hello cartesi (quorum node second)")
	s.Require().Equal(model.EpochStatus_ClaimComputed, epoch.Status,
		"node should compute the claim before the slowed claimer polling interval submits it")

	s.submitQuorumClaim(app, epoch, quorumValidatorIndexA, *epoch.OutputsMerkleRoot)

	stopSharedNode(s.T())
	startSharedNode(s.T())
	slowClaimer = false

	s.waitForQuorumAccepted(app.appName, epoch.Index)
}

func (s *EchoQuorumSuite) TestExternalMajorityStagesBeforeNodeVoteThenNodeAccepts() {
	if !isNodeSelfManaged() {
		s.T().Skip("skipping: external-majority test requires test-managed node to slow claimer polling")
	}
	s.SetExpectedLogs(s.T(), ExpectedLog{
		Pattern: regexp.MustCompile(`service=.*context canceled`),
		Level:   LevelError,
		Reason:  "benign shutdown noise from restarting the shared node with different claimer polling",
	})

	stopSharedNode(s.T())
	startSharedNodeWithEnv(s.T(), "CARTESI_CLAIMER_POLLING_INTERVAL=3600")
	slowClaimer := true
	defer func() {
		if slowClaimer {
			stopSharedNode(s.T())
			startSharedNode(s.T())
		}
	}()

	app := s.deployQuorumEchoApp("echo-quorum-external-majority")
	epoch := s.prepareQuorumEpoch(app.appName, "hello cartesi (quorum external majority)")
	s.Require().Equal(model.EpochStatus_ClaimComputed, epoch.Status,
		"node should compute the claim before the slowed claimer polling interval submits it")

	s.submitQuorumClaim(app, epoch, quorumValidatorIndexA, *epoch.OutputsMerkleRoot)
	s.submitQuorumClaim(app, epoch, quorumValidatorIndexB, *epoch.OutputsMerkleRoot)

	stopSharedNode(s.T())
	startSharedNode(s.T())
	slowClaimer = false

	s.waitForQuorumAccepted(app.appName, epoch.Index)
}

func (s *EchoQuorumSuite) TestDivergentMinorityVoteDoesNotBlockAcceptance() {
	app := s.deployQuorumEchoApp("echo-quorum-divergent-minority")
	epoch := s.prepareQuorumEpoch(app.appName, "hello cartesi (quorum divergent minority)")

	submittedCtx, submittedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	epoch, err := waitForEpochStatus(submittedCtx, s.T(), app.appName, epoch.Index, model.EpochStatus_ClaimSubmitted)
	submittedCancel()
	s.Require().NoError(err, "wait for node to submit quorum claim")

	divergentOutputs := randomOutputsMerkleRoot(s.T(), *epoch.OutputsMerkleRoot)
	s.submitQuorumClaim(app, epoch, quorumValidatorIndexA, *epoch.OutputsMerkleRoot)
	s.submitQuorumClaim(app, epoch, quorumValidatorIndexB, divergentOutputs)

	s.waitForQuorumAccepted(app.appName, epoch.Index)
}

func (s *EchoQuorumSuite) TestDivergentMajorityMarksApplicationInoperable() {
	s.SetExpectedLogs(s.T(),
		ExpectedLog{
			Pattern: regexp.MustCompile(`marking application as diverged.*quorum_divergence_at_staging`),
			Level:   LevelError,
			Reason:  "expected DIVERGED transition after a divergent Quorum majority stages a different claim",
		},
		ExpectedLog{
			Pattern: regexp.MustCompile(`Tick service=claimer.*quorum_divergence_at_staging`),
			Level:   LevelError,
			Reason:  "claimer Tick wraps and re-logs the divergence-induced DIVERGED error",
		},
	)

	app := s.deployQuorumEchoApp("echo-quorum-outvoted")
	epoch := s.prepareQuorumEpoch(app.appName, "hello cartesi (quorum outvoted)")

	submittedCtx, submittedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	epoch, err := waitForEpochStatus(submittedCtx, s.T(), app.appName, epoch.Index, model.EpochStatus_ClaimSubmitted)
	submittedCancel()
	s.Require().NoError(err, "wait for node to submit quorum claim")

	divergentOutputs := randomOutputsMerkleRoot(s.T(), *epoch.OutputsMerkleRoot)
	s.submitQuorumClaim(app, epoch, quorumValidatorIndexA, divergentOutputs)
	s.submitQuorumClaim(app, epoch, quorumValidatorIndexB, divergentOutputs)

	rejectedCtx, rejectedCancel := context.WithTimeout(s.ctx, 5*time.Minute)
	epoch, err = waitForEpochStatus(rejectedCtx, s.T(), app.appName, epoch.Index, model.EpochStatus_ClaimRejected)
	rejectedCancel()
	s.Require().NoError(err, "wait for outvoted quorum epoch to become CLAIM_REJECTED")

	stateCtx, stateCancel := context.WithTimeout(s.ctx, time.Minute)
	err = waitForApplicationStatus(stateCtx, s.T(), app.appName, "DIVERGED")
	stateCancel()
	s.Require().NoError(err, "wait for outvoted quorum app to become DIVERGED")

	status, err := readApplicationStatus(s.ctx, app.appName)
	s.Require().NoError(err, "read app status after quorum divergence")
	s.Require().Contains(status, "quorum_divergence_at_staging")

	// DIVERGED is terminal, and disableApplication rejects terminal states.
	s.appName = ""
}

func (s *EchoQuorumSuite) TestForecloseQuorumClaimBeforeAcceptanceMarksClaimForeclosed() {
	r := s.Require()

	builderEnv := os.Getenv("CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS")
	r.NotEmpty(builderEnv,
		"CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS must be set (run `eval $(make env)`)")

	const guardianIndex uint32 = 1
	guardianKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, guardianIndex)
	r.NoError(err, "derive guardian key")
	guardianAddr := crypto.PubkeyToAddress(guardianKey.PublicKey)

	withdrawalConfigJSON := fmt.Sprintf(`{
		"guardian": "%s",
		"log2_leaves_per_account": 0,
		"log2_max_num_of_accounts": 20,
		"accounts_drive_start_index": 33554432,
		"withdrawal_output_builder": "%s"
	}`, guardianAddr.Hex(), builderEnv)
	withdrawalConfigJSON = strings.ReplaceAll(withdrawalConfigJSON, "\n", "")
	withdrawalConfigJSON = strings.ReplaceAll(withdrawalConfigJSON, "\t", "")

	app := s.deployQuorumEchoApp("foreclose-quorum", "--withdrawal-config", withdrawalConfigJSON)
	epoch := s.prepareQuorumEpoch(app.appName, "hello cartesi (foreclose quorum)")

	submittedCtx, submittedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	epoch, err = waitForEpochStatus(submittedCtx, s.T(), app.appName, epoch.Index, model.EpochStatus_ClaimSubmitted)
	submittedCancel()
	r.NoError(err, "wait for node to submit quorum claim")

	out, err := runCLIWithEnv(s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", guardianIndex)},
		"foreclose", app.appName, "--yes", "--json",
	)
	r.NoError(err, "guardian foreclose CLI call: %s", out)
	s.T().Logf("    foreclose tx: %s", strings.TrimSpace(out))

	stateCtx, stateCancel := context.WithTimeout(s.ctx, 30*time.Second)
	err = waitForApplicationForeclosed(stateCtx, s.T(), app.appName)
	stateCancel()
	r.NoError(err, "app did not record foreclose_block after guardian foreclose()")

	foreclosedCtx, foreclosedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	epoch, err = waitForEpochStatus(foreclosedCtx, s.T(), app.appName, epoch.Index, model.EpochStatus_ClaimForeclosed)
	foreclosedCancel()
	r.NoError(err, "foreclosed quorum claim should become CLAIM_FORECLOSED instead of stalling in CLAIM_SUBMITTED")
	r.NotNil(epoch.OutputsMerkleRoot, "local claim data should be preserved when terminalizing as CLAIM_FORECLOSED")

	// Ordinary foreclosure keeps the app's health status OK, keeps it
	// enabled for L1 observation, and surfaces the marker in `app status`.
	status, err := readApplicationStatus(s.ctx, app.appName)
	r.NoError(err, "read app status after foreclosure")
	r.Equal("OK", firstStatusLine(status),
		"ordinary foreclosure keeps health OK; foreclosure is recorded in foreclose_block")
	r.Contains(status, "Enabled: true")
	r.NotContains(status, "DIVERGED",
		"foreclosure must not transition the app to a terminal health status (DIVERGED/CORRUPTED)")
	r.Contains(status, "Foreclose block:")
	r.Contains(status, "Foreclose transaction:")
}

func (s *EchoQuorumSuite) TestForecloseQuorumOutputExecutionAfterForeclosureIsRecorded() {
	r := s.Require()

	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)

	app := s.deployQuorumEchoApp("foreclose-quorum-output", "--withdrawal-config", withdrawalConfigJSON)
	epoch := s.prepareQuorumEpoch(app.appName, "hello cartesi (foreclose quorum output)")

	outputsResp, err := readOutputs(s.ctx, app.appName)
	r.NoError(err, "read outputs")
	r.Len(outputsResp.Data, echoOutputsPerInput)
	voucherIdx := firstVoucherOutputIndex(s.T(), outputsResp.Data)

	submittedCtx, submittedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	epoch, err = waitForEpochStatus(submittedCtx, s.T(), app.appName, epoch.Index, model.EpochStatus_ClaimSubmitted)
	submittedCancel()
	r.NoError(err, "wait for node to submit quorum claim")

	s.submitQuorumClaim(app, epoch, quorumValidatorIndexA, *epoch.OutputsMerkleRoot)
	s.submitQuorumClaim(app, epoch, quorumValidatorIndexB, *epoch.OutputsMerkleRoot)
	s.waitForQuorumAccepted(app.appName, epoch.Index)

	r.NoError(guardianForeclose(s.ctx, app.appName, guardianIndex), "guardian foreclose")
	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), app.appName),
		"node did not record quorum foreclosure")
	forecloseCancel()

	txHash, err := executeOutput(s.ctx, app.appName, voucherIdx)
	r.NoError(err, "execute accepted quorum voucher after foreclosure")
	r.NotEmpty(txHash)

	execCtx, execCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	err = waitForExecutionRecorded(execCtx, s.T(), app.appName, voucherIdx)
	execCancel()
	r.NoError(err, "wait for post-foreclosure quorum output execution in DB")
}

func (s *EchoQuorumSuite) TestForecloseQuorumWithoutInputs() {
	r := s.Require()

	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)

	app := s.deployQuorumEchoApp("foreclose-quorum-no-input", "--withdrawal-config", withdrawalConfigJSON)

	r.NoError(guardianForeclose(s.ctx, app.appName, guardianIndex),
		"guardian should be able to foreclose a Quorum app with no inputs")

	stateCtx, stateCancel := context.WithTimeout(s.ctx, 30*time.Second)
	err := waitForApplicationForeclosed(stateCtx, s.T(), app.appName)
	stateCancel()
	r.NoError(err, "app did not record foreclose_block after guardian foreclose()")

	status, err := readApplicationStatus(s.ctx, app.appName)
	r.NoError(err, "read app status after no-input quorum foreclosure")
	r.Equal("OK", firstStatusLine(status),
		"ordinary no-input quorum foreclosure keeps health OK; foreclosure is recorded in foreclose_block")
	r.Contains(status, "Enabled: true")
	r.Contains(status, "Foreclose block:")

	_, err = readInput(s.ctx, app.appName, 0)
	r.Error(err, "no-input quorum foreclosure should not create synthetic inputs")
	r.True(isCLIExitError(err), "missing input should be reported by the CLI")
}

func (s *EchoQuorumSuite) deployQuorumEchoApp(prefix string, extraApplicationArgs ...string) quorumAppDeployment {
	r := s.Require()

	validators := quorumValidatorAddresses(s.T())
	quorumArgs := []string{
		"deploy", "quorum",
		"--json",
		"--salt", uniqueSalt(),
		"--claim-staging-period", strconv.FormatUint(quorumClaimStagingPeriod, 10),
	}
	for _, validator := range validators {
		quorumArgs = append(quorumArgs, "--validator", validator.Hex())
	}

	out, err := runCLI(s.ctx, quorumArgs...)
	r.NoError(err, "deploy quorum")

	var quorumDeployment struct {
		Address string `json:"address"`
	}
	r.NoError(json.Unmarshal([]byte(out), &quorumDeployment), "parse quorum deployment")
	r.NotEmpty(quorumDeployment.Address, "quorum deployment missing address")

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	appName := uniqueAppName(prefix)
	applicationArgs := []string{
		"--consensus", quorumDeployment.Address,
		"--salt", uniqueSalt(),
	}
	applicationArgs = append(applicationArgs, extraApplicationArgs...)
	appAddrStr, consensusAddrStr, err := deployApplicationWithConsensus(
		s.ctx,
		appName,
		dappPath,
		applicationArgs...,
	)
	r.NoError(err, "deploy quorum echo application")
	r.Equal(common.HexToAddress(quorumDeployment.Address), common.HexToAddress(consensusAddrStr),
		"application must use the freshly deployed quorum consensus")

	r.NoError(anvilSetBalance(s.ctx, appAddrStr, oneEtherWei), "fund application contract")

	quorumBinding, err := iquorum.NewIQuorum(common.HexToAddress(consensusAddrStr), s.client)
	r.NoError(err, "bind quorum consensus")

	s.appName = appName
	return quorumAppDeployment{
		appName:          appName,
		appAddress:       common.HexToAddress(appAddrStr),
		consensusAddress: common.HexToAddress(consensusAddrStr),
		quorum:           quorumBinding,
	}
}

func (s *EchoQuorumSuite) prepareQuorumEpoch(appName string, payload string) *model.Epoch {
	r := s.Require()

	inputIndex, blockNum, err := sendInput(s.ctx, appName, payload)
	r.NoError(err, "send input")
	r.Equal(uint64(0), inputIndex)
	s.T().Logf("    quorum input accepted on-chain: index=%d block=%d", inputIndex, blockNum)

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	input, err := waitForInputProcessed(processCtx, s.T(), appName, inputIndex)
	processCancel()
	r.NoError(err, "wait for quorum input processing")
	r.Equal(model.InputCompletionStatus_Accepted, input.Status)

	epoch := s.waitForEpochAvailable(appName, input.EpochIndex)
	s.minePastBlock(epoch.LastBlock)
	return s.waitForEpochWithClaim(appName, input.EpochIndex)
}

func (s *EchoQuorumSuite) waitForEpochAvailable(appName string, epochIndex uint64) *model.Epoch {
	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()

	var result *model.Epoch
	var lastErr error
	err := pollUntil(ctx, 2*time.Second, func() (bool, error) {
		epoch, err := readEpoch(ctx, appName, epochIndex)
		if err != nil {
			if isCLIExitError(err) {
				lastErr = err
				s.T().Logf("poll epoch %d: %v (retrying)", epochIndex, err)
				return false, nil
			}
			return false, fmt.Errorf("poll epoch %d: %w", epochIndex, err)
		}
		result = epoch
		return true, nil
	})
	if err != nil && lastErr != nil {
		err = fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	s.Require().NoError(err, "wait for epoch %d to exist", epochIndex)
	return result
}

func (s *EchoQuorumSuite) waitForEpochWithClaim(appName string, epochIndex uint64) *model.Epoch {
	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
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
		if epoch.OutputsMerkleRoot != nil && epoch.MachineHash != nil && isQuorumClaimReadyStatus(epoch.Status) {
			result = epoch
			return true, nil
		}
		s.T().Logf("    waiting for quorum claim for epoch %d (status=%s)", epochIndex, epoch.Status)
		return false, nil
	})
	if err != nil && lastErr != nil {
		err = fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	s.Require().NoError(err, "wait for epoch %d claim computation", epochIndex)
	return result
}

func isQuorumClaimReadyStatus(status model.EpochStatus) bool {
	switch status {
	case model.EpochStatus_ClaimComputed,
		model.EpochStatus_ClaimSubmitted,
		model.EpochStatus_ClaimStaged,
		model.EpochStatus_ClaimAccepted:
		return true
	default:
		return false
	}
}

func (s *EchoQuorumSuite) waitForQuorumAccepted(appName string, epochIndex uint64) {
	stagedCtx, stagedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	staged, err := waitForEpochStatus(stagedCtx, s.T(), appName, epochIndex, model.EpochStatus_ClaimStaged)
	stagedCancel()
	s.Require().NoError(err, "wait for quorum claim to stage")

	if staged.StagedAtBlock != nil {
		s.minePastBlock(*staged.StagedAtBlock + quorumClaimStagingPeriod)
	} else {
		s.Require().NoError(anvilMine(s.ctx, int(quorumClaimStagingPeriod)+1), "mine past claim staging period")
	}

	acceptedCtx, acceptedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(acceptedCtx, s.T(), appName, epochIndex, model.EpochStatus_ClaimAccepted)
	acceptedCancel()
	s.Require().NoError(err, "wait for quorum claim to be accepted")
}

func (s *EchoQuorumSuite) minePastBlock(block uint64) {
	currentBlock, err := s.client.BlockNumber(s.ctx)
	s.Require().NoError(err, "read current block")
	if currentBlock > block {
		return
	}
	blocksToMine := int(block - currentBlock + 1)
	s.Require().NoError(anvilMine(s.ctx, blocksToMine), "mine past block %d", block)
}

func (s *EchoQuorumSuite) submitQuorumClaim(
	app quorumAppDeployment,
	epoch *model.Epoch,
	accountIndex uint32,
	outputsMerkleRoot [32]byte,
) common.Hash {
	r := s.Require()
	r.NotNil(epoch.OutputsMerkleRoot, "epoch %d missing outputs merkle root", epoch.Index)

	key, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, accountIndex)
	r.NoError(err, "derive validator key %d", accountIndex)

	opts, err := bind.NewKeyedTransactorWithChainID(key, s.chainID)
	r.NoError(err, "new validator transactor %d", accountIndex)
	opts.Context = s.ctx

	tx, err := app.quorum.SubmitClaim(
		opts,
		app.appAddress,
		new(big.Int).SetUint64(epoch.LastBlock),
		outputsMerkleRoot,
		merkleProofToBytes32(epoch.OutputsMerkleProof),
	)
	r.NoError(err, "validator %d submit quorum claim", accountIndex)

	receipt, err := bind.WaitMined(s.ctx, s.client, tx)
	r.NoError(err, "wait for validator %d quorum submit tx", accountIndex)
	r.Equal(uint64(1), receipt.Status, "validator %d quorum submit tx must succeed", accountIndex)
	s.T().Logf("    validator mnemonic[%d] submitClaim mined in block %d tx=%s",
		accountIndex, receipt.BlockNumber.Uint64(), tx.Hash().Hex())
	return tx.Hash()
}

func quorumValidatorAddresses(t testing.TB) []common.Address {
	t.Helper()
	indexes := []uint32{quorumNodeValidatorIndex, quorumValidatorIndexA, quorumValidatorIndexB}
	addresses := make([]common.Address, 0, len(indexes))
	for _, index := range indexes {
		key, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, index)
		require.NoError(t, err, "derive validator key %d", index)
		addresses = append(addresses, crypto.PubkeyToAddress(key.PublicKey))
	}
	return addresses
}

func randomOutputsMerkleRoot(t testing.TB, legitimate common.Hash) [32]byte {
	t.Helper()
	for {
		outputs := randomBytes32(t)
		if common.Hash(outputs) != legitimate {
			return outputs
		}
	}
}
