// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package cartesimachine abstracts into an interface the functionalities expected from a machine
// library. It provides an implementation for that interface using the emulator package.
package cartesimachine

import (
	"context"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

type (
	RequestType uint8
	YieldReason uint8
)

type CartesiMachine interface {
	Fork(context.Context) (CartesiMachine, error)
	Continue(context.Context) error
	Run(_ context.Context, until uint64) (emulator.BreakReason, error)
	Close(context.Context) error

	IsAtManualYield(context.Context) (bool, error)
	ReadYieldReason(context.Context) (emulator.HtifYieldReason, error)
	ReadHash(context.Context) ([32]byte, error)
	ReadCycle(context.Context) (uint64, error)
	ReadMemory(context.Context) ([]byte, error)
	WriteRequest(context.Context, []byte, RequestType) error

	PayloadLengthLimit() uint
	Address() string
}
