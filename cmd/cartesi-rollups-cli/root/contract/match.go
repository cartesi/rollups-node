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
	Short: "Inspect a specific tournament match",
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

	bisecting, err := caller.BisectingMatch(cc.callOpts, [32]byte(matchIDHash))
	if err != nil {
		return fmt.Errorf("BisectingMatch: %w", err)
	}

	// Discover commitment hashes for this match from MatchCreated events.
	// The timeout view takes the full match ID. The event also identifies the players.
	commitOne, commitTwo, lookupErr := cc.findMatchCommitments(
		tournamentAddr, deployBlock, [32]byte(matchIDHash))
	if lookupErr != nil {
		return fmt.Errorf("look up MatchCreated commitments: %w", lookupErr)
	}

	timeout, err := caller.ClassifyMatchTimeout(cc.callOpts,
		itournament.MatchId{CommitmentOne: commitOne, CommitmentTwo: commitTwo})
	if err != nil {
		return fmt.Errorf("ClassifyMatchTimeout: %w", err)
	}
	if timeout.ActualPhase != bisecting.ActualPhase {
		return fmt.Errorf(
			"inconsistent match phase at pinned block %d: BisectingMatch=%s, ClassifyMatchTimeout=%s",
			cc.blockNum,
			matchPhaseName(bisecting.ActualPhase),
			matchPhaseName(timeout.ActualPhase),
		)
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
		MatchIDHash:    formatHash([32]byte(matchIDHash)),
		Tournament:     formatAddr(tournamentAddr),
		CommitmentOne:  formatHash(commitOne),
		CommitmentTwo:  formatHash(commitTwo),
		PlayerOneAddr:  registry.resolve(commitOne),
		PlayerTwoAddr:  registry.resolve(commitTwo),
		ActualPhase:    matchPhaseName(bisecting.ActualPhase),
		TimeoutOutcome: matchTimeoutOutcomeName(timeout.Outcome),
		DeferredCharge: timeout.DeferredCharge,
	}

	switch bisecting.ActualPhase {
	case matchPhaseUninitialized:
		// No phase payload exists.
	case matchPhaseBisecting:
		if err := populateBisectingMatchResult(result, bisecting.Value); err != nil {
			return err
		}
	case matchPhaseReadyToSeal:
		ready, rErr := caller.ReadyToSealMatch(cc.callOpts, [32]byte(matchIDHash))
		if rErr != nil {
			return fmt.Errorf("ReadyToSealMatch: %w", rErr)
		}
		if ready.ActualPhase != bisecting.ActualPhase {
			return inconsistentMatchPhaseError(cc.blockNum, "ReadyToSealMatch", ready.ActualPhase, bisecting.ActualPhase)
		}
		if err := populateReadyToSealMatchResult(result, ready.Value); err != nil {
			return err
		}
	case matchPhaseSealed:
		sealed, sErr := caller.SealedMatch(cc.callOpts, [32]byte(matchIDHash))
		if sErr != nil {
			return fmt.Errorf("SealedMatch: %w", sErr)
		}
		if sealed.ActualPhase != bisecting.ActualPhase {
			return inconsistentMatchPhaseError(cc.blockNum, "SealedMatch", sealed.ActualPhase, bisecting.ActualPhase)
		}
		if err := populateSealedMatchResult(result, sealed.Value); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown match phase %d", bisecting.ActualPhase)
	}

	if jsonParam {
		return outputJSON(result)
	}

	p := &printer{w: os.Stdout}
	printMatchResult(p, result)
	p.footer(cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
	return nil
}

func populateBisectingMatchResult(result *MatchResult, value itournament.ITournamentBisectingMatchView) error {
	if value.SegmentStartPosition == nil || value.SegmentStartCycle == nil {
		return errors.New("BisectingMatch returned nil position or cycle in BISECTING phase")
	}
	currentHeight := value.CurrentHeight
	result.CurrentHeight = &currentHeight
	result.SegmentStartPosition = value.SegmentStartPosition.String()
	result.SegmentStartCycle = value.SegmentStartCycle.String()
	result.RevealingParent = formatHash(value.RevealingParent)
	result.WaitingLeft = formatHash(value.WaitingLeft)
	result.WaitingRight = formatHash(value.WaitingRight)
	result.Responder = commitmentSideName(value.Responder)
	return nil
}

func populateReadyToSealMatchResult(result *MatchResult, value itournament.ITournamentReadyToSealMatchView) error {
	if value.SegmentStartPosition == nil || value.SegmentStartCycle == nil {
		return errors.New("ReadyToSealMatch returned nil position or cycle in READY_TO_SEAL phase")
	}
	result.SegmentStartPosition = value.SegmentStartPosition.String()
	result.SegmentStartCycle = value.SegmentStartCycle.String()
	result.RevealingParent = formatHash(value.RevealingParent)
	result.WaitingLeft = formatHash(value.WaitingLeft)
	result.WaitingRight = formatHash(value.WaitingRight)
	result.Responder = commitmentSideName(value.Responder)
	return nil
}

func populateSealedMatchResult(result *MatchResult, value itournament.ITournamentSealedMatchView) error {
	if value.DivergencePosition == nil || value.DivergenceCycle == nil {
		return errors.New("SealedMatch returned nil position or cycle in SEALED phase")
	}
	result.AgreeState = formatHash(value.AgreeState)
	result.DivergencePosition = value.DivergencePosition.String()
	result.DivergenceCycle = value.DivergenceCycle.String()
	result.FinalStateOne = formatHash(value.FinalStateOne)
	result.FinalStateTwo = formatHash(value.FinalStateTwo)
	return nil
}

func inconsistentMatchPhaseError(block uint64, view string, got, want uint8) error {
	return fmt.Errorf(
		"inconsistent match phase at pinned block %d: %s=%s, expected %s",
		block,
		view,
		matchPhaseName(got),
		matchPhaseName(want),
	)
}

func printMatchResult(p *printer, result *MatchResult) {
	p.withSection(fmt.Sprintf("Match  %s", result.MatchIDHash), func() {
		p.field("Tournament", result.Tournament)
		printMatchPlayer(p, "Player One", result.CommitmentOne, result.PlayerOneAddr)
		printMatchPlayer(p, "Player Two", result.CommitmentTwo, result.PlayerTwoAddr)
		p.field("Actual Phase", result.ActualPhase)
		p.field("Timeout Outcome", result.TimeoutOutcome)
		p.field("Deferred Charge", fmt.Sprintf("%d blocks", result.DeferredCharge))

		switch result.ActualPhase {
		case matchPhaseName(matchPhaseBisecting), matchPhaseName(matchPhaseReadyToSeal):
			if result.CurrentHeight != nil {
				p.field("Current Height", fmt.Sprintf("%d", *result.CurrentHeight))
			}
			p.field("Segment Start Position", result.SegmentStartPosition)
			p.field("Segment Start Cycle", result.SegmentStartCycle)
			p.field("Revealing Parent", result.RevealingParent)
			p.field("Waiting Left", result.WaitingLeft)
			p.field("Waiting Right", result.WaitingRight)
			p.field("Responder", result.Responder)
		case matchPhaseName(matchPhaseSealed):
			p.field("Agree State", result.AgreeState)
			p.field("Divergence Position", result.DivergencePosition)
			p.field("Divergence Cycle", result.DivergenceCycle)
			p.field("Final State One", result.FinalStateOne)
			p.field("Final State Two", result.FinalStateTwo)
		}
	})
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
	found := false

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
				found = true
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
	if !found {
		return [32]byte{}, [32]byte{}, fmt.Errorf(
			"MatchCreated event %s was not found between blocks %d and %d",
			formatHash(matchIDHash),
			deployBlock,
			c.blockNum,
		)
	}
	return commitOne, commitTwo, nil
}
