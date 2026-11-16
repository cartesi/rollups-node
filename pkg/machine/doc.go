// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package machine exposes the rollups-oriented operations of a Cartesi
// machine and translates emulator stops into a small Go contract.
//
// A request that reaches a deterministic guest completion returns a response
// value with CompletionStatusAccepted, CompletionStatusRejected,
// CompletionStatusException, CompletionStatusHalted,
// CompletionStatusOverflow, or CompletionStatusUnexpectedYield. Advance can
// also return CompletionStatusInvalidOutputsRoot when an accepted yield
// declares a root length other than 32 bytes. Anything that
// prevents completion—including deadlines, local resource limits, backend
// failures, and configured cycle exhaustion—returns an error. Advance then
// returns no response; Inspect may return partial reports with
// CompletionStatusUnknown. In short: terminal
// guest outcomes travel as values; incomplete execution travels as an error.
// Callers decide how a completed outcome affects canonical application state.
//
// Unknown automatic yield reasons do not stop execution. The node reads their
// reason from HTIF and does not receive their payload. Advance and Inspect expose
// counts per reason in IgnoredAutomaticYields. Inspect retains these diagnostics
// on incomplete execution; Advance returns no response on error.
// Known output and report payloads must fit the TX buffer and be readable.
// A length above that capacity or an error while reading the header or a known
// payload returns an error for incomplete execution. Automatic reception
// preserves backend I/O error chains. An invalid HTIF word size is an internal
// error. Callers select application status and replay policy from these errors.
//
// PRT advances return canonical collections, as defined in model/input_hash_collection.go.
// PaddingRepetitions counts entries. Machine is the sole canonicalizer;
// downstream layers preserve this representation without repair.
//
// A configured cycle span of zero means that no operator cap shortens the
// emulator's fixed window. The resolved endpoint mirrors emulator 0.21's
// saturating imcyclemax arithmetic. ErrMcycleOverflow preserves the distinct
// emulator-reported fact that imcyclemax, rather than a node target, stopped the
// machine.
package machine
