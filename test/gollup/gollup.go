// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package gollup wraps the functionalities provided by libcmt in a rollups-oriented interface.
package gollup

import "github.com/cartesi/rollups-node/test/libcmt"

type (
	Address = libcmt.Address
	Input   = libcmt.Input
	Query   = libcmt.Query
)

type OutputEmitter interface {
	SendVoucher(address Address, value []byte, data []byte) (uint64, error)
	SendNotice(data []byte) (uint64, error)
	SendReport(data []byte) error
	RaiseException(data []byte) error
}

type ReportEmitter interface {
	SendReport(data []byte) error
	RaiseException(data []byte) error
}

// ------------------------------------------------------------------------------------------------

type AdvanceHandler func(OutputEmitter, Input) (accept bool)

type InspectHandler func(ReportEmitter, Query) (accept bool)

type Gollup struct {
	rollup         *libcmt.Rollup
	advanceHandler AdvanceHandler
	inspectHandler InspectHandler
}

// New returns a new [Gollup].
func New(advanceHandler AdvanceHandler, inspectHandler InspectHandler) (*Gollup, error) {
	rollup, err := libcmt.NewRollup()
	if err != nil {
		return nil, err
	}
	return &Gollup{rollup, advanceHandler, inspectHandler}, nil
}

// Close closes the rollup, rendering it unusable.
// Close will return an error if it has already been called.
func (gollup *Gollup) Close() {
	gollup.rollup.Close()
}

// Run runs a loop that perpetually checks for and handles new requests.
func (gollup *Gollup) Run() error {
	accept := true
	for {
		requestType, err := gollup.rollup.Finish(accept)
		if err != nil {
			return err
		}

		switch requestType {
		case libcmt.AdvanceState:
			input, err := gollup.rollup.ReadAdvanceState()
			if err != nil {
				return err
			}
			accept = gollup.advanceHandler(gollup, input)
		case libcmt.InspectState:
			query, err := gollup.rollup.ReadInspectState()
			if err != nil {
				return err
			}
			accept = gollup.inspectHandler(gollup, query)
		default:
			panic("unreachable")
		}
	}
}

// ------------------------------------------------------------------------------------------------

func (gollup *Gollup) SendVoucher(
	address libcmt.Address,
	value []byte,
	data []byte,
) (uint64, error) {
	return gollup.rollup.EmitVoucher(address, value, data)
}

func (gollup *Gollup) SendNotice(data []byte) (uint64, error) {
	return gollup.rollup.EmitNotice(data)
}

func (gollup *Gollup) SendReport(data []byte) error {
	return gollup.rollup.EmitReport(data)
}

func (gollup *Gollup) RaiseException(data []byte) error {
	return gollup.rollup.EmitException(data)
}
