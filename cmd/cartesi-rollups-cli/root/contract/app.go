// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var appCmd = &cobra.Command{
	Use:   "app <application-address>",
	Short: "Query application contract state (IApplication)",
	Args:  cobra.ExactArgs(1),
	RunE:  runApp,
}

func runApp(cmd *cobra.Command, args []string) error {
	cc, cancel, err := initChainClient(cmd, args)
	if err != nil {
		return err
	}
	defer cancel()
	defer cc.eth.Close()

	result, err := cc.queryApp()
	if err != nil {
		return err
	}

	if jsonParam {
		return outputJSON(result)
	}
	printApp(result, cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
	return nil
}

// queryApp reads all IApplication view functions and returns an AppResult.
func (c *chainClient) queryApp() (*AppResult, error) {
	if err := c.ensureContract(c.appAddr, "application"); err != nil {
		return nil, err
	}

	app, err := iapplication.NewIApplicationCaller(c.appAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IApplication: %w", err)
	}

	templateHash, err := app.GetTemplateHash(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetTemplateHash: %w", err)
	}

	owner, err := app.Owner(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("owner: %w", err)
	}

	deploymentBlockRaw, err := app.GetDeploymentBlockNumber(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetDeploymentBlockNumber: %w", err)
	}
	deploymentBlock, err := safeUint64(deploymentBlockRaw, "deployment block")
	if err != nil {
		return nil, err
	}

	executedOutputsRaw, err := app.GetNumberOfExecutedOutputs(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetNumberOfExecutedOutputs: %w", err)
	}
	executedOutputs, err := safeUint64(executedOutputsRaw, "executed outputs")
	if err != nil {
		return nil, err
	}

	consensusAddr, err := app.GetOutputsMerkleRootValidator(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetOutputsMerkleRootValidator: %w", err)
	}

	dataAvailability, err := app.GetDataAvailability(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetDataAvailability: %w", err)
	}

	// Detect consensus type for display.
	consensusLabel := consensusUnknown.String()
	if err := c.ensureContract(consensusAddr, "consensus"); err == nil {
		var detectErr error
		var cType consensusType
		var contractVersion string
		cType, contractVersion, detectErr = c.detectConsensus(consensusAddr)
		if detectErr != nil {
			slog.Warn("failed to detect consensus type", "error", detectErr)
		}
		consensusLabel = cType.String()
		if contractVersion != "" {
			consensusLabel += fmt.Sprintf(" (IConsensus %s)", contractVersion)
		}
	}

	return &AppResult{
		Address:          formatAddr(c.appAddr),
		Owner:            formatAddr(owner),
		TemplateHash:     formatHash(templateHash),
		DeploymentBlock:  deploymentBlock,
		ExecutedOutputs:  executedOutputs,
		ConsensusAddress: formatAddr(consensusAddr),
		ConsensusType:    consensusLabel,
		DataAvailability: "0x" + hex.EncodeToString(dataAvailability),
	}, nil
}

func printApp(r *AppResult, blockNum, chainID, blockTime uint64) {
	p := &printer{w: os.Stdout}
	p.withSection(fmt.Sprintf("Application  %s", r.Address), func() {
		p.field("Template Hash", r.TemplateHash)
		p.field("Owner", r.Owner)
		p.field("Deployment Block", fmt.Sprintf("%d", r.DeploymentBlock))
		p.field("Executed Outputs", fmt.Sprintf("%d", r.ExecutedOutputs))
		p.field("Consensus", fmt.Sprintf("%s (%s)", r.ConsensusAddress, r.ConsensusType))
		p.field("Data Availability", r.DataAvailability)
	})
	p.footer(blockNum, chainID, blockTime)
}

func outputJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// getConsensusAddress is a helper that reads the consensus address from the app contract.
func (c *chainClient) getConsensusAddress() (common.Address, error) {
	app, err := iapplication.NewIApplicationCaller(c.appAddr, c.eth)
	if err != nil {
		return common.Address{}, fmt.Errorf("bind IApplication: %w", err)
	}
	addr, err := app.GetOutputsMerkleRootValidator(c.callOpts)
	if err != nil {
		return common.Address{}, fmt.Errorf("GetOutputsMerkleRootValidator: %w", err)
	}
	return addr, nil
}
