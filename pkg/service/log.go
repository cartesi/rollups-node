// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Implementation of the pflags Value interface.
package service

import (
	"log/slog"
	"os"
)

type LogLevel slog.Level

func (me LogLevel) String() string {
	return slog.Level(me).String()
}
func (me *LogLevel) Set(s string) error {
	m := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	if v, ok := m[s]; ok {
		*me = LogLevel(v)
		return nil
	} else {
		return os.ErrNotExist
	}
}
func (me *LogLevel) Type() string {
	return "LogLevel"
}
