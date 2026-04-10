// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package trace

import (
	"slices"

	"github.com/cartesi/rollups-node/internal/drain"
	"github.com/cartesi/rollups-node/internal/model"
)

// State represents one recorded DrainProtocol state.
type State struct {
	Action        string
	DBEnabled     map[int]bool
	DBDeleted     map[int]bool
	DBAcks        map[int]map[string]bool
	DBHardDeleted map[int]bool
	ForceDeleted  map[int]bool
	AdvMachines   map[int]bool
	ClmKnown      map[int]bool
	ClmInFlight   map[int]bool
	PrtKnown      map[int]bool
	PrtInFlight   map[int]bool
	SvcAlive      map[string]bool
}

// DrainState holds the mutable in-memory state used by the randomized
// simulation and trace recorder.
type DrainState struct {
	AppIDs              []int
	ServiceNames        []string
	ConsensusType       map[int]model.Consensus
	PrtTournamentActive map[int]bool
	DBEnabled           map[int]bool
	DBDeleted           map[int]bool
	DBAcks              map[int]map[string]bool
	DBHardDeleted       map[int]bool
	ForceDeleted        map[int]bool
	AdvMachines         map[int]bool
	ClmKnown            map[int]bool
	ClmInFlight         map[int]bool
	PrtKnown            map[int]bool
	PrtInFlight         map[int]bool
	SvcAlive            map[string]bool
}

// NewDrainState initializes the state to DrainProtocol.Init.
func NewDrainState(
	consensusType map[int]model.Consensus,
	prtTournamentActive map[int]bool,
	serviceNames []string,
) *DrainState {
	appIDs := sortedIntKeys(consensusType)
	sortedServices := append([]string(nil), serviceNames...)
	slices.Sort(sortedServices)

	state := &DrainState{
		AppIDs:              appIDs,
		ServiceNames:        sortedServices,
		ConsensusType:       copyConsensusMap(consensusType),
		PrtTournamentActive: copyIntBoolMap(prtTournamentActive),
		DBEnabled:           make(map[int]bool, len(appIDs)),
		DBDeleted:           make(map[int]bool, len(appIDs)),
		DBAcks:              make(map[int]map[string]bool, len(appIDs)),
		DBHardDeleted:       make(map[int]bool),
		ForceDeleted:        make(map[int]bool),
		AdvMachines:         make(map[int]bool, len(appIDs)),
		ClmKnown:            make(map[int]bool),
		ClmInFlight:         make(map[int]bool),
		PrtKnown:            make(map[int]bool),
		PrtInFlight:         make(map[int]bool),
		SvcAlive:            make(map[string]bool, len(sortedServices)),
	}

	for _, appID := range appIDs {
		state.DBEnabled[appID] = true
		state.DBDeleted[appID] = false
		state.DBAcks[appID] = make(map[string]bool)
		state.AdvMachines[appID] = true
		if _, ok := state.PrtTournamentActive[appID]; !ok {
			state.PrtTournamentActive[appID] = false
		}
	}
	for _, serviceName := range sortedServices {
		state.SvcAlive[serviceName] = true
	}

	return state
}

// Snapshot creates a deep copy suitable for recording.
func (s *DrainState) Snapshot(action string) State {
	return State{
		Action:        action,
		DBEnabled:     copyIntBoolMap(s.DBEnabled),
		DBDeleted:     copyIntBoolMap(s.DBDeleted),
		DBAcks:        copyAckMap(s.DBAcks),
		DBHardDeleted: copyIntBoolMap(s.DBHardDeleted),
		ForceDeleted:  copyIntBoolMap(s.ForceDeleted),
		AdvMachines:   copyIntBoolMap(s.AdvMachines),
		ClmKnown:      copyIntBoolMap(s.ClmKnown),
		ClmInFlight:   copyIntBoolMap(s.ClmInFlight),
		PrtKnown:      copyIntBoolMap(s.PrtKnown),
		PrtInFlight:   copyIntBoolMap(s.PrtInFlight),
		SvcAlive:      copyStrBoolMap(s.SvcAlive),
	}
}

// AliveApps returns app IDs whose row still exists in the database.
func (s *DrainState) AliveApps() []int {
	apps := make([]int, 0, len(s.AppIDs))
	for _, appID := range s.AppIDs {
		if !s.DBHardDeleted[appID] {
			apps = append(apps, appID)
		}
	}
	return apps
}

// IsActive mirrors the TLA+ IsActive predicate.
func (s *DrainState) IsActive(appID int) bool {
	if s.DBHardDeleted[appID] {
		return false
	}
	return drain.IsActive(drain.App{
		ConsensusType: s.ConsensusType[appID],
		Enabled:       s.DBEnabled[appID],
		Deleted:       s.DBDeleted[appID],
	})
}

func (s *DrainState) RequiredServices(appID int) []string {
	return drain.RequiredServices(s.ConsensusType[appID])
}

func (s *DrainState) AllAcked(appID int) bool {
	return drain.AllAcked(s.ConsensusType[appID], s.DBAcks[appID])
}

func (s *DrainState) ClaimerVisible(appID int) bool {
	return s.IsActive(appID) && s.ConsensusType[appID] != model.Consensus_PRT
}

func (s *DrainState) PRTVisible(appID int) bool {
	return s.IsActive(appID) && s.ConsensusType[appID] == model.Consensus_PRT
}

func (s *DrainState) SafeToDisable(appID int) bool {
	return s.ConsensusType[appID] != model.Consensus_PRT || !s.PrtTournamentActive[appID]
}

// Ack records a service ack for an app.
func (s *DrainState) Ack(appID int, serviceName string) {
	if s.DBAcks[appID] == nil {
		s.DBAcks[appID] = make(map[string]bool)
	}
	s.DBAcks[appID][serviceName] = true
}

func copyConsensusMap(src map[int]model.Consensus) map[int]model.Consensus {
	dst := make(map[int]model.Consensus, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func copyIntBoolMap(src map[int]bool) map[int]bool {
	dst := make(map[int]bool, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func copyStrBoolMap(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func copyAckMap(src map[int]map[string]bool) map[int]map[string]bool {
	dst := make(map[int]map[string]bool, len(src))
	for appID, ackSet := range src {
		dst[appID] = copyStrBoolMap(ackSet)
	}
	return dst
}
