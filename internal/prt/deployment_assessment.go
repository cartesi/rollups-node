// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"slices"

	"github.com/ethereum/go-ethereum/common"
)

// The selected head is checked separately from the decoded evidence identity.
func sameDeploymentFacts(a, b DeploymentEvidence) bool {
	return a.Consensus == b.Consensus && a.FactoryKnown == b.FactoryKnown && a.Factory == b.Factory &&
		a.Alpha6Shape == b.Alpha6Shape &&
		a.Transition == b.Transition &&
		a.MarchID == b.MarchID &&
		a.Pristine == b.Pristine &&
		a.Count == b.Count && a.Candidate == b.Candidate && a.Cause == b.Cause &&
		slices.Equal(a.Rows, b.Rows) && (a.Malformed == nil && b.Malformed == nil ||
		a.Malformed != nil && b.Malformed != nil && *a.Malformed == *b.Malformed)
}

// planDeploymentAssessment changes no shared state or durable data.
// A failed read updates diagnostic facts without erasing the previous conclusive evidence.
func planDeploymentAssessment(
	previous deploymentSnapshot, evidence DeploymentEvidence, readErr error, previousHeadCanonical bool,
) deploymentSnapshot {
	next := previous
	if next.Assessment.State == "" {
		next.Assessment.State = deploymentNone
	}
	if evidence.FactoryKnown && next.Alpha6Shape && evidence.Factory != next.ShapeFactory {
		next.Alpha6Shape = false
		next.ShapeFactory = common.Address{}
	}
	if evidence.FactoryKnown && evidence.Alpha6Shape {
		next.Alpha6Shape, next.ShapeFactory = true, evidence.Factory
	}
	if readErr != nil || evidence.Candidate == deploymentNone || evidence.Candidate == "" ||
		next.Assessment.confirmed() {
		return next
	}
	if evidence.Candidate == deploymentCompatible {
		next.Assessment = deploymentAssessment{State: deploymentCompatible}
		next.Head, next.Cause = evidence.Head, evidence.Cause
		return next
	}
	if evidence.Candidate != deploymentGeneration && evidence.Candidate != deploymentTable {
		return next
	}
	prior := next.Assessment
	next.Head, next.Cause = evidence.Head, evidence.Cause
	if hasDeploymentConfirmationPair(prior, evidence) && previousHeadCanonical {
		next.Assessment = deploymentAssessment{State: evidence.Candidate}
	} else {
		next.Assessment = deploymentAssessment{State: deploymentPending, PendingEvidence: new(evidence)}
	}
	return next
}

func hasDeploymentConfirmationPair(prior deploymentAssessment, evidence DeploymentEvidence) bool {
	return prior.State == deploymentPending && prior.PendingEvidence != nil &&
		prior.PendingEvidence.Head.Hash != evidence.Head.Hash && sameDeploymentFacts(*prior.PendingEvidence, evidence)
}
