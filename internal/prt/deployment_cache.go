// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import "sync"

// The observer publishes values. Readers fence their database reads with the
// revision, without holding the mutex during database or protocol calls.
type deploymentCapture struct {
	revision uint64
	snapshot deploymentSnapshot
}

type deploymentCache struct {
	mu      sync.RWMutex
	entries map[deploymentKey]deploymentCapture
}

func (c *deploymentCache) capture(key deploymentKey) deploymentCapture {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok {
		entry.snapshot.Assessment.State = deploymentNone
	}
	return entry
}

func (c *deploymentCache) same(key deploymentKey, captured deploymentCapture) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries[key].revision == captured.revision
}

func (c *deploymentCache) publish(key deploymentKey, snapshot deploymentSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[deploymentKey]deploymentCapture)
	}
	c.entries[key] = deploymentCapture{revision: c.entries[key].revision + 1, snapshot: snapshot}
}
