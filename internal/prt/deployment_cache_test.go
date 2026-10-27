// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeploymentCacheRevisionChangesForEqualPublications(t *testing.T) {
	var cache deploymentCache
	key := deploymentIdentity(prtRevertTestApp())
	original := cache.capture(key)
	require.Equal(t, deploymentNone, original.snapshot.Assessment.State)
	snapshot := deploymentSnapshot{Acknowledged: true, ResetGeneration: 1, Assessment: deploymentAssessment{State: deploymentCompatible}}
	cache.publish(key, snapshot)
	current := cache.capture(key)
	require.False(t, cache.same(key, original))
	require.True(t, cache.same(key, current))
	cache.publish(key, snapshot)
	require.False(t, cache.same(key, current), "an equal publication must still fence a concurrent database read")
	require.Equal(t, snapshot, cache.capture(key).snapshot)
}

func TestDeploymentCacheConcurrentPublicationAndConsumption(t *testing.T) {
	var cache deploymentCache
	key := deploymentIdentity(prtRevertTestApp())
	snapshot := deploymentSnapshot{Acknowledged: true, ResetGeneration: 1, Assessment: deploymentAssessment{State: deploymentCompatible}}
	cache.publish(key, snapshot)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 200 {
				captured := cache.capture(key)
				if captured.snapshot.ResetGeneration != 1 {
					t.Error("mixed cache publication")
					return
				}
				_ = cache.same(key, captured)
			}
		})
	}
	wg.Go(func() {
		for range 200 {
			cache.publish(key, snapshot)
		}
	})
	wg.Wait()
	require.Equal(t, snapshot, cache.capture(key).snapshot)
}
