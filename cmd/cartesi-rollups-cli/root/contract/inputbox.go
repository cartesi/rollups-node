// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
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

type applicationInputBoxCaller interface {
	GetInputBox(opts *bind.CallOpts) (common.Address, error)
}

// resolveInputBoxAddress reads the InputBox selected by this application.
func (c *chainClient) resolveInputBoxAddress() (common.Address, error) {
	app, err := iapplication.NewIApplicationCaller(c.appAddr, c.eth)
	if err != nil {
		return common.Address{}, fmt.Errorf("bind IApplication for InputBox discovery: %w", err)
	}
	return readApplicationInputBox(app, c.callOpts)
}

func readApplicationInputBox(caller applicationInputBoxCaller, opts *bind.CallOpts) (common.Address, error) {
	inputBox, err := caller.GetInputBox(opts)
	if err != nil {
		return common.Address{}, fmt.Errorf("IApplication.GetInputBox: %w", err)
	}
	if inputBox == (common.Address{}) {
		return common.Address{}, fmt.Errorf("IApplication.GetInputBox returned the zero address")
	}
	return inputBox, nil
}
