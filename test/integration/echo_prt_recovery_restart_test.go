// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prt"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
)

// TestEchoPrtAcceptedBondRecoveryAfterRestart prepares an accepted epoch while
// the node is stopped. Restart must discover its unpaid bond without another
// acceptance callback, independently of preparation of the next epoch's claim.
func (s *EchoPrtSuite) TestEchoPrtAcceptedBondRecoveryAfterRestart() {
	if !isNodeSelfManaged() {
		s.T().Skip("bond recovery restart requires a test-managed node")
	}
	r := s.Require()
	s.SetExpectedLogs(s.T(), anvilBlockOutOfRangeAllowlist)
	// Automine admits explicit transactions, but elapsed wall time cannot finish
	// either root while the fixture stops the node or waits for its next join.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := anvilRPC(ctx, "evm_setAutomine", true); err != nil {
			s.T().Errorf("restore Anvil automine: %v", err)
		}
		if err := anvilRPC(ctx, "evm_setIntervalMining", 1); err != nil {
			s.T().Errorf("restore Anvil interval mining: %v", err)
		}
		if sharedNode == nil {
			startSharedNode(s.T())
		}
	}()
	setAnvilIntervalMining(s.ctx, s.T(), 0)
	setAnvilAutomine(s.ctx, s.T(), true)
	dsn, err := config.GetDatabaseConnection()
	r.NoError(err)
	repo, err := factory.NewRepositoryFromConnectionString(s.ctx, dsn.Raw())
	r.NoError(err)
	defer repo.Close()
	readerConfig, _, err := loadRecoveryFixtureConfig()
	r.NoError(err)
	s.appName = uniqueAppName("echo-prt-bond-restart")
	_, err = deployApplication(s.ctx, s.appName,
		envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp"),
		"--salt", uniqueSalt(), prtFlag, claimStagingPeriodFlag, "5")
	r.NoError(err, "deploy the recovery restart application")
	s.waitForRecoveryFixtureCommitment(repo, readerConfig.BlockchainDefaultBlock, 0)
	app, err := repo.GetApplication(s.ctx, s.appName)
	r.NoError(err)
	r.NotNil(app)
	epoch, err := repo.GetEpoch(s.ctx, s.appName, 0)
	r.NoError(err)
	r.NotNil(epoch)
	r.Equal(model.EpochStatus_ClaimComputed, epoch.Status)
	r.True(epoch.HasCompleteStateProof())
	r.NotNil(epoch.TournamentAddress)
	r.NotNil(epoch.Commitment)
	root, err := repo.GetTournament(s.ctx, s.appName, epoch.TournamentAddress.Hex())
	r.NoError(err)
	r.NotNil(root)
	commitment, err := repo.GetCommitment(s.ctx, s.appName, epoch.Index, root.Address.Hex(), epoch.Commitment.Hex())
	r.NoError(err)
	r.NotNil(commitment)
	contract, err := itournament.NewITournament(root.Address, s.ethClient)
	r.NoError(err)
	beforeStop, err := contract.BondRecovery(&bind.CallOpts{Context: s.ctx})
	r.NoError(err)
	r.Equal(uint8(bondDispositionTournamentRunning), beforeStop.Disposition,
		"stop before the root can finish, not between acceptance and recovery")
	stopSharedNode(s.T())

	_, err = mineForTournamentTimeout(s.ctx, s.ethClient, root.Address)
	r.NoError(err)
	consensus, err := idaveconsensus.NewIDaveConsensus(app.IConsensusAddress, s.ethClient)
	r.NoError(err)
	acceptReceipt := s.acceptStoppedPrtEpoch(consensus, app.IConsensusAddress, epoch, commitment.SubmitterAddress)
	observedBlock := s.publishAcceptedRecoveryFixture(repo, epoch.Index+1, acceptReceipt.BlockNumber.Uint64())
	accepted, err := repo.GetEpoch(s.ctx, s.appName, epoch.Index)
	r.NoError(err)
	r.NotNil(accepted)
	r.Equal(model.EpochStatus_ClaimAccepted, accepted.Status)
	r.NotNil(accepted.ClaimTransactionHash)
	r.Equal(acceptReceipt.TxHash, *accepted.ClaimTransactionHash)
	app, err = repo.GetApplication(s.ctx, s.appName)
	r.NoError(err)
	r.Equal(observedBlock, app.LastTournamentCheckBlock)
	storedRoot, err := repo.GetTournament(s.ctx, s.appName, root.Address.Hex())
	r.NoError(err)
	r.NotNil(storedRoot)
	r.Equal(app.LastTournamentCheckBlock, storedRoot.Snapshot.AsOfBlock)
	r.Equal(model.TournamentStandingRootWinner, storedRoot.Snapshot.Standing)
	r.Equal(epoch.Commitment, storedRoot.Snapshot.WinnerCommitment)
	r.Equal(epoch.MachineHash, storedRoot.Snapshot.FinalStateHash)
	r.Equal(model.BondDispositionRecoverable, storedRoot.Snapshot.BondRecovery.Disposition)
	r.NotNil(storedRoot.Snapshot.BondRecovery.Claimer)
	r.Equal(commitment.SubmitterAddress, *storedRoot.Snapshot.BondRecovery.Claimer)
	beforeRestart, err := contract.BondRecovery(&bind.CallOpts{Context: s.ctx})
	r.NoError(err)
	r.Equal(uint8(bondDispositionRecoverable), beforeRestart.Disposition)
	r.Equal(commitment.SubmitterAddress, beforeRestart.Claimer)
	r.Nil(sharedNode, "accepted and recoverable fixture must exist before the new process starts")

	startSharedNode(s.T())
	// Empty epoch 1 is computed normally after the full node resumes.
	// With interval mining still stopped, its root stays open for the join.
	nextRoot := s.waitForRecoveryFixtureCommitment(repo, readerConfig.BlockchainDefaultBlock, 1)
	nextEpoch, err := repo.GetEpoch(s.ctx, s.appName, 1)
	r.NoError(err)
	r.NotNil(nextEpoch)
	r.Equal(model.EpochStatus_ClaimComputed, nextEpoch.Status)
	r.True(nextEpoch.HasCompleteStateProof())
	r.NotNil(nextEpoch.Commitment)
	nextCommitment, err := repo.GetCommitment(s.ctx, s.appName, 1, nextRoot.Address.Hex(), nextEpoch.Commitment.Hex())
	r.NoError(err)
	r.NotNil(nextCommitment)
	r.Equal(commitment.SubmitterAddress, nextCommitment.SubmitterAddress)
	recoveryCtx, cancelRecovery := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	defer cancelRecovery()
	err = pollUntil(recoveryCtx, time.Second, func() (bool, error) {
		_, finalityErr := advanceFixtureFinality(recoveryCtx, s.ethClient, readerConfig.BlockchainDefaultBlock)
		if finalityErr != nil {
			return false, finalityErr
		}
		recovery, readErr := contract.BondRecovery(&bind.CallOpts{Context: recoveryCtx})
		return readErr == nil && recovery.Disposition == uint8(bondDispositionRecovered), readErr
	})
	r.NoError(err, "recover the old bond while keeping idle tournament blocks stopped")
	recovered := s.waitForOwnedRootBondRecovery(root, commitment)
	r.Greater(recovered.Raw.BlockNumber, acceptReceipt.BlockNumber.Uint64(), "the old bond must be recovered after restart")
	r.Greater(nextCommitment.BlockNumber, acceptReceipt.BlockNumber.Uint64(), "the next root must be joined after restart")
	nextContract, err := itournament.NewITournament(nextRoot.Address, s.ethClient)
	r.NoError(err)
	atRecovery := &bind.CallOpts{Context: s.ctx, BlockNumber: new(big.Int).SetUint64(recovered.Raw.BlockNumber)}
	nextRecovery, err := nextContract.BondRecovery(atRecovery)
	r.NoError(err)
	r.Equal(uint8(bondDispositionTournamentRunning), nextRecovery.Disposition,
		"older recovery must run during the current root's known wait")
	joined, err := nextContract.CommitmentStanding(atRecovery, *nextEpoch.Commitment)
	r.NoError(err)
	r.Equal(nextCommitment.BlockNumber <= recovered.Raw.BlockNumber, joined.Joined,
		"the historical join view must agree with its actual inclusion block")
	if joined.Joined {
		r.Equal(commitment.SubmitterAddress, joined.Claimer)
	}
}

func (s *EchoPrtSuite) publishAcceptedRecoveryFixture(repo repository.Repository, nextEpoch, acceptanceBlock uint64) uint64 {
	s.T().Helper()
	r := s.Require()
	// Run real observation without the advancer, validator, or a submitting
	// signer. Recovery is independent of the new epoch's computation, so this
	// stopped-node fixture must explicitly use observer-only PRT settings.
	readerConfig, prtConfig, err := loadRecoveryFixtureConfig()
	r.NoError(err)
	observationBlock, err := advanceFixtureFinality(s.ctx, s.ethClient, readerConfig.BlockchainDefaultBlock)
	r.NoError(err)
	r.GreaterOrEqual(observationBlock, acceptanceBlock, "the acceptance must be visible at the configured block policy")
	headBeforeObservation, err := s.ethClient.BlockNumber(s.ctx)
	r.NoError(err)
	readerService, err := evmreader.Create(s.ctx, &evmreader.CreateInfo{
		Config: *readerConfig, Repository: repo, EthClient: s.ethClient,
	})
	r.NoError(err)
	reader, ok := readerService.(*evmreader.Service)
	r.True(ok)
	// Reader Tick logs individual application read errors and can return nil
	// without advancing this app. Retry the real observer after transient Anvil
	// historical-read failures, and require its durable cursor as positive proof.
	readerCtx, cancelReader := context.WithTimeout(s.ctx, time.Minute)
	defer cancelReader()
	err = pollUntil(readerCtx, time.Second, func() (bool, error) {
		if _, tickErr := reader.Tick(readerCtx); tickErr != nil {
			return false, tickErr
		}
		observed, readErr := repo.GetApplication(readerCtx, s.appName)
		if readErr != nil {
			return false, readErr
		}
		return observed != nil && observed.LastEpochCheckBlock == observationBlock, nil
	})
	r.NoError(err, "the real reader must publish the acceptance event within the fixture deadline")
	app, err := repo.GetApplication(s.ctx, s.appName)
	r.NoError(err)
	r.Equal(observationBlock, app.LastEpochCheckBlock, "the reader must publish the acceptance event")
	epoch, err := repo.GetEpoch(s.ctx, s.appName, nextEpoch)
	r.NoError(err)
	r.NotNil(epoch)
	r.Equal(model.EpochStatus_Closed, epoch.Status)
	r.False(epoch.HasCompleteStateProof())
	r.Nil(sharedNode, "the saved-config override is permitted only while the test-managed node is stopped")
	writer, ok := repo.(interface {
		SaveNodeConfigRaw(context.Context, string, []byte) error
	})
	r.True(ok, "the stopped-node fixture requires its isolated PostgreSQL config writer")
	original, _, _, err := repo.LoadNodeConfigRaw(s.ctx, prt.PrtConfigKey)
	r.NoError(err)
	updated, err := patchPassiveObserverConfig(original, map[string]any{
		passiveClaimSubmissionField: false, "ResolvedSignerAddress": common.Address{},
	})
	r.NoError(err)
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		r.NoError(writer.SaveNodeConfigRaw(cleanupCtx, prt.PrtConfigKey, original), "restore the exact submitting record before restart")
	}()
	r.NoError(writer.SaveNodeConfigRaw(s.ctx, prt.PrtConfigKey, updated))
	prtConfig.FeatureClaimSubmissionEnabled = false
	prtService, err := prt.Create(s.ctx, &prt.CreateInfo{
		Config: *prtConfig, Repository: repo, EthClient: s.ethClient,
	})
	r.NoError(err)
	observer, ok := prtService.(*prt.Service)
	r.True(ok)
	_, err = observer.Tick(s.ctx)
	r.NoError(err)
	epoch, err = repo.GetEpoch(s.ctx, s.appName, nextEpoch)
	r.NoError(err)
	r.Equal(model.EpochStatus_Closed, epoch.Status, "the current claim must remain unprepared")
	head, err := s.ethClient.BlockNumber(s.ctx)
	r.NoError(err)
	r.Equal(headBeforeObservation, head, "fixture publication must not send a transaction")
	return observationBlock
}

func (s *EchoPrtSuite) waitForRecoveryFixtureCommitment(
	repo repository.Repository, policy model.DefaultBlock, epochIndex uint64,
) *model.Tournament {
	s.T().Helper()
	ctx, cancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	defer cancel()
	var tournament *model.Tournament
	err := pollUntil(ctx, time.Second, func() (bool, error) {
		// Only transaction-bearing blocks need successors for finality. An idle
		// node gets no empty blocks, so this wait cannot expire the active root.
		if _, finalityErr := advanceFixtureFinality(ctx, s.ethClient, policy); finalityErr != nil {
			return false, finalityErr
		}
		epoch, readErr := repo.GetEpoch(ctx, s.appName, epochIndex)
		if readErr != nil || epoch == nil || epoch.TournamentAddress == nil || epoch.Commitment == nil {
			return false, readErr
		}
		commitment, readErr := repo.GetCommitment(ctx, s.appName, epochIndex, epoch.TournamentAddress.Hex(), epoch.Commitment.Hex())
		if readErr != nil || commitment == nil {
			return false, readErr
		}
		tournament, readErr = repo.GetTournament(ctx, s.appName, epoch.TournamentAddress.Hex())
		return tournament != nil, readErr
	})
	s.Require().NoError(err, "wait for epoch %d commitment with bounded finality advancement", epochIndex)
	return tournament
}

func (s *EchoPrtSuite) acceptStoppedPrtEpoch(
	consensus *idaveconsensus.IDaveConsensus, address common.Address, epoch *model.Epoch, claimant common.Address,
) *types.Receipt {
	s.T().Helper()
	r := s.Require()
	const externalSignerIndex uint32 = 3
	external := transactorForMnemonicIndex(s.ctx, s.T(), s.ethClient, externalSignerIndex)
	r.NotEqual(claimant, external.From, "fixture settlement must use a signer separate from the node")
	stateProof, err := epoch.StateProof()
	r.NoError(err)
	proof := idaveconsensus.MachineValidityProof{
		IflagsYProof:    idaveconsensus.LeafProof{DataBlock: stateProof.IflagsYDataBlock, Siblings: stateProof.IflagsYProof},
		HtifTohostProof: idaveconsensus.LeafProof{DataBlock: stateProof.HtifTohostDataBlock, Siblings: stateProof.HtifTohostProof},
		TxBufferProof:   idaveconsensus.LeafProof{DataBlock: stateProof.TxBufferDataBlock, Siblings: stateProof.TxBufferProof},
	}
	tx, err := consensus.StageTournamentResult(external, new(big.Int).SetUint64(epoch.Index), proof)
	r.NoError(err, "stage the real machine validity proof while the node is stopped")
	stagedReceipt := waitReceipt(s.ctx, s.T(), s.ethClient, tx)
	r.Equal(types.ReceiptStatusSuccessful, stagedReceipt.Status)
	stagedOpts := &bind.CallOpts{Context: s.ctx, BlockNumber: stagedReceipt.BlockNumber}
	sealed, err := consensus.GetCurrentSealedEpoch(stagedOpts)
	r.NoError(err)
	r.True(sealed.IsTournamentResultStaged)
	r.Zero(sealed.EpochNumber.Cmp(new(big.Int).SetUint64(epoch.Index)))
	r.Equal(stateProof.MachineHash, common.Hash(sealed.StagedPostEpochMachineStateHash))
	r.Equal(stateProof.TxBufferDataBlock, common.Hash(sealed.StagedPostEpochOutputsMerkleRoot))
	r.Zero(sealed.StagingBlockNumber.Cmp(stagedReceipt.BlockNumber))
	acceptReady, err := consensus.CanAcceptStagedTournamentResult(stagedOpts)
	r.NoError(err)
	r.True(acceptReady.IsTournamentResultStaged)
	r.Equal(stateProof.MachineHash, common.Hash(acceptReady.StagedPostEpochMachineStateHash))
	r.Equal(stateProof.TxBufferDataBlock, common.Hash(acceptReady.StagedPostEpochOutputsMerkleRoot))
	period, err := consensus.GetClaimStagingPeriod(&bind.CallOpts{Context: s.ctx})
	r.NoError(err)
	r.True(period.IsUint64())
	r.LessOrEqual(period.Uint64(), uint64(maxBlocksToMine))
	r.NoError(anvilMine(s.ctx, int(period.Uint64())), //nolint:gosec // Bounded by maxBlocksToMine.
		"advance the staging period while the node is stopped")
	tx, err = consensus.AcceptStagedTournamentResult(external, new(big.Int).SetUint64(epoch.Index))
	r.NoError(err)
	receipt := waitReceipt(s.ctx, s.T(), s.ethClient, tx)
	r.Equal(types.ReceiptStatusSuccessful, receipt.Status)
	accepted := s.requirePrtEpochSealed(consensus, address, receipt, epoch.Index+1)
	r.Equal(stateProof.MachineHash, common.Hash(accepted.InitialMachineStateHash))
	r.Equal(stateProof.TxBufferDataBlock, common.Hash(accepted.OutputsMerkleRoot))
	return receipt
}
