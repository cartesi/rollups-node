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
// acceptance callback, once the next epoch's join has had priority.
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
	s.appName = uniqueAppName("echo-prt-bond-restart")
	_, err := deployApplication(s.ctx, s.appName,
		envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp"),
		"--salt", uniqueSalt(), prtFlag, claimStagingPeriodFlag, "5")
	r.NoError(err, "deploy the recovery restart application")
	waitForTournamentAndCommitment(s.ctx, s.T(), r, s.appName, 0)

	dsn, err := config.GetDatabaseConnection()
	r.NoError(err)
	repo, err := factory.NewRepositoryFromConnectionString(s.ctx, dsn.Raw())
	r.NoError(err)
	defer repo.Close()
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
	s.publishAcceptedRecoveryFixture(repo, epoch.Index+1, acceptReceipt.BlockNumber.Uint64())
	accepted, err := repo.GetEpoch(s.ctx, s.appName, epoch.Index)
	r.NoError(err)
	r.NotNil(accepted)
	r.Equal(model.EpochStatus_ClaimAccepted, accepted.Status)
	r.NotNil(accepted.ClaimTransactionHash)
	r.Equal(acceptReceipt.TxHash, *accepted.ClaimTransactionHash)
	app, err = repo.GetApplication(s.ctx, s.appName)
	r.NoError(err)
	r.Equal(acceptReceipt.BlockNumber.Uint64(), app.LastTournamentCheckBlock)
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
	nextRoot := waitForTournamentAndCommitment(s.ctx, s.T(), r, s.appName, 1)
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
	recovered := s.waitForOwnedRootBondRecovery(root, commitment)
	r.Greater(recovered.Raw.BlockNumber, acceptReceipt.BlockNumber.Uint64(), "the old bond must be recovered after restart")
	r.Less(nextCommitment.BlockNumber, recovered.Raw.BlockNumber, "the new root's join must precede old-bond recovery")
	nextContract, err := itournament.NewITournament(nextRoot.Address, s.ethClient)
	r.NoError(err)
	atRecovery := &bind.CallOpts{Context: s.ctx, BlockNumber: new(big.Int).SetUint64(recovered.Raw.BlockNumber)}
	nextRecovery, err := nextContract.BondRecovery(atRecovery)
	r.NoError(err)
	r.Equal(uint8(bondDispositionTournamentRunning), nextRecovery.Disposition,
		"older recovery must run during the current root's known wait")
	joined, err := nextContract.CommitmentStanding(atRecovery, *nextEpoch.Commitment)
	r.NoError(err)
	r.True(joined.Joined)
	r.Equal(commitment.SubmitterAddress, joined.Claimer)
}

func (s *EchoPrtSuite) publishAcceptedRecoveryFixture(repo repository.Repository, nextEpoch, acceptanceBlock uint64) {
	s.T().Helper()
	r := s.Require()
	// Run the real publication paths without the advancer or validator. The new
	// epoch stays unprepared, so PRT cannot select a recovery slot in this tick.
	readerConfig, prtConfig, err := loadRecoveryFixtureConfig()
	r.NoError(err)
	readerService, err := evmreader.Create(s.ctx, &evmreader.CreateInfo{
		Config: *readerConfig, Repository: repo, EthClient: s.ethClient,
	})
	r.NoError(err)
	reader, ok := readerService.(*evmreader.Service)
	r.True(ok)
	_, err = reader.Tick(s.ctx)
	r.NoError(err)
	app, err := repo.GetApplication(s.ctx, s.appName)
	r.NoError(err)
	r.Equal(acceptanceBlock, app.LastEpochCheckBlock, "the reader must publish the acceptance event")
	epoch, err := repo.GetEpoch(s.ctx, s.appName, nextEpoch)
	r.NoError(err)
	r.NotNil(epoch)
	r.Equal(model.EpochStatus_Closed, epoch.Status)
	r.False(epoch.HasCompleteStateProof())
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
	r.Equal(acceptanceBlock, head, "fixture publication must not send a transaction")
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
