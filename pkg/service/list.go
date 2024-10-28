// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// A Service implementation in terms of a list of other services
type ListService struct {
	Service
	Children []*Service
}

type CreateListInfo struct {
	CreateInfo
}

func CreateList(ci CreateListInfo, list *ListService) error {
	err := Create(&ci.CreateInfo, &list.Service)
	if err != nil {
		return err
	}

	if ci.CreateInfo.TelemetryCreate {
		list.ServeMux.Handle("/"+list.Name+"/readyz",
			http.HandlerFunc(list.ReadyHandler))
		list.ServeMux.Handle("/"+list.Name+"/livez",
			http.HandlerFunc(list.AliveHandler))
	}
	return nil
}

func (me *ListService) Alive() bool {
	alive := true
	for _, s := range me.Children {
		alive = alive && s.Alive()
	}
	return alive
}

func (me *ListService) Ready() bool {
	allReady := true
	for _, s := range me.Children {
		ready := s.Ready()
		allReady = allReady && ready
	}
	return allReady
}

func (me *ListService) Reload() []error {
	var all []error
	for _, s := range me.Children {
		es := s.Reload()
		all = append(all, es...)
	}
	return all
}

/*
// Simple tick runs children one at a time
func (me *ListService) Tick() []error {
	var all []error
	for _, s := range me.Children {
		all = append(all, s.Tick()...)
	}
	return all
}
*/

// *
func (me *ListService) Tick() []error {
	c := make(chan []error)
	var all []error
	for _, s := range me.Children {
		go func() {
			c <- s.Tick()
		}()
	}

	limit := time.Duration(0.95 * float64(me.Service.PollInterval))
	deadline := time.After(limit)
	for range me.Children {
		select {
		case errs := <-c:
			all = append(all, errs...)
		case <-deadline:
			me.Logger.Warn("Tick:time limit exceeded",
				"service", me.Name,
				"limit", limit)
			return all
		}
	}
	return all
}

//*/

func (me *ListService) Stop(force bool) []error {
	var all []error
	for _, s := range me.Children {
		es := s.Stop(force)
		all = append(all, es...)
	}
	return all
}

func (me *ListService) AliveHandler(w http.ResponseWriter, r *http.Request) {
	ss := []string{}

	allAlive := true
	for _, s := range me.Children {
		alive := s.Alive()
		allAlive = allAlive && alive
		ss = append(ss, fmt.Sprintf("%s: %v", s.Name, alive))
	}
	ss = append(ss, fmt.Sprintf("%s: %v", me.Name, allAlive))

	if allAlive {
		http.Error(w, strings.Join(ss, "\n"),
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, strings.Join(ss, "\n"))
	}
}

func (me *ListService) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	ss := []string{}

	allReady := true
	for _, s := range me.Children {
		ready := s.Ready()
		allReady = allReady && ready
		ss = append(ss, fmt.Sprintf("%s: %v", s.Name, ready))
	}
	ss = append(ss, fmt.Sprintf("%s: %v", me.Name, allReady))

	if allReady {
		http.Error(w, strings.Join(ss, "\n"),
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, strings.Join(ss, "\n"))
	}
}
