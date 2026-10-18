// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthority"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"math/big"
	"slices"
	"strconv"
	"testing"
	"time"
)

const (
	claimOwnerMethod  = "owner"
	claimSubmitMethod = "submitClaim"
)

type deliveryLookup struct {
	apps        map[string]*model.Application
	epochs      map[string]map[uint64]*model.Epoch
	err         error
	epochErrors map[string]error
	missingNil  bool
	pageErr     error
	hidden      map[int64]bool
}

func (r *deliveryLookup) add(app *model.Application, epoch *model.Epoch) {
	address := app.IApplicationAddress.Hex()
	r.apps[address] = app
	if r.epochs[address] == nil {
		r.epochs[address] = map[uint64]*model.Epoch{}
	}
	r.epochs[address][epoch.Index] = epoch
}
func (r *deliveryLookup) GetApplication(_ context.Context, address string) (*model.Application, error) {
	if r.err != nil {
		return nil, r.err
	}
	app := r.apps[address]
	if app == nil {
		if r.missingNil {
			return nil, nil
		}
		return nil, repository.ErrNotFound
	}
	return app, nil
}
func (r *deliveryLookup) GetEpoch(_ context.Context, address string, index uint64) (*model.Epoch, error) {
	if err := r.epochErrors[address]; err != nil {
		return nil, err
	}
	if r.err != nil {
		return nil, r.err
	}
	epoch := r.epochs[address][index]
	if epoch == nil {
		if r.missingNil {
			return nil, nil
		}
		return nil, repository.ErrNotFound
	}
	return epoch, nil
}

type deliveryBackend struct {
	txsender.Backend
	apps             map[common.Address]*model.Application
	claims           map[int64]iconsensus.IConsensusClaim
	claimsAt         map[uint64]iconsensus.IConsensusClaim
	claimReads       []uint64
	owner            common.Address
	voted            bool
	matchingVote     bool
	votesAt          map[uint64]bool
	matchingVotesAt  map[uint64]bool
	claimReadErr     error
	orphaned         map[common.Hash]bool
	onHeader         func(string)
	headerErr        error
	fail             map[common.Address]error
	failEpochs       map[uint64]error
	last, final      uint64
	safe             uint64
	nonce            uint64
	consumedAt       uint64
	published        []*types.Transaction
	raw              [][]byte
	publishErr       error
	callRefs         []txsender.BlockRef
	canonicalRefs    []txsender.BlockRef
	ownerErr         error
	balance          *big.Int
	validatorID      *big.Int
	blockExecution   map[common.Address]bool
	executionStarted chan struct{}
	onCall           func(string)
}

func deliveryHeader(n uint64) *types.Header {
	return &types.Header{Number: new(big.Int).SetUint64(n), BaseFee: big.NewInt(1), GasLimit: 3000000, Difficulty: big.NewInt(0)}
}
func (b *deliveryBackend) Header(_ context.Context, tag string) (*types.Header, error) {
	if b.onHeader != nil {
		b.onHeader(tag)
	}
	if b.headerErr != nil {
		return nil, b.headerErr
	}
	n := b.last
	switch tag {
	case "finalized":
		n = b.final
	case "safe":
		n = b.safe
		if n == 0 {
			n = b.final
		}
	case "latest":
	default:
		var err error
		n, err = strconv.ParseUint(tag, 0, 64)
		if err != nil {
			return nil, err
		}
	}
	return deliveryHeader(n), nil
}
func (b *deliveryBackend) Canonical(_ context.Context, ref txsender.BlockRef) (bool, error) {
	b.canonicalRefs = append(b.canonicalRefs, ref)
	return !b.orphaned[ref.Hash] && ref.Hash == deliveryHeader(ref.Number).Hash(), nil
}
func (b *deliveryBackend) Nonce(_ context.Context, _ common.Address, ref txsender.BlockRef) (uint64, error) {
	if b.consumedAt != 0 && ref.Number >= b.consumedAt {
		return b.nonce, nil
	}
	return 0, nil
}
func (b *deliveryBackend) Balance(context.Context, common.Address, txsender.BlockRef) (*big.Int, error) {
	if b.balance != nil {
		return new(big.Int).Set(b.balance), nil
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil), nil
}
func (b *deliveryBackend) Code(_ context.Context, address common.Address, _ txsender.BlockRef) ([]byte, error) {
	for _, app := range b.apps {
		if address == app.IApplicationAddress || address == app.IConsensusAddress {
			return []byte{1}, nil
		}
	}
	return nil, nil
}
func (*deliveryBackend) Estimate(context.Context, ethereum.CallMsg, txsender.BlockRef) (uint64, error) {
	return 100000, nil
}
func (*deliveryBackend) Tip(context.Context) (*big.Int, error)   { return big.NewInt(2), nil }
func (*deliveryBackend) Price(context.Context) (*big.Int, error) { return big.NewInt(3), nil }
func (b *deliveryBackend) Call(ctx context.Context, msg ethereum.CallMsg, ref txsender.BlockRef) ([]byte, error) {
	b.callRefs = append(b.callRefs, ref)
	if len(msg.Data) == 0 {
		return nil, nil
	}
	metadata := []*bind.MetaData{
		iapplication.IApplicationMetaData, iauthority.IAuthorityMetaData, iconsensus.IConsensusMetaData, iquorum.IQuorumMetaData,
	}
	for _, meta := range metadata {
		contractABI, err := meta.GetAbi()
		if err != nil {
			return nil, err
		}
		method, err := contractABI.MethodById(msg.Data[:4])
		if err != nil {
			continue
		}
		args, err := method.Inputs.Unpack(msg.Data[4:])
		if err != nil {
			return nil, err
		}
		if b.onCall != nil {
			b.onCall(method.Name)
		}
		switch method.Name {
		case "getOutputsMerkleRootValidator":
			return method.Outputs.Pack(b.apps[*msg.To].IConsensusAddress)
		case "isForeclosed":
			return method.Outputs.Pack(b.apps[*msg.To].ForecloseBlock != 0)
		case claimOwnerMethod:
			if b.ownerErr != nil {
				return nil, b.ownerErr
			}
			return method.Outputs.Pack(b.owner)
		case "validatorId":
			if b.validatorID != nil {
				return method.Outputs.Pack(b.validatorID)
			}
			return method.Outputs.Pack(big.NewInt(1))
		case "isValidatorInFavorOfAnyClaimInEpoch":
			if voted, ok := b.votesAt[args[1].(*big.Int).Uint64()]; ok {
				return method.Outputs.Pack(voted)
			}
			return method.Outputs.Pack(b.voted)
		case "isValidatorInFavorOf":
			if voted, ok := b.matchingVotesAt[args[1].(*big.Int).Uint64()]; ok {
				return method.Outputs.Pack(voted)
			}
			return method.Outputs.Pack(b.matchingVote)
		case "getClaim":
			b.claimReads = append(b.claimReads, args[1].(*big.Int).Uint64())
			if b.claimReadErr != nil {
				return nil, b.claimReadErr
			}
			app := b.apps[args[0].(common.Address)]
			c, ok := b.claims[app.ID]
			if current, exists := b.claimsAt[args[1].(*big.Int).Uint64()]; exists {
				c, ok = current, true
			}
			if !ok {
				c = iconsensus.IConsensusClaim{Status: claimStatusUnstaged, StagingBlockNumber: big.NewInt(0)}
			}
			if c.StagingBlockNumber == nil {
				c.StagingBlockNumber = new(big.Int)
			}
			return method.Outputs.Pack(c)
		case claimSubmitMethod, "acceptClaim":
			if b.blockExecution[args[0].(common.Address)] {
				select {
				case b.executionStarted <- struct{}{}:
				default:
				}
				<-ctx.Done()
				return nil, ctx.Err()
			}
			if err := b.failEpochs[args[1].(*big.Int).Uint64()]; err != nil {
				return nil, err
			}
			if err := b.fail[args[0].(common.Address)]; err != nil {
				return nil, err
			}
			return nil, nil
		}
	}
	return nil, errors.New("unsupported call")
}
func (b *deliveryBackend) Publish(_ context.Context, raw []byte) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	b.published = append(b.published, tx)
	b.raw = append(b.raw, append([]byte(nil), raw...))
	return tx.Hash(), b.publishErr
}
func (b *deliveryBackend) finalize(t *testing.T) {
	t.Helper()
	require.NotEmpty(t, b.published)
	tx := b.published[len(b.published)-1]
	b.consumedAt = b.last + 1
	b.last = b.consumedAt
	b.final = b.consumedAt
	b.nonce = tx.Nonce() + 1

}
func newDeliveryFixture(t *testing.T, policies ...model.DefaultBlock) (*Service, *deliveryBackend, *deliveryLookup) {
	t.Helper()
	s, _, chain := newServiceMock(t)
	key, err := crypto.HexToECDSA("59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
	require.NoError(t, err)
	opts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(31337))
	require.NoError(t, err)
	chain.submitterAddress = opts.From
	backend := &deliveryBackend{
		apps: map[common.Address]*model.Application{}, claims: map[int64]iconsensus.IConsensusClaim{},
		owner: opts.From, fail: map[common.Address]error{}, last: 50, final: 45,
	}
	lookup := &deliveryLookup{apps: map[string]*model.Application{}, epochs: map[string]map[uint64]*model.Epoch{}}
	selected := model.DefaultBlock_Finalized
	if len(policies) > 0 {
		selected = policies[0]
	}
	policy := txsender.Policy{MaxFee: big.NewInt(1000)}
	s.signer = opts.From
	s.deliveryRepository = &claimProvenanceRepository{}
	s.defaultBlock = selected
	s.publisher = &claimPublisher{
		repository: &deliveryCandidateRepository{lookup: lookup}, lookup: lookup, backend: backend,
		signer: opts.From, Logger: s.Logger,
		cooldowns: map[claimAction]time.Time{}, failure: &s.failure,
	}
	s.publisher.sender, err = txsender.New[claimAction](backend, ethutil.NewStaticTransactOptsFactory(opts), 31337, policy,
		s.guardClaimPublication, s.Logger)
	require.NoError(t, err)
	return s, backend, lookup
}

// Only publication pages use this committed-state fixture. The observer keeps
// its own repository mock, so publication cannot see observer-owned traversal.
type deliveryCandidateRepository struct {
	iclaimerRepository
	lookup *deliveryLookup
}

func (r *deliveryCandidateRepository) page(after repository.ClaimCursor, limit uint64, status model.EpochStatus) (
	repository.ClaimPage, error,
) {
	if r.lookup.pageErr != nil {
		return repository.ClaimPage{}, r.lookup.pageErr
	}
	ids := make([]int64, 0, len(r.lookup.apps))
	for address, app := range r.lookup.apps {
		pending := false
		for _, epoch := range r.lookup.epochs[address] {
			pending = pending || epoch.Status == status
		}
		healthy := app.Status == model.ApplicationStatus_OK && app.ForecloseBlock == 0
		if app.Enabled && !r.lookup.hidden[app.ID] && pending && (!after.HealthyOnly || healthy) {
			ids = append(ids, app.ID)
		}
	}
	slices.Sort(ids)
	if len(ids) == 0 {
		return repository.ClaimPage{}, nil
	}
	id := ids[0]
	for _, candidate := range ids {
		if candidate > after.AfterApplicationID {
			id = candidate
			break
		}
	}
	page := repository.ClaimPage{ApplicationID: id}
	for address, app := range r.lookup.apps {
		if app.ID != id {
			continue
		}
		var epochs []*model.Epoch
		for _, epoch := range r.lookup.epochs[address] {
			if epoch.Status == status {
				epochs = append(epochs, epoch)
			}
		}
		slices.SortFunc(epochs, func(a, b *model.Epoch) int {
			if a.Index < b.Index {
				return -1
			}
			if a.Index > b.Index {
				return 1
			}
			return 0
		})
		if len(epochs) == 0 {
			break
		}
		page.Work = append(page.Work, repository.ClaimWork{Application: app, Epoch: epochs[0]})
		last := epochs[0].Index
		if saved, ok := after.Epochs[id]; ok && saved > last {
			last = saved
		}
		var later []*model.Epoch
		for _, epoch := range epochs[1:] {
			if epoch.Index > last {
				later = append(later, epoch)
			}
		}
		if uint64(len(later)) >= limit {
			later = later[:limit-1]
			page.NextEpoch = new(later[len(later)-1].Index)
		}
		for _, epoch := range later {
			page.Work = append(page.Work, repository.ClaimWork{Application: app, Epoch: epoch})
		}
	}

	return page, nil
}
func (r *deliveryCandidateRepository) SelectClaimsToSubmitPage(_ context.Context, after repository.ClaimCursor, limit uint64) (
	repository.ClaimPage, error,
) {
	return r.page(after, limit, model.EpochStatus_ClaimComputed)
}
func (r *deliveryCandidateRepository) SelectClaimsToAcceptPage(_ context.Context, after repository.ClaimCursor, limit uint64) (
	repository.ClaimPage, error,
) {
	return r.page(after, limit, model.EpochStatus_ClaimStaged)
}

func (r *deliveryCandidateRepository) SelectClaimsToStagePage(_ context.Context, after repository.ClaimCursor, limit uint64) (
	repository.ClaimPage, error,
) {
	return r.page(after, limit, model.EpochStatus_ClaimSubmitted)
}

func deliveryApplication(id int64, consensus model.Consensus) (*model.Application, *model.Epoch) {
	app := makeApplication()
	app.ID = id
	app.IApplicationAddress = common.BigToAddress(big.NewInt(id))
	app.IConsensusAddress = common.BigToAddress(big.NewInt(id + 100))
	app.ConsensusType = consensus
	app.Enabled = true
	return app, makeComputedEpoch(app, 3)
}
func mustEpochProof(t *testing.T, epoch *model.Epoch) model.StateProof {
	t.Helper()
	proof, err := epoch.StateProof()
	require.NoError(t, err)
	return proof
}
func queueDelivery(t *testing.T, b *deliveryBackend, r *deliveryLookup, id int64) (*model.Application, *model.Epoch) {
	t.Helper()
	app, epoch := deliveryApplication(id, model.Consensus_Authority)
	r.add(app, epoch)
	b.apps[app.IApplicationAddress] = app
	return app, epoch
}
