// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package claimer

import (
	"context"
	"errors"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthorityfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
	"io"
	"math/big"
	"os"
	"testing"
	"time"
)

// Faults affect the sender's provider boundary. Observation and every successful
// request use real Anvil state; repository selectors and writes use PostgreSQL.
type acceptanceFaultBackend struct {
	txsender.ProtectedClient
	callError    error
	loseReply    bool
	publications []*types.Transaction
	failedCalls  int
}

func (b *acceptanceFaultBackend) Call(ctx context.Context, call ethereum.CallMsg, ref txsender.BlockRef) ([]byte, error) {
	if b.callError != nil {
		b.failedCalls++
		return nil, b.callError
	}
	return b.ProtectedClient.Call(ctx, call, ref)
}

func (b *acceptanceFaultBackend) Publish(ctx context.Context, raw []byte) (common.Hash, error) {
	hash, err := b.ProtectedClient.Publish(ctx, raw)
	if err != nil {
		return hash, err
	}
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		return hash, err
	}
	b.publications = append(b.publications, tx)
	if b.loseReply {
		return common.Hash{}, io.ErrUnexpectedEOF
	}
	return hash, nil
}

func TestAnvilAuthorityAcceptanceRecoversFromOperationalFaults(t *testing.T) {
	if os.Getenv("CARTESI_CLAIMER_ANVIL_TEST") != "1" {
		t.Skip("set CARTESI_CLAIMER_ANVIL_TEST=1 and the isolated capacity admin connection for this release fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	repo := newCapacityRepository(ctx, t)
	client, signer, chainID := capacityChain(ctx, t)
	_, err := repo.RegisterObserver(ctx, "evm-reader", config.PersistentChainConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: chainID.Uint64(),
	})
	require.NoError(t, err)
	app, epoch := seedAcceptanceApplication(ctx, t, repo, client, signer)
	s := newCapacityService(ctx, t, repo, client, signer, chainID)
	backend := &acceptanceFaultBackend{ProtectedClient: txsender.ProtectedClient{Client: client}}
	s.publisher.backend = backend
	s.blockchain.(*claimerBlockchain).backend = backend
	s.publisher.sender, err = txsender.New[claimAction](backend, ethutil.NewStaticTransactOptsFactory(signer), chainID.Uint64(),
		txsender.Policy{MaxFee: big.NewInt(1_000_000_000_000)},
		s.guardClaimPublication, s.Logger)
	require.NoError(t, err)
	consensus, err := iconsensus.NewIConsensus(app.IConsensusAddress, client)
	require.NoError(t, err)
	tx, err := consensus.SubmitClaim(signer, app.IApplicationAddress, new(big.Int).SetUint64(epoch.LastBlock),
		[32]byte(*epoch.MachineHash), consensusMachineValidityProof(mustEpochProof(t, epoch)))
	require.NoError(t, err)
	staged := anvilClaimMined(ctx, t, client, tx)
	anvilClaimFinalized(ctx, t, client, staged.BlockNumber.Uint64())
	setAcceptanceBalance(ctx, t, client, signer.From, "0x0")

	// More passes than the old attempt limit must not disable the application.
	for range 7 {
		clearAcceptanceCooldowns(s)
		tickAcceptance(ctx, t, s)
		assertAcceptanceRunnable(ctx, t, repo, app)
	}
	require.Empty(t, backend.publications)
	stored, err := repo.GetEpoch(ctx, app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimStaged, stored.Status, "Observe records the real staged claim")
	setAcceptanceBalance(ctx, t, client, signer.From, "0x56bc75e2d63100000")
	for _, code := range []int{429, 502} {
		before := backend.failedCalls
		backend.callError = rpc.HTTPError{StatusCode: code, Status: "injected preparation failure"}
		for range 7 {
			clearAcceptanceCooldowns(s)
			tickAcceptance(ctx, t, s)
			assertAcceptanceRunnable(ctx, t, repo, app)
		}
		require.GreaterOrEqual(t, backend.failedCalls-before, 7)
		require.Empty(t, backend.publications)
	}

	backend.callError = nil
	backend.loseReply = true
	clearAcceptanceCooldowns(s)
	tickAcceptance(ctx, t, s)
	require.Len(t, backend.publications, 1, "restored preparation publishes without manual enablement")
	accept := backend.publications[0]
	mined := anvilClaimMined(ctx, t, client, accept)
	require.Equal(t, uint64(types.ReceiptStatusSuccessful), mined.Status)
	anvilClaimFinalized(ctx, t, client, mined.BlockNumber.Uint64())
	for range 7 {
		tickAcceptance(ctx, t, s)
		assertAcceptanceRunnable(ctx, t, repo, app)
	}
	backend.loseReply = false
	_, err = s.Tick(ctx)
	require.NoError(t, err)
	require.NoError(t, s.publisher.tick(ctx))
	assertAcceptanceRunnable(ctx, t, repo, app)
	stored, err = repo.GetEpoch(ctx, app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimAccepted, stored.Status)
	require.Len(t, backend.publications, 1, "recovery must not send a second acceptance")
}

func tickAcceptance(ctx context.Context, t *testing.T, s *Service) {
	t.Helper()
	_, observeErr := s.Tick(ctx)
	err := errors.Join(observeErr, s.publisher.tick(ctx))
	if err != nil {
		require.False(t, txsender.HardFailure(err), "operational fault became a hard failure: %v", err)
		t.Logf("operational wait: %v", err)
	}
}

func clearAcceptanceCooldowns(s *Service) {
	// Advance only the fixture's wall-clock hints; do not wait minutes between
	// faults or reset the current request or DB state.
	for key := range s.publisher.cooldowns {
		s.publisher.cooldowns[key] = time.Time{}
	}
}

func assertAcceptanceRunnable(ctx context.Context, t *testing.T, repo repository.Repository, app *model.Application) {
	t.Helper()
	stored, err := repo.GetApplication(ctx, app.Name)
	require.NoError(t, err)
	require.True(t, stored.Enabled)
	require.Equal(t, model.ApplicationStatus_OK, stored.Status)
}

func setAcceptanceBalance(ctx context.Context, t *testing.T, client *ethclient.Client, address common.Address, value string) {
	t.Helper()
	var result any
	require.NoError(t, client.Client().CallContext(ctx, &result, "anvil_setBalance", address, value))
}

func seedAcceptanceApplication(
	ctx context.Context, t *testing.T, repo repository.Repository, client *ethclient.Client, signer *bind.TransactOpts,
) (*model.Application, *model.Epoch) {
	t.Helper()
	factory, err := iauthorityfactory.NewIAuthorityFactory(anvilFixtureAddress(
		"CARTESI_CONTRACTS_AUTHORITY_FACTORY_ADDRESS", "0xB4d29c86e36385b5321a453C34D288AEB0ad11f9"), client)
	require.NoError(t, err)
	salt := crypto.Keccak256Hash(signer.From.Bytes())
	consensus, err := factory.CalculateAuthorityAddress(&bind.CallOpts{Context: ctx}, signer.From, big.NewInt(1), new(big.Int), salt)
	require.NoError(t, err)
	tx, err := factory.NewAuthority0(signer, signer.From, big.NewInt(1), new(big.Int), salt)
	require.NoError(t, err)
	anvilClaimMined(ctx, t, client, tx)
	appFactory, err := iapplicationfactory.NewIApplicationFactory(anvilFixtureAddress(
		"CARTESI_CONTRACTS_APPLICATION_FACTORY_ADDRESS", "0x35Cd91f13141Bb6A6FC69E1eeDD241bbA1Ddd45F"), client)
	require.NoError(t, err)
	inputBox := anvilFixtureAddress("CARTESI_CONTRACTS_INPUT_BOX_ADDRESS", "0xEbE9f4Dfc04ae10bBeE663859c3dc5A23f94eA3C")
	withdrawal := iapplicationfactory.WithdrawalConfig{}
	address, err := appFactory.CalculateApplicationAddress(
		&bind.CallOpts{Context: ctx}, consensus, signer.From, salt, inputBox, withdrawal, salt)
	require.NoError(t, err)
	tx, err = appFactory.NewApplication0(signer, consensus, signer.From, salt, inputBox, withdrawal, salt)
	require.NoError(t, err)
	deployed := anvilClaimMined(ctx, t, client, tx).BlockNumber.Uint64()
	anvilClaimFinalized(ctx, t, client, deployed)
	app := repotest.NewApplicationBuilder().WithName("acceptance-recovery").WithAddress(address).
		WithEpochLength(1).WithClaimStagingPeriod(0).WithWithdrawalConfig(model.WithdrawalConfig{}).Build()
	app.IConsensusAddress, app.IInputBoxAddress, app.TemplateHash = consensus, inputBox, salt
	app.IInputBoxBlock, app.LastInputCheckBlock = deployed, deployed
	app.ID, err = repo.CreateApplication(ctx, app, false)
	require.NoError(t, err)
	epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).
		WithBlocks(deployed, deployed).WithInputBounds(0, 0).Build()
	require.NoError(t, repo.CreateEpochsAndInputs(ctx, app.Name, map[*model.Epoch][]*model.Input{epoch: nil}, deployed))
	proof := repotest.KeccakStateProof(salt)
	require.NoError(t, repo.UpdateEpochInputsProcessed(ctx, app.Name, 0, &proof))
	epoch, err = repo.GetEpoch(ctx, app.Name, 0)
	require.NoError(t, err)
	repotest.AdvanceEpochStatus(ctx, t, repo, app.Name, epoch, model.EpochStatus_ClaimComputed)
	epoch, err = repo.GetEpoch(ctx, app.Name, 0)
	require.NoError(t, err)
	return app, epoch
}
