// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package inspectmessages defines the fixed HTTP error messages used by the
// inspect server and retained by clients in protected diagnostics.
package inspectmessages

const (
	MissingApplicationAddress = "Missing application address"
	MethodNotAllowed          = "Method not allowed"
	PayloadTooLarge           = "Payload too large"
	BadRequest                = "Bad request"
	TerminalApplication       = "Application is terminal; inspect unavailable"
	MachineNotReady           = "Machine not ready"
	ForeclosedApplication     = "Application was foreclosed; machine unavailable"
	ApplicationNotFound       = "Application not found"
	InspectAtCapacity         = "Application inspect at capacity"
)
