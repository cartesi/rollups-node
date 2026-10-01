// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cartesi/rollups-node/pkg/ethutil"
)

// Development defaults. Addresses match the Dave devnet bundle used by both our
// devnet image and the Dave harness; a case manifest records the addresses it
// was captured with. Every value can be overridden.
const (
	anvilChainID = 31337

	defaultInputBox = "0xEbE9f4Dfc04ae10bBeE663859c3dc5A23f94eA3C"
	// Anvil account 1: the withdrawal address configured in the honeypot program.
	defaultHoneypotWithdrawer = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

	// The public Anvil/Hardhat test mnemonic. Used only on chain 31337.
	testMnemonic = "test test test test test test test test test test test junk"

	authKindMnemonic = "mnemonic"
	envPrtMnemonic   = "CARTESI_PRT_AUTH_MNEMONIC"
	// programYield is the Dave test program that breaks the outputs-root rule.
	programYield    = "yield"
	programHoneypot = "honeypot"
	programEcho     = "echo"
	flagFalse       = "false"
	flagTrue        = "true"
	decimalBase     = 10

	defaultAnvilPort       = 18545
	defaultNodePortBase    = 11000
	defaultRollupsPrtIndex = 6
	// The Dave harness signer, and the run-sling default: test account 7.
	defaultSlingSignerIx = 7
	defaultCLIIndex      = 0
	defaultWithdrawerIx  = 1
)

// devDatabaseURL is the Postgres of "make start-postgres" (see "make env").
const devDatabaseURL = "postgres://postgres:password@localhost:5432/rollupsdb?sslmode=disable" //nolint:gosec // public dev default

// defaultDatabaseURL is the admin connection that the tool uses to create and
// drop run databases.
func defaultDatabaseURL() string {
	return envOr("CARTESI_DATABASE_CONNECTION", devDatabaseURL)
}

// defaultSlingBin is $DAVE_NODE_BIN, or the debug build in $DAVE_ROOT.
func defaultSlingBin() string {
	if bin := os.Getenv("DAVE_NODE_BIN"); bin != "" {
		return bin
	}
	if root := os.Getenv("DAVE_ROOT"); root != "" {
		return filepath.Join(root, slingBinaryPath)
	}
	return ""
}

// envOr returns the value of key, or fallback when key is unset or empty.
func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func parseAddress(name, value string) (common.Address, error) {
	if !common.IsHexAddress(value) {
		return common.Address{}, fmt.Errorf("%s: invalid address %q", name, value)
	}
	return common.HexToAddress(value), nil
}

// testAccount is a key derived from the test mnemonic. It exists only for
// local test chains; callers must check the chain ID first.
type testAccount struct {
	Index   uint32
	Address common.Address
	key     []byte
}

func deriveTestAccount(mnemonic string, index uint32) (*testAccount, error) {
	privateKey, err := ethutil.MnemonicToPrivateKey(mnemonic, index)
	if err != nil {
		return nil, fmt.Errorf("deriving account %d: %w", index, err)
	}
	address, err := ethutil.PrivateKeyToAddress(privateKey)
	if err != nil {
		return nil, fmt.Errorf("deriving account %d address: %w", index, err)
	}
	return &testAccount{Index: index, Address: address, key: crypto.FromECDSA(privateKey)}, nil
}

// requireTestChain refuses to use test keys outside a local test chain.
func requireTestChain(chainID uint64) error {
	if chainID != anvilChainID {
		return fmt.Errorf("refusing to use test-mnemonic keys on chain %d (only %d is allowed)", chainID, anvilChainID)
	}
	return nil
}

// defaultSlingSigner returns the Sling signer used by the harness on the
// local test chain, or the zero address on any other chain.
func defaultSlingSigner(chainID uint64) common.Address {
	if chainID != anvilChainID {
		return common.Address{}
	}
	account, err := deriveTestAccount(testMnemonic, defaultSlingSignerIx)
	if err != nil {
		return common.Address{}
	}
	return account.Address
}

// rollupsPrtSigner returns the address that the rollups node uses for PRT transactions.
func (s *session) rollupsPrtSigner() (common.Address, error) {
	if s.opts.rollupsPrtSigner != "" {
		return parseAddress("--rollups-prt-signer", s.opts.rollupsPrtSigner)
	}
	if kind := lookupEnv(s.env, "CARTESI_PRT_AUTH_KIND"); kind != "" && kind != authKindMnemonic {
		return common.Address{}, fmt.Errorf("CARTESI_PRT_AUTH_KIND is %s; pass --rollups-prt-signer", kind)
	}
	mnemonic := lookupEnv(s.env, envPrtMnemonic)
	if mnemonic == "" {
		return common.Address{}, errors.New("CARTESI_PRT_AUTH_MNEMONIC is unset; pass --rollups-prt-signer")
	}
	index, err := strconv.ParseUint(envValueOr(s.env, "CARTESI_PRT_AUTH_MNEMONIC_ACCOUNT_INDEX", "6"), 10, 32)
	if err != nil {
		return common.Address{}, err
	}
	account, err := deriveTestAccount(mnemonic, uint32(index))
	if err != nil {
		return common.Address{}, err
	}
	return account.Address, nil
}

func envValueOr(env []string, key, fallback string) string {
	if value := lookupEnv(env, key); value != "" {
		return value
	}
	return fallback
}
