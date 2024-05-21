// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package pmutex

import (
	"sync"
	"sync/atomic"
)

// A PMutex is a mutual exclusion lock with priority capabilities.
// A call to HLock always acquires the mutex before LLock.
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

// HLock acquires the mutex for the high-priority threads.
func (lock *PMutex) HLock() {
	lock.waitingHigh.Add(1)
	lock.mutex.Lock()
	lock.waitingHigh.Add(-1)
}

// LLock acquires the mutex for the low-priority threads.
// It waits until there are no high-priority threads trying to acquire the lock.
func (lock *PMutex) LLock() {
	lock.mutex.Lock()
	for lock.waitingHigh.Load() != 0 {
		// NOTE: a cond.Wait() releases the lock uppon being called
		// and tries to acquire it after being awakened.
		lock.waitingLow.Wait()
	}
}

// Unlock releases the mutex for both types of threads.
func (lock *PMutex) Unlock() {
	lock.waitingLow.Broadcast()
	lock.mutex.Unlock()
}
