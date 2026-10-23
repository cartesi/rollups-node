// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func TestMineNewBlock(t *testing.T) {
	// A devnet can mine between independent head reads. Control the RPC state
	// to verify that this call mines once and reads the resulting head.
	state := &mineBlockTestState{number: 41}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("evm", &mineBlockTestEVM{state: state}))
	require.NoError(t, server.RegisterName("eth", &mineBlockTestEth{state: state}))
	defer server.Stop()
	client := ethclient.NewClient(rpc.DialInProc(server))
	defer client.Close()

	number, err := MineNewBlock(t.Context(), client)

	require.NoError(t, err)
	require.Equal(t, uint64(42), number)
	state.mu.Lock()
	defer state.mu.Unlock()
	require.Equal(t, []string{"evm_mine", "eth_blockNumber"}, state.calls)
}

type mineBlockTestState struct {
	mu     sync.Mutex
	number hexutil.Uint64
	calls  []string
}

type mineBlockTestEVM struct{ state *mineBlockTestState }

func (b *mineBlockTestEVM) Mine() (string, error) {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	b.state.calls = append(b.state.calls, "evm_mine")
	b.state.number++
	return "0x0", nil
}

type mineBlockTestEth struct{ state *mineBlockTestState }

func (b *mineBlockTestEth) BlockNumber() (hexutil.Uint64, error) {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	b.state.calls = append(b.state.calls, "eth_blockNumber")
	return b.state.number, nil
}
