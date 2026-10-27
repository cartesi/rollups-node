// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"math/big"
	"slices"
	"testing"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func deploymentTestHead(number uint64) txsender.BlockRef {
	return txsender.BlockRef{Number: number, Hash: common.BigToHash(new(big.Int).SetUint64(number + 1))}
}

func tableDeploymentEvidence(head txsender.BlockRef) DeploymentEvidence {
	e := compatibleDeploymentEvidence(head)
	e.Candidate = deploymentTable
	e.Rows = []DeploymentParameters{{Levels: 2, Log2Step: 44, Height: 48, ResponseBudget: 11, CommitmentBudget: 22, MaxAllowance: 33},
		{Levels: 2, Log2Step: 0, Height: 44, ResponseBudget: 11, CommitmentBudget: 22, MaxAllowance: 33}}
	return e
}

func TestDeploymentAssessmentDistinctHeadsFullEvidenceAndLatch(t *testing.T) {
	first := tableDeploymentEvidence(deploymentTestHead(100))
	state := planDeploymentAssessment(deploymentSnapshot{}, first, nil, true)
	require.Equal(t, deploymentPending, state.Assessment.State)
	repeated := planDeploymentAssessment(state, first, nil, true)
	require.Equal(t, deploymentPending, repeated.Assessment.State)
	unavailable := DeploymentEvidence{Head: deploymentTestHead(101), Candidate: deploymentNone}
	for _, cause := range []error{nil, errors.New("RPC read failed"), txsender.ErrUnsupported} {
		next := planDeploymentAssessment(state, unavailable, cause, true)
		require.Equal(t, state.Assessment, next.Assessment)
		require.Equal(t, first, *next.Assessment.PendingEvidence)
	}
	changed := first.clone()
	changed.Head = deploymentTestHead(102)
	changed.Rows[1].MaxAllowance++
	restarted := planDeploymentAssessment(state, changed, nil, true)
	require.Equal(t, deploymentPending, restarted.Assessment.State)
	require.Equal(t, changed.Head, restarted.Assessment.PendingEvidence.Head)
	require.NotEqual(t, state.Assessment.PendingEvidence, restarted.Assessment.PendingEvidence)
	second := first.clone()
	second.Head = deploymentTestHead(103)
	failedRecheck := planDeploymentAssessment(state, second, errors.New("canonical head unavailable"), true)
	require.Equal(t, state.Assessment, failedRecheck.Assessment)
	require.Equal(t, state.Head, failedRecheck.Head)
	confirmed := planDeploymentAssessment(state, second, nil, true)
	require.Equal(t, deploymentTable, confirmed.Assessment.State)
	require.Nil(t, confirmed.Assessment.PendingEvidence)
	require.Equal(t, second.Head, confirmed.Head)
	require.Nil(t, confirmed.Assessment.PendingEvidence)
	compatible := compatibleDeploymentEvidence(deploymentTestHead(104))
	retained := planDeploymentAssessment(confirmed, compatible, nil, true)
	require.Equal(t, confirmed.Assessment, retained.Assessment)
	require.Equal(t, confirmed.Head, retained.Head)
}

func TestDeploymentAssessmentCanonicalityGenesisAndEarlyShape(t *testing.T) {
	first := tableDeploymentEvidence(deploymentTestHead(0))
	previous := planDeploymentAssessment(deploymentSnapshot{}, first, nil, true)
	second := first.clone()
	second.Head = deploymentTestHead(1)
	invalid := planDeploymentAssessment(previous, second, nil, false)
	require.Equal(t, deploymentPending, invalid.Assessment.State)
	require.Equal(t, second.Head, invalid.Assessment.PendingEvidence.Head)
	valid := planDeploymentAssessment(previous, second, nil, true)
	require.Equal(t, deploymentTable, valid.Assessment.State)
	partial := first.clone()
	partial.Candidate = deploymentNone
	partial.Rows = partial.Rows[:1]
	shaped := planDeploymentAssessment(deploymentSnapshot{}, partial, errors.New("late table RPC failure"), true)
	require.Equal(t, deploymentNone, shaped.Assessment.State)
	require.True(t, shaped.Alpha6Shape)
	missing := DeploymentEvidence{Head: deploymentTestHead(2), Candidate: deploymentNone}
	retained := planDeploymentAssessment(shaped, missing, errors.New("factory read failure"), true)
	require.True(t, retained.Alpha6Shape)
	require.Equal(t, first.Factory, retained.ShapeFactory)
	replaced := missing
	replaced.FactoryKnown = true
	replaced.Factory = common.HexToAddress("0x991")
	other := planDeploymentAssessment(shaped, replaced, nil, true)
	require.False(t, other.Alpha6Shape)
	compatible := planDeploymentAssessment(deploymentSnapshot{}, compatibleDeploymentEvidence(deploymentTestHead(4)), nil, true)
	require.Equal(t, deploymentCompatible, compatible.Assessment.State)
	errored := planDeploymentAssessment(compatible, missing, txsender.ErrUnsupported, true)
	require.Equal(t, compatible.Assessment, errored.Assessment)
	require.Equal(t, compatible.Head, errored.Head)
}

func TestDeploymentFactsCompareDecodedFieldsAndMalformedIdentity(t *testing.T) {
	adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
	evidence, err := adapter.ReadDeployment(t.Context(), b.head)
	require.NoError(t, err)
	evidence.Malformed = &DeploymentCallEvidence{ReturnLength: 1}
	moved := evidence.clone()
	moved.Head = deploymentTestHead(200)
	require.True(t, sameDeploymentFacts(evidence, moved))
	mutations := []struct {
		name   string
		change func(*DeploymentEvidence)
	}{
		{"factory known", func(e *DeploymentEvidence) { e.FactoryKnown = false }},
		{"alpha shape", func(e *DeploymentEvidence) { e.Alpha6Shape = false }},
		{"source factory address", func(e *DeploymentEvidence) { e.Factory[0]++ }},
		{"transition", func(e *DeploymentEvidence) { e.Transition[0]++ }},
		{"marchid", func(e *DeploymentEvidence) { e.MarchID++ }},
		{"pristine", func(e *DeploymentEvidence) { e.Pristine[0]++ }},
		{"count", func(e *DeploymentEvidence) { e.Count++ }},
		{"levels", func(e *DeploymentEvidence) { e.Rows[1].Levels++ }},
		{"child stride word", func(e *DeploymentEvidence) { e.Rows[1].Log2Step++ }},
		{"child height word", func(e *DeploymentEvidence) { e.Rows[1].Height++ }},
		{"response", func(e *DeploymentEvidence) { e.Rows[1].ResponseBudget++ }},
		{"commitment budget word", func(e *DeploymentEvidence) { e.Rows[1].CommitmentBudget++ }},
		{"allowance", func(e *DeploymentEvidence) { e.Rows[1].MaxAllowance++ }},
		{"reply length", func(e *DeploymentEvidence) { e.Malformed.ReturnLength++ }},
		{"reply digest", func(e *DeploymentEvidence) { e.Malformed.ReturnHash[0]++ }},
		{"target", func(e *DeploymentEvidence) { e.Malformed.Target[0]++ }},
		{"selector", func(e *DeploymentEvidence) { e.Malformed.Selector[0]++ }},
		{"row argument flag", func(e *DeploymentEvidence) { e.Malformed.HasLevel = true }},
		{"row argument", func(e *DeploymentEvidence) { e.Malformed.Level++ }},
		{"candidate", func(e *DeploymentEvidence) { e.Candidate = deploymentTable }},
		{"cause", func(e *DeploymentEvidence) { e.Cause = deploymentCauseGeometry }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			changed := evidence.clone()
			tc.change(&changed)
			require.False(t, sameDeploymentFacts(evidence, changed))
		})
	}
}

func TestDeploymentFactsExcludeProviderErrorText(t *testing.T) {
	facts := make([]DeploymentEvidence, 0, 2)
	for _, message := range []string{"temporary archive failure", "execution reverted: incompatible MARCHID"} {
		adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
		request := testDeploymentRequest(b.transition, testDeploymentSelector(testMarchSignature))
		b.overrides[request] = deploymentRPCReply{err: deploymentRPCError{-32000, message}}
		e, err := adapter.ReadDeployment(t.Context(), b.head)
		require.Error(t, err)
		require.Equal(t, deploymentNone, e.Candidate)
		facts = append(facts, e)
	}
	require.True(t, sameDeploymentFacts(facts[0], facts[1]))
}

func (e DeploymentEvidence) clone() DeploymentEvidence {
	e.Rows = slices.Clone(e.Rows)
	if e.Malformed != nil {
		e.Malformed = new(*e.Malformed)
	}
	return e
}
