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

	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var (
	epochFromBlock int64
	epochToBlock   int64
	epochLimitFlag int
)

var epochCmd = &cobra.Command{
	Use:   "epoch <application-address>",
	Short: "Show epoch history (claims, sealed epochs, events)",
	Args:  cobra.ExactArgs(1),
	RunE:  runEpoch,
}

func init() {
	epochCmd.Flags().Int64Var(&epochFromBlock, "from-block", -1,
		"Start block for event scan (default: deployment block)")
	epochCmd.Flags().Int64Var(&epochToBlock, "to-block", -1,
		"End block for event scan (default: pinned block)")
	epochCmd.Flags().IntVar(&epochLimitFlag, "limit", 50, //nolint:mnd
		"Maximum number of events to display")
}

func runEpoch(cmd *cobra.Command, args []string) error {
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

	cType, _, err := cc.detectConsensus(consensusAddr)
	if err != nil {
		return err
	}

	var history *EpochHistory
	switch cType {
	case consensusDave:
		history, err = cc.epochHistoryDave(consensusAddr)
	case consensusAuthority:
		history, err = cc.epochHistoryAuthority(consensusAddr)
	case consensusQuorum:
		history, err = cc.epochHistoryQuorum(consensusAddr)
	case consensusUnknown:
		return fmt.Errorf("unknown consensus type at %s", consensusAddr)
	}
	if err != nil {
		return err
	}

	if jsonParam {
		return outputJSON(history)
	}

	cc.printEpochHistory(history)
	return nil
}

// epochHistoryDave uses FindTransitions on GetCurrentSealedEpoch().EpochNumber
// to discover EpochSealed events.
func (c *chainClient) epochHistoryDave(consensusAddr common.Address) (*EpochHistory, error) {
	daveCaller, err := idaveconsensus.NewIDaveConsensusCaller(consensusAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IDaveConsensus: %w", err)
	}
	daveFilterer, err := idaveconsensus.NewIDaveConsensusFilterer(consensusAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IDaveConsensus filterer: %w", err)
	}

	deployBlockRaw, err := daveCaller.GetDeploymentBlockNumber(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetDeploymentBlockNumber: %w", err)
	}
	deployBlock, err := safeUint64(deployBlockRaw, "deployment block")
	if err != nil {
		return nil, err
	}

	// Get template hash for the machine hash chain.
	appCaller, err := iapplication.NewIApplicationCaller(c.appAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IApplication: %w", err)
	}
	templateHash, err := appCaller.GetTemplateHash(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetTemplateHash: %w", err)
	}

	fromBlock, toBlock, err := c.resolveBlockRange(deployBlock)
	if err != nil {
		return nil, err
	}

	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		opts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
		sealed, sErr := daveCaller.GetCurrentSealedEpoch(opts)
		if sErr != nil {
			return nil, sErr
		}
		return sealed.EpochNumber, nil
	}

	var epochs []EpochEvent
	limit := epochLimitFlag

	onHit := func(block uint64) error {
		if limit > 0 && len(epochs) >= limit {
			return errLimitReached
		}
		q, qErr := buildEventFilterQuery(
			consensusAddr, "EpochSealed",
			idaveconsensus.IDaveConsensusMetaData, block, block,
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
			ev, pErr := daveFilterer.ParseEpochSealed(*log)
			if pErr != nil {
				return pErr
			}
			epochNum, sErr := safeUint64(ev.EpochNumber, "epoch number")
			if sErr != nil {
				return sErr
			}
			inputLower, sErr := safeUint64(ev.InputIndexLowerBound, "input lower bound")
			if sErr != nil {
				return sErr
			}
			inputUpper, sErr := safeUint64(ev.InputIndexUpperBound, "input upper bound")
			if sErr != nil {
				return sErr
			}

			// Check if outputs root is valid on-chain.
			var rootValid *bool
			if rv, rvErr := daveCaller.IsOutputsMerkleRootValid(
				c.callOpts, c.appAddr, ev.OutputsMerkleRoot); rvErr == nil {
				rootValid = &rv
			} else {
				slog.Debug("IsOutputsMerkleRootValid failed", "error", rvErr)
			}

			epochs = append(epochs, EpochEvent{
				EpochNumber:        epochNum,
				BlockNumber:        log.BlockNumber,
				TxHash:             log.TxHash.Hex(),
				InputLowerBound:    inputLower,
				InputUpperBound:    inputUpper,
				InitialMachineHash: formatHash(ev.InitialMachineStateHash),
				OutputsMerkleRoot:  formatHash(ev.OutputsMerkleRoot),
				OutputsRootValid:   rootValid,
				Tournament:         formatAddr(ev.Tournament),
			})
		}
		return nil
	}

	// Use -1 as prevValue so the first sealed epoch (epoch 0) is detected as a
	// transition from -1→0. Without this, epoch 0 would be invisible (0→0 = no change).
	prevValue := big.NewInt(-1)
	if fromBlock > deployBlock && fromBlock > 0 {
		val, pErr := oracle(c.callOpts.Context, fromBlock-1)
		if pErr != nil {
			return nil, fmt.Errorf("get prevValue at block %d: %w", fromBlock-1, pErr)
		}
		prevValue = val
	}

	_, err = ethutil.FindTransitions(
		c.callOpts.Context, fromBlock, toBlock, prevValue, oracle, onHit,
	)
	if err != nil && !errors.Is(err, errLimitReached) {
		return nil, fmt.Errorf("find sealed epoch transitions: %w", err)
	}

	return &EpochHistory{
		ConsensusType: "DaveConsensus",
		TemplateHash:  formatHash(templateHash),
		Epochs:        epochs,
	}, nil
}

// epochHistoryAuthority uses FindTransitions on GetNumberOfAcceptedClaims
// to discover ClaimSubmitted and ClaimAccepted events.
func (c *chainClient) epochHistoryAuthority(
	consensusAddr common.Address,
) (*EpochHistory, error) {
	consensusCaller, err := iconsensus.NewIConsensusCaller(consensusAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IConsensus: %w", err)
	}
	consensusFilterer, err := iconsensus.NewIConsensusFilterer(consensusAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IConsensus filterer: %w", err)
	}

	appCaller, err := iapplication.NewIApplicationCaller(c.appAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IApplication: %w", err)
	}
	deployBlockRaw, err := appCaller.GetDeploymentBlockNumber(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetDeploymentBlockNumber: %w", err)
	}
	deployBlock, err := safeUint64(deployBlockRaw, "deployment block")
	if err != nil {
		return nil, err
	}

	fromBlock, toBlock, err := c.resolveBlockRange(deployBlock)
	if err != nil {
		return nil, err
	}

	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		opts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
		return consensusCaller.GetNumberOfAcceptedClaims(opts)
	}

	var claims []ClaimEvent
	limit := epochLimitFlag

	onHit := func(block uint64) error {
		if limit > 0 && len(claims) >= limit {
			return errLimitReached
		}
		// Filter by app address as indexed topic to avoid fetching events
		// for other applications on shared consensus contracts.
		appTopic := common.BytesToHash(c.appAddr.Bytes())

		// Retrieve ClaimSubmitted events at this block.
		// ClaimSubmitted has indexed topics (submitter, appContract).
		// Use nil wildcard for topic1 (submitter) and appTopic for topic2 (appContract).
		subQ, qErr := buildEventFilterQuery(
			consensusAddr, "ClaimSubmitted",
			iconsensus.IConsensusMetaData, block, block, nil, &appTopic,
		)
		if qErr != nil {
			return qErr
		}
		subItr, fErr := c.filter.ChunkedFilterLogs(c.callOpts.Context, c.eth, subQ)
		if fErr != nil {
			return fErr
		}
		for log, logErr := range subItr {
			if logErr != nil {
				return logErr
			}
			ev, pErr := consensusFilterer.ParseClaimSubmitted(*log)
			if pErr != nil {
				return pErr
			}
			lastBlock, sErr := safeUint64(ev.LastProcessedBlockNumber, "last processed block")
			if sErr != nil {
				return sErr
			}
			claims = append(claims, ClaimEvent{
				EventType:          "ClaimSubmitted",
				BlockNumber:        log.BlockNumber,
				TxHash:             log.TxHash.Hex(),
				Submitter:          formatAddr(ev.Submitter),
				LastProcessedBlock: lastBlock,
				OutputsMerkleRoot:  formatHash(ev.OutputsMerkleRoot),
			})
		}

		// Retrieve ClaimAccepted events at this block.
		accQ, qErr := buildEventFilterQuery(
			consensusAddr, "ClaimAccepted",
			iconsensus.IConsensusMetaData, block, block, &appTopic,
		)
		if qErr != nil {
			return qErr
		}
		accItr, fErr := c.filter.ChunkedFilterLogs(c.callOpts.Context, c.eth, accQ)
		if fErr != nil {
			return fErr
		}
		for log, logErr := range accItr {
			if logErr != nil {
				return logErr
			}
			ev, pErr := consensusFilterer.ParseClaimAccepted(*log)
			if pErr != nil {
				return pErr
			}
			lastBlock, sErr := safeUint64(ev.LastProcessedBlockNumber, "last processed block")
			if sErr != nil {
				return sErr
			}

			var rootValid *bool
			if rv, rvErr := consensusCaller.IsOutputsMerkleRootValid(
				c.callOpts, c.appAddr, ev.OutputsMerkleRoot); rvErr == nil {
				rootValid = &rv
			} else {
				slog.Debug("IsOutputsMerkleRootValid failed", "error", rvErr)
			}

			ce := ClaimEvent{
				EventType:          "ClaimAccepted",
				BlockNumber:        log.BlockNumber,
				TxHash:             log.TxHash.Hex(),
				LastProcessedBlock: lastBlock,
				OutputsMerkleRoot:  formatHash(ev.OutputsMerkleRoot),
				OutputsRootValid:   rootValid,
			}
			claims = append(claims, ce)
		}
		return nil
	}

	prevValue := big.NewInt(0)
	if fromBlock > deployBlock && fromBlock > 0 {
		val, pErr := oracle(c.callOpts.Context, fromBlock-1)
		if pErr != nil {
			return nil, fmt.Errorf("get prevValue at block %d: %w", fromBlock-1, pErr)
		}
		prevValue = val
	}

	_, err = ethutil.FindTransitions(
		c.callOpts.Context, fromBlock, toBlock, prevValue, oracle, onHit,
	)
	if err != nil && !errors.Is(err, errLimitReached) {
		return nil, fmt.Errorf("find claim transitions: %w", err)
	}

	return &EpochHistory{
		ConsensusType: "Authority",
		Claims:        claims,
	}, nil
}

// epochHistoryQuorum uses two-pass: FindTransitions for ClaimAccepted,
// then ChunkedFilterLogs for ClaimSubmitted over the full range.
func (c *chainClient) epochHistoryQuorum(
	consensusAddr common.Address,
) (*EpochHistory, error) {
	consensusCaller, err := iconsensus.NewIConsensusCaller(consensusAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IConsensus: %w", err)
	}
	consensusFilterer, err := iconsensus.NewIConsensusFilterer(consensusAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IConsensus filterer: %w", err)
	}
	quorumCaller, err := iquorum.NewIQuorumCaller(consensusAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IQuorum: %w", err)
	}

	appCaller, err := iapplication.NewIApplicationCaller(c.appAddr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind IApplication: %w", err)
	}
	deployBlockRaw, err := appCaller.GetDeploymentBlockNumber(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetDeploymentBlockNumber: %w", err)
	}
	deployBlock, err := safeUint64(deployBlockRaw, "deployment block")
	if err != nil {
		return nil, err
	}

	numValRaw, err := quorumCaller.NumOfValidators(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("NumOfValidators: %w", err)
	}
	numVal, err := safeUint64(numValRaw, "num validators")
	if err != nil {
		return nil, err
	}
	threshold := 1 + numVal/2 //nolint:mnd

	fromBlock, toBlock, err := c.resolveBlockRange(deployBlock)
	if err != nil {
		return nil, err
	}

	var claims []ClaimEvent
	limit := epochLimitFlag

	// Pass 1: FindTransitions for ClaimAccepted.
	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		opts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
		return consensusCaller.GetNumberOfAcceptedClaims(opts)
	}

	onHit := func(block uint64) error {
		if limit > 0 && len(claims) >= limit {
			return errLimitReached
		}
		// Filter by app address as indexed topic to avoid fetching events
		// for other applications on shared Quorum contracts.
		appTopic := common.BytesToHash(c.appAddr.Bytes())
		accQ, qErr := buildEventFilterQuery(
			consensusAddr, "ClaimAccepted",
			iconsensus.IConsensusMetaData, block, block, &appTopic,
		)
		if qErr != nil {
			return qErr
		}
		accItr, fErr := c.filter.ChunkedFilterLogs(c.callOpts.Context, c.eth, accQ)
		if fErr != nil {
			return fErr
		}
		for log, logErr := range accItr {
			if logErr != nil {
				return logErr
			}
			ev, pErr := consensusFilterer.ParseClaimAccepted(*log)
			if pErr != nil {
				return pErr
			}
			if ev.AppContract != c.appAddr {
				continue
			}
			lastBlock, sErr := safeUint64(ev.LastProcessedBlockNumber, "last processed block")
			if sErr != nil {
				return sErr
			}
			var rootValid *bool
			if rv, rvErr := consensusCaller.IsOutputsMerkleRootValid(
				c.callOpts, c.appAddr, ev.OutputsMerkleRoot); rvErr == nil {
				rootValid = &rv
			} else {
				slog.Debug("IsOutputsMerkleRootValid failed", "error", rvErr)
			}
			claims = append(claims, ClaimEvent{
				EventType:          "ClaimAccepted",
				BlockNumber:        log.BlockNumber,
				TxHash:             log.TxHash.Hex(),
				LastProcessedBlock: lastBlock,
				OutputsMerkleRoot:  formatHash(ev.OutputsMerkleRoot),
				OutputsRootValid:   rootValid,
			})
		}
		return nil
	}

	prevValue := big.NewInt(0)
	if fromBlock > deployBlock && fromBlock > 0 {
		val, pErr := oracle(c.callOpts.Context, fromBlock-1)
		if pErr != nil {
			return nil, fmt.Errorf("get prevValue at block %d: %w", fromBlock-1, pErr)
		}
		prevValue = val
	}

	_, err = ethutil.FindTransitions(
		c.callOpts.Context, fromBlock, toBlock, prevValue, oracle, onHit,
	)
	if err != nil && !errors.Is(err, errLimitReached) {
		return nil, fmt.Errorf("find claim accepted transitions: %w", err)
	}

	// Pass 2: ChunkedFilterLogs for ClaimSubmitted over full range.
	// ClaimSubmitted has indexed topics (submitter, appContract).
	// Use nil wildcard for topic1 (submitter) and appTopic for topic2 (appContract).
	appTopic := common.BytesToHash(c.appAddr.Bytes())
	subQ, err := buildEventFilterQuery(
		consensusAddr, "ClaimSubmitted",
		iconsensus.IConsensusMetaData, fromBlock, toBlock, nil, &appTopic,
	)
	if err != nil {
		return nil, fmt.Errorf("build ClaimSubmitted query: %w", err)
	}
	subItr, err := c.filter.ChunkedFilterLogs(c.callOpts.Context, c.eth, subQ)
	if err != nil {
		return nil, fmt.Errorf("filter ClaimSubmitted: %w", err)
	}

	var submissions []ClaimEvent
	for log, logErr := range subItr {
		if logErr != nil {
			return nil, fmt.Errorf("iterate ClaimSubmitted: %w", logErr)
		}
		if limit > 0 && len(submissions) >= limit {
			break
		}
		ev, pErr := consensusFilterer.ParseClaimSubmitted(*log)
		if pErr != nil {
			return nil, fmt.Errorf("parse ClaimSubmitted: %w", pErr)
		}
		lastBlock, sErr := safeUint64(ev.LastProcessedBlockNumber, "last processed block")
		if sErr != nil {
			return nil, sErr
		}

		// Get vote count for this specific claim.
		votesRaw, vErr := quorumCaller.NumOfValidatorsInFavorOf(
			c.callOpts, c.appAddr, ev.LastProcessedBlockNumber, ev.OutputsMerkleRoot)
		var votes *uint64
		if vErr == nil {
			v, sErr := safeUint64(votesRaw, "votes")
			if sErr == nil {
				votes = &v
			}
		}

		ce := ClaimEvent{
			EventType:          "ClaimSubmitted",
			BlockNumber:        log.BlockNumber,
			TxHash:             log.TxHash.Hex(),
			Submitter:          formatAddr(ev.Submitter),
			LastProcessedBlock: lastBlock,
			OutputsMerkleRoot:  formatHash(ev.OutputsMerkleRoot),
			VotesForClaim:      votes,
			VotesNeeded:        &threshold,
		}
		submissions = append(submissions, ce)
	}

	// Merge submissions and acceptances, sorted by block number.
	allClaims := mergeClaimEvents(submissions, claims)

	return &EpochHistory{
		ConsensusType: "Quorum",
		Claims:        allClaims,
	}, nil
}

// mergeClaimEvents merges two sorted-by-block slices of ClaimEvents.
func mergeClaimEvents(a, b []ClaimEvent) []ClaimEvent {
	result := make([]ClaimEvent, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i].BlockNumber <= b[j].BlockNumber {
			result = append(result, a[i])
			i++
		} else {
			result = append(result, b[j])
			j++
		}
	}
	result = append(result, a[i:]...)
	result = append(result, b[j:]...)
	return result
}

// resolveBlockRange computes the from/to block range for event scanning.
// Returns an error if --from-block or --to-block exceeds the pinned block, or from > to.
func (c *chainClient) resolveBlockRange(deployBlock uint64) (uint64, uint64, error) {
	from := deployBlock
	if epochFromBlock >= 0 {
		from = uint64(epochFromBlock)
	}
	to := c.blockNum
	if epochToBlock >= 0 {
		to = uint64(epochToBlock)
	}
	if from > c.blockNum {
		return 0, 0, fmt.Errorf(
			"--from-block %d exceeds pinned block %d", from, c.blockNum)
	}
	if to > c.blockNum {
		return 0, 0, fmt.Errorf(
			"--to-block %d exceeds pinned block %d", to, c.blockNum)
	}
	if from > to {
		return 0, 0, fmt.Errorf(
			"--from-block %d exceeds --to-block %d", from, to)
	}
	return from, to, nil
}

// printEpochHistory renders the epoch history as text.
func (c *chainClient) printEpochHistory(h *EpochHistory) {
	p := &printer{w: os.Stdout}

	switch h.ConsensusType {
	case "DaveConsensus":
		c.printDaveEpochHistory(p, h)
	case "Authority":
		c.printClaimHistory(p, h, "Authority")
	case "Quorum":
		c.printClaimHistory(p, h, "Quorum")
	}

	p.footer(c.blockNum, c.chainID, c.resolveTimestamp(c.blockNum))
}

func (c *chainClient) printDaveEpochHistory(p *printer, h *EpochHistory) {
	p.withSection(fmt.Sprintf("Epoch History  (DaveConsensus, %d epochs)", len(h.Epochs)), func() {
		if h.TemplateHash != "" {
			p.field("Template Hash", h.TemplateHash+" (epoch 0 initial state)")
		}
	})

	for _, ep := range h.Epochs {
		ts := formatBlockTime(c.resolveTimestamp(ep.BlockNumber))
		p.withSection(
			fmt.Sprintf("Epoch %d  (sealed at block %d%s, tx %s)",
				ep.EpochNumber, ep.BlockNumber, ts, ep.TxHash), func() {
				p.field("Input Range",
					fmt.Sprintf("[%d, %d)", ep.InputLowerBound, ep.InputUpperBound))
				p.field("Initial Machine Hash", ep.InitialMachineHash)
				rootStatus := "unknown"
				if ep.OutputsRootValid != nil {
					rootStatus = "no"
					if *ep.OutputsRootValid {
						rootStatus = "yes"
					}
				}
				p.field("Outputs Merkle Root",
					fmt.Sprintf("%s (valid: %s)", ep.OutputsMerkleRoot, rootStatus))
				p.field("Tournament", ep.Tournament)
			})
	}
}

func (c *chainClient) printClaimHistory(p *printer, h *EpochHistory, cType string) {
	p.withSection(fmt.Sprintf("Epoch History  (%s, %d events)", cType, len(h.Claims)), func() {})

	for _, cl := range h.Claims {
		label := cl.EventType
		ts := formatBlockTime(c.resolveTimestamp(cl.BlockNumber))
		p.withSection(
			fmt.Sprintf("%s at block %d%s  tx %s", label, cl.BlockNumber, ts, cl.TxHash), func() {
				if cl.Submitter != "" {
					p.field("Submitter", cl.Submitter)
				}
				p.field("Last Processed Block", fmt.Sprintf("%d", cl.LastProcessedBlock))
				p.field("Outputs Merkle Root", cl.OutputsMerkleRoot)
				if cl.OutputsRootValid != nil {
					p.field("Root Valid on-chain", fmt.Sprintf("%t", *cl.OutputsRootValid))
				}
				if cl.VotesForClaim != nil && cl.VotesNeeded != nil {
					p.field("Votes",
						fmt.Sprintf("%d / %d needed", *cl.VotesForClaim, *cl.VotesNeeded))
				}
			})
	}
}
