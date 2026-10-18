// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package txsender publishes one current transaction for an exclusively owned signer.
package txsender

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type BlockRef struct {
	Number uint64
	Hash   common.Hash
}

// Current describes the preferred transaction, not every version still in a pool.
type Current[A comparable] struct {
	Action  A
	Neutral bool
}

// Request is immutable transaction content supplied by a service. CheckReturn
// must be a pure ABI predicate; it cannot read or write service state.
type Request struct {
	To          common.Address
	Data        []byte
	Value       *big.Int
	GasLimit    uint64 // minimum allowance; zero uses only the padded estimate
	Logger      *slog.Logger
	CheckReturn func([]byte) bool
}

type Eligibility uint8

const (
	Useful Eligibility = iota
	ProtocolWait
	Obsolete
)

type Planner[A comparable] interface {
	Next(context.Context, BlockRef, *Current[A]) (Decision[A], error)
	Recheck(context.Context, BlockRef, A) (Eligibility, error)
}

type decisionKind uint8

const (
	waitDecision decisionKind = iota
	publishDecision
	cancelDecision
)

// Decision values are made with Publish, Wait or Cancel.
type Decision[A comparable] struct {
	kind    decisionKind
	action  A
	request Request
	reason  string
}

func Publish[A comparable](action A, request Request) Decision[A] {
	return Decision[A]{kind: publishDecision, action: action, request: request}
}
func Wait[A comparable](reason string) Decision[A] { return Decision[A]{reason: reason} }
func Cancel[A comparable](reason string) Decision[A] {
	return Decision[A]{kind: cancelDecision, reason: reason}
}

type Report[A comparable] struct {
	Block     BlockRef
	Nonce     uint64
	Current   *Current[A]
	Attempted bool
	Canceling bool
	Hash      common.Hash
	Class     ReplyClass
}

type Policy struct {
	MaxFee         *big.Int // nil means no operator price ceiling
	Legacy         bool
	SigningTimeout time.Duration // zero leaves signing bounded by the caller
}

var (
	ErrInconclusive = errors.New("protected chain view inconclusive")
	ErrFinality     = errors.New("protected view contradicts saved finality")
	ErrNonCanonical = errors.New("protected block is not canonical on this backend")
	ErrStale        = errors.New("transaction preparation is stale")
	ErrHeld         = errors.New("signer nonce is held by another action")
	ErrLimit        = errors.New("transaction policy limit reached")
	ErrContent      = errors.New("action reused with different transaction content")
	ErrIneligible   = errors.New("action is no longer eligible")
	ErrUnsupported  = errors.New("unsupported transaction signer or protected RPC capability")
)

// Backend never substitutes pending or unpinned account/execution reads.
// Estimate uses the prepared block number for sizing; Call authorizes execution
// at its canonical block hash.
type Backend interface {
	Header(context.Context, string) (*types.Header, error)
	Canonical(context.Context, BlockRef) (bool, error)
	Nonce(context.Context, common.Address, BlockRef) (uint64, error)
	Balance(context.Context, common.Address, BlockRef) (*big.Int, error)
	Code(context.Context, common.Address, BlockRef) ([]byte, error)
	Call(context.Context, ethereum.CallMsg, BlockRef) ([]byte, error)
	Estimate(context.Context, ethereum.CallMsg, BlockRef) (uint64, error)
	Tip(context.Context) (*big.Int, error)
	Price(context.Context) (*big.Int, error)
	Publish(context.Context, []byte) (common.Hash, error)
}
