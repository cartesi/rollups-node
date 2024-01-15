// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"fmt"
	"io"
	"log"
	"os"
)

var (
	ErrorLogger   *log.Logger
	WarningLogger *log.Logger
	InfoLogger    *log.Logger
	DebugLogger   *log.Logger
)

func init() {
	logInit()
}

func logInit() {
	var flags int
	if GetCartesiLogTimestamp() {
		flags = log.LstdFlags
	}

	ErrorLogger = log.New(os.Stderr, "ERROR ", flags)
	WarningLogger = log.New(os.Stderr, "WARN ", flags)
	InfoLogger = log.New(os.Stdout, "INFO ", flags)
	DebugLogger = log.New(os.Stdout, "DEBUG ", flags)

	switch GetCartesiLogLevel() {
	case LogLevelError:
		WarningLogger.SetOutput(io.Discard)
		fallthrough
	case LogLevelWarning:
		InfoLogger.SetOutput(io.Discard)
		fallthrough
	case LogLevelInfo:
		DebugLogger.SetOutput(io.Discard)
	case LogLevelDebug:
		flags |= log.Llongfile
		ErrorLogger.SetFlags(flags)
		WarningLogger.SetFlags(flags)
		InfoLogger.SetFlags(flags)
		DebugLogger.SetFlags(flags)
	default:
		panic("Invalid log level")
	}
}

type LogLevel uint8

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarning
	LogLevelError
)

func toLogLevelFromString(s string) (LogLevel, error) {
	var m = map[string]LogLevel{
		"debug":   LogLevelDebug,
		"info":    LogLevelInfo,
		"warning": LogLevelWarning,
		"error":   LogLevelError,
	}
	if v, ok := m[s]; ok {
		return v, nil
	} else {
		var zeroValue LogLevel
		return zeroValue, fmt.Errorf(`invalid log level "%s"`, s)
	}
}
