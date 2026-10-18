// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package claimer

import (
	"context"
	"fmt"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres"
	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthorityfactory"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"io"
	"log/slog"
	"math/big"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"
)

const (
	capacityApplications = 16
	capacityPollInterval = 3 * time.Second // the configured Claimer default
	capacityDispatchTime = 120 * time.Second
)

type claimPublication struct {
	hash  common.Hash
	nonce uint64
	at    time.Time
}

type capacityBackend struct {
	txsender.ProtectedClient
	mu           sync.Mutex
	publications map[common.Address]claimPublication
}

func (b *capacityBackend) Publish(ctx context.Context, raw []byte) (common.Hash, error) {
	hash, err := b.ProtectedClient.Publish(ctx, raw)
	if err != nil {
		return hash, err
	}
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		return hash, err
	}
	if data := tx.Data(); len(data) >= 36 {
		app := common.BytesToAddress(data[16:36])
		b.mu.Lock()
		if _, recorded := b.publications[app]; !recorded {
			b.publications[app] = claimPublication{hash: hash, nonce: tx.Nonce(), at: time.Now()}
		}
		b.mu.Unlock()
	}
	return hash, nil
}

func (b *capacityBackend) snapshot() map[common.Address]claimPublication {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make(map[common.Address]claimPublication, len(b.publications))
	for app, publication := range b.publications {
		result[app] = publication
	}
	return result
}

// This release fixture measures an initial burst with real PostgreSQL selectors,
// the production Serve loop and naturally produced Anvil blocks.
// A fresh database and random funded signer keep it independent of other tests.
func TestAnvilClaimerInitialSixteenApplicationCapacity(t *testing.T) {
	if os.Getenv("CARTESI_CLAIMER_CAPACITY_TEST") != "1" {
		t.Skip("set CARTESI_CLAIMER_CAPACITY_TEST=1 for the isolated PostgreSQL + Anvil capacity fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	repo := newCapacityRepository(ctx, t)
	client, signer, chainID := capacityChain(ctx, t)
	_, err := repo.RegisterObserver(ctx, "evm-reader", config.PersistentChainConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: chainID.Uint64(),
	})
	require.NoError(t, err)
	apps, deployment := seedCapacityApplications(ctx, t, repo, client, signer)
	anvilClaimFinalized(ctx, t, client, deployment)
	s := newCapacityService(ctx, t, repo, client, signer, chainID)
	backend := &capacityBackend{ProtectedClient: txsender.ProtectedClient{Client: client},
		publications: make(map[common.Address]claimPublication)}
	s.publisher.backend = backend
	s.blockchain.(*claimerBlockchain).backend = backend
	s.publisher.sender, err = txsender.New[claimAction](backend, ethutil.NewStaticTransactOptsFactory(signer), chainID.Uint64(),
		txsender.Policy{MaxFee: big.NewInt(1_000_000_000_000)},
		s.guardClaimPublication, s.Logger)
	require.NoError(t, err)
	startHeader, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err)
	started := time.Now()
	run, stop := context.WithCancel(ctx)
	finished := make(chan error, 1)
	go func() { finished <- s.Serve(run) }()
	defer func() {
		stop()
		require.ErrorIs(t, <-finished, context.Canceled)
	}()
	require.Eventually(t, func() bool { return len(backend.snapshot()) == len(apps) }, capacityDispatchTime, 250*time.Millisecond,
		"every initially ready application must receive its first publication without starvation")
	publications := backend.snapshot()
	var lastDispatch time.Duration
	nonces := make(map[uint64]bool, len(apps))
	var lastReceipt uint64
	for _, app := range apps {
		publication, ok := publications[app.IApplicationAddress]
		require.True(t, ok, "application %s received no publication", app.Name)
		latency := publication.at.Sub(started)
		require.Less(t, latency, capacityDispatchTime)
		lastDispatch = max(lastDispatch, latency)
		require.False(t, nonces[publication.nonce], "distinct initial actions use distinct consumed nonces")
		nonces[publication.nonce] = true
		tx, _, err := client.TransactionByHash(ctx, publication.hash)
		require.NoError(t, err)
		receipt := anvilClaimMined(ctx, t, client, tx)
		lastReceipt = max(lastReceipt, receipt.BlockNumber.Uint64())
		t.Logf("initial app=%s nonce=%d dispatch=%s receipt=%d gas_used=%d", app.Name, publication.nonce,
			latency.Round(time.Millisecond), receipt.BlockNumber.Uint64(), receipt.GasUsed)
	}
	endHeader, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err)
	blocks := endHeader.Number.Uint64() - startHeader.Number.Uint64()
	t.Logf("measured capacity: apps=%d poll=%s last_dispatch=%s natural_blocks=%d block_timestamp_span=%ds",
		len(apps), capacityPollInterval, lastDispatch.Round(time.Millisecond), blocks, endHeader.Time-startHeader.Time)
	anvilClaimFinalized(ctx, t, client, lastReceipt)
	require.Eventually(t, func() bool { return capacityClaimsStaged(ctx, repo, apps) }, 30*time.Second, 250*time.Millisecond,
		"Observe must project every finalized claim into PostgreSQL")
	require.True(t, s.Ready())
}

func capacityClaimsStaged(ctx context.Context, repo repository.Repository, apps []*model.Application) bool {
	for _, app := range apps {
		epoch, err := repo.GetEpoch(ctx, app.Name, 0)
		if err != nil || epoch == nil || epoch.Status != model.EpochStatus_ClaimStaged {
			return false
		}
	}
	return true
}

func capacityChain(ctx context.Context, t *testing.T) (*ethclient.Client, *bind.TransactOpts, *big.Int) {
	t.Helper()
	endpoint := os.Getenv("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8545"
	}
	client, err := ethclient.DialContext(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	chainID, err := client.ChainID(ctx)
	require.NoError(t, err)
	return client, anvilClaimSigner(ctx, t, client, chainID), chainID
}

func seedCapacityApplications(
	ctx context.Context, t *testing.T, repo repository.Repository, client *ethclient.Client, signer *bind.TransactOpts,
) ([]*model.Application, uint64) {
	t.Helper()
	factory, err := iauthorityfactory.NewIAuthorityFactory(anvilFixtureAddress(
		"CARTESI_CONTRACTS_AUTHORITY_FACTORY_ADDRESS", "0xB4d29c86e36385b5321a453C34D288AEB0ad11f9"), client)
	require.NoError(t, err)
	salt := crypto.Keccak256Hash(signer.From.Bytes())
	stagingPeriod := big.NewInt(1_000_000) // acceptance cannot preempt this measured initial submission burst
	consensus, err := factory.CalculateAuthorityAddress(&bind.CallOpts{Context: ctx}, signer.From, big.NewInt(1), stagingPeriod, salt)
	require.NoError(t, err)
	tx, err := factory.NewAuthority0(signer, signer.From, big.NewInt(1), stagingPeriod, salt)
	require.NoError(t, err)
	anvilClaimMined(ctx, t, client, tx)
	appFactory, err := iapplicationfactory.NewIApplicationFactory(anvilFixtureAddress(
		"CARTESI_CONTRACTS_APPLICATION_FACTORY_ADDRESS", "0x35Cd91f13141Bb6A6FC69E1eeDD241bbA1Ddd45F"), client)
	require.NoError(t, err)
	inputBox := anvilFixtureAddress("CARTESI_CONTRACTS_INPUT_BOX_ADDRESS", "0xEbE9f4Dfc04ae10bBeE663859c3dc5A23f94eA3C")
	apps := make([]*model.Application, 0, capacityApplications)
	var lastDeployment uint64
	for index := range capacityApplications {
		appSalt := crypto.Keccak256Hash(salt.Bytes(), big.NewInt(int64(index)).Bytes())
		withdrawal := iapplicationfactory.WithdrawalConfig{}
		address, err := appFactory.CalculateApplicationAddress(
			&bind.CallOpts{Context: ctx}, consensus, signer.From, appSalt, inputBox, withdrawal, appSalt)
		require.NoError(t, err)
		tx, err := appFactory.NewApplication0(signer, consensus, signer.From, appSalt, inputBox, withdrawal, appSalt)
		require.NoError(t, err)
		receipt := anvilClaimMined(ctx, t, client, tx)
		lastDeployment = receipt.BlockNumber.Uint64()
		app := repotest.NewApplicationBuilder().WithName(fmt.Sprintf("capacity-%02d", index)).WithAddress(address).
			WithEpochLength(1).WithClaimStagingPeriod(stagingPeriod.Uint64()).WithWithdrawalConfig(model.WithdrawalConfig{}).Build()
		app.IConsensusAddress, app.IInputBoxAddress, app.TemplateHash = consensus, inputBox, appSalt
		app.IInputBoxBlock, app.LastInputCheckBlock = lastDeployment, lastDeployment
		app.ID, err = repo.CreateApplication(ctx, app, false)
		require.NoError(t, err)
		epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).
			WithBlocks(lastDeployment, lastDeployment).WithInputBounds(0, 0).Build()
		require.NoError(t, repo.CreateEpochsAndInputs(ctx, app.Name, map[*model.Epoch][]*model.Input{epoch: nil}, lastDeployment))
		proof := repotest.KeccakStateProof(appSalt)
		require.NoError(t, repo.UpdateEpochInputsProcessed(ctx, app.Name, 0, &proof))
		epoch, err = repo.GetEpoch(ctx, app.Name, 0)
		require.NoError(t, err)
		epoch.Status = model.EpochStatus_ClaimComputed
		require.NoError(t, repo.UpdateEpochStatus(ctx, app.Name, epoch))
		apps = append(apps, app)
	}
	return apps, lastDeployment
}

func newCapacityService(
	ctx context.Context, t *testing.T, repo repository.Repository, client *ethclient.Client,
	signer *bind.TransactOpts, chainID *big.Int,
) *Service {
	t.Helper()
	svc, err := Create(ctx, &CreateInfo{
		Config: config.ClaimerConfig{
			ClaimerPollingInterval: capacityPollInterval, BlockchainId: chainID.Uint64(),
			BlockchainDefaultBlock: model.DefaultBlock_Finalized, FeatureClaimSubmissionEnabled: true,
			BlockchainHttpRequestTimeout: 10 * time.Second,
			BlockchainTxMaxFeePerGas:     "1000000000000",
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), EthConn: client, Repository: repo,
		TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(signer),
	})
	require.NoError(t, err)
	return svc.(*Service)
}

func newCapacityRepository(ctx context.Context, t *testing.T) repository.Repository {
	t.Helper()
	endpoint := os.Getenv("CARTESI_CLAIMER_CAPACITY_ADMIN_CONNECTION")
	require.NotEmpty(t, endpoint, "CARTESI_CLAIMER_CAPACITY_ADMIN_CONNECTION explicitly authorizes a fresh isolated database")
	admin, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, admin.Close(closeCtx))
	})
	name := fmt.Sprintf("claimer_capacity_%d", time.Now().UnixNano())
	_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	require.NoError(t, err)
	t.Cleanup(func() {
		// DROP DATABASE waits for a cluster-wide checkpoint. Parallel schema
		// fixtures can delay that checkpoint after every owned connection closed.
		const dropTimeout = 30 * time.Second
		dropCtx, cancel := context.WithTimeout(context.Background(), dropTimeout)
		defer cancel()
		_, err := admin.Exec(dropCtx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
		require.NoError(t, err)
	})
	isolated, err := url.Parse(endpoint)
	require.NoError(t, err)
	require.NotEmpty(t, isolated.Host, "the test database endpoint must be a PostgreSQL URL")
	isolated.Path = "/" + name
	migration, err := schema.New(isolated.String())
	require.NoError(t, err)
	require.NoError(t, migration.Upgrade())
	migration.Close()
	repo, err := postgres.NewPostgresRepository(ctx, isolated.String(), 1, 0)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	return repo
}
