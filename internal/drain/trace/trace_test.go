// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package trace_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	draintrace "github.com/cartesi/rollups-node/internal/drain/trace"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

const simMaxSteps = 50

var (
	tlaConsensusType = map[int]model.Consensus{
		1: model.Consensus_Authority,
		2: model.Consensus_Quorum,
		3: model.Consensus_PRT,
	}
	tlaPrtTournamentActive = map[int]bool{
		1: false,
		2: false,
		3: true,
	}
	tlaServiceNames = []string{
		repository.ServiceAdvancer,
		repository.ServiceClaimer,
		repository.ServicePRT,
	}
)

type action struct {
	name        string
	appID       int
	serviceName string
}

func (a action) label() string {
	switch {
	case a.appID != 0:
		return fmt.Sprintf("%s(%d)", a.name, a.appID)
	case a.serviceName != "":
		return fmt.Sprintf("%s(%q)", a.name, a.serviceName)
	default:
		return a.name
	}
}

func TestTraceGeneration(t *testing.T) {
	seeds := []uint64{42, 123, 7, 999, 2024}

	for _, seed := range seeds {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			rec := runSimulation(t, seed)
			verifySafetyInvariants(t, rec)
			runTLCValidation(t, rec, seed)
		})
	}
}

func runSimulation(t *testing.T, seed uint64) *draintrace.Recorder {
	t.Helper()

	rng := rand.New(rand.NewPCG(seed, 0)) //nolint:gosec
	state := draintrace.NewDrainState(tlaConsensusType, tlaPrtTournamentActive, tlaServiceNames)
	rec := draintrace.NewRecorder()
	rec.Record(state.Snapshot("Init"))

	for range simMaxSteps {
		actions := enabledActions(state)
		if len(actions) == 0 {
			break
		}

		act := chooseAction(rng, actions)
		applyAction(state, act)
		rec.Record(state.Snapshot(act.label()))
	}

	t.Logf("seed %d: generated trace with %d states", seed, len(rec.States))
	return rec
}

func enabledActions(state *draintrace.DrainState) []action {
	actions := make([]action, 0)
	for _, appID := range state.AliveApps() {
		if state.DBEnabled[appID] && !state.DBDeleted[appID] && state.SafeToDisable(appID) {
			actions = append(actions, action{name: "OperatorDisable", appID: appID})
		}
		if state.DBEnabled[appID] && !state.DBDeleted[appID] && !state.SafeToDisable(appID) {
			actions = append(actions, action{name: "OperatorForceDisable", appID: appID})
			actions = append(actions, action{name: "OperatorForceSoftDelete", appID: appID})
		}
		if !state.DBEnabled[appID] && !state.DBDeleted[appID] {
			actions = append(actions, action{name: "OperatorReEnable", appID: appID})
			actions = append(actions, action{name: "OperatorSoftDelete", appID: appID})
		}
		if !state.DBDeleted[appID] && (!state.DBEnabled[appID] || state.SafeToDisable(appID)) {
			actions = append(actions, action{name: "OperatorSoftDelete", appID: appID})
		}
		if state.DBDeleted[appID] && state.AllAcked(appID) {
			actions = append(actions, action{name: "OperatorPurge", appID: appID})
		}
		if state.DBDeleted[appID] {
			actions = append(actions, action{name: "OperatorForcePurge", appID: appID})
		}
		if state.SvcAlive[repository.ServiceClaimer] &&
			state.ClmKnown[appID] &&
			!state.ClmInFlight[appID] &&
			state.ClaimerVisible(appID) {
			actions = append(actions, action{name: "ClaimerSubmitClaim", appID: appID})
		}
		if state.ClmInFlight[appID] {
			actions = append(actions, action{name: "ClaimerClaimConfirmed", appID: appID})
		}
		if state.SvcAlive[repository.ServicePRT] &&
			state.PrtKnown[appID] &&
			state.PRTVisible(appID) &&
			!state.PrtInFlight[appID] {
			actions = append(actions, action{name: "PRTSubmitTx", appID: appID})
		}
		if state.PrtInFlight[appID] {
			actions = append(actions, action{name: "PRTTxConfirmed", appID: appID})
		}
	}

	if state.SvcAlive[repository.ServiceAdvancer] {
		actions = append(actions, action{name: "AdvancerTick"})
		actions = append(actions, action{name: "ServiceCrash", serviceName: repository.ServiceAdvancer})
	} else {
		actions = append(actions, action{name: "ServiceRestart", serviceName: repository.ServiceAdvancer})
	}
	if state.SvcAlive[repository.ServiceClaimer] {
		actions = append(actions, action{name: "ClaimerTick"})
		actions = append(actions, action{name: "ServiceCrash", serviceName: repository.ServiceClaimer})
	} else {
		actions = append(actions, action{name: "ServiceRestart", serviceName: repository.ServiceClaimer})
	}
	if state.SvcAlive[repository.ServicePRT] {
		actions = append(actions, action{name: "PRTTick"})
		actions = append(actions, action{name: "ServiceCrash", serviceName: repository.ServicePRT})
	} else {
		actions = append(actions, action{name: "ServiceRestart", serviceName: repository.ServicePRT})
	}

	return actions
}

func chooseAction(rng *rand.Rand, actions []action) action {
	weighted := make([]action, 0, len(actions)*2)
	for _, act := range actions {
		weight := 1
		switch act.name {
		case "AdvancerTick", "ClaimerTick", "PRTTick",
			"ClaimerClaimConfirmed", "PRTTxConfirmed", "ServiceRestart":
			weight = 3
		}
		for range weight {
			weighted = append(weighted, act)
		}
	}
	return weighted[rng.IntN(len(weighted))]
}

func applyAction(state *draintrace.DrainState, act action) {
	switch act.name {
	case "OperatorDisable", "OperatorForceDisable":
		state.DBEnabled[act.appID] = false
	case "OperatorReEnable":
		state.DBEnabled[act.appID] = true
	case "OperatorSoftDelete", "OperatorForceSoftDelete":
		state.DBEnabled[act.appID] = false
		state.DBDeleted[act.appID] = true
	case "OperatorPurge":
		state.DBHardDeleted[act.appID] = true
	case "OperatorForcePurge":
		state.DBHardDeleted[act.appID] = true
		state.ForceDeleted[act.appID] = true
	case "AdvancerTick":
		active := make(map[int]bool)
		for _, appID := range state.AliveApps() {
			if state.IsActive(appID) {
				active[appID] = true
			}
			if state.DBDeleted[appID] &&
				slices.Contains(state.RequiredServices(appID), repository.ServiceAdvancer) {
				state.Ack(appID, repository.ServiceAdvancer)
			}
		}
		state.AdvMachines = active
	case "ClaimerTick":
		visible := make(map[int]bool)
		for _, appID := range state.AliveApps() {
			if state.ClaimerVisible(appID) {
				visible[appID] = true
			}
			if state.DBDeleted[appID] &&
				slices.Contains(state.RequiredServices(appID), repository.ServiceClaimer) &&
				!state.ClmInFlight[appID] {
				state.Ack(appID, repository.ServiceClaimer)
			}
		}
		state.ClmKnown = visible
		for appID := range state.ClmInFlight {
			if !visible[appID] {
				delete(state.ClmInFlight, appID)
			}
		}
	case "PRTTick":
		visible := make(map[int]bool)
		for _, appID := range state.AliveApps() {
			if state.PRTVisible(appID) {
				visible[appID] = true
			}
			if state.DBDeleted[appID] &&
				slices.Contains(state.RequiredServices(appID), repository.ServicePRT) &&
				!state.PrtInFlight[appID] {
				state.Ack(appID, repository.ServicePRT)
			}
		}
		state.PrtKnown = visible
		for appID := range state.PrtInFlight {
			if !visible[appID] {
				delete(state.PrtInFlight, appID)
			}
		}
	case "ClaimerSubmitClaim":
		state.ClmInFlight[act.appID] = true
	case "ClaimerClaimConfirmed":
		delete(state.ClmInFlight, act.appID)
	case "PRTSubmitTx":
		state.PrtInFlight[act.appID] = true
	case "PRTTxConfirmed":
		delete(state.PrtInFlight, act.appID)
	case "ServiceCrash":
		state.SvcAlive[act.serviceName] = false
		switch act.serviceName {
		case repository.ServiceAdvancer:
			state.AdvMachines = make(map[int]bool)
		case repository.ServiceClaimer:
			state.ClmKnown = make(map[int]bool)
			state.ClmInFlight = make(map[int]bool)
		case repository.ServicePRT:
			state.PrtKnown = make(map[int]bool)
			state.PrtInFlight = make(map[int]bool)
		}
	case "ServiceRestart":
		state.SvcAlive[act.serviceName] = true
	}
}

func verifySafetyInvariants(t *testing.T, rec *draintrace.Recorder) {
	t.Helper()

	for i, s := range rec.States {
		for appID, forceDeleted := range s.ForceDeleted {
			if forceDeleted {
				assert.Truef(t, s.DBHardDeleted[appID],
					"step %d (%s): forceDeleted app %d without hard delete", i, s.Action, appID)
			}
		}

		for appID, hardDeleted := range s.DBHardDeleted {
			if !hardDeleted {
				continue
			}
			if !s.ForceDeleted[appID] {
				assert.Truef(t, allAckedForApp(appID, s.DBAcks, tlaConsensusType[appID]),
					"step %d (%s): purged app %d without all acks", i, s.Action, appID)
			}
		}

		for appID, acked := range s.DBAcks {
			if acked[repository.ServiceAdvancer] {
				assert.Falsef(t, s.AdvMachines[appID],
					"step %d (%s): advancer acked app %d but machine still present", i, s.Action, appID)
			}
			if acked[repository.ServiceClaimer] {
				assert.Falsef(t, s.ClmInFlight[appID],
					"step %d (%s): claimer acked app %d but claim still in flight", i, s.Action, appID)
			}
			if acked[repository.ServicePRT] {
				assert.Falsef(t, s.PrtInFlight[appID],
					"step %d (%s): prt acked app %d but tx still in flight", i, s.Action, appID)
			}
			if !s.DBDeleted[appID] {
				assert.Emptyf(t, ackKeys(acked),
					"step %d (%s): non-deleted app %d has acks", i, s.Action, appID)
			}
		}

		for appID, deleted := range s.DBDeleted {
			if deleted {
				assert.Falsef(t, s.DBEnabled[appID],
					"step %d (%s): deleted app %d still enabled", i, s.Action, appID)
			}
		}
	}
}

func allAckedForApp(appID int, acks map[int]map[string]bool, consensusType model.Consensus) bool {
	required, err := repository.DrainServicesForConsensus(consensusType)
	if err != nil {
		return false
	}
	for _, serviceName := range required {
		if !acks[appID][serviceName] {
			return false
		}
	}
	return true
}

func ackKeys(acked map[string]bool) []string {
	keys := make([]string, 0, len(acked))
	for key, value := range acked {
		if value {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func runTLCValidation(t *testing.T, rec *draintrace.Recorder, seed uint64) {
	t.Helper()

	tlaJar := findTLAToolsJar()
	requireTLC := tlcRequired()
	tmpDir := t.TempDir()

	content, err := rec.WriteTLA(filepath.Join(tmpDir, "Trace.tla"))
	require.NoError(t, err)
	t.Logf("Generated Trace.tla (%d bytes, %d states)", len(content), len(rec.States))

	specDir := findSpecDir(t)
	copyTLAFile(t, specDir, tmpDir, "DrainProtocol.tla")
	copyTLAFile(t, specDir, tmpDir, "TraceDrainProtocol.tla")
	copyTLAFile(t, specDir, tmpDir, "TraceDrainProtocol.cfg")

	if tlaJar == "" {
		if requireTLC {
			t.Fatal("TLC is required but TLA_TOOLS_JAR is not set and tla2tools.jar was not found")
		}
		t.Log("TLA_TOOLS_JAR not set and tla2tools.jar not found; " +
			"skipping TLC validation (Go safety checks passed)")
		t.Logf("To run TLC manually:\n  cd %s\n  "+
			"java -jar tla2tools.jar -config TraceDrainProtocol.cfg "+
			"-deadlock -workers auto TraceDrainProtocol", tmpDir)
		return
	}

	cmd := exec.Command(
		"java", "-jar", tlaJar,
		"-config", "TraceDrainProtocol.cfg",
		"-deadlock",
		"-workers", "1",
		"TraceDrainProtocol",
	)
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "Operation not permitted") &&
			!requireTLC {
			t.Log("TLC requires local socket permissions in this environment; " +
				"skipping TLC validation after Go-side safety checks")
			return
		}
		t.Fatalf("TLC failed for seed %d:\n%s", seed, string(output))
	}
	t.Logf("TLC validated trace (seed %d): %d states OK", seed, len(rec.States))
}

func findTLAToolsJar() string {
	if jar := os.Getenv("TLA_TOOLS_JAR"); jar != "" {
		return jar
	}
	candidates := []string{
		"/opt/tla/tla2tools.jar",
		filepath.Join(os.Getenv("HOME"), "tla2tools.jar"),
	}
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if _, err := os.Stat(clean); err == nil { //nolint:gosec
			return clean
		}
	}
	return ""
}

func tlcRequired() bool {
	return os.Getenv("CARTESI_REQUIRE_TLC") == "1"
}

func findSpecDir(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		candidate := filepath.Join(dir, "spec", "drain-protocol")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find spec/drain-protocol directory")
		}
		dir = parent
	}
}

func copyTLAFile(t *testing.T, srcDir, dstDir, filename string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(srcDir, filename))
	require.NoError(t, err, "reading %s", filename)
	err = os.WriteFile(filepath.Join(dstDir, filename), data, 0644)
	require.NoError(t, err, "writing %s", filename)
}
