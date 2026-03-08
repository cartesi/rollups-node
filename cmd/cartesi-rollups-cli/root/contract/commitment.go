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

	commitment, err := caller.GetCommitment(cc.callOpts, [32]byte(commitmentHash))
	if err != nil {
		return fmt.Errorf("GetCommitment: %w", err)
	}

	levelConsts, err := caller.TournamentLevelConstants(cc.callOpts)
	if err != nil {
		return fmt.Errorf("TournamentLevelConstants: %w", err)
	}

	levelName := "root"
	if levelConsts.Level == levelConsts.MaxLevel {
		levelName = "leaf"
	} else if levelConsts.Level > 0 {
		levelName = "inner"
	}

	result := &CommitmentResult{
		Commitment:       formatHash([32]byte(commitmentHash)),
		Tournament:       formatAddr(tournamentAddr),
		TournamentLevel:  fmt.Sprintf("%d/%d (%s)", levelConsts.Level, levelConsts.MaxLevel, levelName),
		ClockAllowance:   commitment.Clock.Allowance,
		ClockStartBlock:  commitment.Clock.StartInstant,
		FinalMachineHash: formatHash(commitment.FinalState),
	}

	if jsonParam {
		return outputJSON(result)
	}

	p := &printer{w: os.Stdout}
	p.withSection(fmt.Sprintf("Commitment  %s", result.Commitment), func() {
		p.field("Tournament",
			fmt.Sprintf("%s (level %s)", result.Tournament, result.TournamentLevel))
		p.field("Clock Allowance",
			fmt.Sprintf("%d blocks remaining", result.ClockAllowance))
		if result.ClockStartBlock > 0 {
			p.field("Clock Start", fmt.Sprintf("block %d", result.ClockStartBlock))
		} else {
			p.field("Clock Start", "not started")
		}
		p.field("Final Machine Hash", result.FinalMachineHash)
	})
	p.footer(cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
	return nil
}
