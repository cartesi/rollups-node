// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package logger provides different log levels by wrapping the default [log.Logger]. There are
// four levels, in decreasing order of priority: Error, Warning, Info, and Debug.
// Error and Warning write to [os.Stderr] and Info and Debug write to [os.Stdout]. If Debug is set
// as the default level, all logs include file and line number data.
//
// The configuration of the log comes from the config package.
package logger

import (
	"io"
	"log"
	"os"

	"github.com/cartesi/rollups-node/internal/config"
)

var (
	Error   *log.Logger
	Warning *log.Logger
	Info    *log.Logger
	Debug   *log.Logger
)

func init() {
	var flags int
	if config.GetLogTimestamp() {
		flags |= log.Ldate | log.Ltime
	}

	Error = log.New(os.Stderr, "ERROR ", flags)
	Warning = log.New(os.Stderr, "WARN ", flags)
	Info = log.New(os.Stdout, "INFO ", flags)
	Debug = log.New(os.Stdout, "DEBUG ", flags)

	switch config.GetLogLevel() {
	case config.LogLevelError:
		Warning.SetOutput(io.Discard)
		fallthrough
	case config.LogLevelWarning:
		Info.SetOutput(io.Discard)
		fallthrough
	case config.LogLevelInfo:
		Debug.SetOutput(io.Discard)
	case config.LogLevelDebug:
		flags |= log.Llongfile
		Error.SetFlags(flags)
		Warning.SetFlags(flags)
		Info.SetFlags(flags)
		Debug.SetFlags(flags)
	default:
		panic("Invalid log level")
	}
}
