// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"
)

// The full live scenario: planned epochs, a fake commitment that must lose by
// timeout, and the execution of every output. See newLiveCommand.

const (
	scenarioFull = "full"

	// defaultLiveDapp is built by "make interop-live-dapp" from
	// test/dapps/interop-live: a payload that starts with rejectPrefix is
	// rejected; any other emits a voucher, a notice, and a report.
	defaultLiveDapp = "applications/interop-live-dapp"
	rejectPrefix    = "reject"

	defaultFakeSignerIx = 5
	partyFake           = "fake"
)

// epochPlan is one planned epoch of the full scenario.
type epochPlan struct {
	name     string
	payloads []string
	fake     bool
}

func fullEpochPlans() []epochPlan {
	return []epochPlan{
		{name: "all accepted", payloads: []string{"epoch 1 input a", "epoch 1 input b", "epoch 1 input c"}},
		{name: "accepted and rejected", payloads: []string{"epoch 2 input a", "reject epoch 2 input b", "epoch 2 input c",
			"reject epoch 2 input d"}},
		{name: "all rejected", payloads: []string{"reject epoch 3 input a", "reject epoch 3 input b", "reject epoch 3 input c"}},
		{name: "no inputs"},
		{name: "all accepted, fake commitment", payloads: []string{"epoch 5 input a", "epoch 5 input b", "epoch 5 input c"},
			fake: true},
	}
}

func plannedAccept(payload string) bool { return !strings.HasPrefix(payload, rejectPrefix) }

// plannedInput is one input that the full scenario sent.
type plannedInput struct {
	Index   uint64
	Epoch   uint64
	Payload string
	Tx      common.Hash
}

// fakeCommitment is the dishonest join of the full scenario.
type fakeCommitment struct {
	Epoch      uint64
	Tournament common.Address
	Commitment common.Hash
	Signer     common.Address
	Tx         common.Hash
	// Honest is the join that the fake commitment was paired with.
	Honest commitmentJoin
}

// fullRun is what the full scenario did, for its checks after verification.
type fullRun struct {
	plans  []epochPlan
	inputs []plannedInput
	sealed map[uint64]sealedEpoch
	fake   *fakeCommitment
}

// liveFull sends each epoch's inputs while that epoch is open, joins a fake
// commitment in the last planned epoch, waits until that epoch is accepted,
// executes every voucher and validates every notice, and freezes the chain.
func (s *session) liveFull(ctx context.Context, run *liveRun) (uint64, error) {
	full := &fullRun{plans: fullEpochPlans(), sealed: map[uint64]sealedEpoch{}}
	run.full = full
	sent := uint64(0)
	for i, plan := range full.plans {
		epoch := uint64(i + 1)
		// Epoch n is open until epoch n-1 is accepted, which takes a whole
		// tournament: inputs sent now land in it.
		for _, payload := range plan.payloads {
			tx, err := s.sendInput(ctx, run, payload)
			if err != nil {
				return 0, err
			}
			full.inputs = append(full.inputs, plannedInput{Index: sent, Epoch: epoch, Payload: payload, Tx: tx})
			run.report.InputTxs = append(run.report.InputTxs, tx)
			sent++
		}
		step("live", "epoch %d (%s): %d inputs sent; waiting for it to be sealed", epoch, plan.name, len(plan.payloads))
		sealed, err := s.waitSealed(ctx, run, epoch)
		if err != nil {
			return 0, err
		}
		full.sealed[epoch] = sealed
		lower := sent - uint64(len(plan.payloads))
		if sealed.Lower != lower || sealed.Upper != sent {
			return 0, fmt.Errorf("epoch %d was sealed with inputs [%d,%d), planned [%d,%d); the plan is broken",
				epoch, sealed.Lower, sealed.Upper, lower, sent)
		}
		if epoch == 1 {
			run.report.InputEpoch = epoch
			if err := s.startSecondNode(ctx, run, sealed.Tournament); err != nil {
				return 0, err
			}
		}
		if plan.fake {
			if full.fake, err = s.joinFakeCommitment(ctx, run, sealed); err != nil {
				return 0, err
			}
		}
	}
	last := uint64(len(full.plans))
	if full.fake != nil {
		step("live", "waiting for epoch %d to be accepted: the Sling node must win the match against the fake commitment", last)
	} else {
		step("live", "waiting for epoch %d to be accepted", last)
	}
	if _, err := s.waitSealed(ctx, run, last+1); err != nil {
		return 0, err
	}
	// Executions need mining, and so do the last bond recoveries; the chain
	// is frozen afterwards.
	run.report.Steps = s.handleOutputs(ctx, run)
	for i := range run.report.Steps {
		run.report.Steps[i].Group = groupOutputs
	}
	if err := s.waitBondRecoveries(ctx, run, uint64(len(full.plans))); err != nil {
		logger.Warn("not every bond was recovered; the bond checks will say which", "error", err)
	}
	head, err := s.freezeLive(ctx, run)
	if err != nil {
		return 0, err
	}
	step("live", "every planned epoch is accepted; chain frozen at block %d", head)
	return head, s.recordJoinOrder(ctx, run, full.sealed[1].Tournament, head)
}

// waitSealed waits until epoch index is sealed (epoch index-1 accepted).
func (s *session) waitSealed(ctx context.Context, run *liveRun, index uint64) (sealedEpoch, error) {
	ctx, cancel := context.WithTimeout(ctx, run.opts.timeout)
	defer cancel()
	var found sealedEpoch
	err := s.waitForLive(ctx, fmt.Sprintf("epoch %d to be sealed", index), func(_ uint64, sealed map[uint64]sealedEpoch) (bool, error) {
		epoch, ok := sealed[index]
		found = epoch
		return ok, nil
	})
	return found, err
}

// joinFakeCommitment waits until an honest commitment joined the epoch's
// root tournament and then joins a fake one from test account 5, which is
// paired with it and never moves (Dave's bad_commitment scenario does the
// same).
func (s *session) joinFakeCommitment(ctx context.Context, run *liveRun, sealed sealedEpoch) (*fakeCommitment, error) {
	waitCtx, cancel := context.WithTimeout(ctx, run.opts.timeout)
	defer cancel()
	var honest commitmentJoin
	err := s.waitFor(waitCtx, fmt.Sprintf("an honest join in epoch %d", sealed.Index), func() (bool, error) {
		header, err := s.chain.head(ctx)
		if err != nil {
			return false, err
		}
		joins, err := s.chain.joins(ctx, sealed.Tournament, header.Number.Uint64())
		if err != nil || len(joins) == 0 {
			return false, err
		}
		honest = joins[0]
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	account, err := deriveTestAccount(testMnemonic, defaultFakeSignerIx)
	if err != nil {
		return nil, err
	}
	commitment, tx, err := s.chain.joinFakeCommitment(ctx, sealed.Tournament, account)
	if err != nil {
		return nil, fmt.Errorf("joining the fake commitment: %w", err)
	}
	if _, err := s.chain.waitReceipt(ctx, tx, receiptTimeout); err != nil {
		return nil, fmt.Errorf("joining the fake commitment: %w", err)
	}
	step("live", "epoch %d: fake commitment %s joined against the honest %s (joined by %s); it never moves",
		sealed.Index, commitment, honest.Commitment, run.party(honest.Submitter))
	return &fakeCommitment{Epoch: sealed.Index, Tournament: sealed.Tournament, Commitment: commitment,
		Signer: account.Address, Tx: tx, Honest: honest}, nil
}

// Output kinds, by the selector of their ABI encoding.
const (
	outputVoucher      = "voucher"
	outputNotice       = "notice"
	outputDelegateCall = "delegate call voucher"
)

// outputKind classifies an encoded output with the generated Outputs ABI.
func outputKind(raw []byte) string {
	parsed, err := outputs.OutputsMetaData.GetAbi()
	if err != nil {
		return textUnknown
	}
	for name, kind := range map[string]string{"Voucher": outputVoucher, "Notice": outputNotice,
		"DelegateCallVoucher": outputDelegateCall} {
		if selector := parsed.Methods[name].ID; len(selector) > 0 && bytes.HasPrefix(raw, selector) {
			return kind
		}
	}
	return textUnknown
}

// handleOutputs executes every voucher and validates every notice with the
// rollups CLI, and waits until the rollups node indexes the executions.
//
//nolint:funlen // one step per kind of work
func (s *session) handleOutputs(ctx context.Context, run *liveRun) []continuationStep {
	all, err := s.api.outputs(ctx, s.appName)
	if err != nil {
		return []continuationStep{{Name: "outputs", Status: checkFail, Detail: err.Error()}}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Index < all[j].Index })
	accepted := 0
	for _, input := range run.full.inputs {
		if plannedAccept(input.Payload) {
			accepted++
		}
	}
	perInput := map[uint64]map[string]int{}
	for _, output := range all {
		if perInput[output.InputIndex] == nil {
			perInput[output.InputIndex] = map[string]int{}
		}
		perInput[output.InputIndex][outputKind(output.RawData)]++
	}
	count := continuationStep{Name: "outputs", Status: checkPass,
		Detail: fmt.Sprintf("%d outputs from %d accepted inputs: one voucher and one notice each", len(all), accepted)}
	for _, input := range run.full.inputs {
		kinds := perInput[input.Index]
		want := 1
		if !plannedAccept(input.Payload) {
			want = 0
		}
		if kinds[outputVoucher] != want || kinds[outputNotice] != want || len(kinds) > 2 {
			count.Status = checkFail
			count.Detail += fmt.Sprintf("; input %d has %v", input.Index, kinds)
		}
	}
	steps := []continuationStep{count}
	// Vouchers move ETH from the application: fund it first, and follow
	// every balance involved.
	ledger, err := s.fundVouchers(ctx, run, all)
	if err != nil {
		steps = append(steps, continuationStep{Name: stepNameBalances, Status: checkFail, Detail: err.Error()})
	}
	step("live", "executing the vouchers and validating the notices of %d outputs", len(all))

	executions := map[uint64]common.Hash{}
	var executeFailures, validateFailures []string
	vouchers, notices := 0, 0
	for _, output := range all {
		index := strconv.FormatUint(output.Index, 10)
		switch outputKind(output.RawData) {
		case outputVoucher:
			vouchers++
			tx, receipt, err := s.executeVoucher(ctx, run, output)
			ledger.addExecution(receipt, output.RawData)
			if err != nil {
				executeFailures = append(executeFailures, fmt.Sprintf("output %d: %v", output.Index, err))
				continue
			}
			executions[output.Index] = tx
		case outputNotice:
			notices++
			out, err := s.runCLI(ctx, "validate", s.appName, index, "--json")
			var result struct {
				Valid bool `json:"valid"`
			}
			if err == nil {
				err = json.Unmarshal([]byte(strings.TrimSpace(out)), &result)
			}
			if err != nil || !result.Valid {
				validateFailures = append(validateFailures, fmt.Sprintf("output %d: not valid (%v, %q)", output.Index, err, out))
			}
		}
	}
	steps = append(steps,
		outputStep("execute vouchers", vouchers, executeFailures,
			fmt.Sprintf("%d of %d executed, each with OutputExecuted for its index", len(executions), vouchers)),
		outputStep("validate notices", notices, validateFailures,
			fmt.Sprintf("%d of %d validated on chain", notices-len(validateFailures), notices)))
	if ledger != nil {
		steps = append(steps, s.closeLedger(ctx, run, ledger))
	}

	indexed := continuationStep{Name: "node indexes the executions", Status: checkPass,
		Detail: fmt.Sprintf("%d execution transactions recorded", len(executions))}
	if len(executions) == 0 {
		indexed.Status, indexed.Detail = checkNotTested, "nothing was executed"
		return append(steps, indexed)
	}
	waitCtx, cancel := context.WithTimeout(ctx, run.opts.timeout)
	defer cancel()
	err = s.waitFor(waitCtx, "the rollups node to index the executions", func() (bool, error) {
		current, err := s.api.outputs(ctx, s.appName)
		if err != nil {
			return false, err
		}
		for _, output := range current {
			want, ok := executions[output.Index]
			switch {
			case !ok:
			case output.ExecutionTransactionHash == nil:
				return false, nil
			case *output.ExecutionTransactionHash != want:
				return false, permanent(fmt.Errorf("output %d execution transaction %s, want %s", output.Index,
					output.ExecutionTransactionHash, want))
			}
		}
		return true, nil
	})
	if err != nil {
		indexed.Status, indexed.Detail = checkFail, err.Error()
	}
	return append(steps, indexed)
}

// outputStep reports one kind of output work: not tested when there was
// nothing to do, failed when any output failed.
func outputStep(name string, total int, failures []string, summary string) continuationStep {
	step := continuationStep{Name: name, Status: checkPass, Detail: summary}
	switch {
	case len(failures) > 0:
		step.Status = checkFail
		step.Detail += "; " + strings.Join(failures, "; ")
	case total == 0:
		step.Status = checkNotTested
	}
	return step
}

// executeVoucher executes one voucher with the rollups CLI and checks the
// OutputExecuted event. The receipt is returned whenever the transaction was
// mined, even when it failed: its gas was spent.
func (s *session) executeVoucher(ctx context.Context, run *liveRun, output model.Output) (common.Hash, *types.Receipt, error) {
	out, err := s.runCLI(ctx, "execute", s.appName, strconv.FormatUint(output.Index, 10), "--yes", "--json")
	if err != nil {
		return common.Hash{}, nil, err
	}
	tx, err := parseCLITransaction(out)
	if err != nil {
		return common.Hash{}, nil, err
	}
	receipt, err := s.chain.waitReceipt(ctx, tx, receiptTimeout)
	if err != nil {
		return common.Hash{}, receipt, err
	}
	executed, err := parseOutputExecuted(receipt, run.app)
	if err != nil {
		return common.Hash{}, receipt, err
	}
	if executed.OutputIndex != output.Index {
		return common.Hash{}, receipt, fmt.Errorf("OutputExecuted for output %d", executed.OutputIndex)
	}
	return tx, receipt, nil
}

// fullPlanSteps checks, at the frozen head and after verification, that the
// fake commitment lost, the bonds, and that the outputs roots on chain equal
// independently computed ones. The epochs table checks each planned epoch.
func (s *session) fullPlanSteps(ctx context.Context, run *liveRun, head uint64) []continuationStep {
	full := run.full
	var steps []continuationStep
	sealed, err := s.chain.sealedEpochs(ctx, run.consensus, head)
	if err != nil {
		return []continuationStep{{Name: "plan", Status: checkFail, Detail: err.Error()}}
	}
	sealedMap := sealedByIndex(sealed)

	if fake := full.fake; fake != nil {
		for _, step := range s.fakeCommitmentSteps(ctx, run, fake, head) {
			step.Group = groupFake
			steps = append(steps, step)
		}
	}
	steps = append(steps, s.bondSteps(ctx, run, head, uint64(len(full.plans)), sealedMap)...)

	roots := continuationStep{Name: "outputs roots", Status: checkPass, Group: groupOutputs}
	all, err := s.api.outputs(ctx, s.appName)
	staged, stagedErr := s.chain.stagedEpochs(ctx, run.consensus, head)
	switch {
	case err != nil:
		roots.Status, roots.Detail = checkFail, err.Error()
	case stagedErr != nil:
		roots.Status, roots.Detail = checkFail, stagedErr.Error()
	default:
		sort.Slice(all, func(i, j int) bool { return all[i].Index < all[j].Index })
		for _, index := range sortedKeys(staged) {
			epoch, ok := sealedMap[index]
			if !ok {
				continue
			}
			var payloads [][]byte
			for _, output := range all {
				if output.InputIndex < epoch.Upper {
					payloads = append(payloads, output.RawData)
				}
			}
			if root := outputsMerkleRoot(payloads); root != staged[index].OutputsRoot {
				roots.Status = checkFail
				roots.Detail += fmt.Sprintf("epoch %d: staged %s, independent %s over %d outputs; ", index,
					staged[index].OutputsRoot, root, len(payloads))
			}
		}
		roots.Detail += fmt.Sprintf("%d staged outputs roots equal the independently computed root of the outputs so far",
			len(staged))
	}
	return append(steps, roots)
}

// fakeCommitmentSteps checks that the fake commitment joined, was paired
// with the honest one, lost its match, and that the honest one won the root.
func (s *session) fakeCommitmentSteps(ctx context.Context, run *liveRun, fake *fakeCommitment, head uint64) []continuationStep {
	joined := continuationStep{Name: fmt.Sprintf("fake commitment in epoch %d", fake.Epoch), Status: checkPass,
		Tx: fake.Tx.Hex(), Sender: fake.Signer.Hex(),
		Detail: fmt.Sprintf("%s joined against the honest %s (joined by %s)", fake.Commitment, fake.Honest.Commitment,
			run.party(fake.Honest.Submitter))}
	matches, err := s.chain.matches(ctx, fake.Tournament, head)
	if err != nil {
		joined.Status, joined.Detail = checkFail, err.Error()
		return []continuationStep{joined}
	}
	lost := continuationStep{Name: "fake commitment loses its match", Status: checkFail,
		Detail: "no match with the fake commitment"}
	for _, match := range matches {
		if match.One != fake.Commitment && match.Two != fake.Commitment {
			continue
		}
		lost.Detail = fmt.Sprintf("match %s is not deleted", match.ID)
		if match.Deleted {
			lost.Status, lost.Tx = checkPass, match.DeleteTx.Hex()
			lost.Detail = fmt.Sprintf("match %s deleted", match.ID)
			if reason := s.nodeDeletionReason(ctx, fake.Tournament, match.ID); reason != "" {
				lost.Detail += "; the rollups node records " + reason
			}
		}
	}
	won := continuationStep{Name: fmt.Sprintf("honest commitment wins epoch %d", fake.Epoch), Status: checkFail}
	standing, err := s.chain.rootStanding(ctx, fake.Tournament, head)
	switch {
	case err != nil:
		won.Detail = err.Error()
	case !standing.HasWinner:
		won.Detail = "the root tournament has no winner"
	case standing.Winner != fake.Honest.Commitment:
		won.Detail = fmt.Sprintf("root winner %s, honest %s", standing.Winner, fake.Honest.Commitment)
	default:
		won.Status, won.Detail = checkPass, "root winner "+standing.Winner.Hex()
	}
	return []continuationStep{joined, lost, won}
}

// nodeDeletionReason returns the deletion reason that the rollups node
// recorded for a match, or "" when it cannot tell.
func (s *session) nodeDeletionReason(ctx context.Context, tournament common.Address, id common.Hash) string {
	matches, err := s.api.matches(ctx, s.appName)
	if err != nil {
		return ""
	}
	for _, match := range matches {
		if match.TournamentAddress == tournament && match.IDHash == id {
			return "deletion reason " + string(match.DeletionReason)
		}
	}
	return ""
}
