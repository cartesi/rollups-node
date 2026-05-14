// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Constants mirror CanonicalMachine.sol.
const (
	log2DataBlockSize = 5
	log2MemorySize    = 64
)

// ValidateWithdrawalConfig is the Go mirror of LibWithdrawalConfig.isValid in
// src/library/LibWithdrawalConfig.sol. It exists so the CLI can surface a clear
// error before sending a transaction that would revert with the opaque
// InvalidWithdrawalConfig selector.
//
// A zero-valued config (no foreclosure / no withdrawal) is valid.
func ValidateWithdrawalConfig(wc iapplicationfactory.WithdrawalConfig) error {
	log2AccountsDriveSize := uint(log2DataBlockSize) +
		uint(wc.Log2MaxNumOfAccounts) +
		uint(wc.Log2LeavesPerAccount)

	if log2AccountsDriveSize > log2MemorySize {
		return fmt.Errorf(
			"withdrawal config: accounts drive larger than machine memory: log2(drive) = %d + %d + %d = %d > %d",
			log2DataBlockSize, wc.Log2MaxNumOfAccounts, wc.Log2LeavesPerAccount,
			log2AccountsDriveSize, log2MemorySize,
		)
	}

	endIndex := new(big.Int).SetUint64(wc.AccountsDriveStartIndex)
	endIndex.Add(endIndex, big.NewInt(1))
	accountsDriveEnd := new(big.Int).Lsh(endIndex, log2AccountsDriveSize)
	memorySize := new(big.Int).Lsh(big.NewInt(1), log2MemorySize)

	if accountsDriveEnd.Cmp(memorySize) > 0 {
		return fmt.Errorf(
			"withdrawal config: accounts drive ends past machine memory: (start+1=%s) << %d > 2^%d",
			endIndex.String(), log2AccountsDriveSize, log2MemorySize,
		)
	}

	return nil
}

// withdrawalConfigArgs is sourced from the abigen-generated
// IApplicationFactory ABI so the encoding stays in lockstep with the
// contract definition. The tuple type is taken from
// `calculateApplicationAddress` (unique signature, no overload ambiguity).
var withdrawalConfigArgs = func() abi.Arguments {
	parsed, err := iapplicationfactory.IApplicationFactoryMetaData.GetAbi()
	if err != nil {
		panic(fmt.Errorf("withdrawal_config: failed to parse iapplicationfactory ABI: %w", err))
	}
	method, ok := parsed.Methods["calculateApplicationAddress"]
	if !ok {
		panic("withdrawal_config: calculateApplicationAddress method not found in ABI")
	}
	for _, in := range method.Inputs {
		if in.Name == "withdrawalConfig" {
			return abi.Arguments{{Name: "withdrawalConfig", Type: in.Type}}
		}
	}
	panic("withdrawal_config: withdrawalConfig argument not found in calculateApplicationAddress ABI")
}()

// EncodeWithdrawalConfig serializes a WithdrawalConfig as the on-chain
// tuple ABI encoding (160 bytes for the all-static field layout). The
// all-zero config encodes to 160 zero bytes — the canonical "no
// foreclosure" representation.
func EncodeWithdrawalConfig(wc iapplicationfactory.WithdrawalConfig) ([]byte, error) {
	return withdrawalConfigArgs.Pack(wc)
}

// GetApplicationWithdrawalConfig reads the WithdrawalConfig struct from a
// deployed IApplication contract via the single getWithdrawalConfig() view.
// Used at registration time when the contract is the source of truth.
//
// abigen emits a distinct WithdrawalConfig struct per contract package, so
// the iapplication-binding result is copied into the iapplicationfactory
// shape callers expect for downstream encoding via EncodeWithdrawalConfig.
func GetApplicationWithdrawalConfig(
	ctx context.Context,
	client *ethclient.Client,
	appAddr common.Address,
) (iapplicationfactory.WithdrawalConfig, error) {
	app, err := iapplication.NewIApplication(appAddr, client)
	if err != nil {
		return iapplicationfactory.WithdrawalConfig{},
			fmt.Errorf("withdrawal config: failed to instantiate IApplication binding: %w", err)
	}

	wc, err := app.GetWithdrawalConfig(&bind.CallOpts{Context: ctx})
	if err != nil {
		return iapplicationfactory.WithdrawalConfig{},
			fmt.Errorf("withdrawal config: getWithdrawalConfig failed: %w", err)
	}
	return iapplicationfactory.WithdrawalConfig{
		Guardian:                wc.Guardian,
		Log2LeavesPerAccount:    wc.Log2LeavesPerAccount,
		Log2MaxNumOfAccounts:    wc.Log2MaxNumOfAccounts,
		AccountsDriveStartIndex: wc.AccountsDriveStartIndex,
		WithdrawalOutputBuilder: wc.WithdrawalOutputBuilder,
	}, nil
}

// VerifyDeployedWithdrawalConfig reads the WithdrawalConfig from the
// just-deployed application contract and asserts field-by-field equality
// against the config the caller passed to the factory. The four binding
// packages (iapplicationfactory, iselfhostedapplicationfactory,
// idaveappfactory, iapplication) each declare their own WithdrawalConfig
// struct; the deploy paths cross between them via Go type conversion,
// which silently masks any abigen field-order drift. This verify catches
// that drift the moment it ships, rather than at some downstream failure
// (claimer reverting, withdrawal output going to the wrong recipient).
func VerifyDeployedWithdrawalConfig(
	ctx context.Context,
	client *ethclient.Client,
	appAddr common.Address,
	expected iapplicationfactory.WithdrawalConfig,
) error {
	actual, err := GetApplicationWithdrawalConfig(ctx, client, appAddr)
	if err != nil {
		return fmt.Errorf("verify withdrawal config: %w", err)
	}
	if actual != expected {
		return fmt.Errorf(
			"verify withdrawal config: on-chain config at %s does not match factory input\n"+
				"  expected: %+v\n"+
				"  actual:   %+v",
			appAddr, expected, actual)
	}
	return nil
}

// CheckWithdrawalOutputBuilderCode performs a cheap sanity check on the
// builder address: if non-zero, it must have bytecode on chain. Skips the
// check when WithdrawalOutputBuilder is the zero address (no-foreclosure
// default).
func CheckWithdrawalOutputBuilderCode(
	ctx context.Context,
	client *ethclient.Client,
	wc iapplicationfactory.WithdrawalConfig,
) error {
	if wc.WithdrawalOutputBuilder == (common.Address{}) {
		return nil
	}
	code, err := client.CodeAt(ctx, wc.WithdrawalOutputBuilder, nil)
	if err != nil {
		return fmt.Errorf("withdrawal config: failed to read builder code at %s: %w",
			wc.WithdrawalOutputBuilder, err)
	}
	if len(code) == 0 {
		return fmt.Errorf("withdrawal config: builder address %s has no code on chain",
			wc.WithdrawalOutputBuilder)
	}
	return nil
}
