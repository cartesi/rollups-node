// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"fmt"
	"net/http"
	"time"
)

const shutdownTimeout = 5 * time.Second

type telemetryService struct {
	HTTPServiceTemplate
	supervisor *Supervisor
	ServeMux   *http.ServeMux
}

func CreateDefaultTelemetry(supervisor *Supervisor, addr string) SupervisedService {
	s := &telemetryService{supervisor: supervisor}

	s.ServeMux = http.NewServeMux()
	s.ServeMux.Handle("/readyz", http.HandlerFunc(s.ReadyHandler))
	s.ServeMux.Handle("/livez", http.HandlerFunc(s.AliveHandler))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(requestIDHeader, "telemetry")
		s.ServeMux.ServeHTTP(w, r)
	})

	cfg := &HTTPServiceConfigs{
		BaseConfigs: BaseConfigs{
			Name:   supervisor.Name + "/telemetry",
			Logger: supervisor.Logger,
		},
		HTTPServerOptions: DefaultTelemetryOptions(),
		Address:           addr,
		ShutdownTimeout:   shutdownTimeout,
	}
	InitHTTPServiceTemplate(&s.HTTPServiceTemplate, cfg, handler)

	s.Logger.Info("Telemetry", "address", addr)

	return s
}

// HTTP handler for `/s.Name/readyz` that exposes the value of Ready()
func (s *telemetryService) ReadyHandler(w http.ResponseWriter, _ *http.Request) {
	if !s.supervisor.Ready() {
		http.Error(w, s.Name+": ready check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, "%s: ready\n", s.Name)
	}
}

// HTTP handler for `/s.Name/livez` that exposes the value of Alive()
func (s *telemetryService) AliveHandler(w http.ResponseWriter, _ *http.Request) {
	if !s.supervisor.Alive() {
		http.Error(w, s.Name+": alive check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, "%s: alive\n", s.Name)
	}
}
