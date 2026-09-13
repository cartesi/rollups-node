// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var commitmentCmd = &cobra.Command{
	Use:   "commitment <application-address> <commitment-hash>",
	Short: "Read a commitment's on-chain state from the current tournament",
	Args:  cobra.ExactArgs(2), //nolint:mnd
	RunE:  runCommitment,
}

func runCommitment(cmd *cobra.Command, args []string) error {
	cc, cancel, err := initChainClient(cmd, args)
	if err != nil {
		return err
	}
	defer cancel()
	defer cc.eth.Close()

	if err := validateHash(args[1], "commitment hash"); err != nil {
		return err
	}
	commitmentHash := common.HexToHash(args[1])

	// Resolve tournament address using the same logic as the tournament subcommand.
	res, err := cc.resolveTournamentAddress(args[:1]) // only pass app addr
	if err != nil {
		return fmt.Errorf("resolve tournament: %w", err)
	}
	tournamentAddr := res.addr

	if err := cc.ensureContract(tournamentAddr, "tournament"); err != nil {
		return err
	}

	caller, err := itournament.NewITournamentCaller(tournamentAddr, cc.eth)
	if err != nil {
		return fmt.Errorf("bind ITournament: %w", err)
	}

	commitment, err := caller.CommitmentStanding(cc.callOpts, [32]byte(commitmentHash))
	if err != nil {
		return fmt.Errorf("CommitmentStanding: %w", err)
	}

	descriptor, err := caller.TournamentDescriptor(cc.callOpts)
	if err != nil {
		return fmt.Errorf("TournamentDescriptor: %w", err)
	}
	maxLevel, err := cc.tournamentMaxLevel()
	if err != nil {
		return err
	}
	if descriptor.Level > maxLevel {
		return fmt.Errorf("tournament level %d exceeds maximum level %d", descriptor.Level, maxLevel)
	}

	blocksRemaining := uint64(0)
	if commitment.ClockRunning && commitment.ClockDeadline > cc.blockNum {
		blocksRemaining = commitment.ClockDeadline - cc.blockNum
	}
	claimer := ""
	if commitment.Claimer != (common.Address{}) {
		claimer = formatAddr(commitment.Claimer)
	}

	result := &CommitmentResult{
		Commitment:       formatHash([32]byte(commitmentHash)),
		Tournament:       formatAddr(tournamentAddr),
		TournamentLevel:  fmt.Sprintf("%d/%d (%s)", descriptor.Level, maxLevel, tournamentLevelName(descriptor.Level, descriptor.Kind)),
		Joined:           commitment.Joined,
		Claimer:          claimer,
		ClockRunning:     commitment.ClockRunning,
		ClockAllowance:   commitment.ClockAllowance,
		ClockDeadline:    commitment.ClockDeadline,
		BlocksRemaining:  blocksRemaining,
		FinalMachineHash: formatHash(commitment.FinalState),
	}

	if jsonParam {
		return outputJSON(result)
	}

	p := &printer{w: os.Stdout}
	p.withSection(fmt.Sprintf("Commitment  %s", result.Commitment), func() {
		p.field("Tournament",
			fmt.Sprintf("%s (level %s)", result.Tournament, result.TournamentLevel))
		p.field("Joined", formatBool(result.Joined))
		if result.Claimer != "" {
			p.field("Claimer", result.Claimer)
		}
		p.field("Clock Running", formatBool(result.ClockRunning))
		p.field("Clock Allowance",
			fmt.Sprintf("%d blocks (raw allowance)", result.ClockAllowance))
		if result.ClockRunning {
			p.field("Clock Deadline", fmt.Sprintf("block %d (inclusive)", result.ClockDeadline))
			p.field("Blocks Remaining", fmt.Sprintf("%d", result.BlocksRemaining))
		} else {
			p.field("Clock Deadline", "paused")
		}
		p.field("Final Machine Hash", result.FinalMachineHash)
	})
	p.footer(cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
	return nil
}
