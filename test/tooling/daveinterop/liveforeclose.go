// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cartesi/rollups-node/internal/model"
)

// The foreclose live scenario: the ERC-20 withdrawal dapp, deposits and a
// withdrawal in epoch 1, deposits in epoch 2, then the guardian forecloses
// the application before epoch 2 is accepted. Every token must come back:
// through the epoch 1 voucher, emergency withdrawals from the accounts drive
// of epoch 1, and refunds of the epoch 2 deposits.

const (
	scenarioForeclose = "foreclose"

	// defaultForecloseDapp is built by "make erc20-withdrawal-dapp".
	defaultForecloseDapp = "applications/erc20-withdrawal-dapp"

	defaultGuardianIx = 1

	// The accounts drive of the ERC-20 withdrawal dapp: a 4 MiB flash drive
	// of 2^17 32-byte records.
	accountsDriveSize            = 4 << 20
	accountsDriveLog2Size        = 22
	accountsLog2MaxNum           = 17
	accountsLog2LeavesPerAccount = 0

	// withdrawSelector starts a withdrawal input of the dapp: 0x01 followed
	// by the amount as 8 big-endian bytes.
	withdrawSelector = 0x01

	sentryGraceTime = 20 * time.Second
)

// depositor is one account of the foreclose scenario.
type depositor struct {
	name     string
	index    uint32
	account  *testAccount
	baseline *big.Int // token balance after the mint
}

// forecloseInput is one input that the scenario sent.
type forecloseInput struct {
	Index    uint64
	Epoch    uint64
	Who      *depositor
	Amount   uint64
	Withdraw bool
	Raw      []byte
	Tx       common.Hash
}

// forecloseRun is the configuration and the progress of the scenario.
type forecloseRun struct {
	token, portal, builder common.Address
	driveStart             uint64
	guardian               *testAccount
	a, b, c                *depositor
	appBaseline            *big.Int
	inputs                 []forecloseInput
	foreclosure            chainEvent
	sealed                 map[uint64]sealedEpoch
}

// newForecloseRun reads the devnet addresses and the accounts drive of the
// template, and checks that the dapp was built for those addresses.
func newForecloseRun(daveRoot, template string) (*forecloseRun, error) {
	fc := &forecloseRun{sealed: map[uint64]sealedEpoch{}}
	var err error
	for name, target := range map[string]*common.Address{"TestUsdc": &fc.token, "Erc20Portal": &fc.portal,
		"TestUsdWithdrawalOutputBuilder": &fc.builder} {
		if *target, err = readDeployment(daveRoot, name); err != nil {
			return nil, err
		}
	}
	raw, err := os.ReadFile(filepath.Join(template, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("reading the template config: %w", err)
	}
	if fc.driveStart, err = accountsDriveStart(raw); err != nil {
		return nil, err
	}
	lower := strings.ToLower(string(raw))
	for name, address := range map[string]common.Address{"TestUsdc": fc.token, "Erc20Portal": fc.portal} {
		if !strings.Contains(lower, strings.ToLower(address.Hex()[2:])) {
			return nil, fmt.Errorf("the template was built for another %s than %s; rebuild it with make erc20-withdrawal-dapp",
				name, address)
		}
	}
	if fc.guardian, err = deriveTestAccount(testMnemonic, defaultGuardianIx); err != nil {
		return nil, err
	}
	for _, d := range []struct {
		target **depositor
		name   string
		index  uint32
	}{{&fc.a, "A", 2}, {&fc.b, "B", 8}, {&fc.c, "C", 9}} {
		account, err := deriveTestAccount(testMnemonic, d.index)
		if err != nil {
			return nil, err
		}
		*d.target = &depositor{name: d.name, index: d.index, account: account}
	}
	return fc, nil
}

// accountsDriveStart finds the accounts drive in a template config.json and
// returns its start as an index of drive-sized nodes.
func accountsDriveStart(configJSON []byte) (uint64, error) {
	var cfg struct {
		Config struct {
			FlashDrive []struct {
				Start  uint64 `json:"start"`
				Length uint64 `json:"length"`
			} `json:"flash_drive"`
		} `json:"config"`
	}
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return 0, fmt.Errorf("parsing the template config: %w", err)
	}
	for _, drive := range cfg.Config.FlashDrive {
		if drive.Length == accountsDriveSize {
			if drive.Start%accountsDriveSize != 0 {
				return 0, fmt.Errorf("the accounts drive at %#x is not aligned to its size", drive.Start)
			}
			return drive.Start >> accountsDriveLog2Size, nil
		}
	}
	return 0, fmt.Errorf("the template has no %d-byte accounts drive", accountsDriveSize)
}

// withdrawalConfig is the --withdrawal-config of the deployment.
func (fc *forecloseRun) withdrawalConfig() string {
	return fmt.Sprintf(`{"guardian":"%s","log2_leaves_per_account":%d,"log2_max_num_of_accounts":%d,`+
		`"accounts_drive_start_index":%d,"withdrawal_output_builder":"%s"}`, fc.guardian.Address.Hex(),
		accountsLog2LeavesPerAccount, accountsLog2MaxNum, fc.driveStart, fc.builder.Hex())
}

// liveForeclose runs the scenario and freezes the chain.
//
//nolint:funlen,gocyclo // one linear procedure is easier to follow than scattered helpers
func (s *session) liveForeclose(ctx context.Context, run *liveRun) (uint64, error) {
	fc := run.foreclose
	var steps []continuationStep
	add := func(step continuationStep) {
		steps = append(steps, step)
		logger.Info(fmt.Sprintf("foreclose: %s %s: %s", step.Status, step.Name, step.Detail))
	}
	defer func() { run.report.Steps = append(run.report.Steps, steps...) }()

	// Mint what each depositor deposits, and one more for B's deposit after
	// the foreclosure, which must revert; the balance after the mint is the
	// balance each depositor must end with.
	for _, mint := range []struct {
		who    *depositor
		amount int64
	}{{fc.a, 100}, {fc.b, 221}, {fc.c, 330}} {
		tx, err := s.chain.mintToken(ctx, fc.token, mint.who.account, big.NewInt(mint.amount))
		if err != nil {
			return 0, err
		}
		if _, err := s.chain.waitReceipt(ctx, tx, receiptTimeout); err != nil {
			return 0, fmt.Errorf("minting TestUsdc for %s: %w", mint.who.name, err)
		}
		if mint.who.baseline, err = s.chain.tokenBalance(ctx, fc.token, mint.who.account.Address); err != nil {
			return 0, err
		}
	}
	var err error
	if fc.appBaseline, err = s.chain.tokenBalance(ctx, fc.token, run.app); err != nil {
		return 0, err
	}
	step("live", "minted TestUsdc: A 100, B 221, C 330")

	// Epoch 1: three deposits, then A withdraws its whole balance.
	for _, deposit := range []struct {
		who    *depositor
		amount uint64
	}{{fc.a, 100}, {fc.b, 200}, {fc.c, 300}} {
		if err := s.depositTokens(ctx, run, deposit.who, deposit.amount, 1); err != nil {
			return 0, err
		}
	}
	if err := s.requestWithdrawal(ctx, run, fc.a, 100, 1); err != nil { //nolint:mnd // A's whole balance
		return 0, err
	}
	step("live", "epoch 1: deposits A 100, B 200, C 300; A withdraws 100; waiting for it to be sealed")
	if fc.sealed[1], err = s.waitPlannedEpoch(ctx, run, 1, 0, 4); err != nil { //nolint:mnd // inputs 0-3
		return 0, err
	}
	run.report.InputEpoch = 1
	if err := s.startSecondNode(ctx, run, fc.sealed[1].Tournament); err != nil {
		return 0, err
	}

	// Epoch 2: two deposits, sealed when epoch 1 is accepted.
	for _, deposit := range []struct {
		who    *depositor
		amount uint64
	}{{fc.b, 20}, {fc.c, 30}} {
		if err := s.depositTokens(ctx, run, deposit.who, deposit.amount, 2); err != nil { //nolint:mnd // epoch 2
			return 0, err
		}
	}
	step("live", "epoch 2: deposits B 20, C 30; waiting for epoch 1 to be accepted")
	if fc.sealed[2], err = s.waitPlannedEpoch(ctx, run, 2, 4, 6); err != nil { //nolint:mnd // inputs 4-5
		return 0, err
	}

	// Foreclose at once: epoch 2 must not be accepted first.
	add(s.forecloseApplication(ctx, run))
	if fc.foreclosure.Block == 0 {
		return 0, fmt.Errorf("the foreclosure failed; see %s", displayPath(filepath.Join(s.runDir, "cli.log")))
	}
	add(expectRevert("deposit after the foreclosure", "ApplicationForeclosed", func() error {
		_, err := s.runCLIAs(ctx, fc.b.index, "deposit", "erc20", s.appName, "--portal", fc.portal.Hex(), "--token",
			fc.token.Hex(), "--amount", "1", "--approve", "--yes", "--json")
		return err
	}))

	add(s.executeWithdrawalVoucher(ctx, run))
	proofs, replayStep := s.proveAccountsDrive(ctx, run)
	add(replayStep)
	add(s.proveDriveRoot(ctx, proofs))
	add(s.emergencyWithdrawals(ctx, run, proofs))
	add(s.refundDeposits(ctx, run))
	add(s.tokensBack(ctx, run))

	// Epoch 2's tournament goes on after the foreclosure. Only when it ends
	// do the nodes try to settle the epoch, which the foreclosure forbids,
	// and does its joiner get the bond back.
	step("live", "waiting for epoch 2's tournament to end: then the nodes try to settle it, and its bond must come back")
	if err := s.waitBondRecoveries(ctx, run, 2); err != nil { //nolint:mnd // epochs 0-2
		logger.Warn("not every bond was recovered; the bond checks will say which", "error", err)
	}
	drain := s.waitDrain(ctx, run)
	drain.Group = groupNodes
	add(drain)
	step("live", "watching what the nodes send for %s more", sentryGraceTime)
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(sentryGraceTime):
	}
	head, err := s.freezeLive(ctx, run)
	if err != nil {
		return 0, err
	}
	step("live", "chain frozen at block %d", head)

	steps = append(steps, s.afterForeclosureSteps(ctx, run, head)...)
	sealed, err := s.chain.sealedEpochs(ctx, run.consensus, head)
	if err != nil {
		return head, err
	}
	steps = append(steps, s.bondSteps(ctx, run, head, 2, sealedByIndex(sealed))...) //nolint:mnd // epochs 0-2
	return head, s.recordJoinOrder(ctx, run, fc.sealed[1].Tournament, head)
}

// depositTokens deposits TestUsdc through the ERC-20 portal with the rollups
// CLI, signed by the depositor, and records the input.
func (s *session) depositTokens(ctx context.Context, run *liveRun, who *depositor, amount, epoch uint64) error {
	fc := run.foreclose
	out, err := s.runCLIAs(ctx, who.index, "deposit", "erc20", s.appName, "--portal", fc.portal.Hex(), "--token", fc.token.Hex(),
		"--amount", strconv.FormatUint(amount, 10), "--approve", "--yes", "--json")
	if err != nil {
		return err
	}
	tx, err := parseCLITransaction(out)
	if err != nil {
		return err
	}
	return s.recordInput(ctx, run, forecloseInput{Epoch: epoch, Who: who, Amount: amount, Tx: tx})
}

// requestWithdrawal sends the dapp's withdrawal input from the depositor.
func (s *session) requestWithdrawal(ctx context.Context, run *liveRun, who *depositor, amount, epoch uint64) error {
	payload := make([]byte, 9)
	payload[0] = withdrawSelector
	binary.BigEndian.PutUint64(payload[1:], amount)
	tx, err := s.chain.addInput(ctx, s.manifest.Deployments.InputBox, run.app, who.account, payload)
	if err != nil {
		return err
	}
	if _, err := s.chain.waitReceipt(ctx, tx, receiptTimeout); err != nil {
		return err
	}
	return s.recordInput(ctx, run, forecloseInput{Epoch: epoch, Who: who, Amount: amount, Withdraw: true, Tx: tx})
}

// recordInput finds the InputAdded event of a transaction.
func (s *session) recordInput(ctx context.Context, run *liveRun, input forecloseInput) error {
	header, err := s.chain.head(ctx)
	if err != nil {
		return err
	}
	all, err := s.chain.inputsAdded(ctx, s.manifest.Deployments.InputBox, run.app, header.Number.Uint64())
	if err != nil {
		return err
	}
	for _, added := range all {
		if added.Tx == input.Tx {
			input.Index, input.Raw = added.Index, added.Raw
			run.foreclose.inputs = append(run.foreclose.inputs, input)
			run.report.InputTxs = append(run.report.InputTxs, input.Tx)
			return nil
		}
	}
	return fmt.Errorf("no InputAdded event for transaction %s", input.Tx)
}

// waitPlannedEpoch waits until the epoch is sealed and checks its inputs.
func (s *session) waitPlannedEpoch(ctx context.Context, run *liveRun, index, lower, upper uint64) (sealedEpoch, error) {
	sealed, err := s.waitSealed(ctx, run, index)
	if err != nil {
		return sealed, err
	}
	if sealed.Lower != lower || sealed.Upper != upper {
		return sealed, fmt.Errorf("epoch %d was sealed with inputs [%d,%d), planned [%d,%d); the plan is broken",
			index, sealed.Lower, sealed.Upper, lower, upper)
	}
	return sealed, nil
}

// forecloseApplication forecloses the application as the guardian and checks
// that epoch 2 was still unaccepted.
func (s *session) forecloseApplication(ctx context.Context, run *liveRun) continuationStep {
	fc := run.foreclose
	result := continuationStep{Name: "foreclosure", Status: checkFail, Sender: fc.guardian.Address.Hex()}
	out, err := s.runCLIAs(ctx, defaultGuardianIx, "foreclose", s.appName, "--yes", "--json")
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	tx, err := parseCLITransaction(out)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	receipt, err := s.chain.waitReceipt(ctx, tx, receiptTimeout)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	block := receipt.BlockNumber.Uint64()
	fc.foreclosure = chainEvent{Block: block, Tx: tx}
	result.Tx, result.Block = tx.Hex(), block
	sealed, err := s.chain.sealedEpochs(ctx, run.consensus, block)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	if _, accepted := sealedByIndex(sealed)[3]; accepted {
		result.Detail = "epoch 2 was accepted before the foreclosure; its deposits are final and cannot be refunded"
		return result
	}
	foreclosed, err := s.chain.isForeclosed(ctx, run.app, block)
	if err != nil || !foreclosed {
		result.Detail = fmt.Sprintf("isForeclosed is %t (%v)", foreclosed, err)
		return result
	}
	result.Status = checkPass
	result.Detail = fmt.Sprintf("the guardian foreclosed the application at block %d; epoch 1 accepted, epoch 2 sealed and "+
		"never accepted", block)
	return result
}

// expectRevert runs a call that must fail with the named error.
func expectRevert(name, revert string, call func() error) continuationStep {
	result := continuationStep{Name: name, Status: checkPass, Detail: "reverted with " + revert}
	err := call()
	switch {
	case err == nil:
		result.Status, result.Detail = checkFail, "succeeded; expected "+revert
	case !strings.Contains(err.Error(), revert):
		result.Status, result.Detail = checkFail, fmt.Sprintf("failed without %s: %v", revert, err)
	}
	return result
}

// executeWithdrawalVoucher executes the voucher of A's withdrawal, which
// epoch 1 finalized before the foreclosure.
func (s *session) executeWithdrawalVoucher(ctx context.Context, run *liveRun) continuationStep {
	fc := run.foreclose
	result := continuationStep{Name: "voucher of epoch 1", Status: checkFail}
	var withdrawal forecloseInput
	for _, input := range fc.inputs {
		if input.Withdraw {
			withdrawal = input
		}
	}
	outputs, err := s.api.outputs(ctx, s.appName)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	var voucher *model.Output
	for i := range outputs {
		if outputs[i].InputIndex == withdrawal.Index && outputKind(outputs[i].RawData) == outputVoucher {
			voucher = &outputs[i]
		}
	}
	if voucher == nil {
		result.Detail = fmt.Sprintf("input %d has no voucher", withdrawal.Index)
		return result
	}
	tx, receipt, err := s.executeVoucher(ctx, run, *voucher)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	result.Tx, result.Block = tx.Hex(), receipt.BlockNumber.Uint64()
	amount, ok := findTransfer(receipt, fc.token, fc.a.account.Address)
	if !ok || amount.Uint64() != withdrawal.Amount {
		result.Detail = fmt.Sprintf("TestUsdc transfer to A: %v, expected %d", amount, withdrawal.Amount)
		return result
	}
	err = s.waitFor(ctx, "the rollups node to index the voucher execution", func() (bool, error) {
		current, err := s.api.outputs(ctx, s.appName)
		if err != nil {
			return false, err
		}
		for _, output := range current {
			if output.Index == voucher.Index && output.ExecutionTransactionHash != nil {
				return *output.ExecutionTransactionHash == tx, nil
			}
		}
		return false, nil
	})
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	result.Status = checkPass
	result.Detail = fmt.Sprintf("output %d executed after the foreclosure: %d TestUsdc to A; the rollups node records it",
		voucher.Index, withdrawal.Amount)
	return result
}

// accountProofs are the machine tool's proof files.
type accountProofs struct {
	driveRoot string
	withdraw  map[string]string // by depositor name
}

// proveAccountsDrive rebuilds the machine state of epoch 1 with the machine
// tool and proves the accounts of B and C (A withdrew everything).
func (s *session) proveAccountsDrive(ctx context.Context, run *liveRun) (*accountProofs, continuationStep) {
	fc := run.foreclose
	result := continuationStep{Name: "accounts drive of epoch 1", Status: checkFail}
	epochs, err := s.api.epochs(ctx, s.appName)
	if err != nil {
		result.Detail = err.Error()
		return nil, result
	}
	var machineHash common.Hash
	for _, epoch := range epochs {
		if epoch.Index == 1 && epoch.MachineHash != nil {
			machineHash = *epoch.MachineHash
		}
	}
	header, err := s.chain.head(ctx)
	if err != nil {
		result.Detail = err.Error()
		return nil, result
	}
	finalized, err := s.chain.lastFinalizedMachineRoot(ctx, run.consensus, run.app, header.Number.Uint64())
	if err != nil {
		result.Detail = err.Error()
		return nil, result
	}
	if machineHash != finalized {
		result.Detail = fmt.Sprintf("the last finalized machine root %s is not epoch 1's %s", finalized, machineHash)
		return nil, result
	}
	store := filepath.Join(s.runDir, "epoch-1-machine")
	out, err := s.runTool(ctx, run.opts.machineToolBin, "machine-tool.log", s.env, "replay", "--template",
		filepath.Join(s.runDir, "template-rollups"), "--application", s.appName, "--to-epoch", "1", "--store", store)
	if err != nil {
		result.Detail = err.Error()
		return nil, result
	}
	var replayed struct {
		ProcessedInputs uint64 `json:"processed_inputs"`
		MachineRoot     string `json:"machine_root"`
	}
	if err := json.Unmarshal([]byte(out), &replayed); err != nil {
		result.Detail = fmt.Sprintf("parsing the replay output %q: %v", out, err)
		return nil, result
	}
	if common.HexToHash(replayed.MachineRoot) != finalized || replayed.ProcessedInputs != 4 {
		result.Detail = fmt.Sprintf("replay: %d inputs, machine root %s; expected 4 and %s", replayed.ProcessedInputs,
			replayed.MachineRoot, finalized)
		return nil, result
	}
	proofs := &accountProofs{withdraw: map[string]string{}}
	for _, who := range []*depositor{fc.b, fc.c} {
		drive := filepath.Join(s.runDir, "drive-root-proof.json")
		withdraw := filepath.Join(s.runDir, "withdraw-proof-"+who.name+".json")
		if _, err := s.proveAccount(ctx, run, store, who, drive, withdraw); err != nil {
			result.Detail = err.Error()
			return nil, result
		}
		proofs.driveRoot, proofs.withdraw[who.name] = drive, withdraw
	}
	_, err = s.proveAccount(ctx, run, store, fc.a, filepath.Join(s.runDir, "drive-root-proof-A.json"),
		filepath.Join(s.runDir, "withdraw-proof-A.json"))
	switch {
	case err == nil:
		result.Detail = "A withdrew its whole balance, but the accounts drive still has an account for it"
		return nil, result
	case !strings.Contains(err.Error(), "account not found"):
		result.Detail = "proving A: " + err.Error()
		return nil, result
	}
	result.Status = checkPass
	result.Detail = fmt.Sprintf("the machine tool replayed 4 inputs to the last finalized state %s; accounts of B and C proved; "+
		"A has no account", shortHash(finalized.Hex()))
	return proofs, result
}

func (s *session) proveAccount(ctx context.Context, run *liveRun, store string, who *depositor, drive, withdraw string,
) (string, error) {
	return s.runTool(ctx, run.opts.machineToolBin, "machine-tool.log", s.env, "prove", "accounts-drive", "--snapshot", store,
		"--accounts-drive-start-index", strconv.FormatUint(run.foreclose.driveStart, 10),
		"--log2-max-num-of-accounts", strconv.Itoa(accountsLog2MaxNum),
		"--log2-leaves-per-account", strconv.Itoa(accountsLog2LeavesPerAccount),
		"--account", who.account.Address.Hex(), "--out-drive-root-proof", drive, "--out-withdraw-proof", withdraw)
}

// proveDriveRoot proves the accounts-drive root on chain, once.
func (s *session) proveDriveRoot(ctx context.Context, proofs *accountProofs) continuationStep {
	result := continuationStep{Name: "accounts-drive proof", Status: checkFail}
	if proofs == nil {
		result.Status, result.Detail = checkNotTested, "no proofs"
		return result
	}
	tx, err := s.cliTransaction(ctx, "prove-drive-root", s.appName, "--proof-file", proofs.driveRoot, "--yes", "--json")
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	result.Tx = tx.Hex()
	again := expectRevert("", "AccountsDriveMerkleRootAlreadyProved", func() error {
		_, err := s.runCLI(ctx, "prove-drive-root", s.appName, "--proof-file", proofs.driveRoot, "--yes", "--json")
		return err
	})
	if again.Status == checkFail {
		result.Detail = "a second proof: " + again.Detail
		return result
	}
	err = s.waitFor(ctx, "the rollups node to index the accounts-drive proof", func() (bool, error) {
		app, err := s.api.application(ctx, s.appName)
		return err == nil && app.AccountsDriveProvedTransaction != nil && *app.AccountsDriveProvedTransaction == tx, err
	})
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	result.Status = checkPass
	result.Detail = "proved once; a second proof reverted with AccountsDriveMerkleRootAlreadyProved; the rollups node records it"
	return result
}

// cliTransaction runs a CLI command that sends a transaction and waits for
// its receipt.
func (s *session) cliTransaction(ctx context.Context, args ...string) (common.Hash, error) {
	out, err := s.runCLI(ctx, args...)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := parseCLITransaction(out)
	if err != nil {
		return common.Hash{}, err
	}
	_, err = s.chain.waitReceipt(ctx, tx, receiptTimeout)
	return tx, err
}

// emergencyWithdrawals withdraws the epoch 1 balances of B and C.
func (s *session) emergencyWithdrawals(ctx context.Context, run *liveRun, proofs *accountProofs) continuationStep {
	fc := run.foreclose
	result := continuationStep{Name: "emergency withdrawals", Status: checkFail}
	if proofs == nil {
		result.Status, result.Detail = checkNotTested, "no proofs"
		return result
	}
	for _, w := range []struct {
		who    *depositor
		amount uint64
	}{{fc.b, 200}, {fc.c, 300}} {
		tx, err := s.cliTransaction(ctx, "withdraw", s.appName, "--proof-file", proofs.withdraw[w.who.name], "--yes", "--json")
		if err != nil {
			result.Detail = fmt.Sprintf("%s: %v", w.who.name, err)
			return result
		}
		receipt, err := s.chain.eth.TransactionReceipt(ctx, tx)
		if err != nil {
			result.Detail = err.Error()
			return result
		}
		if amount, ok := findTransfer(receipt, fc.token, w.who.account.Address); !ok || amount.Uint64() != w.amount {
			result.Detail = fmt.Sprintf("%s received %v TestUsdc, expected %d", w.who.name, amount, w.amount)
			return result
		}
	}
	again := expectRevert("", "AccountFundsAlreadyWithdrawn", func() error {
		_, err := s.runCLI(ctx, "withdraw", s.appName, "--proof-file", proofs.withdraw[fc.b.name], "--yes", "--json")
		return err
	})
	if again.Status == checkFail {
		result.Detail = "withdrawing B again: " + again.Detail
		return result
	}
	err := s.waitFor(ctx, "the rollups node to index the withdrawals", func() (bool, error) {
		withdrawals, err := s.api.withdrawals(ctx, s.appName)
		return len(withdrawals) == 2, err //nolint:mnd // B and C
	})
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	result.Status = checkPass
	result.Detail = "B received 200 and C 300 TestUsdc; withdrawing again reverted with AccountFundsAlreadyWithdrawn; " +
		"the rollups node lists both"
	return result
}

// refundDeposits refunds the epoch 2 deposits, which were never finalized.
func (s *session) refundDeposits(ctx context.Context, run *liveRun) continuationStep {
	fc := run.foreclose
	result := continuationStep{Name: "refunds of epoch 2", Status: checkFail}
	inputFile := func(input forecloseInput) (string, error) {
		path := filepath.Join(s.runDir, fmt.Sprintf("input-%d.hex", input.Index))
		return path, os.WriteFile(path, []byte("0x"+common.Bytes2Hex(input.Raw)), 0o600) //nolint:mnd
	}
	var refunded []string
	for _, input := range fc.inputs {
		if input.Epoch != 2 { //nolint:mnd // epoch 2
			continue
		}
		path, err := inputFile(input)
		if err != nil {
			result.Detail = err.Error()
			return result
		}
		tx, err := s.cliTransaction(ctx, "refund", s.appName, strconv.FormatUint(input.Index, 10), "--input-file", path,
			"--yes", "--json")
		if err != nil {
			result.Detail = fmt.Sprintf("input %d: %v", input.Index, err)
			return result
		}
		receipt, err := s.chain.eth.TransactionReceipt(ctx, tx)
		if err != nil {
			result.Detail = err.Error()
			return result
		}
		if amount, ok := findTransfer(receipt, fc.token, input.Who.account.Address); !ok || amount.Uint64() != input.Amount {
			result.Detail = fmt.Sprintf("input %d: %s received %v TestUsdc, expected %d", input.Index, input.Who.name, amount,
				input.Amount)
			return result
		}
		refunded = append(refunded, fmt.Sprintf("input %d: %d to %s", input.Index, input.Amount, input.Who.name))
	}
	for _, negative := range []struct {
		input  forecloseInput
		revert string
	}{{fc.inputs[0], "CannotRefundFinalizedInput"}, {fc.inputs[4], "RefundAlreadyIssued"}} {
		path, err := inputFile(negative.input)
		if err != nil {
			result.Detail = err.Error()
			return result
		}
		check := expectRevert("", negative.revert, func() error {
			_, err := s.runCLI(ctx, "refund", s.appName, strconv.FormatUint(negative.input.Index, 10), "--input-file", path,
				"--yes", "--json")
			return err
		})
		if check.Status == checkFail {
			result.Detail = fmt.Sprintf("refunding input %d: %s", negative.input.Index, check.Detail)
			return result
		}
	}
	header, err := s.chain.head(ctx)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	issued, err := s.chain.refundsIssued(ctx, run.app, header.Number.Uint64())
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	if len(issued) != len(refunded) {
		result.Detail = fmt.Sprintf("%d RefundIssued events for %d refunds", len(issued), len(refunded))
		return result
	}
	result.Status = checkPass
	result.Detail = strings.Join(refunded, "; ") + "; refunding epoch 1's input 0 reverted with CannotRefundFinalizedInput, " +
		"and input 4 again with RefundAlreadyIssued"
	return result
}

// tokensBack requires every depositor to end with its balance after the
// mint, and the application with what it held before the deposits.
func (s *session) tokensBack(ctx context.Context, run *liveRun) continuationStep {
	fc := run.foreclose
	result := continuationStep{Name: "tokens back", Status: checkPass}
	var parts []string
	for _, who := range []*depositor{fc.a, fc.b, fc.c} {
		balance, err := s.chain.tokenBalance(ctx, fc.token, who.account.Address)
		if err != nil {
			result.Status, result.Detail = checkFail, err.Error()
			return result
		}
		parts = append(parts, fmt.Sprintf("%s %s", who.name, balance))
		if balance.Cmp(who.baseline) != 0 {
			result.Status = checkFail
			parts[len(parts)-1] += fmt.Sprintf(" (expected %s)", who.baseline)
		}
	}
	appBalance, err := s.chain.tokenBalance(ctx, fc.token, run.app)
	if err != nil {
		result.Status, result.Detail = checkFail, err.Error()
		return result
	}
	parts = append(parts, fmt.Sprintf("application %s", appBalance))
	if appBalance.Cmp(fc.appBaseline) != 0 {
		result.Status = checkFail
		parts[len(parts)-1] += fmt.Sprintf(" (expected %s)", fc.appBaseline)
	}
	result.Detail = "TestUsdc after everything: " + strings.Join(parts, ", ") + "; every depositor has all its tokens back"
	if result.Status == checkFail {
		result.Detail = "TestUsdc after everything: " + strings.Join(parts, ", ")
	}
	return result
}

// waitDrain waits until the rollups node ends every unaccepted epoch as
// CLAIM_FORECLOSED: epoch 2, sealed but never accepted, and epoch 3, which
// was open at the foreclosure.
func (s *session) waitDrain(ctx context.Context, run *liveRun) continuationStep {
	result := continuationStep{Name: "rollups node closes the epochs", Status: checkPass}
	var drained []uint64
	waitCtx, cancel := context.WithTimeout(ctx, run.opts.timeout)
	defer cancel()
	err := s.waitFor(waitCtx, "the rollups node to drain the foreclosed epochs", func() (bool, error) {
		epochs, err := s.api.epochs(ctx, s.appName)
		if err != nil {
			return false, err
		}
		drained = drained[:0]
		for _, epoch := range epochs {
			switch {
			case epoch.Index < 2: //nolint:mnd // epochs 0 and 1 were accepted
			case epoch.Status == model.EpochStatus_ClaimForeclosed:
				drained = append(drained, epoch.Index)
			default:
				return false, nil
			}
		}
		return len(drained) > 0, nil
	})
	if err != nil {
		result.Status, result.Detail = checkFail, err.Error()
		return result
	}
	parts := make([]string, 0, len(drained))
	for _, index := range drained {
		if _, sealed := run.foreclose.sealed[index]; sealed {
			parts = append(parts, fmt.Sprintf("epoch %d (sealed, never accepted)", index))
		} else {
			parts = append(parts, fmt.Sprintf("epoch %d (open at the foreclosure)", index))
		}
	}
	result.Detail = strings.Join(parts, " and ") + " are CLAIM_FORECLOSED; epochs 0 and 1 stay CLAIM_ACCEPTED"
	return result
}

// afterForeclosureSteps reports every transaction that each node sent after
// the foreclosure. The rollups node must send none that reverts. The Sling
// node's reverted transactions are a finding about the Sling node, not a
// failure of the run.
func (s *session) afterForeclosureSteps(ctx context.Context, run *liveRun, head uint64) []continuationStep {
	fc := run.foreclose
	sent, err := s.chain.transactionsFrom(ctx, []common.Address{run.rollupsSigner, run.sling.Address}, fc.foreclosure.Block, head)
	if err != nil {
		return []continuationStep{{Name: "transactions after the foreclosure", Status: checkFail, Detail: err.Error()}}
	}
	window := fmt.Sprintf("blocks %d to %d", fc.foreclosure.Block+1, head)
	rollups := continuationStep{Name: "rollups node transactions", Status: checkPass, Group: groupNodes}
	summary := summarizeTxs(sent[run.rollupsSigner])
	switch {
	case summary.count == 0:
		rollups.Detail = "no transactions in " + window
	case summary.failed > 0:
		rollups.Status = checkFail
		rollups.Detail = fmt.Sprintf("%s, in %s: %s", summary.revertedText(), window, summary.text)
	default:
		rollups.Detail = fmt.Sprintf("%s, none reverted, in %s: %s", countText(summary.count, "transaction"), window, summary.text)
	}
	sling := continuationStep{Name: "Sling node transactions", Status: checkPass, Group: groupNodes}
	summary = summarizeTxs(sent[run.sling.Address])
	switch {
	case summary.count == 0:
		sling.Status, sling.Detail = checkNotTested, "no transactions in "+window
	case summary.failed > 0:
		sling.Status = checkFinding
		sling.Detail = fmt.Sprintf("%s, in %s: %s. The Sling node does not check for the foreclosure and keeps sending "+
			"calls that it makes revert.", summary.revertedText(), window, summary.text)
	default:
		sling.Detail = fmt.Sprintf("%s, none reverted, in %s: %s", countText(summary.count, "transaction"), window, summary.text)
	}
	return []continuationStep{rollups, sling}
}
