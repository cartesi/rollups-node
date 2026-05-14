// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"
)

var (
	tournamentTreeFlag     bool
	tournamentEventsFlag   bool
	tournamentEpochFlag    int64
	tournamentLimitFlag    int
	tournamentMaxNodesFlag int
)

var tournamentCmd = &cobra.Command{
	Use:   "tournament <application-address> [tournament-address]",
	Short: "Query tournament contract state (PRT dispute resolution)",
	Args:  cobra.RangeArgs(1, 2), //nolint:mnd
	RunE:  runTournament,
}

func init() {
	tournamentCmd.Flags().BoolVar(&tournamentTreeFlag, "tree", false,
		"Recursively traverse the full tournament hierarchy.\n"+
			"WARNING: generates many RPC calls (8+ view calls + event discovery\n"+
			"per node). On rate-limited providers, use --max-nodes and --limit\n"+
			"to control RPC volume")
	tournamentCmd.Flags().BoolVar(&tournamentEventsFlag, "events", false,
		"Show detailed event lists (commitments, matches, advances)")
	tournamentCmd.Flags().Int64Var(&tournamentEpochFlag, "epoch", -1,
		"Inspect tournament for a specific epoch number (default: current sealed epoch)")
	tournamentCmd.Flags().IntVar(&tournamentLimitFlag, "limit", 50, //nolint:mnd
		"Maximum events per type per tournament")
	tournamentCmd.Flags().IntVar(&tournamentMaxNodesFlag, "max-nodes", 100, //nolint:mnd
		"Maximum tournament nodes to visit in --tree traversal")
}

func runTournament(cmd *cobra.Command, args []string) error {
	cc, cancel, err := initChainClient(cmd, args)
	if err != nil {
		return err
	}
	defer cancel()
	defer cc.eth.Close()

	res, err := cc.resolveTournamentAddress(args)
	if err != nil {
		return err
	}

	if err := cc.ensureContract(res.addr, "tournament"); err != nil {
		return err
	}

	if tournamentTreeFlag {
		// --tree always includes events for commitment registry.
		tree, tErr := cc.walkTournamentTree(res.addr, res.deployBlock, true)
		if tErr != nil {
			return tErr
		}
		if jsonParam {
			return outputJSON(tree.root)
		}
		p := &printer{w: os.Stdout}
		printTournamentContext(p, cc, res)
		renderTournamentTree(p, tree.root, tree, 0)
		p.footer(cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
		return nil
	}

	result, err := cc.queryTournament(res.addr)
	if err != nil {
		return err
	}

	if tournamentEventsFlag {
		events, eErr := cc.fetchTournamentEvents(res.addr, res.deployBlock)
		if eErr != nil {
			slog.Warn("failed to fetch tournament events", "error", eErr)
		} else {
			registry := make(commitmentRegistry)
			for _, cj := range events.commitmentsJoined {
				registry[cj.commitment] = cj.submitter
			}
			result.Commitments = formatCommitmentEvents(events.commitmentsJoined)
			result.Matches = formatMatchEvents(events.matchesCreated, events.matchesDeleted, registry)
			result.Advances = formatAdvanceEvents(events.matchesAdvanced)
		}
	}

	if jsonParam {
		return outputJSON(result)
	}

	p := &printer{w: os.Stdout}
	printTournamentContext(p, cc, res)
	printTournamentBasic(p, result)

	if tournamentEventsFlag {
		printTournamentEvents(p, result)
	}

	p.footer(cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
	return nil
}

// printTournamentContext prints application and epoch context before tournament details.
func printTournamentContext(p *printer, cc *chainClient, res tournamentResolution) {
	p.withSection(fmt.Sprintf("Application  %s", formatAddr(cc.appAddr)), func() {
		if res.epochNumber >= 0 {
			p.field("Epoch", fmt.Sprintf("%d", res.epochNumber))
		}
	})
}

// tournamentResolution holds the result of resolving a tournament address.
type tournamentResolution struct {
	addr        common.Address
	deployBlock uint64
	epochNumber int64 // -1 if unknown (direct address)
}

// resolveTournamentAddress determines the tournament address from:
// (1) positional arg, (2) --epoch flag, (3) current sealed epoch.
func (c *chainClient) resolveTournamentAddress(
	args []string,
) (tournamentResolution, error) {
	consensusAddr, err := c.getConsensusAddress()
	if err != nil {
		return tournamentResolution{}, fmt.Errorf("get consensus address: %w", err)
	}

	// Verify the consensus is DaveConsensus before calling Dave-specific methods.
	// Authority/Quorum contracts do not have tournaments.
	cType, _, err := c.detectConsensus(consensusAddr)
	if err != nil {
		return tournamentResolution{}, fmt.Errorf("detect consensus type: %w", err)
	}
	if cType != consensusDave {
		return tournamentResolution{}, fmt.Errorf(
			"tournament is only available for DaveConsensus (PRT); "+
				"consensus at %s is %s", consensusAddr, cType)
	}

	daveCaller, err := idaveconsensus.NewIDaveConsensusCaller(consensusAddr, c.eth)
	if err != nil {
		return tournamentResolution{}, fmt.Errorf("bind IDaveConsensus: %w", err)
	}

	deployBlockRaw, err := daveCaller.GetDeploymentBlockNumber(c.callOpts)
	if err != nil {
		return tournamentResolution{}, fmt.Errorf("GetDeploymentBlockNumber: %w", err)
	}
	deployBlock, err := safeUint64(deployBlockRaw, "deployment block")
	if err != nil {
		return tournamentResolution{}, err
	}

	// (1) Direct tournament address from positional arg.
	if len(args) > 1 {
		if !common.IsHexAddress(args[1]) {
			return tournamentResolution{},
				fmt.Errorf("invalid tournament address: %q", args[1])
		}
		return tournamentResolution{
			addr: common.HexToAddress(args[1]), deployBlock: deployBlock, epochNumber: -1,
		}, nil
	}

	// (2) --epoch flag: find tournament from EpochSealed events.
	if tournamentEpochFlag >= 0 {
		addr, fErr := c.findTournamentForEpoch(consensusAddr, daveCaller, deployBlock,
			uint64(tournamentEpochFlag))
		if fErr != nil {
			return tournamentResolution{}, fErr
		}
		return tournamentResolution{
			addr: addr, deployBlock: deployBlock, epochNumber: tournamentEpochFlag,
		}, nil
	}

	// (3) Default: current sealed epoch.
	sealed, err := daveCaller.GetCurrentSealedEpoch(c.callOpts)
	if err != nil {
		return tournamentResolution{}, fmt.Errorf("GetCurrentSealedEpoch: %w", err)
	}
	if sealed.Tournament == (common.Address{}) {
		return tournamentResolution{},
			fmt.Errorf("no sealed epochs yet — tournament not available")
	}
	epochNum, err := safeUint64(sealed.EpochNumber, "epoch number")
	if err != nil {
		return tournamentResolution{}, err
	}
	if epochNum > math.MaxInt64 {
		return tournamentResolution{},
			fmt.Errorf("epoch number %d exceeds int64 max", epochNum)
	}
	return tournamentResolution{
		addr: sealed.Tournament, deployBlock: deployBlock, epochNumber: int64(epochNum),
	}, nil
}

// findTournamentForEpoch locates the tournament address for a specific epoch
// using FindTransitions on the sealed epoch counter.
func (c *chainClient) findTournamentForEpoch(
	consensusAddr common.Address,
	daveCaller *idaveconsensus.IDaveConsensusCaller,
	deployBlock, targetEpoch uint64,
) (common.Address, error) {
	filterer, err := idaveconsensus.NewIDaveConsensusFilterer(consensusAddr, c.eth)
	if err != nil {
		return common.Address{}, fmt.Errorf("bind IDaveConsensus filterer: %w", err)
	}

	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		opts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
		sealed, sErr := daveCaller.GetCurrentSealedEpoch(opts)
		if sErr != nil {
			return nil, sErr
		}
		return sealed.EpochNumber, nil
	}

	var result common.Address

	onHit := func(block uint64) error {
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
			ev, pErr := filterer.ParseEpochSealed(*log)
			if pErr != nil {
				return pErr
			}
			epochNum, sErr := safeUint64(ev.EpochNumber, "epoch number")
			if sErr != nil {
				return sErr
			}
			if epochNum == targetEpoch {
				result = ev.Tournament
				return errFound
			}
		}
		return nil
	}

	// Use -1 as prevValue so epoch 0 is detected as a transition from -1→0.
	_, err = ethutil.FindTransitions(
		c.callOpts.Context, deployBlock, c.blockNum, big.NewInt(-1), oracle, onHit,
	)
	if err != nil && !errors.Is(err, errFound) {
		return common.Address{}, fmt.Errorf("find epoch %d tournament: %w", targetEpoch, err)
	}
	if result == (common.Address{}) {
		return common.Address{}, fmt.Errorf("epoch %d not found in EpochSealed events", targetEpoch)
	}
	return result, nil
}

// queryTournament reads ITournament view functions and returns a TournamentResult.
func (c *chainClient) queryTournament(addr common.Address) (*TournamentResult, error) {
	if err := c.ensureContract(addr, "tournament"); err != nil {
		return nil, err
	}
	caller, err := itournament.NewITournamentCaller(addr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind ITournament: %w", err)
	}

	levelConsts, err := caller.TournamentLevelConstants(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("TournamentLevelConstants: %w", err)
	}

	closed, err := caller.IsClosed(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("IsClosed: %w", err)
	}

	finished, err := caller.IsFinished(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("IsFinished: %w", err)
	}

	bondWei, err := caller.BondValue(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("BondValue: %w", err)
	}

	commitmentsRaw, err := caller.GetCommitmentJoinedCount(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetCommitmentJoinedCount: %w", err)
	}
	commitments, err := safeUint64(commitmentsRaw, "commitments joined")
	if err != nil {
		return nil, err
	}

	matchesCreatedRaw, err := caller.GetMatchCreatedCount(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetMatchCreatedCount: %w", err)
	}
	matchesCreated, err := safeUint64(matchesCreatedRaw, "matches created")
	if err != nil {
		return nil, err
	}

	matchesAdvancedRaw, err := caller.GetMatchAdvancedCount(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetMatchAdvancedCount: %w", err)
	}
	matchesAdvanced, err := safeUint64(matchesAdvancedRaw, "matches advanced")
	if err != nil {
		return nil, err
	}

	matchesDeletedRaw, err := caller.GetMatchDeletedCount(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetMatchDeletedCount: %w", err)
	}
	matchesDeleted, err := safeUint64(matchesDeletedRaw, "matches deleted")
	if err != nil {
		return nil, err
	}

	innerRaw, err := caller.GetNewInnerTournamentCount(c.callOpts)
	if err != nil {
		return nil, fmt.Errorf("GetNewInnerTournamentCount: %w", err)
	}
	inner, err := safeUint64(innerRaw, "inner tournaments")
	if err != nil {
		return nil, err
	}

	result := &TournamentResult{
		Address:           formatAddr(addr),
		Level:             levelConsts.Level,
		MaxLevel:          levelConsts.MaxLevel,
		Log2Step:          levelConsts.Log2step,
		Height:            levelConsts.Height,
		Closed:            closed,
		Finished:          finished,
		BondWei:           bondWei.String(),
		BondETH:           weiToETH(bondWei),
		CommitmentsJoined: commitments,
		MatchesCreated:    matchesCreated,
		MatchesAdvanced:   matchesAdvanced,
		MatchesDeleted:    matchesDeleted,
		InnerTournaments:  inner,
	}

	// Query finish details if tournament is finished.
	if finished {
		isFinished, finBlock, tErr := caller.TimeFinished(c.callOpts)
		if tErr == nil && isFinished {
			result.FinishedAtBlock = &finBlock
		}

		isRoot := levelConsts.Level == 0
		tFinished, hasWin, winner, finalState, tErr := c.tournamentResult(caller, isRoot)
		if tErr == nil && tFinished {
			result.HasWinner = &hasWin
			if hasWin {
				result.WinnerCommitment = formatHash(winner)
				if finalState != [32]byte{} {
					result.FinalMachineHash = formatHash(finalState)
				}
			}
		}
	}

	// CanBeEliminated for non-root tournaments.
	if levelConsts.Level > 0 {
		canElim, cErr := caller.CanBeEliminated(c.callOpts)
		if cErr == nil {
			result.CanBeEliminated = &canElim
		}
	}

	return result, nil
}

// tournamentResult handles ArbitrationResult (root) or InnerTournamentWinner (non-root)
// with TournamentFailedNoWinner revert detection.
func (c *chainClient) tournamentResult(
	caller *itournament.ITournamentCaller,
	isRoot bool,
) (finished bool, hasWinner bool, winner [32]byte, finalState [32]byte, err error) {
	if !isRoot {
		isFinished, _, winnerCommitment, _, iErr := caller.InnerTournamentWinner(c.callOpts)
		if iErr != nil {
			return false, false, [32]byte{}, [32]byte{}, iErr
		}
		return isFinished, isFinished && winnerCommitment != [32]byte{},
			winnerCommitment, [32]byte{}, nil
	}

	result, err := caller.ArbitrationResult(c.callOpts)
	if err != nil {
		if ethutil.IsCustomError(err, itournament.ITournamentMetaData, "TournamentFailedNoWinner") {
			return true, false, [32]byte{}, [32]byte{}, nil
		}
		return false, false, [32]byte{}, [32]byte{}, err
	}
	hasWin := result.WinnerCommitment != [32]byte{}
	return result.Finished, hasWin, result.WinnerCommitment, result.FinalState, nil
}

// commitmentRegistry maps commitment hashes to submitter addresses.
type commitmentRegistry map[[32]byte]common.Address

func (r commitmentRegistry) resolve(commitment [32]byte) string {
	if addr, ok := r[commitment]; ok {
		return addr.Hex()
	}
	return ""
}

// tournamentEvents holds raw event data from a single tournament.
type tournamentEvents struct {
	commitmentsJoined []rawCommitmentJoined
	matchesCreated    []rawMatchCreated
	matchesAdvanced   []rawMatchAdvanced
	matchesDeleted    []rawMatchDeleted
}

type rawCommitmentJoined struct {
	commitment     [32]byte
	finalStateHash [32]byte
	submitter      common.Address
	blockNumber    uint64
	txHash         common.Hash
}

type rawMatchCreated struct {
	matchIDHash [32]byte
	one         [32]byte
	two         [32]byte
	leftOfTwo   [32]byte
	blockNumber uint64
	txHash      common.Hash
}

type rawMatchAdvanced struct {
	matchIDHash [32]byte
	otherParent [32]byte
	leftNode    [32]byte
	blockNumber uint64
	txHash      common.Hash
}

type rawMatchDeleted struct {
	matchIDHash      [32]byte
	one              [32]byte
	two              [32]byte
	reason           uint8
	winnerCommitment uint8
	blockNumber      uint64
	txHash           common.Hash
}

// fetchTournamentEvents retrieves events using FindTransitions per counter.
func (c *chainClient) fetchTournamentEvents(
	addr common.Address,
	fromBlock uint64,
) (*tournamentEvents, error) {
	caller, err := itournament.NewITournamentCaller(addr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind ITournament caller: %w", err)
	}
	filterer, err := itournament.NewITournamentFilterer(addr, c.eth)
	if err != nil {
		return nil, fmt.Errorf("bind ITournament filterer: %w", err)
	}

	events := &tournamentEvents{}
	limit := tournamentLimitFlag

	// CommitmentJoined
	if err := c.findAndCollectEvents(addr, fromBlock, "CommitmentJoined",
		func(ctx context.Context, block uint64) (*big.Int, error) {
			opts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
			return caller.GetCommitmentJoinedCount(opts)
		},
		func(log *types.Log) (bool, error) {
			if limit > 0 && len(events.commitmentsJoined) >= limit {
				return true, nil
			}
			ev, pErr := filterer.ParseCommitmentJoined(*log)
			if pErr != nil {
				return false, pErr
			}
			events.commitmentsJoined = append(events.commitmentsJoined, rawCommitmentJoined{
				commitment:     ev.Commitment,
				finalStateHash: ev.FinalStateHash,
				submitter:      ev.Submitter,
				blockNumber:    log.BlockNumber,
				txHash:         log.TxHash,
			})
			return false, nil
		},
	); err != nil {
		return nil, err
	}

	// MatchCreated
	if err := c.findAndCollectEvents(addr, fromBlock, "MatchCreated",
		func(ctx context.Context, block uint64) (*big.Int, error) {
			opts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
			return caller.GetMatchCreatedCount(opts)
		},
		func(log *types.Log) (bool, error) {
			if limit > 0 && len(events.matchesCreated) >= limit {
				return true, nil
			}
			ev, pErr := filterer.ParseMatchCreated(*log)
			if pErr != nil {
				return false, pErr
			}
			events.matchesCreated = append(events.matchesCreated, rawMatchCreated{
				matchIDHash: ev.MatchIdHash,
				one:         ev.One,
				two:         ev.Two,
				leftOfTwo:   ev.LeftOfTwo,
				blockNumber: log.BlockNumber,
				txHash:      log.TxHash,
			})
			return false, nil
		},
	); err != nil {
		return nil, err
	}

	// MatchAdvanced
	if err := c.findAndCollectEvents(addr, fromBlock, "MatchAdvanced",
		func(ctx context.Context, block uint64) (*big.Int, error) {
			opts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
			return caller.GetMatchAdvancedCount(opts)
		},
		func(log *types.Log) (bool, error) {
			if limit > 0 && len(events.matchesAdvanced) >= limit {
				return true, nil
			}
			ev, pErr := filterer.ParseMatchAdvanced(*log)
			if pErr != nil {
				return false, pErr
			}
			events.matchesAdvanced = append(events.matchesAdvanced, rawMatchAdvanced{
				matchIDHash: ev.MatchIdHash,
				otherParent: ev.OtherParent,
				leftNode:    ev.LeftNode,
				blockNumber: log.BlockNumber,
				txHash:      log.TxHash,
			})
			return false, nil
		},
	); err != nil {
		return nil, err
	}

	// MatchDeleted
	if err := c.findAndCollectEvents(addr, fromBlock, "MatchDeleted",
		func(ctx context.Context, block uint64) (*big.Int, error) {
			opts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
			return caller.GetMatchDeletedCount(opts)
		},
		func(log *types.Log) (bool, error) {
			if limit > 0 && len(events.matchesDeleted) >= limit {
				return true, nil
			}
			ev, pErr := filterer.ParseMatchDeleted(*log)
			if pErr != nil {
				return false, pErr
			}
			events.matchesDeleted = append(events.matchesDeleted, rawMatchDeleted{
				matchIDHash:      ev.MatchIdHash,
				one:              ev.One,
				two:              ev.Two,
				reason:           ev.Reason,
				winnerCommitment: ev.WinnerCommitment,
				blockNumber:      log.BlockNumber,
				txHash:           log.TxHash,
			})
			return false, nil
		},
	); err != nil {
		return nil, err
	}

	return events, nil
}

// findAndCollectEvents uses FindTransitions to discover transition blocks,
// then queries and processes events at each block. Processing stops when
// the process callback returns done=true.
func (c *chainClient) findAndCollectEvents(
	addr common.Address,
	fromBlock uint64,
	eventName string,
	oracle ethutil.TransitionQueryFn,
	process func(*types.Log) (done bool, err error),
) error {
	onHit := func(block uint64) error {
		q, qErr := buildEventFilterQuery(
			addr, eventName, itournament.ITournamentMetaData, block, block,
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
			done, pErr := process(log)
			if pErr != nil {
				return pErr
			}
			if done {
				return errLimitReached
			}
		}
		return nil
	}

	// Wrap oracle to handle contracts that don't exist at fromBlock yet
	// (e.g., inner tournaments created after the DaveConsensus deploy block).
	safeOracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		val, oErr := oracle(ctx, block)
		if oErr != nil && errors.Is(oErr, bind.ErrNoCode) {
			return big.NewInt(0), nil
		}
		return val, oErr
	}

	_, err := ethutil.FindTransitions(
		c.callOpts.Context, fromBlock, c.blockNum, big.NewInt(0), safeOracle, onHit,
	)
	if err != nil && !errors.Is(err, errLimitReached) {
		return fmt.Errorf("find %s transitions: %w", eventName, err)
	}
	return nil
}

// tournamentTree holds the BFS traversal result.
type tournamentTree struct {
	root     *TournamentResult
	nodes    []*TournamentResult
	children map[common.Address][]*TournamentResult
	registry commitmentRegistry
}

// walkTournamentTree performs BFS across the tournament hierarchy.
func (c *chainClient) walkTournamentTree(
	rootAddr common.Address,
	fromBlock uint64,
	includeEvents bool,
) (*tournamentTree, error) {
	tree := &tournamentTree{
		children: make(map[common.Address][]*TournamentResult),
		registry: make(commitmentRegistry),
	}

	type queueItem struct {
		addr   common.Address
		parent *common.Address
	}

	queue := []queueItem{{addr: rootAddr}}
	visited := make(map[common.Address]bool)
	maxNodes := tournamentMaxNodesFlag

	for len(queue) > 0 && len(tree.nodes) < maxNodes {
		item := queue[0]
		queue[0] = queueItem{} // avoid retaining references in the backing array
		queue = queue[1:]

		if visited[item.addr] {
			continue
		}
		visited[item.addr] = true

		info, err := c.queryTournament(item.addr)
		if err != nil {
			return nil, fmt.Errorf("query tournament %s: %w", item.addr, err)
		}

		if includeEvents {
			events, eErr := c.fetchTournamentEvents(item.addr, fromBlock)
			if eErr != nil {
				slog.Warn("failed to fetch events", "tournament", item.addr, "error", eErr)
			} else {
				for _, cj := range events.commitmentsJoined {
					tree.registry[cj.commitment] = cj.submitter
				}
				info.Commitments = formatCommitmentEvents(events.commitmentsJoined)
				info.Matches = formatMatchEvents(
					events.matchesCreated, events.matchesDeleted, tree.registry)
				info.Advances = formatAdvanceEvents(events.matchesAdvanced)
			}
		}

		tree.nodes = append(tree.nodes, info)
		if tree.root == nil {
			tree.root = info
		}
		if item.parent != nil {
			tree.children[*item.parent] = append(tree.children[*item.parent], info)
		}

		// Discover child tournaments.
		if info.Level < info.MaxLevel && info.InnerTournaments > 0 {
			children, dErr := c.discoverChildTournaments(item.addr, fromBlock)
			if dErr != nil {
				slog.Warn("failed to discover children", "tournament", item.addr, "error", dErr)
				continue
			}
			for _, child := range children {
				parentAddr := item.addr
				queue = append(queue, queueItem{addr: child, parent: &parentAddr})
			}
		}
	}

	if len(queue) > 0 {
		slog.Warn("tournament tree truncated",
			"max_nodes", maxNodes, "remaining_in_queue", len(queue))
	}

	// Second pass: annotate winner addresses and populate Children.
	for _, node := range tree.nodes {
		if node.WinnerCommitment != "" {
			winnerHash := common.HexToHash(node.WinnerCommitment)
			node.WinnerAddress = tree.registry.resolve([32]byte(winnerHash))
		}
		addr := common.HexToAddress(node.Address)
		if kids, ok := tree.children[addr]; ok {
			node.Children = kids
		}
	}

	return tree, nil
}

// discoverChildTournaments uses FindTransitions with GetNewInnerTournamentCount.
func (c *chainClient) discoverChildTournaments(
	tournamentAddr common.Address,
	fromBlock uint64,
) ([]common.Address, error) {
	caller, err := itournament.NewITournamentCaller(tournamentAddr, c.eth)
	if err != nil {
		return nil, err
	}
	filterer, err := itournament.NewITournamentFilterer(tournamentAddr, c.eth)
	if err != nil {
		return nil, err
	}

	var children []common.Address

	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		opts := &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
		return caller.GetNewInnerTournamentCount(opts)
	}

	onHit := func(block uint64) error {
		q, qErr := buildEventFilterQuery(
			tournamentAddr, "NewInnerTournament",
			itournament.ITournamentMetaData, block, block,
		)
		if qErr != nil {
			return qErr
		}
		itr, fErr := c.filter.ChunkedFilterLogs(c.callOpts.Context, c.eth, q)
		if fErr != nil {
			return fmt.Errorf("filter NewInnerTournament at block %d: %w", block, fErr)
		}
		for log, logErr := range itr {
			if logErr != nil {
				return logErr
			}
			ev, pErr := filterer.ParseNewInnerTournament(*log)
			if pErr != nil {
				return pErr
			}
			children = append(children, ev.ChildTournament)
		}
		return nil
	}

	_, err = ethutil.FindTransitions(
		c.callOpts.Context, fromBlock, c.blockNum, big.NewInt(0), oracle, onHit,
	)
	if err != nil {
		return nil, fmt.Errorf("find child tournament transitions: %w", err)
	}
	return children, nil
}

// buildEventFilterQuery constructs a FilterQuery for the named event.
// Each indexedTopic corresponds to an indexed event parameter in order.
// Use nil for a wildcard (match any value at that position).
func buildEventFilterQuery(
	contractAddr common.Address,
	eventName string,
	metaData *bind.MetaData,
	fromBlock, toBlock uint64,
	indexedTopics ...*common.Hash,
) (ethereum.FilterQuery, error) {
	contractABI, err := metaData.GetAbi()
	if err != nil {
		return ethereum.FilterQuery{}, fmt.Errorf("get ABI: %w", err)
	}

	ev, ok := contractABI.Events[eventName]
	if !ok {
		return ethereum.FilterQuery{}, fmt.Errorf("event %q not found in ABI", eventName)
	}

	topicSets := [][]any{{ev.ID}}
	for _, t := range indexedTopics {
		if t == nil {
			topicSets = append(topicSets, nil)
		} else {
			topicSets = append(topicSets, []any{*t})
		}
	}

	topics, err := abi.MakeTopics(topicSets...)
	if err != nil {
		return ethereum.FilterQuery{}, fmt.Errorf("make topics for %s: %w", eventName, err)
	}

	return ethereum.FilterQuery{
		Addresses: []common.Address{contractAddr},
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Topics:    topics,
	}, nil
}

// Format helpers: convert raw events to JSON output types.

func formatCommitmentEvents(raw []rawCommitmentJoined) []CommitmentEvent {
	if len(raw) == 0 {
		return nil
	}
	out := make([]CommitmentEvent, len(raw))
	for i, r := range raw {
		out[i] = CommitmentEvent{
			Commitment:     formatHash(r.commitment),
			FinalStateHash: formatHash(r.finalStateHash),
			Submitter:      formatAddr(r.submitter),
			BlockNumber:    r.blockNumber,
			TxHash:         r.txHash.Hex(),
		}
	}
	return out
}

func formatMatchEvents(
	created []rawMatchCreated,
	deleted []rawMatchDeleted,
	registry commitmentRegistry,
) []MatchEvent {
	if len(created) == 0 {
		return nil
	}

	// Build deletion map keyed by match ID hash.
	deletionMap := make(map[[32]byte]rawMatchDeleted, len(deleted))
	for _, d := range deleted {
		deletionMap[d.matchIDHash] = d
	}

	out := make([]MatchEvent, len(created))
	for i, r := range created {
		me := MatchEvent{
			MatchIDHash:   formatHash(r.matchIDHash),
			CommitmentOne: formatHash(r.one),
			CommitmentTwo: formatHash(r.two),
			PlayerOneAddr: registry.resolve(r.one),
			PlayerTwoAddr: registry.resolve(r.two),
			LeftOfTwo:     formatHash(r.leftOfTwo),
			BlockNumber:   r.blockNumber,
			TxHash:        r.txHash.Hex(),
		}

		if d, ok := deletionMap[r.matchIDHash]; ok {
			me.DeletionReason = matchDeletionReason(d.reason)
			me.Winner = matchWinner(d.winnerCommitment)
			block := d.blockNumber
			me.DeletionBlock = &block
			me.DeletionTxHash = d.txHash.Hex()

			// Resolve winner address from commitment.
			switch d.winnerCommitment {
			case 1:
				me.WinnerAddr = registry.resolve(d.one)
			case 2: //nolint:mnd
				me.WinnerAddr = registry.resolve(d.two)
			}
		}

		out[i] = me
	}
	return out
}

func formatAdvanceEvents(raw []rawMatchAdvanced) []MatchAdvanceEvent {
	if len(raw) == 0 {
		return nil
	}
	out := make([]MatchAdvanceEvent, len(raw))
	for i, r := range raw {
		out[i] = MatchAdvanceEvent{
			MatchIDHash: formatHash(r.matchIDHash),
			OtherParent: formatHash(r.otherParent),
			LeftNode:    formatHash(r.leftNode),
			BlockNumber: r.blockNumber,
			TxHash:      r.txHash.Hex(),
		}
	}
	return out
}

// Text output helpers.

func printTournamentBasic(p *printer, r *TournamentResult) {
	levelName := "Root"
	if r.Level == r.MaxLevel {
		levelName = "Leaf"
	} else if r.Level > 0 {
		levelName = "Inner"
	}

	p.withSection(fmt.Sprintf("%s Tournament  %s  (level %d/%d)",
		levelName, r.Address, r.Level, r.MaxLevel), func() {
		p.field("Status", tournamentStatus(r.Closed, r.Finished))
		if r.Finished && r.FinishedAtBlock != nil {
			p.field("Finished", fmt.Sprintf("yes (block %d)", *r.FinishedAtBlock))
		}
		if r.HasWinner != nil {
			if *r.HasWinner {
				winInfo := r.WinnerCommitment
				if r.WinnerAddress != "" {
					winInfo += fmt.Sprintf("  (%s)", r.WinnerAddress)
				}
				p.field("Winner", winInfo)
				if r.FinalMachineHash != "" {
					p.field("Final Machine Hash", r.FinalMachineHash)
				}
			} else {
				p.field("Winner", "NONE (all commitments eliminated)")
			}
		}
		p.field("Bond", r.BondETH)
		p.field("Commitments Joined", fmt.Sprintf("%d", r.CommitmentsJoined))
		p.field("Matches Created", fmt.Sprintf("%d", r.MatchesCreated))
		p.field("Matches Advanced", fmt.Sprintf("%d", r.MatchesAdvanced))
		p.field("Matches Deleted", fmt.Sprintf("%d", r.MatchesDeleted))
		p.field("Inner Tournaments", fmt.Sprintf("%d", r.InnerTournaments))
		if r.CanBeEliminated != nil {
			p.field("Can Be Eliminated", fmt.Sprintf("%t", *r.CanBeEliminated))
		}
	})
}

func printTournamentEvents(p *printer, r *TournamentResult) {
	if len(r.Commitments) > 0 {
		p.withSection("Commitments Joined:", func() {
			for i, c := range r.Commitments {
				p.withSection(fmt.Sprintf("[%d] Commitment  %s", i+1, c.Commitment), func() {
					p.field("Submitter", c.Submitter)
					p.field("Final State", c.FinalStateHash)
					p.field("Block", fmt.Sprintf("%d", c.BlockNumber))
					p.field("Tx", c.TxHash)
				})
			}
		})
	}

	if len(r.Matches) > 0 {
		p.withSection("Matches:", func() {
			for i, m := range r.Matches {
				p.withSection(fmt.Sprintf("[%d] Match  %s", i+1, m.MatchIDHash), func() {
					oneInfo := m.CommitmentOne
					if m.PlayerOneAddr != "" {
						oneInfo += fmt.Sprintf("  (%s)", m.PlayerOneAddr)
					}
					p.field("Player One", oneInfo)
					twoInfo := m.CommitmentTwo
					if m.PlayerTwoAddr != "" {
						twoInfo += fmt.Sprintf("  (%s)", m.PlayerTwoAddr)
					}
					p.field("Player Two", twoInfo)
					p.field("Block", fmt.Sprintf("%d", m.BlockNumber))
					p.field("Tx", m.TxHash)
					if m.DeletionReason != "" {
						p.field("Outcome", fmt.Sprintf("%s → %s", m.DeletionReason, m.Winner))
						if m.WinnerAddr != "" {
							p.field("Winner Address", m.WinnerAddr)
						}
					}
				})
			}
		})
	}

	if len(r.Advances) > 0 {
		p.withSection("Match Advances:", func() {
			for i, a := range r.Advances {
				p.withSection(fmt.Sprintf("[%d] Match  %s", i+1, a.MatchIDHash), func() {
					p.field("Block", fmt.Sprintf("%d", a.BlockNumber))
					p.field("Tx", a.TxHash)
				})
			}
		})
	}
}

func renderTournamentTree(
	p *printer,
	root *TournamentResult,
	tree *tournamentTree,
	depth int,
) {
	prefix := ""
	if depth > 0 {
		prefix = strings.Repeat("\t", depth-1) + "└─ "
	}

	levelName := "Root"
	if root.Level == root.MaxLevel {
		levelName = "Leaf"
	} else if root.Level > 0 {
		levelName = "Inner"
	}

	if depth > 0 {
		fmt.Fprintf(p.w, "\n")
	}
	fmt.Fprintf(p.w, "%s%s Tournament  %s  (level %d/%d)\n",
		prefix, levelName, root.Address,
		root.Level, root.MaxLevel)

	indent := strings.Repeat("\t", depth+1)
	fmt.Fprintf(p.w, "%sStatus               %s\n",
		indent, tournamentStatus(root.Closed, root.Finished))

	if root.Finished && root.FinishedAtBlock != nil {
		fmt.Fprintf(p.w, "%sFinished             yes (block %d)\n",
			indent, *root.FinishedAtBlock)
	}
	if root.HasWinner != nil {
		if *root.HasWinner {
			winInfo := root.WinnerCommitment
			if root.WinnerAddress != "" {
				winInfo += fmt.Sprintf("  (%s)", root.WinnerAddress)
			}
			fmt.Fprintf(p.w, "%sWinner               %s\n", indent, winInfo)
		} else {
			fmt.Fprintf(p.w, "%sWinner               NONE (all commitments eliminated)\n", indent)
		}
	}
	fmt.Fprintf(p.w,
		"%sCommitments: %d  |  Matches: %d created, %d advanced, %d deleted  |  Inner: %d\n",
		indent, root.CommitmentsJoined, root.MatchesCreated,
		root.MatchesAdvanced, root.MatchesDeleted, root.InnerTournaments)

	// Recurse into children.
	addr := common.HexToAddress(root.Address)
	for _, child := range tree.children[addr] {
		renderTournamentTree(p, child, tree, depth+1)
	}
}
