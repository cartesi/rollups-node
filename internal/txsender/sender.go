// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

type account struct {
	block   BlockRef
	header  *types.Header
	nonce   uint64
	balance *big.Int
}

type prepared[A comparable] struct {
	action  A
	neutral bool
	request Request
	nonce   uint64
	block   BlockRef
	gas     uint64
	quote
	raw  []byte
	hash common.Hash
}

// Sender belongs to one publisher goroutine. Its only transaction state is one
// preferred current-nonce request. Distributed alternatives can still execute.
type Sender[A comparable] struct {
	backend Backend
	factory ethutil.TransactOptsFactory
	chainID uint64
	policy  Policy
	guard   func(context.Context) error
	logger  *slog.Logger
	current *prepared[A]
}

func New[A comparable](backend Backend, factory ethutil.TransactOptsFactory, chainID uint64, policy Policy,
	guard func(context.Context) error, logger *slog.Logger) (*Sender[A], error) {
	if backend == nil || factory == nil || chainID == 0 || guard == nil {
		return nil, errors.New("sender requires backend, signer, chain identity and publication guard")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if policy.MaxFee != nil {
		policy.MaxFee = copyInt(policy.MaxFee)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Sender[A]{backend: backend, factory: factory, chainID: chainID, policy: policy, guard: guard, logger: logger}, nil
}

func (s *Sender[A]) From() common.Address { return s.factory.From() }

func (s *Sender[A]) checked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.guard(ctx)
}

func (s *Sender[A]) account(ctx context.Context) (account, error) {
	header, err := s.backend.Header(ctx, latestBlockTag)
	if err != nil {
		return account{}, inconclusive(err)
	}
	block, err := headerRef(header)
	if err != nil {
		return account{}, err
	}
	nonce, err := s.backend.Nonce(ctx, s.From(), block)
	if err != nil {
		return account{}, inconclusive(err)
	}
	balance, err := s.backend.Balance(ctx, s.From(), block)
	if err != nil {
		return account{}, inconclusive(err)
	}
	code, err := s.backend.Code(ctx, s.From(), block)
	if err != nil {
		return account{}, inconclusive(err)
	}
	if len(code) != 0 {
		return account{}, fmt.Errorf("%w: signer code or delegation is outside the supported EOA profile", ErrUnsupported)
	}
	if balance == nil || balance.Sign() < 0 || balance.BitLen() > 256 ||
		(header.BaseFee != nil && header.BaseFee.Sign() < 0) {
		return account{}, ErrInconclusive
	}
	if err := s.canonical(ctx, block); err != nil {
		return account{}, err
	}
	return account{block: block, header: types.CopyHeader(header), nonce: nonce, balance: copyInt(balance)}, nil
}

func (s *Sender[A]) canonical(ctx context.Context, block BlockRef) error {
	ok, err := s.backend.Canonical(ctx, block)
	if err != nil {
		return inconclusive(err)
	}
	if !ok {
		return ErrNonCanonical
	}
	return nil
}

func (s *Sender[A]) currentAction() *Current[A] {
	if s.current == nil {
		return nil
	}
	return &Current[A]{Action: s.current.action, Neutral: s.current.neutral}
}

// PublishNext runs one publication decision. A failed read cannot authorize
// cancellation. Latest mined nonce progress retires the cache without receipts.
func (s *Sender[A]) PublishNext(ctx context.Context, planner Planner[A]) (report Report[A], err error) {
	defer func() { report.Current = s.currentAction() }()
	if planner == nil {
		return report, errors.New("publication planner is required")
	}
	if err := s.checked(ctx); err != nil {
		return report, err
	}
	snap, err := s.account(ctx)
	if err != nil {
		return report, err
	}
	report.Block, report.Nonce = snap.block, snap.nonce
	if s.current != nil && s.current.nonce > snap.nonce {
		canonical, readErr := s.backend.Canonical(ctx, s.current.block)
		if readErr != nil {
			return report, inconclusive(readErr)
		}
		if canonical {
			return report, ErrInconclusive // A lagging replica cannot revoke a later canonical account read.
		}
	}
	if s.current != nil && s.current.nonce != snap.nonce {
		s.current = nil
	}
	decision, err := planner.Next(ctx, snap.block, s.currentAction())
	if err != nil {
		return report, err
	}
	if decision.kind == waitDecision || decision.kind == cancelDecision && s.current == nil {
		return report, nil
	}
	neutral := decision.kind == cancelDecision
	report.Canceling = neutral
	if neutral {
		decision.action = s.current.action
		decision.request = Request{To: s.From(), Value: new(big.Int), GasLimit: intrinsicGas,
			Logger: s.current.request.Logger}
	}
	request := cloneRequest(decision.request)
	if err := validRequest(request); err != nil {
		return report, err
	}
	old := s.current
	sameIntent := old != nil && old.action == decision.action && old.neutral == neutral
	if old != nil && !old.neutral && !neutral && old.action == decision.action && !sameContent(old.request, request) {
		return report, ErrContent
	}
	q, err := s.fees(ctx, snap)
	if err != nil {
		return report, err
	}
	fundingCap := q.cap
	if sameIntent && old.legacy == q.legacy {
		fundingCap = minimum(q.cap, old.cap) // Adequate held bytes can still be cheaper than the fresh quote.
	}
	if err := checkFunds(snap.balance, max(uint64(intrinsicGas), request.GasLimit), fundingCap, request.Value); err != nil {
		return report, fmt.Errorf("minimum funding before gas estimation: %w", err)
	}
	sizingQuote := q
	if sameIntent && old.legacy == q.legacy && coversQuotedTip(old.quote, q, snap.header.BaseFee) &&
		!clearsReplacement(q, old.quote) {
		sizingQuote = old.quote // Sizing must not demand funding for a quote whose bytes we intend to reuse.
	}
	candidate, estimate, err := s.prepare(ctx, snap, decision.action, request, neutral, sizingQuote)
	if err != nil {
		return report, err
	}
	candidate.quote = q
	reusable := sameIntent && old.legacy == q.legacy && old.gas >= max(request.GasLimit, estimate) &&
		old.gas <= snap.header.GasLimit && coversQuotedTip(old.quote, q, snap.header.BaseFee)
	if reusable && !clearsReplacement(q, old.quote) {
		retained := *old
		retained.request, retained.block = request, snap.block
		candidate = &retained
	} else if old != nil && !reusable {
		candidate.quote = s.replacementFees(q, old.quote)
	}
	class, err := s.send(ctx, planner, snap, candidate, &report)
	if !priceRefusal(class) {
		return report, err
	}
	retryQuote := candidate.quote
	if sameIntent && class == ReplacementUnderpriced && definiteRefusal(err, class) {
		// A lost reply can leave cached bytes that the pool never admitted.
		retryQuote = q
	}
	retry := s.retryFees(retryQuote)
	if retry.cap.Cmp(candidate.cap) != 0 || retry.tip.Cmp(candidate.tip) != 0 {
		candidate.quote, candidate.raw = retry, nil
		s.requestLogger(request).Debug("Retrying price-refused transaction with the full fresh quote",
			"signer", s.From(), "nonce", snap.nonce, "fee_cap", retry.cap, "tip", retry.tip,
			"reason", decision.reason)
		class, err = s.send(ctx, planner, snap, candidate, &report)
	}
	if sameIntent && class == ReplacementUnderpriced && definiteRefusal(err, class) {
		return report, nil
	}
	return report, err
}

func cloneRequest(request Request) Request {
	request.Data = append([]byte(nil), request.Data...)
	request.Value = copyInt(request.Value)
	return request
}

func sameContent(a, b Request) bool {
	return a.To == b.To && a.Value.Cmp(b.Value) == 0 && bytes.Equal(a.Data, b.Data)
}

func validRequest(request Request) error {
	if request.Value.Sign() < 0 || request.Value.BitLen() > 256 {
		maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint256Bits), big.NewInt(1))
		return limitError("transaction value", request.Value, maximum)
	}
	if len(request.Data) > defaultCallBytes {
		return countLimit("transaction data bytes", len(request.Data), defaultCallBytes)
	}
	return nil
}

func (s *Sender[A]) prepare(ctx context.Context, snap account, action A, request Request,
	neutral bool, q quote) (*prepared[A], uint64, error) {
	ceiling := min(snap.header.GasLimit, uint64(supportedTransactionGasCeiling))
	if request.GasLimit > ceiling {
		return nil, 0, gasLimit("transaction gas", request.GasLimit, ceiling)
	}
	estimate, estimateErr := s.backend.Estimate(ctx, message(s.From(), request, 0, q), snap.block)
	var gas uint64
	if estimateErr != nil {
		var refusal rpc.Error
		if errors.As(estimateErr, &refusal) {
			return nil, 0, fmt.Errorf("gas estimate refused: %w", estimateErr)
		}
		old := s.current
		if ctx.Err() != nil || errors.Is(estimateErr, context.Canceled) ||
			old == nil || old.action != action || old.neutral != neutral || !sameContent(old.request, request) ||
			old.gas < request.GasLimit || old.gas > ceiling {
			return nil, 0, fmt.Errorf("gas estimate unavailable: %w", estimateErr)
		}
		// An unavailable estimate supplies no new gas requirement. Reuse only
		// this request's existing gas; exact simulation still has to succeed.
		gas, estimate = old.gas, old.gas
	} else {
		if estimate > ceiling {
			return nil, 0, gasLimit("estimated transaction gas", estimate, ceiling)
		}
		padding := uint64(0)
		if !neutral {
			padding = max(estimate/gasPaddingDivisor, uint64(minimumGasPadding))
			if request.Value.Sign() > 0 {
				padding = max(estimate, uint64(minimumGasPadding))
			}
		}
		if padding > ceiling-estimate {
			gas = ceiling
		} else {
			gas = max(request.GasLimit, estimate+padding)
		}
	}
	if gas < intrinsicGas || gas > ceiling {
		return nil, 0, gasLimit("transaction gas", gas, ceiling)
	}
	candidate := &prepared[A]{action: action, neutral: neutral, request: request, nonce: snap.nonce,
		block: snap.block, gas: gas, quote: q}
	return candidate, estimate, nil
}

func message(from common.Address, request Request, gas uint64, q quote) ethereum.CallMsg {
	m := ethereum.CallMsg{From: from, To: &request.To, Data: request.Data, Value: copyInt(request.Value), Gas: gas}
	if q.legacy {
		m.GasPrice = copyInt(q.cap)
	} else {
		m.GasFeeCap, m.GasTipCap = copyInt(q.cap), copyInt(q.tip)
	}
	return m
}

func (s *Sender[A]) preflight(ctx context.Context, snap account, candidate *prepared[A]) error {
	if candidate.gas > snap.header.GasLimit {
		return gasLimit("block gas", candidate.gas, snap.header.GasLimit)
	}
	if snap.header.BaseFee != nil && candidate.cap.Cmp(snap.header.BaseFee) < 0 {
		return limitError("base fee", snap.header.BaseFee, candidate.cap)
	}
	if err := checkFunds(snap.balance, candidate.gas, candidate.cap, candidate.request.Value); err != nil {
		return err
	}
	result, err := s.backend.Call(ctx, message(s.From(), candidate.request, candidate.gas, candidate.quote), snap.block)
	if err != nil {
		return err
	}
	if candidate.request.CheckReturn != nil && !candidate.request.CheckReturn(result) {
		return ErrIneligible
	}
	return nil
}

func checkFunds(balance *big.Int, gas uint64, feeCap, value *big.Int) error {
	cost := new(big.Int).Mul(feeCap, new(big.Int).SetUint64(gas))
	cost.Add(cost, value)
	if cost.Cmp(balance) > 0 {
		return limitError("signer balance", cost, balance)
	}
	return nil
}

func (s *Sender[A]) sameEnvelope(candidate *prepared[A]) bool {
	old := s.current
	return old != nil && old.action == candidate.action && old.neutral == candidate.neutral &&
		sameContent(old.request, candidate.request) && old.nonce == candidate.nonce && old.gas == candidate.gas &&
		old.legacy == candidate.legacy && old.cap.Cmp(candidate.cap) == 0 && old.tip.Cmp(candidate.tip) == 0
}

func (s *Sender[A]) sign(ctx context.Context, candidate *prepared[A]) error {
	if s.sameEnvelope(candidate) {
		candidate.raw, candidate.hash = s.current.raw, s.current.hash
		return nil
	}
	if s.policy.SigningTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.policy.SigningTimeout)
		defer cancel()
	}
	opts, err := s.factory.NewTransactOpts(ctx)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts == nil || opts.Signer == nil || opts.From != s.From() {
		return fmt.Errorf("%w: signer factory returned missing options or a different address", ErrUnsupported)
	}
	chainID := new(big.Int).SetUint64(s.chainID)
	var unsigned *types.Transaction
	if candidate.legacy {
		unsigned = types.NewTx(&types.LegacyTx{Nonce: candidate.nonce, To: &candidate.request.To,
			Value: candidate.request.Value, Data: candidate.request.Data, Gas: candidate.gas, GasPrice: candidate.cap})
	} else {
		unsigned = types.NewTx(&types.DynamicFeeTx{ChainID: chainID, Nonce: candidate.nonce, To: &candidate.request.To,
			Value: candidate.request.Value, Data: candidate.request.Data, Gas: candidate.gas,
			GasFeeCap: candidate.cap, GasTipCap: candidate.tip})
	}
	signed, err := opts.Signer(opts.From, unsigned)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if signed == nil {
		return ErrContent
	}
	from, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	if err != nil || from != s.From() || signed.ChainId().Cmp(chainID) != 0 || signed.Type() != unsigned.Type() ||
		signed.Nonce() != candidate.nonce || signed.To() == nil || *signed.To() != candidate.request.To ||
		signed.Value().Cmp(candidate.request.Value) != 0 || signed.Gas() != candidate.gas ||
		!bytes.Equal(signed.Data(), candidate.request.Data) || signed.GasFeeCap().Cmp(candidate.cap) != 0 ||
		signed.GasTipCap().Cmp(candidate.tip) != 0 {
		return ErrContent
	}
	candidate.raw, err = signed.MarshalBinary()
	candidate.hash = signed.Hash()
	return err
}

func (s *Sender[A]) recheck(ctx context.Context, planner Planner[A], original account, candidate *prepared[A]) error {
	if err := s.checked(ctx); err != nil {
		return err
	}
	if err := s.canonical(ctx, original.block); err != nil {
		return err
	}
	fresh, err := s.account(ctx)
	if err != nil {
		return err
	}
	if fresh.nonce != candidate.nonce || fresh.block.Number < original.block.Number {
		return ErrStale
	}
	eligibility, err := planner.Recheck(ctx, fresh.block, candidate.action)
	if err != nil {
		return err
	}
	if eligibility == ProtocolWait {
		return ErrInconclusive
	}
	if candidate.neutral && eligibility != Obsolete || !candidate.neutral && eligibility != Useful {
		return ErrIneligible
	}
	if err := s.preflight(ctx, fresh, candidate); err != nil {
		return err
	}
	return s.checked(ctx)
}

func (s *Sender[A]) send(ctx context.Context, planner Planner[A], snap account, candidate *prepared[A],
	report *Report[A]) (ReplyClass, error) {
	if err := s.preflight(ctx, snap, candidate); err != nil {
		return "", err
	}
	if err := s.sign(ctx, candidate); err != nil {
		return "", err
	}
	if err := s.recheck(ctx, planner, snap, candidate); err != nil {
		return "", err
	}
	old := s.current
	s.current = candidate // Retain bytes before the possibly ambiguous POST.
	report.Attempted, report.Hash = true, candidate.hash
	hash, err := s.backend.Publish(ctx, append([]byte(nil), candidate.raw...))
	class := ClassifyPublication(err)
	if err != nil && class != Known && class != Uncertain && !definiteRefusal(err, class) {
		class = Uncertain
	}
	if err == nil && hash != candidate.hash {
		class, err = Uncertain, errors.New("publication returned a different transaction hash")
	}
	report.Class = class
	if class == Accepted || class == Known {
		err = nil
	} else if class != Uncertain {
		s.current = old
	}
	s.requestLogger(candidate.request).Info("Transaction publication", "signer", s.From(), "nonce", candidate.nonce,
		"tx", candidate.hash, "to", candidate.request.To, "reply", class, "gas", candidate.gas,
		"fee_cap", candidate.cap, "tip", candidate.tip, "neutral", candidate.neutral)
	if err != nil {
		return class, &PublicationError{Class: class, Err: err}
	}
	return class, nil
}

func (s *Sender[A]) requestLogger(request Request) *slog.Logger {
	if request.Logger != nil {
		return request.Logger
	}
	return s.logger
}
