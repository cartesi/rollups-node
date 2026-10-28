// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"math/big"
)

type quote struct {
	cap, tip *big.Int
	legacy   bool
}

func (s *Sender[A]) fees(ctx context.Context, snap account) (quote, error) {
	q := quote{legacy: s.policy.Legacy || snap.header.BaseFee == nil}
	var err error
	if q.legacy {
		q.cap, err = s.backend.Price(ctx)
	} else {
		q.tip, err = s.backend.Tip(ctx)
		if q.tip != nil {
			q.cap = new(big.Int).Mul(snap.header.BaseFee, big.NewInt(baseFeeMultiplier))
			q.cap.Add(q.cap, q.tip)
		}
	}
	if err != nil {
		return quote{}, err
	}
	if q.cap == nil || q.cap.Sign() <= 0 || q.cap.BitLen() > 256 ||
		(!q.legacy && (q.tip == nil || q.tip.Sign() < 0 || q.tip.BitLen() > 256)) {
		return quote{}, ErrInconclusive
	}
	if s.policy.MaxFee != nil {
		q.cap = minimum(q.cap, s.policy.MaxFee)
	}
	if q.legacy {
		q.tip = copyInt(q.cap)
	} else {
		q.tip = minimum(q.tip, q.cap)
	}
	return q, nil
}

func (s *Sender[A]) retryFees(q quote) quote {
	r := quote{legacy: q.legacy, cap: copyInt(q.cap), tip: copyInt(q.cap)}
	if q.legacy {
		r.cap = replacementMinimum(q.cap)
		if s.policy.MaxFee != nil {
			r.cap = minimum(r.cap, s.policy.MaxFee)
		}
		r.tip = copyInt(r.cap)
	}
	return r
}

// A pool can require a larger bump. This matches geth's default without keeping
// a history of bids or promising that a replacement will be accepted.
func clearsReplacement(q, old quote) bool {
	return q.legacy == old.legacy && q.cap.Cmp(replacementMinimum(old.cap)) >= 0 &&
		q.tip.Cmp(replacementMinimum(old.tip)) >= 0
}

func coversQuotedTip(old, fresh quote, baseFee *big.Int) bool {
	if baseFee == nil {
		return true
	}
	required := copyInt(baseFee)
	if !old.legacy {
		required.Add(required, minimum(old.tip, fresh.tip))
	} else {
		return old.cap.Cmp(baseFee) > 0
	}
	return old.cap.Cmp(required) >= 0
}

func (s *Sender[A]) replacementFees(q, old quote) quote {
	feeCap := maximum(q.cap, replacementMinimum(old.cap))
	if s.policy.MaxFee != nil && feeCap.Cmp(s.policy.MaxFee) > 0 {
		// The old copy may have left the pool. Try the fresh capped quote; a
		// definitive refusal preserves the previous current request.
		return q
	}
	q.cap = feeCap
	q.tip = maximum(q.tip, replacementMinimum(old.tip))
	if q.legacy {
		q.tip = copyInt(q.cap)
	}
	return q
}

func maximum(a, b *big.Int) *big.Int {
	if a.Cmp(b) > 0 {
		return copyInt(a)
	}
	return copyInt(b)
}

func priceRefusal(class ReplyClass) bool {
	return class == ReplacementUnderpriced || class == Underpriced || class == BaseFeeLow
}
