// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cartesi/rollups-node/internal/model"
)

const (
	verifyPollInterval = 3 * time.Second
	verifyLogInterval  = 15 * time.Second
	// verifyStableDelay separates the two snapshots that must agree before a
	// complete snapshot is evaluated: several polling intervals of the node.
	verifyStableDelay = 10 * time.Second
)

type checkStatus string

const (
	checkPass      checkStatus = "pass"
	checkFail      checkStatus = "fail"
	checkNotTested checkStatus = "not-tested"
	// checkFinding reports a defect of the Sling node that the run observed;
	// it does not fail the run.
	checkFinding checkStatus = "finding"
)

// Check names that other code refers to.
const (
	checkNameAgreement       = "agreement.state_change"
	checkNameInputs          = "inputs"
	checkNameSealed          = "epochs.sealed"
	checkNameStaged          = "epochs.staged"
	checkNameAccepted        = "epochs.accepted"
	checkNameTournaments     = "tournaments"
	checkNameJoins           = "tournaments.commitments"
	checkNameMatches         = "tournaments.matches"
	checkNameSlingCommitment = "commitments.sling"
	checkNameSlingFinalState = "final_states.sling"
)

type checkResult struct {
	Name   string      `json:"name"`
	Status checkStatus `json:"status"`
	Detail string      `json:"detail"`
	Items  []string    `json:"items,omitempty"`
}

// fail records one mismatch.
func (r *checkResult) fail(format string, args ...any) {
	r.Items = append(r.Items, fmt.Sprintf(format, args...))
}

// settle sets the status: not tested when nothing was compared, failed when
// any item was recorded.
func (r *checkResult) settle(compared int) checkResult {
	switch {
	case len(r.Items) > 0:
		r.Status = checkFail
	case compared == 0:
		r.Status = checkNotTested
	default:
		r.Status = checkPass
	}
	return *r
}

type verifyReport struct {
	Application string         `json:"application"`
	Address     common.Address `json:"address"`
	Head        uint64         `json:"head"`
	Passed      bool           `json:"passed"`
	// Gaps are completion predicates that were still missing at the deadline.
	Gaps     []string       `json:"gaps,omitempty"`
	Checks   []checkResult  `json:"checks"`
	Coverage []coverageItem `json:"coverage"`
}

// Coverage names. The plan calls them C1 and C2.
const (
	// coverageAgreement (C1): the rollups node computed the same result as the
	// Sling node for at least one epoch that changed the machine state.
	coverageAgreement = "computation agreement"
	// coverageTournaments (C2): the rollups node indexed every sealed epoch,
	// tournament, commitment, and match that the chain has.
	coverageTournaments = "tournament indexing"
)

// coverageItem tells whether a run exercised one property at all. A check can
// pass without testing anything; coverage says so.
type coverageItem struct {
	Name   string `json:"name"`
	Tested bool   `json:"tested"`
	Detail string `json:"detail"`
}

// verifyTarget names what to verify and the independent expectations.
type verifyTarget struct {
	chain            *chain
	api              *nodeAPI
	app              string // name or address, as registered in the node
	appAddress       common.Address
	consensus        common.Address
	inputBox         common.Address
	claims           map[uint64]common.Hash
	expectedStatus   model.ApplicationStatus
	requireAgreement bool
	// slingSigner is the Sling node's signer. Its root-tournament joins and
	// sentry claims are chain evidence of its own computation.
	slingSigner common.Address
	// rollupsSigner is the rollups node's PRT signer, when known. It must
	// differ from slingSigner, or the "evidence" is the node's own.
	rollupsSigner common.Address
	// atBlock pins chain reads; zero means the current head on each attempt.
	// verify needs a chain that does not move: the node must reach the block.
	atBlock uint64
	// watch reports a participant that stopped; the wait then ends at once.
	watch func() error
	// pinned caches the chain side while atBlock pins it.
	pinned *chainFacts
}

// chainFacts is the chain side of a snapshot, read with the contract bindings.
type chainFacts struct {
	Head uint64
	// ChainConsensus and ChainTemplate are what the application contract says.
	ChainConsensus   common.Address
	ChainTemplate    common.Hash
	ChainInputCount  uint64
	ChainInputs      []chainInput
	Sealed           []sealedEpoch
	Staged           map[uint64]stagedEpoch
	ChainTournaments map[common.Address]chainTournament
	ChainJoins       map[common.Address][]commitmentJoin
	ChainMatches     []chainMatch
	Standings        map[uint64]rootStanding
	// SlingJoins and SlingSentry hold the Sling node's own values.
	SlingJoins  map[uint64]commitmentJoin
	SlingSentry map[uint64]common.Hash
	// Foreclosure, DriveProof, and ChainWithdrawals are the post-foreclosure
	// facts of the application; nil or empty before foreclosure.
	Foreclosure      *chainEvent
	DriveProof       *driveProof
	ChainWithdrawals []chainWithdrawal
}

// nodeFacts is the rollups node side of a snapshot, read through its API.
type nodeFacts struct {
	App         *model.Application
	Inputs      []model.Input
	Epochs      map[uint64]model.Epoch
	Tournaments map[common.Address]model.Tournament
	Commitments []model.Commitment
	Matches     []model.Match
	Withdrawals []model.Withdrawal
}

// verifyFacts is one snapshot of both sides.
type verifyFacts struct {
	chainFacts
	nodeFacts
}

func collectFacts(ctx context.Context, t *verifyTarget) (*verifyFacts, error) {
	head := t.atBlock
	if head == 0 {
		header, err := t.chain.head(ctx)
		if err != nil {
			return nil, err
		}
		head = header.Number.Uint64()
	}
	node, err := collectNodeFacts(ctx, t)
	if err != nil {
		return nil, err
	}
	onChain := t.pinned
	if onChain == nil || onChain.Head != head {
		if onChain, err = collectChainFacts(ctx, t, head); err != nil {
			return nil, err
		}
		if t.atBlock != 0 {
			t.pinned = onChain
		}
	}
	return &verifyFacts{chainFacts: *onChain, nodeFacts: *node}, nil
}

func collectNodeFacts(ctx context.Context, t *verifyTarget) (*nodeFacts, error) {
	f := &nodeFacts{Epochs: map[uint64]model.Epoch{}, Tournaments: map[common.Address]model.Tournament{}}
	var err error
	if f.App, err = t.api.application(ctx, t.app); err != nil {
		return nil, err
	}
	if f.Inputs, err = t.api.inputs(ctx, t.app); err != nil {
		return nil, err
	}
	epochs, err := t.api.epochs(ctx, t.app)
	if err != nil {
		return nil, err
	}
	for _, epoch := range epochs {
		f.Epochs[epoch.Index] = epoch
	}
	tournaments, err := t.api.tournaments(ctx, t.app)
	if err != nil {
		return nil, err
	}
	for _, tournament := range tournaments {
		f.Tournaments[tournament.Address] = tournament
	}
	if f.Commitments, err = t.api.commitments(ctx, t.app); err != nil {
		return nil, err
	}
	if f.Matches, err = t.api.matches(ctx, t.app); err != nil {
		return nil, err
	}
	if f.Withdrawals, err = t.api.withdrawals(ctx, t.app); err != nil {
		return nil, err
	}
	return f, nil
}

func collectChainFacts(ctx context.Context, t *verifyTarget, head uint64) (*chainFacts, error) {
	c := t.chain
	f := &chainFacts{Head: head, ChainJoins: map[common.Address][]commitmentJoin{}, Standings: map[uint64]rootStanding{},
		SlingJoins: map[uint64]commitmentJoin{}, SlingSentry: map[uint64]common.Hash{}}
	var err error
	if f.ChainConsensus, f.ChainTemplate, err = c.applicationFacts(ctx, t.appAddress, head); err != nil {
		return nil, err
	}
	if f.ChainInputCount, err = c.inputCount(ctx, t.inputBox, t.appAddress, head); err != nil {
		return nil, err
	}
	if f.ChainInputs, err = c.inputsAdded(ctx, t.inputBox, t.appAddress, head); err != nil {
		return nil, err
	}
	if f.Sealed, err = c.sealedEpochs(ctx, t.consensus, head); err != nil {
		return nil, err
	}
	if f.Staged, err = c.stagedEpochs(ctx, t.consensus, head); err != nil {
		return nil, err
	}
	roots := make(map[common.Address]uint64, len(f.Sealed))
	for _, sealed := range f.Sealed {
		roots[sealed.Tournament] = sealed.Index
		if f.Standings[sealed.Index], err = c.rootStanding(ctx, sealed.Tournament, head); err != nil {
			return nil, err
		}
	}
	if f.ChainTournaments, err = c.tournamentTree(ctx, roots, head); err != nil {
		return nil, err
	}
	for address := range f.ChainTournaments {
		if f.ChainJoins[address], err = c.joins(ctx, address, head); err != nil {
			return nil, err
		}
		matches, err := c.matches(ctx, address, head)
		if err != nil {
			return nil, err
		}
		f.ChainMatches = append(f.ChainMatches, matches...)
	}
	if f.Foreclosure, err = c.foreclosure(ctx, t.appAddress, head); err != nil {
		return nil, err
	}
	if f.DriveProof, err = c.driveProof(ctx, t.appAddress, head); err != nil {
		return nil, err
	}
	if f.ChainWithdrawals, err = c.withdrawals(ctx, t.appAddress, head); err != nil {
		return nil, err
	}
	if t.slingSigner != (common.Address{}) {
		for _, sealed := range f.Sealed {
			for _, join := range f.ChainJoins[sealed.Tournament] {
				if join.Submitter == t.slingSigner {
					f.SlingJoins[sealed.Index] = join
				}
			}
		}
		if f.SlingSentry, err = c.sentryClaims(ctx, t.consensus, t.slingSigner, head); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func epochRank(status model.EpochStatus) int {
	switch status {
	case model.EpochStatus_Open:
		return 0
	case model.EpochStatus_Closed:
		return 1
	case model.EpochStatus_InputsProcessed:
		return 2 //nolint:mnd
	case model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted:
		return 3 //nolint:mnd
	case model.EpochStatus_ClaimStaged:
		return 4 //nolint:mnd
	case model.EpochStatus_ClaimAccepted:
		return 5 //nolint:mnd
	case model.EpochStatus_ClaimRejected, model.EpochStatus_ClaimForeclosed:
		return -1
	}
	return -1
}

// foreclosedEpoch reports whether a node epoch legitimately ended as
// CLAIM_FORECLOSED: the application is foreclosed on chain and the epoch was
// never accepted.
func (f *verifyFacts) foreclosedEpoch(epoch model.Epoch) bool {
	_, accepted := f.acceptance(epoch.Index)
	return f.Foreclosure != nil && epoch.Status == model.EpochStatus_ClaimForeclosed && !accepted
}

// acceptance returns the EpochSealed event of epoch n+1: DaveConsensus seals
// epoch n+1 in the same transaction that accepts epoch n.
func (f *chainFacts) acceptance(n uint64) (sealedEpoch, bool) {
	for _, sealed := range f.Sealed {
		if sealed.Index == n+1 {
			return sealed, true
		}
	}
	return sealedEpoch{}, false
}

func (f *chainFacts) isSealed(n uint64) bool {
	for _, sealed := range f.Sealed {
		if sealed.Index == n {
			return true
		}
	}
	return false
}

// inputEpoch returns the epoch that contains input index: a sealed epoch, or
// the open epoch after the last sealed one.
func (f *chainFacts) inputEpoch(index uint64) (uint64, bool) {
	for _, sealed := range f.Sealed {
		if sealed.Lower <= index && index < sealed.Upper {
			return sealed.Index, true
		}
	}
	if n := len(f.Sealed); n > 0 && index >= f.Sealed[n-1].Upper {
		return f.Sealed[n-1].Index + 1, true
	}
	return 0, false
}

func (f *chainFacts) joinCount() int {
	count := 0
	for _, joins := range f.ChainJoins {
		count += len(joins)
	}
	return count
}

// completionGaps lists the positive work predicates that are not yet true.
// terminal is true when waiting longer cannot help.
func completionGaps(f *verifyFacts, expected model.ApplicationStatus) (gaps []string, terminal bool) {
	if f.App.Status != expected {
		reason := ""
		if f.App.Reason != nil {
			reason = *f.App.Reason
		}
		gaps = append(gaps, fmt.Sprintf("application status is %s, expected %s (%s)", f.App.Status, expected, reason))
		// Only an OK application can still reach another status by itself.
		if f.App.Status != model.ApplicationStatus_OK {
			return gaps, true
		}
	}
	// After foreclosure the input and epoch scans stop at the foreclosure
	// block; tournaments are still observed.
	scanned := f.Head
	if f.Foreclosure != nil {
		scanned = min(f.Head, f.Foreclosure.Block)
	}
	for name, want := range map[string][2]uint64{
		"last_input_check_block":      {f.App.LastInputCheckBlock, scanned},
		"last_epoch_check_block":      {f.App.LastEpochCheckBlock, scanned},
		"last_tournament_check_block": {f.App.LastTournamentCheckBlock, f.Head},
	} {
		if want[0] < want[1] {
			gaps = append(gaps, fmt.Sprintf("%s %d < %d", name, want[0], want[1]))
		}
	}
	if f.Foreclosure != nil && f.App.ForecloseBlock == 0 {
		gaps = append(gaps, "the foreclosure is not indexed")
	}
	if f.DriveProof != nil && f.App.AccountsDriveProvedBlock == 0 {
		gaps = append(gaps, "the accounts-drive proof is not indexed")
	}
	if indexed, total := len(f.Withdrawals), len(f.ChainWithdrawals); indexed < total {
		gaps = append(gaps, fmt.Sprintf("%d of %d withdrawals are indexed", indexed, total))
	}
	if uint64(len(f.Inputs)) < f.ChainInputCount {
		gaps = append(gaps, fmt.Sprintf("the rollups node has %d of %d inputs", len(f.Inputs), f.ChainInputCount))
	}
	if unprocessed := countUnprocessed(f.Inputs); unprocessed > 0 {
		gaps = append(gaps, fmt.Sprintf("%d inputs are not processed", unprocessed))
	}
	for _, sealed := range f.Sealed {
		epoch, ok := f.Epochs[sealed.Index]
		_, accepted := f.acceptance(sealed.Index)
		_, staged := f.Staged[sealed.Index]
		switch {
		case !ok:
			gaps = append(gaps, fmt.Sprintf("sealed epoch %d is missing", sealed.Index))
		case f.foreclosedEpoch(epoch):
		case epochRank(epoch.Status) < epochRank(model.EpochStatus_ClaimComputed):
			gaps = append(gaps, fmt.Sprintf("epoch %d is %s", sealed.Index, epoch.Status))
		case accepted && epoch.Status != model.EpochStatus_ClaimAccepted:
			gaps = append(gaps, fmt.Sprintf("epoch %d is accepted on chain but %s in the rollups node", sealed.Index, epoch.Status))
		case staged && epochRank(epoch.Status) < epochRank(model.EpochStatus_ClaimStaged):
			gaps = append(gaps, fmt.Sprintf("epoch %d is staged on chain but %s in the rollups node", sealed.Index, epoch.Status))
		}
	}
	if missing := countMissing(f.ChainTournaments, f.Tournaments); missing > 0 {
		gaps = append(gaps, fmt.Sprintf("%d tournaments are not indexed", missing))
	}
	if indexed, total := len(f.Commitments), f.joinCount(); indexed < total {
		gaps = append(gaps, fmt.Sprintf("%d of %d commitments are indexed", indexed, total))
	}
	if indexed, total := len(f.Matches), len(f.ChainMatches); indexed < total {
		gaps = append(gaps, fmt.Sprintf("%d of %d matches are indexed", indexed, total))
	}
	sort.Strings(gaps)
	return gaps, false
}

func countUnprocessed(inputs []model.Input) int {
	count := 0
	for _, input := range inputs {
		if input.Status == model.InputCompletionStatus_None {
			count++
		}
	}
	return count
}

func countMissing[V, W any](want map[common.Address]V, have map[common.Address]W) int {
	missing := 0
	for address := range want {
		if _, ok := have[address]; !ok {
			missing++
		}
	}
	return missing
}

// progressText summarizes how far the rollups node is, for the wait log.
func progressText(f *verifyFacts) string {
	computed := 0
	for _, sealed := range f.Sealed {
		if epoch, ok := f.Epochs[sealed.Index]; ok && epochRank(epoch.Status) >= epochRank(model.EpochStatus_ClaimComputed) {
			computed++
		}
	}
	cursor := min(f.App.LastInputCheckBlock, f.App.LastEpochCheckBlock, f.App.LastTournamentCheckBlock)
	return fmt.Sprintf("inputs %d/%d processed, epochs %d/%d computed, tournaments %d/%d indexed, scanned to block %d of %d",
		len(f.Inputs)-countUnprocessed(f.Inputs), f.ChainInputCount, computed, len(f.Sealed),
		len(f.ChainTournaments)-countMissing(f.ChainTournaments, f.Tournaments), len(f.ChainTournaments), cursor, f.Head)
}

// fingerprint summarizes the node state that a later snapshot must repeat.
func fingerprint(f *verifyFacts) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %d %d %d %d|", f.App.Status, len(f.Inputs), len(f.Tournaments), len(f.Commitments), len(f.Matches))
	for _, input := range f.Inputs {
		fmt.Fprintf(&b, "%d:%s ", input.Index, input.Status)
	}
	for _, index := range sortedKeys(f.Epochs) {
		epoch := f.Epochs[index]
		fmt.Fprintf(&b, "|%d:%s:%s:%s:%s", index, epoch.Status, hashText(epoch.Commitment), hashText(epoch.MachineHash),
			blockText(epoch.StagedAtBlock))
	}
	return b.String()
}

// textNone stands for a missing optional value, and for "no signer";
// textUnknown for a value that could not be read.
const (
	textNone    = "none"
	textUnknown = "unknown"
)

func hashText(hash *common.Hash) string {
	if hash == nil {
		return textNone
	}
	return hash.Hex()
}

func blockText(block *uint64) string {
	if block == nil {
		return textNone
	}
	return fmt.Sprint(*block)
}

// evaluate runs every comparison on one snapshot. It reads nothing, so that it
// can be unit tested.
func evaluate(f *verifyFacts, t *verifyTarget) []checkResult {
	commitments, commitmentMatched := checkSlingCommitments(f, t.claims, t.rollupsSigner)
	finalStates, finalStateMatched := checkSlingFinalStates(f)
	for index := range finalStateMatched {
		commitmentMatched[index] = true
	}
	return []checkResult{
		checkApplicationStatus(f, t.expectedStatus),
		checkApplicationIdentity(f, t),
		checkInputs(f),
		checkSealedEpochs(f),
		checkStagedEpochs(f),
		checkAcceptedEpochs(f),
		commitments,
		finalStates,
		checkRootWinners(f),
		checkTournaments(f),
		checkTournamentCommitments(f),
		checkTournamentMatches(f),
		checkForeclosure(f),
		checkWithdrawals(f),
		checkStateChange(f, commitmentMatched, t.requireAgreement),
	}
}

// checkForeclosure compares the node's view of the foreclosure and of the
// accounts-drive proof with the application's events, both ways, and
// requires every epoch that was never accepted to end as CLAIM_FORECLOSED.
func checkForeclosure(f *verifyFacts) checkResult {
	result := checkResult{Name: "application.foreclosure", Detail: "the application is not foreclosed"}
	switch {
	case f.Foreclosure == nil && f.App.ForecloseBlock != 0:
		result.fail("the rollups node records a foreclosure at block %d that is not on chain", f.App.ForecloseBlock)
		return result.settle(1)
	case f.Foreclosure == nil:
		return result.settle(0)
	}
	e := f.Foreclosure
	if f.App.ForecloseBlock != e.Block || f.App.ForecloseTransaction == nil || *f.App.ForecloseTransaction != e.Tx {
		result.fail("foreclosure: rollups block %d tx %v, chain block %d tx %s", f.App.ForecloseBlock,
			f.App.ForecloseTransaction, e.Block, e.Tx)
	}
	result.Detail = fmt.Sprintf("foreclosed at block %d", e.Block)
	var drained []uint64
	for _, index := range sortedKeys(f.Epochs) {
		epoch := f.Epochs[index]
		if _, accepted := f.acceptance(index); accepted {
			continue
		}
		if epoch.Status != model.EpochStatus_ClaimForeclosed {
			result.fail("epoch %d is %s; an epoch never accepted must end as CLAIM_FORECLOSED", index, epoch.Status)
			continue
		}
		drained = append(drained, index)
	}
	sealed := map[uint64]bool{}
	for _, epoch := range f.Sealed {
		sealed[epoch.Index] = true
	}
	var closedSealed, closedOpen []uint64
	for _, index := range drained {
		if sealed[index] {
			closedSealed = append(closedSealed, index)
		} else {
			closedOpen = append(closedOpen, index)
		}
	}
	switch {
	case len(drained) == 0:
		result.Detail += "; no epoch left to close"
	case len(closedOpen) == 0:
		result.Detail += fmt.Sprintf("; CLAIM_FORECLOSED: %s (sealed, never accepted)", epochsText(closedSealed))
	case len(closedSealed) == 0:
		result.Detail += fmt.Sprintf("; CLAIM_FORECLOSED: %s (open at the foreclosure)", epochsText(closedOpen))
	default:
		result.Detail += fmt.Sprintf("; CLAIM_FORECLOSED: %s (sealed, never accepted) and %s (open at the foreclosure)",
			epochsText(closedSealed), epochsText(closedOpen))
	}
	switch p := f.DriveProof; {
	case p == nil && f.App.AccountsDriveProvedBlock != 0:
		result.fail("the rollups node records an accounts-drive proof that is not on chain")
	case p == nil:
		result.Detail += "; accounts drive not proved"
	case f.App.AccountsDriveProvedBlock != p.Block || f.App.AccountsDriveProvedTransaction == nil ||
		*f.App.AccountsDriveProvedTransaction != p.Tx || f.App.AccountsDriveMerkleRoot == nil ||
		*f.App.AccountsDriveMerkleRoot != p.Root:
		result.fail("accounts-drive proof: rollups block %d root %v, chain block %d root %s", f.App.AccountsDriveProvedBlock,
			f.App.AccountsDriveMerkleRoot, p.Block, p.Root)
	default:
		result.Detail += fmt.Sprintf("; accounts drive proved at block %d", p.Block)
	}
	return result.settle(1)
}

// checkWithdrawals compares every Withdrawal event with the node's
// withdrawals, both ways.
func checkWithdrawals(f *verifyFacts) checkResult {
	result := checkResult{Name: "withdrawals",
		Detail: fmt.Sprintf("chain %d, rollups node %d", len(f.ChainWithdrawals), len(f.Withdrawals))}
	indexed := make(map[uint64]model.Withdrawal, len(f.Withdrawals))
	for _, withdrawal := range f.Withdrawals {
		indexed[withdrawal.AccountIndex] = withdrawal
	}
	onChain := map[uint64]bool{}
	for _, want := range f.ChainWithdrawals {
		onChain[want.AccountIndex] = true
		got, ok := indexed[want.AccountIndex]
		switch {
		case !ok:
			result.fail("account %d: not indexed", want.AccountIndex)
		case !bytes.Equal(got.Account, want.Account) || !bytes.Equal(got.Output, want.Output):
			result.fail("account %d: account or output differs from the Withdrawal event", want.AccountIndex)
		case got.BlockNumber != want.Block || got.TransactionHash != want.Tx || got.LogIndex != want.LogIndex:
			result.fail("account %d: block %d tx %s, chain block %d tx %s", want.AccountIndex, got.BlockNumber,
				got.TransactionHash, want.Block, want.Tx)
		}
	}
	for index := range indexed {
		if !onChain[index] {
			result.fail("account %d: indexed but not on chain", index)
		}
	}
	return result.settle(len(onChain))
}

func checkApplicationStatus(f *verifyFacts, expected model.ApplicationStatus) checkResult {
	result := checkResult{Name: "application.status", Detail: fmt.Sprintf("status %s (expected %s)", f.App.Status, expected)}
	if f.App.Status != expected {
		reason := "no reason recorded"
		if f.App.Reason != nil {
			reason = *f.App.Reason
		}
		result.fail("%s", reason)
	}
	return result.settle(1)
}

// checkApplicationIdentity checks that the node and the chain describe the
// application that the run targets: the same contracts and template.
func checkApplicationIdentity(f *verifyFacts, t *verifyTarget) checkResult {
	result := checkResult{Name: "application.identity",
		Detail: fmt.Sprintf("application %s, consensus %s", t.appAddress, t.consensus)}
	if f.App.IApplicationAddress != t.appAddress {
		result.fail("the rollups node's application is %s", f.App.IApplicationAddress)
	}
	if f.App.IConsensusAddress != t.consensus {
		result.fail("the rollups node's consensus is %s", f.App.IConsensusAddress)
	}
	if f.App.ConsensusType != model.Consensus_PRT {
		result.fail("the rollups node's consensus type is %s, not PRT", f.App.ConsensusType)
	}
	if f.ChainConsensus != t.consensus {
		result.fail("the application's outputs Merkle root validator on chain is %s", f.ChainConsensus)
	}
	if f.App.TemplateHash != f.ChainTemplate {
		result.fail("template hash: rollups %s, chain %s", f.App.TemplateHash, f.ChainTemplate)
	}
	return result.settle(1)
}

// checkInputs compares every InputAdded event with the node's input, in both
// directions, and requires every input to be processed.
func checkInputs(f *verifyFacts) checkResult {
	byIndex := make(map[uint64]model.Input, len(f.Inputs))
	for _, input := range f.Inputs {
		byIndex[input.Index] = input
	}
	result := checkResult{Name: checkNameInputs,
		Detail: fmt.Sprintf("chain %d, rollups node %d: %s", f.ChainInputCount, len(f.Inputs), inputCounts(f.Inputs))}
	if uint64(len(f.ChainInputs)) != f.ChainInputCount {
		result.fail("the chain has %d InputAdded events but counts %d inputs", len(f.ChainInputs), f.ChainInputCount)
	}
	onChain := make(map[uint64]bool, len(f.ChainInputs))
	for _, want := range f.ChainInputs {
		onChain[want.Index] = true
		input, ok := byIndex[want.Index]
		if !ok {
			result.fail("input %d is not indexed", want.Index)
			continue
		}
		epoch, known := f.inputEpoch(want.Index)
		switch {
		case !bytes.Equal(input.RawData, want.Raw):
			result.fail("input %d: the payload differs from InputAdded", want.Index)
		case input.BlockNumber != want.Block || input.TransactionHash != want.Tx || input.LogIndex != uint64(want.LogIndex):
			result.fail("input %d: block %d tx %s log %d, chain block %d tx %s log %d", want.Index, input.BlockNumber,
				input.TransactionHash, input.LogIndex, want.Block, want.Tx, want.LogIndex)
		case !known || input.EpochIndex != epoch:
			result.fail("input %d is in epoch %d; the chain puts it in epoch %d", want.Index, input.EpochIndex, epoch)
		case input.Status == model.InputCompletionStatus_None:
			result.fail("input %d is not processed", want.Index)
		}
	}
	for _, input := range f.Inputs {
		if !onChain[input.Index] {
			result.fail("input %d is not on chain", input.Index)
		}
	}
	return result.settle(len(f.ChainInputs))
}

// inputCounts is the Inputs column, for example "2 accepted, 2 rejected".
func inputCounts(inputs []model.Input) string {
	if len(inputs) == 0 {
		return "none"
	}
	counts := map[model.InputCompletionStatus]int{}
	var order []model.InputCompletionStatus
	for _, input := range inputs {
		if counts[input.Status] == 0 {
			order = append(order, input.Status)
		}
		counts[input.Status]++
	}
	parts := make([]string, 0, len(order))
	for _, status := range order {
		parts = append(parts, fmt.Sprintf("%d %s", counts[status], strings.ToLower(string(status))))
	}
	return strings.Join(parts, ", ")
}

// checkSealedEpochs compares sealed epochs in both directions: every sealed
// epoch is in the node, and every node epoch is sealed, except the open one.
func checkSealedEpochs(f *verifyFacts) checkResult {
	result := checkResult{Name: checkNameSealed,
		Detail: fmt.Sprintf("%d sealed epochs on chain (%s)", len(f.Sealed), epochsText(f.sealedIndices()))}
	if len(f.Sealed) == 0 {
		// Deployment seals epoch 0, so a PRT application always has one.
		result.fail("no EpochSealed events: wrong consensus address, or the chain lacks the deployment")
		return result.settle(0)
	}
	for _, sealed := range f.Sealed {
		epoch, ok := f.Epochs[sealed.Index]
		switch {
		case !ok:
			result.fail("epoch %d is missing", sealed.Index)
		case epoch.InputIndexLowerBound != sealed.Lower || epoch.InputIndexUpperBound != sealed.Upper:
			result.fail("epoch %d bounds: rollups [%d,%d), chain [%d,%d)", sealed.Index,
				epoch.InputIndexLowerBound, epoch.InputIndexUpperBound, sealed.Lower, sealed.Upper)
		case epoch.LastBlock != sealed.Block:
			result.fail("epoch %d last block: rollups %d, EpochSealed at %d", sealed.Index, epoch.LastBlock, sealed.Block)
		case epoch.TournamentAddress == nil || *epoch.TournamentAddress != sealed.Tournament:
			result.fail("epoch %d tournament: rollups %v, chain %s", sealed.Index, epoch.TournamentAddress, sealed.Tournament)
		case f.foreclosedEpoch(epoch):
		case epoch.Commitment == nil || epoch.MachineHash == nil:
			result.fail("epoch %d has no computed commitment (%s)", sealed.Index, epoch.Status)
		}
	}
	last := f.Sealed[len(f.Sealed)-1]
	for _, index := range sortedKeys(f.Epochs) {
		epoch := f.Epochs[index]
		switch {
		case f.isSealed(index):
		case index != last.Index+1:
			result.fail("epoch %d is in the rollups node but not sealed on chain", index)
		case epoch.Status != model.EpochStatus_Open && !f.foreclosedEpoch(epoch):
			result.fail("open epoch %d is %s", index, epoch.Status)
		case epoch.InputIndexLowerBound != last.Upper || epoch.InputIndexUpperBound != f.ChainInputCount:
			result.fail("open epoch %d bounds: rollups [%d,%d), chain [%d,%d)", index, epoch.InputIndexLowerBound,
				epoch.InputIndexUpperBound, last.Upper, f.ChainInputCount)
		}
	}
	return result.settle(len(f.Sealed))
}

// checkStagedEpochs compares staged epochs in both directions.
func checkStagedEpochs(f *verifyFacts) checkResult {
	result := checkResult{Name: checkNameStaged,
		Detail: fmt.Sprintf("%d staged epochs on chain (%s)", len(f.Staged), epochsText(sortedKeys(f.Staged)))}
	for _, index := range sortedKeys(f.Staged) {
		staged := f.Staged[index]
		epoch, ok := f.Epochs[index]
		switch {
		case !ok:
			result.fail("epoch %d is missing", index)
		case epoch.MachineHash == nil || *epoch.MachineHash != staged.FinalState:
			result.fail("epoch %d final state: rollups %v, chain %s", index, epoch.MachineHash, staged.FinalState)
		case epoch.TxBufferDataBlock == nil || *epoch.TxBufferDataBlock != staged.OutputsRoot:
			result.fail("epoch %d outputs root: rollups %v, chain %s", index, epoch.TxBufferDataBlock, staged.OutputsRoot)
		case epoch.StagedAtBlock != nil && *epoch.StagedAtBlock != staged.Block:
			result.fail("epoch %d staged at block: rollups %d, chain %d", index, *epoch.StagedAtBlock, staged.Block)
		}
	}
	compared := len(f.Staged)
	for _, index := range sortedKeys(f.Epochs) {
		epoch := f.Epochs[index]
		if epoch.Status != model.EpochStatus_ClaimStaged && epoch.StagedAtBlock == nil {
			continue
		}
		compared++
		if _, ok := f.Staged[index]; !ok {
			result.fail("epoch %d is %s (staged at %s) in the rollups node but not staged on chain", index, epoch.Status,
				blockText(epoch.StagedAtBlock))
		}
	}
	return result.settle(compared)
}

// checkAcceptedEpochs compares accepted epochs in both directions. The node
// records the accepting transaction as the claim transaction.
func checkAcceptedEpochs(f *verifyFacts) checkResult {
	result := checkResult{Name: checkNameAccepted}
	var accepted, open []uint64
	for _, sealed := range f.Sealed {
		acceptance, ok := f.acceptance(sealed.Index)
		if !ok {
			open = append(open, sealed.Index)
			continue
		}
		accepted = append(accepted, sealed.Index)
		epoch, ok := f.Epochs[sealed.Index]
		switch {
		case !ok || epoch.Status != model.EpochStatus_ClaimAccepted:
			result.fail("epoch %d is accepted on chain but %s in the rollups node", sealed.Index, epoch.Status)
		case epoch.ClaimTransactionHash == nil || *epoch.ClaimTransactionHash != acceptance.Tx:
			result.fail("epoch %d claim transaction: rollups %v, accepting transaction %s", sealed.Index,
				epoch.ClaimTransactionHash, acceptance.Tx)
		}
	}
	for _, index := range sortedKeys(f.Epochs) {
		if _, ok := f.acceptance(index); !ok && f.Epochs[index].Status == model.EpochStatus_ClaimAccepted {
			result.fail("epoch %d is CLAIM_ACCEPTED in the rollups node but not accepted on chain", index)
		}
	}
	result.Detail = fmt.Sprintf("%d epochs accepted on chain (%s)", len(accepted), epochsText(accepted))
	switch {
	case len(open) > 0 && f.Foreclosure != nil:
		result.Detail += fmt.Sprintf("; %s sealed, never to be accepted (foreclosed)", epochsText(open))
	case len(open) > 0:
		result.Detail += fmt.Sprintf("; %s sealed, not accepted yet", epochsText(open))
	}
	return result.settle(len(accepted))
}

// checkSlingCommitments compares the rollups node's commitments with the Sling
// node's: its stage-log claims and its own root-tournament joins. The detail
// accounts for every sealed epoch: an epoch that someone else joined first has
// no Sling commitment on chain (one commitment is joined once), and the Sling
// node logs its commitment only when it stages.
func checkSlingCommitments(f *verifyFacts, claims map[uint64]common.Hash, rollupsSigner common.Address,
) (checkResult, map[uint64]bool) {
	matched := map[uint64]bool{}
	result := checkResult{Name: checkNameSlingCommitment}
	reference := map[uint64]common.Hash{}
	for index, claim := range claims {
		reference[index] = claim
	}
	for index, join := range f.SlingJoins {
		if claim, ok := reference[index]; ok && claim != join.Commitment {
			result.fail("epoch %d: the Sling stage log claims %s, but the Sling node joined %s", index, claim, join.Commitment)
			continue
		}
		reference[index] = join.Commitment
	}
	for _, index := range sortedKeys(reference) {
		epoch, ok := f.Epochs[index]
		switch {
		case !ok || epoch.Commitment == nil:
			result.fail("epoch %d: no rollups commitment; sling %s", index, reference[index])
		case *epoch.Commitment != reference[index]:
			result.fail("epoch %d: rollups %s, sling %s", index, epoch.Commitment, reference[index])
		default:
			matched[index] = true
		}
	}
	result.Detail = fmt.Sprintf("%s match (Sling joins: %s; Sling stage log: %s)", epochsText(sortedKeys(matched)),
		indicesText(sortedKeys(f.SlingJoins)), indicesText(sortedKeys(claims)))
	if uncovered := f.uncoveredEpochs(reference, rollupsSigner); uncovered != "" {
		result.Detail += "; not compared: " + uncovered
	}
	return result.settle(len(reference)), matched
}

// uncoveredEpochs explains the sealed epochs that have no Sling commitment,
// grouped by who joined their root tournament.
func (f *verifyFacts) uncoveredEpochs(covered map[uint64]common.Hash, rollupsSigner common.Address) string {
	var byRollups, byOthers, unjoined []uint64
	for _, sealed := range f.Sealed {
		if _, ok := covered[sealed.Index]; ok {
			continue
		}
		joins := f.ChainJoins[sealed.Tournament]
		switch {
		case len(joins) == 0:
			unjoined = append(unjoined, sealed.Index)
		case rollupsSigner != (common.Address{}) && joins[0].Submitter == rollupsSigner:
			byRollups = append(byRollups, sealed.Index)
		default:
			byOthers = append(byOthers, sealed.Index)
		}
	}
	var parts []string
	if len(byRollups) > 0 {
		parts = append(parts, epochsText(byRollups)+", which the rollups node joined first (the Sling node joined no other commitment)")
	}
	if len(byOthers) > 0 {
		parts = append(parts, epochsText(byOthers)+", which another signer joined first")
	}
	if len(unjoined) > 0 {
		parts = append(parts, epochsText(unjoined)+", not joined yet")
	}
	return strings.Join(parts, "; ")
}

// checkSlingFinalStates compares the rollups node's final machine states with
// the Sling node's: the final state of its joins and its sentry claims.
func checkSlingFinalStates(f *verifyFacts) (checkResult, map[uint64]bool) {
	matched := map[uint64]bool{}
	result := checkResult{Name: checkNameSlingFinalState}
	reference := map[uint64]common.Hash{}
	for index, join := range f.SlingJoins {
		reference[index] = join.FinalState
	}
	for index, state := range f.SlingSentry {
		if joined, ok := reference[index]; ok && joined != state {
			result.fail("epoch %d: the Sling node joined with state %s, but its sentry claim is %s", index, joined, state)
			continue
		}
		reference[index] = state
	}
	for _, index := range sortedKeys(reference) {
		epoch, ok := f.Epochs[index]
		switch {
		case !ok || epoch.MachineHash == nil:
			result.fail("epoch %d: no rollups final state; sling %s", index, reference[index])
		case *epoch.MachineHash != reference[index]:
			result.fail("epoch %d: rollups %s, sling %s", index, epoch.MachineHash, reference[index])
		default:
			matched[index] = true
		}
	}
	result.Detail = fmt.Sprintf("%s match (Sling sentry claims: %s; Sling joins: %s)", epochsText(sortedKeys(matched)),
		indicesText(sortedKeys(f.SlingSentry)), indicesText(sortedKeys(f.SlingJoins)))
	var missing []uint64
	for _, index := range f.sealedIndices() {
		if _, ok := reference[index]; !ok {
			missing = append(missing, index)
		}
	}
	if len(missing) > 0 {
		result.Detail += fmt.Sprintf("; not compared: %s, with no Sling sentry claim or join yet", epochsText(missing))
	}
	return result.settle(len(reference)), matched
}

// checkRootWinners requires every finished root to have the rollups node's
// commitment as winner. It detects divergence; it is not evidence of
// computation agreement, because the winner can be the node's own commitment.
func checkRootWinners(f *verifyFacts) checkResult {
	result := checkResult{Name: "commitments.root_winner"}
	finished := make([]uint64, 0, len(f.Standings))
	var running, empty []uint64
	for _, index := range sortedKeys(f.Standings) {
		standing := f.Standings[index]
		switch {
		case !standing.HasWinner && standing.Finished:
			empty = append(empty, index)
			continue
		case !standing.HasWinner:
			running = append(running, index)
			continue
		}
		finished = append(finished, index)
		epoch, ok := f.Epochs[index]
		if !ok || epoch.Commitment == nil || *epoch.Commitment != standing.Winner {
			result.fail("epoch %d: root winner %s, rollups %v", index, standing.Winner, epoch.Commitment)
		}
	}
	result.Detail = fmt.Sprintf("finished roots: %s; the rollups commitment won %d of %d", indicesText(finished),
		len(finished)-len(result.Items), len(finished))
	if len(running) > 0 {
		result.Detail += fmt.Sprintf("; the root of %s has no winner yet", epochsText(running))
	}
	if len(empty) > 0 {
		result.Detail += fmt.Sprintf("; the root of %s finished with no winner", epochsText(empty))
	}
	return result.settle(len(finished))
}

// checkTournaments compares the tournament tree in both directions: address,
// parent, and epoch.
func checkTournaments(f *verifyFacts) checkResult {
	result := checkResult{Name: checkNameTournaments,
		Detail: fmt.Sprintf("chain %d, rollups node %d", len(f.ChainTournaments), len(f.Tournaments))}
	for address, want := range f.ChainTournaments {
		tournament, ok := f.Tournaments[address]
		parent := common.Address{}
		if ok && tournament.ParentTournamentAddress != nil {
			parent = *tournament.ParentTournamentAddress
		}
		switch {
		case !ok:
			result.fail("%s is not indexed", address)
		case parent != want.Parent:
			result.fail("%s parent: rollups %s, chain %s", address, parent, want.Parent)
		case tournament.EpochIndex != want.Epoch:
			result.fail("%s epoch: rollups %d, chain %d", address, tournament.EpochIndex, want.Epoch)
		}
	}
	for address := range f.Tournaments {
		if _, ok := f.ChainTournaments[address]; !ok {
			result.fail("%s is indexed but not on chain", address)
		}
	}
	sort.Strings(result.Items)
	return result.settle(len(f.ChainTournaments))
}

type tournamentKey struct {
	Tournament common.Address
	ID         common.Hash
}

// checkTournamentCommitments compares every CommitmentJoined event with the
// node's commitments, in both directions.
func checkTournamentCommitments(f *verifyFacts) checkResult {
	result := checkResult{Name: checkNameJoins,
		Detail: fmt.Sprintf("chain %d, rollups node %d", f.joinCount(), len(f.Commitments))}
	indexed := make(map[tournamentKey]model.Commitment, len(f.Commitments))
	for _, commitment := range f.Commitments {
		indexed[tournamentKey{commitment.TournamentAddress, commitment.Commitment}] = commitment
	}
	onChain := map[tournamentKey]bool{}
	for address, joins := range f.ChainJoins {
		for _, join := range joins {
			key := tournamentKey{address, join.Commitment}
			onChain[key] = true
			commitment, ok := indexed[key]
			switch {
			case !ok:
				result.fail("%s in %s is not indexed", join.Commitment, address)
			case commitment.FinalStateHash != join.FinalState || commitment.SubmitterAddress != join.Submitter:
				result.fail("%s in %s: final state %s by %s, chain %s by %s", join.Commitment, address,
					commitment.FinalStateHash, commitment.SubmitterAddress, join.FinalState, join.Submitter)
			case commitment.BlockNumber != join.Block || commitment.TxHash != join.Tx:
				result.fail("%s in %s: block %d tx %s, chain block %d tx %s", join.Commitment, address,
					commitment.BlockNumber, commitment.TxHash, join.Block, join.Tx)
			}
		}
	}
	for key := range indexed {
		if !onChain[key] {
			result.fail("%s in %s is indexed but not on chain", key.ID, key.Tournament)
		}
	}
	sort.Strings(result.Items)
	return result.settle(len(onChain))
}

// checkTournamentMatches compares every MatchCreated and MatchDeleted event
// with the node's matches, in both directions.
func checkTournamentMatches(f *verifyFacts) checkResult {
	result := checkResult{Name: checkNameMatches,
		Detail: fmt.Sprintf("chain %d, rollups node %d", len(f.ChainMatches), len(f.Matches))}
	indexed := make(map[tournamentKey]model.Match, len(f.Matches))
	for _, match := range f.Matches {
		indexed[tournamentKey{match.TournamentAddress, match.IDHash}] = match
	}
	onChain := map[tournamentKey]bool{}
	for _, want := range f.ChainMatches {
		key := tournamentKey{want.Tournament, want.ID}
		onChain[key] = true
		match, ok := indexed[key]
		deleted := ok && match.DeletionReason != "" && match.DeletionReason != model.MatchDeletionReason_NOT_DELETED
		switch {
		case !ok:
			result.fail("match %s in %s is not indexed", want.ID, want.Tournament)
		case match.CommitmentOne != want.One || match.CommitmentTwo != want.Two || match.LeftOfTwo != want.LeftOfTwo:
			result.fail("match %s in %s: commitments differ from MatchCreated", want.ID, want.Tournament)
		case match.BlockNumber != want.Block || match.TxHash != want.Tx:
			result.fail("match %s in %s: block %d tx %s, chain block %d tx %s", want.ID, want.Tournament,
				match.BlockNumber, match.TxHash, want.Block, want.Tx)
		case deleted != want.Deleted:
			result.fail("match %s in %s: deleted %t (%s), chain deleted %t", want.ID, want.Tournament, deleted,
				match.DeletionReason, want.Deleted)
		case deleted && (match.DeletionTxHash == nil || *match.DeletionTxHash != want.DeleteTx):
			result.fail("match %s in %s: deletion tx %v, chain %s", want.ID, want.Tournament, match.DeletionTxHash, want.DeleteTx)
		}
	}
	for key := range indexed {
		if !onChain[key] {
			result.fail("match %s in %s is indexed but not on chain", key.ID, key.Tournament)
		}
	}
	sort.Strings(result.Items)
	return result.settle(len(onChain))
}

// checkStateChange decides whether computation agreement was tested: at least
// one epoch that matches the Sling node's evidence must change state.
// Rejection-only epochs end at their initial state and prove little.
func checkStateChange(f *verifyFacts, matched map[uint64]bool, requireAgreement bool) checkResult {
	result := checkResult{Name: checkNameAgreement, Status: checkPass}
	var changed, unchanged, unmatched []uint64
	for _, sealed := range f.Sealed {
		if !matched[sealed.Index] {
			unmatched = append(unmatched, sealed.Index)
			continue
		}
		epoch := f.Epochs[sealed.Index]
		if epoch.MachineHash != nil && *epoch.MachineHash != sealed.InitialState {
			changed = append(changed, sealed.Index)
		} else {
			unchanged = append(unchanged, sealed.Index)
		}
	}
	result.Detail = fmt.Sprintf("epochs that match the Sling node and changed state: %s; unchanged: %s",
		indicesText(changed), indicesText(unchanged))
	if len(unmatched) > 0 {
		result.Detail += fmt.Sprintf("; without Sling evidence: %s", indicesText(unmatched))
	}
	if len(changed) == 0 {
		result.Status = checkNotTested
		if requireAgreement {
			result.Status = checkFail
			result.Detail += "; agreement is required"
		}
	}
	return result
}

func summarize(report *verifyReport) {
	report.Passed = len(report.Gaps) == 0
	byName := map[string]checkResult{}
	for _, check := range report.Checks {
		byName[check.Name] = check
		if check.Status == checkFail {
			report.Passed = false
		}
	}
	notFailed := func(names ...string) bool {
		for _, name := range names {
			if byName[name].Status == checkFail {
				return false
			}
		}
		return true
	}
	agreement, tournaments := byName[checkNameAgreement], byName[checkNameTournaments]
	report.Coverage = []coverageItem{
		{Name: coverageAgreement, Detail: agreement.Detail,
			Tested: agreement.Status == checkPass && notFailed(checkNameSlingCommitment, checkNameSlingFinalState)},
		{Name: coverageTournaments,
			Detail: fmt.Sprintf("tournaments %s; commitments %s; matches %s", tournaments.Detail, byName[checkNameJoins].Detail,
				byName[checkNameMatches].Detail),
			Tested: tournaments.Status == checkPass && byName[checkNameSealed].Status == checkPass &&
				notFailed(checkNameJoins, checkNameMatches)},
	}
}

// verifyUntil collects facts until every completion predicate holds in two
// snapshots in a row, the application reaches a status it cannot leave, or
// the deadline passes, and then evaluates the checks once.
func verifyUntil(ctx context.Context, t *verifyTarget, wait time.Duration) (*verifyReport, error) {
	if t.slingSigner != (common.Address{}) && t.slingSigner == t.rollupsSigner {
		return nil, fmt.Errorf("the Sling signer %s is the rollups node's own PRT signer; its joins are not independent evidence",
			t.slingSigner)
	}
	started := time.Now()
	deadline := started.Add(wait)
	var lastLog time.Time
	stable := "" // fingerprint of the previous complete snapshot
	for {
		if t.watch != nil {
			if err := t.watch(); err != nil {
				return nil, err
			}
		}
		delay := verifyPollInterval
		facts, err := collectFacts(ctx, t)
		switch {
		case err != nil && time.Now().After(deadline):
			return nil, err
		case err != nil:
			logger.Warn("reading the chain or the rollups node failed; retrying", "error", err)
			stable = ""
		default:
			gaps, terminal := completionGaps(facts, t.expectedStatus)
			current := fingerprint(facts)
			if (len(gaps) == 0 && current == stable) || terminal || time.Now().After(deadline) {
				report := &verifyReport{Application: t.app, Address: t.appAddress, Head: facts.Head, Gaps: gaps,
					Checks: evaluate(facts, t)}
				summarize(report)
				return report, nil
			}
			if len(gaps) == 0 {
				logger.Debug("complete; checking that the rollups node is stable")
				stable, delay = current, verifyStableDelay
				break
			}
			stable = ""
			if time.Since(lastLog) >= verifyLogInterval {
				step("verify", "waiting for the rollups node (%s): %s", time.Since(started).Round(time.Second), progressText(facts))
				lastLog = time.Now()
			}
			logger.Debug("waiting for the rollups node", "missing", len(gaps), "first", gaps[0])
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
}

func (f *chainFacts) sealedIndices() []uint64 {
	indices := make([]uint64, 0, len(f.Sealed))
	for _, sealed := range f.Sealed {
		indices = append(indices, sealed.Index)
	}
	return indices
}

// indicesText renders sorted epoch indices compactly, for example "0–6" or
// "1, 3, 4, 6", or "none".
func indicesText(indices []uint64) string {
	if len(indices) == 0 {
		return "none"
	}
	var parts []string
	for i := 0; i < len(indices); {
		j := i
		for j+1 < len(indices) && indices[j+1] == indices[j]+1 {
			j++
		}
		switch j {
		case i:
			parts = append(parts, strconv.FormatUint(indices[i], 10))
		case i + 1:
			parts = append(parts, strconv.FormatUint(indices[i], 10), strconv.FormatUint(indices[j], 10))
		default:
			parts = append(parts, fmt.Sprintf("%d–%d", indices[i], indices[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ", ")
}

// epochsText is indicesText with the noun: "epoch 5", "epochs 0–6", "no epoch".
func epochsText(indices []uint64) string {
	switch len(indices) {
	case 0:
		return "no epoch"
	case 1:
		return "epoch " + indicesText(indices)
	}
	return "epochs " + indicesText(indices)
}

func sortedKeys[V any](m map[uint64]V) []uint64 {
	keys := make([]uint64, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
