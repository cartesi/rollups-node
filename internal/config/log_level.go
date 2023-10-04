// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import "fmt"

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
