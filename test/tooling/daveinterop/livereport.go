// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cartesi/rollups-node/internal/model"
)

// Step groups: the sections of a live report, in display order.
const (
	groupScenario  = "Scenario"
	groupOutputs   = "Outputs"
	groupFake      = "Fake commitment"
	groupBonds     = "Bonds"
	epoch0Plan     = "no inputs (sealed at deployment)"
	textNoValue    = "–"
	epochTableRule = "  "
)

var groupOrder = []string{groupScenario, groupOutputs, groupFake, groupBonds}

// liveEpoch is one row of the epochs table of a live report: what the
// scenario planned, what the chain shows, and the rollups node's status.
type liveEpoch struct {
	Index    uint64      `json:"index"`
	Plan     string      `json:"plan,omitempty"`
	Inputs   string      `json:"inputs"`
	JoinedBy []string    `json:"joined_by,omitempty"`
	Chain    string      `json:"chain"`
	Node     string      `json:"rollups_node"`
	Status   checkStatus `json:"status,omitempty"`
	Detail   string      `json:"detail,omitempty"`
}

// epochExpectation is what a scenario planned for one epoch: its inputs and
// their outcome, and whether it is accepted.
type epochExpectation struct {
	plan     string
	inputs   map[uint64]model.InputCompletionStatus
	accepted bool
}

// epochTable reads every epoch that the chain or the rollups node knows at
// the head and checks the scenario's expectations.
//
//nolint:gocyclo,funlen // one column per branch
func (s *session) epochTable(ctx context.Context, run *liveRun, head uint64, expect map[uint64]epochExpectation,
) ([]liveEpoch, error) {
	sealedList, err := s.chain.sealedEpochs(ctx, run.consensus, head)
	if err != nil {
		return nil, err
	}
	sealed := sealedByIndex(sealedList)
	nodeEpochs, err := s.api.epochs(ctx, s.appName)
	if err != nil {
		return nil, err
	}
	inputs, err := s.api.inputs(ctx, s.appName)
	if err != nil {
		return nil, err
	}
	nodeStatus := map[uint64]model.EpochStatus{}
	for _, epoch := range nodeEpochs {
		nodeStatus[epoch.Index] = epoch.Status
	}
	byIndex := map[uint64]model.Input{}
	byEpoch := map[uint64][]model.Input{}
	for _, input := range inputs {
		byIndex[input.Index] = input
		byEpoch[input.EpochIndex] = append(byEpoch[input.EpochIndex], input)
	}
	indices := map[uint64]bool{}
	for index := range sealed {
		indices[index] = true
	}
	for index := range nodeStatus {
		indices[index] = true
	}

	rows := make([]liveEpoch, 0, len(indices))
	for _, index := range sortedKeys(indices) {
		row := liveEpoch{Index: index, Plan: expect[index].plan, Inputs: inputCounts(byEpoch[index]), Node: textNoValue}
		switch {
		case row.Plan != "":
		case index == 0:
			row.Plan = epoch0Plan
		case expect == nil && index == run.report.InputEpoch:
			row.Plan = fmt.Sprintf("%d inputs from the input sender", len(run.report.InputTxs))
		}
		if status, ok := nodeStatus[index]; ok {
			row.Node = string(status)
		}
		epoch, isSealed := sealed[index]
		acceptance, accepted := sealed[index+1]
		switch {
		case accepted:
			row.Chain = "accepted"
			if sender, err := s.chain.txSender(ctx, acceptance.Tx); err == nil {
				row.Chain = "accepted by " + run.party(sender)
			}
		case isSealed:
			row.Chain = "sealed"
		default:
			row.Chain = "open"
		}
		if isSealed {
			joins, err := s.chain.joins(ctx, epoch.Tournament, head)
			if err != nil {
				return nil, err
			}
			for _, join := range joins {
				row.JoinedBy = append(row.JoinedBy, run.party(join.Submitter))
			}
		}
		if e, ok := expect[index]; ok {
			row.Status, row.Detail = e.check(index, byIndex, byEpoch[index], isSealed, accepted)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// check compares an epoch with its expectation. An epoch planned to be
// sealed is not tested until it is.
func (e epochExpectation) check(index uint64, byIndex map[uint64]model.Input, inEpoch []model.Input, isSealed, accepted bool,
) (checkStatus, string) {
	if !isSealed {
		return checkNotTested, "not sealed"
	}
	var problems []string
	for _, i := range sortedKeys(e.inputs) {
		want := e.inputs[i]
		got, ok := byIndex[i]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("input %d is not in the rollups node", i))
		case got.Status != want || got.EpochIndex != index:
			problems = append(problems, fmt.Sprintf("input %d is %s in epoch %d, planned %s", i, got.Status, got.EpochIndex, want))
		}
	}
	for _, input := range inEpoch {
		if _, planned := e.inputs[input.Index]; !planned {
			problems = append(problems, fmt.Sprintf("input %d was not planned in this epoch", input.Index))
		}
	}
	if e.accepted && !accepted {
		problems = append(problems, "not accepted on chain")
	}
	if len(problems) > 0 {
		return checkFail, strings.Join(problems, "; ")
	}
	return checkPass, ""
}

// epochExpectations is what the scenario of the run planned for each epoch.
func (run *liveRun) epochExpectations() map[uint64]epochExpectation {
	if run.full != nil {
		return run.full.expectations()
	}
	return nil
}

func (full *fullRun) expectations() map[uint64]epochExpectation {
	expect := map[uint64]epochExpectation{0: {plan: epoch0Plan, accepted: true}}
	for i, plan := range full.plans {
		epoch := uint64(i + 1)
		e := epochExpectation{plan: plan.name, accepted: true, inputs: map[uint64]model.InputCompletionStatus{}}
		for _, input := range full.inputs {
			if input.Epoch != epoch {
				continue
			}
			e.inputs[input.Index] = model.InputCompletionStatus_Rejected
			if plannedAccept(input.Payload) {
				e.inputs[input.Index] = model.InputCompletionStatus_Accepted
			}
		}
		expect[epoch] = e
	}
	return expect
}

// writeText is the text form of a live run: a header with the verdict and
// counts, the epochs table, the scenario's sections, and the verification.
func (r *liveReport) writeText(w io.Writer) {
	st := newTextStyle(w)
	verdict := st.verdict(r.Passed)
	st.heading(w, fmt.Sprintf("live %s · %s", r.Scenario, r.Order), verdict)
	st.field(w, "checks", st.tally(r.statuses()))
	st.field(w, "run dir", displayPath(r.RunDir))
	st.field(w, "app", fmt.Sprintf("%s (%s)", r.Application, r.Program))
	st.field(w, "rollups", r.RollupsSigner.Hex())
	sling := r.SlingSigner.Hex()
	if r.SlingSentry {
		sling += ", the application's sentry"
	}
	st.field(w, "sling", sling)
	if r.Error != "" {
		st.field(w, "error", st.paint(ansiRed, r.Error))
	}

	if len(r.Epochs) > 0 {
		st.section(w, "Epochs", "")
		r.writeEpochs(w, st)
	}
	for _, group := range stepGroups(r.Steps) {
		st.section(w, group.name, "")
		width := nameWidth(group.steps)
		for _, s := range group.steps {
			st.row(w, s.Status, s.Name, width, s.Detail)
		}
	}
	if r.Verify != nil {
		r.Verify.writeSection(w, st, "Verification", "the rollups node against the chain and the Sling node's evidence")
	}
}

func (r *liveReport) writeEpochs(w io.Writer, st textStyle) {
	header := []string{"epoch", "plan", "inputs", "joined by", "on chain", "rollups node"}
	cells := make([][]string, 0, len(r.Epochs))
	for _, e := range r.Epochs {
		joined := strings.Join(e.JoinedBy, ", ")
		if joined == "" {
			joined = textNoValue
		}
		cells = append(cells, []string{fmt.Sprint(e.Index), valueOr(e.Plan, textNoValue), e.Inputs, joined, e.Chain, e.Node})
	}
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = textWidth(h)
		for _, row := range cells {
			widths[i] = max(widths[i], textWidth(row[i]))
		}
	}
	line := func(mark string, row []string) string {
		parts := make([]string, len(row))
		for i, cell := range row {
			parts[i] = pad(cell, widths[i])
		}
		return strings.TrimRight("  "+mark+"  "+strings.Join(parts, epochTableRule), " ")
	}
	fmt.Fprintln(w, st.paint(ansiDim, line(" ", header)))
	for i, e := range r.Epochs {
		fmt.Fprintln(w, line(st.mark(e.Status), cells[i]))
		if e.Status == checkFail {
			for _, line := range wrapWords(e.Detail, max(st.width-markColumn, minDetailWidth)) {
				fmt.Fprintln(w, strings.Repeat(" ", markColumn)+st.paint(ansiRed, line))
			}
		}
	}
	if r.OrderChecked.Name != "" {
		fmt.Fprintln(w)
		detail := strings.Replace(r.OrderChecked.Detail, "first join by", fmt.Sprintf("first join of epoch %d by", r.InputEpoch), 1)
		st.row(w, r.OrderChecked.Status, "join order", textWidth("join order"), detail)
	}
}

// statuses counts every check of the report, for the header.
func (r *liveReport) statuses() map[checkStatus]int {
	counts := map[checkStatus]int{}
	for _, e := range r.Epochs {
		if e.Status != "" {
			counts[e.Status]++
		}
	}
	if r.OrderChecked.Name != "" {
		counts[r.OrderChecked.Status]++
	}
	for _, s := range r.Steps {
		counts[s.Status]++
	}
	if r.Verify != nil {
		for _, c := range r.Verify.Checks {
			counts[c.Status]++
		}
	}
	return counts
}

type stepGroup struct {
	name  string
	steps []continuationStep
}

// stepGroups sorts steps into their sections, in groupOrder and then in
// order of appearance. A step without a group belongs to the scenario.
func stepGroups(steps []continuationStep) []stepGroup {
	byName := map[string][]continuationStep{}
	var seen []string
	for _, s := range steps {
		group := valueOr(s.Group, groupScenario)
		if _, ok := byName[group]; !ok {
			seen = append(seen, group)
		}
		byName[group] = append(byName[group], s)
	}
	var groups []stepGroup
	for _, name := range append(append([]string{}, groupOrder...), seen...) {
		if list, ok := byName[name]; ok {
			groups = append(groups, stepGroup{name: name, steps: list})
			delete(byName, name)
		}
	}
	return groups
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
