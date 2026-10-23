// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type daveConsensusViewRPC struct{ mock.Mock }

type daveEventQuery struct {
	BlockHash common.Hash      `json:"blockHash"`
	Address   []common.Address `json:"address"`
	Topics    [][]common.Hash  `json:"topics"`
	FromBlock json.RawMessage  `json:"fromBlock"`
	ToBlock   json.RawMessage  `json:"toBlock"`
}

func (m *daveConsensusViewRPC) GetLogs(_ context.Context, query daveEventQuery) ([]types.Log, error) {
	args := m.Called(query)
	logs, _ := args.Get(0).([]types.Log)
	return logs, args.Error(1)
}

func TestDaveConsensusAdapterFiltersAcceptanceByExactBlockHashContractAndEpoch(t *testing.T) {
	contract, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	require.NoError(t, err)
	address, block := common.HexToAddress("0x100"), common.HexToHash("0x110")
	event := contract.Events["EpochSealed"]
	epoch := uint64(4)
	machine, outputs := common.HexToHash("0x400"), common.HexToHash("0x500")
	data, err := event.Inputs.NonIndexed().Pack(big.NewInt(5), big.NewInt(9), machine, outputs, common.HexToAddress("0x200"))
	require.NoError(t, err)
	log := types.Log{Address: address, BlockHash: block, BlockNumber: 110, TxHash: common.HexToHash("0x999"),
		Topics: []common.Hash{event.ID, common.BigToHash(new(big.Int).SetUint64(epoch))}, Data: data}
	backend := &daveConsensusViewRPC{}
	backend.On("GetLogs", daveEventQuery{BlockHash: block, Address: []common.Address{address},
		Topics: [][]common.Hash{{event.ID}, {common.BigToHash(new(big.Int).SetUint64(epoch))}},
	}).Return([]types.Log{log}, nil).Once()
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	adapter, err := NewDaveConsensusAdapter(address, client)
	require.NoError(t, err)
	events, err := adapter.EpochSealedAt(t.Context(), block, epoch)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, epoch, events[0].EpochNumber.Uint64())
	require.Equal(t, [32]byte(machine), events[0].InitialMachineStateHash)
	require.Equal(t, [32]byte(outputs), events[0].OutputsMerkleRoot)
	require.Equal(t, log, events[0].Raw)
	backend.AssertExpectations(t)
}

func (m *daveConsensusViewRPC) Call(
	_ context.Context, call map[string]json.RawMessage, block rpc.BlockNumber,
) (hexutil.Bytes, error) {
	var address common.Address
	var input hexutil.Bytes
	if err := json.Unmarshal(call["to"], &address); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(call["input"], &input); err != nil {
		return nil, err
	}
	args := m.Called(address, []byte(input), block)
	return args.Get(0).([]byte), args.Error(1)
}

func TestDaveConsensusAdapterDecodesContractViews(t *testing.T) {
	contractABI, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	require.NoError(t, err)
	address := common.HexToAddress("0x100")
	tournament := common.HexToAddress("0x11")
	winner := common.HexToHash("0x22")
	machineHash := common.HexToHash("0x33")
	outputsRoot := common.HexToHash("0x44")
	block := big.NewInt(20)

	for i, name := range []string{"first flag", "second flag", "third flag"} {
		t.Run(name, func(t *testing.T) {
			var flags [3]bool
			flags[i] = true
			backend := &daveConsensusViewRPC{}
			for _, view := range []struct {
				method string
				values []any
			}{
				{"getCurrentSealedEpoch", []any{
					big.NewInt(1), big.NewInt(2), big.NewInt(3), tournament, flags[0], big.NewInt(4), machineHash, outputsRoot,
				}},
				{"canStageTournamentResult", []any{flags[0], flags[1], flags[2], big.NewInt(5), winner, machineHash}},
				{"canAcceptStagedTournamentResult", []any{flags[0], flags[1], flags[2], big.NewInt(6), machineHash, outputsRoot}},
			} {
				method := contractABI.Methods[view.method]
				encoded, err := method.Outputs.Pack(view.values...)
				require.NoError(t, err)
				backend.On("Call", address, method.ID, rpc.BlockNumber(block.Int64())).Return(encoded, nil).Once()
			}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", backend))
			t.Cleanup(server.Stop)
			client := ethclient.NewClient(rpc.DialInProc(server))
			t.Cleanup(client.Close)
			adapter, err := NewDaveConsensusAdapter(address, client)
			require.NoError(t, err)
			opts := &bind.CallOpts{Context: t.Context(), BlockNumber: block}

			sealed, err := adapter.GetCurrentSealedEpoch(opts)
			require.NoError(t, err)
			require.Equal(t, CurrentSealedEpoch{
				EpochNumber: 1, InputIndexLowerBound: 2, InputIndexUpperBound: 3, Tournament: tournament,
				IsTournamentResultStaged: flags[0], StagingBlockNumber: 4,
				StagedPostEpochMachineStateHash: machineHash, StagedPostEpochOutputsMerkleRoot: outputsRoot,
			}, sealed)
			stage, err := adapter.CanStageTournamentResult(opts)
			require.NoError(t, err)
			require.Equal(t, CanStageTournamentResult{
				IsFinished: flags[0], IsTournamentFailed: flags[1], IsTournamentResultStaged: flags[2],
				EpochNumber: 5, WinnerCommitment: winner, WinnerPostEpochMachineStateHash: machineHash,
			}, stage)
			accept, err := adapter.CanAcceptStagedTournamentResult(opts)
			require.NoError(t, err)
			require.Equal(t, CanAcceptStagedTournamentResult{
				IsTournamentResultStaged: flags[0], DoAllSentriesAgreeWithStagedTournamentResult: flags[1],
				IsClaimStagingPeriodOver: flags[2], EpochNumber: 6,
				StagedPostEpochMachineStateHash: machineHash, StagedPostEpochOutputsMerkleRoot: outputsRoot,
			}, accept)
			backend.AssertExpectations(t)
		})
	}
}
