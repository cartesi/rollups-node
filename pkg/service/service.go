// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/lmittmann/tint"
)

var (
	ErrInvalid        = errors.New("invalid argument") // invalid argument
	ErrAlreadyStarted = errors.New("is already started")
	ErrServiceStopped = errors.New("service stopped unexpectedly")
	ErrServiceBadInit = errors.New("service initialization failure")
)

// Service interface with a service supervisor.
type SupervisedService interface {
	String() string
	Serve(context.Context) error
	Ready() bool
	Teardown()
}

// Basic template for single services under a single management.
type BaseTemplate struct {
	Name   string
	Logger *slog.Logger
}

// BaseConfigs stores configuration for the InitServiceTemplate function
type BaseConfigs struct {
	Name     string
	Logger   *slog.Logger
	LogLevel slog.Level
	LogColor bool
}

// Initialize the 'ServiceTemplate' structure using values from 'ServiceConfigs'.
// 'impl' must be a reference to the concrete service implementation that
// embeds 'ServiceTemplate'
func InitServiceTemplate(s *BaseTemplate, c *BaseConfigs) {
	s.Name = c.Name

	// log
	s.Logger = c.Logger
	if s.Logger == nil {
		s.Logger = NewLogger(s.Name, c.LogLevel, c.LogColor)
	}

	s.Logger.Info("Create", "log_level", c.LogLevel)
}

// Default implementation of some abstract methods (except `Serve`).
// Remove them to force concrete services to provide implementation for them.
func (s *BaseTemplate) String() string { return s.Name }
func (s *BaseTemplate) Ready() bool    { return true }
func (s *BaseTemplate) Teardown()      {}

/*
 * Service Logger
 */

func NewLogger(name string, level slog.Level, color bool) *slog.Logger {
	level = config.ResolveServiceLogLevel(name, level)
	opts := &tint.Options{
		Level:     level,
		AddSource: level == slog.LevelDebug,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
		NoColor:    !color,
	}
	handler := tint.NewHandler(os.Stdout, opts)
	return slog.New(handler).With("service", name)
}
