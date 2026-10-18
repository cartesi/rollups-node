// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/cartesi/rollups-node/internal/httpclient"

	"github.com/ethereum/go-ethereum/rpc"
)

type ReplyClass string

const (
	Accepted               ReplyClass = "accepted"
	Known                  ReplyClass = "already-known"
	NonceLow               ReplyClass = "nonce-too-low"
	ReplacementUnderpriced ReplyClass = "replacement-underpriced"
	Underpriced            ReplyClass = "transaction-underpriced"
	BaseFeeLow             ReplyClass = "base-fee-too-low"
	Unfunded               ReplyClass = "insufficient-funds"
	GasRejected            ReplyClass = "gas-policy-refusal"
	FeeRejected            ReplyClass = "total-fee-refusal"
	ConstructionRejected   ReplyClass = "construction-refusal"
	PoolFull               ReplyClass = "pool-capacity"
	Uncertain              ReplyClass = "uncertain"
)

// PublicationError retains the classification and original cause of a failed
// publication. A refusal does not establish nonce consumption or domain success.
type PublicationError struct {
	Class ReplyClass
	Err   error
}

func (e *PublicationError) Error() string { return e.Err.Error() }
func (e *PublicationError) Unwrap() error { return e.Err }

// NonceLowOnly permits replanning after a nonce-too-low publication reply.
// Joined transport or application failures must still be reported as errors.
func (e *PublicationError) NonceLowOnly() bool {
	return e != nil && e.Class == NonceLow && nonceLowCausesOnly(e.Err)
}

func nonceLowCausesOnly(err error) bool { return definiteRefusal(err, NonceLow) }

// ClassifyPublication is valid only for eth_sendRawTransaction replies. It
// never searches transport errors, body data or execution reverts for phrases.
func ClassifyPublication(err error) ReplyClass {
	if err == nil {
		return Accepted
	}
	var message string
	var code int
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) {
		code = rpcErr.ErrorCode()
		message = rpcErr.Error()
	} else {
		var httpErr rpc.HTTPError
		if !errors.As(err, &httpErr) {
			return Uncertain
		}
		var e struct {
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(httpErr.Body, &e) != nil || e.Error == nil || e.Error.Code == 3 {
			return Uncertain
		}
		message, code = e.Error.Message, e.Error.Code
	}
	switch httpclient.PublicationMessage(code, message) {
	case "already known":
		return Known
	case "nonce too low":
		return NonceLow
	case "replacement transaction underpriced":
		return ReplacementUnderpriced
	case "transaction underpriced":
		return Underpriced
	case "max fee per gas less than block base fee":
		return BaseFeeLow
	case "insufficient funds":
		return Unfunded
	case "intrinsic gas too low":
		return GasRejected
	case "tx fee (redacted) exceeds the configured cap":
		return FeeRejected
	case "invalid sender":
		return ConstructionRejected
	case "txpool is full":
		return PoolFull
	default:
		return Uncertain
	}
}

// A joined transport failure can hide an earlier accepted POST. A definitive
// refusal requires every original cause to be the same RPC refusal.
func definiteRefusal(err error, class ReplyClass) bool {
	switch cause := err.(type) {
	case net.Error:
		return false
	case interface{ Unwrap() []error }:
		children := cause.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !definiteRefusal(child, class) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		if child := cause.Unwrap(); child != nil {
			return definiteRefusal(child, class)
		}
	}
	return ClassifyPublication(err) == class
}
