// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// DetectConsensusType identifies the consensus contract at addr through ERC-165.
// It binds the consensus contracts and adds the address to every error.
//
// It does not validate the result: callers decide which types they accept, and
// must call ethutil.ValidateConsensusVersion and ValidateQuorumConsensus when
// they need the node to be able to operate the contract.
func DetectConsensusType(
	ctx context.Context,
	caller bind.ContractCaller,
	addr common.Address,
) (ethutil.ConsensusType, string, error) {
	supportsInterface, err := iconsensus.NewIConsensusCaller(addr, caller)
	if err != nil {
		return ethutil.ConsensusUnknown, "", fmt.Errorf(
			"consensus %s: bind IConsensus: %w", addr, err)
	}
	detected, version, err := ethutil.DetectConsensusType(&bind.CallOpts{Context: ctx}, supportsInterface)
	if err != nil {
		return ethutil.ConsensusUnknown, "", fmt.Errorf("consensus %s: %w", addr, err)
	}
	return detected, version, nil
}

// ValidateQuorumConsensus verifies that a detected Quorum consensus reports at
// least one validator. The error names the address, so it is clear that the
// consensus was detected and this is a validation failure.
func ValidateQuorumConsensus(
	ctx context.Context,
	caller bind.ContractCaller,
	addr common.Address,
) error {
	quorum, err := iquorum.NewIQuorumCaller(addr, caller)
	if err != nil {
		return fmt.Errorf("consensus %s: bind IQuorum: %w", addr, err)
	}
	if err := ethutil.ValidateQuorum(&bind.CallOpts{Context: ctx}, quorum); err != nil {
		return fmt.Errorf("consensus %s: %w", addr, err)
	}
	return nil
}
