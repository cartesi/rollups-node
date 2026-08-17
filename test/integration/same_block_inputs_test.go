// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// SameBlockInputsSuite covers the EVM reader's handling of several InputAdded
// events that share a single L1 block — the input-indexing and input→epoch
// assignment that a multi-input block exercises.
type SameBlockInputsSuite struct {
	suite.Suite
	LogChecker
	ctx     context.Context
	cancel  context.CancelFunc
	client  *ethclient.Client
	appName string
}

func TestSameBlockInputs(t *testing.T) {
	suite.Run(t, new(SameBlockInputsSuite))
}

// Generated from testdata/Spambox.sol with solc 0.8.30, no optimizer:
//
//	solc --abi --bin testdata/Spambox.sol
//
// Kept as static testdata so this integration test does not require forge or
// solc at runtime. Regenerate both files together whenever Spambox.sol changes.
//
//go:embed testdata/spambox_abi.json
var spamboxABIJSON string

//go:embed testdata/spambox_bytecode.hex
var spamboxBytecode string

func (s *SameBlockInputsSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 15*time.Minute)
	s.client = newIntegrationEthClient(s.ctx, s.T())
}

func (s *SameBlockInputsSuite) TearDownSuite() {
	if s.client != nil {
		s.client.Close()
	}
	s.cancel()
}

func (s *SameBlockInputsSuite) SetupTest() {
	s.StartLogCapture()
	s.appName = ""
}

func (s *SameBlockInputsSuite) TearDownTest() {
	if s.appName != "" {
		_ = disableApplication(s.ctx, s.appName) //nolint:errcheck
	}
	s.CheckLogs(s.T())
}

// TestMultipleInputsOneBlockAuthority sends several inputs in a single block to
// an Authority application and asserts they are all ingested with sequential
// indices and assigned to one epoch (a single block always maps to a single
// Authority epoch).
func (s *SameBlockInputsSuite) TestMultipleInputsOneBlockAuthority() {
	s.appName = uniqueAppName("same-block-inputs-authority")
	s.runMultipleInputsOneBlock(nil)
}

// TestMultipleInputsOneBlockPRT is the DaveConsensus counterpart: several inputs
// in a single block land in the current open epoch.
func (s *SameBlockInputsSuite) TestMultipleInputsOneBlockPRT() {
	s.appName = uniqueAppName("same-block-inputs-prt")
	s.runMultipleInputsOneBlock([]string{"--prt"})
}

// TestMultipleInputsOneTransactionAuthority reproduces F29's exact trigger:
// one L1 transaction emits several InputAdded logs for the same application.
func (s *SameBlockInputsSuite) TestMultipleInputsOneTransactionAuthority() {
	r := s.Require()
	s.appName = uniqueAppName("same-tx-inputs-authority")

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	appAddrStr, err := deployApplication(s.ctx, s.appName, dappPath, "--salt", uniqueSalt())
	r.NoError(err, "deploy app")
	appAddr := common.HexToAddress(appAddrStr)

	inputBoxAddr := inputBoxAddress(s.T())
	inputBox, err := iinputbox.NewIInputBox(inputBoxAddr, s.client)
	r.NoError(err, "bind input box")

	startIndex := inputBoxInputCount(s.ctx, s.T(), s.client, inputBoxAddr, appAddr)
	controlTx, err := inputBox.AddInput(transactorForMnemonicIndex(s.ctx, s.T(), s.client, 2),
		appAddr, []byte("CONTROL-SINGLE"))
	r.NoError(err, "submit control input")
	receiptCtx, receiptCancel := context.WithTimeout(s.ctx, 30*time.Second)
	controlReceipt := waitReceipt(receiptCtx, s.T(), s.client, controlTx)
	receiptCancel()
	r.Equal(uint64(1), controlReceipt.Status, "control input transaction must succeed")

	controlCtx, controlCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	controlInput, err := waitForInputProcessed(controlCtx, s.T(), s.appName, startIndex)
	controlCancel()
	r.NoError(err, "wait for control input")
	r.Equal(controlReceipt.TxHash, controlInput.TransactionHash)

	spambox := deploySpambox(s.ctx, s.T(), s.client, inputBoxAddr)

	const spamCount = 5
	spamOpts := transactorForMnemonicIndex(s.ctx, s.T(), s.client, 3)
	spamOpts.GasLimit = 3_000_000 //nolint:mnd
	spamTx, err := spambox.Transact(spamOpts, "spam", appAddr, big.NewInt(spamCount))
	r.NoError(err, "submit spam transaction")
	receiptCtx, receiptCancel = context.WithTimeout(s.ctx, 30*time.Second)
	spamReceipt := waitReceipt(receiptCtx, s.T(), s.client, spamTx)
	receiptCancel()
	r.Equal(uint64(1), spamReceipt.Status, "spam transaction must succeed")

	logIndexByInputIndex := make(map[uint64]uint64, spamCount)
	seenLogIndexes := make(map[uint64]struct{}, spamCount)
	for _, rawLog := range spamReceipt.Logs {
		event, err := inputBox.ParseInputAdded(*rawLog)
		if err != nil || event.AppContract != appAddr {
			continue
		}
		r.Equal(spamReceipt.TxHash, event.Raw.TxHash)
		inputIndex := event.Index.Uint64()
		logIndex := uint64(event.Raw.Index)
		logIndexByInputIndex[inputIndex] = logIndex
		seenLogIndexes[logIndex] = struct{}{}
	}
	r.Len(logIndexByInputIndex, spamCount, "spam tx must emit InputAdded logs for this app")
	r.Len(seenLogIndexes, spamCount, "spam logs must have distinct log indexes")

	for i := uint64(0); i < spamCount; i++ {
		idx := startIndex + 1 + i
		inputCtx, inputCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
		input, err := waitForInputProcessed(inputCtx, s.T(), s.appName, idx)
		inputCancel()
		r.NoError(err, "wait for spam input %d", idx)
		r.Equal(spamReceipt.TxHash, input.TransactionHash)
		r.Equal(logIndexByInputIndex[idx], input.LogIndex)
	}

	s.waitForInputCursorPast(spamReceipt.BlockNumber.Uint64())

	// The F29 wedge showed up as unique-violation retry noise: assert the spam
	// transaction produced none, on top of the cursor-advance check above.
	s.assertNoNodeLogLineContains("SQLSTATE 23505")

	allInputs, err := listInputsByRPC(s.ctx, s.appName, nil)
	r.NoError(err, "list all inputs through JSON-RPC")
	r.Equal(startIndex+spamCount+1, uint64(len(allInputs.Data)))
	for i, input := range allInputs.Data {
		r.Equal(startIndex+uint64(i), input.Index)
	}

	txHash := spamReceipt.TxHash.Hex()
	filtered, err := listInputsByRPC(s.ctx, s.appName, &txHash)
	r.NoError(err, "list inputs by spam transaction hash through JSON-RPC")
	r.Len(filtered.Data, spamCount)
	for i, input := range filtered.Data {
		expectedIndex := startIndex + 1 + uint64(i)
		r.Equal(expectedIndex, input.Index)
		r.Equal(spamReceipt.TxHash, input.TransactionHash)
		r.Equal(logIndexByInputIndex[expectedIndex], input.LogIndex)
	}

	out, err := runCLI(s.ctx, "read", "inputs", s.appName, "--transaction-hash", spamReceipt.TxHash.Hex())
	r.NoError(err, "list inputs by spam transaction hash")
	var cliFiltered api.ListResponse[model.Input]
	r.NoError(json.Unmarshal([]byte(out), &cliFiltered), "parse filtered inputs")
	r.Len(cliFiltered.Data, spamCount)
	for i, input := range cliFiltered.Data {
		expectedIndex := startIndex + 1 + uint64(i)
		r.Equal(expectedIndex, input.Index)
		r.Equal(spamReceipt.TxHash, input.TransactionHash)
		r.Equal(logIndexByInputIndex[expectedIndex], input.LogIndex)
	}
}

// runMultipleInputsOneBlock deploys an application, batches three inputs into a
// single L1 block, and verifies the node ingests them with contiguous indices,
// processes each through the machine, and assigns all of them to the same epoch.
func (s *SameBlockInputsSuite) runMultipleInputsOneBlock(extraDeployArgs []string) {
	r := s.Require()
	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")

	deployArgs := append([]string{"--salt", uniqueSalt()}, extraDeployArgs...)
	appAddrStr, err := deployApplication(s.ctx, s.appName, dappPath, deployArgs...)
	r.NoError(err, "deploy app")
	appAddr := common.HexToAddress(appAddrStr)

	inputBoxAddr := inputBoxAddress(s.T())
	startIndex := inputBoxInputCount(s.ctx, s.T(), s.client, inputBoxAddr, appAddr)

	inputBox, err := iinputbox.NewIInputBox(inputBoxAddr, s.client)
	r.NoError(err, "bind input box")

	nextOpts := func(signerIndex uint32, gasPriceGwei int64) *bind.TransactOpts {
		opts := *transactorForMnemonicIndex(s.ctx, s.T(), s.client, signerIndex)
		opts.GasLimit = 2_000_000 //nolint:mnd
		opts.GasPrice = new(big.Int).Mul(
			big.NewInt(gasPriceGwei),
			big.NewInt(1_000_000_000), //nolint:mnd
		)
		return &opts
	}

	// Batch the inputs into one block. The devnet mines on a one-second interval,
	// so disabling automine alone is not enough — interval mining would sweep the
	// pending txs into separate blocks. Turn both off, submit, mine once, restore.
	// Safe because the integration suites run sequentially (no t.Parallel).
	setAnvilAutomine(s.ctx, s.T(), false)
	defer setAnvilAutomine(s.ctx, s.T(), true)
	setAnvilIntervalMining(s.ctx, s.T(), 0)
	defer setAnvilIntervalMining(s.ctx, s.T(), 1)

	// Distinct funded senders with descending gas prices fix the in-block order
	// under anvil's default fee ordering, so input indices follow submission order.
	signers := []uint32{2, 3, 4}
	gasPrices := []int64{12, 11, 10}
	const numInputs = 3
	txs := make([]*types.Transaction, numInputs)
	for i := 0; i < numInputs; i++ {
		payload := []byte(fmt.Sprintf("same-block-input-%d", i))
		txs[i], err = inputBox.AddInput(nextOpts(signers[i], gasPrices[i]), appAddr, payload)
		r.NoError(err, "submit input %d", i)
	}

	r.NoError(anvilMine(s.ctx, 1), "mine the input batch into one block")

	// Every input must land in the same block, in submission order.
	receiptCtx, receiptCancel := context.WithTimeout(s.ctx, 30*time.Second)
	receipts := make([]*types.Receipt, numInputs)
	for i := 0; i < numInputs; i++ {
		receipts[i] = waitReceipt(receiptCtx, s.T(), s.client, txs[i])
		r.Equal(uint64(1), receipts[i].Status, "input %d transaction must succeed", i)
	}
	receiptCancel()
	for i := 1; i < numInputs; i++ {
		r.Equal(receipts[0].BlockNumber.Uint64(), receipts[i].BlockNumber.Uint64(),
			"all inputs must be mined in the same block")
		r.Less(receipts[i-1].TransactionIndex, receipts[i].TransactionIndex,
			"inputs must keep submission order within the block")
	}

	// Every input must be ingested with a contiguous index, processed by the
	// machine, and assigned to the same epoch.
	var epochIndex uint64
	for i := 0; i < numInputs; i++ {
		idx := startIndex + uint64(i)
		inputCtx, inputCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
		input, err := waitForInputProcessed(inputCtx, s.T(), s.appName, idx)
		inputCancel()
		r.NoError(err, "wait for input %d to be processed", idx)
		r.Equal(model.InputCompletionStatus_Accepted, input.Status, "input %d should be accepted", idx)
		if i == 0 {
			epochIndex = input.EpochIndex
		} else {
			r.Equal(epochIndex, input.EpochIndex,
				"all inputs from one block must be assigned to the same epoch")
		}
	}

	s.T().Logf("batched %d inputs into block %d, indices %d..%d, all in epoch %d",
		numInputs, receipts[0].BlockNumber.Uint64(), startIndex, startIndex+numInputs-1, epochIndex)
}

// TestInputsBeforeAndAfterEpochSealedSameBlock drives a DaveConsensus epoch
// boundary into the middle of a single L1 block: one input lands before the
// seal (EpochSealed) and one after, in the same block. The reader must assign
// the pre-seal input to the just-sealed epoch and the post-seal input to the
// new open epoch — the case where the two epochs overlap on one block
// (sealed.LastBlock == open.FirstBlock).
//
// The seal is DaveConsensus.settle(), which the node also issues on its own.
// We submit our own settle inside the batch (reusing the validator's computed
// outputs root + proof from the DB) and bracket the two inputs with extreme gas
// prices: the pre-seal input is mined first and the post-seal input last, so
// whichever settle the block orders between them captures an input-index
// boundary of exactly one. A duplicate node settle in the same block reverts
// harmlessly, leaving the same boundary.
func (s *SameBlockInputsSuite) TestInputsBeforeAndAfterEpochSealedSameBlock() {
	r := s.Require()
	s.SetExpectedLogs(s.T(), prtBlockOutOfRangeAllowlist)
	s.appName = uniqueAppName("same-block-seal-prt")

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	appAddrStr, err := deployApplication(s.ctx, s.appName, dappPath, "--salt", uniqueSalt(), "--prt")
	r.NoError(err, "deploy PRT app")
	appAddr := common.HexToAddress(appAddrStr)

	inputBoxAddr := inputBoxAddress(s.T())
	r.Equal(uint64(0), inputBoxInputCount(s.ctx, s.T(), s.client, inputBoxAddr, appAddr),
		"fresh PRT app should have no inputs yet (epoch 0 is sealed empty at deploy)")

	// Epoch 0 is sealed empty at deploy. Wait for the node to join its root
	// tournament and for the validator to compute the outputs root + proof that
	// settle(0) needs. This happens before we pass the timeout, so the node has
	// not settled epoch 0 itself yet.
	tournament := s.mineUntilTournamentReady(0)
	root, proof, consensusAddr := s.readEpochSettlementData(0)

	inputBox, err := iinputbox.NewIInputBox(inputBoxAddr, s.client)
	r.NoError(err, "bind input box")
	consensus, err := idaveconsensus.NewIDaveConsensus(consensusAddr, s.client)
	r.NoError(err, "bind dave consensus")

	nextOpts := func(signerIndex uint32, gasPriceGwei int64) *bind.TransactOpts {
		opts := *transactorForMnemonicIndex(s.ctx, s.T(), s.client, signerIndex)
		opts.GasLimit = 3_000_000 //nolint:mnd
		opts.GasPrice = new(big.Int).Mul(
			big.NewInt(gasPriceGwei),
			big.NewInt(1_000_000_000), //nolint:mnd
		)
		return &opts
	}

	// Keep mining under our control for the whole settle window so neither the
	// timeout blocks nor the node's own settle get auto-mined out from under us.
	setAnvilAutomine(s.ctx, s.T(), false)
	defer setAnvilAutomine(s.ctx, s.T(), true)
	setAnvilIntervalMining(s.ctx, s.T(), 0)
	defer setAnvilIntervalMining(s.ctx, s.T(), 1)

	// Make settle(0) valid by passing epoch 0's root-tournament timeout.
	_, err = mineForTournamentTimeout(s.ctx, s.client, tournament.Address)
	r.NoError(err, "mine past epoch 0 tournament timeout")

	root32 := [32]byte(root)
	proof32 := make([][32]byte, len(proof))
	for i := range proof {
		proof32[i] = [32]byte(proof[i])
	}

	// One block: pre-seal input (top gas → first), seal (mid gas), post-seal
	// input (bottom gas → last). Distinct senders avoid same-account nonce
	// coupling. We do not assert the seal receipt status — if the node's own
	// settle wins the in-block ordering, ours reverts, but the boundary is the
	// same because both settles execute between the two inputs.
	preTx, err := inputBox.AddInput(nextOpts(2, 1000), appAddr, []byte("before-seal")) //nolint:mnd
	r.NoError(err, "submit pre-seal input")
	_, err = consensus.Settle(nextOpts(3, 500), big.NewInt(0), root32, proof32) //nolint:mnd
	r.NoError(err, "submit settle for epoch 0")
	postTx, err := inputBox.AddInput(nextOpts(4, 1), appAddr, []byte("after-seal"))
	r.NoError(err, "submit post-seal input")

	r.NoError(anvilMine(s.ctx, 1), "mine the input/seal/input batch into one block")

	// Both inputs must share the seal block, in order.
	receiptCtx, receiptCancel := context.WithTimeout(s.ctx, 30*time.Second)
	preReceipt := waitReceipt(receiptCtx, s.T(), s.client, preTx)
	postReceipt := waitReceipt(receiptCtx, s.T(), s.client, postTx)
	receiptCancel()
	r.Equal(uint64(1), preReceipt.Status, "pre-seal input transaction must succeed")
	r.Equal(uint64(1), postReceipt.Status, "post-seal input transaction must succeed")
	r.Equal(preReceipt.BlockNumber.Uint64(), postReceipt.BlockNumber.Uint64(),
		"both inputs must be mined in the same block as the seal")
	r.Less(preReceipt.TransactionIndex, postReceipt.TransactionIndex,
		"pre-seal input must be ordered before post-seal input")
	sealBlock := preReceipt.BlockNumber.Uint64()

	// The pre-seal input belongs to the just-sealed epoch 1; the post-seal input
	// belongs to the new open epoch 2.
	preCtx, preCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	preInput, err := waitForInputProcessed(preCtx, s.T(), s.appName, 0)
	preCancel()
	r.NoError(err, "wait for pre-seal input processing")
	r.Equal(uint64(1), preInput.EpochIndex, "pre-seal input must land in the sealed epoch (1)")

	postCtx, postCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	postInput, err := waitForInputProcessed(postCtx, s.T(), s.appName, 1)
	postCancel()
	r.NoError(err, "wait for post-seal input processing")
	r.Equal(uint64(2), postInput.EpochIndex, "post-seal input must land in the new open epoch (2)")

	// The block boundary overlaps: the sealed epoch's last block equals both the
	// seal block and the open epoch's first block.
	sealedEpoch, err := readEpoch(s.ctx, s.appName, 1)
	r.NoError(err, "read sealed epoch 1")
	openEpoch, err := readEpoch(s.ctx, s.appName, 2)
	r.NoError(err, "read open epoch 2")
	r.Equal(sealBlock, sealedEpoch.LastBlock, "sealed epoch's last block must equal the seal block")
	r.Equal(sealedEpoch.LastBlock, openEpoch.FirstBlock, "epoch boundary must overlap by one block")

	s.T().Logf("seal block=%d: input 0 -> epoch 1 (sealed [0,1)), input 1 -> epoch 2 (open)", sealBlock)
}

// mineUntilTournamentReady waits for the application's root tournament for the
// given epoch and for the node to join it with a commitment, mining one block
// per poll so the chain keeps advancing. The devnet does not mine empty blocks
// when idle, and the EVM reader needs new block headers to process the sealed
// epoch and the node's commitment-join event; without the nudge a freshly
// deployed app that receives no transactions would never make progress.
func (s *SameBlockInputsSuite) mineUntilTournamentReady(epochIndex uint64) *model.Tournament {
	r := s.Require()
	ctx, cancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	defer cancel()

	var tournament *model.Tournament
	err := pollUntil(ctx, 2*time.Second, func() (bool, error) {
		if err := anvilMine(ctx, 1); err != nil {
			return false, err
		}
		tournaments, err := readTournaments(ctx, s.appName)
		if err != nil {
			if isCLIExitError(err) {
				return false, nil
			}
			return false, err
		}
		tournament = findRootTournament(tournaments.Data, epochIndex)
		if tournament == nil {
			return false, nil
		}
		commitments, err := readCommitments(ctx, s.appName)
		if err != nil {
			if isCLIExitError(err) {
				return false, nil
			}
			return false, err
		}
		return findCommitmentForEpoch(commitments.Data, epochIndex) != nil, nil
	})
	r.NoError(err, "wait for epoch %d tournament and commitment", epochIndex)
	return tournament
}

// readEpochSettlementData reads, from the node database, the outputs merkle root
// and proof the validator computed for the epoch (used to drive settle ourselves)
// and the application's DaveConsensus address. It waits until the root has been
// computed.
func (s *SameBlockInputsSuite) readEpochSettlementData(
	epochIndex uint64,
) (root common.Hash, proof []common.Hash, consensusAddr common.Address) {
	r := s.Require()
	dsn, err := config.GetDatabaseConnection()
	r.NoError(err, "get database connection")
	repo, err := factory.NewRepositoryFromConnectionString(s.ctx, dsn.Raw())
	r.NoError(err, "open repository")
	defer repo.Close()

	app, err := repo.GetApplication(s.ctx, s.appName)
	r.NoError(err, "get application")
	r.NotNil(app, "application must exist")
	consensusAddr = app.IConsensusAddress

	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()
	err = pollUntil(ctx, 2*time.Second, func() (bool, error) {
		epoch, err := repo.GetEpoch(ctx, s.appName, epochIndex)
		if err != nil {
			return false, err
		}
		if epoch == nil || epoch.TxBufferDataBlock == nil {
			return false, nil
		}
		root = *epoch.TxBufferDataBlock
		proof = epoch.TxBufferProof
		return true, nil
	})
	r.NoError(err, "wait for epoch %d settlement data (outputs merkle root)", epochIndex)
	return root, proof, consensusAddr
}

// deploySpambox deploys the Spambox helper contract and fails the test on any
// error, matching the waitReceipt style it already depends on.
func deploySpambox(
	ctx context.Context,
	t testing.TB,
	client *ethclient.Client,
	inputBoxAddr common.Address,
) *bind.BoundContract {
	t.Helper()
	r := require.New(t)
	parsed, err := abi.JSON(strings.NewReader(spamboxABIJSON))
	r.NoError(err, "parse Spambox ABI")
	opts := transactorForMnemonicIndex(ctx, t, client, 5)
	opts.GasLimit = 3_000_000 //nolint:mnd
	_, tx, contract, err := bind.DeployContract(
		opts, parsed, common.FromHex(strings.TrimSpace(spamboxBytecode)), client, inputBoxAddr)
	r.NoError(err, "deploy Spambox")
	receiptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	receipt := waitReceipt(receiptCtx, t, client, tx)
	r.Equal(types.ReceiptStatusSuccessful, receipt.Status,
		"Spambox deployment reverted in tx %s", tx.Hash())
	return contract
}

func listInputsByRPC(
	ctx context.Context,
	appName string,
	transactionHash *string,
) (*api.ListResponse[model.Input], error) {
	req := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
		ID      int    `json:"id"`
	}{
		JSONRPC: "2.0",
		Method:  "cartesi_listInputs",
		Params: api.ListInputsParams{
			Application:     appName,
			TransactionHash: transactionHash,
			Limit:           50,
		},
		ID: 1,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := envOrDefault("CARTESI_JSONRPC_API_URL", "http://localhost:10011/rpc")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := anvilHTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 512)) //nolint:errcheck
		return nil, fmt.Errorf("cartesi_listInputs HTTP %d: %s", resp.StatusCode, string(payload))
	}

	var rpcResp struct {
		Result *api.ListResponse[model.Input] `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("cartesi_listInputs error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if rpcResp.Result == nil {
		return nil, fmt.Errorf("cartesi_listInputs returned no result")
	}
	return rpcResp.Result, nil
}

func (s *SameBlockInputsSuite) waitForInputCursorPast(blockNumber uint64) {
	r := s.Require()
	dsn, err := config.GetDatabaseConnection()
	r.NoError(err, "get database connection")
	repo, err := factory.NewRepositoryFromConnectionString(s.ctx, dsn.Raw())
	r.NoError(err, "open repository")
	defer repo.Close()

	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()
	err = pollUntil(ctx, 2*time.Second, func() (bool, error) {
		app, err := repo.GetApplication(ctx, s.appName)
		if err != nil {
			// Retry transient DB errors until the poll deadline, matching the
			// other wait helpers; a persistent failure still times out loudly.
			s.T().Logf("retrying GetApplication while waiting for input cursor: %v", err)
			return false, nil
		}
		if app == nil {
			return false, nil
		}
		return app.LastInputCheckBlock >= blockNumber, nil
	})
	r.NoError(err, "input cursor did not advance past block %d", blockNumber)
}

// assertNoNodeLogLineContains fails the test if any node log line emitted
// since StartLogCapture contains the given substring, regardless of level.
// Unlike CheckLogs, which only flags unexpected ERR lines, this pins the
// absence of a specific marker (e.g. SQLSTATE 23505 retry noise).
func (s *SameBlockInputsSuite) assertNoNodeLogLineContains(substr string) {
	logFile := os.Getenv("CARTESI_TEST_NODE_LOG_FILE")
	if logFile == "" {
		s.T().Log("CARTESI_TEST_NODE_LOG_FILE not set, skipping node log substring scan")
		return
	}
	f, err := os.Open(logFile)
	s.Require().NoError(err, "open node log file")
	defer f.Close()

	var hits []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // handle long lines (stack traces, JSON)
	for scanner.Scan() {
		line := stripANSI(scanner.Text())
		if ts, ok := parseLogTimestamp(line); ok && ts.Before(s.logStart) {
			continue
		}
		if strings.Contains(line, substr) {
			hits = append(hits, line)
		}
	}
	s.Require().NoError(scanner.Err(), "read node log file")
	s.Require().Empty(hits, "node logs must not contain %q", substr)
}
