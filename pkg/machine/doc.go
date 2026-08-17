// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package machine exposes the rollups-oriented operations of a Cartesi
// machine and translates emulator stops into a small Go contract.
//
// A request that reaches a deterministic guest completion returns a response
// value with CompletionStatusAccepted, CompletionStatusRejected,
// CompletionStatusException, CompletionStatusHalted,
// CompletionStatusOverflow, or CompletionStatusUnexpectedYield. Anything that
// prevents completion—including deadlines, local resource limits, backend
// failures, and configured cycle exhaustion—returns an error. Advance then
// returns no response; Inspect may return partial reports with
// CompletionStatusUnknown. In short: terminal
// guest outcomes travel as values; incomplete execution travels as an error.
// Callers decide how a completed outcome affects canonical application state.
//
// For a PRT advance, AdvanceResponse contains a canonical compressed input hash
// collection. PeriodicStateHashes contains the sampled machine root hashes
// before the final canonical root. PaddingRepetitions says how many times that
// final root completes the collection, so
//
//	len(PeriodicStateHashes) + PaddingRepetitions == InputEntryCapacity
//
// and PaddingRepetitions is positive. The machine implementation is the sole
// canonicalizer of this representation; downstream layers validate it but do
// not repair it.
//
// Related terms describe different levels of the same construction. A root
// hash identifies one machine state. Periodic state hashes are sampled roots.
// An input hash collection is those samples plus the repeated final root. The
// DAVE computation hash commits to the larger sequence assembled from these
// collections. Persistence stores the compressed collection as state-hash rows.
//
// A configured cycle span of zero means that no operator cap shortens the
// emulator's fixed window. The resolved endpoint mirrors emulator 0.21's
// saturating imcyclemax arithmetic. ErrMcycleOverflow preserves the distinct
// emulator-reported fact that imcyclemax, rather than a node target, stopped the
// machine.
package machine
