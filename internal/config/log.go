// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarning
	LogLevelError
)

type LogLevel uint8

func (l LogLevel) Level() slog.Level {
	switch l {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelInfo:
		return slog.LevelInfo
	case LogLevelWarning:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	default:
		panic("invalid log level")
	}
}

func (l LogLevel) LogValue() slog.Value {
	switch l {
	case LogLevelDebug:
		return slog.StringValue("debug")
	case LogLevelInfo:
		return slog.StringValue("info")
	case LogLevelWarning:
		return slog.StringValue("warning")
	case LogLevelError:
		return slog.StringValue("error")
	default:
		panic("invalid log level")
	}
}

// Initializes the default logger with the options from nodeConfig.
// Should be called before attempting to log the first message.
func InitLog(nodeConfig NodeConfig) {
	opts := new(tint.Options)
	opts.Level = nodeConfig.CartesiLogLevel()
	if opts.Level == LogLevelDebug {
		opts.AddSource = true
		// Remove the directory from the source's filename
		opts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				source := a.Value.Any().(*slog.Source)
				source.File = filepath.Base(source.File)
			}
			return a
		}
	}
	// Disables color if CartesiLogPretty==false or if stdout is not a terminal
	if !(nodeConfig.CartesiLogPretty() && isatty.IsTerminal(os.Stdout.Fd())) {
		opts.NoColor = true
	}
	handler := tint.NewHandler(os.Stdout, opts)
	slog.SetDefault(slog.New(handler))
}
