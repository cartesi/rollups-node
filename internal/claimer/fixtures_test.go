// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/lmittmann/tint"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type chainIDRPC struct {
	chainID uint64
}

func (s *chainIDRPC) ChainId(_ context.Context) (*hexutil.Big, error) {
	chainID := hexutil.Big(*new(big.Int).SetUint64(s.chainID))
	return &chainID, nil
}

func newTestEthClient(t *testing.T, chainID uint64) *ethclient.Client {
	server := rpc.NewServer()
	t.Cleanup(server.Stop)

	err := server.RegisterName("eth", &chainIDRPC{chainID: chainID})
	require.NoError(t, err)

	rpcClient := rpc.DialInProc(server)
	t.Cleanup(rpcClient.Close)

	client := ethclient.NewClient(rpcClient)
	t.Cleanup(client.Close)
	return client
}

func newServiceMock(t *testing.T) (*Service, *claimerRepositoryMock, *claimerBlockchainMock) {
	opts := &tint.Options{
		Level:     slog.LevelDebug,
		AddSource: true,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
	}
	handler := tint.NewTextHandler(os.Stdout, opts)
	repository := &claimerRepositoryMock{}
	blockchain := &claimerBlockchainMock{
		submitterAddress: common.HexToAddress("0x0000000000000000000000000000000000000001"),
		hasSubmitter:     true,
	}

	claimer := &Service{
		submissionEnabled: true,
		repository:        repository,
		blockchain:        blockchain,
	}
	service.InitTickServiceTemplate(
		&claimer.TickServiceTemplate,
		&service.TickServiceConfigs{
			BaseConfigs: service.BaseConfigs{
				Logger: slog.New(handler),
			},
		},
		claimer,
	)
	return claimer, repository, blockchain
}

func makeApplication() *model.Application {
	return repotest.NewApplicationBuilder().
		WithEpochLength(10).
		Build()
}

func makeEpoch(id int64, status model.EpochStatus, i uint64) *model.Epoch {
	outputsMerkleRoot := common.HexToHash("0x01")
	proof := repotest.KeccakStateProof(outputsMerkleRoot)
	txHash := common.HexToHash("0x02")
	e := repotest.NewEpochBuilder(id).
		WithIndex(i).
		WithBlocks(i*10, i*10+9).
		WithStatus(status).
		WithClaimTransactionHash(txHash).
		WithTxBufferDataBlock(outputsMerkleRoot).
		WithMachineHash(proof.MachineHash).
		Build()
	e.TxBufferProof = stateProofHashes(proof.TxBufferProof)
	e.IflagsYDataBlock = model.Pointer(proof.IflagsYDataBlock)
	e.IflagsYProof = stateProofHashes(proof.IflagsYProof)
	e.HtifTohostDataBlock = model.Pointer(proof.HtifTohostDataBlock)
	e.HtifTohostProof = stateProofHashes(proof.HtifTohostProof)
	if status == model.EpochStatus_ClaimStaged {
		// CHECK constraint: staged_iff_block.
		b := uint64(i*10 + 1)
		e.StagedAtBlock = &b
	}
	return e
}

func stateProofHashes(proof [][32]byte) []common.Hash {
	result := make([]common.Hash, len(proof))
	for i := range proof {
		result[i] = proof[i]
	}
	return result
}

func makeAcceptedEpoch(app *model.Application, i uint64) *model.Epoch {
	return makeEpoch(app.ID, model.EpochStatus_ClaimAccepted, i)
}

func makeSubmittedEpoch(app *model.Application, i uint64) *model.Epoch {
	return makeEpoch(app.ID, model.EpochStatus_ClaimSubmitted, i)
}

func makeComputedEpoch(app *model.Application, i uint64) *model.Epoch {
	return makeEpoch(app.ID, model.EpochStatus_ClaimComputed, i)
}
func makeEpochMap(epochs ...*model.Epoch) map[int64]*model.Epoch {
	result := map[int64]*model.Epoch{}
	for _, epoch := range epochs {
		result[epoch.ApplicationID] = epoch
	}
	return result
}
func makeApplicationMap(apps ...*model.Application) map[int64]*model.Application {
	result := map[int64]*model.Application{}
	for _, app := range apps {
		result[app.ID] = app
	}
	return result
}

// testMachineHash returns a stable [32]byte derived from the epoch — good
// enough for fixtures that don't need a real on-chain match. Tests that
// exercise the machineMerkleRoot cross-check should construct their own
// machine hash and use the field-named struct literal.
func testMachineHash(epoch *model.Epoch) [32]byte {
	if epoch.MachineHash != nil {
		return *epoch.MachineHash
	}
	return [32]byte{}
}

func makeSubmittedEvent(app *model.Application, epoch *model.Epoch) *iconsensus.IConsensusClaimSubmitted {
	return makeSubmittedEventWithTxHash(app, epoch, *epoch.ClaimTransactionHash)
}

func makeSubmittedEventWithTxHash(
	app *model.Application,
	epoch *model.Epoch,
	txHash common.Hash,
) *iconsensus.IConsensusClaimSubmitted {
	return &iconsensus.IConsensusClaimSubmitted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(epoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *epoch.TxBufferDataBlock,
		MachineMerkleRoot:        testMachineHash(epoch),
		Raw: types.Log{
			TxHash:      txHash,
			BlockNumber: epoch.LastBlock + 5,
		},
	}
}

func makeSubmittedEventWithRoots(
	app *model.Application,
	epoch *model.Epoch,
	outputs common.Hash,
	machine common.Hash,
) *iconsensus.IConsensusClaimSubmitted {
	event := makeSubmittedEvent(app, epoch)
	event.OutputsMerkleRoot = outputs
	event.MachineMerkleRoot = machine
	return event
}

// makeClaimStagedLog creates a types.Log that ParseClaimStaged can decode.
// Used to build receipt logs for the staging fast-path in tests.
func makeClaimStagedLog(app *model.Application, epoch *model.Epoch) types.Log {
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		panic(fmt.Sprintf("failed to get IConsensus ABI: %v", err))
	}
	event, ok := parsed.Events["ClaimStaged"]
	if !ok {
		panic("IConsensus ABI does not define ClaimStaged event")
	}
	data, err := event.Inputs.NonIndexed().Pack(
		new(big.Int).SetUint64(epoch.LastBlock),
		*epoch.TxBufferDataBlock,
		testMachineHash(epoch),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to pack ClaimStaged event data: %v", err))
	}
	return types.Log{
		Address: app.IConsensusAddress,
		Topics: []common.Hash{
			event.ID,
			common.BytesToHash(app.IApplicationAddress.Bytes()),
		},
		Data: data,
	}
}

// makeStagedEvent constructs an IConsensusClaimStaged matching the epoch.
func makeStagedEvent(app *model.Application, epoch *model.Epoch) *iconsensus.IConsensusClaimStaged {
	return &iconsensus.IConsensusClaimStaged{
		LastProcessedBlockNumber: new(big.Int).SetUint64(epoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *epoch.TxBufferDataBlock,
		MachineMerkleRoot:        testMachineHash(epoch),
		Raw: types.Log{
			BlockNumber: epoch.LastBlock + 5,
		},
	}
}

func makeAcceptedEvent(app *model.Application, epoch *model.Epoch) *iconsensus.IConsensusClaimAccepted {
	return &iconsensus.IConsensusClaimAccepted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(epoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *epoch.TxBufferDataBlock,
		MachineMerkleRoot:        testMachineHash(epoch),
		Raw: types.Log{
			TxHash:      common.HexToHash(epoch.ClaimTransactionHash.Hex()),
			BlockNumber: epoch.LastBlock + 5,
		},
	}
}

// withForeclosed returns a copy with the foreclosure block and transaction set.
func withForeclosed(app *model.Application, block uint64) *model.Application {
	foreclosed := *app
	foreclosed.ForecloseBlock = block
	txHash := common.HexToHash("0xcafe")
	foreclosed.ForecloseTransaction = &txHash
	return &foreclosed
}

func makeStagedEpoch(app *model.Application, i uint64, stagedAtBlock uint64) *model.Epoch {
	e := makeEpoch(app.ID, model.EpochStatus_ClaimStaged, i)
	e.StagedAtBlock = &stagedAtBlock
	return e
}

func buildClaimStagedLog(app *model.Application, epoch *model.Epoch,
	outputs common.Hash, machine common.Hash) types.Log {
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	event := parsed.Events["ClaimStaged"]
	data, err := event.Inputs.NonIndexed().Pack(
		new(big.Int).SetUint64(epoch.LastBlock),
		[32]byte(outputs),
		[32]byte(machine),
	)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Address: app.IConsensusAddress,
		Topics: []common.Hash{
			event.ID,
			common.BytesToHash(app.IApplicationAddress.Bytes()),
		},
		Data: data,
	}
}

const claimSubmittedEventName = "ClaimSubmitted"

func claimEventLog(app *model.Application, epoch *model.Epoch, name string) *types.Log {
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	event := parsed.Events[name]
	args := []any{new(big.Int).SetUint64(epoch.LastBlock), [32]byte(*epoch.TxBufferDataBlock), [32]byte(*epoch.MachineHash)}
	topics := []common.Hash{event.ID, common.BytesToHash(app.IApplicationAddress.Bytes())}
	if name == claimSubmittedEventName {
		topics = []common.Hash{event.ID, common.BytesToHash(common.HexToAddress("0x1").Bytes()),
			common.BytesToHash(app.IApplicationAddress.Bytes())}
	}
	data, err := event.Inputs.NonIndexed().Pack(args...)
	if err != nil {
		panic(err)
	}
	return &types.Log{Address: app.IConsensusAddress, Topics: topics, Data: data, BlockNumber: 99}
}

func failClaimObservation(t *testing.T, s *Service) {
	t.Helper()
	chain := s.blockchain.(*claimerBlockchainMock)
	cause := errors.New("observation provider unavailable")
	chain.On("getDefaultBlockNumber", mock.Anything).Return((*big.Int)(nil), cause).Once()
	_, err := s.Tick(t.Context())
	require.ErrorIs(t, err, cause)
	require.Nil(t, s.failure.Load())
}
