// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

const unfundedEstimateMessage = "insufficient funds for gas * price + value"

// Z18: publication classification uses error.message only. Incorrect or
// ambiguous replies may delay progress; they never establish chain progress.
func TestClassifyPublicationZ18(t *testing.T) {
	for _, test := range []struct {
		message string
		want    ReplyClass
	}{
		{knownPublicationMessage, Known}, {nonceLowMessage, NonceLow},
		{"Known transaction", Known},
		{"Unknown transaction", Uncertain},
		{"Gas price below current base fee", BaseFeeLow},
		{"Gas price below configured minimum gas price", Underpriced},
		{replacementUnderpricedMessage, ReplacementUnderpriced},
		{admissionUnderpricedMessage, Underpriced},
		{"max fee per gas less than block base fee", BaseFeeLow},
		{unfundedEstimateMessage, Unfunded},
		{"intrinsic gas too low", GasRejected},
		{"tx fee (1.0 ether) exceeds the configured cap", FeeRejected},
		{"invalid sender", ConstructionRejected}, {"txpool is full", PoolFull},
		{"execution reverted: nonce too low", Uncertain}, {"something unknown", Uncertain},
	} {
		t.Run(test.message, func(t *testing.T) {
			err := testRPCError{message: test.message, code: -32000}
			require.Equal(t, test.want, ClassifyPublication(fmt.Errorf("wrapped: %w", err)))
		})
	}
	require.Equal(t, Accepted, ClassifyPublication(nil))
	require.Equal(t, Uncertain, ClassifyPublication(errors.New(nonceLowMessage)))
	require.Equal(t, Uncertain, ClassifyPublication(testRPCError{message: knownPublicationMessage, code: 3}))
	for _, body := range []string{
		`<html>nonce too low</html>`,
		`{"error":{"code":-32000,"message":"unknown","data":"nonce too low"}}`,
		`{"error":{"code":3,"message":"nonce too low"}}`,
		`{"result":"nonce too low"}`,
		`{"error":{"code":-32000,"message":"execution reverted: already known"}}`,
	} {
		require.Equal(t, Uncertain, ClassifyPublication(rpc.HTTPError{StatusCode: 503, Body: []byte(body)}))
	}
	err := rpc.HTTPError{StatusCode: 429, Body: []byte(`{"error":{"code":-32000,"message":"already known"}}`)}
	require.Equal(t, Known, ClassifyPublication(err))
}

func TestPublicationNonceWaitRequiresEveryOriginalCauseToBeANonceReply(t *testing.T) {
	nonceReply := testRPCError{message: nonceLowMessage, code: -32000}
	transport := errors.New("connection closed during publication")
	for _, test := range []struct {
		name  string
		class ReplyClass
		cause error
		wait  bool
	}{
		{"RPC reply", NonceLow, nonceReply, true},
		{"wrapped reply", NonceLow, fmt.Errorf("RPC: %w", nonceReply), true},
		{"joined nonce replies", NonceLow, errors.Join(nonceReply, nonceReply), true},
		{"unknown sibling", NonceLow, errors.Join(nonceReply, transport), false},
		{"nested unknown sibling", NonceLow, fmt.Errorf("RPC: %w", errors.Join(nonceReply, transport)), false},
		{"transport", Uncertain, transport, false},
		{"transport wraps RPC reply", NonceLow, &url.Error{Op: "Post", URL: "http://provider", Err: nonceReply}, false},
		{"plain text lookalike", NonceLow, errors.New(nonceReply.message), false},
		{"execution revert", NonceLow, testRPCError{message: nonceReply.message, code: 3}, false},
		{"other reply class", Uncertain, nonceReply, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			publication := &PublicationError{Class: test.class, Err: test.cause}
			require.Equal(t, test.wait, publication.NonceLowOnly())
			require.ErrorIs(t, publication, test.cause)
			require.Equal(t, test.cause.Error(), publication.Error())
		})
	}
	require.False(t, (&PublicationError{Class: NonceLow}).NonceLowOnly())
	var absent *PublicationError
	require.False(t, absent.NonceLowOnly())
}
