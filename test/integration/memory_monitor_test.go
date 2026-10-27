// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"fmt"
	"sync"
	"time"
)

type rssMeasurement struct {
	baseline, peak uint64
	samples        uint64
	err            error
}

// monitorNodeRSS sums the managed service processes in multiprocess mode.
// Shared resident pages can be counted more than once. Child machine processes
// are excluded. The peak is sampled every 10 ms; shorter spikes may be missed.
// Call stop before reading the result; repeated stops return the same result.
func monitorNodeRSS(node nodeHandle) (stop func() rssMeasurement, err error) {
	var procs []*nodeSubprocess
	switch n := node.(type) {
	case *nodeProcess:
		procs = []*nodeSubprocess{n.nodeSubprocess}
	case *multiNode:
		procs = n.procs
	default:
		return nil, fmt.Errorf("RSS measurement requires managed node processes")
	}
	read := func() (uint64, error) {
		var total uint64
		for _, p := range procs {
			rss, err := processRSS(p.cmd.Process.Pid)
			if err != nil {
				return 0, fmt.Errorf("read %s RSS: %w", p.name, err)
			}
			total += rss
		}
		return total, nil
	}
	baseline, err := read()
	if err != nil {
		return nil, err
	}
	result := rssMeasurement{baseline: baseline, peak: baseline, samples: 1}
	quit, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				rss, err := read()
				if err != nil {
					result.err = err
					return
				}
				result.peak = max(result.peak, rss)
				result.samples++
			}
		}
	}()
	var once sync.Once
	return func() rssMeasurement {
		once.Do(func() { close(quit) })
		<-done
		return result
	}, nil
}
