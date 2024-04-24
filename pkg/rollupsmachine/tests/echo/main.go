// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"log/slog"

	"github.com/cartesi/rollups-node/pkg/gollup"
	"github.com/cartesi/rollups-node/pkg/libcmt"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/tests/echo/protocol"
)

var counter = 0

func log(s string) {
	slog.Info("============================== " + s)
}

func advance(emitter gollup.OutputEmitter, input *libcmt.Input) bool {
	counter++

	inputData := protocol.FromBytes[protocol.InputData](input.Data)
	if inputData.Reject {
		return false
	}
	if inputData.Exception {
		emitter.RaiseException(protocol.AdvanceException)
		return false
	}

	for i := 0; i < inputData.Vouchers; i++ {
		voucherData := protocol.VoucherData{Counter: counter, Quote: inputData.Quote, Index: i}
		bytes, err := voucherData.ToBytes()
		if err != nil {
			panic(err)
		}
		emitter.SendVoucher(input.Sender, protocol.VoucherValue.Bytes(), bytes)
	}
	for i := 0; i < inputData.Notices; i++ {
		noticeData := protocol.NoticeData{Counter: counter, Quote: inputData.Quote, Index: i}
		bytes, err := noticeData.ToBytes()
		if err != nil {
			panic(err)
		}
		emitter.SendNotice(bytes)
	}
	for i := 0; i < inputData.Reports; i++ {
		reportData := protocol.Report{Counter: counter, Quote: inputData.Quote, Index: i}
		bytes, err := reportData.ToBytes()
		if err != nil {
			panic(err)
		}
		emitter.SendReport(bytes)
	}

	return true
}

func inspect(emitter gollup.ReportEmitter, query *libcmt.Query) bool {
	queryData := protocol.FromBytes[protocol.QueryData](query.Data)
	if queryData.Reject {
		return false
	}
	if queryData.Exception {
		emitter.RaiseException(protocol.InspectException)
		return false
	}

	for i := 0; i < queryData.Reports; i++ {
		reportData := protocol.Report{Counter: counter, Quote: queryData.Quote, Index: i}
		bytes, err := reportData.ToBytes()
		if err != nil {
			panic(err)
		}
		emitter.SendReport(bytes)
	}
	return true
}

func main() {
	log("Start app.")
	defer log("End app.")
	gollup, err := gollup.New(advance, inspect)
	if err != nil {
		panic(err)
	}
	defer gollup.Destroy()
	err = gollup.Run()
	if err != nil {
		panic(err)
	}
}
