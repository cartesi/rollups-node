// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"

	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var matchCmd = &cobra.Command{
	Use:   "match <application-address> <match-id-hash>",
	Short: "Inspect a specific match's bisection state",
	Args:  cobra.ExactArgs(2), //nolint:mnd
	RunE:  runMatch,
}

func runMatch(cmd *cobra.Command, args []string) error {
	cc, cancel, err := initChainClient(cmd, args)
	if err != nil {
		return err
	}
	defer cancel()
	defer cc.eth.Close()

	if err := validateHash(args[1], "match ID hash"); err != nil {
		return err
	}
	matchIDHash := common.HexToHash(args[1])

	// Resolve tournament address using the same logic as the tournament subcommand.
	res, err := cc.resolveTournamentAddress(args[:1])
	if err != nil {
		return fmt.Errorf("resolve tournament: %w", err)
	}
	tournamentAddr, deployBlock := res.addr, res.deployBlock

	if err := cc.ensureContract(tournamentAddr, "tournament"); err != nil {
		return err
	}

	caller, err := itournament.NewITournamentCaller(tournamentAddr, cc.eth)
	if err != nil {
		return fmt.Errorf("bind ITournament: %w", err)
	}

	matchState, err := caller.GetMatch(cc.callOpts, [32]byte(matchIDHash))
	if err != nil {
		return fmt.Errorf("GetMatch: %w", err)
	}

	cycle, err := caller.GetMatchCycle(cc.callOpts, [32]byte(matchIDHash))
	if err != nil {
		return fmt.Errorf("GetMatchCycle: %w", err)
	}

	// Discover commitment hashes for this match from MatchCreated events.
	// We need the two commitment hashes to call CanWinMatchByTimeout and to show players.
	commitOne, commitTwo, lookupErr := cc.findMatchCommitments(
		tournamentAddr, deployBlock, [32]byte(matchIDHash))
	if lookupErr != nil {
		slog.Warn("failed to look up match commitments", "error", lookupErr)
	}

	// Attempt CanWinMatchByTimeout if we have both commitments.
	var canWinByTimeout bool
	if commitOne != ([32]byte{}) && commitTwo != ([32]byte{}) {
		canWinByTimeout, _ = caller.CanWinMatchByTimeout(cc.callOpts,
			itournament.MatchId{CommitmentOne: commitOne, CommitmentTwo: commitTwo})
	}

	// Build commitment registry for player address resolution.
	registry := make(commitmentRegistry)
	if commitOne != ([32]byte{}) || commitTwo != ([32]byte{}) {
		events, eErr := cc.fetchTournamentEvents(tournamentAddr, deployBlock)
		if eErr != nil {
			slog.Warn("failed to fetch events for address resolution", "error", eErr)
		} else {
			for _, cj := range events.commitmentsJoined {
				registry[cj.commitment] = cj.submitter
			}
		}
	}

	result := &MatchResult{
		MatchIDHash:         formatHash([32]byte(matchIDHash)),
		Tournament:          formatAddr(tournamentAddr),
		CommitmentOne:       formatHash(commitOne),
		CommitmentTwo:       formatHash(commitTwo),
		PlayerOneAddr:       registry.resolve(commitOne),
		PlayerTwoAddr:       registry.resolve(commitTwo),
		CurrentHeight:       matchState.CurrentHeight,
		RunningLeafPosition: matchState.RunningLeafPosition.String(),
		MachineCycle:        cycle.String(),
		CanWinByTimeout:     canWinByTimeout,
		LeftNode:            formatHash(matchState.LeftNode),
		RightNode:           formatHash(matchState.RightNode),
		OtherParent:         formatHash(matchState.OtherParent),
	}

	if jsonParam {
		return outputJSON(result)
	}

	p := &printer{w: os.Stdout}
	p.withSection(fmt.Sprintf("Match  %s", result.MatchIDHash), func() {
		p.field("Tournament", result.Tournament)
		printMatchPlayer(p, "Player One", result.CommitmentOne, result.PlayerOneAddr)
		printMatchPlayer(p, "Player Two", result.CommitmentTwo, result.PlayerTwoAddr)
		p.field("Current Height", fmt.Sprintf("%d", result.CurrentHeight))
		p.field("Running Leaf Pos", result.RunningLeafPosition)
		p.field("Machine Cycle", result.MachineCycle)
		p.field("Can Win by Timeout", fmt.Sprintf("%t", result.CanWinByTimeout))
		p.field("Left Node", result.LeftNode)
		p.field("Right Node", result.RightNode)
		p.field("Other Parent", result.OtherParent)
	})
	p.footer(cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
	return nil
}

func printMatchPlayer(p *printer, label, commitment, addr string) {
	info := commitment
	if addr != "" {
		info += fmt.Sprintf("  (%s)", addr)
	}
	p.field(label, info)
}

// findMatchCommitments looks up the MatchCreated event for the given match ID hash
// to retrieve the two commitment hashes. Returns zero hashes and an error on failure.
func (c *chainClient) findMatchCommitments(
	tournamentAddr common.Address,
	deployBlock uint64,
	matchIDHash [32]byte,
) ([32]byte, [32]byte, error) {
	filterer, err := itournament.NewITournamentFilterer(tournamentAddr, c.eth)
	if err != nil {
		return [32]byte{}, [32]byte{}, fmt.Errorf("bind ITournament filterer: %w", err)
	}
	caller, err := itournament.NewITournamentCaller(tournamentAddr, c.eth)
	if err != nil {
		return [32]byte{}, [32]byte{}, fmt.Errorf("bind ITournament caller: %w", err)
	}

	var commitOne, commitTwo [32]byte

	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		opts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
		return caller.GetMatchCreatedCount(opts)
	}

	onHit := func(block uint64) error {
		q, qErr := buildEventFilterQuery(
			tournamentAddr, "MatchCreated",
			itournament.ITournamentMetaData, block, block,
		)
		if qErr != nil {
			return qErr
		}
		itr, fErr := c.filter.ChunkedFilterLogs(c.callOpts.Context, c.eth, q)
		if fErr != nil {
			return fErr
		}
		for log, logErr := range itr {
			if logErr != nil {
				return logErr
			}
			ev, pErr := filterer.ParseMatchCreated(*log)
			if pErr != nil {
				return pErr
			}
			if ev.MatchIdHash == matchIDHash {
				commitOne = ev.One
				commitTwo = ev.Two
				return errFound
			}
		}
		return nil
	}

	_, err = ethutil.FindTransitions(
		c.callOpts.Context, deployBlock, c.blockNum, big.NewInt(0), oracle, onHit,
	)
	if err != nil && !errors.Is(err, errFound) {
		return [32]byte{}, [32]byte{}, fmt.Errorf("find match commitments: %w", err)
	}
	return commitOne, commitTwo, nil
}
