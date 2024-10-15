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
	ENoIService = fmt.Errorf("IServiceHooks parameter is mandatory")
)

/* Service hook points exposed to service implementers */
type IServiceHooks interface {
	Alive()  bool
	Ready()  bool
	Reload() bool
	Tick()   bool
}

/* Configuration values to Create this service. */
type CreateInfo struct {
	Name                     string
	LogLevel                 string
	PollInterval             time.Duration
	Context                  context.Context
	SignalTrapsCreate        bool
	TelemetryCreate          bool
	TelemetryAddress         string
	TelemetryRetryInterval   time.Duration
}

/* Service metrics */
type Metrics struct {
	tickCount atomic.Uint64
}

/* Runtime values to run this service. */
type Service struct {
	Name                     string
	logLevel                 slog.Level
	ticker                  *time.Ticker
	context                  context.Context
	cancel                   context.CancelFunc
	sighup                   chan os.Signal // SIGHUP to reload
	sigint                   chan os.Signal // SIGINT to exit gracefully
	running                  atomic.Bool
	httpMux                  *http.ServeMux
	telemetryServer          *http.Server
	telemetryStart           func()
	hook                     IServiceHooks

	metrics                  Metrics
}

////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////

/* Create the service using CreateInfo as the configuration.
 * An empty config should still create a functioning server if possible. */
func Create(ci CreateInfo, is IServiceHooks, s *Service) error {
	// name
	if ci.Name != "" {
		s.Name = ci.Name
	}
	if is == nil {
		return ENoIService
	}
	s.hook = is

	// log
	s.logLevel = map[string]slog.Level {
		"debug" : slog.LevelDebug,
		"info"  : slog.LevelInfo,
		"warn"  : slog.LevelWarn,
		"error" : slog.LevelError,
	}[ci.LogLevel]

	if ci.LogLevel == "" {
		s.logLevel = slog.LevelDebug
	}
	opts := &tint.Options{
		Level:      s.logLevel,
		AddSource:  s.logLevel == slog.LevelDebug,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
	}
	handler := tint.NewHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// ticker
	if s.ticker == nil {
		if ci.PollInterval == 0 {
			ci.PollInterval = 1000 * time.Millisecond
		}
		s.ticker = time.NewTicker(ci.PollInterval)
	}

	// cancelation
	if s.cancel == nil {
		if ci.Context == nil {
			ci.Context = context.Background()
		}
		s.context, s.cancel = context.WithCancel(ci.Context)
	}

	// signal handlers
	if ci.SignalTrapsCreate {
		s.sighup = make(chan os.Signal, 1)
		signal.Notify(s.sighup, syscall.SIGHUP)

		s.sigint = make(chan os.Signal, 1)
		signal.Notify(s.sigint, syscall.SIGINT)
	}

	// telemetry endpoints
	if ci.TelemetryCreate {
		if ci.TelemetryAddress == "" {
			ci.TelemetryAddress = ":80"
		}
		if ci.TelemetryRetryInterval == 0 {
			ci.TelemetryRetryInterval = 5 * time.Second
		}
		s.httpMux = http.NewServeMux()
		s.telemetryServer, s.telemetryStart =
			s.StartTelemetryServer(ci.TelemetryAddress, 3,
			ci.TelemetryRetryInterval, s.httpMux)
	}

	slog.Info("create",
		"service",   s.Name,
		"pid",       os.Getpid(),
		"log-level", s.logLevel)
	return nil
}

/* Start the service in:
 * - serve=true blocking or,
 * - serve=false non-blocking mode */
func (s *Service) Start(serve bool) {
	slog.Info("start", "service", s.Name)

	if (s.telemetryServer != nil) {
		go s.telemetryStart()
	}
	s.running.Store(true)
	if serve {
		s.loop()
	} else {
		go s.loop()
	}
}

/* Stop the service in:
 * - force=true forcefully or,
 * - force=false gracefully by giving it time to shutdown its components */
func (s *Service) Stop(force bool) {
	slog.Info("stop", "service", s.Name, "force", force)

	s.running.Store(false)
	if (s.telemetryServer != nil) {
		s.telemetryServer.Close()
	}
	if force {
		s.cancel()
	}
}

/* Reload behavior is service specific. A common, expected use is to
 * reconfigure a running service. */
func (s *Service) reload() bool {
	slog.Info("reload", "service", s.Name)
	return s.hook.Reload()
}

func (s *Service) ready() bool {
	ready := s.running.Load() && s.hook.Ready()
	slog.Info("checkready", "service", s.Name, "ready", ready)
	return ready
}

func (s *Service) alive() bool {
	alive := s.hook.Alive()
	slog.Info("checkalive", "service", s.Name, "alive", alive)
	return alive
}

func (s *Service) tick() {
	start := time.Now()

	s.hook.Tick()

	elapsed := time.Since(start)
	slog.Info("tick",
		"service", s.Name,
		"tick", s.metrics.tickCount.Load(),
		"duration", elapsed)
	s.metrics.tickCount.Add(1)

}

func (s *Service) loop() {
	s.tick()
	for s.running.Load() {
		select {
		case <-s.sighup:
			s.reload()
		case <-s.sigint:
			s.Stop(false)
		case <-s.context.Done():
			s.Stop(true)
		case <-s.ticker.C:
			s.tick()
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
// telemetry (health, metrics)
////////////////////////////////////////////////////////////////////////////////

/* Create `/s.Name/readyz`, `/s.Name/livez` and `/s.Name/metrics` HTTP endpoints on
 * addr. Recreate the HTTP server up to maxRetries in case it goes down
 * unexpectedly waiting for retryInterval beteen each attempt. Route endpoints
 * with mux. */
func (s *Service) StartTelemetryServer(
	addr string,
	maxRetries int,
	retryInterval time.Duration,
	mux *http.ServeMux,
) (*http.Server, func()) {
	mux.Handle("/"+s.Name+"/readyz",  http.HandlerFunc(s.ReadyHandler))
	mux.Handle("/"+s.Name+"/livez",   http.HandlerFunc(s.AliveHandler))

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return server, func() {
		for retry := 0; retry < maxRetries+1; retry++ {
			slog.Info("http", "service", s.Name, "addr", addr)
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				slog.Error("http",
					"service", s.Name,
					"error", err,
					"try", retry+1,
					"maxRetries", maxRetries)
			}
			time.Sleep(retryInterval)
		}
	}
}

/* HTTP handler for `/s.Name/readyz` that exposes the value of Ready() */
func (s *Service) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if !s.ready() {
		http.Error(w, s.Name+": ready check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, s.Name+": ready\n")
	}
}

/* HTTP handler for `/s.Name/livez` that exposes the value of Alive() */
func (s *Service) AliveHandler(w http.ResponseWriter, r *http.Request) {
	if !s.alive() {
		http.Error(w, s.Name+": alive check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, s.Name+": alive\n")
	}
}
