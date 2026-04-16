// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/lmittmann/tint"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

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
	handler := tint.NewHandler(os.Stdout, opts)
	repository := &claimerRepositoryMock{}
	blockchain := &claimerBlockchainMock{
		submitterAddress: common.HexToAddress("0x0000000000000000000000000000000000000001"),
		hasSubmitter:     true,
	}

	claimer := &Service{
		submissionEnabled: true,
		claimsInFlight:    map[int64]inFlightTx{},
		acceptsInFlight:   map[int64]inFlightTx{},
		acceptAttempts:    map[acceptAttemptKey]uint64{},
		maxAcceptAttempts: defaultMaxAcceptAttempts,
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
	outputsMerkleRoot := common.HexToHash("0x01") // dummy value
	machineHash := common.HexToHash("0x03")       // dummy value; matches events via testMachineHash
	txHash := common.HexToHash("0x02")            // dummy value
	e := repotest.NewEpochBuilder(id).
		WithIndex(i).
		WithBlocks(i*10, i*10+9).
		WithStatus(status).
		WithClaimTransactionHash(txHash).
		WithOutputsMerkleRoot(outputsMerkleRoot).
		WithMachineHash(machineHash).
		Build()
	if status == model.EpochStatus_ClaimStaged {
		// CHECK constraint: staged_iff_block.
		b := uint64(i*10 + 1)
		e.StagedAtBlock = &b
	}
	return e
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
		OutputsMerkleRoot:        *epoch.OutputsMerkleRoot,
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
		*epoch.OutputsMerkleRoot,
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
		OutputsMerkleRoot:        *epoch.OutputsMerkleRoot,
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
		OutputsMerkleRoot:        *epoch.OutputsMerkleRoot,
		MachineMerkleRoot:        testMachineHash(epoch),
		Raw: types.Log{
			TxHash:      common.HexToHash(epoch.ClaimTransactionHash.Hex()),
			BlockNumber: epoch.LastBlock + 5,
		},
	}
}

// rpcDataError simulates an RPC error with revert data, as returned by
// eth_estimateGas when the contract reverts.
type rpcDataError struct {
	code int
	msg  string
	data any
}

func (e *rpcDataError) Error() string  { return e.msg }
func (e *rpcDataError) ErrorCode() int { return e.code }
func (e *rpcDataError) ErrorData() any { return e.data }

// notFirstClaimError creates an error that mimics a NotFirstClaim revert
// from eth_estimateGas, with the ABI error selector as revert data.
func notFirstClaimError() error {
	parsed, _ := iconsensus.IConsensusMetaData.GetAbi()
	id := parsed.Errors["NotFirstClaim"].ID
	selector := fmt.Sprintf("0x%x", id[:4])
	return &rpcDataError{
		code: 3,
		msg:  "execution reverted",
		data: selector + "000000000000000000000000" +
			"01000000000000000000000000000000000000000000000000000000000000" +
			"0000000000000000000000000000000000000000000000000000000000000027",
	}
}

// consensusRevertError creates a typed revert with only the 4-byte selector —
// sufficient for the classifier to match by name. Looks up the error in
// IConsensus first, then IQuorum (for Quorum-only errors like
// CallerIsNotValidator), then IApplication (for merkle library errors like
// InvalidNodeIndex, which consensus calls raise but only the application ABI
// declares).
func consensusRevertError(errorName string) error {
	consensusABI, _ := iconsensus.IConsensusMetaData.GetAbi()
	quorumABI, _ := iquorum.IQuorumMetaData.GetAbi()
	applicationABI, _ := iapplication.IApplicationMetaData.GetAbi()
	var id common.Hash
	if e, ok := consensusABI.Errors[errorName]; ok {
		id = e.ID
	} else if e, ok := quorumABI.Errors[errorName]; ok {
		id = e.ID
	} else if e, ok := applicationABI.Errors[errorName]; ok {
		id = e.ID
	} else {
		panic(fmt.Sprintf("unknown typed error: %s", errorName))
	}
	return &rpcDataError{
		code: 3,
		msg:  "execution reverted",
		data: fmt.Sprintf("0x%x", id[:4]),
	}
}

// appRevertDataError creates an ApplicationReverted or
// IllformedApplicationReturnData revert carrying the given application
// returndata, ABI-encoded as the contract would emit it.
func appRevertDataError(errorName string, returndata []byte) error {
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	abiErr, ok := parsed.Errors[errorName]
	if !ok {
		panic(fmt.Sprintf("unknown typed error: %s", errorName))
	}
	packed, err := abiErr.Inputs.Pack(
		common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"),
		returndata,
	)
	if err != nil {
		panic(err)
	}
	payload := append(append([]byte{}, abiErr.ID[:4]...), packed...)
	return &rpcDataError{
		code: 3,
		msg:  "execution reverted",
		data: fmt.Sprintf("0x%x", payload),
	}
}

// notPastBlockError creates a NotPastBlock revert carrying the contract's
// (lastProcessedBlockNumber, upperBound) arguments, ABI-encoded as the
// contract would emit it.
func notPastBlockError(lastProcessed, upperBound uint64) error {
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	abiErr := parsed.Errors["NotPastBlock"]
	packed, err := abiErr.Inputs.Pack(
		new(big.Int).SetUint64(lastProcessed),
		new(big.Int).SetUint64(upperBound),
	)
	if err != nil {
		panic(err)
	}
	payload := append(append([]byte{}, abiErr.ID[:4]...), packed...)
	return &rpcDataError{
		code: 3,
		msg:  "execution reverted",
		data: fmt.Sprintf("0x%x", payload),
	}
}

// claimNotStagedError creates a typed ClaimNotStaged revert carrying the
// given on-chain claim status, ABI-encoded as the contract would emit it.
func claimNotStagedError(status uint8) error {
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	abiErr := parsed.Errors["ClaimNotStaged"]
	packed, err := abiErr.Inputs.Pack(
		common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"),
		big.NewInt(42),
		[32]byte(common.HexToHash("0xabcd")),
		status,
	)
	if err != nil {
		panic(err)
	}
	payload := append(append([]byte{}, abiErr.ID[:4]...), packed...)
	return &rpcDataError{
		code: 3,
		msg:  "execution reverted",
		data: fmt.Sprintf("0x%x", payload),
	}
}

// TestDecodeClaimNotStagedStatus pins the ABI-decode path used by
// handleAcceptClaimRevert. The status byte must come from the contract's

func withForeclosed(app *model.Application, block uint64) *model.Application {
	copy := *app
	copy.ForecloseBlock = block
	txHash := common.HexToHash("0xcafe")
	copy.ForecloseTransaction = &txHash
	return &copy
}

// TestSubmitClaimForeclosesUnstagedForeclosedApp verifies the
// foreclosure-broadcast guard. A foreclosed app whose chain state is
// UNSTAGED still goes through the pre-submit reconciliation read
// (findClaimSubmittedEventAndSucc + getClaimStatus) — those would mirror
// any pre-foreclosure on-chain-accepted state into the local DB — but the
// submitClaimToBlockchain broadcast must be SKIPPED and the local claim

func makeStagedEpoch(app *model.Application, i uint64, stagedAtBlock uint64) *model.Epoch {
	e := makeEpoch(app.ID, model.EpochStatus_ClaimStaged, i)
	e.StagedAtBlock = &stagedAtBlock
	return e
}

// TestStagingFastPathDivergence — Authority's submitClaim receipt contains a
// ClaimStaged event with a divergent machineMerkleRoot. The fast path detects

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

// TestStageByObservation — submitted epoch + ClaimStaged event observed in
// the next-tick scan → transition to CLAIM_STAGED with staged_at_block
