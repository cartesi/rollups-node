// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
	"testing"
)

// VER2 probe for mutant M4a. A finalized tag served by another replica can name
// an existing height with a different hash. The proposed header must be
// validated before the watermark moves; otherwise the next honest pass latches
// ErrFinality on a mark that was never canonical.
func TestNonCanonicalExistingHeightCannotAdvanceFinalizedWatermark(t *testing.T) {
	backend := &finalizedViewRPC{final: 30}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	chain := &claimerBlockchain{
		client: client, defaultBlock: model.DefaultBlock_Finalized, backend: txsender.ProtectedClient{Client: client},
	}
	backend.final = 10
	_, err := chain.getDefaultBlockNumber(t.Context())
	require.NoError(t, err)
	// The tag returns height 20 from a different branch; by-number reads of 20
	// return the canonical header (conflict applies only to the tag path here).
	tagged := &taggedConflictRPC{finalizedViewRPC: backend, tagNumber: 20}
	server2 := rpc.NewServer()
	require.NoError(t, server2.RegisterName("eth", tagged))
	t.Cleanup(server2.Stop)
	client2 := ethclient.NewClient(rpc.DialInProc(server2))
	t.Cleanup(client2.Close)
	chain.client, chain.backend = client2, txsender.ProtectedClient{Client: client2}
	backend.final = 25
	_, err = chain.getDefaultBlockNumber(t.Context())
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.EqualValues(t, 10, chain.observed.Number, "an unvalidated tag cannot move the watermark")
	tagged.tagNumber = 0
	block, err := chain.getDefaultBlockNumber(t.Context())
	require.NoError(t, err, "the next honest view proceeds; no sticky finality fault")
	require.EqualValues(t, 25, block.Uint64())
}

type taggedConflictRPC struct {
	*finalizedViewRPC
	tagNumber uint64
}

func (v *taggedConflictRPC) GetBlockByNumber(ctx context.Context, n rpc.BlockNumber, full bool) (any, error) {
	if n == rpc.FinalizedBlockNumber && v.tagNumber != 0 {
		header := deliveryHeader(v.tagNumber)
		header.Extra = []byte("other replica branch")
		return header, nil
	}
	return v.finalizedViewRPC.GetBlockByNumber(ctx, n, full)
}

// Quorum permits one vote per validator and epoch, even for a different claim.
func TestQuorumPriorVoteDoesNotPublishAnotherVote(t *testing.T) {
	s, b, r := newDeliveryFixture(t)
	app, epoch := deliveryApplication(1, model.Consensus_Quorum)
	r.add(app, epoch)
	b.apps[app.IApplicationAddress] = app
	b.voted = true
	err := s.publisher.tick(t.Context())
	require.NoError(t, err)
	require.Empty(t, b.published)
}
