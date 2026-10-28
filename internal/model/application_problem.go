// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

// ApplicationProblem identifies the earliest known epoch that cannot publish
// local results. It survives a later integrity stop; it is not a status history.
type ApplicationProblem struct {
	EpochIndex uint64
	Kind       ApplicationStatus
	InputIndex *uint64
}
