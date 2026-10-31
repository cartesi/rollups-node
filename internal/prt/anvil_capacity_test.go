// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package prt

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres"
	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveappfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

const (
	prtCapacityApplications = 16
	prtCapacityPoll         = 3 * time.Second
	prtCapacityLimit        = 120 * time.Second
	prtCapacityFixtureLimit = 6 * time.Minute
)

type prtPublication struct {
	hash  common.Hash
	nonce uint64
	at    time.Time
}

type prtCapacityBackend struct {
	txsender.ProtectedClient
	mu           sync.Mutex
	publications map[common.Address]prtPublication
}

func (b *prtCapacityBackend) Publish(ctx context.Context, raw []byte) (common.Hash, error) {
	hash, err := b.ProtectedClient.Publish(ctx, raw)
	if err != nil {
		return hash, err
	}
	tx := new(types.Transaction)
	if err = tx.UnmarshalBinary(raw); err != nil {
		return hash, err
	}
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	if err != nil {
		return hash, err
	}
	if tx.To() != nil && len(tx.Data()) >= 4 && string(tx.Data()[:4]) == string(contractABI.Methods["joinTournament"].ID) {
		b.mu.Lock()
		if _, recorded := b.publications[*tx.To()]; !recorded {
			b.publications[*tx.To()] = prtPublication{hash: hash, nonce: tx.Nonce(), at: time.Now()}
		}
		b.mu.Unlock()
	}
	return hash, nil
}

func (b *prtCapacityBackend) snapshot() map[common.Address]prtPublication {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make(map[common.Address]prtPublication, len(b.publications))
	for root, publication := range b.publications {
		result[root] = publication
	}
	return result
}

// This fixture measures one healthy deployment profile. Every nonce goes
// through the production Observe and dispatch pass at the default cadence.
// It uses a fresh PostgreSQL database and random funded signer, and relies on
// the devnet's natural producer without snapshots, mining, or resetting state.
func TestAnvilPRTInitialSixteenApplicationCapacity(t *testing.T) {
	if os.Getenv("CARTESI_PRT_CAPACITY_TEST") != "1" {
		t.Skip("set CARTESI_PRT_CAPACITY_TEST=1 for the isolated PostgreSQL + Anvil PRT capacity fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), prtCapacityFixtureLimit)
	defer cancel()
	repo := newPRTCapacityRepository(ctx, t)
	client, signer, chainID := prtCapacityChain(ctx, t)
	_, err := repo.RegisterObserver(ctx, "evm-reader", config.PersistentChainConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: chainID.Uint64(),
	})
	require.NoError(t, err)
	apps, roots, lastDeployment := seedPRTCapacityApplications(ctx, t, repo, client, signer)
	prtCapacityFinalized(ctx, t, client, lastDeployment)
	backend := &prtCapacityBackend{ProtectedClient: txsender.ProtectedClient{Client: client},
		publications: make(map[common.Address]prtPublication)}
	s := newPRTCapacityService(ctx, t, repo, client, signer, chainID, backend)
	startHeader, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err)
	for _, root := range roots {
		adapter, err := NewITournamentAdapter(root, client, ethutil.Filter{})
		require.NoError(t, err)
		descriptor, err := adapter.Descriptor(&bind.CallOpts{Context: ctx})
		require.NoError(t, err)
		require.EqualValues(t, model.Log2EpochComputationHashLeafCount, descriptor.Height)
		require.Less(t, startHeader.Number.Uint64()+1, descriptor.StartInstant+descriptor.Allowance,
			"all sixteen roots must still accept joins at the common measurement start")
		t.Logf("initial root=%s age=%d allowance=%d", root, startHeader.Number.Uint64()-descriptor.StartInstant, descriptor.Allowance)
	}
	started := time.Now()
	run, stop := context.WithCancel(ctx)
	finished := make(chan error, 1)
	go func() { finished <- s.Serve(run) }()
	defer func() {
		stop()
		require.ErrorIs(t, <-finished, context.Canceled)
	}()
	require.Eventually(t, func() bool { return len(backend.snapshot()) == len(roots) }, prtCapacityLimit, 250*time.Millisecond,
		"every initially ready application must receive its first join publication")
	publications := backend.snapshot()
	nonces := make(map[uint64]bool, len(roots))
	var lastDispatch time.Duration
	var lastReceipt uint64
	for index, root := range roots {
		publication, ok := publications[root]
		require.True(t, ok)
		latency := publication.at.Sub(started)
		require.Less(t, latency, prtCapacityLimit)
		lastDispatch = max(lastDispatch, latency)
		require.False(t, nonces[publication.nonce], "every initial join follows consumption of the preceding nonce")
		nonces[publication.nonce] = true
		tx, _, err := client.TransactionByHash(ctx, publication.hash)
		require.NoError(t, err)
		receipt := prtCapacityMined(ctx, t, client, tx)
		lastReceipt = max(lastReceipt, receipt.BlockNumber.Uint64())
		adapter, err := NewITournamentAdapter(root, client, ethutil.Filter{})
		require.NoError(t, err)
		descriptor, err := adapter.Descriptor(&bind.CallOpts{Context: ctx})
		require.NoError(t, err)
		require.Less(t, receipt.BlockNumber.Uint64(), descriptor.StartInstant+descriptor.Allowance,
			"join execution must precede the strict on-chain deadline")
		t.Logf("initial app=%s nonce=%d dispatch=%s receipt=%d gas_used=%d", apps[index].Name, publication.nonce,
			latency.Round(time.Millisecond), receipt.BlockNumber.Uint64(), receipt.GasUsed)
	}
	endHeader, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err)
	t.Logf("measured PRT capacity: apps=%d poll=%s last_dispatch=%s natural_blocks=%d block_timestamp_span=%ds",
		len(apps), prtCapacityPoll, lastDispatch.Round(time.Millisecond),
		endHeader.Number.Uint64()-startHeader.Number.Uint64(), endHeader.Time-startHeader.Time)
	prtCapacityFinalized(ctx, t, client, lastReceipt)
	require.Eventually(t, func() bool { return prtCapacityJoinsObserved(ctx, repo, apps, roots, signer.From) },
		30*time.Second, 250*time.Millisecond, "Observe must project every finalized join into PostgreSQL")
	require.True(t, s.Ready())
}

func prtCapacityJoinsObserved(
	ctx context.Context, repo repository.Repository, apps []*model.Application, roots []common.Address, signer common.Address,
) bool {
	for index, app := range apps {
		epoch, err := repo.GetEpoch(ctx, app.Name, 0)
		if err != nil || epoch == nil || epoch.Commitment == nil {
			return false
		}
		commitment, err := repo.GetCommitment(ctx, app.Name, 0, roots[index].Hex(), epoch.Commitment.Hex())
		if err != nil || commitment == nil || commitment.Snapshot.Claimer != signer {
			return false
		}
	}
	return true
}

func prtCapacityChain(ctx context.Context, t *testing.T) (*ethclient.Client, *bind.TransactOpts, *big.Int) {
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
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	require.NoError(t, err)
	signer.Context = ctx
	var funded any
	require.NoError(t, client.Client().CallContext(ctx, &funded, "anvil_setBalance", signer.From, "0x56bc75e2d63100000"))
	return client, signer, chainID
}

func prtCapacityMined(ctx context.Context, t *testing.T, client *ethclient.Client, tx *types.Transaction) *types.Receipt {
	t.Helper()
	receipt, err := bind.WaitMined(ctx, client, tx)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status)
	return receipt
}

func prtCapacityFinalized(ctx context.Context, t *testing.T, client *ethclient.Client, atLeast uint64) {
	t.Helper()
	wait, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	backend := txsender.ProtectedClient{Client: client}
	require.Eventually(t, func() bool {
		header, err := backend.Header(wait, "finalized")
		if err != nil || header.Number.Uint64() < atLeast {
			return false
		}
		latest, err := backend.Header(wait, "latest")
		require.NoError(t, err)
		t.Logf("natural producer: latest=%d finalized=%d finality_lag=%d", latest.Number.Uint64(), header.Number.Uint64(),
			latest.Number.Uint64()-header.Number.Uint64())
		return true
	}, 90*time.Second, 250*time.Millisecond, "deployment/receipt must become naturally finalized")
}

func seedPRTCapacityApplications(
	ctx context.Context, t *testing.T, repo repository.Repository, client *ethclient.Client, signer *bind.TransactOpts,
) ([]*model.Application, []common.Address, uint64) {
	t.Helper()
	factoryAddress := os.Getenv("CARTESI_CONTRACTS_DAVE_APP_FACTORY_ADDRESS")
	if factoryAddress == "" {
		factoryAddress = "0xF8f6dFA3A3e40C10926839FFd8a221C87C8b2ccD"
	}
	factory, err := idaveappfactory.NewIDaveAppFactory(common.HexToAddress(factoryAddress), client)
	require.NoError(t, err)
	apps := make([]*model.Application, 0, prtCapacityApplications)
	roots := make([]common.Address, 0, prtCapacityApplications)
	var lastDeployment uint64
	for index := range prtCapacityApplications {
		salt := crypto.Keccak256Hash(signer.From.Bytes(), big.NewInt(int64(index)).Bytes())
		withdrawal := idaveappfactory.WithdrawalConfig{}
		addresses, err := factory.CalculateDaveAppAddress(&bind.CallOpts{Context: ctx}, salt, big.NewInt(1_000_000),
			signer.From, nil, withdrawal, salt)
		require.NoError(t, err)
		tx, err := factory.NewDaveApp(signer, salt, big.NewInt(1_000_000), signer.From, nil, withdrawal, salt)
		require.NoError(t, err)
		receipt := prtCapacityMined(ctx, t, client, tx)
		lastDeployment = receipt.BlockNumber.Uint64()
		consensus, err := NewDaveConsensusAdapter(addresses.DaveConsensusAddress, client)
		require.NoError(t, err)
		sealed, err := consensus.GetCurrentSealedEpoch(&bind.CallOpts{Context: ctx})
		require.NoError(t, err)
		require.Zero(t, sealed.EpochNumber)
		app := repotest.NewApplicationBuilder().WithName(fmt.Sprintf("prt-capacity-%02d", index)).
			WithAddress(addresses.AppContractAddress).WithConsensus(model.Consensus_PRT).WithEpochLength(1).
			WithClaimStagingPeriod(1_000_000).WithWithdrawalConfig(model.WithdrawalConfig{}).Build()
		app.IConsensusAddress, app.TemplateHash = addresses.DaveConsensusAddress, salt
		app.IInputBoxAddress = common.HexToAddress("0xEbE9f4Dfc04ae10bBeE663859c3dc5A23f94eA3C")
		app.IInputBoxBlock, app.LastInputCheckBlock = lastDeployment, lastDeployment
		app.ID, err = repo.CreateApplication(ctx, app, false)
		require.NoError(t, err)
		epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).
			WithBlocks(lastDeployment, lastDeployment).
			WithInputBounds(sealed.InputIndexLowerBound, sealed.InputIndexUpperBound).Build()
		epoch.TournamentAddress = &sealed.Tournament
		require.NoError(t, repo.CreateEpochsAndInputs(ctx, app.Name, map[*model.Epoch][]*model.Input{epoch: nil}, lastDeployment))
		proof := repotest.KeccakStateProof(salt)
		require.NoError(t, repo.UpdateEpochInputsProcessed(ctx, app.Name, 0, &proof))
		epoch, err = repo.GetEpoch(ctx, app.Name, 0)
		require.NoError(t, err)
		epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
		tree := merkle.Proof{Node: *epoch.MachineHash, Pos: big.NewInt(1<<model.Log2EpochComputationHashLeafCount - 1),
			Siblings: epoch.CommitmentProof}
		epoch.Commitment = new(tree.BuildRoot())
		require.NoError(t, repo.StoreClaimAndProofs(ctx, epoch, nil))
		apps, roots = append(apps, app), append(roots, sealed.Tournament)
	}
	return apps, roots, lastDeployment
}

func newPRTCapacityService(
	ctx context.Context, t *testing.T, repo repository.Repository, client *ethclient.Client,
	signer *bind.TransactOpts, chainID *big.Int, backend txsender.Backend,
) *Service {
	t.Helper()
	svc, err := Create(ctx, &CreateInfo{
		Config: config.PrtConfig{
			PrtPollingInterval: prtCapacityPoll, BlockchainId: chainID.Uint64(), BlockchainDefaultBlock: model.DefaultBlock_Finalized,
			FeatureClaimSubmissionEnabled: true, BlockchainHttpRequestTimeout: 10 * time.Second, BlockchainTxMaxFeePerGas: "1000000000000"},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), EthClient: client, Repository: repo, DeliveryBackend: backend,
		TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(signer),
	})
	require.NoError(t, err)
	return svc.(*Service)
}

func newPRTCapacityRepository(ctx context.Context, t *testing.T) repository.Repository {
	t.Helper()
	endpoint := os.Getenv("CARTESI_PRT_CAPACITY_ADMIN_CONNECTION")
	require.NotEmpty(t, endpoint, "CARTESI_PRT_CAPACITY_ADMIN_CONNECTION explicitly authorizes a fresh isolated database")
	admin, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, admin.Close(closeCtx))
	})
	name := fmt.Sprintf("prt_capacity_%d", time.Now().UnixNano())
	_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	require.NoError(t, err)
	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
