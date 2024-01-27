// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package gollup

import "github.com/cartesi/rollups-node/pkg/libcmt"

type OutputEmitter interface {
	SendVoucher(address [20]byte, value []byte, data []byte) error
	SendNotice(data []byte) error
}

type ReportEmitter interface {
	SendReport(data []byte) error
}

// ------------------------------------------------------------------------------------------------

type AdvanceHandler func(OutputEmitter, *libcmt.Input) bool

type InspectHandler func(ReportEmitter, *libcmt.Query) bool

type Gollup struct {
	rollup         *libcmt.Rollup
	advanceHandler AdvanceHandler
	inspectHandler InspectHandler
}

func New(advanceHandler AdvanceHandler, inspectHandler InspectHandler) (*Gollup, error) {
	rollup, err := libcmt.NewRollup()
	if err != nil {
		return nil, err
	}
	return &Gollup{rollup, advanceHandler, inspectHandler}, nil
}

func (gollup *Gollup) Destroy() {
	gollup.rollup.Destroy()
}

func (gollup *Gollup) Run() error {
	accept := true
	for {
		finish, err := gollup.rollup.Finish(accept)
		if err != nil {
			return err
		}

		switch finish.NextRequestType {
		case libcmt.AdvanceStateRequest:
			input, err := gollup.rollup.ReadAdvanceState()
			if err != nil {
				return err
			}
			accept = gollup.advanceHandler(gollup, input)
		case libcmt.InspectStateRequest:
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

func (gollup *Gollup) SendVoucher(address [20]byte, value []byte, data []byte) error {
	return gollup.rollup.EmitVoucher(address, value, data)
}

func (gollup *Gollup) SendNotice(data []byte) error {
	return gollup.rollup.EmitNotice(data)
}

func (gollup *Gollup) SendReport(data []byte) error {
	return gollup.rollup.EmitReport(data)
}
