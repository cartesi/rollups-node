// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func withdrawalConfigForGuardian(t testing.TB, guardianIndex uint32) (string, common.Address) {
	t.Helper()
	r := require.New(t)

	builderEnv := os.Getenv("CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS")
	r.NotEmpty(builderEnv,
		"CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS must be set (run `eval $(make env)`)")

	guardianKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, guardianIndex)
	r.NoError(err, "derive guardian key")
	guardianAddr := crypto.PubkeyToAddress(guardianKey.PublicKey)

	withdrawalConfigJSON := strings.NewReplacer("\n", "", "\t", "").Replace(fmt.Sprintf(`{
		"guardian": "%s",
		"log2_leaves_per_account": 0,
		"log2_max_num_of_accounts": 20,
		"accounts_drive_start_index": 33554432,
		"withdrawal_output_builder": "%s"
	}`, guardianAddr.Hex(), builderEnv))

	return withdrawalConfigJSON, guardianAddr
}

func guardianForeclose(ctx context.Context, appName string, guardianIndex uint32) error {
	out, err := runCLIWithEnv(ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", guardianIndex)},
		"foreclose", appName, "--yes", "--json",
	)
	if err != nil {
		return fmt.Errorf("guardian foreclose CLI call: %w (%s)", err, out)
	}
	return nil
}

func firstVoucherOutputIndex(t testing.TB, outputs []api.DecodedOutput) uint64 {
	t.Helper()
	for _, output := range outputs {
		if output.DecodedData != nil && output.DecodedData.Type == "Voucher" {
			return output.Index
		}
	}
	require.FailNow(t, "voucher output not found")
	return 0
}

func newIntegrationEthClient(ctx context.Context, t testing.TB) *ethclient.Client {
	t.Helper()
	endpoint := envOrDefault("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	client, err := ethclient.DialContext(ctx, endpoint)
	require.NoError(t, err, "dial ethclient")
	return client
}

func inputBoxAddress(t testing.TB) common.Address {
	t.Helper()
	value := os.Getenv("CARTESI_CONTRACTS_INPUT_BOX_ADDRESS")
	require.NotEmpty(t, value, "CARTESI_CONTRACTS_INPUT_BOX_ADDRESS must be set (run `eval $(make env)`)")
	return common.HexToAddress(value)
}

func transactorForMnemonicIndex(
	ctx context.Context,
	t testing.TB,
	client *ethclient.Client,
	index uint32,
) *bind.TransactOpts {
	t.Helper()
	key, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, index)
	require.NoError(t, err, "derive mnemonic[%d] key", index)
	chainID, err := client.ChainID(ctx)
	require.NoError(t, err, "fetch chain id")
	opts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	require.NoError(t, err, "create transactor for mnemonic[%d]", index)
	opts.Context = ctx
	return opts
}

func inputBoxInputCount(
	ctx context.Context,
	t testing.TB,
	client *ethclient.Client,
	inputBoxAddr common.Address,
	appAddr common.Address,
) uint64 {
	t.Helper()
	inputBox, err := iinputbox.NewIInputBox(inputBoxAddr, client)
	require.NoError(t, err, "bind input box")
	count, err := inputBox.GetNumberOfInputs(&bind.CallOpts{Context: ctx}, appAddr)
	require.NoError(t, err, "read input count")
	require.True(t, count.IsUint64(), "input count must fit uint64")
	return count.Uint64()
}

func waitReceipt(ctx context.Context, t testing.TB, client *ethclient.Client, tx *types.Transaction) *types.Receipt {
	t.Helper()
	receipt, err := bind.WaitMined(ctx, client, tx)
	require.NoError(t, err, "wait for tx %s", tx.Hash().Hex())
	return receipt
}

func outputValidityProof(output *api.DecodedOutput) iapplication.OutputValidityProof {
	siblings := make([][32]byte, len(output.OutputHashesSiblings))
	for i, hash := range output.OutputHashesSiblings {
		siblings[i] = hash
	}
	return iapplication.OutputValidityProof{
		OutputIndex:          output.Index,
		OutputHashesSiblings: siblings,
	}
}

func setAnvilAutomine(ctx context.Context, t testing.TB, enabled bool) {
	t.Helper()
	require.NoError(t, anvilRPC(ctx, "evm_setAutomine", enabled), "set Anvil automine=%t", enabled)
}

// setAnvilIntervalMining sets Anvil's interval-mining period in seconds; 0
// disables interval mining. The devnet boots with --block-time 1, so a test that
// must batch several transactions into a single explicit block has to turn
// interval mining off first — disabling automine alone is not enough, because
// the one-second timer keeps sweeping pending transactions into separate blocks.
func setAnvilIntervalMining(ctx context.Context, t testing.TB, seconds int) {
	t.Helper()
	require.NoError(t, anvilRPC(ctx, "evm_setIntervalMining", seconds),
		"set Anvil interval mining=%ds", seconds)
}
