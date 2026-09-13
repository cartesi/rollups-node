// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/model"
)

// TestTransactionsWithoutDatabase keeps the node's database available, but gives
// each transaction command an invalid connection string. Only the node needs a
// database to process the input and publish its output proof.
func (s *EchoAuthoritySuite) TestTransactionsWithoutDatabase() {
	r := s.Require()
	s.appName = uniqueAppName("echo-no-cli-db")
	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	address, err := deployApplication(s.ctx, s.appName, dappPath, "--salt", uniqueSalt())
	r.NoError(err)
	r.NoError(anvilSetBalance(s.ctx, address, oneEtherWei))
	noDatabase := []string{"CARTESI_DATABASE_CONNECTION=invalid-database-connection"}

	// A wrong global InputBox must not override the Application contract.
	sendEnv := append([]string{"CARTESI_CONTRACTS_INPUT_BOX_ADDRESS=0x0000000000000000000000000000000000000001"}, noDatabase...)
	// Echo returns a voucher to the signer. Empty call data permits its EOA
	// destination; nonempty call data would require a destination contract.
	text, err := runCLIWithEnv(s.ctx, sendEnv, "send", address, "", "--yes", "--json")
	r.NoError(err, "send by address without database access")
	var sent cli.SendResult
	r.NoError(json.Unmarshal([]byte(text), &sent))
	r.Equal("mined", sent.Status)
	inputIndex, err := hexutil.DecodeUint64(sent.InputIndex)
	r.NoError(err)
	r.Zero(inputIndex)
	r.NotEmpty(sent.TransactionHash)

	processCtx, cancelProcess := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancelProcess()
	input, err := waitForInputProcessed(processCtx, s.T(), s.appName, inputIndex)
	r.NoError(err)
	r.Equal(model.InputCompletionStatus_Accepted, input.Status)
	minePastEpochBoundary(s.ctx, s.T(), r, s.appName, input.EpochIndex)
	claimCtx, cancelClaim := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	defer cancelClaim()
	_, err = waitForEpochStatus(claimCtx, s.T(), s.appName, input.EpochIndex, model.EpochStatus_ClaimAccepted)
	r.NoError(err)

	// Select the same two fields as the documented jq export. The CLI reads
	// them from the file during execution, with no database or node API lookup.
	outputs, err := readOutputs(s.ctx, s.appName)
	r.NoError(err)
	var voucherIndex *uint64
	const voucherOutputType = "Voucher"
	for _, output := range outputs.Data {
		if output.DecodedData != nil && output.DecodedData.Type == voucherOutputType {
			index := output.Index
			voucherIndex = &index
			break
		}
	}
	r.NotNil(voucherIndex, "echo output must include a voucher")
	indexText := strconv.FormatUint(*voucherIndex, 10)
	text, err = runCLIWithEnv(s.ctx, noDatabase, "read", "outputs", address, indexText, "--jsonrpc")
	r.NoError(err, "export proof through JSON-RPC without database access")
	var exported struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	r.NoError(json.Unmarshal([]byte(text), &exported))
	r.Contains(exported.Data, "raw_data")
	r.Contains(exported.Data, "output_hashes_siblings")
	proof, err := json.Marshal(map[string]json.RawMessage{
		"raw_data":               exported.Data["raw_data"],
		"output_hashes_siblings": exported.Data["output_hashes_siblings"],
	})
	r.NoError(err)
	proofFile := filepath.Join(s.T().TempDir(), "output-proof.json")
	r.NoError(os.WriteFile(proofFile, proof, 0o600))
	executeEnv := append([]string{"CARTESI_JSONRPC_API_URL=invalid-jsonrpc-url"}, noDatabase...)

	// A manual gas limit permits a call to an address with no code. Receipt
	// status 1 must not be reported as output execution without its event.
	client, err := ethclient.DialContext(s.ctx, envOrDefault("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545"))
	r.NoError(err)
	defer client.Close()
	const unusedAccountIndex = 9
	noCodeAddress := mnemonicAddress(s.T(), unusedAccountIndex)
	code, err := client.CodeAt(s.ctx, noCodeAddress, nil)
	r.NoError(err)
	r.Empty(code, "the negative test requires an address with no contract code")
	_, err = runCLIWithEnv(s.ctx, executeEnv, "execute", noCodeAddress.Hex(), indexText,
		"--proof-file", proofFile, "--yes", "--json", "--gas-limit=500000")
	r.ErrorContains(err, "no matching OutputExecuted event", "a mined no-op must not report output execution")

	text, err = runCLIWithEnv(s.ctx, executeEnv, "execute", address, indexText, "--proof-file", proofFile, "--yes", "--json")
	r.NoError(err, "execute with supplied data without database or node API access")
	var executed cli.TransactionResult
	r.NoError(json.Unmarshal([]byte(text), &executed))
	r.Equal("mined", executed.Status)
	r.NotEmpty(executed.TransactionHash)
	execCtx, cancelExecution := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancelExecution()
	r.NoError(waitForExecutionRecorded(execCtx, s.T(), s.appName, *voucherIndex))

	_, err = runCLIWithEnv(s.ctx, executeEnv,
		"execute", address, indexText, "--proof-file", proofFile, "--yes", "--json", "--gas-limit=0")
	r.ErrorContains(err, "OutputNotReexecutable", "the supplied proof must not permit replay")
}
