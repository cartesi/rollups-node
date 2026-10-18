// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type finalizedViewRPC struct {
	final    uint64
	conflict bool
	tagOnly  uint64
}

func (v *finalizedViewRPC) GetBlockByNumber(_ context.Context, n rpc.BlockNumber, _ bool) (*types.Header, error) {
	var number uint64
	switch {
	case n == rpc.FinalizedBlockNumber || n == rpc.LatestBlockNumber || n == rpc.SafeBlockNumber:
		number = v.final
		if v.tagOnly != 0 {
			number = v.tagOnly
		}
	case n >= 0:
		number = uint64(n)
		if number > v.final {
			return nil, fmt.Errorf("block %d is beyond this replica's head", number)
		}
	default:
		return nil, fmt.Errorf("unsupported block tag: %d", n)
	}
	header := deliveryHeader(number)
	if v.conflict && number == 20 {
		header.Extra = []byte("different finalized branch")
	}
	return header, nil
}

func TestUnvalidatedFinalizedTagCannotPoisonObservationWatermark(t *testing.T) {
	backend := &finalizedViewRPC{final: 10}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	chain := &claimerBlockchain{
		client: client, defaultBlock: model.DefaultBlock_Finalized, backend: txsender.ProtectedClient{Client: client},
	}
	_, err := chain.getDefaultBlockNumber(t.Context())
	require.NoError(t, err)
	backend.tagOnly = 1_000_000
	_, err = chain.getDefaultBlockNumber(t.Context())
	require.Error(t, err)
	require.EqualValues(t, 10, chain.observed.Number)
	backend.tagOnly = 0
	for head := uint64(11); head < 15; head++ {
		backend.final = head
		block, err := chain.getDefaultBlockNumber(t.Context())
		require.NoError(t, err)
		require.Equal(t, head, block.Uint64())
	}
}

func TestFinalizedObservationSkewDefersAndAdvancedContradictionStaysStopped(t *testing.T) {
	backend := &finalizedViewRPC{final: 20}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	chain := &claimerBlockchain{
		client: client, defaultBlock: model.DefaultBlock_Finalized, backend: txsender.ProtectedClient{Client: client},
	}
	observed, err := chain.getDefaultBlockNumber(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 20, observed.Uint64())
	backend.final = 19
	_, err = chain.getDefaultBlockNumber(t.Context())
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.EqualValues(t, 20, chain.observed.Number)
	backend.final = 21
	backend.conflict = true
	_, err = chain.getDefaultBlockNumber(t.Context())
	require.ErrorIs(t, err, txsender.ErrFinality)
	backend.conflict = false
	_, err = chain.getDefaultBlockNumber(t.Context())
	require.ErrorIs(t, err, txsender.ErrFinality, "a stronger contradictory view requires operator investigation")
}

func TestEqualFinalizedHeightWithDifferentHashStaysStopped(t *testing.T) {
	backend := &finalizedViewRPC{final: 20}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	chain := &claimerBlockchain{
		client: client, defaultBlock: model.DefaultBlock_Finalized, backend: txsender.ProtectedClient{Client: client},
	}
	_, err := chain.getDefaultBlockNumber(t.Context())
	require.NoError(t, err)
	backend.conflict = true
	_, err = chain.getDefaultBlockNumber(t.Context())
	require.ErrorIs(t, err, txsender.ErrFinality)
	backend.conflict = false
	_, err = chain.getDefaultBlockNumber(t.Context())
	require.ErrorIs(t, err, txsender.ErrFinality)
}

func TestProvisionalObservationPolicyAcceptsChangedChainWithoutAFinalityLatch(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe} {
		t.Run(policy.String(), func(t *testing.T) {
			backend := &finalizedViewRPC{final: 20}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", backend))
			t.Cleanup(server.Stop)
			client := ethclient.NewClient(rpc.DialInProc(server))
			t.Cleanup(client.Close)
			chain := &claimerBlockchain{client: client, defaultBlock: policy, backend: txsender.ProtectedClient{Client: client}}
			_, err := chain.getDefaultBlockNumber(t.Context())
			require.NoError(t, err)
			previous := chain.observed
			backend.conflict = true
			_, err = chain.getDefaultBlockNumber(t.Context())
			require.NoError(t, err, "configured provisional reads do not promise rollback or finalized immutability")
			require.NotEqual(t, previous.Hash, chain.observed.Hash)
			require.Nil(t, chain.fault)
		})
	}
}
