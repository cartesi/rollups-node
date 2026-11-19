// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

// ConsensusType represents the detected on-chain consensus mechanism.
type ConsensusType int

const (
	ConsensusUnknown   ConsensusType = iota
	ConsensusAuthority               // IAuthority (single validator, Ownable)
	ConsensusQuorum                  // IQuorum (multi-validator, majority threshold)
	ConsensusPRT                     // IDaveConsensus (PRT)
)

// String returns the human-readable consensus label.
func (t ConsensusType) String() string {
	switch t {
	case ConsensusAuthority:
		return "Authority"
	case ConsensusQuorum:
		return "Quorum"
	case ConsensusPRT:
		return "DaveConsensus (PRT)"
	case ConsensusUnknown:
		return "Unknown"
	}
	return "Unknown"
}

// ToModel maps the detected consensus type to the node's persisted enum.
// ConsensusUnknown maps to the empty value.
func (t ConsensusType) ToModel() model.Consensus {
	switch t {
	case ConsensusAuthority:
		return model.Consensus_Authority
	case ConsensusQuorum:
		return model.Consensus_Quorum
	case ConsensusPRT:
		return model.Consensus_PRT
	case ConsensusUnknown:
	}
	return ""
}

// ERC-165 interface IDs for consensus type detection.
// Solidity's type(I).interfaceId XORs only functions DEFINED in I, excluding inherited ones.
// Every literal below is pinned by a unit test that recomputes it from the Solidity
// signatures, so a drifting constant cannot silently select the wrong consensus type.
var (
	// IDataProvider (Dave v3.0.0-alpha.8): provideMerkleRootOfInput(uint256,bytes)
	iDataProviderInterfaceID = [4]byte{0x7a, 0x96, 0xf4, 0x80}
	// IConsensus interface IDs by version (own functions only, excluding inherited
	// isOutputsMerkleRootValid). Checked in order; first match wins.
	// v3.0.0-alpha: computed at init from the binding's ABI to stay in lockstep
	// with the contract — see computeIConsensusV3InterfaceID.
	iConsensusInterfaceIDv30 = computeIConsensusV3InterfaceID()
	// v2.2.0: submitClaim(address,uint256,bytes32) ^ getEpochLength() ^
	// getNumberOfAcceptedClaims() ^ getNumberOfSubmittedClaims()
	iConsensusInterfaceIDv220 = [4]byte{0x90, 0xb2, 0xf3, 0x46}
	// v2.1.1: submitClaim(address,uint256,bytes32) ^ getEpochLength() ^
	// getNumberOfAcceptedClaims() (no getNumberOfSubmittedClaims)
	iConsensusInterfaceIDv21x = [4]byte{0x7e, 0xec, 0xfc, 0xec}
	// IQuorum: own 7 functions (excluding inherited IConsensus). Signatures (types)
	// are identical across v2.1.x / v2.2.0 / v3 — only Solidity param NAMES changed,
	// which do not affect the selector.
	iQuorumInterfaceID = [4]byte{0x3c, 0x92, 0x5a, 0x62}

	// ERC-165 defines these two queries as the test that a contract really answers
	// supportsInterface instead of a fallback that returns true for any call.
	erc165InterfaceID        = [4]byte{0x01, 0xff, 0xc9, 0xa7} // supportsInterface(bytes4)
	erc165InvalidInterfaceID = [4]byte{0xff, 0xff, 0xff, 0xff} // must answer false
)

// CurrentIConsensusVersion labels the IConsensus interface that the node's contract
// bindings target. `app register` and `deploy application --consensus` refuse a
// consensus that does not report it, because the node's claim transactions would
// revert against an older generation.
const CurrentIConsensusVersion = "v3.0.0-alpha.10"

// computeIConsensusV3InterfaceID returns the ERC-165 interface ID of v3 IConsensus.
// Per Solidity's `type(I).interfaceId`, the ID is the XOR of selectors of the
// functions DECLARED in IConsensus (8 of them); inherited functions
// (`isOutputsMerkleRootValid` from IOutputsMerkleRootValidator, `version` from
// IVersionGetter, IApplicationChecker which has no functions) are excluded.
// Computed at package init from the binding's embedded ABI so a contract-level
// rename or signature change is automatically reflected.
func computeIConsensusV3InterfaceID() [4]byte {
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		panic(fmt.Errorf("computeIConsensusV3InterfaceID: parse ABI: %w", err))
	}
	// Methods declared in IConsensus.sol (v3), excluding inherited functions.
	methodNames := []string{
		"submitClaim",
		"acceptClaim",
		"getEpochLength",
		"getClaimStagingPeriod",
		"getNumberOfAcceptedClaims",
		"getNumberOfStagedClaims",
		"getNumberOfSubmittedClaims",
		"getClaim",
	}
	var id [4]byte
	for _, name := range methodNames {
		m, ok := parsed.Methods[name]
		if !ok {
			panic(fmt.Errorf("computeIConsensusV3InterfaceID: method %q not found in IConsensus ABI", name))
		}
		if len(m.ID) != 4 { //nolint:mnd // ABI method selectors are exactly four bytes.
			panic(fmt.Errorf("computeIConsensusV3InterfaceID: method %q selector is %d bytes, expected 4", name, len(m.ID)))
		}
		for i := range 4 {
			id[i] ^= m.ID[i]
		}
	}
	return id
}

// SupportsInterfaceCaller is the subset of an ERC-165 contract binding needed
// to identify a consensus implementation.
type SupportsInterfaceCaller interface {
	SupportsInterface(opts *bind.CallOpts, interfaceID [4]byte) (bool, error)
}

// ValidatorCounter is the subset of an IQuorum binding needed to validate the
// validator set of a detected Quorum consensus.
type ValidatorCounter interface {
	NumOfValidators(opts *bind.CallOpts) (*big.Int, error)
}

// iConsensusVersion pairs an ERC-165 interface ID with a human-readable contract version label.
type iConsensusVersion struct {
	id      [4]byte
	version string
}

var iConsensusVersions = []iConsensusVersion{
	{iConsensusInterfaceIDv30, CurrentIConsensusVersion},
	{iConsensusInterfaceIDv220, "v2.2.0"},
	{iConsensusInterfaceIDv21x, "v2.1.x"},
}

// DetectConsensusType uses ERC-165 supportsInterface to determine the consensus type.
// It first applies the ERC-165 detection procedure, then checks IDataProvider -> PRT,
// IQuorum -> Quorum, IConsensus -> Authority.
// Any read error aborts detection; there is no fallback classification.
// The returned contractVersion is non-empty only for Authority/Quorum, indicating which
// IConsensus interface version was matched.
func DetectConsensusType(
	opts *bind.CallOpts,
	caller SupportsInterfaceCaller,
) (ConsensusType, string, error) {
	if err := checkERC165(opts, caller); err != nil {
		return ConsensusUnknown, "", err
	}

	isPRT, err := caller.SupportsInterface(opts, iDataProviderInterfaceID)
	if err != nil {
		return ConsensusUnknown, "", fmt.Errorf("supportsInterface(IDataProvider): %w", err)
	}
	if isPRT {
		return ConsensusPRT, "", nil
	}

	isQuorum, err := caller.SupportsInterface(opts, iQuorumInterfaceID)
	if err != nil {
		return ConsensusUnknown, "", fmt.Errorf("supportsInterface(IQuorum): %w", err)
	}
	if isQuorum {
		// Quorum is also an IConsensus; probe the current IConsensus interface
		// to surface the contract version. Older Quorum versions (pre-v3) report
		// empty and the caller renders the label without the version suffix.
		isCurrent, err := caller.SupportsInterface(opts, iConsensusInterfaceIDv30)
		if err != nil {
			return ConsensusUnknown, "", fmt.Errorf(
				"supportsInterface(IConsensus %s) for Quorum: %w", CurrentIConsensusVersion, err)
		}
		if isCurrent {
			return ConsensusQuorum, CurrentIConsensusVersion, nil
		}
		return ConsensusQuorum, "", nil
	}

	for _, cv := range iConsensusVersions {
		isConsensus, err := caller.SupportsInterface(opts, cv.id)
		if err != nil {
			return ConsensusUnknown, "", fmt.Errorf("supportsInterface(IConsensus): %w", err)
		}
		if isConsensus {
			return ConsensusAuthority, cv.version, nil
		}
	}

	return ConsensusUnknown, "", errors.New(
		"contract does not implement IDataProvider, IQuorum, or IConsensus" +
			" — wrong address or unsupported consensus type")
}

// checkERC165 applies the detection procedure from EIP-165: the contract must answer
// true for the ERC-165 interface ID and false for 0xffffffff. Without it, a contract
// whose fallback returns ABI-encoded true for any call would be classified as PRT.
func checkERC165(opts *bind.CallOpts, caller SupportsInterfaceCaller) error {
	implementsERC165, err := caller.SupportsInterface(opts, erc165InterfaceID)
	if err != nil {
		return fmt.Errorf("supportsInterface(ERC-165): %w", err)
	}
	if !implementsERC165 {
		return errors.New("contract does not implement ERC-165 (EIP-165)")
	}
	invalidQuery, err := caller.SupportsInterface(opts, erc165InvalidInterfaceID)
	if err != nil {
		return fmt.Errorf("supportsInterface(0xffffffff): %w", err)
	}
	if invalidQuery {
		return errors.New(
			"contract does not implement ERC-165 (EIP-165): supportsInterface(0xffffffff) returned true")
	}
	return nil
}

// ValidateConsensusVersion refuses a consensus contract whose IConsensus interface
// version differs from the one the node's bindings target, because the node cannot
// operate it. PRT (Dave) contracts do not implement IConsensus and are exempt.
func ValidateConsensusVersion(detected ConsensusType, version string) error {
	if detected == ConsensusPRT {
		return nil
	}
	if version != CurrentIConsensusVersion {
		return fmt.Errorf("unsupported IConsensus contract version %q; the node requires %q",
			version, CurrentIConsensusVersion)
	}
	return nil
}

// ValidateQuorum verifies that a Quorum consensus has at least one validator.
// It rejects a nil count instead of dereferencing it.
func ValidateQuorum(opts *bind.CallOpts, counter ValidatorCounter) error {
	numOfValidators, err := counter.NumOfValidators(opts)
	if err != nil {
		return fmt.Errorf("NumOfValidators: %w", err)
	}
	if numOfValidators == nil || numOfValidators.Sign() == 0 {
		return errors.New("quorum consensus reports zero validators")
	}
	return nil
}
