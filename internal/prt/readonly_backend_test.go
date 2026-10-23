// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type hashBindingRPC struct{ blocks []rpc.BlockNumberOrHash }

func (s *hashBindingRPC) GetCode(_ context.Context, _ common.Address, block rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	s.blocks = append(s.blocks, block)
	return []byte{1}, nil
}
func (s *hashBindingRPC) Call(_ context.Context, _ map[string]any, block rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	s.blocks = append(s.blocks, block)
	return []byte{2}, nil
}

func TestReadOnlyBindingBackendRequiresCanonicalHashAndCannotPublish(t *testing.T) {
	server := rpc.NewServer()
	t.Cleanup(server.Stop)
	api := &hashBindingRPC{}
	require.NoError(t, server.RegisterName("eth", api))
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	backend := readOnlyBackend{Client: client}
	hash := common.HexToHash("0x123")
	address := common.HexToAddress("0xabc")
	code, err := backend.CodeAtHash(t.Context(), address, hash)
	require.NoError(t, err)
	require.Equal(t, []byte{1}, code)
	result, err := backend.CallContractAtHash(t.Context(), ethereum.CallMsg{To: &address}, hash)
	require.NoError(t, err)
	require.Equal(t, []byte{2}, result)
	require.Len(t, api.blocks, 2)
	for _, block := range api.blocks {
		require.True(t, block.RequireCanonical)
		require.Equal(t, &hash, block.BlockHash)
	}
	require.ErrorContains(t, backend.SendTransaction(t.Context(), types.NewTx(&types.LegacyTx{})), "cannot publish")
}

func TestFinalizedDeliveryReferenceNeverSubstitutesLatest(t *testing.T) {
	f := newDeliveryFixture(t)
	f.d.defaultBlock = "FINALIZED"
	client := &ethClientMock{}
	f.d.client = client
	header := &types.Header{Number: big.NewInt(100)}
	client.On("HeaderByNumber", t.Context(), big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(header, nil).Once()
	client.On("HeaderByNumber", t.Context(), big.NewInt(100)).Return(header, nil).Once()
	reference, err := f.d.observationReference(t.Context())
	require.NoError(t, err)
	require.Equal(t, txsender.BlockRef{Number: 100, Hash: header.Hash()}, reference)
	client.AssertExpectations(t)
}
