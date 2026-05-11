// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// The Service package provides basic functionality for implementing long running programs.
// Services are created with the Create function that receives a CreateInfo for its configuration.
// The runtime information is then stored in the Service.
//
// The recommended way to implement a new service is to:
//   - embed a [ServiceConfigs] struct into a new Create<type>Info struct.
//   - embed a [ServiceTemplate] struct into a new <type>Service struct.
//   - embed a [Create] call into a new Create<type> function.
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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/version"
	"github.com/lmittmann/tint"
)

const telemetryShutdownTimeout = 5 * time.Second

var (
	ErrInvalid = fmt.Errorf("Invalid Argument") // invalid argument
	ErrServiceStopped = fmt.Errorf("Service was stopped")
)

// Public interface with methods to manipulate the service.
type IService interface {
	Alive() bool
	Ready() bool
	Reload() []error
	Stop(bool) []error
	Serve() error
	String() string
}

/*
 * Service template for services that do continuous processing.
 */

// Internal interface with abstract methods called by ServiceTemplate.
// These methods are not part of the public service interface.
type LifecycleImpl interface {
	Alive() bool
	Ready() bool
	OnReload() []error
	OnStop(bool) []error
	OnServe(ctx context.Context) error
}

// ServiceTemplate stores runtime information.
type ServiceTemplate struct {
	Name          string
	Logger        *slog.Logger
	lifecycleImpl LifecycleImpl
	context       context.Context
	cancelContext context.CancelFunc
	sigHangUp     chan os.Signal // SIGHUP to reload
	sigShutdown   chan os.Signal // SIGINT/SIGTERM to exit gracefully
	ServeMux      *http.ServeMux
	telemetry     *http.Server
	telemetryFunc func() error

	// stopped server Stop() run exactly once, even when Stop() is called
	// multiple times (by the child's Serve() loop and by the parent orchestrator).
	stopped atomic.Bool
}

// ServiceConfigs stores configuration for the InitServiceTemplate function
type ServiceConfigs struct {
	Name                 string
	Logger               *slog.Logger
	LogLevel             slog.Level
	LogColor             bool
	Context              context.Context
	Cancel               context.CancelFunc
	EnableSignalHandling bool
	TelemetryCreate      bool
	TelemetryAddress     string
	ServeMux             *http.ServeMux  // used only for unit testing
}

// Initialize the 'ServiceTemplate' structure using values from 'CreateInfo'.
// 'impl' must be a reference to the concrete service implementation that
// embeds 'ServiceTemplate'
func InitServiceTemplate(c *ServiceConfigs, s *ServiceTemplate, impl LifecycleImpl) error {
	if c == nil || s == nil || impl == nil {
		return ErrInvalid
	}

	s.lifecycleImpl = impl

	s.Name = c.Name
	s.Logger = c.Logger

	// log
	if s.Logger == nil {
		s.Logger = NewServiceLogger(c)
	}

	// context and cancelation
	if c.Context == nil {
		c.Context = context.Background()
	}
	s.context = c.Context
	if c.Cancel == nil {
		s.context, c.Cancel = context.WithCancel(c.Context)
	}
	s.cancelContext = c.Cancel

	// signal handling
	if c.EnableSignalHandling {
		if s.sigHangUp == nil {
			s.sigHangUp = make(chan os.Signal, 1)
			signal.Notify(s.sigHangUp, syscall.SIGHUP)
		}
		if s.sigShutdown == nil {
			s.sigShutdown = make(chan os.Signal, 1)
			signal.Notify(s.sigShutdown, syscall.SIGINT, syscall.SIGTERM)
		}
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
		s.telemetry, s.telemetryFunc = s.CreateDefaultTelemetry(c.TelemetryAddress)
		go func() {
			if err := s.telemetryFunc(); err != nil {
				s.Logger.Error("Telemetry HTTP server failed", "error", err)
			}
		}()
	}

	s.Logger.Info("Create", "version", version.BuildVersion, "log_level", c.LogLevel, "pid", os.Getpid())
	if s.telemetry != nil {
		s.Logger.Info("Telemetry", "address", s.telemetry.Addr)
	}
	return nil
}

// Default implementation of some abstract methods (except `OnServe`).
// Remove them to force concrete services to provide implementation for them.
func (s *ServiceTemplate) OnReload() []error { return nil }
func (s *ServiceTemplate) OnStop(bool) []error { return nil }
func (s *ServiceTemplate) Alive() bool { return true }
func (s *ServiceTemplate) Ready() bool { return true }
func (s *ServiceTemplate) String() string { return s.Name }

func (s *ServiceTemplate) Reload() []error {
	if s.stopped.Load() {
		return []error{ErrServiceStopped}
	}

	start := time.Now()
	errs := s.lifecycleImpl.OnReload()
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

func (s *ServiceTemplate) Stop(force bool) []error {
	// CAS achieves once-semantics: the second caller returns immediately
	// (fire-and-forget) rather than blocking like sync.Once. This is safe
	// because the orchestrator calls Cancel() after Stop() and waits for
	// the Serve goroutine to exit.
	if !s.stopped.CompareAndSwap(false, true) {
		return nil // already stopped
	}

	start := time.Now()
	errs := s.lifecycleImpl.OnStop(force)
	if s.telemetry != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), telemetryShutdownTimeout)
		defer cancel()
		if err := s.telemetry.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, err)
		}
	}
	if s.sigShutdown != nil {
		signal.Stop(s.sigShutdown)
	}
	if s.sigHangUp != nil {
		signal.Stop(s.sigHangUp)
	}
	elapsed := time.Since(start)

	s.cancelContext()

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
	return errs
}

func (s *ServiceTemplate) Serve() error {
	if s.stopped.Load() {
		return ErrServiceStopped
	}

	go func() {
		for {
			select {
			case <-s.sigHangUp:
				s.Reload()
			case <-s.sigShutdown:
				s.Stop(false) // Graceful shutdown; errors are logged by Stop.
				return
			case <-s.context.Done():
				s.Stop(true) // Stop logs errors internally.
				return
			}
		}
	}()

	defer s.Stop(true)

	return s.lifecycleImpl.OnServe(s.context)
}

// LogConfig logs the service configuration at debug level.
// Intended for use by standalone service binaries after Create.
func (s *ServiceTemplate) LogConfig(config any) {
	s.Logger.Info("Starting service", "config", config)
}

/*
 * Alternative service template that implements the tick-based processing.
 */

type TickImpl interface {
	Tick(ctx context.Context) (bool, []error)
}

type TickServiceTemplate struct {
	ServiceTemplate
	tickImpl   TickImpl
	ticker     *time.Ticker
}

type TickServiceConfigs struct {
	ServiceConfigs
	PollInterval time.Duration
}

func InitTickServiceTemplate(
	cfg *TickServiceConfigs,
	tmpl *TickServiceTemplate,
	lifecycleImpl LifecycleImpl,
	tickImpl TickImpl,
) error {
	if cfg == nil || tmpl == nil || tickImpl == nil {
		return ErrInvalid
	}

	err := InitServiceTemplate(&cfg.ServiceConfigs, &tmpl.ServiceTemplate, lifecycleImpl)
	if err != nil {
		return err
	}

	tmpl.tickImpl = tickImpl

	// ticker
	if cfg.PollInterval == 0 {
		cfg.PollInterval = time.Minute
	}
	tmpl.ticker = time.NewTicker(cfg.PollInterval)

	return nil
}

func (s *TickServiceTemplate) tick(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	start := time.Now()
	reschedule, errs := s.tickImpl.Tick(ctx)
	elapsed := time.Since(start)

	if len(errs) > 0 {
		s.Logger.Error("Tick",
			"duration", elapsed,
			"reschedule", reschedule,
			"error", errs,
		)
	} else {
		s.Logger.Debug("Tick",
			"duration", elapsed,
			"reschedule", reschedule,
		)
	}
	return reschedule
}

func (s *TickServiceTemplate) OnStop(bool) []error {
	s.ticker.Stop()
	return nil
}

func (s *TickServiceTemplate) OnServe(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	for s.tick(ctx) {}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.ticker.C:
			for s.tick(ctx) {}
		}
	}
}

/*
 * Service Logger
 */

func NewLogger(level slog.Level, color bool) *slog.Logger {
	opts := &tint.Options{
		Level:     level,
		AddSource: level == slog.LevelDebug,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
		NoColor:    !color,
	}
	handler := tint.NewHandler(os.Stdout, opts)
	return slog.New(handler)
}

func NewServiceLogger(c *ServiceConfigs) *slog.Logger {
	return NewLogger(c.LogLevel, c.LogColor).With("service", c.Name)
}

/*
 * Service Telemetry
 */

func (s *ServiceTemplate) CreateDefaultTelemetry(addr string) (*http.Server, func() error) {
	s.ServeMux.Handle("/readyz", http.HandlerFunc(s.ReadyHandler))
	s.ServeMux.Handle("/livez", http.HandlerFunc(s.AliveHandler))

	// Telemetry deliberately omits RequestIDMiddleware. /livez and /readyz are
	// hit every few seconds per pod per service by orchestrators like
	// Kubernetes; burning a crypto/rand UUID per probe is measurable overhead
	// for 1-byte idempotent responses that have nothing to correlate against.
	// RecoverMiddleware is kept so panics still become clean 500s.
	// A static request ID is set so panic logs show "telemetry" instead of "".
	handler := RecoverMiddleware(s.Logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(requestIDHeader, "telemetry")
		s.ServeMux.ServeHTTP(w, r)
	}))
	server := NewHTTPServer(addr, handler, DefaultTelemetryOptions(), s.Logger)
	StartupBindWarning(s.Logger, s.Name+"/telemetry", addr)

	return server, func() error {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// HTTP handler for `/s.Name/readyz` that exposes the value of Ready()
func (s *ServiceTemplate) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if !s.lifecycleImpl.Ready() {
		http.Error(w, s.Name+": ready check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, "%s: ready\n", s.Name)
	}
}

// HTTP handler for `/s.Name/livez` that exposes the value of Alive()
func (s *ServiceTemplate) AliveHandler(w http.ResponseWriter, r *http.Request) {
	if !s.lifecycleImpl.Alive() {
		http.Error(w, s.Name+": alive check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, "%s: alive\n", s.Name)
	}
}
