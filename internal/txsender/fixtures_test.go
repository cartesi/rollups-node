// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

const knownPublicationMessage = "already known"
const replacementUnderpricedMessage = "replacement transaction underpriced"
const admissionUnderpricedMessage = "transaction underpriced"
const nonceLowMessage = "nonce too low"

type fakeBackend struct {
	head           *types.Header
	headers        map[uint64]*types.Header
	nonces         map[common.Hash]uint64
	balance        *big.Int
	tip, price     *big.Int
	code           []byte
	estimate       uint64
	estimateErr    error
	estimateFn     func(ethereum.CallMsg) (uint64, error)
	estimateBlocks []BlockRef
	calls          []ethereum.CallMsg
	callBlocks     []BlockRef
	published      [][]byte
	tags           []string
	callFn         func(ethereum.CallMsg) ([]byte, error)
	publishFn      func([]byte) error
	publishErr     error
	headerErr      error
	nonceErr       error
}

func newFakeBackend() *fakeBackend {
	b := &fakeBackend{headers: make(map[uint64]*types.Header), nonces: make(map[common.Hash]uint64),
		balance: big.NewInt(1_000_000_000_000_000_000), tip: big.NewInt(2), price: big.NewInt(12), estimate: 21_000}
	b.advance(1, 0)
	return b
}

func (b *fakeBackend) advance(number, nonce uint64) {
	h := &types.Header{Number: new(big.Int).SetUint64(number), BaseFee: big.NewInt(10),
		GasLimit: 30_000_000, GasUsed: 15_000_000, Time: number, Extra: []byte{1}}
	b.headers[number], b.nonces[h.Hash()], b.head = h, nonce, h
}

func (b *fakeBackend) Header(_ context.Context, tag string) (*types.Header, error) {
	b.tags = append(b.tags, tag)
	if b.headerErr != nil {
		return nil, b.headerErr
	}
	if tag == latestBlockTag {
		return types.CopyHeader(b.head), nil
	}
	n, err := hexutil.DecodeUint64(tag)
	if err != nil {
		return nil, err
	}
	if h := b.headers[n]; h != nil {
		return types.CopyHeader(h), nil
	}
	return nil, ethereum.NotFound
}
func (b *fakeBackend) Canonical(_ context.Context, ref BlockRef) (bool, error) {
	h := b.headers[ref.Number]
	return h != nil && h.Hash() == ref.Hash, nil
}
func (b *fakeBackend) Nonce(_ context.Context, _ common.Address, ref BlockRef) (uint64, error) {
	if b.nonceErr != nil {
		return 0, b.nonceErr
	}
	n, ok := b.nonces[ref.Hash]
	if !ok {
		return 0, ethereum.NotFound
	}
	return n, nil
}
func (b *fakeBackend) Balance(context.Context, common.Address, BlockRef) (*big.Int, error) {
	return copyInt(b.balance), nil
}
func (b *fakeBackend) Code(context.Context, common.Address, BlockRef) ([]byte, error) {
	return b.code, nil
}
func (b *fakeBackend) Call(_ context.Context, msg ethereum.CallMsg, block BlockRef) ([]byte, error) {
	b.calls, b.callBlocks = append(b.calls, msg), append(b.callBlocks, block)
	if b.callFn != nil {
		return b.callFn(msg)
	}
	return []byte{1}, nil
}
func (b *fakeBackend) Estimate(_ context.Context, msg ethereum.CallMsg, block BlockRef) (uint64, error) {
	b.estimateBlocks = append(b.estimateBlocks, block)
	if b.estimateFn != nil {
		return b.estimateFn(msg)
	}
	return b.estimate, b.estimateErr
}
func (b *fakeBackend) Tip(context.Context) (*big.Int, error)   { return copyInt(b.tip), nil }
func (b *fakeBackend) Price(context.Context) (*big.Int, error) { return copyInt(b.price), nil }
func (b *fakeBackend) Publish(_ context.Context, raw []byte) (common.Hash, error) {
	b.published = append(b.published, append([]byte(nil), raw...))
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	if b.publishFn != nil {
		return tx.Hash(), b.publishFn(raw)
	}
	return tx.Hash(), b.publishErr
}

type fakeSigner struct {
	opts      *bind.TransactOpts
	signs     int
	afterSign func()
	mutate    func(*types.Transaction) *types.Transaction
}

func newFakeSigner(t *testing.T) *fakeSigner {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	opts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(31337))
	require.NoError(t, err)
	return &fakeSigner{opts: opts}
}
func (f *fakeSigner) From() common.Address { return f.opts.From }
func (f *fakeSigner) NewTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	opts := *f.opts
	opts.Context = ctx
	opts.Signer = func(address common.Address, tx *types.Transaction) (*types.Transaction, error) {
		f.signs++
		if f.mutate != nil {
			tx = f.mutate(tx)
		}
		signed, err := f.opts.Signer(address, tx)
		if f.afterSign != nil {
			f.afterSign()
		}
		return signed, err
	}
	return &opts, nil
}

type fakePlanner struct {
	action      int
	request     Request
	decision    decisionKind
	nextErr     error
	eligibility Eligibility
	recheckErr  error
	nextFn      func(BlockRef, *Current[int]) (Decision[int], error)
	recheckFn   func(BlockRef, int) (Eligibility, error)
	seen        []*Current[int]
}

func newPlanner() *fakePlanner {
	return &fakePlanner{action: 1, request: testRequest(), decision: publishDecision}
}
func (p *fakePlanner) Next(_ context.Context, block BlockRef, current *Current[int]) (Decision[int], error) {
	p.seen = append(p.seen, current)
	if p.nextFn != nil {
		return p.nextFn(block, current)
	}
	return Decision[int]{kind: p.decision, action: p.action, request: p.request}, p.nextErr
}
func (p *fakePlanner) Recheck(_ context.Context, block BlockRef, action int) (Eligibility, error) {
	if p.recheckFn != nil {
		return p.recheckFn(block, action)
	}
	return p.eligibility, p.recheckErr
}
func testRequest() Request {
	return Request{To: common.HexToAddress("0x100"), Data: []byte{1, 2, 3}, Value: new(big.Int)}
}
func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
func testSender(t *testing.T) (*Sender[int], *fakeBackend, *fakeSigner) {
	t.Helper()
	b, f := newFakeBackend(), newFakeSigner(t)
	s, err := New[int](b, f, 31337, Policy{}, func(ctx context.Context) error { return ctx.Err() }, testLogger())
	require.NoError(t, err)
	return s, b, f
}
func publishedTx(t *testing.T, b *fakeBackend, index int) *types.Transaction {
	t.Helper()
	tx := new(types.Transaction)
	require.NoError(t, tx.UnmarshalBinary(b.published[index]))
	return tx
}

type testRPCError struct {
	message string
	code    int
}

func (e testRPCError) Error() string  { return e.message }
func (e testRPCError) ErrorCode() int { return e.code }

var errLostReply = errors.New("connection closed after publication")
