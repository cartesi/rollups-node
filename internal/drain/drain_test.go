// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package drain_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cartesi/rollups-node/internal/drain"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func TestConsensusAwareRequiredServices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		consensusType model.Consensus
		want          []string
	}{
		{
			consensusType: model.Consensus_Authority,
			want:          []string{repository.ServiceAdvancer, repository.ServiceClaimer},
		},
		{
			consensusType: model.Consensus_Quorum,
			want:          []string{repository.ServiceAdvancer, repository.ServiceClaimer},
		},
		{
			consensusType: model.Consensus_PRT,
			want:          []string{repository.ServiceAdvancer, repository.ServicePRT},
		},
	}

	for _, tc := range tests {
		t.Run(string(tc.consensusType), func(t *testing.T) {
			assert.Equal(t, tc.want, drain.RequiredServices(tc.consensusType))
		})
	}
}

func TestPurgeRequiresAllAcks(t *testing.T) {
	t.Parallel()

	allServices := []string{
		repository.ServiceAdvancer,
		repository.ServiceClaimer,
		repository.ServicePRT,
	}

	for _, consensusType := range model.ConsensusAllValues {
		required := drain.RequiredServices(consensusType)
		t.Run(string(consensusType), func(t *testing.T) {
			for mask := 0; mask < 1<<len(allServices); mask++ {
				acked := make(map[string]bool, len(allServices))
				subset := make([]string, 0, len(allServices))
				for i, serviceName := range allServices {
					if mask&(1<<i) == 0 {
						continue
					}
					acked[serviceName] = true
					subset = append(subset, serviceName)
				}

				want := true
				for _, serviceName := range required {
					if !acked[serviceName] {
						want = false
						break
					}
				}

				assert.Equalf(t, want, drain.AllAcked(consensusType, acked),
					"consensus=%s acked=%v", consensusType, subset)
			}
		})
	}
}

func TestReEnableCycle(t *testing.T) {
	t.Parallel()

	apps := map[int]drain.App{
		1: {
			ConsensusType: model.Consensus_Authority,
			Enabled:       true,
			Deleted:       false,
		},
	}
	acks := map[int]map[string]bool{
		1: {},
	}

	assert.True(t, drain.IsActive(apps[1]))
	assert.False(t, drain.AllAcked(apps[1].ConsensusType, acks[1]))

	app := apps[1]
	app.Enabled = false
	apps[1] = app
	assert.False(t, drain.IsActive(apps[1]))
	assert.Empty(t, drain.AppsNeedingAck(repository.ServiceAdvancer, apps, acks))

	app.Enabled = true
	apps[1] = app
	assert.True(t, drain.IsActive(apps[1]))
	assert.Empty(t, acks[1])

	app.Enabled = false
	app.Deleted = true
	apps[1] = app

	assert.Equal(t, []int{1}, drain.AppsNeedingAck(repository.ServiceAdvancer, apps, acks))
	assert.Equal(t, []int{1}, drain.AppsNeedingAck(repository.ServiceClaimer, apps, acks))

	acks[1][repository.ServiceAdvancer] = true
	assert.False(t, drain.AllAcked(apps[1].ConsensusType, acks[1]))

	acks[1][repository.ServiceClaimer] = true
	assert.True(t, drain.AllAcked(apps[1].ConsensusType, acks[1]))

	got := mapsKeys(acks[1])
	assert.Equal(t,
		[]string{repository.ServiceAdvancer, repository.ServiceClaimer},
		got,
	)
}

func mapsKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key, value := range m {
		if value {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func ExampleRequiredServices() {
	fmt.Println(drain.RequiredServices(model.Consensus_PRT))
	// Output: [advancer prt]
}
