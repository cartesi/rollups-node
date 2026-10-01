// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveappfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
)

// The verifier decodes chain evidence with the generated contract bindings
// only. It does not use the rollups node's adapters or event mappers, so a mapping
// bug in the node cannot produce both sides of a comparison.

// Solidity ITournament.TournamentStanding.ROOT_WINNER.
const tournamentStandingRootWinner = 2

const (
	receiptPollInterval = 500 * time.Millisecond
	gasLimitSimpleTx    = 300_000
)

type chain struct {
	url string
	raw *rpc.Client
	eth *ethclient.Client
}

func dialChain(ctx context.Context, url string) (*chain, error) {
	raw, err := rpc.DialContext(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", url, err)
	}
	return &chain{url: url, raw: raw, eth: ethclient.NewClient(raw)}, nil
}

func (c *chain) close() { c.raw.Close() }

func (c *chain) chainID(ctx context.Context) (uint64, error) {
	id, err := c.eth.ChainID(ctx)
	if err != nil {
		return 0, fmt.Errorf("reading chain id: %w", err)
	}
	return id.Uint64(), nil
}

func (c *chain) head(ctx context.Context) (*types.Header, error) {
	header, err := c.eth.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("reading head: %w", err)
	}
	return header, nil
}

// anvil calls an Anvil-specific RPC method.
func (c *chain) anvil(ctx context.Context, method string, params ...any) error {
	if err := c.raw.CallContext(ctx, nil, method, params...); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	return nil
}

// historyBatchSize is the number of eth_getBalance calls per JSON-RPC batch.
const historyBatchSize = 500

const (
	// historyStallTimeout ends the wait when the set of unreadable blocks
	// stops shrinking: the dump itself lacks those states.
	historyStallTimeout = 30 * time.Second
	historyLogInterval  = 10 * time.Second
)

// waitForHistory waits until the state of every block below head is readable.
//
// Anvil 1.5.1 opens its RPC port before it has loaded the historical states of
// a dump, and it loads them in no particular order (they are keyed by block
// hash). Until a state is loaded, reads at that block fail with
// BlockOutOfRangeError. The rollups node reads counters at arbitrary blocks
// (binary searches), so every block must be readable, not a sample.
func (c *chain) waitForHistory(ctx context.Context, stage string, head uint64, timeout time.Duration) error {
	if head <= 1 {
		return nil
	}
	missing := make([]uint64, 0, head-1)
	for block := uint64(1); block < head; block++ {
		missing = append(missing, block)
	}
	started := time.Now()
	deadline := started.Add(timeout)
	lastProgress, lastLog := started, started
	for {
		still, err := c.unreadableBlocks(ctx, missing)
		if err != nil {
			return fmt.Errorf("reading the dump history: %w", err)
		}
		if len(still) == 0 {
			logger.Debug("dump history readable", "blocks", head-1, "after", time.Since(started).Round(time.Second))
			return nil
		}
		if len(still) < len(missing) {
			lastProgress = time.Now()
		}
		missing = still
		if time.Since(lastProgress) > historyStallTimeout || time.Now().After(deadline) {
			return fmt.Errorf("the dump has no state for %d of %d blocks (%s); Anvil stopped loading it (fixture failure)",
				len(missing), head-1, describeBlocks(missing))
		}
		if time.Since(lastLog) >= historyLogInterval {
			step(stage, "Anvil is still loading the dump history: %d of %d blocks readable", head-1-uint64(len(missing)), head-1)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// unreadableBlocks returns the blocks whose state cannot be read, in order.
func (c *chain) unreadableBlocks(ctx context.Context, blocks []uint64) ([]uint64, error) {
	var missing []uint64
	for start := 0; start < len(blocks); start += historyBatchSize {
		chunk := blocks[start:min(len(blocks), start+historyBatchSize)]
		results := make([]hexutil.Big, len(chunk))
		batch := make([]rpc.BatchElem, len(chunk))
		for i, block := range chunk {
			batch[i] = rpc.BatchElem{Method: "eth_getBalance",
				Args: []any{common.Address{}, hexutil.EncodeUint64(block)}, Result: &results[i]}
		}
		if err := c.raw.BatchCallContext(ctx, batch); err != nil {
			return nil, err
		}
		for i, elem := range batch {
			if elem.Error != nil {
				missing = append(missing, chunk[i])
			}
		}
	}
	return missing, nil
}

// describeBlocks renders sorted block numbers as ranges, for example
// "24-99, 120", listing at most a few ranges.
func describeBlocks(blocks []uint64) string {
	const maxRanges = 4
	var ranges []string
	for i := 0; i < len(blocks); {
		j := i
		for j+1 < len(blocks) && blocks[j+1] == blocks[j]+1 {
			j++
		}
		if len(ranges) == maxRanges {
			ranges = append(ranges, "…")
			break
		}
		if i == j {
			ranges = append(ranges, strconv.FormatUint(blocks[i], 10))
		} else {
			ranges = append(ranges, fmt.Sprintf("%d-%d", blocks[i], blocks[j]))
		}
		i = j + 1
	}
	return strings.Join(ranges, ", ")
}

func filterTo(ctx context.Context, from, to uint64) *bind.FilterOpts {
	end := to
	return &bind.FilterOpts{Start: from, End: &end, Context: ctx}
}

func callAt(ctx context.Context, block uint64) *bind.CallOpts {
	return &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
}

type daveApp struct {
	App         common.Address `json:"app"`
	Consensus   common.Address `json:"consensus"`
	DeployBlock uint64         `json:"deploy_block"`
}

// daveApps lists the applications created by the Dave application factory.
func (c *chain) daveApps(ctx context.Context, factory common.Address, to uint64) ([]daveApp, error) {
	contract, err := idaveappfactory.NewIDaveAppFactory(factory, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterDaveAppCreated(filterTo(ctx, 0, to))
	if err != nil {
		return nil, fmt.Errorf("reading DaveAppCreated: %w", err)
	}
	defer it.Close()
	var apps []daveApp
	for it.Next() {
		apps = append(apps, daveApp{App: it.Event.AppContract, Consensus: it.Event.DaveConsensus, DeployBlock: it.Event.Raw.BlockNumber})
	}
	return apps, it.Error()
}

// selectDaveApp returns the requested application, or the only one.
func selectDaveApp(apps []daveApp, requested string) (daveApp, error) {
	if requested != "" {
		for _, app := range apps {
			if strings.EqualFold(app.App.Hex(), requested) {
				return app, nil
			}
		}
		return daveApp{}, fmt.Errorf("application %s was not created by the Dave factory", requested)
	}
	switch len(apps) {
	case 0:
		return daveApp{}, errors.New("no DaveAppCreated event found")
	case 1:
		return apps[0], nil
	default:
		return daveApp{}, fmt.Errorf("%d Dave applications found; select one with --app", len(apps))
	}
}

// applicationFacts returns what the application contract says about itself:
// its outputs Merkle root validator (the consensus) and its template hash.
func (c *chain) applicationFacts(ctx context.Context, app common.Address, block uint64) (common.Address, common.Hash, error) {
	contract, err := iapplication.NewIApplication(app, c.eth)
	if err != nil {
		return common.Address{}, common.Hash{}, err
	}
	validator, err := contract.GetOutputsMerkleRootValidator(callAt(ctx, block))
	if err != nil {
		return common.Address{}, common.Hash{}, fmt.Errorf("reading the outputs Merkle root validator: %w", err)
	}
	template, err := contract.GetTemplateHash(callAt(ctx, block))
	if err != nil {
		return common.Address{}, common.Hash{}, fmt.Errorf("reading template hash: %w", err)
	}
	return validator, template, nil
}

func (c *chain) templateHash(ctx context.Context, app common.Address, block uint64) (common.Hash, error) {
	contract, err := iapplication.NewIApplication(app, c.eth)
	if err != nil {
		return common.Hash{}, err
	}
	hash, err := contract.GetTemplateHash(callAt(ctx, block))
	if err != nil {
		return common.Hash{}, fmt.Errorf("reading template hash: %w", err)
	}
	return hash, nil
}

type sealedEpoch struct {
	Index        uint64         `json:"index"`
	Lower        uint64         `json:"input_index_lower_bound"`
	Upper        uint64         `json:"input_index_upper_bound"`
	InitialState common.Hash    `json:"initial_machine_state_hash"`
	Tournament   common.Address `json:"tournament"`
	Block        uint64         `json:"block"`
	Tx           common.Hash    `json:"transaction_hash"`
}

func (c *chain) sealedEpochs(ctx context.Context, consensus common.Address, to uint64) ([]sealedEpoch, error) {
	contract, err := idaveconsensus.NewIDaveConsensus(consensus, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterEpochSealed(filterTo(ctx, 0, to), nil)
	if err != nil {
		return nil, fmt.Errorf("reading EpochSealed: %w", err)
	}
	defer it.Close()
	var epochs []sealedEpoch
	for it.Next() {
		e := it.Event
		epochs = append(epochs, sealedEpoch{
			Index: e.EpochNumber.Uint64(), Lower: e.InputIndexLowerBound.Uint64(), Upper: e.InputIndexUpperBound.Uint64(),
			InitialState: e.InitialMachineStateHash, Tournament: e.Tournament,
			Block: e.Raw.BlockNumber, Tx: e.Raw.TxHash,
		})
	}
	sort.Slice(epochs, func(i, j int) bool { return epochs[i].Index < epochs[j].Index })
	return epochs, it.Error()
}

type stagedEpoch struct {
	Index       uint64      `json:"index"`
	FinalState  common.Hash `json:"final_machine_state_hash"`
	OutputsRoot common.Hash `json:"outputs_merkle_root"`
	Block       uint64      `json:"block"`
	Tx          common.Hash `json:"transaction_hash"`
}

func (c *chain) stagedEpochs(ctx context.Context, consensus common.Address, to uint64) (map[uint64]stagedEpoch, error) {
	contract, err := idaveconsensus.NewIDaveConsensus(consensus, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterEpochStaged(filterTo(ctx, 0, to), nil)
	if err != nil {
		return nil, fmt.Errorf("reading EpochStaged: %w", err)
	}
	defer it.Close()
	staged := map[uint64]stagedEpoch{}
	for it.Next() {
		e := it.Event
		staged[e.EpochNumber.Uint64()] = stagedEpoch{
			Index: e.EpochNumber.Uint64(), FinalState: e.StagedPostEpochMachineStateHash,
			OutputsRoot: e.StagedPostEpochOutputsMerkleRoot, Block: e.Raw.BlockNumber, Tx: e.Raw.TxHash,
		}
	}
	return staged, it.Error()
}

// chainTournament places one tournament in the tree of an epoch.
type chainTournament struct {
	Parent common.Address `json:"parent"` // the zero address for a root
	Epoch  uint64         `json:"epoch"`
}

// tournamentTree returns every tournament reachable from the roots (mapped to
// their epochs) through NewInnerTournament events.
func (c *chain) tournamentTree(ctx context.Context, roots map[common.Address]uint64, to uint64,
) (map[common.Address]chainTournament, error) {
	tree := map[common.Address]chainTournament{}
	queue := make([]common.Address, 0, len(roots))
	for root, epoch := range roots {
		tree[root] = chainTournament{Epoch: epoch}
		queue = append(queue, root)
	}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		contract, err := itournament.NewITournament(parent, c.eth)
		if err != nil {
			return nil, err
		}
		it, err := contract.FilterNewInnerTournament(filterTo(ctx, 0, to), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("reading NewInnerTournament of %s: %w", parent, err)
		}
		for it.Next() {
			childAddress := it.Event.ChildTournament
			if _, seen := tree[childAddress]; !seen {
				tree[childAddress] = chainTournament{Parent: parent, Epoch: tree[parent].Epoch}
				queue = append(queue, childAddress)
			}
		}
		err = it.Error()
		it.Close()
		if err != nil {
			return nil, err
		}
	}
	return tree, nil
}

type rootStanding struct {
	Finished  bool        `json:"finished"`
	HasWinner bool        `json:"has_winner"`
	Winner    common.Hash `json:"winner"`
}

func (c *chain) rootStanding(ctx context.Context, tournament common.Address, block uint64) (rootStanding, error) {
	contract, err := itournament.NewITournament(tournament, c.eth)
	if err != nil {
		return rootStanding{}, err
	}
	view, err := contract.TournamentStanding(callAt(ctx, block))
	if err != nil {
		return rootStanding{}, fmt.Errorf("reading standing of %s: %w", tournament, err)
	}
	winner := view.Standing == tournamentStandingRootWinner && view.HasCandidate
	return rootStanding{Finished: view.FinishedAt != 0, HasWinner: winner, Winner: view.Candidate}, nil
}

// sentryClaims returns the final machine state that sentry claimed per epoch.
func (c *chain) sentryClaims(ctx context.Context, consensus, sentry common.Address, to uint64) (map[uint64]common.Hash, error) {
	contract, err := idaveconsensus.NewIDaveConsensus(consensus, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterSentryClaim(filterTo(ctx, 0, to), nil, nil, []common.Address{sentry})
	if err != nil {
		return nil, fmt.Errorf("reading SentryClaim: %w", err)
	}
	defer it.Close()
	claims := map[uint64]common.Hash{}
	for it.Next() {
		claims[it.Event.EpochNumber.Uint64()] = it.Event.PostEpochMachineStateHash
	}
	return claims, it.Error()
}

type commitmentJoin struct {
	Commitment common.Hash    `json:"commitment"`
	FinalState common.Hash    `json:"final_state_hash"`
	Submitter  common.Address `json:"submitter"`
	Block      uint64         `json:"block"`
	Tx         common.Hash    `json:"transaction_hash"`
}

func (c *chain) joins(ctx context.Context, tournament common.Address, to uint64) ([]commitmentJoin, error) {
	contract, err := itournament.NewITournament(tournament, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterCommitmentJoined(filterTo(ctx, 0, to), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("reading CommitmentJoined: %w", err)
	}
	defer it.Close()
	var joins []commitmentJoin
	for it.Next() {
		e := it.Event
		joins = append(joins, commitmentJoin{Commitment: e.Commitment, FinalState: e.FinalStateHash, Submitter: e.Submitter,
			Block: e.Raw.BlockNumber, Tx: e.Raw.TxHash})
	}
	return joins, it.Error()
}

// chainMatch is a MatchCreated event and, when the match ended, its
// MatchDeleted event.
type chainMatch struct {
	Tournament common.Address
	ID         common.Hash
	One        common.Hash
	Two        common.Hash
	LeftOfTwo  common.Hash
	Block      uint64
	Tx         common.Hash
	Deleted    bool
	DeleteTx   common.Hash
}

func (c *chain) matches(ctx context.Context, tournament common.Address, to uint64) ([]chainMatch, error) {
	contract, err := itournament.NewITournament(tournament, c.eth)
	if err != nil {
		return nil, err
	}
	created, err := contract.FilterMatchCreated(filterTo(ctx, 0, to), nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("reading MatchCreated of %s: %w", tournament, err)
	}
	defer created.Close()
	var matches []chainMatch
	byID := map[common.Hash]int{}
	for created.Next() {
		e := created.Event
		byID[e.MatchIdHash] = len(matches)
		matches = append(matches, chainMatch{Tournament: tournament, ID: e.MatchIdHash, One: e.One, Two: e.Two,
			LeftOfTwo: e.LeftOfTwo, Block: e.Raw.BlockNumber, Tx: e.Raw.TxHash})
	}
	if err := created.Error(); err != nil {
		return nil, err
	}
	deleted, err := contract.FilterMatchDeleted(filterTo(ctx, 0, to), nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("reading MatchDeleted of %s: %w", tournament, err)
	}
	defer deleted.Close()
	for deleted.Next() {
		i, ok := byID[deleted.Event.MatchIdHash]
		if !ok {
			return nil, fmt.Errorf("MatchDeleted of %s for unknown match %x", tournament, deleted.Event.MatchIdHash)
		}
		matches[i].Deleted, matches[i].DeleteTx = true, deleted.Event.Raw.TxHash
	}
	return matches, deleted.Error()
}

// fakeCommitmentNodes builds a structurally valid commitment whose states
// are all zero, as Dave's bad_commitment scenario does: the final state is
// zero, and its proof is height-1 zero siblings followed by the left node.
func fakeCommitmentNodes(height uint64) (finalState common.Hash, proof [][32]byte, left, right common.Hash) {
	for i := uint64(1); i < height; i++ {
		right = crypto.Keccak256Hash(common.Hash{}.Bytes(), right.Bytes())
		proof = append(proof, [32]byte{})
	}
	proof = append(proof, left)
	return finalState, proof, left, right
}

// joinFakeCommitment joins a fake commitment in a tournament and returns it
// with the join transaction. It pays the tournament's bond.
func (c *chain) joinFakeCommitment(ctx context.Context, tournament common.Address, account *testAccount,
) (common.Hash, common.Hash, error) {
	contract, err := itournament.NewITournament(tournament, c.eth)
	if err != nil {
		return common.Hash{}, common.Hash{}, err
	}
	descriptor, err := contract.TournamentDescriptor(&bind.CallOpts{Context: ctx})
	if err != nil {
		return common.Hash{}, common.Hash{}, fmt.Errorf("reading the tournament descriptor: %w", err)
	}
	bond, err := contract.BondValue(&bind.CallOpts{Context: ctx})
	if err != nil {
		return common.Hash{}, common.Hash{}, fmt.Errorf("reading the bond value: %w", err)
	}
	finalState, proof, left, right := fakeCommitmentNodes(descriptor.Height)
	opts, err := c.transactor(ctx, account)
	if err != nil {
		return common.Hash{}, common.Hash{}, err
	}
	opts.GasLimit = 0 // estimate: the join also creates a match
	opts.Value = bond
	tx, err := contract.JoinTournament(opts, finalState, proof, left, right)
	if err != nil {
		return common.Hash{}, common.Hash{}, fmt.Errorf("sending joinTournament: %w", err)
	}
	return crypto.Keccak256Hash(left.Bytes(), right.Bytes()), tx.Hash(), nil
}

type bondRecovery struct {
	Commitment common.Hash    `json:"commitment"`
	Claimer    common.Address `json:"claimer"`
	Payment    *big.Int       `json:"payment"`
	Burned     *big.Int       `json:"burned"`
	Block      uint64         `json:"block"`
	Tx         common.Hash    `json:"transaction_hash"`
}

func (c *chain) bondRecoveries(ctx context.Context, tournament common.Address, to uint64) ([]bondRecovery, error) {
	contract, err := itournament.NewITournament(tournament, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterBondRecovered(filterTo(ctx, 0, to), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("reading BondRecovered: %w", err)
	}
	defer it.Close()
	var recoveries []bondRecovery
	for it.Next() {
		e := it.Event
		recoveries = append(recoveries, bondRecovery{Commitment: e.Commitment, Claimer: e.Claimer, Payment: e.Payment,
			Burned: e.Burned, Block: e.Raw.BlockNumber, Tx: e.Raw.TxHash})
	}
	return recoveries, it.Error()
}

// chainEvent locates a log.
type chainEvent struct {
	Block    uint64      `json:"block"`
	Tx       common.Hash `json:"transaction_hash"`
	LogIndex uint        `json:"log_index"`
}

func eventOf(log types.Log) chainEvent {
	return chainEvent{Block: log.BlockNumber, Tx: log.TxHash, LogIndex: log.Index}
}

// foreclosure returns the Foreclosure event of an application, or nil.
func (c *chain) foreclosure(ctx context.Context, app common.Address, to uint64) (*chainEvent, error) {
	contract, err := iapplication.NewIApplication(app, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterForeclosure(filterTo(ctx, 0, to))
	if err != nil {
		return nil, fmt.Errorf("reading Foreclosure: %w", err)
	}
	defer it.Close()
	var found *chainEvent
	for it.Next() {
		event := eventOf(it.Event.Raw)
		found = &event
	}
	return found, it.Error()
}

// driveProof is an AccountsDriveMerkleRootProved event.
type driveProof struct {
	chainEvent
	Root common.Hash `json:"root"`
}

func (c *chain) driveProof(ctx context.Context, app common.Address, to uint64) (*driveProof, error) {
	contract, err := iapplication.NewIApplication(app, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterAccountsDriveMerkleRootProved(filterTo(ctx, 0, to))
	if err != nil {
		return nil, fmt.Errorf("reading AccountsDriveMerkleRootProved: %w", err)
	}
	defer it.Close()
	var found *driveProof
	for it.Next() {
		found = &driveProof{chainEvent: eventOf(it.Event.Raw), Root: it.Event.AccountsDriveMerkleRoot}
	}
	return found, it.Error()
}

// chainWithdrawal is a Withdrawal event.
type chainWithdrawal struct {
	chainEvent
	AccountIndex uint64
	Account      []byte
	Output       []byte
}

func (c *chain) withdrawals(ctx context.Context, app common.Address, to uint64) ([]chainWithdrawal, error) {
	contract, err := iapplication.NewIApplication(app, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterWithdrawal(filterTo(ctx, 0, to), nil)
	if err != nil {
		return nil, fmt.Errorf("reading Withdrawal: %w", err)
	}
	defer it.Close()
	var withdrawals []chainWithdrawal
	for it.Next() {
		withdrawals = append(withdrawals, chainWithdrawal{chainEvent: eventOf(it.Event.Raw), AccountIndex: it.Event.AccountIndex,
			Account: it.Event.Account, Output: it.Event.Output})
	}
	return withdrawals, it.Error()
}

// refundsIssued returns the RefundIssued events of an application by input index.
func (c *chain) refundsIssued(ctx context.Context, app common.Address, to uint64) (map[uint64]chainEvent, error) {
	contract, err := iapplication.NewIApplication(app, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterRefundIssued(filterTo(ctx, 0, to), nil)
	if err != nil {
		return nil, fmt.Errorf("reading RefundIssued: %w", err)
	}
	defer it.Close()
	refunds := map[uint64]chainEvent{}
	for it.Next() {
		refunds[it.Event.InputIndex.Uint64()] = eventOf(it.Event.Raw)
	}
	return refunds, it.Error()
}

func (c *chain) isForeclosed(ctx context.Context, app common.Address, block uint64) (bool, error) {
	contract, err := iapplication.NewIApplication(app, c.eth)
	if err != nil {
		return false, err
	}
	foreclosed, err := contract.IsForeclosed(callAt(ctx, block))
	if err != nil {
		return false, fmt.Errorf("reading isForeclosed: %w", err)
	}
	return foreclosed, nil
}

// lastFinalizedMachineRoot is the machine state that the accounts-drive proof
// must reproduce: the post-state of the last accepted epoch.
func (c *chain) lastFinalizedMachineRoot(ctx context.Context, consensus, app common.Address, block uint64) (common.Hash, error) {
	contract, err := idaveconsensus.NewIDaveConsensus(consensus, c.eth)
	if err != nil {
		return common.Hash{}, err
	}
	root, err := contract.GetLastFinalizedMachineMerkleRoot(callAt(ctx, block), app)
	if err != nil {
		return common.Hash{}, fmt.Errorf("reading getLastFinalizedMachineMerkleRoot: %w", err)
	}
	return root, nil
}

// chainInput is one InputAdded event.
type chainInput struct {
	Index    uint64
	Raw      []byte
	Block    uint64
	Tx       common.Hash
	LogIndex uint
}

func (c *chain) inputsAdded(ctx context.Context, inputBox, app common.Address, to uint64) ([]chainInput, error) {
	contract, err := iinputbox.NewIInputBox(inputBox, c.eth)
	if err != nil {
		return nil, err
	}
	it, err := contract.FilterInputAdded(filterTo(ctx, 0, to), []common.Address{app}, nil)
	if err != nil {
		return nil, fmt.Errorf("reading InputAdded: %w", err)
	}
	defer it.Close()
	var inputs []chainInput
	for it.Next() {
		e := it.Event
		inputs = append(inputs, chainInput{Index: e.Index.Uint64(), Raw: e.Input, Block: e.Raw.BlockNumber, Tx: e.Raw.TxHash,
			LogIndex: e.Raw.Index})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Index < inputs[j].Index })
	return inputs, it.Error()
}

// partialRefunds returns the total that a tournament paid out as gas
// refunds (successful PartialBondRefund events), and to whom.
func (c *chain) partialRefunds(ctx context.Context, tournament common.Address, to uint64,
) (*big.Int, map[common.Address]*big.Int, error) {
	contract, err := itournament.NewITournament(tournament, c.eth)
	if err != nil {
		return nil, nil, err
	}
	it, err := contract.FilterPartialBondRefund(filterTo(ctx, 0, to), nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("reading PartialBondRefund: %w", err)
	}
	defer it.Close()
	total, byRecipient := new(big.Int), map[common.Address]*big.Int{}
	for it.Next() {
		if !it.Event.Success {
			continue // the value stays in the tournament
		}
		total.Add(total, it.Event.Value)
		if byRecipient[it.Event.Recipient] == nil {
			byRecipient[it.Event.Recipient] = new(big.Int)
		}
		byRecipient[it.Event.Recipient].Add(byRecipient[it.Event.Recipient], it.Event.Value)
	}
	return total, byRecipient, it.Error()
}

func (c *chain) bondValue(ctx context.Context, tournament common.Address, block uint64) (*big.Int, error) {
	contract, err := itournament.NewITournament(tournament, c.eth)
	if err != nil {
		return nil, err
	}
	bond, err := contract.BondValue(callAt(ctx, block))
	if err != nil {
		return nil, fmt.Errorf("reading the bond value of %s: %w", tournament, err)
	}
	return bond, nil
}

func (c *chain) balanceAt(ctx context.Context, account common.Address, block uint64) (*big.Int, error) {
	balance, err := c.eth.BalanceAt(ctx, account, new(big.Int).SetUint64(block))
	if err != nil {
		return nil, fmt.Errorf("reading the balance of %s at block %d: %w", account, block, err)
	}
	return balance, nil
}

// receiveABI declares a payable receive function. The binding refuses a
// plain transfer to a contract whose ABI does not declare one.
const receiveABI = `[{"type":"receive","stateMutability":"payable"}]`

// sendValue sends a plain ETH transfer to an account or to a contract with a
// receive function.
func (c *chain) sendValue(ctx context.Context, account *testAccount, to common.Address, value *big.Int) (common.Hash, error) {
	parsed, err := abi.JSON(strings.NewReader(receiveABI))
	if err != nil {
		return common.Hash{}, err
	}
	opts, err := c.transactor(ctx, account)
	if err != nil {
		return common.Hash{}, err
	}
	opts.Value = value
	tx, err := bind.NewBoundContract(to, parsed, c.eth, c.eth, c.eth).Transfer(opts)
	if err != nil {
		return common.Hash{}, fmt.Errorf("sending %s wei to %s: %w", value, to, err)
	}
	return tx.Hash(), nil
}

// gasCost is what a mined transaction cost its sender in gas.
func gasCost(receipt *types.Receipt) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
}

func (c *chain) inputCount(ctx context.Context, inputBox, app common.Address, block uint64) (uint64, error) {
	contract, err := iinputbox.NewIInputBox(inputBox, c.eth)
	if err != nil {
		return 0, err
	}
	count, err := contract.GetNumberOfInputs(callAt(ctx, block), app)
	if err != nil {
		return 0, fmt.Errorf("reading InputBox counter at block %d: %w", block, err)
	}
	return count.Uint64(), nil
}

// txSender returns the sender of a mined transaction.
func (c *chain) txSender(ctx context.Context, hash common.Hash) (common.Address, error) {
	tx, _, err := c.eth.TransactionByHash(ctx, hash)
	if err != nil {
		return common.Address{}, fmt.Errorf("reading transaction %s: %w", hash, err)
	}
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return common.Address{}, fmt.Errorf("recovering sender of %s: %w", hash, err)
	}
	return sender, nil
}

// waitReceipt waits for a mined receipt and requires success.
func (c *chain) waitReceipt(ctx context.Context, hash common.Hash, timeout time.Duration) (*types.Receipt, error) {
	deadline := time.Now().Add(timeout)
	for {
		receipt, err := c.eth.TransactionReceipt(ctx, hash)
		if err == nil {
			if receipt.Status != types.ReceiptStatusSuccessful {
				return receipt, fmt.Errorf("transaction %s reverted", hash)
			}
			return receipt, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("no receipt for %s within %s: %w", hash, timeout, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(receiptPollInterval):
		}
	}
}

// Minimal ERC-20 surface for the honeypot case. The devnet TestFungibleToken
// has an open mint(uint256) that mints to the caller.
const erc20ABI = `[
 {"type":"function","name":"mint","inputs":[{"name":"value","type":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},
 {"type":"function","name":"balanceOf","inputs":[{"name":"account","type":"address"}],
  "outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"}
]`

var transferEventTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

func erc20Contract(address common.Address, backend bind.ContractBackend) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, backend, backend, backend), nil
}

func (c *chain) tokenBalance(ctx context.Context, token, account common.Address) (*big.Int, error) {
	contract, err := erc20Contract(token, c.eth)
	if err != nil {
		return nil, err
	}
	var out []any
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &out, "balanceOf", account); err != nil {
		return nil, fmt.Errorf("reading token balance: %w", err)
	}
	balance, ok := out[0].(*big.Int)
	if !ok {
		return nil, errors.New("unexpected balanceOf result")
	}
	return balance, nil
}

func (c *chain) transactor(ctx context.Context, account *testAccount) (*bind.TransactOpts, error) {
	chainID, err := c.chainID(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireTestChain(chainID); err != nil {
		return nil, err
	}
	key, err := crypto.ToECDSA(account.key)
	if err != nil {
		return nil, err
	}
	opts, err := bind.NewKeyedTransactorWithChainID(key, new(big.Int).SetUint64(chainID))
	if err != nil {
		return nil, err
	}
	opts.Context = ctx
	opts.GasLimit = gasLimitSimpleTx
	return opts, nil
}

func (c *chain) mintToken(ctx context.Context, token common.Address, account *testAccount, amount *big.Int) (common.Hash, error) {
	contract, err := erc20Contract(token, c.eth)
	if err != nil {
		return common.Hash{}, err
	}
	opts, err := c.transactor(ctx, account)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := contract.Transact(opts, "mint", amount)
	if err != nil {
		return common.Hash{}, fmt.Errorf("sending mint: %w", err)
	}
	return tx.Hash(), nil
}

func (c *chain) addInput(ctx context.Context, inputBox, app common.Address, account *testAccount, payload []byte) (common.Hash, error) {
	contract, err := iinputbox.NewIInputBox(inputBox, c.eth)
	if err != nil {
		return common.Hash{}, err
	}
	opts, err := c.transactor(ctx, account)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := contract.AddInput(opts, app, payload)
	if err != nil {
		return common.Hash{}, fmt.Errorf("sending addInput: %w", err)
	}
	return tx.Hash(), nil
}

// parseOutputExecuted finds the OutputExecuted log of app in a receipt.
func parseOutputExecuted(receipt *types.Receipt, app common.Address) (*iapplication.IApplicationOutputExecuted, error) {
	filterer, err := iapplication.NewIApplicationFilterer(app, nil)
	if err != nil {
		return nil, err
	}
	for _, log := range receipt.Logs {
		if log.Address != app {
			continue
		}
		if event, err := filterer.ParseOutputExecuted(*log); err == nil {
			return event, nil
		}
	}
	return nil, errors.New("receipt has no OutputExecuted log from the application")
}

// findTransfer finds an ERC-20 Transfer log of token to recipient.
func findTransfer(receipt *types.Receipt, token, recipient common.Address) (*big.Int, bool) {
	for _, log := range receipt.Logs {
		if log.Address != token || len(log.Topics) != 3 || log.Topics[0] != transferEventTopic {
			continue
		}
		if common.BytesToAddress(log.Topics[2].Bytes()) == recipient {
			return new(big.Int).SetBytes(log.Data), true
		}
	}
	return nil, false
}
