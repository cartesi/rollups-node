// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var consensusCmd = &cobra.Command{
	Use:   "consensus <application-address>",
	Short: "Query consensus contract state (auto-detects Authority/Quorum/DaveConsensus)",
	Args:  cobra.ExactArgs(1),
	RunE:  runConsensus,
}

func runConsensus(cmd *cobra.Command, args []string) error {
	cc, cancel, err := initChainClient(cmd, args)
	if err != nil {
		return err
	}
	defer cancel()
	defer cc.eth.Close()

	consensusAddr, err := cc.getConsensusAddress()
	if err != nil {
		return err
	}

	if err := cc.ensureContract(consensusAddr, "consensus"); err != nil {
		return err
	}

	cType, contractVersion, err := cc.detectConsensus(consensusAddr)
	if err != nil {
		return err
	}

	switch cType {
	case consensusAuthority:
		return cc.printAuthority(consensusAddr, contractVersion)
	case consensusQuorum:
		return cc.printQuorum(consensusAddr, contractVersion)
	case consensusDave:
		return cc.printDave(consensusAddr)
	case consensusUnknown:
		return fmt.Errorf("unknown consensus type at %s", consensusAddr)
	}
	return fmt.Errorf("unknown consensus type at %s", consensusAddr)
}

func (c *chainClient) printAuthority(addr common.Address, contractVersion string) error {
	result, err := c.queryAuthority(addr, contractVersion)
	if err != nil {
		return err
	}

	if jsonParam {
		return outputJSON(result)
	}

	p := &printer{w: os.Stdout}
	p.withSection(fmt.Sprintf("Authority  %s", result.Address), func() {
		p.field("Owner (Validator)", result.Owner)
		p.field("Epoch Length", fmt.Sprintf("%d blocks", result.EpochLength))
		p.field("Claim Staging Period", fmt.Sprintf("%d blocks", result.ClaimStagingPeriod))
		p.field("Accepted Claims", fmt.Sprintf("%d", result.AcceptedClaims))
		p.field("Staged Claims", fmt.Sprintf("%d", result.StagedClaims))
		p.field("IConsensus Version", result.ContractVersion)
	})
	p.footer(c.blockNum, c.chainID, c.resolveTimestamp(c.blockNum))
	return nil
}

func (c *chainClient) printQuorum(addr common.Address, contractVersion string) error {
	result, err := c.queryQuorum(addr, contractVersion)
	if err != nil {
		return err
	}

	if jsonParam {
		return outputJSON(result)
	}

	p := &printer{w: os.Stdout}
	p.withSection(fmt.Sprintf("Quorum  %s", result.Address), func() {
		p.field("Validators", fmt.Sprintf("%d", result.NumValidators))
		p.field("Quorum Threshold",
			fmt.Sprintf("%d (computed: strict majority)", result.QuorumThreshold))
		p.field("Epoch Length", fmt.Sprintf("%d blocks", result.EpochLength))
		p.field("Claim Staging Period", fmt.Sprintf("%d blocks", result.ClaimStagingPeriod))
		p.field("Accepted Claims", fmt.Sprintf("%d", result.AcceptedClaims))
		p.field("Staged Claims", fmt.Sprintf("%d", result.StagedClaims))
		if result.ContractVersion != "" {
			p.field("IConsensus Version", result.ContractVersion)
		}
		for i, v := range result.Validators {
			p.field(fmt.Sprintf("  Validator #%d", i+1), v)
		}
	})
	p.footer(c.blockNum, c.chainID, c.resolveTimestamp(c.blockNum))
	return nil
}

func (c *chainClient) printDave(addr common.Address) error {
	result, err := c.queryDave(addr)
	if err != nil {
		return err
	}

	if jsonParam {
		return outputJSON(result)
	}

	p := &printer{w: os.Stdout}
	p.withSection(fmt.Sprintf("DaveConsensus  %s", result.Address), func() {
		p.field("Deployment Block", fmt.Sprintf("%d", result.DeploymentBlock))
		p.field("InputBox", result.InputBox)
		p.field("Tournament Factory", result.Factory)
		printTournamentFinished(p, result)
		p.field("Current Sealed Epoch", fmt.Sprintf("%d", result.CurrentEpochNumber))
		p.field("Input Range",
			fmt.Sprintf("[%d, %d)", result.InputLowerBound, result.InputUpperBound))
		p.field("Root Tournament", result.RootTournament)
	})
	p.footer(c.blockNum, c.chainID, c.resolveTimestamp(c.blockNum))
	return nil
}

// printTournamentFinished renders the IsFinished field, distinguishing winner from no-winner.
// Note: IsFinished means the tournament has concluded, NOT that settle() can be called
// successfully — settle() will revert if there is no winner.
func printTournamentFinished(p *printer, r *DaveConsensusResult) {
	if !r.IsFinished {
		p.field("Tournament Finished", "no")
		return
	}
	if r.HasWinner != nil && !*r.HasWinner {
		p.field("Tournament Finished", "yes (NO WINNER — all commitments eliminated)")
		return
	}
	if r.WinnerCommitment != "" {
		p.field("Tournament Finished",
			fmt.Sprintf("yes (winner: %s)", r.WinnerCommitment))
		return
	}
	p.field("Tournament Finished", "yes")
}
