// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package cartesimachine abstracts into an interface the functionalities expected from a machine
// library. It provides an implementation for that interface using the emulator package.
package cartesimachine

import "github.com/cartesi/rollups-node/pkg/emulator"

type (
	RequestType uint8
	YieldReason uint8
)

type CartesiMachine interface {
	Fork() (CartesiMachine, error)
	Continue() error
	Run(until uint64) (emulator.BreakReason, error)
	Close() error

	IsAtManualYield() (bool, error)
	ReadYieldReason() (emulator.HtifYieldReason, error)
	ReadHash() ([32]byte, error)
	ReadCycle() (uint64, error)
	ReadMemory() ([]byte, error)
	WriteRequest([]byte, RequestType) error

	PayloadLengthLimit() uint
	Address() string
}
