// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package drain

import (
	"slices"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

// App models the drain-relevant lifecycle state for one application.
type App struct {
	ConsensusType model.Consensus
	Enabled       bool
	Deleted       bool
}

// RequiredServices returns the services that must acknowledge drain for the
// given consensus type.
func RequiredServices(ct model.Consensus) []string {
	services, err := repository.DrainServicesForConsensus(ct)
	if err != nil {
		return nil
	}
	return append([]string(nil), services...)
}

// IsActive mirrors the repository's active filter for lifecycle state.
func IsActive(app App) bool {
	return app.Enabled && !app.Deleted
}

// AllAcked reports whether the ack set contains every service required for the
// application's consensus type.
func AllAcked(ct model.Consensus, acked map[string]bool) bool {
	required := RequiredServices(ct)
	if len(required) == 0 {
		return false
	}
	for _, serviceName := range required {
		if !acked[serviceName] {
			return false
		}
	}
	return true
}

// AppsNeedingAck returns soft-deleted applications that the given service must
// acknowledge and has not already acknowledged.
func AppsNeedingAck(
	serviceName string,
	apps map[int]App,
	acks map[int]map[string]bool,
) []int {
	consensusTypes := repository.ConsensusTypesForService(serviceName)
	if len(consensusTypes) == 0 {
		return nil
	}

	allowed := make(map[model.Consensus]bool, len(consensusTypes))
	for _, ct := range consensusTypes {
		allowed[ct] = true
	}

	ids := make([]int, 0, len(apps))
	for appID := range apps {
		ids = append(ids, appID)
	}
	slices.Sort(ids)

	result := make([]int, 0, len(ids))
	for _, appID := range ids {
		app := apps[appID]
		if !app.Deleted || !allowed[app.ConsensusType] {
			continue
		}
		if acks[appID] != nil && acks[appID][serviceName] {
			continue
		}
		result = append(result, appID)
	}
	return result
}

// AckableApps returns the apps that a service can acknowledge on this tick
// after excluding apps still blocked by in-flight work.
func AckableApps(
	serviceName string,
	apps map[int]App,
	acks map[int]map[string]bool,
	inFlight map[int]bool,
) []int {
	needAck := AppsNeedingAck(serviceName, apps, acks)
	if len(inFlight) == 0 {
		return needAck
	}

	result := make([]int, 0, len(needAck))
	for _, appID := range needAck {
		if !inFlight[appID] {
			result = append(result, appID)
		}
	}
	return result
}
