// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package pmutex provides a mutual exclusion lock with priority capabilities.
package pmutex

import (
	"sync"
	"sync/atomic"
)

// A PMutex is a mutual exclusion lock with priority capabilities.
// When contested, a call to HLock always acquires the mutex before LLock.
type PMutex struct {
	// Main mutex.
	mutex *sync.Mutex

	// Condition variable for the waiting low-priority threads.
	waitingLow *sync.Cond

	// Quantity of high-priority threads waiting to acquire the lock.
	waitingHigh *atomic.Int32
}

// New creates a new PMutex.
func New() *PMutex {
	mutex := &sync.Mutex{}
	return &PMutex{
		mutex:       mutex,
		waitingLow:  sync.NewCond(mutex),
		waitingHigh: &atomic.Int32{},
	}
}

// HLock acquires the mutex for high-priority threads.
func (pmutex *PMutex) HLock() {
	pmutex.waitingHigh.Add(1)
	pmutex.mutex.Lock()
	pmutex.waitingHigh.Add(-1)
}

// LLock acquires the mutex for low-priority threads.
// (It waits until there are no high-priority threads trying to acquire the lock.)
func (pmutex *PMutex) LLock() {
	pmutex.mutex.Lock()

	// Even after acquiring the lock, a low-priority thread releases it if there are any
	// high-priority threads waiting.
	for pmutex.waitingHigh.Load() != 0 {
		// NOTE: a cond.Wait() releases the lock uppon being called
		// and tries to acquire it after being awakened.
		pmutex.waitingLow.Wait()
	}
}

// Unlock releases the mutex for both types of threads.
func (pmutex *PMutex) Unlock() {
	pmutex.waitingLow.Broadcast()
	pmutex.mutex.Unlock()
}
