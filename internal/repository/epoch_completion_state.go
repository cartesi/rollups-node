// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
)

// EpochCompletionState is a coherent database view used to decide whether an
// epoch can complete. A present application with a missing epoch is inconclusive,
// rather than evidence that its retained transaction is obsolete.
type EpochCompletionState struct {
	Application *model.Application
	Epoch       *model.Epoch
	Problem     *model.ApplicationProblem
}

// AllowsEpochCompletion checks status and the typed boundary. Callers retain
// their existing enable, foreclosure, validation and protocol checks.
func (s *EpochCompletionState) AllowsEpochCompletion() (bool, error) {
	if s == nil || s.Application == nil || s.Epoch == nil {
		return false, fmt.Errorf("application epoch source is unavailable")
	}
	if s.Epoch.ApplicationID != s.Application.ID {
		return false, fmt.Errorf("epoch %d does not belong to application %d", s.Epoch.Index, s.Application.ID)
	}
	status := s.Application.Status
	stopped := status.IsExecutionTerminal() || status == model.ApplicationStatus_InvalidOutputsRoot
	if status != model.ApplicationStatus_OK && !stopped {
		return false, nil
	}
	if s.Problem == nil {
		if stopped {
			return false, fmt.Errorf("application %d has status %s without an epoch stop boundary", s.Application.ID, status)
		}
		return true, nil
	}
	p := s.Problem
	if !p.Kind.IsExecutionTerminal() && p.Kind != model.ApplicationStatus_InvalidOutputsRoot ||
		p.Kind.IsExecutionTerminal() && p.InputIndex == nil {
		return false, fmt.Errorf("application %d has an inconsistent epoch stop boundary", s.Application.ID)
	}
	return s.Epoch.Index < p.EpochIndex, nil
}
