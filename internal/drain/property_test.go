// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package drain_test

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/drain"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func TestScanDiscoversSoftDeletedApps(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(42, 0)) //nolint:gosec
	services := []string{
		repository.ServiceAdvancer,
		repository.ServiceClaimer,
		repository.ServicePRT,
	}

	for trial := range 50 {
		apps, acks := randomAppsAndAcks(rng, 8)
		for _, serviceName := range services {
			got := drain.AppsNeedingAck(serviceName, apps, acks)
			want := expectedAppsNeedingAck(serviceName, apps, acks)
			assert.Equalf(t, want, got, "trial=%d service=%s", trial, serviceName)
		}
	}
}

func TestPRTDefersOnInFlight(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(123, 0)) //nolint:gosec
	for trial := range 50 {
		apps, acks := randomAppsAndAcks(rng, 8)
		inFlight := make(map[int]bool)

		base := drain.AppsNeedingAck(repository.ServicePRT, apps, acks)
		for _, appID := range base {
			if rng.IntN(2) == 0 {
				inFlight[appID] = true
			}
		}

		got := drain.AckableApps(repository.ServicePRT, apps, acks, inFlight)
		want := filterOut(base, inFlight)
		assert.Equalf(t, want, got, "trial=%d", trial)
	}
}

func TestClaimerDefersOnInFlight(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(7, 0)) //nolint:gosec
	for trial := range 50 {
		apps, acks := randomAppsAndAcks(rng, 8)
		inFlight := make(map[int]bool)

		base := drain.AppsNeedingAck(repository.ServiceClaimer, apps, acks)
		for _, appID := range base {
			if rng.IntN(2) == 0 {
				inFlight[appID] = true
			}
		}

		got := drain.AckableApps(repository.ServiceClaimer, apps, acks, inFlight)
		want := filterOut(base, inFlight)
		assert.Equalf(t, want, got, "trial=%d", trial)
	}

	apps := map[int]drain.App{
		1: {
			ConsensusType: model.Consensus_Authority,
			Enabled:       false,
			Deleted:       true,
		},
	}
	acks := map[int]map[string]bool{1: {}}
	inFlight := map[int]bool{1: true}

	assert.Empty(t, drain.AckableApps(repository.ServiceClaimer, apps, acks, inFlight))

	delete(inFlight, 1)
	assert.Equal(t, []int{1},
		drain.AckableApps(repository.ServiceClaimer, apps, acks, inFlight))
}

func TestNoAcksBeforeDrain(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(999, 0)) //nolint:gosec
	services := []string{
		repository.ServiceAdvancer,
		repository.ServiceClaimer,
		repository.ServicePRT,
	}

	for seed := range 10 {
		apps := make(map[int]drain.App, 5)
		acks := make(map[int]map[string]bool, 5)
		for appID := 1; appID <= 5; appID++ {
			apps[appID] = drain.App{
				ConsensusType: randomConsensus(rng),
				Enabled:       true,
				Deleted:       false,
			}
			acks[appID] = make(map[string]bool)
		}

		for step := range 100 {
			appID := rng.IntN(len(apps)) + 1
			app := apps[appID]

			switch rng.IntN(5) {
			case 0:
				if !app.Deleted {
					app.Enabled = false
				}
			case 1:
				if !app.Deleted {
					app.Enabled = true
				}
			case 2:
				app.Enabled = false
				app.Deleted = true
			case 3:
				serviceName := services[rng.IntN(len(services))]
				for _, ackAppID := range drain.AckableApps(serviceName, apps, acks, nil) {
					ensureAckMap(acks, ackAppID)[serviceName] = true
				}
			default:
				for _, serviceName := range services {
					for _, ackAppID := range drain.AckableApps(serviceName, apps, acks, nil) {
						ensureAckMap(acks, ackAppID)[serviceName] = true
					}
				}
			}

			apps[appID] = app
			for checkID, checkApp := range apps {
				if checkApp.Deleted {
					continue
				}
				assert.Emptyf(t, mapsKeys(acks[checkID]),
					"seed=%d step=%d app=%d", seed, step, checkID)
			}
		}
	}
}

func TestDrainEventuallyCompletes(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(2024, 0)) //nolint:gosec

	for trial := range 20 {
		apps := make(map[int]drain.App, 6)
		acks := make(map[int]map[string]bool, 6)
		var deletedCount int
		for appID := 1; appID <= 6; appID++ {
			app := drain.App{
				ConsensusType: randomConsensus(rng),
				Enabled:       false,
				Deleted:       rng.IntN(2) == 0,
			}
			if app.Deleted {
				deletedCount++
			} else {
				app.Enabled = true
			}
			apps[appID] = app
			acks[appID] = make(map[string]bool)
		}
		if deletedCount == 0 {
			apps[1] = drain.App{
				ConsensusType: model.Consensus_Authority,
				Enabled:       false,
				Deleted:       true,
			}
		}

		claimerInFlight := make(map[int]bool)
		prtInFlight := make(map[int]bool)
		for appID, app := range apps {
			if !app.Deleted {
				continue
			}
			switch app.ConsensusType {
			case model.Consensus_Authority, model.Consensus_Quorum:
				if rng.IntN(2) == 0 {
					claimerInFlight[appID] = true
				}
			case model.Consensus_PRT:
				if rng.IntN(2) == 0 {
					prtInFlight[appID] = true
				}
			}
		}

		for range 8 {
			applyAcks(acks, repository.ServiceAdvancer,
				drain.AckableApps(repository.ServiceAdvancer, apps, acks, nil))
			applyAcks(acks, repository.ServiceClaimer,
				drain.AckableApps(repository.ServiceClaimer, apps, acks, claimerInFlight))
			applyAcks(acks, repository.ServicePRT,
				drain.AckableApps(repository.ServicePRT, apps, acks, prtInFlight))

			releaseOne(claimerInFlight)
			releaseOne(prtInFlight)
		}

		for appID, app := range apps {
			if !app.Deleted {
				continue
			}
			assert.Truef(t, drain.AllAcked(app.ConsensusType, acks[appID]),
				"trial=%d app=%d", trial, appID)
		}
	}
}

func randomAppsAndAcks(rng *rand.Rand, count int) (map[int]drain.App, map[int]map[string]bool) {
	apps := make(map[int]drain.App, count)
	acks := make(map[int]map[string]bool, count)
	for appID := 1; appID <= count; appID++ {
		deleted := rng.IntN(3) == 0
		apps[appID] = drain.App{
			ConsensusType: randomConsensus(rng),
			Enabled:       !deleted && rng.IntN(2) == 0,
			Deleted:       deleted,
		}
		acks[appID] = make(map[string]bool)
		for _, serviceName := range []string{
			repository.ServiceAdvancer,
			repository.ServiceClaimer,
			repository.ServicePRT,
		} {
			if deleted && rng.IntN(4) == 0 {
				acks[appID][serviceName] = true
			}
		}
	}
	return apps, acks
}

func expectedAppsNeedingAck(
	serviceName string,
	apps map[int]drain.App,
	acks map[int]map[string]bool,
) []int {
	allowed := make(map[model.Consensus]bool)
	for _, ct := range repository.ConsensusTypesForService(serviceName) {
		allowed[ct] = true
	}

	result := make([]int, 0, len(apps))
	for appID, app := range apps {
		if !app.Deleted || !allowed[app.ConsensusType] {
			continue
		}
		if acks[appID][serviceName] {
			continue
		}
		result = append(result, appID)
	}
	slices.Sort(result)
	return result
}

func filterOut(ids []int, excluded map[int]bool) []int {
	result := make([]int, 0, len(ids))
	for _, id := range ids {
		if !excluded[id] {
			result = append(result, id)
		}
	}
	return result
}

func applyAcks(acks map[int]map[string]bool, serviceName string, appIDs []int) {
	for _, appID := range appIDs {
		ensureAckMap(acks, appID)[serviceName] = true
	}
}

func ensureAckMap(acks map[int]map[string]bool, appID int) map[string]bool {
	if acks[appID] == nil {
		acks[appID] = make(map[string]bool)
	}
	return acks[appID]
}

func releaseOne(inFlight map[int]bool) {
	if len(inFlight) == 0 {
		return
	}

	ids := make([]int, 0, len(inFlight))
	for appID := range inFlight {
		ids = append(ids, appID)
	}
	slices.Sort(ids)
	delete(inFlight, ids[0])
}

func randomConsensus(rng *rand.Rand) model.Consensus {
	return model.ConsensusAllValues[rng.IntN(len(model.ConsensusAllValues))]
}

func TestHelperSanity(t *testing.T) {
	t.Parallel()

	apps := map[int]drain.App{
		1: {ConsensusType: model.Consensus_Authority, Enabled: false, Deleted: true},
	}
	acks := map[int]map[string]bool{}
	require.Equal(t, []int{1}, drain.AppsNeedingAck(repository.ServiceAdvancer, apps, acks))
}
