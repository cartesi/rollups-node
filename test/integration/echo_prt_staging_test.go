// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"math/big"
	"strconv"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

// TestEchoPrtStagingPath keeps the node running through the complete staging
// interval. The test mines blocks but never submits an acceptance transaction.
func (s *EchoPrtSuite) TestEchoPrtStagingPath() {
	r := s.Require()
	s.appName = uniqueAppName("echo-prt-staging")
	const claimStagingPeriod uint64 = 300

	runEchoLifecycleTest(s.ctx, s.T(), r, echoLifecycleConfig{
		AppName:  s.appName,
		DappPath: envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp"),
		Payload:  "prt-staging",
		ExtraDeployArgs: []string{
			prtFlag, claimStagingPeriodFlag, strconv.FormatUint(claimStagingPeriod, 10),
		},
		PreClaimHook: func(ctx context.Context, _ testing.TB, r *require.Assertions, appName string) {
			dsn, err := config.GetDatabaseConnection()
			r.NoError(err, "get database connection")
			repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
			r.NoError(err, "open repository")
			defer repo.Close()
			app, err := repo.GetApplication(ctx, appName)
			r.NoError(err, "read PRT application")
			consensus, err := idaveconsensus.NewIDaveConsensus(app.IConsensusAddress, s.ethClient)
			r.NoError(err, "bind Dave consensus")
			period, err := consensus.GetClaimStagingPeriod(&bind.CallOpts{Context: ctx})
			r.NoError(err, "read configured staging period")
			r.Zero(period.Cmp(new(big.Int).SetUint64(claimStagingPeriod)), "the configured staging period must match")

			// Epoch 0 is sealed empty at deployment. Epoch 1 contains the input.
			for _, epochIndex := range []uint64{0, 1} {
				s.finalizeEpochAfterStagingPeriod(consensus, epochIndex, claimStagingPeriod)
			}
		},
	})
}

func (s *EchoPrtSuite) finalizeEpochAfterStagingPeriod(
	consensus *idaveconsensus.IDaveConsensus,
	epochIndex, claimStagingPeriod uint64,
) {
	s.T().Helper()
	r := s.Require()
	tournament := waitForTournamentAndCommitment(s.ctx, s.T(), r, s.appName, epochIndex)
	_, err := mineForTournamentTimeout(s.ctx, s.ethClient, tournament.Address)
	r.NoError(err, "mine past epoch %d tournament timeout", epochIndex)
	waitForTournamentWinner(s.ctx, s.T(), r, s.ethClient, s.appName, epochIndex)

	stagedCtx, stagedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	staged, err := waitForEpochStatus(stagedCtx, s.T(), s.appName, epochIndex, model.EpochStatus_ClaimStaged)
	stagedCancel()
	r.NoError(err, "wait for epoch %d staging", epochIndex)
	r.NotNil(staged.StagedAtBlock, "the node must persist the staging block")

	currentBlock, err := s.ethClient.BlockNumber(s.ctx)
	r.NoError(err, "read block before claim acceptance")
	acceptanceBlock := *staged.StagedAtBlock + claimStagingPeriod
	r.Less(currentBlock, acceptanceBlock, "the test must observe the epoch before acceptance is permitted")
	callOpts := &bind.CallOpts{Context: s.ctx, BlockNumber: new(big.Int).SetUint64(currentBlock)}
	sealed, err := consensus.GetCurrentSealedEpoch(callOpts)
	r.NoError(err, "read staged epoch on chain")
	r.Zero(sealed.EpochNumber.Cmp(new(big.Int).SetUint64(epochIndex)), "the epoch must not be accepted early")
	r.True(sealed.IsTournamentResultStaged)
	r.Zero(sealed.StagingBlockNumber.Cmp(new(big.Int).SetUint64(*staged.StagedAtBlock)),
		"the persisted staging block must match the contract")
	canAccept, err := consensus.CanAcceptStagedTournamentResult(callOpts)
	r.NoError(err, "read acceptance conditions before the boundary")
	r.False(canAccept.IsClaimStagingPeriodOver)
	r.False(canAccept.DoAllSentriesAgreeWithStagedTournamentResult, "this test must not use the sentry fast path")

	root, err := itournament.NewITournament(tournament.Address, s.ethClient)
	r.NoError(err, "bind root tournament")
	recovery, err := root.BondRecovery(callOpts)
	r.NoError(err, "read root bond owner before acceptance")
	r.Equal(uint8(bondDispositionRecoverable), recovery.Disposition)
	commitments, err := readCommitments(s.ctx, s.appName)
	r.NoError(err, "read the node commitment")
	commitment := findCommitmentForEpoch(commitments.Data, epochIndex)
	r.NotNil(commitment)
	r.Equal(commitment.SubmitterAddress, recovery.Claimer, "the node must own the root bond")

	// Only advance time. The live node must submit acceptance and recover its bond.
	blocksToMine := acceptanceBlock - currentBlock
	r.LessOrEqual(blocksToMine, claimStagingPeriod)
	r.NoError(anvilMine(s.ctx, int(blocksToMine)), //nolint:gosec // Bounded by claimStagingPeriod.
		"mine to the PRT acceptance boundary")
	accepted := waitForPrtEpochAcceptedAndBondRecovered(
		s.ctx, s.T(), r, s.ethClient, s.appName, epochIndex, tournament.Address)
	r.Equal(staged.StagedAtBlock, accepted.StagedAtBlock, "acceptance must preserve the staging block")

	receipt, err := s.ethClient.TransactionReceipt(s.ctx, *accepted.ClaimTransactionHash)
	r.NoError(err, "read the node acceptance receipt")
	r.Equal(types.ReceiptStatusSuccessful, receipt.Status)
	r.GreaterOrEqual(receipt.BlockNumber.Uint64(), acceptanceBlock, "acceptance must not occur before the period ends")
	tx, pending, err := s.ethClient.TransactionByHash(s.ctx, receipt.TxHash)
	r.NoError(err, "read the node acceptance transaction")
	r.False(pending)
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	r.NoError(err, "recover acceptance sender")
	r.Equal(commitment.SubmitterAddress, sender, "the node PRT signer must submit acceptance")
}
