// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// The Service package provides basic functionality for implementing long running programs.
// Services are created with the Create function that receives a CreateInfo for its configuration.
// The runtime information is then stored in the Service.
//
// The recommended way to implement a new service is to:
//   - embed a [CreateInfo] struct into a new Create<type>Info struct.
//   - embed a [Service] struct into a new <type>Service struct.
//   - embed a [Create] call into a new Create<type> function.
//
// Check DummyService, SlowService and ListService source code for examples of how to do it.
//
// To use a service, call its corresponding Create function with a matching CreateInfo and Service,
// then fill in the appropriate CreateInfo fields.
// Here are a few of the available options:
//   - Name: string representing this service, will show up in the logs.
//   - Impl: what to use as the ServiceImpl interface, use itself in this case.
//   - LogLevel: One of 'debug', 'info', 'warn', 'error'.
//   - ProcOwner: Declare this as the process owner and run additional setup.
//   - TelemetryCreate: Setup a http.ServeMux and serve a HTTP endpoint in a go routine.
//   - TelemetryAddress: Address to use when TelemetryCreate is enabled.
//
// Hook up the `livez` and `readyz` handlers into the HTTP mux.
// Then Run the server
//
// Example shows the creation of a [DummyService] by calling [CreateDummy].
//
//	package main
//
//	import (
//		"github.com/cartesi/rollups-node/pkg/service"
//	)
//
//	func main() {
//		s := service.DummyService{}
//		err := service.CreateDummy(service.CreateDummyInfo{
//			CreateInfo: service.CreateInfo{
//				Name:             "nil",
//				Impl:             &s,
//				LogLevel:         "info",
//				ProcOwner:        true,
//				TelemetryCreate:  true,
//				TelemetryAddress: ":8081",
//			},
//		}, &s)
//		if err != nil {
//			s.Logger.Error("Fatal", "error", err)
//			os.Exit(1)
//		}
//		s.CreateDefaultHandlers("/" + s.Name)
//		s.Serve()
//	}
package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lmittmann/tint"
)

var (
	ErrInvalid = fmt.Errorf("Invalid Argument") // invalid argument
)

type ServiceImpl interface {
	Alive() bool
	Ready() bool
	Reload() []error
	Tick() []error
	Stop(bool) []error
}

type IService interface {
	Alive() bool
	Ready() bool
	Reload() []error
	Tick() []error
	Stop(bool) []error
	Serve() error
	String() string
}

// CreateInfo stores initialization data for the Create function
type CreateInfo struct {
	Name                 string
	Impl                 ServiceImpl
	LogLevel             LogLevel
	LogPretty            bool
	ServeMux             *http.ServeMux
	Context              context.Context
	PollInterval         time.Duration
	EnableSignalHandling bool
	TelemetryCreate      bool
	TelemetryAddress     string
}

// Service stores runtime information.
type Service struct {
	Running       atomic.Bool
	Name          string
	Impl          ServiceImpl
	Logger        *slog.Logger
	Ticker        *time.Ticker
	PollInterval  time.Duration
	Context       context.Context
	Cancel        context.CancelFunc
	Sighup        chan os.Signal // SIGHUP to reload
	Sigint        chan os.Signal // SIGINT to exit gracefully
	ServeMux      *http.ServeMux
	Telemetry     *http.Server
	TelemetryFunc func() error
}

// Create a service by:
//   - using values from s if non zero,
//   - using values from c,
//   - using default values when applicable
func Create(c *CreateInfo, s *Service) error {
	if c == nil || c.Impl == nil || c.Impl == s || s == nil {
		return ErrInvalid
	}

	s.Running.Store(false)
	s.Name = c.Name
	s.Impl = c.Impl

	// log
	if s.Logger == nil {
		s.Logger = NewLogger(slog.Level(c.LogLevel), c.LogPretty)
		s.Logger = s.Logger.With("service", s.Name)
	}

	// context and cancelation
	if s.Context == nil {
		if c.Context == nil {
			c.Context = context.Background()
		}
		s.Context = c.Context
	}
	if s.Cancel == nil {
		s.Context, s.Cancel = context.WithCancel(c.Context)
	}

	// ticker
	if s.Ticker == nil {
		if c.PollInterval == 0 {
			c.PollInterval = 60 * time.Second
		}
		s.PollInterval = c.PollInterval
		s.Ticker = time.NewTicker(s.PollInterval)
	}

	// signal handling
	if s.Sighup == nil {
		s.Sighup = make(chan os.Signal, 1)
		signal.Notify(s.Sighup, syscall.SIGHUP)
	}
	if s.Sigint == nil {
		s.Sigint = make(chan os.Signal, 1)
		signal.Notify(s.Sigint, syscall.SIGINT)
	}

	// telemetry
	if c.TelemetryCreate {
		if s.ServeMux == nil {
			if c.ServeMux == nil {
				c.ServeMux = http.NewServeMux()
			}
			s.ServeMux = c.ServeMux
		}
		if c.TelemetryAddress == "" {
			c.TelemetryAddress = ":8080"
		}
		s.Telemetry, s.TelemetryFunc = s.CreateDefaultTelemetry(
			c.TelemetryAddress, 3, 5*time.Second, s.ServeMux)
		go s.TelemetryFunc()
	}

	s.Logger.Info("Create", "LogLevel", c.LogLevel, "pid", os.Getpid())
	return nil
}

func (s *Service) Alive() bool {
	return s.Impl.Alive()
}

func (s *Service) Ready() bool {
	return s.Impl.Ready()
}

func (s *Service) Reload() []error {
	start := time.Now()
	errs := s.Impl.Reload()
	elapsed := time.Since(start)

	if len(errs) > 0 {
		s.Logger.Error("Reload",
			"duration", elapsed,
			"error", errs)
	} else {
		s.Logger.Info("Reload",
			"duration", elapsed)
	}
	return errs
}

func (s *Service) Tick() []error {
	start := time.Now()
	errs := s.Impl.Tick()
	elapsed := time.Since(start)

	if len(errs) > 0 {
		s.Logger.Error("Tick",
			"duration", elapsed,
			"error", errs)
	} else {
		s.Logger.Debug("Tick",
			"duration", elapsed)
	}
	return errs
}

func (s *Service) Stop(force bool) []error {
	start := time.Now()
	errs := s.Impl.Stop(force)
	if s.Telemetry != nil {
		s.Telemetry.Shutdown(s.Context)
	}
	elapsed := time.Since(start)

	s.Running.Store(false)
	if len(errs) > 0 {
		s.Logger.Error("Stop",
			"force", force,
			"duration", elapsed,
			"error", errs)
	} else {
		s.Logger.Info("Stop",
			"force", force,
			"duration", elapsed)
	}
	return nil
}

func (s *Service) Serve() error {
	s.Running.Store(true)
	s.Tick()
	for s.Running.Load() {
		select {
		case <-s.Sighup:
			s.Reload()
		case <-s.Sigint:
			s.Stop(false)
		case <-s.Context.Done():
			s.Stop(true)
		case <-s.Ticker.C:
			s.Tick()
		}
	}
	return nil
}

func (s *Service) String() string {
	return s.Name
}

func NewLogger(level slog.Level, pretty bool) *slog.Logger {
	opts := &tint.Options{
		Level:     level,
		AddSource: level == slog.LevelDebug,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
		NoColor:    !pretty,
	}
	handler := tint.NewHandler(os.Stdout, opts)
	return slog.New(handler)
}

func WithTimeout(limit time.Duration, fn func() error) error {
	ch := make(chan error)
	deadline := time.After(limit)
	go func() {
		ch <- fn()
	}()

	select {
	case err := <-ch:
		return err
	case <-deadline:
		return fmt.Errorf("Time limit exceeded")
	}
}

// Telemetry
func (s *Service) CreateDefaultHandlers(prefix string) {
	s.ServeMux.Handle(prefix+"/readyz", http.HandlerFunc(s.ReadyHandler))
	s.ServeMux.Handle(prefix+"/livez", http.HandlerFunc(s.AliveHandler))
}

func (s *Service) CreateDefaultTelemetry(
	addr string,
	maxRetries int,
	retryInterval time.Duration,
	mux *http.ServeMux,
) (*http.Server, func() error) {
	server := &http.Server{
		Addr:     addr,
		Handler:  mux,
		ErrorLog: slog.NewLogLogger(s.Logger.Handler(), slog.LevelError),
	}
	return server, func() error {
		s.Logger.Info("Telemetry", "addr", addr)
		var err error = nil
		for retry := 0; retry < maxRetries+1; retry++ {
			switch err = server.ListenAndServe(); err {
			case http.ErrServerClosed:
				return nil
			default:
				s.Logger.Error("http",
					"error", err,
					"try", retry+1,
					"maxRetries", maxRetries,
					"error", err)
			}
			time.Sleep(retryInterval)
		}
		return err
	}
}

// HTTP handler for `/s.Name/readyz` that exposes the value of Ready()
func (s *Service) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if !s.Ready() {
		http.Error(w, s.Name+": ready check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, s.Name+": ready\n")
	}
}

// HTTP handler for `/s.Name/livez` that exposes the value of Alive()
func (s *Service) AliveHandler(w http.ResponseWriter, r *http.Request) {
	if !s.Alive() {
		http.Error(w, s.Name+": alive check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, s.Name+": alive\n")
	}
}
