// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/cartesistatetransition"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/imultileveltournamentfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
)

func (s *EchoPrtSuite) TestEchoPrtDeploymentPins() {
	r := s.Require()
	s.SetExpectedLogs(s.T(), anvilBlockOutOfRangeAllowlist)
	s.appName = uniqueAppName("echo-prt-deployment")
	_, err := deployApplication(s.ctx, s.appName,
		envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp"),
		prtFlag, "--salt", uniqueSalt())
	r.NoError(err)
	dsn, err := config.GetDatabaseConnection()
	r.NoError(err)
	repo, err := factory.NewRepositoryFromConnectionString(s.ctx, dsn.Raw())
	r.NoError(err)
	defer repo.Close()
	readerConfig, _, err := loadRecoveryFixtureConfig()
	r.NoError(err)
	tournament := s.waitForRecoveryFixtureCommitment(repo, readerConfig.BlockchainDefaultBlock, 0)
	app, err := repo.GetApplication(s.ctx, s.appName)
	r.NoError(err)
	r.NotNil(app)
	r.Equal(model.ApplicationStatus_OK, app.Status)
	epoch, err := repo.GetEpoch(s.ctx, s.appName, 0)
	r.NoError(err)
	r.NotNil(epoch)
	r.NotNil(epoch.Commitment)
	commitment, err := repo.GetCommitment(s.ctx, s.appName, 0, tournament.Address.Hex(), epoch.Commitment.Hex())
	r.NoError(err)
	r.NotNil(commitment)
	opts := &bind.CallOpts{Context: s.ctx, BlockNumber: new(big.Int).SetUint64(commitment.BlockNumber)}
	consensus, err := idaveconsensus.NewIDaveConsensus(app.IConsensusAddress, s.ethClient)
	r.NoError(err)
	factoryAddress, err := consensus.GetTournamentFactory(opts)
	r.NoError(err)
	tournamentFactory, err := imultileveltournamentfactory.NewIMultiLevelTournamentFactory(factoryAddress, s.ethClient)
	r.NoError(err)
	transitionAddress, err := tournamentFactory.StateTransition(opts)
	r.NoError(err)
	transition, err := cartesistatetransition.NewCartesiStateTransition(transitionAddress, s.ethClient)
	r.NoError(err)
	marchID, err := transition.CMMARCHID(opts)
	r.NoError(err)
	r.Equal(model.CartesiMachineMarchID, marchID)
	pristine, err := transition.UARCHPRISTINESTATEHASH(opts)
	r.NoError(err)
	r.Equal(model.UarchPristineStateHash(), common.Hash(pristine))
	levels, err := tournamentFactory.TournamentLevelCount(opts)
	r.NoError(err)
	r.Equal(model.TournamentLevelCount, levels)
	for level := uint64(0); level < levels; level++ {
		parameters, readErr := tournamentFactory.TournamentParameters(opts, level)
		r.NoError(readErr)
		stride, height, geometryErr := model.TournamentLevelGeometry(level)
		r.NoError(geometryErr)
		r.Equal(levels, parameters.Levels)
		r.Equal(stride, parameters.Log2step)
		r.Equal(height, parameters.Height)
	}
	root, err := itournament.NewITournament(tournament.Address, s.ethClient)
	r.NoError(err)
	bond, err := root.BondValue(opts)
	r.NoError(err)
	r.Positive(bond.Sign())
	s.T().Logf("root %s bondValue=%s", tournament.Address, bond)
	events, err := root.FilterCommitmentJoined(&bind.FilterOpts{
		Context: s.ctx, Start: commitment.BlockNumber, End: &commitment.BlockNumber,
	}, [][32]byte{commitment.Commitment}, []common.Address{commitment.SubmitterAddress})
	r.NoError(err)
	defer func() { r.NoError(events.Close()) }()
	r.True(events.Next(), "the node must emit its own CommitmentJoined")
	joined := events.Event
	r.False(events.Next(), "the node must join this root once")
	r.NoError(events.Error())
	r.Equal(commitment.Commitment, common.Hash(joined.Commitment))
	r.Equal(commitment.FinalStateHash, common.Hash(joined.FinalStateHash))
	r.Equal(commitment.TxHash, joined.Raw.TxHash)
	r.Equal(commitment.LogIndex, uint64(joined.Raw.Index))
	s.requirePrtEventReceipt(joined.Raw)
	tx, pending, err := s.ethClient.TransactionByHash(s.ctx, joined.Raw.TxHash)
	r.NoError(err)
	r.False(pending)
	r.Equal(&tournament.Address, tx.To())
	r.Zero(tx.Value().Cmp(bond), "the node must pay exactly bondValue")
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	r.NoError(err)
	r.True(bytes.HasPrefix(tx.Data(), contractABI.Methods["joinTournament"].ID))
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	r.NoError(err)
	r.Equal(commitment.SubmitterAddress, sender)
	app, err = repo.GetApplication(s.ctx, s.appName)
	r.NoError(err)
	r.NotNil(app)
	r.Equal(model.ApplicationStatus_OK, app.Status, "compatible deployment must remain healthy")
}
