// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"
)

// Money checks of the full live scenario: the balances that funding and
// executing vouchers move, and the bonds of every root tournament.

// defaultFunderIx is the test account that funds the application.
const defaultFunderIx = 4

const (
	gweiInWei     = 1_000_000_000
	milliEtherWei = 1_000_000_000_000_000
	etherDecimals = 18
)

var weiPerGwei = big.NewInt(gweiInWei)

// decodeVoucher returns the destination and value of an encoded voucher.
func decodeVoucher(raw []byte) (common.Address, *big.Int, error) {
	parsed, err := outputs.OutputsMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, err
	}
	method := parsed.Methods["Voucher"]
	if !bytes.HasPrefix(raw, method.ID) {
		return common.Address{}, nil, errors.New("not a voucher")
	}
	values, err := method.Inputs.Unpack(raw[len(method.ID):])
	if err != nil {
		return common.Address{}, nil, err
	}
	destination, ok := values[0].(common.Address)
	value, ok2 := values[1].(*big.Int)
	if !ok || !ok2 {
		return common.Address{}, nil, errors.New("unexpected voucher fields")
	}
	return destination, value, nil
}

// voucherLedger follows the balances that funding and executing vouchers
// move. Between its two readings only the funding and the executions touch
// these accounts, so every change is exact, gas included.
type voucherLedger struct {
	block      uint64 // balances were read here, before the funding
	parties    []ledgerParty
	funder     common.Address
	executor   common.Address
	funded     *big.Int
	fundingGas *big.Int
	// transferred sums the values of the executed vouchers per destination.
	transferred map[common.Address]*big.Int
	executorGas *big.Int
	executions  int
}

type ledgerParty struct {
	name    string
	address common.Address
	before  *big.Int
}

// fundVouchers reads the balances of every account involved and funds the
// application with the total value of its vouchers.
func (s *session) fundVouchers(ctx context.Context, run *liveRun, all []model.Output) (*voucherLedger, error) {
	l := &voucherLedger{funded: new(big.Int), fundingGas: new(big.Int), transferred: map[common.Address]*big.Int{},
		executorGas: new(big.Int)}
	destinations := map[common.Address]bool{}
	for _, output := range all {
		if outputKind(output.RawData) != outputVoucher {
			continue
		}
		destination, value, err := decodeVoucher(output.RawData)
		if err != nil {
			return nil, fmt.Errorf("output %d: %w", output.Index, err)
		}
		l.funded.Add(l.funded, value)
		destinations[destination] = true
	}
	funder, err := deriveTestAccount(testMnemonic, defaultFunderIx)
	if err != nil {
		return nil, err
	}
	executor, err := s.cliSigner()
	if err != nil {
		return nil, err
	}
	l.funder, l.executor = funder.Address, executor.Address
	l.parties = []ledgerParty{{name: "application", address: run.app}, {name: "input sender", address: run.sender.Address},
		{name: "funder (account 4)", address: funder.Address}, {name: "executor (CLI signer)", address: executor.Address}}
	for destination := range destinations {
		if destination != run.sender.Address {
			l.parties = append(l.parties, ledgerParty{name: "voucher destination", address: destination})
		}
	}
	header, err := s.chain.head(ctx)
	if err != nil {
		return nil, err
	}
	l.block = header.Number.Uint64()
	for i := range l.parties {
		if l.parties[i].before, err = s.chain.balanceAt(ctx, l.parties[i].address, l.block); err != nil {
			return nil, err
		}
	}
	if l.funded.Sign() == 0 {
		return l, nil
	}
	tx, err := s.chain.sendValue(ctx, funder, run.app, l.funded)
	if err != nil {
		return nil, err
	}
	receipt, err := s.chain.waitReceipt(ctx, tx, receiptTimeout)
	if err != nil {
		return nil, fmt.Errorf("funding the application: %w", err)
	}
	l.fundingGas = gasCost(receipt)
	step("live", "funded the application with %s for its vouchers", formatWei(l.funded))
	return l, nil
}

// addExecution records one execute transaction: its gas, and, when it
// succeeded, the value that its voucher moved.
func (l *voucherLedger) addExecution(receipt *types.Receipt, raw []byte) {
	if l == nil || receipt == nil {
		return
	}
	l.executorGas.Add(l.executorGas, gasCost(receipt))
	if receipt.Status != types.ReceiptStatusSuccessful {
		return
	}
	l.executions++
	if destination, value, err := decodeVoucher(raw); err == nil {
		if l.transferred[destination] == nil {
			l.transferred[destination] = new(big.Int)
		}
		l.transferred[destination].Add(l.transferred[destination], value)
	}
}

// expectedChanges is what each account must have gained (or lost) between
// the two readings.
func (l *voucherLedger) expectedChanges(app common.Address) map[common.Address]*big.Int {
	want := map[common.Address]*big.Int{}
	add := func(account common.Address, delta *big.Int) {
		if want[account] == nil {
			want[account] = new(big.Int)
		}
		want[account].Add(want[account], delta)
	}
	add(app, l.funded)
	for destination, value := range l.transferred {
		add(app, new(big.Int).Neg(value))
		add(destination, value)
	}
	add(l.funder, new(big.Int).Neg(new(big.Int).Add(l.funded, l.fundingGas)))
	add(l.executor, new(big.Int).Neg(l.executorGas))
	return want
}

// closeLedger reads the balances again and compares every change with the
// expected one.
func (s *session) closeLedger(ctx context.Context, run *liveRun, l *voucherLedger) continuationStep {
	result := continuationStep{Name: stepNameBalances, Status: checkPass}
	header, err := s.chain.head(ctx)
	if err != nil {
		result.Status, result.Detail = checkFail, err.Error()
		return result
	}
	want := l.expectedChanges(run.app)
	var parts, failures []string
	for _, party := range l.parties {
		after, err := s.chain.balanceAt(ctx, party.address, header.Number.Uint64())
		if err != nil {
			result.Status, result.Detail = checkFail, err.Error()
			return result
		}
		got := new(big.Int).Sub(after, party.before)
		expected := want[party.address]
		if expected == nil {
			expected = new(big.Int)
		}
		parts = append(parts, fmt.Sprintf("%s %s", party.name, signedWei(got)))
		if got.Cmp(expected) != 0 {
			failures = append(failures, fmt.Sprintf("%s changed by %s, expected %s", party.name, signedWei(got), signedWei(expected)))
		}
	}
	result.Detail = fmt.Sprintf("between blocks %d and %d: %s (funding %s plus %s gas; %d executions cost %s gas)",
		l.block, header.Number, strings.Join(parts, ", "), formatWei(l.funded), formatWei(l.fundingGas), l.executions,
		formatWei(l.executorGas))
	if len(failures) > 0 {
		result.Status = checkFail
		result.Detail = strings.Join(failures, "; ") + "; " + result.Detail
	}
	return result
}

// cliSigner is the account that the rollups CLI signs with.
func (s *session) cliSigner() (*testAccount, error) {
	if kind := lookupEnv(s.env, "CARTESI_AUTH_KIND"); kind != "" && kind != authKindMnemonic {
		return nil, fmt.Errorf("CARTESI_AUTH_KIND is %s; the balance check needs the CLI mnemonic", kind)
	}
	index, err := strconv.ParseUint(envValueOr(s.env, "CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX", "0"), 10, 32)
	if err != nil {
		return nil, err
	}
	return deriveTestAccount(envValueOr(s.env, "CARTESI_AUTH_MNEMONIC", testMnemonic), uint32(index))
}

// waitBondRecoveries waits, while the chain still moves, until the root
// tournament of every epoch from 0 to last has recovered its bond, or has
// finished with nobody joined (after a foreclosure, the rollups node does
// not join).
func (s *session) waitBondRecoveries(ctx context.Context, run *liveRun, last uint64) error {
	ctx, cancel := context.WithTimeout(ctx, run.opts.timeout)
	defer cancel()
	return s.waitFor(ctx, fmt.Sprintf("the bond recoveries of epochs 0 to %d", last), func() (bool, error) {
		header, err := s.chain.head(ctx)
		if err != nil {
			return false, err
		}
		head := header.Number.Uint64()
		sealed, err := s.chain.sealedEpochs(ctx, run.consensus, head)
		if err != nil {
			return false, err
		}
		byIndex := sealedByIndex(sealed)
		for n := uint64(0); n <= last; n++ {
			epoch, ok := byIndex[n]
			if !ok {
				return false, nil
			}
			recoveries, err := s.chain.bondRecoveries(ctx, epoch.Tournament, head)
			if err != nil {
				return false, err
			}
			if len(recoveries) > 0 {
				continue
			}
			if done, err := s.finishedEmpty(ctx, epoch.Tournament, head); err != nil || !done {
				return false, err
			}
		}
		return true, nil
	})
}

// bondSteps checks the bond of the root tournament of every epoch from 0 to
// last. Every join pays one bond; dispute actions refund
// their gas from that balance (PartialBondRefund); at the end the winner's
// joiner receives the whole balance when it is at most one bond, and one
// bond plus a tenth of the rest otherwise; the remainder is burned, and the
// tournament is left empty.
func (s *session) bondSteps(ctx context.Context, run *liveRun, head, last uint64, sealed map[uint64]sealedEpoch,
) []continuationStep {
	var steps []continuationStep
	for n := uint64(0); n <= last; n++ {
		epoch, ok := sealed[n]
		if !ok {
			steps = append(steps, continuationStep{Name: fmt.Sprintf("epoch %d", n), Status: checkNotTested, Group: groupBonds,
				Detail: "not sealed"})
			continue
		}
		steps = append(steps, s.bondStep(ctx, run, n, epoch.Tournament, head))
	}
	return steps
}

//nolint:gocyclo // one check per bond rule
func (s *session) bondStep(ctx context.Context, run *liveRun, index uint64, tournament common.Address, head uint64,
) continuationStep {
	result := continuationStep{Name: fmt.Sprintf("epoch %d", index), Status: checkFail, Group: groupBonds}
	bond, err := s.chain.bondValue(ctx, tournament, head)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	joins, err := s.chain.joins(ctx, tournament, head)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	refunded, refundedTo, err := s.chain.partialRefunds(ctx, tournament, head)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	recoveries, err := s.chain.bondRecoveries(ctx, tournament, head)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	standing, err := s.chain.rootStanding(ctx, tournament, head)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	left, err := s.chain.balanceAt(ctx, tournament, head)
	if err != nil {
		result.Detail = err.Error()
		return result
	}

	if len(joins) == 0 {
		if standing.Finished && len(recoveries) == 0 && left.Sign() == 0 {
			result.Status, result.Detail = checkPass, "nobody joined; no bond to recover"
		} else {
			result.Detail = fmt.Sprintf("nobody joined, but finished %t, %d BondRecovered events, %s held",
				standing.Finished, len(recoveries), formatWei(left))
		}
		return result
	}
	wantPayment, wantBurned := bondPayout(bond, len(joins), refunded)
	winnerJoiner := common.Address{}
	for _, join := range joins {
		if standing.HasWinner && join.Commitment == standing.Winner {
			winnerJoiner = join.Submitter
		}
	}

	var problems []string
	switch {
	case !standing.HasWinner:
		problems = append(problems, "the root tournament has no winner")
	case len(recoveries) != 1:
		problems = append(problems, fmt.Sprintf("%d BondRecovered events, expected 1", len(recoveries)))
	}
	if len(recoveries) == 1 {
		r := recoveries[0]
		result.Tx = r.Tx.Hex()
		if r.Commitment != standing.Winner || r.Claimer != winnerJoiner {
			problems = append(problems, fmt.Sprintf("recovered by %s for %s, expected the winner's joiner %s",
				run.party(r.Claimer), r.Commitment, run.party(winnerJoiner)))
		}
		if r.Payment.Cmp(wantPayment) != 0 || r.Burned.Cmp(wantBurned) != 0 {
			problems = append(problems, fmt.Sprintf("paid %s and burned %s, expected %s and %s",
				formatWei(r.Payment), formatWei(r.Burned), formatWei(wantPayment), formatWei(wantBurned)))
		}
	}
	if left.Sign() != 0 {
		problems = append(problems, fmt.Sprintf("the tournament still holds %s", formatWei(left)))
	}

	result.Detail = fmt.Sprintf("%d %s × %s bond", len(joins), plural(len(joins), "join", "joins"), formatWei(bond))
	for recipient, value := range refundedTo {
		result.Detail += fmt.Sprintf("; %s refunded to %s", formatWei(value), run.party(recipient))
	}
	result.Detail += fmt.Sprintf("; %s to the winner's joiner %s", formatWei(wantPayment), run.party(winnerJoiner))
	if wantBurned.Sign() > 0 {
		result.Detail += fmt.Sprintf("; %s burned", formatWei(wantBurned))
	}
	if len(problems) > 0 {
		result.Detail = strings.Join(problems, "; ") + "; " + result.Detail
		return result
	}
	result.Status = checkPass
	result.Detail += "; empty afterwards"
	return result
}

// finishedEmpty tells whether a root tournament finished with nobody joined.
func (s *session) finishedEmpty(ctx context.Context, tournament common.Address, head uint64) (bool, error) {
	joins, err := s.chain.joins(ctx, tournament, head)
	if err != nil || len(joins) > 0 {
		return false, err
	}
	standing, err := s.chain.rootStanding(ctx, tournament, head)
	return standing.Finished, err
}

// bondPayout is Tournament.tryRecoveringBond: of the balance left after the
// gas refunds, the winner's joiner receives all of it when it is at most one
// bond, and one bond plus a tenth of the rest otherwise; the remainder is
// burned.
func bondPayout(bond *big.Int, joins int, refunded *big.Int) (payment, burned *big.Int) {
	balance := new(big.Int).Mul(bond, big.NewInt(int64(joins)))
	balance.Sub(balance, refunded)
	payment = new(big.Int).Set(balance)
	if balance.Cmp(bond) > 0 {
		payment.Add(bond, new(big.Int).Div(new(big.Int).Sub(balance, bond), big.NewInt(10))) //nolint:mnd // a tenth
	}
	return payment, new(big.Int).Sub(balance, payment)
}

// formatWei renders an amount in gwei when it is a whole number of gwei
// below 0.001 ETH, and in ETH otherwise.
func formatWei(value *big.Int) string {
	if value == nil {
		return "0"
	}
	abs := new(big.Int).Abs(value)
	if abs.Cmp(big.NewInt(milliEtherWei)) < 0 && new(big.Int).Mod(abs, weiPerGwei).Sign() == 0 {
		return new(big.Int).Div(value, weiPerGwei).String() + " gwei"
	}
	ether := new(big.Float).Quo(new(big.Float).SetInt(value), big.NewFloat(1e18)) //nolint:mnd // wei per ether
	text := strings.TrimRight(strings.TrimRight(ether.Text('f', etherDecimals), "0"), ".")
	return text + " ETH"
}

func signedWei(value *big.Int) string {
	if value.Sign() > 0 {
		return "+" + formatWei(value)
	}
	return formatWei(value)
}
