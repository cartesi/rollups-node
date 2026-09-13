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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

// TestEchoPrtStagingPath keeps the node running through the complete staging
// interval. The test mines blocks but never submits an acceptance transaction.
func (s *EchoPrtSuite) TestEchoPrtStagingPath() {
	r := s.Require()
	s.appName = uniqueAppName("echo-prt-staging")
	s.SetExpectedLogs(s.T(), anvilBlockOutOfRangeAllowlist)
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
			sentries, err := consensus.GetNumberOfSentries(&bind.CallOpts{Context: ctx})
			r.NoError(err, "read configured sentries")
			r.Zero(sentries.Sign(), "the staging test must not have a sentry fast path")

			// Epoch 0 is sealed empty at deployment. Epoch 1 contains the input.
			for _, epochIndex := range []uint64{0, 1} {
				s.finalizeEpochAfterStagingPeriod(consensus, app.IConsensusAddress, epochIndex, claimStagingPeriod)
			}
		},
	})
}

func (s *EchoPrtSuite) finalizeEpochAfterStagingPeriod(
	consensus *idaveconsensus.IDaveConsensus,
	consensusAddress common.Address,
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

	commitments, err := readCommitments(s.ctx, s.appName)
	r.NoError(err, "read the node commitment")
	commitment := findCommitmentForEpoch(commitments.Data, epochIndex)
	r.NotNil(commitment)
	recovered := s.waitForOwnedRootBondRecovery(tournament, commitment)
	r.Less(recovered.Raw.BlockNumber, acceptanceBlock, "the node must recover before acceptance becomes eligible")
	callOpts.BlockNumber.SetUint64(recovered.Raw.BlockNumber)
	sealed, err = consensus.GetCurrentSealedEpoch(callOpts)
	r.NoError(err, "read current epoch at the recovery block")
	r.Zero(sealed.EpochNumber.Cmp(new(big.Int).SetUint64(epochIndex)))
	r.True(sealed.IsTournamentResultStaged)
	canAccept, err = consensus.CanAcceptStagedTournamentResult(callOpts)
	r.NoError(err, "read acceptance conditions at the recovery block")
	r.False(canAccept.IsClaimStagingPeriodOver)
	r.False(canAccept.DoAllSentriesAgreeWithStagedTournamentResult)

	// Recovery is already mined. Only the node may submit acceptance.
	currentBlock, err = s.ethClient.BlockNumber(s.ctx)
	r.NoError(err, "read block after root bond recovery")
	r.Less(currentBlock, acceptanceBlock, "recovery must finish within the controlled staging window")
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
	sealedEvent := s.requirePrtEpochSealed(consensus, consensusAddress, receipt, epochIndex+1)
	r.Less(recovered.Raw.BlockNumber, sealedEvent.Raw.BlockNumber,
		"BondRecovered must precede the next epoch's EpochSealed event")
	tx, pending, err := s.ethClient.TransactionByHash(s.ctx, receipt.TxHash)
	r.NoError(err, "read the node acceptance transaction")
	r.False(pending)
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	r.NoError(err, "recover acceptance sender")
	r.Equal(commitment.SubmitterAddress, sender, "the node PRT signer must submit acceptance")
}

func (s *EchoPrtSuite) waitForOwnedRootBondRecovery(
	tournament *model.Tournament, commitment *model.Commitment,
) *itournament.ITournamentBondRecovered {
	s.T().Helper()
	r := s.Require()
	ctx, cancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	defer cancel()
	waitForRootBondRecovered(ctx, s.T(), r, s.ethClient, tournament.Address)
	root, err := itournament.NewITournament(tournament.Address, s.ethClient)
	r.NoError(err)
	head, err := s.ethClient.BlockNumber(ctx)
	r.NoError(err)
	events, err := root.FilterBondRecovered(&bind.FilterOpts{
		Context: ctx, Start: tournament.StartInstant, End: &head,
	}, [][32]byte{commitment.Commitment}, []common.Address{commitment.SubmitterAddress})
	r.NoError(err, "read the winning claimer's BondRecovered event")
	defer func() { r.NoError(events.Close()) }()
	r.True(events.Next(), "the recovered root must have a BondRecovered event")
	event := events.Event
	r.False(events.Next(), "a root bond must be paid only once")
	r.NoError(events.Error())
	r.Equal(tournament.Address, event.Raw.Address)
	r.Equal(commitment.Commitment, common.Hash(event.Commitment))
	r.Equal(commitment.SubmitterAddress, event.Claimer)
	r.Positive(event.Payment.Sign(), "the winner must receive its bond")
	s.requirePrtEventReceipt(event.Raw)
	tx, pending, err := s.ethClient.TransactionByHash(ctx, event.Raw.TxHash)
	r.NoError(err, "read the bond recovery transaction")
	r.False(pending)
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	r.NoError(err, "recover bond recovery sender")
	r.Equal(commitment.SubmitterAddress, sender, "the node PRT signer must send the recovery transaction")
	return event
}

func (s *EchoPrtSuite) requirePrtEpochSealed(
	consensus *idaveconsensus.IDaveConsensus, address common.Address, receipt *types.Receipt, epoch uint64,
) *idaveconsensus.IDaveConsensusEpochSealed {
	s.T().Helper()
	var sealed *idaveconsensus.IDaveConsensusEpochSealed
	for _, raw := range receipt.Logs {
		if raw.Address != address {
			continue
		}
		event, err := consensus.ParseEpochSealed(*raw)
		if err != nil || event.EpochNumber.Cmp(new(big.Int).SetUint64(epoch)) != 0 {
			continue
		}
		s.Require().Nil(sealed, "an acceptance must emit exactly one matching EpochSealed event")
		sealed = event
	}
	s.Require().NotNil(sealed, "acceptance must seal epoch %d", epoch)
	s.requirePrtEventReceipt(sealed.Raw)
	return sealed
}

func (s *EchoPrtSuite) requirePrtEventReceipt(raw types.Log) {
	s.T().Helper()
	r := s.Require()
	r.False(raw.Removed)
	receipt, err := s.ethClient.TransactionReceipt(s.ctx, raw.TxHash)
	r.NoError(err, "read event transaction receipt")
	r.Equal(types.ReceiptStatusSuccessful, receipt.Status)
	r.Equal(raw.BlockHash, receipt.BlockHash)
	r.Equal(raw.BlockNumber, receipt.BlockNumber.Uint64())
	r.Equal(raw.TxHash, receipt.TxHash)
	r.Equal(raw.TxIndex, receipt.TransactionIndex)
	for _, mined := range receipt.Logs {
		if mined.Index == raw.Index {
			r.Equal(raw.Address, mined.Address)
			r.Equal(raw.Topics, mined.Topics)
			r.Equal(raw.Data, mined.Data)
			return
		}
	}
	r.FailNow("event log index is absent from its transaction receipt")
}
