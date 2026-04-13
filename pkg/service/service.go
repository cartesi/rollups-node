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

	"github.com/cartesi/rollups-node/internal/version"
	"github.com/lmittmann/tint"
)

const telemetryShutdownTimeout = 5 * time.Second

var (
	ErrInvalid = fmt.Errorf("Invalid Argument") // invalid argument
)

// ServiceImpl is the interface that concrete services must implement.
//
// IMPORTANT: Stop() implementations that shadow Service.Stop() MUST call
// s.SetStopping() as their first action. This sets the stopping flag so that
// a concurrent Tick() can detect shutdown-in-progress via IsStopping() and
// suppress expected teardown errors (e.g., context.Canceled from in-flight
// RPCs). Without this call, the race window between Stop() tearing down
// resources and Tick() observing the cancellation produces spurious errors.
//
// When Stop() is called through the framework's Service.Stop() dispatch,
// the flag is set automatically before Impl.Stop() runs. But the node
// orchestrator calls child.Stop() directly (Go method resolution picks the
// concrete type's Stop, bypassing Service.Stop), so the impl's SetStopping()
// is the only thing that sets the flag on that path.
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
	LogLevel             slog.Level
	LogColor             bool
	EnableSignalHandling bool
	TelemetryCreate      bool
	TelemetryAddress     string
	PollInterval         time.Duration
	Impl                 ServiceImpl
	Logger               *slog.Logger
	ServeMux             *http.ServeMux
	Context              context.Context
	Cancel               context.CancelFunc

	// EnableReschedule, when true, creates a self-continuation channel.
	// Services that discover remaining work after a Tick() call
	// SignalReschedule() to re-tick immediately without waiting for the
	// timer interval.
	//
	// Migration: When the events library (feature/events-library-research)
	// ships, Serve() will gain an additional EventChannel case for external
	// cross-service notifications. Reschedule remains complementary:
	// Reschedule = internal self-continuation ("I have more work"),
	// EventChannel = external stimulus ("another service produced work").
	// Both coexist in the select loop alongside the Ticker safety-net.
	EnableReschedule bool
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
	SigShutdown   chan os.Signal // SIGINT/SIGTERM to exit gracefully
	ServeMux      *http.ServeMux
	Telemetry     *http.Server
	TelemetryFunc func() error
	reschedule    chan struct{} // self-continuation signal; see CreateInfo.EnableReschedule

	// stopping is set to true at the beginning of Stop(), before Impl.Stop()
	// is called. Services can check this via IsStopping() from Tick() to
	// detect that shutdown is in progress and suppress errors that are
	// expected during teardown (e.g., context.Canceled from in-flight RPC
	// calls). This covers the race window between Stop() being called and
	// ctx.Cancel() propagating.
	stopping atomic.Bool
}

// Create a service by:
//   - using values from s if non zero,
//   - using values from c,
//   - using default values when applicable
func Create(ctx context.Context, c *CreateInfo, s *Service) error {
	if c == nil || c.Impl == nil || c.Impl == s || s == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s.Running.Store(false)
	s.Name = c.Name
	s.Impl = c.Impl
	s.Logger = c.Logger

	// log
	if s.Logger == nil {
		s.Logger = NewServiceLogger(c)
	}

	// context and cancelation
	if s.Context == nil {
		if c.Context == nil {
			c.Context = context.Background()
		}
		s.Context = c.Context
	}
	if s.Cancel == nil {
		if c.Cancel == nil {
			s.Context, c.Cancel = context.WithCancel(c.Context)
		}
		s.Cancel = c.Cancel
	}

	// ticker
	if s.Ticker == nil {
		if c.PollInterval == 0 {
			c.PollInterval = time.Minute
		}
		s.PollInterval = c.PollInterval
		s.Ticker = time.NewTicker(s.PollInterval)
	}

	// self-rescheduling
	if c.EnableReschedule {
		s.reschedule = make(chan struct{}, 1)
	}

	// signal handling
	if c.EnableSignalHandling {
		if s.Sighup == nil {
			s.Sighup = make(chan os.Signal, 1)
			signal.Notify(s.Sighup, syscall.SIGHUP)
		}
		if s.SigShutdown == nil {
			s.SigShutdown = make(chan os.Signal, 1)
			signal.Notify(s.SigShutdown, syscall.SIGINT, syscall.SIGTERM)
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
		s.Telemetry, s.TelemetryFunc = s.CreateDefaultTelemetry(c.TelemetryAddress, 3, 5*time.Second)
		go s.TelemetryFunc()
	}

	s.Logger.Info("Create", "version", version.BuildVersion, "log_level", c.LogLevel, "pid", os.Getpid())
	if s.Telemetry != nil {
		s.Logger.Info("Telemetry", "address", s.Telemetry.Addr)
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

// IsStopping reports whether Stop() has been called. Services use this in
// Tick() to detect shutdown-in-progress and suppress expected teardown errors.
func (s *Service) IsStopping() bool {
	return s.stopping.Load()
}

// SetStopping sets the stopping flag. Services whose Stop() method shadows
// Service.Stop() (i.e., every ServiceImpl) must call this at the top of their
// Stop so that concurrent Tick goroutines can observe IsStopping() == true
// before resources are torn down.
func (s *Service) SetStopping() {
	s.stopping.Store(true)
}

func (s *Service) Stop(force bool) []error {
	s.stopping.Store(true)
	start := time.Now()
	errs := s.Impl.Stop(force)
	if s.Telemetry != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), telemetryShutdownTimeout)
		defer cancel()
		if err := s.Telemetry.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, err)
		}
	}
	if s.SigShutdown != nil {
		signal.Stop(s.SigShutdown)
	}
	if s.Sighup != nil {
		signal.Stop(s.Sighup)
	}
	elapsed := time.Since(start)

	s.Running.Store(false)
	s.Cancel()
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

// rescheduleChan returns the reschedule channel, or nil if rescheduling is disabled.
// A nil channel in a select case blocks forever, preserving timer-only behavior.
func (s *Service) rescheduleChan() <-chan struct{} {
	return s.reschedule
}

// SignalReschedule performs a non-blocking send on the reschedule channel.
// If a signal is already pending, this is a no-op (one wake is sufficient).
// Does nothing if rescheduling is not enabled.
// INVARIANT: This method must never block.
func (s *Service) SignalReschedule() {
	select {
	case s.reschedule <- struct{}{}:
	default:
	}
}

// DrainReschedule consumes and discards a pending reschedule signal, if any.
// Returns true if a signal was pending. Intended for testing.
func (s *Service) DrainReschedule() bool {
	select {
	case <-s.reschedule:
		return true
	default:
		return false
	}
}

func (s *Service) Serve() error {
	s.Running.Store(true)

	// Check for context cancellation before the first tick.
	select {
	case <-s.Context.Done():
		s.Stop(true) // Stop logs errors internally.
		return nil
	default:
	}

	s.Tick()
	for s.Running.Load() {
		select {
		case <-s.Sighup:
			s.Reload()
		case <-s.SigShutdown:
			s.Stop(false) // Graceful shutdown; errors are logged by Stop.
			return nil
		case <-s.Context.Done():
			s.Stop(true) // Stop logs errors internally.
			return nil
		case <-s.Ticker.C:
			s.Tick()
		case <-s.rescheduleChan():
			s.Tick()
		}
	}
	return nil
}

func (s *Service) String() string {
	return s.Name
}

// LogConfig logs the service configuration at debug level.
// Intended for use by standalone service binaries after Create.
func (s *Service) LogConfig(config any) {
	s.Logger.Info("Starting service", "config", config)
}

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

func NewServiceLogger(c *CreateInfo) *slog.Logger {
	return NewLogger(c.LogLevel, c.LogColor).With("service", c.Name)
}

// Telemetry
func (s *Service) CreateDefaultTelemetry(
	addr string,
	maxRetries int,
	retryInterval time.Duration,
) (*http.Server, func() error) {
	s.ServeMux.Handle("/readyz", http.HandlerFunc(s.ReadyHandler))
	s.ServeMux.Handle("/livez", http.HandlerFunc(s.AliveHandler))

	server := &http.Server{
		Addr:     addr,
		Handler:  s.ServeMux,
		ErrorLog: slog.NewLogLogger(s.Logger.Handler(), slog.LevelError),
	}
	return server, func() error {
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
		fmt.Fprintf(w, "%s: ready\n", s.Name)
	}
}

// HTTP handler for `/s.Name/livez` that exposes the value of Alive()
func (s *Service) AliveHandler(w http.ResponseWriter, r *http.Request) {
	if !s.Alive() {
		http.Error(w, s.Name+": alive check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, "%s: alive\n", s.Name)
	}
}
