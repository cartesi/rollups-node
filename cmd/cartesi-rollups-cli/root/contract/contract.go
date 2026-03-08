// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/cobra"
)

var (
	blockParam      string
	jsonParam       bool
	timeoutParam    string
	timestampsParam bool
)

// Cmd is the parent `contract` cobra command.
var Cmd = &cobra.Command{
	Use:   "contract",
	Short: "[Experimental] Read on-chain contract state directly via eth_call and eth_getLogs",
	Long: `
Read-only diagnostic tool that queries Cartesi Ethereum smart contracts directly.
No database required — only an RPC endpoint and an application address.

Experimental:
This tool is experimental and may produce inaccurate results. Values are read
directly from on-chain contracts and may not reflect finalized state. Verify
critical information independently before acting on it.

Supported Environment Variables:
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT    Ethereum RPC endpoint URL
  CARTESI_CONTRACTS_INPUT_BOX_ADDRESS InputBox contract address (optional override)`,
	Run: runContract,
}

func init() {
	Cmd.PersistentFlags().StringVar(&blockParam, "block", "latest",
		"Query view functions at this block number (default: latest, pinned at startup)")
	Cmd.PersistentFlags().BoolVar(&jsonParam, "json", false,
		"Machine-readable JSON output")
	Cmd.PersistentFlags().StringVar(&timeoutParam, "timeout", "30s",
		"Context deadline for the entire command execution")
	Cmd.PersistentFlags().BoolVar(&timestampsParam, "timestamps", false,
		"Resolve block numbers to wall-clock times (extra RPC calls)")

	// Unhide root persistent flags in the contract command's help.
	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, args []string) {
		if f := command.Flags().Lookup("blockchain-http-endpoint"); f != nil {
			f.Hidden = false
		}
		if f := command.Flags().Lookup("inputbox"); f != nil {
			f.Hidden = false
		}
		origHelpFunc(command, args)
	})

	Cmd.AddCommand(appCmd)
	Cmd.AddCommand(consensusCmd)
	Cmd.AddCommand(inputboxCmd)
	Cmd.AddCommand(tournamentCmd)
	Cmd.AddCommand(epochCmd)
	Cmd.AddCommand(summaryCmd)
	Cmd.AddCommand(outputCmd)
	Cmd.AddCommand(commitmentCmd)
	Cmd.AddCommand(matchCmd)
}

func runContract(cmd *cobra.Command, _ []string) {
	err := cmd.Help()
	cobra.CheckErr(err)
}

// consensusType represents the detected on-chain consensus mechanism.
type consensusType int

const (
	consensusUnknown   consensusType = iota
	consensusAuthority               // IAuthority (single validator, Ownable)
	consensusQuorum                  // IQuorum (multi-validator, majority threshold)
	consensusDave                    // IDaveConsensus (PRT)
)

func (t consensusType) String() string {
	switch t {
	case consensusAuthority:
		return "Authority"
	case consensusQuorum:
		return "Quorum"
	case consensusDave:
		return "DaveConsensus (PRT)"
	case consensusUnknown:
		return "Unknown"
	}
	return "Unknown"
}

// ERC-165 interface IDs for consensus type detection.
// Solidity's type(I).interfaceId XORs only functions DEFINED in I, excluding inherited ones.
var (
	// IDataProvider: provideMerkleRootOfInput(uint256,bytes)
	iDataProviderInterfaceID = [4]byte{0x7a, 0x96, 0xf4, 0x80}
	// IConsensus interface IDs by version (own functions only, excluding inherited
	// isOutputsMerkleRootValid). Checked in order; first match wins.
	// v2.2.0: submitClaim ^ getEpochLength ^ getNumberOfAcceptedClaims ^ getNumberOfSubmittedClaims
	iConsensusInterfaceIDv220 = [4]byte{0x90, 0xb2, 0xf3, 0x46}
	// v2.1.x: submitClaim ^ getEpochLength ^ getNumberOfAcceptedClaims (no getNumberOfSubmittedClaims)
	iConsensusInterfaceIDv21x = [4]byte{0x7e, 0xec, 0xfc, 0xec}
	// IQuorum: own 7 functions (excluding inherited IConsensus). Same across versions.
	iQuorumInterfaceID = [4]byte{0x3c, 0x92, 0x5a, 0x62}
)

// chainClient holds the shared Ethereum client and call options for all subcommands.
// All view functions are called through this client to ensure consistent block-number
// queries. The block number is ALWAYS pinned to a concrete value (never nil/latest).
type chainClient struct {
	eth      *ethclient.Client
	callOpts *bind.CallOpts
	filter   ethutil.Filter
	appAddr  common.Address
	chainID  uint64
	blockNum uint64

	// tsCache caches block number → timestamp lookups for --timestamps.
	tsMu    sync.RWMutex
	tsCache map[uint64]uint64
}

func newChainClient(
	ctx context.Context,
	endpoint string,
	appAddr common.Address,
	block *big.Int,
) (*chainClient, error) {
	client, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("connect to RPC endpoint: %w", err)
	}
	success := false
	defer func() {
		if !success {
			client.Close()
		}
	}()

	chainIDRaw, err := client.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("get chain ID: %w", err)
	}
	chainID, err := safeUint64(chainIDRaw, "chain ID")
	if err != nil {
		return nil, err
	}

	var blockNum uint64
	if block != nil && block.Sign() >= 0 {
		blockNum, err = safeUint64(block, "block number")
		if err != nil {
			return nil, err
		}
	} else {
		// block is nil ("latest") or negative ("safe", "finalized").
		// Resolve to a concrete block number via the RPC.
		header, hErr := client.HeaderByNumber(ctx, block)
		if hErr != nil {
			return nil, fmt.Errorf("resolve block tag: %w", hErr)
		}
		blockNum = header.Number.Uint64()
	}
	pinnedBlock := new(big.Int).SetUint64(blockNum)

	success = true
	return &chainClient{
		eth: client,
		callOpts: &bind.CallOpts{
			Context:     ctx,
			BlockNumber: pinnedBlock,
		},
		filter: ethutil.Filter{
			MinChunkSize: new(big.Int).Set(ethutil.DefaultMinChunkSize),
			Logger:       slog.Default(),
		},
		appAddr:  appAddr,
		chainID:  chainID,
		blockNum: blockNum,
		tsCache:  make(map[uint64]uint64),
	}, nil
}

// resolveTimestamp returns the timestamp for a block number, using a cache.
// Returns 0 if --timestamps is not enabled or the lookup fails.
func (c *chainClient) resolveTimestamp(blockNum uint64) uint64 {
	if !timestampsParam {
		return 0
	}
	// Check cache under read lock first to avoid holding the write lock
	// during a blocking RPC call.
	c.tsMu.RLock()
	if ts, ok := c.tsCache[blockNum]; ok {
		c.tsMu.RUnlock()
		return ts
	}
	c.tsMu.RUnlock()

	header, err := c.eth.HeaderByNumber(
		c.callOpts.Context, new(big.Int).SetUint64(blockNum))
	if err != nil {
		slog.Warn("failed to resolve timestamp", "block", blockNum, "error", err)
		return 0
	}

	c.tsMu.Lock()
	c.tsCache[blockNum] = header.Time
	c.tsMu.Unlock()
	return header.Time
}

// ensureContract verifies the address has deployed code at the pinned block.
func (c *chainClient) ensureContract(addr common.Address, label string) error {
	code, err := c.eth.CodeAt(c.callOpts.Context, addr, c.callOpts.BlockNumber)
	if err != nil {
		return fmt.Errorf("check contract code at %s: %w", addr, err)
	}
	if len(code) == 0 {
		return fmt.Errorf(
			"no contract deployed at %s (%s) — wrong address or wrong chain?",
			addr, label)
	}
	return nil
}

// iConsensusVersion pairs an ERC-165 interface ID with a human-readable contract version label.
type iConsensusVersion struct {
	id      [4]byte
	version string
}

var iConsensusVersions = []iConsensusVersion{
	{iConsensusInterfaceIDv220, "v2.2.0"},
	{iConsensusInterfaceIDv21x, "v2.1.x"},
}

// detectConsensus uses ERC-165 supportsInterface to determine the consensus type.
// Checks IDataProvider → Dave, IQuorum → Quorum, IConsensus → Authority.
// The returned contractVersion is non-empty only for Authority/Quorum, indicating which
// IConsensus interface version was matched.
func (c *chainClient) detectConsensus(
	consensusAddr common.Address,
) (consensusType, string, error) {
	caller, err := idaveconsensus.NewIDaveConsensusCaller(consensusAddr, c.eth)
	if err != nil {
		return consensusUnknown, "", fmt.Errorf("bind consensus contract: %w", err)
	}

	isDave, err := caller.SupportsInterface(c.callOpts, iDataProviderInterfaceID)
	if err != nil {
		return consensusUnknown, "", fmt.Errorf("supportsInterface(IDataProvider): %w", err)
	}
	if isDave {
		return consensusDave, "", nil
	}

	isQuorum, err := caller.SupportsInterface(c.callOpts, iQuorumInterfaceID)
	if err != nil {
		return consensusUnknown, "", fmt.Errorf("supportsInterface(IQuorum): %w", err)
	}
	if isQuorum {
		return consensusQuorum, "", nil
	}

	for _, cv := range iConsensusVersions {
		isConsensus, err := caller.SupportsInterface(c.callOpts, cv.id)
		if err != nil {
			return consensusUnknown, "", fmt.Errorf("supportsInterface(IConsensus): %w", err)
		}
		if isConsensus {
			return consensusAuthority, cv.version, nil
		}
	}

	return consensusUnknown, "", fmt.Errorf(
		"consensus at %s does not implement IDataProvider, IQuorum, or IConsensus"+
			" — wrong address or unsupported consensus type", consensusAddr)
}

// safeUint64 converts a *big.Int to uint64, returning an error if the value is nil or overflows.
func safeUint64(v *big.Int, field string) (uint64, error) {
	if v == nil {
		return 0, fmt.Errorf("%s: nil value", field)
	}
	if !v.IsUint64() {
		return 0, fmt.Errorf("%s value %s exceeds uint64 range", field, v.String())
	}
	return v.Uint64(), nil
}

// parseBlockFlag parses the --block flag value into a *big.Int.
// Returns nil for "latest". Returns negative big.Int for named tags
// ("safe", "finalized") using go-ethereum's rpc.BlockNumber constants.
// The caller resolves all non-positive values to a concrete block number.
func parseBlockFlag(s string) (*big.Int, error) {
	switch s {
	case "", "latest":
		return nil, nil
	case "safe":
		return big.NewInt(int64(rpc.SafeBlockNumber)), nil
	case "finalized":
		return big.NewInt(int64(rpc.FinalizedBlockNumber)), nil
	}
	n, ok := new(big.Int).SetString(s, 0)
	if !ok || n.Sign() < 0 {
		return nil, fmt.Errorf("invalid block number: %q (use a number, \"latest\", \"safe\", or \"finalized\")", s)
	}
	return n, nil
}

// validateHash checks that s is a valid 32-byte hex hash (with or without 0x prefix).
func validateHash(s, label string) error {
	h := strings.TrimPrefix(s, "0x")
	if len(h) != 64 { //nolint:mnd
		return fmt.Errorf("invalid %s: expected 32-byte hex (66 chars with 0x prefix), got %q", label, s)
	}
	if _, err := hex.DecodeString(h); err != nil {
		return fmt.Errorf("invalid %s: %w", label, err)
	}
	return nil
}

// initChainClient creates a chainClient from the command flags and arguments.
func initChainClient(cmd *cobra.Command, args []string) (*chainClient, context.CancelFunc, error) {
	if len(args) < 1 {
		return nil, nil, fmt.Errorf("application address argument is required")
	}

	appAddrStr := args[0]
	if !common.IsHexAddress(appAddrStr) {
		return nil, nil, fmt.Errorf("invalid application address: %q", appAddrStr)
	}
	appAddr := common.HexToAddress(appAddrStr)

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return nil, nil, fmt.Errorf("blockchain endpoint: %w", err)
	}

	block, err := parseBlockFlag(blockParam)
	if err != nil {
		return nil, nil, err
	}

	ctx := cmd.Context()

	var cancel context.CancelFunc
	if timeoutParam != "" {
		dur, parseErr := time.ParseDuration(timeoutParam)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("invalid timeout: %w", parseErr)
		}
		ctx, cancel = context.WithTimeout(ctx, dur)
	} else {
		cancel = func() {}
	}

	cc, err := newChainClient(ctx, ethEndpoint.Raw(), appAddr, block)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	return cc, cancel, nil
}
