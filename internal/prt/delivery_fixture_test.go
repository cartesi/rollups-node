// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

type deliveryFixture struct {
	d         *delivery
	app       *model.Application
	epoch     *model.Epoch
	repo      *deliveryRepoFake
	backend   *deliveryBackendFake
	block     txsender.BlockRef
	consensus *daveConsensusAdapterMock
	factory   *adapterFactoryMock
}

func newDeliveryFixture(t *testing.T) *deliveryFixture {
	t.Helper()
	service, legacyRepo := newPRTServiceMock()
	app := prtRevertTestApp()
	app.Enabled, app.LastTournamentCheckBlock = true, 100
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	applyPRTStateProof(epoch, repotest.KeccakStateProof(common.HexToHash("0x500")))
	epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	opts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(31337))
	require.NoError(t, err)
	repo := &deliveryRepoFake{prtRepositoryMock: legacyRepo, apps: []*model.Application{app}, signer: opts.From}
	repo.publishRoot(app, epoch, 100)
	legacyRepo.On("GetEpoch", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index).Return(epoch, nil).Maybe()
	backend := &deliveryBackendFake{app: app, code: map[common.Address][]byte{opts.From: nil}, nonce: 7,
		headers: map[uint64]*types.Header{}}
	backend.advance(100)
	backend.advance(110)
	block := txsender.BlockRef{Number: 110, Hash: backend.headers[110].Hash()}
	consensus := newCompatibleDaveConsensusAdapterMock()
	factory := &adapterFactoryMock{}
	factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Maybe()
	service.repository, service.defaultBlock, service.pollInterval = repo, model.DefaultBlock_Finalized, 10*time.Millisecond
	defaultClient := &ethClientMock{}
	expectNumericWindowHeaders(defaultClient)
	defaultClient.On("HeaderByNumber", mock.Anything, mock.MatchedBy(func(number *big.Int) bool {
		return number == nil || number.Sign() < 0
	})).Return(&types.Header{Number: big.NewInt(100)}, nil).Maybe()
	service.client, service.adapterFactory = defaultClient, factory
	service.txOptsFactory = ethutil.NewStaticTransactOptsFactory(opts)
	policy, err := deliveryPolicy(config.PrtConfig{BlockchainTxMaxFeePerGas: "100"})
	require.NoError(t, err)
	d := &delivery{Service: service, deliveryBackend: backend, deliveryPolicy: policy,
		deliverySweeps:    map[actionKind]*applicationSweep{},
		candidateCooldown: map[actionID]time.Time{}, rootBondRecoveryScans: map[int64]*rootBondRecoveryScan{}}
	d.sender, err = txsender.New[actionID](backend, service.txOptsFactory, 31337, policy, d.guardPublication, service.Logger)
	require.NoError(t, err)
	return &deliveryFixture{d, app, epoch, repo, backend, block, consensus, factory}
}

func (f *deliveryFixture) snapshot(view daveConsensusSnapshot) {
	matcher := mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber == nil && opts.BlockHash == f.block.Hash && !opts.Pending
	})
	f.consensus.On("GetCurrentSealedEpoch", matcher).Return(view.sealed, nil).Maybe()
	f.consensus.On("CanStageTournamentResult", matcher).Return(view.stage, nil).Maybe()
	f.consensus.On("CanAcceptStagedTournamentResult", matcher).Return(view.accept, nil).Maybe()
}

func (b *deliveryBackendFake) advance(number uint64) {
	b.latest = number
	b.headers[number] = &types.Header{Number: new(big.Int).SetUint64(number), GasLimit: 30_000_000, BaseFee: big.NewInt(1)}
}
func (b *deliveryBackendFake) Header(ctx context.Context, tag string) (*types.Header, error) {
	b.headerReads = append(b.headerReads, tag)
	if b.onHeader != nil {
		if err := b.onHeader(ctx); err != nil {
			return nil, err
		}
	}
	if b.headerError != nil {
		return nil, b.headerError
	}
	number := b.latest
	if tag == "safe" || tag == "finalized" {
		number = 100
	}
	return types.CopyHeader(b.headers[number]), nil
}
func (b *deliveryBackendFake) Canonical(_ context.Context, block txsender.BlockRef) (bool, error) {
	header := b.headers[block.Number]
	return header != nil && header.Hash() == block.Hash, nil
}
func (b *deliveryBackendFake) Nonce(context.Context, common.Address, txsender.BlockRef) (uint64, error) {
	return b.nonce, nil
}
func (b *deliveryBackendFake) Balance(context.Context, common.Address, txsender.BlockRef) (*big.Int, error) {
	if b.balance != nil {
		return new(big.Int).Set(b.balance), nil
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(22), nil), nil
}
func (b *deliveryBackendFake) Tip(context.Context) (*big.Int, error)   { return big.NewInt(1), nil }
func (b *deliveryBackendFake) Price(context.Context) (*big.Int, error) { return big.NewInt(3), nil }
func (b *deliveryBackendFake) Estimate(context.Context, ethereum.CallMsg, txsender.BlockRef) (uint64, error) {
	return intrinsicGasFloor, nil
}
func (b *deliveryBackendFake) Publish(_ context.Context, raw []byte) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	b.published = append(b.published, tx)
	b.raw = append(b.raw, append([]byte(nil), raw...))
	if b.onPublish != nil {
		b.onPublish(tx)
	}
	return tx.Hash(), b.publishError
}

// singleAction drives real preparation/signing/POST for a domain plan. It does
// not emulate nonce retention or transaction publication.
type singleAction struct {
	planner *delivery
	action  plannedAction
}

// Direct sender tests create one fresh pass, like delivery.tick does in production.
func (d *delivery) Next(ctx context.Context, block txsender.BlockRef,
	current *txsender.Current[actionID]) (txsender.Decision[actionID], error) {
	return (&deliveryPass{delivery: d}).Next(ctx, block, current)
}

func (p singleAction) Next(context.Context, txsender.BlockRef, *txsender.Current[actionID]) (txsender.Decision[actionID], error) {
	return txsender.Publish(p.action.Key, p.action.Request), nil
}
func (p singleAction) Recheck(ctx context.Context, block txsender.BlockRef, action actionID) (txsender.Eligibility, error) {
	return p.planner.Recheck(ctx, block, action)
}
