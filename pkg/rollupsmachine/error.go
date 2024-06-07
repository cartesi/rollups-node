// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package rollupsmachine

import (
	"errors"
	"fmt"

	. "github.com/cartesi/rollups-node/pkg/model"
)

const unreachable = "internal error: entered unreacheable code"

var (
	ErrMaxCycles  = errors.New("reached limit cycles")
	ErrMaxOutputs = fmt.Errorf("reached maximum number of emitted outputs (%d)", maxOutputs)
	ErrHashSize   = fmt.Errorf("hash does not have exactly %d bytes", HashSize)

	ErrFailed              = errors.New("machine failed")
	ErrHalted              = errors.New("machine halted")
	ErrYieldedWithProgress = errors.New("machine yielded with progress")
	ErrYieldedSoftly       = errors.New("machine yielded softly")

	// Load (and isReadyForRequests) errors
	ErrNewRemoteMachineManager     = errors.New("could not create the remote machine manager")
	ErrRemoteLoadMachine           = errors.New("remote server was not able to load the machine")
	ErrNotReadyForRequests         = errors.New("machine is not ready to receive requests")
	ErrNotAtManualYield            = errors.New("not at manual yield")
	ErrLastInputWasRejected        = errors.New("last input was rejected")
	ErrLastInputYieldedAnException = errors.New("last input yielded an exception")

	// Fork errors
	ErrFork       = errors.New("could not fork the machine")
	ErrOrphanFork = errors.New("forked cartesi machine was left orphan")

	// Destroy errors
	ErrRemoteShutdown = errors.New("could not shut down the remote machine")
	ErrMachineDestroy = errors.New("could not destroy the inner machine")
)
