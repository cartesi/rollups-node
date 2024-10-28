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
//			panic(err)
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

// CreateInfo stores initialization data for the Create function
type CreateInfo struct {
	Name                 string
	Impl                 ServiceImpl
	LogLevel             string
	ProcOwner            bool
	ServeMux             *http.ServeMux
	Context              context.Context
	PollInterval         time.Duration
	EnableSignalHandling bool
	TelemetryCreate      bool
	TelemetryAddress     string
}

// Service stores runtime information.
type Service struct {
	Running        atomic.Bool
	Name           string
	Impl           ServiceImpl
	Logger         *slog.Logger
	Ticker         *time.Ticker
	PollInterval   time.Duration
	Context        context.Context
	Cancel         context.CancelFunc
	Sighup         chan os.Signal // SIGHUP to reload
	Sigint         chan os.Signal // SIGINT to exit gracefully
	ServeMux       *http.ServeMux
	HTTPServer     *http.Server
	HTTPServerFunc func() error
}

// Create a service by:
//   - using values from s if non zero,
//   - using values from ci,
//   - using default values when applicable
func Create(ci *CreateInfo, s *Service) error {
	if ci == nil {
		return ErrInvalid
	}

	if s == nil {
		return ErrInvalid
	}

	// running
	s.Running.Store(false)

	// name
	if s.Name == "" {
		s.Name = ci.Name
	}

	// impl
	if s.Impl == nil {
		s.Impl = ci.Impl
	}

	// log
	var LogLevel slog.Level
	if s.Logger == nil {
		LogLevel = map[string]slog.Level{
			"debug": slog.LevelDebug,
			"info":  slog.LevelInfo,
			"warn":  slog.LevelWarn,
			"error": slog.LevelError,
		}[ci.LogLevel]

		if ci.LogLevel == "" {
			LogLevel = slog.LevelDebug
		}
		// opts := &tint.Options{
		// 	Level:     LogLevel,
		// 	AddSource: LogLevel == slog.LevelDebug,
		// 	// RFC3339 with milliseconds and without timezone
		// 	TimeFormat: "2006-01-02T15:04:05.000",
		// }
		// handler := tint.NewHandler(os.Stdout, opts)
		// s.Logger = slog.New(handler)
		s.Logger = slog.Default()
	}

	// context and cancelation
	if s.Context == nil {
		if ci.Context == nil {
			ci.Context = context.Background()
		}
		s.Context = ci.Context
	}
	if s.Cancel == nil {
		s.Context, s.Cancel = context.WithCancel(ci.Context)
	}

	if ci.ProcOwner {
		// ticker
		if s.Ticker == nil {
			if ci.PollInterval == 0 {
				ci.PollInterval = 1000 * time.Millisecond
			}
			s.PollInterval = ci.PollInterval
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
	}

	// telemetry
	if ci.TelemetryCreate {
		if s.ServeMux == nil {
			if ci.ServeMux == nil {
				if !ci.ProcOwner {
					s.Logger.Warn("Create:Created a new ServeMux",
						"service", s.Name,
						"ProcOwner", ci.ProcOwner,
						"LogLevel", LogLevel)
				}
				ci.ServeMux = http.NewServeMux()
			}
			s.ServeMux = ci.ServeMux
		}
		if ci.TelemetryAddress == "" {
			ci.TelemetryAddress = ":8080"
		}
		s.HTTPServer, s.HTTPServerFunc = s.CreateDefaultTelemetry(
			ci.TelemetryAddress, 3, 5*time.Second, s.ServeMux)
		go s.HTTPServerFunc()
	}

	// ProcOwner will be ready on the call to Serve
	if ci.ProcOwner {
		s.Logger.Info("Create",
			"service", s.Name,
			"LogLevel", LogLevel,
			"pid", os.Getpid())
	} else {
		s.Running.Store(true)
		s.Logger.Info("Create",
			"service", s.Name,
			"LogLevel", LogLevel)
	}
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

	if errs != nil {
		s.Logger.Error("Reload",
			"service", s.Name,
			"duration", elapsed,
			"error", errs)
	} else {
		s.Logger.Info("Reload",
			"service", s.Name,
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
			"service", s.Name,
			"duration", elapsed,
			"error", errs)
	} else {
		s.Logger.Debug("Tick",
			"service", s.Name,
			"duration", elapsed)
	}
	return errs
}

func (s *Service) Stop(force bool) []error {
	start := time.Now()
	errs := s.Impl.Stop(force)
	if s.HTTPServer != nil {
		s.HTTPServer.Shutdown(s.Context)
	}
	elapsed := time.Since(start)

	s.Running.Store(false)
	if errs != nil {
		s.Logger.Error("Stop",
			"force", force,
			"service", s.Name,
			"duration", elapsed,
			"error", errs)
	} else {
		s.Logger.Info("Stop",
			"force", force,
			"service", s.Name,
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
		Addr:    addr,
		Handler: mux,
	}
	return server, func() error {
		s.Logger.Info("Telemetry", "service", s.Name, "addr", addr)
		var err error = nil
		for retry := 0; retry < maxRetries+1; retry++ {
			switch err = server.ListenAndServe(); err {
			case http.ErrServerClosed:
				return nil
			default:
				s.Logger.Error("http",
					"service", s.Name,
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
