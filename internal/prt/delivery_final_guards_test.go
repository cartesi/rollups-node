// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestPRTObserverFinalityContradictionStopsTheRealPublisherAfterALaterGoodHead(t *testing.T) {
	f := newDeliveryFixture(t)
	backend := f.backend
	f.d.delivery, f.d.submissionEnabled = f.d, true
	f.d.observerFacts = txsender.BlockRef{Number: 100, Hash: backend.headers[100].Hash()}
	backend.advance(111)
	client := &ethClientMock{}
	f.d.client = client
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(backend.headers[111], nil).Once()
	client.On("HeaderByNumber", mock.Anything, big.NewInt(111)).Return(backend.headers[111], nil).Once()
	conflict := types.CopyHeader(backend.headers[100])
	conflict.Extra = []byte("contradictory finalized history")
	client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(conflict, nil).Once()
	require.ErrorIs(t, f.d.observe(t.Context()), txsender.ErrFinality)
	require.False(t, f.d.Ready())
	backend.advance(112)
	call := f.d.call(f.app, f.epoch.Index, stageTournament, f.app.IConsensusAddress, []byte{1}, new(big.Int), 0)
	report, err := f.d.sender.PublishNext(t.Context(), singleAction{f.d, call})
	require.ErrorIs(t, err, txsender.ErrFinality)
	require.False(t, report.Attempted)
	require.ErrorIs(t, f.d.tick(t.Context()), txsender.ErrFinality)
	require.Empty(t, backend.published)
	require.False(t, f.d.Ready(), "a later valid head cannot clear the observer's hard stop")
	client.AssertExpectations(t)
}
