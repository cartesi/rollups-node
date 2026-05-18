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
	OnReload() []error
	OnStop(bool) []error
	OnServe(ctx context.Context) error
}

type IService interface {
	Alive() bool
	Ready() bool
	Reload() []error
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
	context       context.Context
	cancelContext context.CancelFunc
	Sighup        chan os.Signal // SIGHUP to reload
	SigShutdown   chan os.Signal // SIGINT/SIGTERM to exit gracefully
	ServeMux      *http.ServeMux
	Telemetry     *http.Server
	TelemetryFunc func() error

	// stopping is set to true at the beginning of Stop(), before Impl.Stop()
	// is called. Services can check this via IsStopping() from Tick() to
	// detect that shutdown is in progress and suppress errors that are
	// expected during teardown (e.g., context.Canceled from in-flight RPC
	// calls). This covers the race window between Stop() being called and
	// cancelContext() propagating.
	stopping atomic.Bool

	// cleanedUp server Stop() run exactly once, even when Stop() is called
	// multiple times (by the child's Serve() loop and by the parent orchestrator).
	cleanedUp atomic.Bool
}

// Create a service by:
//   - using values from s if non zero,
//   - using values from c,
//   - using default values when applicable
func Create(ctx context.Context, c *CreateInfo, s *Service) error {
	if c == nil || c.Impl == nil || s == nil {
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
		s.Telemetry, s.TelemetryFunc = s.CreateDefaultTelemetry(c.TelemetryAddress)
		go func() {
			if err := s.TelemetryFunc(); err != nil {
				s.Logger.Error("Telemetry HTTP server failed", "error", err)
			}
		}()
	}

	s.Logger.Info("Create", "version", version.BuildVersion, "log_level", c.LogLevel, "pid", os.Getpid())
	if s.Telemetry != nil {
		s.Logger.Info("Telemetry", "address", s.Telemetry.Addr)
	}
	return nil
}

func (s *Service) OnReload() []error { return nil }
func (s *Service) OnStop(bool) []error { return nil }
func (s *Service) Alive() bool { return true }
func (s *Service) Ready() bool { return true }

func (s *Service) Reload() []error {
	start := time.Now()
	errs := s.Impl.OnReload()
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
	// CAS achieves once-semantics: the second caller returns immediately
	// (fire-and-forget) rather than blocking like sync.Once. This is safe
	// because the orchestrator calls Cancel() after Stop() and waits for
	// the Serve goroutine to exit.
	if !s.cleanedUp.CompareAndSwap(false, true) {
		return nil // already stopped
	}

	s.stopping.Store(true)
	start := time.Now()
	errs := s.Impl.OnStop(force)
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

func (s *Service) Serve() error {
	s.Running.Store(true)

	// Check for context cancellation before the first tick.
	select {
	case <-s.context.Done():
		s.Stop(true) // Stop logs errors internally.
		return nil
	default:
	}

	go func() {
		for s.Running.Load() {
			select {
			case <-s.Sighup:
				s.Reload()
			case <-s.SigShutdown:
				s.Stop(false) // Graceful shutdown; errors are logged by Stop.
				return
			case <-s.context.Done():
				s.Stop(true) // Stop logs errors internally.
				return
			}
		}
	}()

	defer s.Stop(false)

	return s.Impl.OnServe(s.context)
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
func (s *Service) CreateDefaultTelemetry(addr string) (*http.Server, func() error) {
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
func (s *Service) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if !s.Impl.Ready() {
		http.Error(w, s.Name+": ready check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, "%s: ready\n", s.Name)
	}
}

// HTTP handler for `/s.Name/livez` that exposes the value of Alive()
func (s *Service) AliveHandler(w http.ResponseWriter, r *http.Request) {
	if !s.Impl.Alive() {
		http.Error(w, s.Name+": alive check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, "%s: alive\n", s.Name)
	}
}

////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////

type TickImpl interface {
	Tick(ctx context.Context) []error
}

type TickService struct {
	Service
	TickImpl
	ticker     *time.Ticker
	reschedule chan struct{} // self-continuation signal; see CreateInfo.EnableReschedule
}

func NewTickService(c *CreateInfo, s *TickService) error {
	err := Create(context.Background(), c, &s.Service)
	if err != nil {
		return err
	}

	s.Service.Impl = s

	// ticker
	if c.PollInterval == 0 {
		c.PollInterval = time.Minute
	}
	s.ticker = time.NewTicker(c.PollInterval)

	// self-rescheduling
	if c.EnableReschedule {
		s.reschedule = make(chan struct{}, 1)
	}

	return nil
}

func (s *TickService) tick(ctx context.Context) []error {
	start := time.Now()
	errs := s.Tick(ctx)
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

func (s *TickService) OnStop(bool) []error {
	s.ticker.Stop()
	return nil
}

func (s *TickService) OnServe(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.ticker.C:
			s.tick(ctx)
		// 'reschedule' is nil when rescheduling is disabled thus blocking forever,
		// preserving timer-only behavior.
		case <-s.reschedule:
			s.tick(ctx)
		}
	}
}

// SignalReschedule performs a non-blocking send on the reschedule channel.
// If a signal is already pending, this is a no-op (one wake is sufficient).
// Does nothing if rescheduling is not enabled.
// INVARIANT: This method must never block.
func (s *TickService) SignalReschedule() {
	select {
	case s.reschedule <- struct{}{}:
	default:
	}
}

// DrainReschedule consumes and discards a pending reschedule signal, if any.
// Returns true if a signal was pending. Intended for testing.
func (s *TickService) DrainReschedule() bool {
	select {
	case <-s.reschedule:
		return true
	default:
		return false
	}
}
