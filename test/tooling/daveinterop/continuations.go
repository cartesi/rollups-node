// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cartesi/rollups-node/internal/model"
)

const (
	waitPollInterval  = 2 * time.Second
	waitLogInterval   = 30 * time.Second
	receiptTimeout    = 2 * time.Minute
	maxMiningFailures = 5
	stepNameBalances  = "balances"
	maxNameColumn     = 34
)

type continuationStep struct {
	Name   string      `json:"name"`
	Status checkStatus `json:"status"`
	Detail string      `json:"detail"`
	Tx     string      `json:"transaction_hash,omitempty"`
	Block  uint64      `json:"block,omitempty"`
	Sender string      `json:"sender,omitempty"`
}

type continuationReport struct {
	Name   string             `json:"name"`
	Passed bool               `json:"passed"`
	Steps  []continuationStep `json:"steps"`
	Error  string             `json:"error,omitempty"`
}

func (r *continuationReport) add(step continuationStep) {
	r.Steps = append(r.Steps, step)
	logger.Info(fmt.Sprintf("%s: %s %s: %s", r.Name, step.Status, step.Name, step.Detail))
}

func (r *continuationReport) finish(err error) *continuationReport {
	r.Passed = err == nil
	for _, step := range r.Steps {
		if step.Status == checkFail {
			r.Passed = false
		}
	}
	if err != nil {
		r.Error = err.Error()
	}
	return r
}

// runContinuations mines blocks in the background and runs each continuation
// in order.
func (s *session) runContinuations(ctx context.Context, names []string) []*continuationReport {
	minerCtx, stopMiner := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.mine(minerCtx)
	}()
	defer func() {
		stopMiner()
		wg.Wait()
	}()

	reports := make([]*continuationReport, 0, len(names))
	for _, name := range names {
		step(name, "starting; %s at the default mining rate (%d blocks every %s)", aboutText(continuationDurations[name]),
			s.opts.mineStep, s.opts.mineInterval)
		caseCtx, cancel := context.WithTimeout(ctx, s.opts.continuationTimeout)
		var report *continuationReport
		switch name {
		case contRollupsAlone:
			report = s.rollupsContinuesAlone(caseCtx)
		case contOutputExecution:
			report = s.outputExecution(caseCtx)
		}
		cancel()
		step(name, "%s", passFail(report.Passed))
		reports = append(reports, report)
	}
	return reports
}

// mine advances the test-owned chain. The chain was loaded with mining off,
// so pending transactions are included only by these steps. It stops, and
// records why, when mining keeps failing or when one more step would take the
// chain past the historical states that Anvil keeps. The waits of the run
// then fail at once (see checkParticipants).
func (s *session) mine(ctx context.Context) {
	ticker := time.NewTicker(s.opts.mineInterval)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		header, err := s.chain.head(ctx)
		if err == nil && header.Number.Uint64()+s.opts.mineStep >= s.opts.persistedStates {
			s.setMinerError(fmt.Errorf("the chain reached block %d; Anvil keeps only %d historical states "+
				"(raise --anvil-persisted-states)", header.Number, s.opts.persistedStates))
			return
		}
		if err == nil {
			err = s.chain.anvil(ctx, "anvil_mine", hexutil.EncodeUint64(s.opts.mineStep))
		}
		switch {
		case ctx.Err() != nil:
			return
		case err == nil:
			failures = 0
		case failures+1 < maxMiningFailures:
			failures++
			logger.Warn("mining failed; retrying", "error", err)
		default:
			s.setMinerError(fmt.Errorf("mining failed %d times: %w", maxMiningFailures, err))
			return
		}
	}
}

func (s *session) setMinerError(err error) {
	s.minerMu.Lock()
	defer s.minerMu.Unlock()
	if s.minerErr == nil {
		s.minerErr = err
		logger.Error("stopped mining", "error", err)
	}
}

func (s *session) minerError() error {
	s.minerMu.Lock()
	defer s.minerMu.Unlock()
	return s.minerErr
}

// permanentError marks a condition error that waiting cannot fix.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

func permanent(err error) error { return &permanentError{err: err} }

// waitFor polls cond until it returns true, a participant exits, the miner
// stops, cond returns a permanent error, or ctx expires. Other errors from
// cond are retried. Every waitLogInterval it logs how long it has waited.
func (s *session) waitFor(ctx context.Context, what string, cond func() (bool, error)) error {
	var lastErr error
	started := time.Now()
	lastLog := started
	for {
		ok, err := cond()
		if ok {
			return nil
		}
		var fatal *permanentError
		if errors.As(err, &fatal) {
			return fmt.Errorf("waiting for %s: %w", what, fatal.err)
		}
		if err != nil {
			lastErr = err
		}
		if exitErr := s.checkParticipants(); exitErr != nil {
			return exitErr
		}
		if time.Since(lastLog) >= waitLogInterval {
			lastLog = time.Now()
			block := textUnknown
			if header, err := s.chain.head(ctx); err == nil {
				block = header.Number.String()
			}
			step("wait", "%s: %s so far, chain at block %s", what, time.Since(started).Round(time.Second), block)
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for %s (last error: %w)", what, lastErr)
			}
			return fmt.Errorf("timed out waiting for %s", what)
		case <-time.After(waitPollInterval):
		}
	}
}

// senderStep checks that a transaction was sent by want.
func (s *session) senderStep(ctx context.Context, name string, tx common.Hash, block uint64, want common.Address) continuationStep {
	step := continuationStep{Name: name, Tx: tx.Hex(), Block: block}
	sender, err := s.chain.txSender(ctx, tx)
	if err != nil {
		step.Status, step.Detail = checkFail, err.Error()
		return step
	}
	step.Sender = sender.Hex()
	if sender == want {
		step.Status, step.Detail = checkPass, "sent by the rollups PRT signer"
	} else {
		step.Status, step.Detail = checkFail, fmt.Sprintf("sent by %s, not the rollups PRT signer %s", sender, want)
	}
	return step
}

func sealedByIndex(epochs []sealedEpoch) map[uint64]sealedEpoch {
	byIndex := make(map[uint64]sealedEpoch, len(epochs))
	for _, epoch := range epochs {
		byIndex[epoch.Index] = epoch
	}
	return byIndex
}

// rollupsContinuesAlone checks that the rollups node, with no Sling node
// running, stages and accepts the first unaccepted epoch, then joins, stages, accepts,
// and recovers its own bond for the next one.
func (s *session) rollupsContinuesAlone(ctx context.Context) *continuationReport {
	report := &continuationReport{Name: contRollupsAlone}
	rollupsSigner, err := s.rollupsPrtSigner()
	if err != nil {
		return report.finish(err)
	}
	consensus := s.manifest.Application.Consensus
	header, err := s.chain.head(ctx)
	if err != nil {
		return report.finish(err)
	}
	sealed, err := s.chain.sealedEpochs(ctx, consensus, header.Number.Uint64())
	if err != nil {
		return report.finish(err)
	}
	stagedAtStart, err := s.chain.stagedEpochs(ctx, consensus, header.Number.Uint64())
	if err != nil {
		return report.finish(err)
	}
	byIndex := sealedByIndex(sealed)
	target, found := uint64(0), false
	for _, epoch := range sealed {
		if _, next := byIndex[epoch.Index+1]; !next {
			target, found = epoch.Index, true
			break
		}
	}
	if !found {
		return report.finish(errors.New("no sealed epoch is waiting for acceptance"))
	}
	next := target + 1
	report.add(continuationStep{Name: "select", Status: checkPass,
		Detail: fmt.Sprintf("target epoch %d; rollups PRT signer %s", target, rollupsSigner)})

	var nextTournament common.Address
	err = s.waitFor(ctx, fmt.Sprintf("acceptance of epoch %d and the rollups bond recovery of epoch %d", next, next), func() (bool, error) {
		h, err := s.chain.head(ctx)
		if err != nil {
			return false, err
		}
		sealed, err = s.chain.sealedEpochs(ctx, consensus, h.Number.Uint64())
		if err != nil {
			return false, err
		}
		byIndex = sealedByIndex(sealed)
		nextEpoch, ok := byIndex[next]
		if !ok {
			return false, nil
		}
		nextTournament = nextEpoch.Tournament
		if _, ok := byIndex[next+1]; !ok {
			return false, nil
		}
		recoveries, err := s.chain.bondRecoveries(ctx, nextTournament, h.Number.Uint64())
		if err != nil {
			return false, err
		}
		for _, recovery := range recoveries {
			if recovery.Claimer == rollupsSigner {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return report.finish(err)
	}

	header, err = s.chain.head(ctx)
	if err != nil {
		return report.finish(err)
	}
	head := header.Number.Uint64()
	staged, err := s.chain.stagedEpochs(ctx, consensus, head)
	if err != nil {
		return report.finish(err)
	}
	// Epoch target: the rollups node stages it (unless someone else did before
	// this continuation started) and accepts it.
	report.add(s.targetStagedStep(ctx, target, staged, stagedAtStart, rollupsSigner))
	acceptTarget := byIndex[next]
	report.add(s.senderStep(ctx, fmt.Sprintf("accept epoch %d", target), acceptTarget.Tx, acceptTarget.Block, rollupsSigner))

	// Epoch next: the rollups node joins, stages, accepts, and recovers its own bond.
	joins, err := s.chain.joins(ctx, nextTournament, head)
	if err != nil {
		return report.finish(err)
	}
	joinStep := continuationStep{Name: fmt.Sprintf("join epoch %d", next), Status: checkFail, Detail: "no join by the rollups PRT signer"}
	for _, join := range joins {
		if join.Submitter == rollupsSigner {
			joinStep = continuationStep{Name: joinStep.Name, Status: checkPass, Tx: join.Tx.Hex(), Block: join.Block,
				Sender: rollupsSigner.Hex(), Detail: "commitment " + join.Commitment.Hex()}
		}
	}
	report.add(joinStep)
	if stagedNext, ok := staged[next]; ok {
		report.add(s.senderStep(ctx, fmt.Sprintf("stage epoch %d", next), stagedNext.Tx, stagedNext.Block, rollupsSigner))
	} else {
		report.add(continuationStep{Name: fmt.Sprintf("stage epoch %d", next), Status: checkFail, Detail: "no EpochStaged event"})
	}
	acceptNext := byIndex[next+1]
	report.add(s.senderStep(ctx, fmt.Sprintf("accept epoch %d", next), acceptNext.Tx, acceptNext.Block, rollupsSigner))

	recoveries, err := s.chain.bondRecoveries(ctx, nextTournament, head)
	if err != nil {
		return report.finish(err)
	}
	for _, recovery := range recoveries {
		if recovery.Claimer == rollupsSigner {
			step := s.senderStep(ctx, fmt.Sprintf("recover bond of epoch %d", next), recovery.Tx, recovery.Block, rollupsSigner)
			step.Detail += fmt.Sprintf("; payment %s wei", recovery.Payment)
			if recovery.Block > acceptNext.Block {
				step.Detail += "; recovered after acceptance"
			} else {
				step.Detail += "; recovered before acceptance"
			}
			report.add(step)
		}
	}

	// The target epoch's bond belongs to whoever joined it first. The rollups
	// node must not be paid for a bond it does not own.
	targetTournament := byIndex[target].Tournament
	targetJoins, err := s.chain.joins(ctx, targetTournament, head)
	if err != nil {
		return report.finish(err)
	}
	rollupsJoinedTarget := false
	for _, join := range targetJoins {
		rollupsJoinedTarget = rollupsJoinedTarget || join.Submitter == rollupsSigner
	}
	targetRecoveries, err := s.chain.bondRecoveries(ctx, targetTournament, head)
	if err != nil {
		return report.finish(err)
	}
	foreign := continuationStep{Name: fmt.Sprintf("bond of epoch %d", target), Status: checkPass,
		Detail: "no payment to the rollups signer for a bond it does not own"}
	for _, recovery := range targetRecoveries {
		if recovery.Claimer == rollupsSigner && !rollupsJoinedTarget {
			foreign = continuationStep{Name: foreign.Name, Status: checkFail, Tx: recovery.Tx.Hex(),
				Detail: "the rollups signer was paid for a bond it did not post"}
		}
	}
	if rollupsJoinedTarget {
		foreign.Status, foreign.Detail = checkNotTested, "the rollups signer joined this epoch"
	}
	report.add(foreign)

	// The node records both acceptances with the acceptance transactions.
	for _, pair := range []struct {
		epoch uint64
		tx    common.Hash
	}{{target, acceptTarget.Tx}, {next, acceptNext.Tx}} {
		report.add(s.nodeAcceptedStep(ctx, pair.epoch, pair.tx))
	}
	return report.finish(nil)
}

// targetStagedStep judges who staged the target epoch. The rollups node must
// stage it, unless another signer (the Sling node, during capture, or the
// rollups node in an earlier continuation) staged it before this
// continuation started; that is reported as not tested.
func (s *session) targetStagedStep(ctx context.Context, target uint64, staged, stagedAtStart map[uint64]stagedEpoch,
	rollupsSigner common.Address,
) continuationStep {
	name := fmt.Sprintf("stage epoch %d", target)
	event, ok := staged[target]
	if !ok {
		return continuationStep{Name: name, Status: checkFail, Detail: "no EpochStaged event"}
	}
	step := s.senderStep(ctx, name, event.Tx, event.Block, rollupsSigner)
	if _, before := stagedAtStart[target]; before && step.Status == checkFail && step.Sender != "" {
		who := step.Sender
		if common.HexToAddress(step.Sender) == s.manifest.Reference.Signer {
			who = "the Sling node (" + step.Sender + ")"
		}
		step.Status, step.Detail = checkNotTested, "staged by "+who+" before this continuation"
	}
	return step
}

func (s *session) nodeAcceptedStep(ctx context.Context, index uint64, tx common.Hash) continuationStep {
	step := continuationStep{Name: fmt.Sprintf("node records epoch %d accepted", index), Tx: tx.Hex()}
	err := s.waitFor(ctx, step.Name, func() (bool, error) {
		epochs, err := s.api.epochs(ctx, s.appName)
		if err != nil {
			return false, err
		}
		for _, epoch := range epochs {
			if epoch.Index == index && epoch.Status == model.EpochStatus_ClaimAccepted {
				if epoch.ClaimTransactionHash == nil || *epoch.ClaimTransactionHash != tx {
					return false, permanent(fmt.Errorf("claim transaction %v, want %s", epoch.ClaimTransactionHash, tx))
				}
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		step.Status, step.Detail = checkFail, err.Error()
	} else {
		step.Status, step.Detail = checkPass, "CLAIM_ACCEPTED with the acceptance transaction"
	}
	return step
}

type cliTransaction struct {
	TransactionHash string `json:"transaction_hash"`
	Status          string `json:"status"`
}

func parseCLITransaction(out string) (common.Hash, error) {
	var result cliTransaction
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		return common.Hash{}, fmt.Errorf("parsing CLI output %q: %w", out, err)
	}
	if !strings.HasPrefix(result.TransactionHash, "0x") {
		return common.Hash{}, fmt.Errorf("CLI output has no transaction hash: %q", out)
	}
	return common.HexToHash(result.TransactionHash), nil
}

// outputExecution deposits TestFungibleToken into the honeypot application,
// requests a withdrawal, waits for the rollups node to settle the epoch, executes
// the voucher, and checks every effect.
func (s *session) outputExecution(ctx context.Context) *continuationReport {
	report := &continuationReport{Name: contOutputExecution}
	m := s.manifest
	if m.Provenance.Program != programHoneypot {
		return report.finish(fmt.Errorf("output-execution needs the honeypot program, the case uses %q", m.Provenance.Program))
	}
	if err := requireTestChain(m.Chain.ChainID); err != nil {
		return report.finish(err)
	}
	amount, ok := new(big.Int).SetString(s.opts.depositWei, decimalBase)
	if !ok || amount.Sign() <= 0 {
		return report.finish(fmt.Errorf("invalid --deposit-amount %q", s.opts.depositWei))
	}
	cliIndex, err := strconv.ParseUint(envValueOr(s.env, "CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX", "0"), 10, 32)
	if err != nil {
		return report.finish(err)
	}
	depositor, err := deriveTestAccount(envValueOr(s.env, "CARTESI_AUTH_MNEMONIC", testMnemonic), uint32(cliIndex))
	if err != nil {
		return report.finish(err)
	}
	withdrawer, err := deriveTestAccount(testMnemonic, defaultWithdrawerIx)
	if err != nil {
		return report.finish(err)
	}
	if withdrawer.Address != common.HexToAddress(defaultHoneypotWithdrawer) {
		return report.finish(fmt.Errorf("test account %d is %s, honeypot expects %s", defaultWithdrawerIx, withdrawer.Address,
			defaultHoneypotWithdrawer))
	}
	app := m.Application.Address
	token := m.Deployments.TestFungibleToken
	portal := m.Deployments.Erc20Portal

	appBefore, err := s.chain.tokenBalance(ctx, token, app)
	if err != nil {
		return report.finish(err)
	}
	withdrawerBefore, err := s.chain.tokenBalance(ctx, token, withdrawer.Address)
	if err != nil {
		return report.finish(err)
	}

	mintTx, err := s.chain.mintToken(ctx, token, depositor, amount)
	if err != nil {
		return report.finish(err)
	}
	if _, err := s.chain.waitReceipt(ctx, mintTx, receiptTimeout); err != nil {
		return report.finish(err)
	}
	report.add(continuationStep{Name: "mint", Status: checkPass, Tx: mintTx.Hex(),
		Detail: fmt.Sprintf("%s TestFungibleToken to %s", amount, depositor.Address)})

	out, err := s.runCLI(ctx, "deposit", "erc20", app.Hex(), "--portal", portal.Hex(), "--token", token.Hex(),
		"--amount", amount.String(), "--approve", "--yes", "--json")
	if err != nil {
		return report.finish(err)
	}
	depositTx, err := parseCLITransaction(out)
	if err != nil {
		return report.finish(err)
	}
	report.add(continuationStep{Name: "deposit", Status: checkPass, Tx: depositTx.Hex(), Detail: "cartesi-rollups-cli deposit erc20"})

	withdrawTx, err := s.chain.addInput(ctx, m.Deployments.InputBox, app, withdrawer, nil)
	if err != nil {
		return report.finish(err)
	}
	if _, err := s.chain.waitReceipt(ctx, withdrawTx, receiptTimeout); err != nil {
		return report.finish(err)
	}
	report.add(continuationStep{Name: "request withdrawal", Status: checkPass, Tx: withdrawTx.Hex(),
		Detail: "empty input from " + withdrawer.Address.Hex()})

	depositInput, err := s.waitInput(ctx, depositTx)
	if err != nil {
		return report.finish(err)
	}
	report.add(inputStep("deposit input", depositInput))
	withdrawInput, err := s.waitInput(ctx, withdrawTx)
	if err != nil {
		return report.finish(err)
	}
	report.add(inputStep("withdrawal input", withdrawInput))
	if withdrawInput.Status != model.InputCompletionStatus_Accepted {
		return report.finish(errors.New("the withdrawal input was not accepted"))
	}

	outputs, err := s.api.outputs(ctx, s.appName)
	if err != nil {
		return report.finish(err)
	}
	// The tx-buffer word after the withdrawal input is the root of every output
	// emitted up to that input, in output index order.
	sort.Slice(outputs, func(i, j int) bool { return outputs[i].Index < outputs[j].Index })
	var voucher *model.Output
	payloads := make([][]byte, 0, len(outputs))
	for i := range outputs {
		if outputs[i].InputIndex > withdrawInput.Index {
			continue
		}
		if outputs[i].Index != uint64(len(payloads)) {
			return report.finish(fmt.Errorf("the rollups node lists output %d where output %d belongs", outputs[i].Index,
				len(payloads)))
		}
		payloads = append(payloads, outputs[i].RawData)
		if outputs[i].InputIndex == withdrawInput.Index {
			if voucher != nil {
				return report.finish(errors.New("the withdrawal input emitted more than one output"))
			}
			voucher = &outputs[i]
		}
	}
	if voucher == nil {
		return report.finish(errors.New("the withdrawal input emitted no output"))
	}
	root := outputsMerkleRoot(payloads)
	rootStep := continuationStep{Name: "outputs root", Status: checkPass,
		Detail: fmt.Sprintf("independent root %s over %d outputs", root, len(payloads))}
	if withdrawInput.TxBufferDataBlock == nil || *withdrawInput.TxBufferDataBlock != root {
		rootStep.Status = checkFail
		rootStep.Detail += fmt.Sprintf("; node tx-buffer word %v", withdrawInput.TxBufferDataBlock)
	}
	report.add(rootStep)

	epochIndex := withdrawInput.EpochIndex
	err = s.waitFor(ctx, fmt.Sprintf("acceptance of epoch %d", epochIndex), func() (bool, error) {
		epochs, err := s.api.epochs(ctx, s.appName)
		if err != nil {
			return false, err
		}
		for _, epoch := range epochs {
			if epoch.Index == epochIndex {
				return epoch.Status == model.EpochStatus_ClaimAccepted, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return report.finish(err)
	}
	report.add(continuationStep{Name: "epoch accepted", Status: checkPass, Detail: fmt.Sprintf("epoch %d", epochIndex)})

	out, err = s.runCLI(ctx, "execute", app.Hex(), strconv.FormatUint(voucher.Index, 10), "--yes", "--json")
	if err != nil {
		return report.finish(err)
	}
	executeTx, err := parseCLITransaction(out)
	if err != nil {
		return report.finish(err)
	}
	receipt, err := s.chain.waitReceipt(ctx, executeTx, receiptTimeout)
	if err != nil {
		return report.finish(err)
	}
	executeStep := continuationStep{Name: "execute voucher", Status: checkPass, Tx: executeTx.Hex(), Block: receipt.BlockNumber.Uint64()}
	executed, err := parseOutputExecuted(receipt, app)
	switch {
	case err != nil:
		executeStep.Status, executeStep.Detail = checkFail, err.Error()
	case executed.OutputIndex != voucher.Index:
		executeStep.Status = checkFail
		executeStep.Detail = fmt.Sprintf("OutputExecuted index %d, want %d", executed.OutputIndex, voucher.Index)
	default:
		executeStep.Detail = fmt.Sprintf("OutputExecuted for output %d", voucher.Index)
	}
	if transferred, ok := findTransfer(receipt, token, withdrawer.Address); !ok || transferred.Cmp(amount) != 0 {
		executeStep.Status = checkFail
		executeStep.Detail += fmt.Sprintf("; token Transfer to the withdrawer %v, want %s", transferred, amount)
	}
	report.add(executeStep)

	appAfter, err := s.chain.tokenBalance(ctx, token, app)
	if err != nil {
		return report.finish(err)
	}
	withdrawerAfter, err := s.chain.tokenBalance(ctx, token, withdrawer.Address)
	if err != nil {
		return report.finish(err)
	}
	balanceStep := continuationStep{Name: stepNameBalances, Status: checkPass,
		Detail: fmt.Sprintf("application %s -> %s, withdrawer %s -> %s", appBefore, appAfter, withdrawerBefore, withdrawerAfter)}
	if new(big.Int).Sub(withdrawerAfter, withdrawerBefore).Cmp(amount) != 0 || appAfter.Cmp(appBefore) != 0 {
		balanceStep.Status = checkFail
	}
	report.add(balanceStep)

	indexed := continuationStep{Name: "node indexes the execution", Tx: executeTx.Hex()}
	err = s.waitFor(ctx, indexed.Name, func() (bool, error) {
		outputs, err := s.api.outputs(ctx, s.appName)
		if err != nil {
			return false, err
		}
		for _, output := range outputs {
			if output.Index == voucher.Index && output.ExecutionTransactionHash != nil {
				if *output.ExecutionTransactionHash != executeTx {
					return false, permanent(fmt.Errorf("execution transaction %s, want %s", output.ExecutionTransactionHash, executeTx))
				}
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		indexed.Status, indexed.Detail = checkFail, err.Error()
	} else {
		indexed.Status, indexed.Detail = checkPass, "execution_transaction_hash recorded"
	}
	report.add(indexed)
	return report.finish(nil)
}

func (s *session) waitInput(ctx context.Context, tx common.Hash) (*model.Input, error) {
	var input *model.Input
	err := s.waitFor(ctx, "input of transaction "+tx.Hex(), func() (bool, error) {
		inputs, err := s.api.inputsByTransaction(ctx, s.appName, tx.Hex())
		if err != nil {
			return false, err
		}
		if len(inputs) != 1 || inputs[0].Status == model.InputCompletionStatus_None {
			return false, nil
		}
		input = &inputs[0]
		return true, nil
	})
	return input, err
}

func inputStep(name string, input *model.Input) continuationStep {
	step := continuationStep{Name: name, Status: checkPass, Tx: input.TransactionHash.Hex(),
		Detail: fmt.Sprintf("input %d in epoch %d is %s", input.Index, input.EpochIndex, input.Status)}
	if input.Status != model.InputCompletionStatus_Accepted {
		step.Status = checkFail
	}
	return step
}

func nameWidth(steps []continuationStep) int {
	width := 0
	for _, s := range steps {
		width = max(width, textWidth(s.Name))
	}
	return min(width, maxNameColumn)
}
