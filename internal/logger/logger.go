// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package logger provides different log levels by wrapping the default [log.Logger]. There are
// four levels, in decreasing order of priority: Error, Warning, Info, and Debug.
// Error and Warning write to [os.Stderr] and Info and Debug write to [os.Stdout]. If Debug is set
// as the default level, all logs include file and line number data.
//
// The package can be configured with two environment variables:
//
// CARTESI_LOG_LEVEL: defines the main log level. [Info] is the default.
// CARTESI_LOG_ENABLE_TIMESTAMP: a flag that adds date and time information to the log entries.
// It is disabled by default.
package logger

import (
	"io"
	"log"
	"os"
)

var (
	Error   *log.Logger
	Warning *log.Logger
	Info    *log.Logger
	Debug   *log.Logger
)

func Init(logLevel string, enableTimestamp bool) {
	var flags int
	if enableTimestamp {
		flags |= log.Ldate | log.Ltime
	}

	Error = log.New(os.Stderr, "ERROR ", flags)
	Warning = log.New(os.Stderr, "WARN ", flags)
	Info = log.New(os.Stdout, "INFO ", flags)
	Debug = log.New(os.Stdout, "DEBUG ", flags)

	switch logLevel {
	case "error":
		Warning.SetOutput(io.Discard)
		fallthrough
	case "warning":
		Info.SetOutput(io.Discard)
		fallthrough
	case "info":
		Debug.SetOutput(io.Discard)
	case "debug":
		flags |= log.Llongfile
		Error.SetFlags(flags)
		Warning.SetFlags(flags)
		Info.SetFlags(flags)
		Debug.SetFlags(flags)
	default:
		panic("Invalid log level")
	}
}
