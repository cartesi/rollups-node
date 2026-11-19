// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package ethutiltest provides test doubles for the JSON-RPC calls that the CLI
// makes to consensus contracts. It is imported by tests only.
package ethutiltest

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// ERC-165 interface IDs, recomputed from the signatures declared in the pinned
// Solidity sources (dave v3.0.0-alpha.8 for IDataProvider; rollups-contracts
// v2.2.0 and v3.0.0-alpha.10 for IConsensus, v3.0.0-alpha.10 for IQuorum).
// They intentionally do not come from the ethutil constants, so a fake built with
// them cannot follow a drifted constant.
var (
	// ERC165InterfaceID is the ERC-165 introspection interface ID.
	ERC165InterfaceID = [4]byte{0x01, 0xff, 0xc9, 0xa7}
	// ERC165InvalidInterfaceID must be answered with false by an ERC-165 contract.
	ERC165InvalidInterfaceID = [4]byte{0xff, 0xff, 0xff, 0xff}
	// IDataProviderInterfaceID identifies a Dave (PRT) consensus.
	IDataProviderInterfaceID = interfaceID("provideMerkleRootOfInput(uint256,bytes)")
	// IQuorumInterfaceID identifies a Quorum consensus.
	IQuorumInterfaceID = interfaceID(
		"numOfValidators()",
		"validatorId(address)",
		"validatorById(uint256)",
		"numOfValidatorsInFavorOfAnyClaimInEpoch(address,uint256)",
		"isValidatorInFavorOfAnyClaimInEpoch(address,uint256,uint256)",
		"numOfValidatorsInFavorOf(address,uint256,bytes32)",
		"isValidatorInFavorOf(address,uint256,bytes32,uint256)",
	)
	// IConsensusInterfaceID identifies the IConsensus generation the node binds.
	IConsensusInterfaceID = interfaceID(
		"submitClaim(address,uint256,bytes32,((bytes32,bytes32[]),(bytes32,bytes32[]),(bytes32,bytes32[])))",
		"acceptClaim(address,uint256,bytes32)",
		"getEpochLength()",
		"getClaimStagingPeriod()",
		"getNumberOfAcceptedClaims(address)",
		"getNumberOfStagedClaims(address)",
		"getNumberOfSubmittedClaims(address)",
		"getClaim(address,uint256,bytes32)",
	)
	// IConsensusV220InterfaceID identifies the older IConsensus generation 2.2.0.
	IConsensusV220InterfaceID = interfaceID(
		"submitClaim(address,uint256,bytes32)",
		"getEpochLength()",
		"getNumberOfAcceptedClaims()",
		"getNumberOfSubmittedClaims()",
	)
)

// interfaceID returns the ERC-165 interface ID: the XOR of the selectors of the
// declared functions, as Solidity's type(I).interfaceId computes it.
func interfaceID(signatures ...string) (id [4]byte) {
	for _, signature := range signatures {
		selector := crypto.Keccak256([]byte(signature))[:4]
		for i := range id {
			id[i] ^= selector[i]
		}
	}
	return id
}

// ERC165Only returns the answers of a contract that implements ERC-165 and the
// given interface IDs. Every other interface ID answers false.
func ERC165Only(ids ...[4]byte) map[[4]byte]bool {
	answers := map[[4]byte]bool{ERC165InterfaceID: true}
	for _, id := range ids {
		answers[id] = true
	}
	return answers
}

var (
	consensusABI = mustABI(iconsensus.IConsensusMetaData)
	quorumABI    = mustABI(iquorum.IQuorumMetaData)
)

func mustABI(metadata *bind.MetaData) *abi.ABI {
	parsed, err := metadata.GetAbi()
	if err != nil {
		panic(fmt.Errorf("ethutiltest: parse ABI: %w", err))
	}
	return parsed
}

// ConsensusRPC is a fake eth JSON-RPC backend that answers the eth_call queries
// made by consensus detection and by the deploy/register commands. Register it
// with rpc.Server.RegisterName("eth", ...).
//
// supportsInterface answers are keyed by the interface ID decoded from the call
// data, never by call order, so a wrong or reordered probe fails the test.
type ConsensusRPC struct {
	// SupportsInterface answers supportsInterface(interfaceID). Missing IDs answer false.
	SupportsInterface map[[4]byte]bool
	// SupportsInterfaceErr makes supportsInterface return a JSON-RPC error for an ID.
	SupportsInterfaceErr map[[4]byte]error
	// NumOfValidators answers IQuorum.numOfValidators(). Nil answers zero.
	NumOfValidators *big.Int
	// EpochLength answers IConsensus.getEpochLength().
	EpochLength uint64
	// ClaimStagingPeriod answers IConsensus.getClaimStagingPeriod().
	ClaimStagingPeriod uint64
}

// Call implements the eth_call JSON-RPC method.
func (r *ConsensusRPC) Call(_ context.Context, call struct {
	Input hexutil.Bytes `json:"input"`
}, _ string) (hexutil.Bytes, error) {
	if len(call.Input) < 4 { //nolint:mnd // A selector is four bytes.
		return nil, fmt.Errorf("consensus fake: call data too short: %x", call.Input)
	}
	method, err := consensusABI.MethodById(call.Input[:4])
	if err != nil {
		method, err = quorumABI.MethodById(call.Input[:4])
		if err != nil {
			return nil, fmt.Errorf("consensus fake: unexpected contract call: %x", call.Input)
		}
	}

	switch method.Name {
	case "supportsInterface":
		return r.supportsInterface(method, call.Input[4:])
	case "numOfValidators":
		validators := r.NumOfValidators
		if validators == nil {
			validators = big.NewInt(0)
		}
		return method.Outputs.Pack(validators)
	case "getEpochLength":
		return method.Outputs.Pack(new(big.Int).SetUint64(r.EpochLength))
	case "getClaimStagingPeriod":
		return method.Outputs.Pack(new(big.Int).SetUint64(r.ClaimStagingPeriod))
	default:
		return nil, fmt.Errorf("consensus fake: unexpected method %q", method.Name)
	}
}

func (r *ConsensusRPC) supportsInterface(method *abi.Method, input []byte) (hexutil.Bytes, error) {
	values, err := method.Inputs.Unpack(input)
	if err != nil {
		return nil, fmt.Errorf("consensus fake: unpack supportsInterface argument: %w", err)
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("consensus fake: supportsInterface expects one argument, got %d", len(values))
	}
	interfaceID, ok := values[0].([4]byte)
	if !ok {
		return nil, fmt.Errorf("consensus fake: supportsInterface argument is %T, not [4]byte", values[0])
	}
	if err := r.SupportsInterfaceErr[interfaceID]; err != nil {
		return nil, err
	}
	return method.Outputs.Pack(r.SupportsInterface[interfaceID])
}
