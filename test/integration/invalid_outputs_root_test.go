// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"math/big"
	"regexp"
	"strconv"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	jsonrpcclient "github.com/cartesi/rollups-node/pkg/jsonrpc/client"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// The public API does not expose the execution hash collection. Check its
// persisted rows directly while the proof and completion are checked via API.
func (s *TerminalMachineStatesSuite) terminalStateHashes(input *model.Input) []model.EpochStateHash {
	s.T().Helper()
	r := s.Require()
	dsn, err := config.GetDatabaseConnection()
	r.NoError(err)
	repo, err := factory.NewRepositoryFromConnectionString(s.ctx, dsn.Raw())
	r.NoError(err)
	defer repo.Close()
	app, err := repo.GetApplication(s.ctx, s.appName)
	r.NoError(err)
	var rows []model.EpochStateHash
	var cursor *uint64
	const pageSize = 10_000
	for {
		page, err := repo.ListEpochStateHashes(s.ctx, app.ID, input.EpochIndex, cursor, pageSize)
		r.NoError(err)
		rows = append(rows, page...)
		if len(page) < pageSize {
			break
		}
		last := page[len(page)-1].Index
		cursor = &last
	}
	r.NotEmpty(rows)
	var span uint64
	for _, row := range rows {
		r.Equal(input.Index, row.InputIndex)
		span += row.Repetitions
	}
	r.Equal(model.InputHashCollectionCapacity, span)
	r.NotNil(input.MachineHash)
	r.Equal(model.FinalStateEntry(*input.MachineHash), rows[len(rows)-1].MachineHash)
	return rows
}

func (s *TerminalMachineStatesSuite) TestInvalidOutputsRootLengthSurvivesRestart() {
	s.runInvalidOutputsRootLength(false)
}

func (s *TerminalMachineStatesSuite) TestInvalidOutputsRootLengthPrtSurvivesRestart() {
	s.runInvalidOutputsRootLength(true)
}

func (s *TerminalMachineStatesSuite) runInvalidOutputsRootLength(prt bool) {
	s.T().Helper()
	tc := terminalMachineStateCase{
		namePrefix:        "invalid-root-length",
		dappPathEnv:       "CARTESI_TEST_INVALID_OUTPUTS_ROOT_LENGTH_DAPP_PATH",
		defaultDappPath:   "applications/invalid-outputs-root-length-dapp",
		payloadPrefix:     "invalid-root-length",
		description:       "a guest that declares a 31-byte root in an accepted yield",
		inputStatus:       model.InputCompletionStatus_InvalidOutputsRoot,
		applicationStatus: model.ApplicationStatus_InvalidOutputsRoot,
		reasonSuffix:      ": an accepted yield must declare exactly 32 bytes",
	}
	if prt {
		tc.deployArgs = []string{prtFlag}
	}
	s.runTerminalMachineState(tc)
}

func (s *TerminalMachineStatesSuite) TestInvalidOutputsRootValue() {
	s.appName = uniqueAppName("invalid-root-value")
	dappPath := envOrDefault("CARTESI_TEST_INVALID_OUTPUTS_ROOT_RECOVERY_DAPP_PATH",
		"applications/invalid-outputs-root-recovery-dapp")
	appAddress, consensusAddress, err := deployApplicationWithConsensus(s.ctx, s.appName, dappPath,
		terminalFixtureSaltFlag, uniqueSalt())
	s.Require().NoError(err)
	index, _, err := sendInput(s.ctx, s.appName, "invalid-root-value")
	s.Require().NoError(err)
	s.Require().Zero(index)
	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()
	input, err := waitForInputProcessed(ctx, s.T(), s.appName, index)
	s.Require().NoError(err)
	s.Require().Equal(model.InputCompletionStatus_Accepted, input.Status,
		"a later validator finding must not rewrite the completed input")
	minePastEpochBoundary(ctx, s.T(), s.Require(), s.appName, input.EpochIndex)
	invalid := s.requireComputedInvalidOutputsRoot(ctx, input.EpochIndex, 1)
	client := newIntegrationEthClient(ctx, s.T())
	defer client.Close()
	consensus, err := iconsensus.NewIConsensus(common.HexToAddress(consensusAddress), client)
	s.Require().NoError(err)
	assertUnstaged := func(ctx context.Context) {
		claim, readErr := consensus.GetClaim(&bind.CallOpts{Context: ctx}, common.HexToAddress(appAddress),
			new(big.Int).SetUint64(invalid.LastBlock), *invalid.MachineHash)
		s.Require().NoError(readErr)
		s.Require().Zero(claim.Status, "the IOR claim must remain UNSTAGED on chain")
	}
	observeIORPublicationWindow(ctx, s.T(), assertUnstaged)

	// The same runtime corrects the declaration on its next input. Actual
	// healthy publication also proves that the publisher and signer are active.
	nextIndex, _, err := sendInput(ctx, s.appName, "corrected-root-value")
	s.Require().NoError(err)
	s.Require().Equal(index+1, nextIndex)
	next, err := waitForInputProcessed(ctx, s.T(), s.appName, nextIndex)
	s.Require().NoError(err)
	s.Require().Equal(model.InputCompletionStatus_Accepted, next.Status)
	s.Require().Greater(next.EpochIndex, invalid.Index)
	minePastEpochBoundary(ctx, s.T(), s.Require(), s.appName, next.EpochIndex)
	healthy, err := waitForEpochStatus(ctx, s.T(), s.appName, next.EpochIndex, model.EpochStatus_ClaimAccepted)
	s.Require().NoError(err)
	s.Require().True(healthy.HasMatchingOutputsRoot())
	claim, err := consensus.GetClaim(&bind.CallOpts{Context: ctx}, common.HexToAddress(appAddress),
		new(big.Int).SetUint64(healthy.LastBlock), *healthy.MachineHash)
	s.Require().NoError(err)
	s.Require().Equal(uint8(2), claim.Status, "the later healthy claim must be ACCEPTED on chain")
	s.Require().Equal(*healthy.TxBufferDataBlock, common.Hash(claim.StagedOutputsMerkleRoot))
	assertUnstaged(ctx)
	stored, err := readEpoch(ctx, s.appName, invalid.Index)
	s.Require().NoError(err)
	s.Require().Equal(model.EpochStatus_ClaimComputed, stored.Status)
	s.Require().Nil(stored.ClaimTransactionHash)
	s.Require().True(stored.HasInvalidOutputsRoot())
	rpc := jsonrpcclient.NewClient(envOrDefault("CARTESI_JSONRPC_API_URL", "http://localhost:10011/rpc"))
	s.Require().Equal(model.ApplicationStatus_OK, s.getApplication(rpc).Status)
}

func (s *TerminalMachineStatesSuite) TestInvalidTemplateOutputsRootInFirstPrtEpoch() {
	s.appName = uniqueAppName("invalid-template-root")
	dappPath := envOrDefault("CARTESI_TEST_INVALID_TEMPLATE_OUTPUTS_ROOT_DAPP_PATH",
		"applications/invalid-template-outputs-root-dapp")
	appAddress, consensusAddress, err := deployApplicationWithConsensus(s.ctx, s.appName, dappPath,
		terminalFixtureSaltFlag, uniqueSalt(), prtFlag,
		claimStagingPeriodFlag, strconv.FormatUint(iorClaimStagingPeriod, 10))
	s.Require().NoError(err)
	ctx, cancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	defer cancel()
	epoch := s.requireComputedInvalidOutputsRoot(ctx, 0, 0)
	s.Require().Zero(epoch.InputIndexLowerBound)
	s.Require().Zero(epoch.InputIndexUpperBound, "the first sealed epoch must contain no inputs")
	client := newIntegrationEthClient(ctx, s.T())
	defer client.Close()
	app, err := iapplication.NewIApplication(common.HexToAddress(appAddress), client)
	s.Require().NoError(err)
	template, err := app.GetTemplateHash(&bind.CallOpts{Context: ctx})
	s.Require().NoError(err)
	s.Require().Equal(common.Hash(template), *epoch.MachineHash)
	consensus, err := idaveconsensus.NewIDaveConsensus(common.HexToAddress(consensusAddress), client)
	s.Require().NoError(err)
	period, err := consensus.GetClaimStagingPeriod(&bind.CallOpts{Context: ctx})
	s.Require().NoError(err)
	s.Require().Zero(period.Cmp(new(big.Int).SetUint64(iorClaimStagingPeriod)))
	sentries, err := consensus.GetNumberOfSentries(&bind.CallOpts{Context: ctx})
	s.Require().NoError(err)
	s.Require().Zero(sentries.Sign(), "the test must not have a sentry fast path")
	tournament := waitForTournamentAndCommitment(ctx, s.T(), s.Require(), s.appName, epoch.Index)
	commitments, err := readCommitments(ctx, s.appName)
	s.Require().NoError(err)
	joined := findCommitmentForEpoch(commitments.Data, epoch.Index)
	s.Require().NotNil(joined)
	s.Require().Equal(tournament.Address, joined.TournamentAddress)
	s.Require().Equal(*epoch.Commitment, joined.Commitment)
	s.Require().Equal(common.Hash(template), joined.FinalStateHash)
	s.Require().NotZero(joined.TxHash, "the commitment must come from an observed join transaction")
	s.Require().NotZero(joined.BlockNumber)
	_, err = mineForTournamentTimeout(ctx, client, tournament.Address)
	s.Require().NoError(err)
	waitForTournamentWinner(ctx, s.T(), s.Require(), client, s.appName, epoch.Index)

	// A healthy root settles in this same environment before the negative
	// interval. The IOR veto is not masked by an inactive publisher or signer.
	s.requireHealthyPrtPublication(ctx, client)
	observeIORPublicationWindow(ctx, s.T(), func(ctx context.Context) {
		s.requireIORPrtState(ctx, consensus, epoch, false)
	})
	s.requireNoIORStageEvent(ctx, consensus, epoch)
	s.SetExpectedLogs(s.T(), anvilBlockOutOfRangeAllowlist, s.iorDetectionExpectedLog(),
		ExpectedLog{Pattern: regexp.MustCompile(`condition=ior_winner_settlement_withheld.*application=` + regexp.QuoteMeta(s.appName)),
			Level: LevelError, Required: true, Reason: "the current true winner is held by the local IOR veto"},
		ExpectedLog{Pattern: regexp.MustCompile(`condition=ior_result_staged.*application=` + regexp.QuoteMeta(s.appName)),
			Level: LevelError, Required: true, Reason: "another actor stages the true IOR result"},
		ExpectedLog{Pattern: regexp.MustCompile(`condition=ior_result_accepted.*application=` + regexp.QuoteMeta(s.appName)),
			Level: LevelError, Required: true, Reason: "matching external acceptance retains the IOR fact"})
	external := transactorForMnemonicIndex(ctx, s.T(), client, iorExternalSettlementAccount)
	s.Require().NotEqual(joined.SubmitterAddress, external.From, "the external actor must not be the node claimer")
	proof := iorDaveMachineProof(s.T(), epoch)
	tx, err := consensus.StageTournamentResult(external, new(big.Int).SetUint64(epoch.Index), proof)
	s.Require().NoError(err)
	receipt, err := bind.WaitMined(ctx, client, tx)
	s.Require().NoError(err)
	s.Require().Equal(types.ReceiptStatusSuccessful, receipt.Status)
	staged, err := waitForEpochStatus(ctx, s.T(), s.appName, epoch.Index, model.EpochStatus_ClaimStaged)
	s.Require().NoError(err)
	s.Require().NotNil(staged.StagedAtBlock)
	s.Require().Equal(receipt.BlockNumber.Uint64(), *staged.StagedAtBlock)
	mineIORAcceptanceBoundary(ctx, s.T(), client, *staged.StagedAtBlock)
	canAccept, err := consensus.CanAcceptStagedTournamentResult(&bind.CallOpts{Context: ctx})
	s.Require().NoError(err)
	s.Require().True(canAccept.IsClaimStagingPeriodOver)
	s.Require().False(canAccept.DoAllSentriesAgreeWithStagedTournamentResult)
	observeIORPublicationWindow(ctx, s.T(), func(ctx context.Context) {
		s.requireIORPrtState(ctx, consensus, epoch, true)
	})

	// Computation in the accumulating successor remains enabled before accept.
	index, _, err := sendInput(ctx, s.appName, "before-external-accept")
	s.Require().NoError(err)
	s.Require().Zero(index)
	input, err := waitForInputProcessed(ctx, s.T(), s.appName, index)
	s.Require().NoError(err)
	s.Require().Equal(model.InputCompletionStatus_Accepted, input.Status)
	s.Require().Equal(epoch.Index+1, input.EpochIndex)
	tx, err = consensus.AcceptStagedTournamentResult(external, new(big.Int).SetUint64(epoch.Index))
	s.Require().NoError(err)
	receipt, err = bind.WaitMined(ctx, client, tx)
	s.Require().NoError(err)
	s.Require().Equal(types.ReceiptStatusSuccessful, receipt.Status)
	accepted, err := waitForEpochStatus(ctx, s.T(), s.appName, epoch.Index, model.EpochStatus_ClaimAccepted)
	s.Require().NoError(err)
	s.Require().Equal(epoch.TxBufferDataBlock, accepted.TxBufferDataBlock)
	s.Require().Equal(epoch.CalculatedOutputsRoot, accepted.CalculatedOutputsRoot)
	s.Require().True(accepted.HasInvalidOutputsRoot())
	later := s.requireComputedInvalidOutputsRoot(ctx, input.EpochIndex, 1)
	laterTournament := waitForTournamentAndCommitment(ctx, s.T(), s.Require(), s.appName, later.Index)
	commitments, err = readCommitments(ctx, s.appName)
	s.Require().NoError(err)
	laterJoin := findCommitmentForEpoch(commitments.Data, later.Index)
	s.Require().NotNil(laterJoin)
	s.Require().Equal(laterTournament.Address, laterJoin.TournamentAddress)
	s.Require().Equal(*later.Commitment, laterJoin.Commitment)
	s.Require().NotZero(laterJoin.TxHash)
	index, _, err = sendInput(ctx, s.appName, "after-external-accept")
	s.Require().NoError(err)
	s.Require().Equal(uint64(1), index)
	input, err = waitForInputProcessed(ctx, s.T(), s.appName, index)
	s.Require().NoError(err)
	s.Require().Equal(model.InputCompletionStatus_Accepted, input.Status)
	s.Require().Greater(input.EpochIndex, later.Index)
	rpc := jsonrpcclient.NewClient(envOrDefault("CARTESI_JSONRPC_API_URL", "http://localhost:10011/rpc"))
	s.Require().Equal(model.ApplicationStatus_OK, s.getApplication(rpc).Status)
}

func (s *TerminalMachineStatesSuite) requireComputedInvalidOutputsRoot(
	ctx context.Context, epochIndex, processed uint64,
) *model.Epoch {
	s.T().Helper()
	r := s.Require()
	epoch, err := waitForEpochStatus(ctx, s.T(), s.appName, epochIndex, model.EpochStatus_ClaimComputed)
	r.NoError(err)
	rpc := jsonrpcclient.NewClient(envOrDefault("CARTESI_JSONRPC_API_URL", "http://localhost:10011/rpc"))
	app := s.getApplication(rpc)
	r.Equal(model.ApplicationStatus_OK, app.Status)
	r.Equal(processed, app.ProcessedInputs)
	r.Nil(app.Reason)
	r.Equal(new(common.Hash{}), epoch.TxBufferDataBlock)
	calculated := merkle.CreatePostContext()[merkle.TREE_DEPTH]
	r.Equal(&calculated, epoch.CalculatedOutputsRoot)
	r.True(epoch.HasInvalidOutputsRoot())
	r.Nil(epoch.ClaimTransactionHash, "local settlement is withheld")
	r.True(epoch.HasCompleteStateProof())
	if app.IsDaveConsensus() {
		r.NotNil(epoch.Commitment, "the true execution remains available to join")
		r.Len(epoch.CommitmentProof, int(model.Log2EpochComputationHashLeafCount))
	}
	if len(s.expectedLogs) == 0 {
		s.SetExpectedLogs(s.T(), anvilBlockOutOfRangeAllowlist, s.iorDetectionExpectedLog())
	}
	return epoch
}

func (s *TerminalMachineStatesSuite) iorDetectionExpectedLog() ExpectedLog {
	return ExpectedLog{
		Pattern: regexp.MustCompile(`condition=ior_settlement_withheld.*application=` + regexp.QuoteMeta(s.appName)),
		Level:   LevelWarn, Required: true,
		Reason: "the completed epoch retains its invalid-root evidence while execution remains enabled",
	}
}
