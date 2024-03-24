// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"log/slog"

	"github.com/cartesi/rollups-node/pkg/libcmt"
)

func advance(emitter libcmt.OutputEmitter, input *libcmt.Input) bool {
	slog.Info("Handling advance")
	emitter.SendNotice(input.Data)
	return true
}

func inspect(emitter libcmt.ReportEmitter, query *libcmt.Query) bool {
	slog.Info("Handling inspect")
	emitter.SendReport(query.Data)
	return true
}

func main() {
	slog.Info("=============== Start app.")
	defer slog.Info("=============== End app.")
	gollup, err := libcmt.NewGollup(advance, inspect)
	if err != nil {
		panic(err)
	}
	defer gollup.Destroy()
	err = gollup.Run()
	if err != nil {
		panic(err)
	}
}
