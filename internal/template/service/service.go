/* Service template with:
 * - cancellation
 * - readiness check
 * - liveliness check
 * - logging
 * - polling
 * - reloading
 * - signal handling
 */
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

const (
	/* Name is used to identify this service on log messages and on
	 * the health checks endpoints */
	Name = "template"
)

/* Configuration values to Create this service. */
type CreateInfo struct {
	LogLevel                 string
	PollInterval             time.Duration
	Context                  context.Context
	SignalTrapsEnabled       bool
	TelemetrysEnabled        bool
	TelemetryAddress         string
	TelemetryRetryInterval   time.Duration

	// TODO: your fields go here
}

/* Runtime values to run this service. */
type Service struct {
	logLevel                 slog.Level
	ticker                  *time.Ticker
	context                  context.Context
	cancel                   context.CancelFunc
	sighup                   chan os.Signal // SIGHUP to reload
	sigint                   chan os.Signal // SIGINT to exit gracefully
	running                  atomic.Bool
	httpMux                  *http.ServeMux
	telemetryServer          *http.Server
	telemetryServerFunc       func()

	//ci      CreateInfo // maybe needed for reload

	// TODO: your fields go here
}

////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////

/* Create the service using CreateInfo as the configuration.
 * An empty config should still create a functioning server if possible. */
func Create(ci CreateInfo) (s *Service, e error) {
	s = &Service{}

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

	// poller
	if ci.PollInterval == 0 {
		ci.PollInterval = 1000 * time.Millisecond
	}
	s.ticker = time.NewTicker(ci.PollInterval)

	// cancelation
	if ci.Context == nil {
		ci.Context = context.Background()
	}
	s.context, s.cancel = context.WithCancel(ci.Context)

	// signal handlers
	if ci.SignalTrapsEnabled {
		s.sighup = make(chan os.Signal, 1)
		signal.Notify(s.sighup, syscall.SIGHUP)

		s.sigint = make(chan os.Signal, 1)
		signal.Notify(s.sigint, syscall.SIGINT)
	}

	// telemetry endpoints
	if ci.TelemetrysEnabled {
		if ci.TelemetryAddress == "" {
			ci.TelemetryAddress = ":80"
		}
		if ci.TelemetryRetryInterval == 0 {
			ci.TelemetryRetryInterval = 5 * time.Second
		}
		s.httpMux = http.NewServeMux()
		s.telemetryServer, s.telemetryServerFunc =
			s.StartTelemetryServer(ci.TelemetryAddress, 3,
			ci.TelemetryRetryInterval, s.httpMux)
	}

	// TODO: your initialization goes here

	slog.Info("create",
		"service",   Name,
		"pid",       os.Getpid(),
		"log-level", s.logLevel)
	return s, nil
}

/* Start the service in:
 * - serve=true blocking or,
 * - serve=false non-blocking mode */
func (s *Service) Start(serve bool) {
	slog.Info("start", "service", Name)

	go s.telemetryServerFunc()
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
	slog.Info("stop", "service", Name, "force", force)

	s.running.Store(false)
	s.telemetryServer.Close()
	if force {
		s.cancel()
	}
}

/* Reload behavior is service specific. A common, expected use is to
 * reconfigure a running service. */
func (s *Service) Reload() {
	slog.Info("reload", "service", Name)
}

/* Service is Ready */
func (s *Service) Ready() bool {
	running := s.running.Load()

	slog.Info("checkready", "service", Name, "ready", running)
	return running
}

/* Service is Alive */
func (s *Service) Alive() bool {
	alive := true

	slog.Info("checkalive", "service", Name, "alive", alive)
	return alive
}

func (s *Service) loop() {
	for s.running.Load() {
		select {
		case <-s.sighup:
			s.Reload()
		case <-s.sigint:
			s.Stop(false)
		case <-s.context.Done():
			s.Stop(true)
		case <-s.ticker.C:
			slog.Info("tick", "service", Name)

			// TODO: you service goes here
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
// telemetry checks
////////////////////////////////////////////////////////////////////////////////

/* Create `/Name/readyz` and `/Name/livez` HTTP endpoints on
 * addr. Recreate the HTTP server up to maxRetries in case it goes down
 * unexpectedly waiting for retryInterval beteen each attempt. Rounte the
 * endpoints with mux. */
func (s *Service) StartTelemetryServer(
	addr string,
	maxRetries int,
	retryInterval time.Duration,
	mux *http.ServeMux,
) (*http.Server, func()) {
	mux.Handle("/"+Name+"/readyz",  http.HandlerFunc(s.ReadyHandler))
	mux.Handle("/"+Name+"/livez",   http.HandlerFunc(s.AliveHandler))
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return server, func() {
		for retry := 0; retry < maxRetries+1; retry++ {
			slog.Info("http", "service", Name, "addr", addr)
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				slog.Error("http",
					"service", Name,
					"error", err,
					"try", retry+1,
					"maxRetries", maxRetries)
			}
			time.Sleep(retryInterval)
		}
	}
}

/* HTTP handler for `/Name/readyz` that exposes the value of Ready() */
func (s *Service) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if !s.Ready() {
		http.Error(w, Name+": ready check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, Name+": ready\n")
	}
}

/* HTTP handler for `/Name/livez` that exposes the value of Alive() */
func (s *Service) AliveHandler(w http.ResponseWriter, r *http.Request) {
	if !s.Alive() {
		http.Error(w, Name+": alive check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, Name+": alive\n")
	}
}
