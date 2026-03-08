// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var inputboxCmd = &cobra.Command{
	Use:   "inputbox <application-address>",
	Short: "Query InputBox contract state (total inputs for application)",
	Args:  cobra.ExactArgs(1),
	RunE:  runInputBox,
}

func runInputBox(cmd *cobra.Command, args []string) error {
	cc, cancel, err := initChainClient(cmd, args)
	if err != nil {
		return err
	}
	defer cancel()
	defer cc.eth.Close()

	result, err := cc.queryInputBox()
	if err != nil {
		return err
	}

	if jsonParam {
		return outputJSON(result)
	}

	p := &printer{w: os.Stdout}
	p.withSection("InputBox", func() {
		if result.Address != "" {
			p.field("Address", result.Address)
		}
		p.field("Total Inputs", fmt.Sprintf("%d", result.TotalInputs))
	})
	p.footer(cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
	return nil
}

// queryInputBox returns the InputBox state for the application.
// The InputBox address is auto-discovered from DaveConsensus or provided via --inputbox flag.
func (c *chainClient) queryInputBox() (*InputBoxResult, error) {
	inputBoxAddr, err := c.resolveInputBoxAddress()
	if err != nil {
		return nil, err
	}

	if err := c.ensureContract(inputBoxAddr, "InputBox"); err != nil {
		return nil, err
	}

	caller, err := iinputbox.NewIInputBoxCaller(inputBoxAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IInputBox: %w", err)
	}

	totalRaw, err := caller.GetNumberOfInputs(c.callOpts, c.appAddr)
	if err != nil {
		return nil, fmt.Errorf("GetNumberOfInputs: %w", err)
	}
	total, err := safeUint64(totalRaw, "total inputs")
	if err != nil {
		return nil, err
	}

	return &InputBoxResult{
		Address:     formatAddr(inputBoxAddr),
		TotalInputs: total,
	}, nil
}

// resolveInputBoxAddress discovers the InputBox address.
// Priority: (1) --inputbox flag / env var, (2) DaveConsensus.GetInputBox().
func (c *chainClient) resolveInputBoxAddress() (common.Address, error) {
	// Try config (--inputbox flag or CARTESI_CONTRACTS_INPUT_BOX_ADDRESS env var).
	addr, err := config.GetContractsInputBoxAddress()
	if err == nil && addr != (common.Address{}) {
		return addr, nil
	}

	// Try auto-discovery from DaveConsensus.
	consensusAddr, cErr := c.getConsensusAddress()
	if cErr != nil {
		return common.Address{}, fmt.Errorf(
			"cannot determine InputBox address: no --inputbox flag and consensus lookup failed: %w",
			cErr)
	}

	cType, _, cErr := c.detectConsensus(consensusAddr)
	if cErr != nil {
		return common.Address{}, fmt.Errorf(
			"cannot determine InputBox address: no --inputbox flag and consensus detection failed: %w",
			cErr)
	}

	if cType == consensusDave {
		daveCaller, dErr := idaveconsensus.NewIDaveConsensusCaller(consensusAddr, c.eth)
		if dErr != nil {
			return common.Address{}, fmt.Errorf("bind IDaveConsensus for InputBox discovery: %w", dErr)
		}
		inputBox, dErr := daveCaller.GetInputBox(c.callOpts)
		if dErr != nil {
			return common.Address{}, fmt.Errorf("IDaveConsensus.GetInputBox: %w", dErr)
		}
		return inputBox, nil
	}

	return common.Address{}, fmt.Errorf(
		"cannot auto-discover InputBox address for %s consensus; "+
			"use --inputbox flag or set CARTESI_CONTRACTS_INPUT_BOX_ADDRESS", cType)
}
