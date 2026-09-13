// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
)

// TestEchoPrtSentrySettlement deploys sentry slots through the CLI. External
// signers claim the locally computed state; the Go node stages and accepts it.
func (s *EchoPrtSuite) TestEchoPrtSentrySettlement() {
	r := s.Require()
	s.appName = uniqueAppName("echo-prt-sentries")
	// The tournament helper mines many blocks while the node remains active.
	s.SetExpectedLogs(s.T(), prtBlockOutOfRangeAllowlist)
	const (
		claimStagingPeriod uint64 = 1000
		managerIndex       uint32 = 9
		firstSentryIndex   uint32 = 7
		secondSentryIndex  uint32 = 8
		rotatedIndex       uint32 = 4
	)
	// These accounts are distinct from the devnet claimer (0) and PRT signer (6).
	manager := transactorForMnemonicIndex(s.ctx, s.T(), s.ethClient, managerIndex)
	first := transactorForMnemonicIndex(s.ctx, s.T(), s.ethClient, firstSentryIndex)
	second := transactorForMnemonicIndex(s.ctx, s.T(), s.ethClient, secondSentryIndex)
	rotated := transactorForMnemonicIndex(s.ctx, s.T(), s.ethClient, rotatedIndex)

	runEchoLifecycleTest(s.ctx, s.T(), r, echoLifecycleConfig{
		AppName:  s.appName,
		DappPath: envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp"),
		Payload:  "prt-sentries",
		ExtraDeployArgs: []string{
			prtFlag, claimStagingPeriodFlag, strconv.FormatUint(claimStagingPeriod, 10),
			"--sentry-manager", manager.From.Hex(), "--sentries", first.From.Hex() + "," + second.From.Hex(),
		},
		PreClaimHook: func(ctx context.Context, _ testing.TB, r *require.Assertions, appName string) {
			dsn, err := config.GetDatabaseConnection()
			r.NoError(err, "get database connection")
			repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
			r.NoError(err, "open repository")
			defer repo.Close()
			app, err := repo.GetApplication(ctx, appName)
			r.NoError(err, "read PRT application")
			r.NotNil(app)
			consensus, err := idaveconsensus.NewIDaveConsensus(app.IConsensusAddress, s.ethClient)
			r.NoError(err, "bind Dave consensus")
			period, err := consensus.GetClaimStagingPeriod(&bind.CallOpts{Context: ctx})
			r.NoError(err, "read configured staging period")
			r.Zero(period.Cmp(new(big.Int).SetUint64(claimStagingPeriod)))
			originalSentries := []common.Address{first.From, second.From}
			s.checkSentryInspection(app.IApplicationAddress, manager.From, originalSentries, "latest")

			// Epoch 0 is sealed empty at deployment. Epoch 1 contains the input.
			s.finalizeEpochWithSentries(consensus, 0, claimStagingPeriod, first, second)
			beforeRotation, err := s.ethClient.BlockNumber(ctx)
			r.NoError(err, "read block before sentry rotation")
			tx, err := consensus.RotateSentry(manager, first.From, rotated.From)
			r.NoError(err, "rotate first sentry with the configured manager")
			receipt := waitReceipt(ctx, s.T(), s.ethClient, tx)
			r.Equal(types.ReceiptStatusSuccessful, receipt.Status)
			oldID, err := consensus.GetSentryId(&bind.CallOpts{Context: ctx}, first.From)
			r.NoError(err, "read retired sentry ID")
			r.Zero(oldID.Sign())
			newID, err := consensus.GetSentryId(&bind.CallOpts{Context: ctx}, rotated.From)
			r.NoError(err, "read replacement sentry ID")
			r.Zero(newID.Cmp(big.NewInt(1)), "rotation must preserve the first slot")
			s.checkSentryInspection(app.IApplicationAddress, manager.From,
				[]common.Address{rotated.From, second.From}, "latest")
			s.checkSentryInspection(app.IApplicationAddress, manager.From,
				originalSentries, strconv.FormatUint(beforeRotation, 10))
			s.finalizeEpochWithSentries(consensus, 1, claimStagingPeriod, rotated, second)
		},
	})
}

func (s *EchoPrtSuite) finalizeEpochWithSentries(
	consensus *idaveconsensus.IDaveConsensus,
	epochIndex, claimStagingPeriod uint64,
	first, second *bind.TransactOpts,
) {
	s.T().Helper()
	r := s.Require()
	tournament := waitForTournamentAndCommitment(s.ctx, s.T(), r, s.appName, epochIndex)
	_, err := mineForTournamentTimeout(s.ctx, s.ethClient, tournament.Address)
	r.NoError(err, "mine past epoch %d tournament timeout", epochIndex)
	waitForTournamentWinner(s.ctx, s.T(), r, s.ethClient, s.appName, epochIndex)

	stagedCtx, cancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	staged, err := waitForEpochStatus(stagedCtx, s.T(), s.appName, epochIndex, model.EpochStatus_ClaimStaged)
	cancel()
	r.NoError(err, "wait for epoch %d staging", epochIndex)
	r.NotNil(staged.StagedAtBlock)
	r.NotNil(staged.MachineHash, "sentries need the state computed by the validator")
	acceptanceBlock := *staged.StagedAtBlock + claimStagingPeriod
	commitments, err := readCommitments(s.ctx, s.appName)
	r.NoError(err, "read the node commitment")
	commitment := findCommitmentForEpoch(commitments.Data, epochIndex)
	r.NotNil(commitment)
	r.NotEqual(commitment.SubmitterAddress, first.From, "the first sentry must not use the PRT signer")
	r.NotEqual(commitment.SubmitterAddress, second.From, "the second sentry must not use the PRT signer")

	// Use local computation, not the staged hash read from the consensus contract.
	tx, err := consensus.SubmitSentryClaim(first, new(big.Int).SetUint64(epochIndex), *staged.MachineHash)
	r.NoError(err, "submit the first sentry claim for epoch %d", epochIndex)
	firstReceipt := waitReceipt(s.ctx, s.T(), s.ethClient, tx)
	r.Equal(types.ReceiptStatusSuccessful, firstReceipt.Status)
	callOpts := &bind.CallOpts{Context: s.ctx, BlockNumber: firstReceipt.BlockNumber}
	canAccept, err := consensus.CanAcceptStagedTournamentResult(callOpts)
	r.NoError(err, "read acceptance conditions after the first sentry claim")
	r.Zero(canAccept.EpochNumber.Cmp(new(big.Int).SetUint64(epochIndex)))
	r.True(canAccept.IsTournamentResultStaged)
	r.False(canAccept.DoAllSentriesAgreeWithStagedTournamentResult, "one of two sentries is not sufficient")
	r.False(canAccept.IsClaimStagingPeriodOver)
	r.Less(firstReceipt.BlockNumber.Uint64(), acceptanceBlock)
	claimCount, err := consensus.GetSentryClaimCount(callOpts, new(big.Int).SetUint64(epochIndex), *staged.MachineHash)
	r.NoError(err, "read the first sentry claim count")
	r.Zero(claimCount.Cmp(big.NewInt(1)))

	tx, err = consensus.SubmitSentryClaim(second, new(big.Int).SetUint64(epochIndex), *staged.MachineHash)
	r.NoError(err, "submit the second sentry claim for epoch %d", epochIndex)
	secondReceipt := waitReceipt(s.ctx, s.T(), s.ethClient, tx)
	r.Equal(types.ReceiptStatusSuccessful, secondReceipt.Status)
	callOpts.BlockNumber = secondReceipt.BlockNumber
	canAccept, err = consensus.CanAcceptStagedTournamentResult(callOpts)
	r.NoError(err, "read acceptance conditions after both sentry claims")
	r.Zero(canAccept.EpochNumber.Cmp(new(big.Int).SetUint64(epochIndex)))
	r.True(canAccept.IsTournamentResultStaged)
	r.True(canAccept.DoAllSentriesAgreeWithStagedTournamentResult)
	r.False(canAccept.IsClaimStagingPeriodOver)

	// The test sends no acceptance transaction. The live Go node must accept.
	accepted := waitForPrtEpochAcceptedAndBondRecovered(
		s.ctx, s.T(), r, s.ethClient, s.appName, epochIndex, tournament.Address)
	r.Equal(staged.StagedAtBlock, accepted.StagedAtBlock)
	receipt, err := s.ethClient.TransactionReceipt(s.ctx, *accepted.ClaimTransactionHash)
	r.NoError(err, "read the node acceptance receipt")
	r.Equal(types.ReceiptStatusSuccessful, receipt.Status)
	r.Less(receipt.BlockNumber.Uint64(), acceptanceBlock, "all sentries must permit acceptance before the deadline")
	tx, pending, err := s.ethClient.TransactionByHash(s.ctx, receipt.TxHash)
	r.NoError(err, "read the node acceptance transaction")
	r.False(pending)
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	r.NoError(err, "recover acceptance sender")
	r.Equal(commitment.SubmitterAddress, sender, "the node PRT signer must submit acceptance")
}

type inspectedSentry struct {
	ID      uint64 `json:"id"`
	Address string `json:"address"`
}

type sentryInspection struct {
	Manager  string            `json:"sentry_manager"`
	Count    uint64            `json:"num_sentries"`
	Sentries []inspectedSentry `json:"sentries"`
}

func (s *EchoPrtSuite) checkSentryInspection(app, manager common.Address, sentries []common.Address, block string) {
	s.T().Helper()
	r := s.Require()
	expected := make([]inspectedSentry, len(sentries))
	for i, address := range sentries {
		expected[i] = inspectedSentry{ID: uint64(i) + 1, Address: address.Hex()}
	}
	for _, command := range []string{"consensus", "summary"} {
		args := []string{"contract", command, app.Hex(), "--block", block}
		text, err := runCLI(s.ctx, append(args, "--json")...)
		r.NoError(err, "inspect sentries with contract %s at %s", command, block)
		data := []byte(text)
		if command == "summary" {
			var summary struct {
				Consensus      json.RawMessage `json:"consensus"`
				ConsensusError string          `json:"consensus_error"`
			}
			r.NoError(json.Unmarshal(data, &summary))
			r.Empty(summary.ConsensusError)
			r.NotEmpty(summary.Consensus)
			data = summary.Consensus
		}
		var got sentryInspection
		r.NoError(json.Unmarshal(data, &got))
		r.Equal(manager.Hex(), got.Manager)
		r.Equal(uint64(len(sentries)), got.Count)
		r.Equal(expected, got.Sentries, "the roster must retain slot IDs and address order")

		text, err = runCLI(s.ctx, args...)
		r.NoError(err, "inspect sentries in contract %s text output", command)
		text = strings.Join(strings.Fields(text), " ")
		r.Contains(text, "Sentry Manager "+manager.Hex())
		r.Contains(text, fmt.Sprintf("Sentries %d", len(sentries)))
		for _, sentry := range expected {
			r.Contains(text, fmt.Sprintf("Sentry #%d %s", sentry.ID, sentry.Address))
		}
	}
}
